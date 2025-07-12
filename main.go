package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/xiaoqidun/qqwry"
)

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

	result := strings.TrimSpace(location.Country + " " + location.Province + " " + location.City + " " + location.District + " " + location.ISP)
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
	ipv4Regex := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):\d+$`)
	if matches := ipv4Regex.FindStringSubmatch(peerAddress); len(matches) > 1 {
		return matches[1]
	}
	
	// IPv6 format: [ip]:port
	if strings.Contains(peerAddress, "[") && strings.Contains(peerAddress, "]:") {
		ipv6Regex := regexp.MustCompile(`^\[([^\]]+)\]:\d+$`)
		if matches := ipv6Regex.FindStringSubmatch(peerAddress); len(matches) > 1 {
			return matches[1]
		}
	}
	
	return ""
}

func runSSCommand(args []string) (string, error) {
	cmd := exec.Command("ss", args...)
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
	
	// Find header line (look for State or Recv-Q)
	headerLine := ""
	headerIndex := -1
	for i, line := range lines {
		if strings.Contains(line, "State") || strings.Contains(line, "Recv-Q") {
			headerLine = line
			headerIndex = i
			break
		}
	}
	if headerIndex == -1 {
		return output
	}
	
	// Add IPInfo column to header
	newHeader := headerLine + "\tIPInfo"
	processedLines := append(lines[:headerIndex], newHeader)
	peerRegex := regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}:\d+|\[[^\]]+\]:\d+|\*:\*|0\.0\.0\.0:\*)`)
	
	// Process each data line
	for i := headerIndex + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			processedLines = append(processedLines, line)
			continue
		}
		
		// Find all potential peer addresses in the line
		matches := peerRegex.FindAllString(line, -1)
		peerAddress := ""
		if len(matches) >= 2 {
			peerAddress = matches[1]
		} else if len(matches) == 1 {
			// Check if this is the peer address by looking at position
			// If it's after some whitespace and fields, it's likely the peer
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				for j := 3; j < len(fields); j++ {
					if peerRegex.MatchString(fields[j]) {
						peerAddress = fields[j]
						break
					}
				}
			}
		}
		
		// Get IP info
		ipInfo := "N/A"
		if peerAddress != "" && peerAddress != "*" && peerAddress != "*:*" {
			ip := parsePeerAddress(peerAddress)
			if ip != "" {
				ipInfo = getIPInfo(ip)
			}
		}
		
		// Ensure line ends with tab + IP info
		processedLines = append(processedLines, strings.TrimRight(line, " \t\n\r")+"\t"+ipInfo)
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
	
	// Download qqwry.ipdb if not exists
	if !downloadQQwry() {
		fmt.Println("Cannot proceed without qqwry.ipdb file")
		os.Exit(1)
	}
	
	// Load IP database
	if err := qqwry.LoadFile("qqwry.ipdb"); err != nil {
		fmt.Printf("Failed to load qqwry.ipdb: %v\n", err)
		os.Exit(1)
	}
	
	// Run ss command
	ssOutput, err := runSSCommand(args)
	if err != nil {
		os.Exit(1)
	}
	
	// Process output and add IP info column
	processedOutput := processSSOutput(ssOutput)
	fmt.Print(processedOutput + "\n")
}
