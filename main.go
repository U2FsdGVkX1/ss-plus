package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"unsafe"

	"github.com/xiaoqidun/qqwry"
)

// Terminal window size structure
type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// Get terminal width
func getTerminalWidth() int {
	ws := &winsize{}
	retCode, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)))

	if int(retCode) == -1 {
		fmt.Fprintf(os.Stderr, "Warning: Could not get terminal width: %v\n", errno)
		return 80 // Default width
	}
	return int(ws.Col)
}

func downloadQQwry() bool {
	qqwryFile := "qqwry.ipdb"
	if _, err := os.Stat(qqwryFile); err == nil {
		return true
	}
	
	fmt.Println("qqwry.ipdb not found, downloading from GitHub...")
	url := "https://cdn.jsdelivr.net/npm/qqwry.raw.ipdb/qqwry.ipdb"

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("\nFailed to download qqwry.ipdb: %v\n", err)
		fmt.Printf("Please download it manually from: %s\n", url)
		return false
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("\nFailed to download qqwry.ipdb: HTTP %d\n", resp.StatusCode)
		fmt.Printf("Please download it manually from: %s\n", url)
		return false
	}
	
	file, err := os.Create(qqwryFile)
	if err != nil {
		fmt.Printf("\nFailed to create file: %v\n", err)
		return false
	}
	defer file.Close()
	
	// Simple progress indication
	fmt.Print("Downloading...")
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		fmt.Printf("\nFailed to write file: %v\n", err)
		return false
	}
	
	fmt.Println("\nDownload completed successfully!")
	return true
}

func getIPInfo(ipAddress string) string {
	if ipAddress == "" || ipAddress == "0.0.0.0" || ipAddress == "127.0.0.1" {
		return "Local/Unknown"
	}
	
	location, err := qqwry.QueryIP(ipAddress)
	if err != nil {
		return fmt.Sprintf("Query failed: %v", err)
	}

	result := strings.TrimSpace(location.Province + " " + location.City + " " + location.District + " " + location.ISP)
	if result == "" {
		return "Unknown"
	}
	return result
}

func parsePeerAddress(peerAddress string) string {
	if peerAddress == "" || peerAddress == "*" {
		return ""
	}
	
	// IPv4 format: ip:port
	if ipv4Regex := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):\d+$`); ipv4Regex.MatchString(peerAddress) {
		return ipv4Regex.FindStringSubmatch(peerAddress)[1]
	}
	
	// IPv6 format: [ip]:port
	if strings.Contains(peerAddress, "[") && strings.Contains(peerAddress, "]:") {
		if ipv6Regex := regexp.MustCompile(`^\[([^\]]+)\]:\d+$`); ipv6Regex.MatchString(peerAddress) {
			return ipv6Regex.FindStringSubmatch(peerAddress)[1]
		}
	}
	
	return ""
}

func runSSCommand(args []string) (string, error) {
	cmd := exec.Command("ss", args...)
	
	// Set terminal width and environment variables
	termWidth := getTerminalWidth()
	env := os.Environ()
	env = append(env, fmt.Sprintf("COLUMNS=%d", termWidth))
	env = append(env, "TERM=xterm")
	cmd.Env = env
	
	// Get output
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "Failed to run ss command: %v\n", err)
			fmt.Fprintf(os.Stderr, "Error output: %s\n", exitError.Stderr)
		} else {
			fmt.Fprintf(os.Stderr, "Error: ss command not found, please ensure it's installed\n")
		}
		return "", err
	}
	return string(output), nil
}

func processSSOutput(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return output
	}
	
	// Find header line
	headerIndex := -1
	for i, line := range lines {
		if strings.Contains(line, "State") || strings.Contains(line, "Recv-Q") {
			headerIndex = i
			break
		}
	}
	if headerIndex == -1 {
		return output
	}
	
	// Add IPInfo column to header
	processedLines := append(lines[:headerIndex], lines[headerIndex])
	
	// Process each data line
	peerRegex := regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}:\d+|\[[^\]]+\]:\d+|\*:\*|0\.0\.0\.0:\*)`)
	
	for i := headerIndex + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			processedLines = append(processedLines, line)
			continue
		}
		
		// Find peer address and get IP info
		matches := peerRegex.FindAllString(line, -1)
		ipInfo := "N/A"
		
		if len(matches) >= 2 {
			// Second match is usually the peer address
			if ip := parsePeerAddress(matches[1]); ip != "" {
				ipInfo = getIPInfo(ip)
			}
		} else if len(matches) == 1 {
			// Check if this is the peer address by field position
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				for j := 3; j < len(fields); j++ {
					if peerRegex.MatchString(fields[j]) {
						if ip := parsePeerAddress(fields[j]); ip != "" {
							ipInfo = getIPInfo(ip)
						}
						break
					}
				}
			}
		}
		
		processedLines = append(processedLines, strings.TrimRight(line, " \t\n\r")+" "+ipInfo)
	}
	
	return strings.Join(processedLines, "\n")
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("Usage: go run main.go <ss command arguments>")
		fmt.Println("Example: go run main.go -nltp")
		return
	}
	
	// Download and load IP database
	if !downloadQQwry() {
		fmt.Println("Cannot proceed without qqwry.ipdb file")
		os.Exit(1)
	}
	
	if err := qqwry.LoadFile("qqwry.ipdb"); err != nil {
		fmt.Printf("Failed to load qqwry.ipdb: %v\n", err)
		os.Exit(1)
	}
	
	// Run ss command and process output
	ssOutput, _ := runSSCommand(args)
	processedOutput := processSSOutput(ssOutput)
	fmt.Print(processedOutput + "\n")
}
