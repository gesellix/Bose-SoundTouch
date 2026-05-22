package handlers

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dirsToSkip names docs/ subdirectories whose contents are not meant
// to appear in SUMMARY.md. These are asset / partial / archive trees,
// not narrative documentation:
//
//   - _includes : HTML partials consumed by the docs site renderer
//   - archive   : superseded plans / status reports kept for the
//     record but deliberately unlinked
//   - diagrams  : Mermaid sources for embedded diagrams
//   - images    : binary assets + a directory README that explains them
//
// Adding a new top-level dir under docs/ does NOT require touching
// this list — only add the dir name here when its contents should
// stay out of SUMMARY.md by design. Individual file exclusions live
// in .docsignore instead.
var dirsToSkip = map[string]bool{
	"_includes": true,
	"archive":   true,
	"diagrams":  true,
	"images":    true,
}

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

	// Walk the entire docs tree. Directory-level exclusions live in
	// dirsToSkip above (asset / archive trees); file-level exclusions
	// live in .docsignore (individual narrative docs that are
	// intentionally unlinked). New subdirectories are picked up
	// automatically — this is the behaviour amazon-music-oauth.md
	// surprised us by lacking.
	err = filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path == docsDir {
				return nil
			}

			rel, relErr := filepath.Rel(docsDir, path)
			if relErr != nil {
				return relErr
			}

			// Skip only top-level asset / archive directories. Nested
			// directories inside narrative trees (e.g. docs/guides/foo/)
			// would still be walked.
			if !strings.ContainsRune(rel, filepath.Separator) && dirsToSkip[d.Name()] {
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
		t.Errorf("Error walking docs directory: %v", err)
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
