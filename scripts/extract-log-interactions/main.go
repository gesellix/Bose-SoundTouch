package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type Interaction struct {
	Type      string      `json:"type"`      // ToNative, FromNative, ToNetwork
	Payload   string      `json:"payload"`   // Raw string
	Parsed    interface{} `json:"parsed"`    // JSON if possible
	Timestamp string      `json:"timestamp"` // If available
	ID        string      `json:"id"`        // Internal ID if available
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run extract-log-interactions.go <log_file>")
		return
	}

	logFile := os.Args[1]
	file, err := os.Open(logFile)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	// Regex patterns
	toNativeRegex := regexp.MustCompile(`To Native :  "(.*)"`)
	fromNativeRegex := regexp.MustCompile(`From Native :  "(.*)"`)
	toNetworkRegex := regexp.MustCompile(`To Network "(.*)"`)
	timestampRegex := regexp.MustCompile(`Js_Console_Msg:  "(\d{2}:\d{2}:\d{2}\.\d{3})`)

	scanner := bufio.NewScanner(file)
	lastTimestamp := ""

	fmt.Println("### Bose SoundTouch Internal Log Interactions")
	fmt.Println("-------------------------------------------------")

	for scanner.Scan() {
		line := scanner.Text()

		// Track timestamp from console msgs
		if tsMatch := timestampRegex.FindStringSubmatch(line); len(tsMatch) > 1 {
			lastTimestamp = tsMatch[1]
		}

		if match := toNetworkRegex.FindStringSubmatch(line); len(match) > 1 {
			printAppInteraction("TO NETWORK", match[1], lastTimestamp, "")
		} else if match := toNativeRegex.FindStringSubmatch(line); len(match) > 1 {
			payload := cleanPayload(match[1])
			id := extractID(payload)
			printAppInteraction("TO NATIVE", payload, lastTimestamp, id)
		} else if match := fromNativeRegex.FindStringSubmatch(line); len(match) > 1 {
			payload := cleanPayload(match[1])
			id := extractID(payload)
			printAppInteraction("FROM NATIVE", payload, lastTimestamp, id)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading file: %v\n", err)
	}
}

func cleanPayload(p string) string {
	// Remove escaped quotes and leading/trailing quotes
	p = strings.ReplaceAll(p, `\"`, `"`)
	return p
}

func extractID(p string) string {
	// Try to find "id":X
	idRegex := regexp.MustCompile(`"id":\s*(\d+)`)
	match := idRegex.FindStringSubmatch(p)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func printAppInteraction(typ, payload, ts, id string) {
	fmt.Printf("\n### %s", typ)
	if ts != "" {
		fmt.Printf(" [%s]", ts)
	}
	if id != "" {
		fmt.Printf(" (ID: %s)", id)
	}
	fmt.Println()

	// Try to prettify if it's JSON
	var obj interface{}
	if err := json.Unmarshal([]byte(payload), &obj); err == nil {
		pretty, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Printf("/*\n%s\n*/\n", string(pretty))
	} else {
		// Just print raw (might be XML or plain text)
		fmt.Printf("/*\n%s\n*/\n", payload)
	}
	fmt.Println("-------------------------------------------------")
}
