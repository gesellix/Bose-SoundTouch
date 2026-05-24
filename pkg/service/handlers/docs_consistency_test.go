package handlers

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dirsToSkip names docs/content/ subdirectories whose contents are not
// required to have Hugo front matter. These are asset or infrastructure
// trees, not narrative documentation.
var dirsToSkip = map[string]bool{
	"archive": true, // superseded plans kept for the record
}

func TestDocsConsistency(t *testing.T) {
	projectRoot := "../../.."
	contentDir := filepath.Join(projectRoot, "docs", "content")

	err := filepath.WalkDir(contentDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path == contentDir {
				return nil
			}
			rel, relErr := filepath.Rel(contentDir, path)
			if relErr != nil {
				return relErr
			}
			// Skip only top-level excluded directories.
			if !strings.ContainsRune(rel, filepath.Separator) && dirsToSkip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		if !hasTitleFrontMatter(t, path) {
			rel, _ := filepath.Rel(contentDir, path)
			t.Errorf("docs/content/%s: missing 'title:' in front matter (Hugo requires it for correct rendering)", rel)
		}

		return nil
	})

	if err != nil {
		t.Errorf("Error walking docs/content directory: %v", err)
	}
}

// hasTitleFrontMatter returns true if the file has a YAML front matter block
// containing a non-empty title: field.
func hasTitleFrontMatter(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Errorf("cannot open %s: %v", path, err)
		return true // don't double-report
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)

	// First non-empty line must be "---"
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line != "---" {
			return false
		}
		break
	}

	// Scan until closing "---", looking for title:
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			return false // closed without finding title
		}
		if strings.HasPrefix(line, "title:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "title:"))
			value = strings.Trim(value, `"'`)
			return value != ""
		}
	}
	return false
}
