// Package main demonstrates the new recording filename format that includes date information.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
)

func main() {
	fmt.Println("=== Recording Filename Format Demo ===")
	fmt.Println()

	// Create a temporary directory for the demo
	tmpDir, err := os.MkdirTemp("", "recording-filename-demo")
	if err != nil {
		log.Fatalf("Failed to create temp directory: %v", err)
	}

	defer func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			log.Printf("Failed to remove temp directory: %v", removeErr)
		}
	}()

	fmt.Printf("Demo recordings will be saved to: %s\n\n", tmpDir)

	// Create a recorder with async disabled for predictable demo output
	if envErr := os.Setenv("RECORDER_ASYNC", "false"); envErr != nil {
		log.Printf("Failed to set environment variable: %v", envErr)
	}

	recorder := proxy.NewRecorder(tmpDir)
	defer recorder.Close()

	fmt.Printf("Recorder session ID: %s\n", recorder.SessionID)
	fmt.Println()

	// Create some sample HTTP requests to record
	requests := []struct {
		method   string
		path     string
		category string
	}{
		{"GET", "/info", "self"},
		{"POST", "/volume", "self"},
		{"GET", "/nowPlaying", "self"},
		{"PUT", "/preset_1", "self"},
	}

	fmt.Println("Recording sample HTTP interactions...")
	fmt.Println()

	for i, req := range requests {
		// Create a mock HTTP request
		httpReq, reqErr := http.NewRequest(req.method, "http://soundtouch.local:8090"+req.path, nil)
		if reqErr != nil {
			log.Printf("Failed to create request: %v", reqErr)
			continue
		}

		// Create a mock response
		httpRes := &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Request:    httpReq,
		}
		httpRes.Header.Set("Content-Type", "application/xml")

		// Record the interaction
		err = recorder.Record(req.category, httpReq, httpRes)
		if err != nil {
			log.Printf("Failed to record interaction: %s", sanitizeErr(err))
			continue
		}

		fmt.Printf("%d. Recorded: %s %s\n", i+1, req.method, req.path)

		// Small delay to show different timestamps
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println()
	fmt.Println("=== Generated Filenames ===")
	fmt.Println()

	// Walk through the recordings directory to show the generated filenames
	interactionsDir := filepath.Join(tmpDir, "interactions")

	err = filepath.Walk(interactionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(info.Name(), ".http") {
			// Get relative path from interactions directory
			rel, _ := filepath.Rel(interactionsDir, path)
			fmt.Printf("📁 %s\n", rel)

			// Parse and explain the filename format
			filename := info.Name()
			parts := strings.Split(strings.TrimSuffix(filename, ".http"), "-")

			if len(parts) == 4 && len(parts[1]) == 8 {
				// New format: count-yyyyMMdd-HHMMSS.sss-method.http
				counter := parts[0]
				dateStr := parts[1]
				timeStr := parts[2]
				method := parts[3]

				// Format for display
				date := dateStr[0:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
				time := timeStr[0:2] + ":" + timeStr[2:4] + ":" + timeStr[4:]

				fmt.Printf("   📋 Format: count-yyyyMMdd-HHMMSS.sss-method.http\n")
				fmt.Printf("   🔢 Counter: %s\n", counter)
				fmt.Printf("   📅 Date: %s (from %s)\n", date, dateStr)
				fmt.Printf("   🕒 Time: %s (from %s)\n", time, timeStr)
				fmt.Printf("   🔧 Method: %s\n", method)
				fmt.Printf("   ✨ Full timestamp: %s %s\n", date, time)
			} else {
				fmt.Printf("   ⚠️  Legacy format or unexpected structure\n")
			}

			fmt.Println()
		}

		return nil
	})
	if err != nil {
		log.Printf("Error walking directory: %v", err)
	}

	fmt.Println("=== Comparison with Old Format ===")
	fmt.Println()
	fmt.Println("🔴 OLD format (time only): 0047-21-53-06.128-GET.http")
	fmt.Println("   - No date information in filename")
	fmt.Println("   - Date extracted from session ID directory")
	fmt.Println("   - Confusing when recordings span midnight")
	fmt.Println()
	fmt.Println("🟢 NEW format (date + time): 0047-20260223-215306.128-GET.http")
	fmt.Println("   - Complete timestamp in filename")
	fmt.Println("   - Self-contained, no need to check directory")
	fmt.Println("   - Clear chronological ordering")
	fmt.Println()

	fmt.Println("=== Benefits ===")
	fmt.Println("✅ No confusion when recordings cross midnight")
	fmt.Println("✅ Complete timestamp visible at a glance")
	fmt.Println("✅ Better sorting and organization")
	fmt.Println("✅ Backwards compatible with existing parsing logic")
	fmt.Println()

	// Test the list interactions functionality
	fmt.Println("=== Using ListInteractions API ===")
	fmt.Println()

	interactions, err := recorder.ListInteractions("", "", "")
	if err != nil {
		log.Printf("Failed to list interactions: %v", err)
		return
	}

	fmt.Printf("Found %d recorded interactions:\n", len(interactions))

	for i := range interactions {
		interaction := &interactions[i]
		fmt.Printf("%d. %s %s - %s (File: %s)\n",
			i+1, interaction.Method, interaction.Path,
			interaction.Timestamp, interaction.ID)
	}

	fmt.Printf("\nDemo completed! Recordings saved in: %s\n", tmpDir)
	fmt.Println("You can explore the generated files to see the new format in action.")
}
