package main

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/go-chi/chi/v5"
)

func TestPrintRoutes(t *testing.T) {
	// Initialize a minimal server to get the router
	server := handlers.NewServer(nil, nil, "http://localhost:8000", true, true, true)
	r := setupRouter(server)

	var routes []string
	walkFunc := func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		route = strings.ReplaceAll(route, "/*/", "/")
		handlerName := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
		// Clean up the handler name (remove package path)
		// For example, "github.com/gesellix/bose-soundtouch/cmd/soundtouch-service.setupRouter.func1"
		// or "command-line-arguments.setupRouter.func1"
		// or "main.setupRouter.func1"
		parts := strings.Split(handlerName, "/")
		if len(parts) > 0 {
			handlerName = parts[len(parts)-1]
		}
		// Now we might have "soundtouch-service.setupRouter.func1"
		// or "command-line-arguments.setupRouter.func1"
		// or "main.setupRouter.func1"
		// Let's remove the first part if it's a known varying package name
		if idx := strings.Index(handlerName, "setupRouter"); idx != -1 {
			handlerName = handlerName[idx:]
		}
		// In case it's not setupRouter but still has a package prefix
		for {
			dotIdx := strings.Index(handlerName, ".")
			if dotIdx == -1 {
				break
			}
			prefix := handlerName[:dotIdx]
			if prefix == "main" || prefix == "command-line-arguments" || strings.Contains(prefix, "soundtouch-service") {
				handlerName = handlerName[dotIdx+1:]
			} else {
				break
			}
		}

		// Also remove any ".funcN" suffix if it's an anonymous function
		if idx := strings.Index(handlerName, ".func"); idx != -1 {
			handlerName = handlerName[:idx]
		}

		routes = append(routes, fmt.Sprintf("%-8s %-60s %s", method, route, handlerName))
		return nil
	}

	if err := chi.Walk(r, walkFunc); err != nil {
		t.Fatalf("Failed to walk routes: %v", err)
	}

	sort.Strings(routes)

	output := strings.Join(routes, "\n") + "\n"

	// Define snapshot path
	snapshotPath := "testdata/router_routes.txt"
	actualPath := "testdata/router_routes.actual.txt"

	// Always write the current (actual) routes to a file
	if err := os.WriteFile(actualPath, []byte(output), 0644); err != nil {
		t.Fatalf("Failed to write actual routes: %v", err)
	}

	// Check if snapshot exists
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		// Create testdata directory if it doesn't exist
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatalf("Failed to create testdata directory: %v", err)
		}
		// Initial snapshot creation
		if err := os.WriteFile(snapshotPath, []byte(output), 0644); err != nil {
			t.Fatalf("Failed to write snapshot: %v", err)
		}
		t.Logf("Initial snapshot created at %s", snapshotPath)
		return
	}

	// Read existing snapshot
	existingOutput, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to read snapshot: %v", err)
	}

	if string(existingOutput) != output {
		t.Errorf("Router routes changed! Diff the snapshot at %s with %s", snapshotPath, actualPath)
	}
}
