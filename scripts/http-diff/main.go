package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

func main() {
	htmlOutput := flag.String("html", "", "Path to save HTML diff report")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Println("Usage: http-diff [options] <file1.http> <file2.http>")
		fmt.Println("Options:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	file1 := args[0]
	file2 := args[1]

	body1, err := extractBody(file1)
	if err != nil {
		fmt.Printf("Error extracting body from %s: %v\n", file1, err)
		os.Exit(1)
	}

	body2, err := extractBody(file2)
	if err != nil {
		fmt.Printf("Error extracting body from %s: %v\n", file2, err)
		os.Exit(1)
	}

	norm1 := normalize(body1)
	norm2 := normalize(body2)

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(norm1, norm2, false)
	lineDiffs := dmp.DiffCleanupSemantic(diffs)

	if *htmlOutput != "" {
		err := generateHTML(*htmlOutput, file1, file2, lineDiffs)
		if err != nil {
			fmt.Printf("Error generating HTML: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("HTML report generated: %s\n", *htmlOutput)
		return
	}

	// Custom line-by-line diff for better readability
	for _, diff := range lineDiffs {
		switch diff.Type {
		case diffmatchpatch.DiffInsert:
			lines := strings.Split(diff.Text, "\n")
			for _, line := range lines {
				if line != "" {
					fmt.Printf("+ %s\n", line)
				}
			}
		case diffmatchpatch.DiffDelete:
			lines := strings.Split(diff.Text, "\n")
			for _, line := range lines {
				if line != "" {
					fmt.Printf("- %s\n", line)
				}
			}
		case diffmatchpatch.DiffEqual:
			// Optionally skip unchanged lines or show context
			lines := strings.Split(diff.Text, "\n")
			// Filter out empty lines from splitting
			var cleanLines []string
			for _, l := range lines {
				if strings.TrimSpace(l) != "" {
					cleanLines = append(cleanLines, l)
				}
			}

			if len(cleanLines) > 6 {
				fmt.Printf("  %s\n", cleanLines[0])
				fmt.Printf("  %s\n", cleanLines[1])
				fmt.Printf("  ...\n")
				fmt.Printf("  %s\n", cleanLines[len(cleanLines)-2])
				fmt.Printf("  %s\n", cleanLines[len(cleanLines)-1])
			} else {
				for _, line := range cleanLines {
					fmt.Printf("  %s\n", line)
				}
			}
		}
	}
}

func extractBody(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Use a non-greedy regex to find the LAST /* ... */ block which typically contains the body
	re := regexp.MustCompile(`(?s)/\*\s*(<\?xml.*?|\{.*?|\[.*?)\s*\*/`)
	matches := re.FindAllStringSubmatch(string(content), -1)
	if len(matches) > 0 {
		// Return the last match as it's more likely to be the response body
		lastMatch := matches[len(matches)-1]
		return strings.TrimSpace(lastMatch[1]), nil
	}

	return "", fmt.Errorf("could not find body in /* ... */ block")
}

func normalize(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	// Mask common timestamps and changing fields
	timestampRegex := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
	body = timestampRegex.ReplaceAllString(body, "[TIMESTAMP]")

	// Mask account IDs if they are variable, but usually they match in these files.
	// Let's stick to timestamps for now.

	// Try XML first
	if strings.HasPrefix(body, "<?xml") || strings.Contains(body, "<") {
		// For XML, let's also try to mask specific tags like <updatedOn> or <createdOn>
		tagsToMask := []string{"updatedOn", "createdOn", "lastModified", "timestamp"}
		for _, tag := range tagsToMask {
			re := regexp.MustCompile(fmt.Sprintf(`<%s>.*?</%s>`, tag, tag))
			body = re.ReplaceAllString(body, fmt.Sprintf("<%s>[MASKED]</%s>", tag, tag))
		}

		// Also mask empty attributes that might be noisy, like displayName=""
		body = regexp.MustCompile(`\s+displayName=""`).ReplaceAllString(body, "")

		var out bytes.Buffer
		decoder := xml.NewDecoder(strings.NewReader(body))
		encoder := xml.NewEncoder(&out)
		encoder.Indent("", "  ")
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				// If it's not valid XML, maybe it's just a fragment, continue or return body
				break
			}
			// Trim whitespace from CharData to normalize
			if cd, ok := token.(xml.CharData); ok {
				token = xml.CharData(bytes.TrimSpace(cd))
			}
			err = encoder.EncodeToken(token)
			if err != nil {
				break
			}
		}
		encoder.Flush()
		if out.Len() > 0 {
			return out.String()
		}
	}

	// Try JSON
	if strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
		var obj interface{}
		if err := json.Unmarshal([]byte(body), &obj); err == nil {
			// Mask some JSON fields if they are common
			maskJSON(obj)
			pretty, _ := json.MarshalIndent(obj, "", "  ")
			return string(pretty)
		}
	}

	return body
}

