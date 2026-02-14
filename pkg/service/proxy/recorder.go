package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Recorder handles persisting HTTP interactions as .http files.
type Recorder struct {
	BaseDir    string
	SessionID  string
	SessionDir string
	Patterns   PathPatterns
	counter    uint64
	variables  map[string]string
	mu         sync.Mutex
}

// NewRecorder creates a new HTTP interaction recorder.
func NewRecorder(baseDir string) *Recorder {
	sessionID := time.Now().Format("20060102-150405") + "-" + fmt.Sprintf("%d", os.Getpid())
	return &Recorder{
		BaseDir:   baseDir,
		SessionID: sessionID,
		Patterns:  DefaultPatterns(),
		variables: make(map[string]string),
	}
}

// Record persists a request and response to a .http file in the specified category (e.g., "self" or "upstream").
func (r *Recorder) Record(category string, req *http.Request, res *http.Response) error {
	if r.BaseDir == "" {
		return nil
	}

	// Group by URL path, sanitizing variable segments like IP addresses
	pathSegments := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	sanitizedSegments := make([]string, 0, len(pathSegments))
	replacements := make(map[string]string)
	for _, segment := range pathSegments {
		if segment == "" {
			continue
		}

		sanitized, replacement := r.Patterns.Sanitize(segment)
		sanitizedSegments = append(sanitizedSegments, sanitized)
		if replacement != "" {
			replacements[segment] = replacement
		}
	}

	subDir := "root"
	if len(sanitizedSegments) > 0 {
		subDir = filepath.Join(sanitizedSegments...)
	}

	dir := filepath.Join(r.BaseDir, "interactions", r.SessionID, category, subDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	timestamp := time.Now().Format("15-04-05.000")
	count := atomic.AddUint64(&r.counter, 1)
	filename := fmt.Sprintf("%04d-%s-%s.http", count, timestamp, req.Method)
	path := filepath.Join(dir, filename)

	var buf bytes.Buffer

	// Write Request
	displayURL := req.URL.String()
	for orig, repl := range replacements {
		displayURL = strings.ReplaceAll(displayURL, orig, "{{"+strings.Trim(repl, "{}")+"}}")
	}

	buf.WriteString(fmt.Sprintf("### %s %s\n", req.Method, displayURL))
	buf.WriteString(fmt.Sprintf("%s %s\n", req.Method, displayURL))
	for k, vv := range req.Header {
		for _, v := range vv {
			val := v
			for orig, repl := range replacements {
				val = strings.ReplaceAll(val, orig, "{{"+strings.Trim(repl, "{}")+"}}")
			}
			buf.WriteString(fmt.Sprintf("%s: %s\n", k, val))
		}
	}
	buf.WriteString("\n")

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil {
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			buf.Write(bodyBytes)
			buf.WriteString("\n")
		}
	}

	// Write Response if available
	if res != nil {
		buf.WriteString("\n")
		buf.WriteString("> {% \n")
		buf.WriteString(fmt.Sprintf("    // Response: %d %s\n", res.StatusCode, http.StatusText(res.StatusCode)))
		buf.WriteString("    // Headers:\n")
		for k, vv := range res.Header {
			for _, v := range vv {
				buf.WriteString(fmt.Sprintf("    // %s: %s\n", k, v))
			}
		}
		buf.WriteString("%}\n")

		if res.Body != nil {
			bodyBytes, err := io.ReadAll(res.Body)
			if err == nil {
				res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				contentType := res.Header.Get("Content-Type")
				if strings.Contains(contentType, "xml") || strings.Contains(contentType, "json") || strings.Contains(contentType, "text") {
					buf.WriteString("\n/*\n")
					buf.Write(bodyBytes)
					buf.WriteString("\n*/\n")
				} else {
					buf.WriteString(fmt.Sprintf("\n// [Binary response body: %d bytes]\n", len(bodyBytes)))
				}
			}
		}
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}

	return r.updateEnvFile(replacements)
}

func (r *Recorder) updateEnvFile(newVars map[string]string) error {
	if len(newVars) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	changed := false
	for orig, repl := range newVars {
		key := strings.Trim(repl, "{}")
		if r.variables[key] != orig {
			r.variables[key] = orig
			changed = true
		}
	}

	if !changed {
		return nil
	}

	envFile := filepath.Join(r.BaseDir, "interactions", r.SessionID, "http-client.env.json")

	// Create the structure: {"session": {"key": "val"}}
	content := map[string]map[string]string{
		"session": r.variables,
	}

	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(envFile, data, 0644)
}
