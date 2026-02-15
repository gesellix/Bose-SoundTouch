package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
	Redact     bool
	counter    uint64
	variables  map[string]string
	mu         sync.Mutex
	queue      chan recordingTask
}

type recordingTask struct {
	category     string
	req          *http.Request
	res          *http.Response
	replacements map[string]string
	dir          string
	path         string
}

// InteractionStats represents statistics for recorded interactions.
type InteractionStats struct {
	TotalRequests int            `json:"total_requests"`
	ByService     map[string]int `json:"by_service"`
	BySession     map[string]int `json:"by_session"`
}

// Interaction represents a single recorded HTTP interaction.
type Interaction struct {
	ID        string `json:"id"`
	Session   string `json:"session"`
	Category  string `json:"category"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	File      string `json:"file"`
	Counter   int    `json:"counter"`
	Status    int    `json:"status"`
	Timestamp string `json:"timestamp"`
}

// NewRecorder creates a new HTTP interaction recorder.
func NewRecorder(baseDir string) *Recorder {
	sessionID := time.Now().Format("20060102-150405") + "-" + fmt.Sprintf("%d", os.Getpid())

	r := &Recorder{
		BaseDir:   baseDir,
		SessionID: sessionID,
		Patterns:  DefaultPatterns(),
		variables: make(map[string]string),
	}

	// Use environment variable to control async recording, default to true for production
	// but allow disabling it for tests if needed.
	if os.Getenv("RECORDER_ASYNC") != "false" {
		r.queue = make(chan recordingTask, 100)
		go r.worker()
	} else {
		log.Println("[DEBUG_LOG] Recorder starting in synchronous mode")
	}

	return r
}

// Close stops the recorder and waits for pending tasks to finish.
func (r *Recorder) Close() {
	if r.queue != nil {
		close(r.queue)
		// We might want to wait here, but for now just closing is a start
	}
}

// Record logs an interaction to the configured category.
func (r *Recorder) Record(category string, req *http.Request, res *http.Response) error {
	if r.BaseDir == "" {
		return nil
	}

	sanitizedSegments, replacements := r.getSanitizedSegments(req.URL.Path)
	dir := r.getRecordingDir(category, sanitizedSegments)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	path := r.getRecordingPath(dir, req.Method)

	// Shallow copy request for the worker to avoid data races if the original is reused
	// but Note: body is already buffered/replaced in middleware if needed.
	// We need to be careful about bodies being closed.
	task := recordingTask{
		category:     category,
		req:          req,
		res:          res,
		replacements: replacements,
		dir:          dir,
		path:         path,
	}

	// For testing purposes or if queue is nil, fallback to synchronous
	if r.queue == nil {
		r.save(task)
		return nil
	}

	select {
	case r.queue <- task:
		return nil
	default:
		return fmt.Errorf("recording queue full, dropping interaction for %s", req.URL.Path)
	}
}

func (r *Recorder) save(task recordingTask) {
	var buf bytes.Buffer
	r.writeRequest(&buf, task.req, task.replacements)

	if task.res != nil {
		r.writeResponse(&buf, task.res)
	}

	if err := os.WriteFile(task.path, buf.Bytes(), 0644); err != nil {
		log.Printf("failed to write recording to %s: %v", task.path, err)
	}

	_ = r.updateEnvFile(task.replacements)
}

func (r *Recorder) worker() {
	for task := range r.queue {
		r.save(task)
	}
}

func (r *Recorder) getSanitizedSegments(path string) ([]string, map[string]string) {
	pathSegments := strings.Split(strings.Trim(path, "/"), "/")
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

	return sanitizedSegments, replacements
}

func (r *Recorder) getRecordingDir(category string, sanitizedSegments []string) string {
	subDir := "root"
	if len(sanitizedSegments) > 0 {
		subDir = filepath.Join(sanitizedSegments...)
	}

	return filepath.Join(r.BaseDir, "interactions", r.SessionID, category, subDir)
}

func (r *Recorder) getRecordingPath(dir, method string) string {
	timestamp := time.Now().Format("15-04-05.000")
	count := atomic.AddUint64(&r.counter, 1)
	filename := fmt.Sprintf("%04d-%s-%s.http", count, timestamp, method)

	return filepath.Join(dir, filename)
}

func (r *Recorder) writeRequest(buf *bytes.Buffer, req *http.Request, replacements map[string]string) {
	displayURL := req.URL.String()
	for orig, repl := range replacements {
		displayURL = strings.ReplaceAll(displayURL, orig, "{{"+strings.Trim(repl, "{}")+"}}")
	}

	fmt.Fprintf(buf, "### %s %s\n", req.Method, displayURL)

	for orig, repl := range replacements {
		key := strings.Trim(repl, "{}")
		fmt.Fprintf(buf, "// %s: %s\n", key, orig)
	}

	fmt.Fprintf(buf, "%s %s\n", req.Method, displayURL)
	fmt.Fprintf(buf, "Host: %s\n", req.Host)

	for k, vv := range req.Header {
		if r.Redact && isSensitive(k) {
			fmt.Fprintf(buf, "%s: [REDACTED]\n", k)
			continue
		}

		for _, v := range vv {
			val := v
			for orig, repl := range replacements {
				val = strings.ReplaceAll(val, orig, "{{"+strings.Trim(repl, "{}")+"}}")
			}

			fmt.Fprintf(buf, "%s: %s\n", k, val)
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
}

func (r *Recorder) writeResponse(buf *bytes.Buffer, res *http.Response) {
	buf.WriteString("\n")
	buf.WriteString("> {% \n")
	fmt.Fprintf(buf, "    // Response: %d %s\n", res.StatusCode, http.StatusText(res.StatusCode))
	buf.WriteString("    // Headers:\n")

	for k, vv := range res.Header {
		if r.Redact && isSensitive(k) {
			fmt.Fprintf(buf, "    // %s: [REDACTED]\n", k)
			continue
		}

		for _, v := range vv {
			fmt.Fprintf(buf, "    // %s: %s\n", k, v)
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
				fmt.Fprintf(buf, "\n// [Binary response body: %d bytes]\n", len(bodyBytes))
			}
		}
	}
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

// GetInteractionStats returns statistics about recorded interactions.
func (r *Recorder) GetInteractionStats() (*InteractionStats, error) {
	stats := &InteractionStats{
		ByService: make(map[string]int),
		BySession: make(map[string]int),
	}

	interactionsDir := filepath.Join(r.BaseDir, "interactions")
	if _, err := os.Stat(interactionsDir); os.IsNotExist(err) {
		return stats, nil
	}

	err := filepath.Walk(interactionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".http") {
			stats.TotalRequests++

			// Extract category (self/upstream) and session from path
			// Path is like: .../interactions/<session>/<category>/...
			rel, err := filepath.Rel(interactionsDir, path)
			if err != nil {
				return err
			}

			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) >= 2 {
				sessionID := parts[0]
				category := parts[1]
				stats.BySession[sessionID]++
				stats.ByService[category]++
			}
		}

		return nil
	})

	return stats, err
}

// ListInteractions returns a list of recorded interactions.
func (r *Recorder) ListInteractions(sessionFilter, categoryFilter, sinceFilter string) ([]Interaction, error) {
	interactions := make([]Interaction, 0)
	interactionsDir := filepath.Join(r.BaseDir, "interactions")

	if _, err := os.Stat(interactionsDir); os.IsNotExist(err) {
		return interactions, nil
	}

	err := filepath.Walk(interactionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(info.Name(), ".http") {
			return nil
		}

		rel, err := filepath.Rel(interactionsDir, path)
		if err != nil {
			return err
		}

		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 3 {
			return nil
		}

		sessionID, category := parts[0], parts[1]
		if (sessionFilter != "" && sessionID != sessionFilter) || (categoryFilter != "" && category != categoryFilter) {
			return nil
		}

		interaction, ok := r.parseInteractionFile(rel, path, parts)
		if !ok {
			return nil
		}

		if sinceFilter != "" && interaction.Timestamp != "" {
			fullTS := r.getFullTimestamp(sessionID, interaction.ID)

			normalizedSince := strings.ReplaceAll(strings.ReplaceAll(sinceFilter, ":", "-"), " ", "-")
			if fullTS != "" && fullTS < normalizedSince {
				return nil
			}
		}

		interactions = append(interactions, interaction)

		return nil
	})

	return interactions, err
}

func (r *Recorder) parseInteractionFile(rel, path string, parts []string) (Interaction, bool) {
	sessionID, category := parts[0], parts[1]
	filename := parts[len(parts)-1]
	fnParts := strings.Split(strings.TrimSuffix(filename, ".http"), "-")

	date := ""
	if len(sessionID) >= 8 {
		date = sessionID[0:4] + "-" + sessionID[4:6] + "-" + sessionID[6:8]
	}

	timestamp := ""

	if len(fnParts) >= 4 {
		timeStr := fnParts[1] + ":" + fnParts[2] + ":" + fnParts[3]
		timestamp = timeStr

		if date != "" {
			timestamp = date + " " + timeStr
		}
	}

	requestPath := "/" + strings.Join(parts[2:len(parts)-1], "/")
	if requestPath == "/root" {
		requestPath = "/"
	}

	method, counter := "UNKNOWN", 0
	if len(fnParts) >= 1 {
		_, _ = fmt.Sscanf(fnParts[0], "%d", &counter)
	}

	if len(fnParts) >= 5 {
		method = fnParts[4]
	}

	return Interaction{
		ID:        filename,
		Session:   sessionID,
		Category:  category,
		Method:    method,
		Path:      requestPath,
		File:      rel,
		Counter:   counter,
		Status:    r.peekStatus(path),
		Timestamp: timestamp,
	}, true
}

func (r *Recorder) getFullTimestamp(sessionID, filename string) string {
	if len(sessionID) < 8 {
		return ""
	}

	date := sessionID[0:4] + "-" + sessionID[4:6] + "-" + sessionID[6:8]
	fnParts := strings.Split(strings.TrimSuffix(filename, ".http"), "-")

	if len(fnParts) < 4 {
		return ""
	}

	return date + "-" + fnParts[1] + "-" + fnParts[2] + "-" + fnParts[3]
}

func (r *Recorder) peekStatus(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if !strings.Contains(line, "// Response:") {
			continue
		}

		trimmedLine := strings.TrimPrefix(strings.TrimSpace(line), "//")
		trimmedLine = strings.TrimPrefix(strings.TrimSpace(trimmedLine), "Response:")
		trimmedLine = strings.TrimSpace(trimmedLine)

		status := 0
		_, _ = fmt.Sscanf(trimmedLine, "%d", &status)

		return status
	}

	return 0
}

// DeleteSession deletes a specific recording session.
func (r *Recorder) DeleteSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}

	sessionDir := filepath.Join(r.BaseDir, "interactions", sessionID)

	return os.RemoveAll(sessionDir)
}

// CleanupSessions deletes all but the most recent keepCount sessions.
func (r *Recorder) CleanupSessions(keepCount int) error {
	interactionsDir := filepath.Join(r.BaseDir, "interactions")

	entries, err := os.ReadDir(interactionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	var sessions []os.DirEntry

	for _, entry := range entries {
		if entry.IsDir() {
			sessions = append(sessions, entry)
		}
	}

	if len(sessions) <= keepCount {
		return nil
	}

	// Sort sessions by name (timestamp) descending to keep the newest ones
	// Session ID format: 20260102-150405-PID
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name() > sessions[j].Name()
	})

	for i := keepCount; i < len(sessions); i++ {
		sessionDir := filepath.Join(interactionsDir, sessions[i].Name())
		if err := os.RemoveAll(sessionDir); err != nil {
			return fmt.Errorf("failed to delete session %s: %w", sessions[i].Name(), err)
		}
	}

	return nil
}

// GetInteractionContent returns the raw content of a recorded interaction.
func (r *Recorder) GetInteractionContent(relPath string) ([]byte, error) {
	fullPath := filepath.Join(r.BaseDir, "interactions", relPath)
	return os.ReadFile(fullPath)
}