func maskJSON(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for k, val := range v {
			if strings.Contains(strings.ToLower(k), "timestamp") || k == "updatedOn" || k == "createdOn" || k == "expires_at" {
				v[k] = "[MASKED]"
			} else {
				maskJSON(val)
			}
		}
	case []interface{}:
		for _, item := range v {
			maskJSON(item)
		}
	}
}

func generateHTML(path, file1, file2 string, diffs []diffmatchpatch.Diff) error {
	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>HTTP Diff Report</title>
    <style>
        body { font-family: monospace; line-height: 1.2; background: #f8f9fa; color: #212529; padding: 20px; }
        .container { max-width: 1200px; margin: 0 auto; background: #fff; padding: 20px; border: 1px solid #dee2e6; border-radius: 4px; }
        .header { margin-bottom: 20px; border-bottom: 2px solid #eee; padding-bottom: 10px; }
        .diff-table { width: 100%; border-collapse: collapse; table-layout: fixed; }
        .diff-table td { vertical-align: top; padding: 2px 4px; border: 1px solid #eee; overflow-wrap: break-word; }
        .line-num { width: 40px; text-align: right; color: #999; background: #fdfdfd; user-select: none; }
        .diff-equal { background: #fff; }
        .diff-insert { background: #e6ffec; text-decoration: none; color: #1a7f37; }
        .diff-delete { background: #ffebe9; text-decoration: none; color: #cf222e; }
        .diff-change-marker { font-weight: bold; margin-right: 5px; }
        h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
        .files { font-size: 0.9rem; color: #666; }
        pre { margin: 0; white-space: pre-wrap; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>HTTP Response Body Diff</h1>
            <div class="files">
                Left: <strong>` + html.EscapeString(file1) + `</strong><br>
                Right: <strong>` + html.EscapeString(file2) + `</strong>
            </div>
        </div>
        <table class="diff-table">
`)

	type sideBySideLine struct {
		leftText  string
		rightText string
		class     string
	}
	var lines []sideBySideLine

	for _, diff := range diffs {
		text := html.EscapeString(diff.Text)
		split := strings.Split(text, "\n")
		// Remove trailing empty string from split if it exists
		if len(split) > 0 && split[len(split)-1] == "" {
			split = split[:len(split)-1]
		}

		switch diff.Type {
		case diffmatchpatch.DiffEqual:
			for _, line := range split {
				lines = append(lines, sideBySideLine{leftText: line, rightText: line, class: "diff-equal"})
			}
		case diffmatchpatch.DiffInsert:
			for _, line := range split {
				lines = append(lines, sideBySideLine{leftText: "", rightText: line, class: "diff-insert"})
			}
		case diffmatchpatch.DiffDelete:
			for _, line := range split {
				lines = append(lines, sideBySideLine{leftText: line, rightText: "", class: "diff-delete"})
			}
		}
	}

	for i, line := range lines {
		leftMarker := ""
		rightMarker := ""
		if line.class == "diff-insert" {
			rightMarker = "+"
		} else if line.class == "diff-delete" {
			leftMarker = "-"
		}

		sb.WriteString(fmt.Sprintf(`
            <tr class="%s">
                <td class="line-num">%d</td>
                <td><pre><span class="diff-change-marker">%s</span>%s</pre></td>
                <td class="line-num">%d</td>
                <td><pre><span class="diff-change-marker">%s</span>%s</pre></td>
            </tr>`, line.class, i+1, leftMarker, line.leftText, i+1, rightMarker, line.rightText))
	}

	sb.WriteString(`
        </table>
    </div>
</body>
</html>
`)

	return os.WriteFile(path, []byte(sb.String()), 0644)
}
