package handlers

import (
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
