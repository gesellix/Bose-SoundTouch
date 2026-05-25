package proxy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

	// rootMu guards lazy initialisation of root.
	rootMu sync.Mutex
	// root is an os.Root anchored at BaseDir; all filesystem operations
	// that take a caller-derivable path go through it so the Go runtime
	// guarantees containment regardless of what the path string contains.
	root *os.Root
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
	ID         string               `json:"id"`
	Session    string               `json:"session"`
	Category   string               `json:"category"`
	Method     string               `json:"method"`
	Path       string               `json:"path"`
	File       string               `json:"file"`
	Counter    int                  `json:"counter"`
	Status     int                  `json:"status"`
	Timestamp  string               `json:"timestamp"`
	SCMUDCData *EnrichedSCMUDCEvent `json:"scmudc_data,omitempty"`
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
		log.Println("Recorder starting in synchronous mode")
	}

	return r
}

// Close stops the recorder and waits for pending tasks to finish.
func (r *Recorder) Close() {
	if r.queue != nil {
		close(r.queue)
		// We might want to wait here, but for now just closing is a start
	}

	r.rootMu.Lock()
	defer r.rootMu.Unlock()

	if r.root != nil {
		_ = r.root.Close()
		r.root = nil
	}
}

// getRoot lazily opens the *os.Root anchored at r.BaseDir. The directory is
// MkdirAll-created on first call.
func (r *Recorder) getRoot() (*os.Root, error) {
	r.rootMu.Lock()
	defer r.rootMu.Unlock()

	if r.root != nil {
		return r.root, nil
	}

	if r.BaseDir == "" {
		return nil, fmt.Errorf("recorder: BaseDir not configured")
	}

	if err := os.MkdirAll(r.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("recorder: ensure BaseDir %s: %w", r.BaseDir, err)
	}

	root, err := os.OpenRoot(r.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("recorder: open root at %s: %w", r.BaseDir, err)
	}

	r.root = root

	return root, nil
}

// rootRel converts an absolute path under r.BaseDir to its root-relative form.
func (r *Recorder) rootRel(absPath string) (string, error) {
	if !filepath.IsAbs(absPath) {
		a, err := filepath.Abs(absPath)
		if err != nil {
			return "", err
		}

		absPath = a
	}

	if absPath == r.BaseDir {
		return ".", nil
	}

	rel, err := filepath.Rel(r.BaseDir, absPath)
	if err != nil {
		return "", fmt.Errorf("recorder: %s outside BaseDir: %w", absPath, err)
	}

	if rel == "." || rel == "" {
		return ".", nil
	}

	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("recorder: %s outside BaseDir", absPath)
	}

	return rel, nil
}

func (r *Recorder) rootMkdirAll(absPath string, perm os.FileMode) error {
	root, err := r.getRoot()
	if err != nil {
		return err
	}

	rel, err := r.rootRel(absPath)
	if err != nil {
		return err
	}

	if rel == "." {
		return nil
	}

	return root.MkdirAll(rel, perm)
}

func (r *Recorder) rootWriteFile(absPath string, data []byte, perm os.FileMode) error {
	root, err := r.getRoot()
	if err != nil {
		return err
	}

	rel, err := r.rootRel(absPath)
	if err != nil {
		return err
	}

	return root.WriteFile(rel, data, perm)
}

func (r *Recorder) rootReadFile(absPath string) ([]byte, error) {
	root, err := r.getRoot()
	if err != nil {
		return nil, err
	}

	rel, err := r.rootRel(absPath)
	if err != nil {
		return nil, err
	}

	return root.ReadFile(rel)
}

func (r *Recorder) rootStat(absPath string) (os.FileInfo, error) {
	root, err := r.getRoot()
	if err != nil {
		return nil, err
	}

	rel, err := r.rootRel(absPath)
	if err != nil {
		return nil, err
	}

	return root.Stat(rel)
}

func (r *Recorder) rootRemoveAll(absPath string) error {
	root, err := r.getRoot()
	if err != nil {
		return err
	}

	rel, err := r.rootRel(absPath)
	if err != nil {
		return err
	}

	return root.RemoveAll(rel)
}

