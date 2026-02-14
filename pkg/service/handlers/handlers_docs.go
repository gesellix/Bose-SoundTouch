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
		path = "guides/SURVIVAL-GUIDE.md"
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

	// Load sidebar (SUMMARY.md)
	summaryContent, _ := os.ReadFile(filepath.Join("docs", "SUMMARY.md"))

	sidebar := ""
	if len(summaryContent) > 0 {
		// Render summary to HTML
		sidebar = string(blackfriday.Run(summaryContent))
		// Adjust links in sidebar to be relative to /docs/
		sidebar = strings.ReplaceAll(sidebar, "href=\"guides/", "href=\"/docs/guides/")
		sidebar = strings.ReplaceAll(sidebar, "href=\"reference/", "href=\"/docs/reference/")
		sidebar = strings.ReplaceAll(sidebar, "href=\"analysis/", "href=\"/docs/analysis/")
		// Fix relative links that don't have a directory prefix (root docs)
		// We look for href="filename.md" and replace with href="/docs/filename.md"
		// This avoids manual listing of every file.
		sidebar = s.fixSidebarLinks(sidebar)
	}

	// Render markdown to HTML
	output := blackfriday.Run(content)

	// Wrap in a documentation template with sidebar
	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>%s - Bose SoundTouch Toolkit Docs</title>
    <link rel="icon" href="/media/favicon-braille.svg" type="image/svg+xml">
    <link rel="stylesheet" href="/web/css/style.css">
    <style>
        body { margin: 0; padding: 0; display: flex; font-family: sans-serif; height: 100vh; overflow: hidden; }
        .sidebar { width: 300px; background: #f8f9fa; border-right: 1px solid #dee2e6; padding: 20px; overflow-y: auto; flex-shrink: 0; }
        .content-area { flex-grow: 1; overflow-y: auto; padding: 40px; }
        .markdown-body { max-width: 800px; margin: 0 auto; line-height: 1.6; color: #333; }
        h1, h2, h3 { color: #2196F3; }
        pre { background: #f4f4f4; padding: 15px; border-radius: 5px; overflow-x: auto; }
        code { font-family: monospace; background: #eee; padding: 2px 4px; border-radius: 3px; }
        pre code { background: none; padding: 0; }
        a { color: #2196F3; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .back-link { margin-bottom: 20px; display: block; font-weight: bold; }
        .sidebar h2 { font-size: 1.1em; margin-top: 20px; color: #666; text-transform: uppercase; letter-spacing: 1px; }
        .sidebar ul { list-style: none; padding: 0; }
        .sidebar li { margin-bottom: 8px; }
        .sidebar a { color: #444; font-size: 0.95em; }
        .sidebar a:hover { color: #2196F3; }
    </style>
</head>
<body>
    <div class="sidebar">
        <a href="/" class="back-link">&larr; Back to Toolkit</a>
        %s
    </div>
    <div class="content-area">
        <div class="markdown-body">
            %s
        </div>
   	</div>
</body>
</html>`, path, sidebar, output)
}

// fixSidebarLinks ensures that relative links in the SUMMARY.md (sidebar)
// are correctly prefixed with /docs/ for the web UI.
func (s *Server) fixSidebarLinks(sidebar string) string {
	// Root links like [Label](file.md) become href="file.md"
	// We want href="/docs/file.md", but only if it doesn't already start with /docs/
	// and isn't an external link.
	// Since blackfriday renders [Label](file.md) as <a href="file.md">

	// A simple but effective way is to use a regex or just check for common patterns.
	// We already handled subdirectories. Now we handle files in the root of docs/

	// We'll look for href="filename.md" where filename doesn't contain a slash
	// and isn't already prefixed.

	// Since we know our doc files always end in .md, we can look for that.
	lines := strings.Split(sidebar, "\n")
	for i, line := range lines {
		if strings.Contains(line, "href=\"") && !strings.Contains(line, "href=\"/docs/") && !strings.Contains(line, "://") {
			// Extract filename
			start := strings.Index(line, "href=\"") + 6
			end := strings.Index(line[start:], "\"") + start
			filename := line[start:end]

			if strings.HasSuffix(filename, ".md") && !strings.Contains(filename, "/") {
				lines[i] = strings.ReplaceAll(line, "href=\""+filename+"\"", "href=\"/docs/"+filename+"\"")
			}
		}
	}

	return strings.Join(lines, "\n")
}
