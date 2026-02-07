// Package main provides a demo client for the SoundTouch service API,
// demonstrating how to interact with devices and retrieve media information.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// This example demonstrates how to interact with the soundtouch-service API
// to list discovered devices.

func main() {
	// 1. Trigger a discovery scan
	fmt.Println("Triggering discovery scan...")

	resp, err := http.Post("http://localhost:8000/setup/discover", "application/json", nil)
	if err != nil {
		log.Fatalf("Failed to trigger discovery: %v\nMake sure soundtouch-service is running on localhost:8000", err)
	}

	_ = resp.Body.Close()

	// Wait a bit for discovery to find some devices
	fmt.Println("Waiting 5 seconds for discovery...")
	time.Sleep(5 * time.Second)

	// 2. List discovered devices
	fmt.Println("Fetching discovered devices...")

	resp, err = http.Get("http://localhost:8000/setup/devices")
	if err != nil {
		log.Fatalf("Failed to fetch devices: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()

		log.Fatalf("Failed to read response body: %v", err)
	}

	_ = resp.Body.Close()

	var devices []map[string]interface{}
	if err := json.Unmarshal(body, &devices); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if len(devices) == 0 {
		fmt.Println("No devices discovered yet.")
		return
	}

	fmt.Printf("Discovered %d devices:\n", len(devices))

	for _, d := range devices {
		fmt.Printf("- %s (IP: %s, Model: %s)\n", d["name"], d["ip_address"], d["product_code"])
	}
}
