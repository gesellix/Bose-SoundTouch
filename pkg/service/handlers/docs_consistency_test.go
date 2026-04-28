package handlers

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsConsistency(t *testing.T) {
	// Root of the project relative to this test file
	// The test runs in the directory of the package
	projectRoot := "../../.."
	docsDir := filepath.Join(projectRoot, "docs")
	summaryPath := filepath.Join(docsDir, "SUMMARY.md")

	summaryContent, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("Failed to read SUMMARY.md: %v", err)
	}

	summaryText := string(summaryContent)

	docsIgnore := readDocsIgnore(t, filepath.Join(projectRoot, ".docsignore"))

	// List of directories to check
	dirsToCheck := []string{".", "guides", "reference", "analysis"}

	for _, dir := range dirsToCheck {
		dirPath := filepath.Join(docsDir, dir)
		err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Don't recurse into subdirectories if we are checking the root,
				// as they are handled separately or ignored (like archive)
				if dir == "." && path != dirPath {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".md") {
				return nil
			}

			// Skip SUMMARY.md itself
			if d.Name() == "SUMMARY.md" {
				return nil
			}

			// Skip files listed in .docsignore at the project root
			for _, skip := range docsIgnore {
				if strings.HasSuffix(path, filepath.FromSlash(skip)) {
					return nil
				}
			}

			// Get relative path from docs/
			relPath, err := filepath.Rel(docsDir, path)
			if err != nil {
				return err
			}

			// Check if this file is linked in SUMMARY.md
			// We look for [Label](relPath)
			linkPattern := "(" + relPath + ")"
			if !strings.Contains(summaryText, linkPattern) {
				t.Errorf("Documentation file %s is not linked in docs/SUMMARY.md", relPath)
			}

			return nil
		})

		if err != nil {
			t.Errorf("Error walking directory %s: %v", dir, err)
		}
	}
}

// readDocsIgnore reads a .docsignore file and returns the non-empty, non-comment lines.
// If the file does not exist it returns nil without failing the test.
func readDocsIgnore(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("Failed to read %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Error reading %s: %v", path, err)
	}
	return patterns
}