func (r *Recorder) rootReadDir(absPath string) ([]os.DirEntry, error) {
	root, err := r.getRoot()
	if err != nil {
		return nil, err
	}

	rel, err := r.rootRel(absPath)
	if err != nil {
		return nil, err
	}

	d, err := root.Open(rel)
	if err != nil {
		return nil, err
	}

	defer func() { _ = d.Close() }()

	// *os.File.ReadDir(-1) returns directory order; os.ReadDir sorts by
	// name. Match the sorted contract so callers don't see a surprise.
	entries, err := d.ReadDir(-1)
	if err != nil {
		return entries, err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	return entries, nil
}

func (r *Recorder) rootOpen(absPath string) (*os.File, error) {
	root, err := r.getRoot()
	if err != nil {
		return nil, err
	}

	rel, err := r.rootRel(absPath)
	if err != nil {
		return nil, err
	}

	return root.Open(rel)
}

// Record logs an interaction to the configured category.
func (r *Recorder) Record(category string, req *http.Request, res *http.Response) error {
	if r.BaseDir == "" {
		return nil
	}

	sanitizedSegments, replacements := r.getSanitizedSegments(req.URL.Path)

	dir, err := r.getRecordingDir(category, sanitizedSegments)
	if err != nil {
		return err
	}

	if err := r.rootMkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	path := r.getRecordingPath(dir, req.Method)

	// If we are in async mode, we MUST copy the bodies now because the caller
	// might close them as soon as Record() returns.
	var (
		clonedReq *http.Request
		clonedRes *http.Response
	)

	if r.queue != nil {
		// Clone request
		clonedReq = req.Clone(req.Context())
		if req.Body != nil {
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				log.Printf("failed to read request body for async recording: %v", err)

				clonedReq.Body = http.NoBody
			} else {
				// Reset original body for subsequent consumers (though Record is usually called at the end)
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				// Set body for async task
				clonedReq.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				clonedReq.ContentLength = int64(len(bodyBytes))
			}
		}

		// Clone response if present
		if res != nil {
			clonedRes = &http.Response{
				StatusCode: res.StatusCode,
				Header:     res.Header.Clone(),
				Request:    clonedReq,
			}
			if res.Body != nil {
				bodyBytes, err := io.ReadAll(res.Body)
				if err != nil {
					log.Printf("failed to read response body for async recording: %v", err)

					res.Body = http.NoBody
					clonedRes.Body = http.NoBody
				} else {
					res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
					clonedRes.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
					clonedRes.ContentLength = int64(len(bodyBytes))
				}
			}
		}
	} else {
		clonedReq = req
		clonedRes = res
	}

	task := recordingTask{
		category:     category,
		req:          clonedReq,
		res:          clonedRes,
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
	var (
		buf      bytes.Buffer
		enriched *EnrichedSCMUDCEvent
	)

	// Check if this is a SCMUDC request and enrich it

	if strings.Contains(task.req.URL.Path, "/v1/scmudc/") && task.req.Body != nil {
		bodyBytes, err := io.ReadAll(task.req.Body)
		if err == nil {
			task.req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			enriched = enrichSCMUDCRequest(bodyBytes)
		}
	}

	r.writeRequestWithEnrichment(&buf, task.req, task.replacements, enriched)

	if task.res != nil {
		r.writeResponseWithEnrichment(&buf, task.res, enriched)
	}

	if err := r.rootWriteFile(task.path, buf.Bytes(), 0644); err != nil {
		log.Printf("failed to write recording to %s: %s", sanitizeLog(task.path), sanitizeErr(err))
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

// safeJoin joins r.BaseDir with elem and refuses to construct paths that
// would escape BaseDir. Each element must satisfy filepath.IsLocal — i.e.
// it must not be absolute, must not contain ".." segments, and (on Windows)
// must not name a reserved device. CodeQL recognises filepath.IsLocal as
// a path-traversal sanitiser, so taint analysis at call sites that hand the
// result to os.* terminates here.
func (r *Recorder) safeJoin(elem ...string) (string, error) {
	if r.BaseDir == "" {
		return "", fmt.Errorf("recorder: BaseDir not configured")
	}

	for _, e := range elem {
		if e == "" {
			// filepath.Join silently skips empty components, but
			// filepath.IsLocal returns false for "" — treat empties as
			// no-ops to preserve the existing call shapes.
			continue
		}

		if !filepath.IsLocal(e) {
			return "", fmt.Errorf("recorder: path component %q escapes BaseDir", e)
		}
	}

	return filepath.Join(append([]string{r.BaseDir}, elem...)...), nil
}

func (r *Recorder) getRecordingDir(category string, sanitizedSegments []string) (string, error) {
	subDir := "root"
	if len(sanitizedSegments) > 0 {
		subDir = filepath.Join(sanitizedSegments...)
	}

	return r.safeJoin("interactions", r.SessionID, category, subDir)
}

func (r *Recorder) getRecordingPath(dir, method string) string {
	timestamp := time.Now().Format("20060102-150405.000")
	count := atomic.AddUint64(&r.counter, 1)
	filename := fmt.Sprintf("%04d-%s-%s.http", count, timestamp, method)

	return filepath.Join(dir, filename)
}

func (r *Recorder) writeRequestWithEnrichment(buf *bytes.Buffer, req *http.Request, replacements map[string]string, enriched *EnrichedSCMUDCEvent) {
	displayURL := req.URL.String()
	for orig, repl := range replacements {
		displayURL = strings.ReplaceAll(displayURL, orig, "{{"+strings.Trim(repl, "{}")+"}}")
	}

	fmt.Fprintf(buf, "### %s %s\n", req.Method, displayURL)

	for orig, repl := range replacements {
		key := strings.Trim(repl, "{}")
		fmt.Fprintf(buf, "// %s: %s\n", key, orig)
	}

	// Add SCMUDC enriched comments
	if enriched != nil {
		enrichedComments := generateSCMUDCComments(enriched)
		for _, comment := range enrichedComments {
			fmt.Fprintf(buf, "%s\n", comment)
		}
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

func (r *Recorder) writeResponseWithEnrichment(buf *bytes.Buffer, res *http.Response, enriched *EnrichedSCMUDCEvent) {
	buf.WriteString("\n")
	buf.WriteString("> {% \n")
	fmt.Fprintf(buf, "    // Response: %d %s\n", res.StatusCode, http.StatusText(res.StatusCode))

	// Add SCMUDC enrichment summary in response
	if enriched != nil {
		buf.WriteString("    //\n")
		buf.WriteString("    // SCMUDC Event Analysis:\n")
		fmt.Fprintf(buf, "    // - Origin: %s (%s)\n", getOriginDescription(enriched.Origin), enriched.Origin)
		fmt.Fprintf(buf, "    // - Action: %s\n", enriched.Action)
		fmt.Fprintf(buf, "    // - Summary: %s\n", enriched.Summary)

		if enriched.DecodedData != nil {
			fmt.Fprintf(buf, "    // - Content: %s\n", enriched.DecodedData.ItemName)

			if enriched.DecodedData.SourceAccount != "" {
				fmt.Fprintf(buf, "    // - Account: %s\n", enriched.DecodedData.SourceAccount)
			}
		}
	}

	buf.WriteString("    //\n")
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
			} else if len(bodyBytes) > 0 {
				fmt.Fprintf(buf, "\n[Binary response body: %d bytes]\n", len(bodyBytes))
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

	return r.rootWriteFile(envFile, data, 0644)
}

// GetInteractionStats returns statistics about recorded interactions.
func (r *Recorder) GetInteractionStats() (*InteractionStats, error) {
	stats := &InteractionStats{
		ByService: make(map[string]int),
		BySession: make(map[string]int),
	}

	interactionsDir := filepath.Join(r.BaseDir, "interactions")
	if _, err := r.rootStat(interactionsDir); os.IsNotExist(err) {
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

	if _, err := r.rootStat(interactionsDir); os.IsNotExist(err) {
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

	timestamp := ""
	method, counter := "UNKNOWN", 0

	if len(fnParts) >= 1 {
		_, _ = fmt.Sscanf(fnParts[0], "%d", &counter)
	}

	// Check if this is the new format: count-yyyyMMdd-HHMMSS.sss-method.http
	// New format has 4 parts and the second part is 8 digits (yyyyMMdd)
	if len(fnParts) == 4 && len(fnParts[1]) == 8 {
		dateStr := fnParts[1] // yyyyMMdd
		timeStr := fnParts[2] // HHMMSS.sss
		method = fnParts[3]

		// Format date: yyyyMMdd -> yyyy-MM-dd
		date := dateStr[0:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]

		// Format time: HHMMSS.sss -> HH:MM:SS.sss
		if len(timeStr) >= 6 {
			time := timeStr[0:2] + ":" + timeStr[2:4] + ":" + timeStr[4:]
			timestamp = date + " " + time
		}
	} else if len(fnParts) >= 5 {
		// Legacy format: count-HH-MM-SS.sss-method.http
		// Extract date from sessionID for backward compatibility
		date := ""
		if len(sessionID) >= 8 {
			date = sessionID[0:4] + "-" + sessionID[4:6] + "-" + sessionID[6:8]
		}

		timeStr := fnParts[1] + ":" + fnParts[2] + ":" + fnParts[3]
		timestamp = timeStr

		if date != "" {
			timestamp = date + " " + timeStr
		}

		method = fnParts[4]
	}

	requestPath := "/" + strings.Join(parts[2:len(parts)-1], "/")
	if requestPath == "/root" {
		requestPath = "/"
	}

	interaction := Interaction{
		ID:        filename,
		Session:   sessionID,
		Category:  category,
		Method:    method,
		Path:      requestPath,
		File:      rel,
		Counter:   counter,
		Status:    r.peekStatus(path),
		Timestamp: timestamp,
	}

	// Extract SCMUDC enrichment data if this is a SCMUDC request
	if strings.Contains(requestPath, "/v1/scmudc/") {
		interaction.SCMUDCData = r.extractSCMUDCFromFile(path)
	}

	return interaction, true
}

// extractSCMUDCFromFile parses SCMUDC enrichment data from a .http file
func (r *Recorder) extractSCMUDCFromFile(path string) *EnrichedSCMUDCEvent {
	content, err := r.rootReadFile(path)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(content), "\n")

	var (
		enriched    EnrichedSCMUDCEvent
		foundSCMUDC bool
		bodyStart   int
	)

	// Look for SCMUDC enrichment comments

	for i, line := range lines {
		line = strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "// Origin: "):
			parts := strings.Split(line, " (")
			if len(parts) >= 2 {
				enriched.Origin = strings.TrimSuffix(parts[1], ")")
				foundSCMUDC = true
			}
		case strings.HasPrefix(line, "// Action: "):
			enriched.Action = strings.TrimPrefix(line, "// Action: ")
		case strings.HasPrefix(line, "// Command: "):
			enriched.Command = strings.TrimPrefix(line, "// Command: ")
		case strings.HasPrefix(line, "// Summary: "):
			enriched.Summary = strings.TrimPrefix(line, "// Summary: ")
		case strings.HasPrefix(line, "// - Source: "):
			r.ensureDecodedData(&enriched)
			enriched.DecodedData.ContentType = strings.TrimPrefix(line, "// - Source: ")
		case strings.HasPrefix(line, "// - Item: "):
			r.ensureDecodedData(&enriched)
			enriched.DecodedData.ItemName = strings.TrimPrefix(line, "// - Item: ")
		case strings.HasPrefix(line, "// - Account: "):
			r.ensureDecodedData(&enriched)
			enriched.DecodedData.SourceAccount = strings.TrimPrefix(line, "// - Account: ")
		case strings.HasPrefix(line, "// - Artwork: "):
			r.ensureDecodedData(&enriched)
			enriched.DecodedData.ArtworkURL = strings.TrimPrefix(line, "// - Artwork: ")
		case line == "// - Presetable: Yes":
			r.ensureDecodedData(&enriched)
			enriched.DecodedData.IsPresetable = true
		case line == "{" && i > 0:
			// Found start of JSON body
			bodyStart = i
			goto endLoop
		}
	}

endLoop:
	// If we didn't find enrichment comments but this is a SCMUDC request,
	// try to parse the JSON body directly
	if !foundSCMUDC && bodyStart > 0 {
		if parsed := r.parseSCMUDBody(lines, bodyStart); parsed != nil {
			return parsed
		}
	}

	if !foundSCMUDC {
		return nil
	}

	return &enriched
}

func (r *Recorder) ensureDecodedData(enriched *EnrichedSCMUDCEvent) {
	if enriched.DecodedData == nil {
		enriched.DecodedData = &DecodedContent{}
	}
}

func (r *Recorder) parseSCMUDBody(lines []string, bodyStart int) *EnrichedSCMUDCEvent {
	var bodyLines []string

	inBody := false
	braceCount := 0

	for i := bodyStart; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "{" && !inBody {
			inBody = true

			bodyLines = append(bodyLines, line)
			braceCount = 1
		} else if inBody {
			bodyLines = append(bodyLines, line)

			braceCount += strings.Count(line, "{") - strings.Count(line, "}")
			if braceCount == 0 {
				break
			}
		}
	}

	if len(bodyLines) > 0 {
		bodyJSON := strings.Join(bodyLines, "\n")
		return enrichSCMUDCRequest([]byte(bodyJSON))
	}

	return nil
}

func (r *Recorder) getFullTimestamp(sessionID, filename string) string {
	fnParts := strings.Split(strings.TrimSuffix(filename, ".http"), "-")

	// Check if this is the new format: count-yyyyMMdd-HHMMSS.sss-method.http
	// New format has 4 parts and the second part is 8 digits (yyyyMMdd)
	if len(fnParts) == 4 && len(fnParts[1]) == 8 {
		dateStr := fnParts[1] // yyyyMMdd
		timeStr := fnParts[2] // HHMMSS.sss

		if len(timeStr) >= 6 {
			date := dateStr[0:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
			time := timeStr[0:2] + "-" + timeStr[2:4] + "-" + timeStr[4:]

			return date + "-" + time
		}
	} else if len(fnParts) >= 5 {
		// Legacy format: count-HH-MM-SS.sss-method.http
		if len(sessionID) < 8 {
			return ""
		}

		date := sessionID[0:4] + "-" + sessionID[4:6] + "-" + sessionID[6:8]

		return date + "-" + fnParts[1] + "-" + fnParts[2] + "-" + fnParts[3]
	}

	return ""
}

func (r *Recorder) peekStatus(path string) int {
	content, err := r.rootReadFile(path)
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

	sessionDir, err := r.safeJoin("interactions", sessionID)
	if err != nil {
		return err
	}

	return r.rootRemoveAll(sessionDir)
}

// CleanupSessions deletes all but the most recent keepCount sessions.
func (r *Recorder) CleanupSessions(keepCount int) error {
	interactionsDir := filepath.Join(r.BaseDir, "interactions")

	entries, err := r.rootReadDir(interactionsDir)
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
		if err := r.rootRemoveAll(sessionDir); err != nil {
			return fmt.Errorf("failed to delete session %s: %w", sessions[i].Name(), err)
		}
	}

	return nil
}

// GetInteractionContent returns the raw content of a recorded interaction.
func (r *Recorder) GetInteractionContent(relPath string) ([]byte, error) {
	fullPath, err := r.safeJoin("interactions", relPath)
	if err != nil {
		return nil, err
	}

	return r.rootReadFile(fullPath)
}

// ArchiveSession creates a .tar.gz archive of the specified session and writes it to w.
func (r *Recorder) ArchiveSession(sessionID string, w io.Writer) (err error) {
	sessionDir, err := r.safeJoin("interactions", sessionID)
	if err != nil {
		return err
	}

	info, statErr := r.rootStat(sessionDir)
	if statErr != nil {
		return statErr
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", sessionID)
	}

	gw := gzip.NewWriter(w)

	defer func() {
		if closeErr := gw.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	tw := tar.NewWriter(gw)

	defer func() {
		if closeErr := tw.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	return filepath.Walk(sessionDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		header, hErr := tar.FileInfoHeader(info, info.Name())
		if hErr != nil {
			return hErr
		}

		rel, rErr := filepath.Rel(sessionDir, path)
		if rErr != nil {
			return rErr
		}

		header.Name = rel

		if whErr := tw.WriteHeader(header); whErr != nil {
			return whErr
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		f, oErr := r.rootOpen(path)
		if oErr != nil {
			return oErr
		}

		defer func() { _ = f.Close() }()

		_, cErr := io.Copy(tw, f)

		return cErr
	})
}
