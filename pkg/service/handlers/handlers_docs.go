package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/russross/blackfriday/v2"
)

// HandleDocs returns a handler for serving documentation files as HTML.
func (s *Server) HandleDocs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/docs")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = "CLOUD-SHUTDOWN-GUIDE.md"
	}

	// Ensure we only serve files from the docs directory
	filePath := filepath.Join("docs", path)
	if !strings.HasPrefix(filepath.Clean(filePath), "docs") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Render markdown to HTML
	output := blackfriday.Run(content)

	// Wrap in a simple HTML template
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>%s - Bose SoundTouch Toolkit Docs</title>
    <link rel="icon" href="/media/favicon-braille.svg" type="image/svg+xml">
    <link rel="stylesheet" href="/web/css/style.css">
    <style>
        body { max-width: 800px; margin: 40px auto; padding: 0 20px; line-height: 1.6; color: #333; }
        h1, h2, h3 { color: #2196F3; }
        pre { background: #f4f4f4; padding: 15px; border-radius: 5px; overflow-x: auto; }
        code { font-family: monospace; background: #eee; padding: 2px 4px; border-radius: 3px; }
        pre code { background: none; padding: 0; }
        a { color: #2196F3; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .back-link { margin-bottom: 20px; display: block; }
    </style>
</head>
<body>
    <a href="/" class="back-link">&larr; Back to Toolkit</a>
    <div class="markdown-body">
        %s
    </div>
</body>
</html>`, path, output)
}
