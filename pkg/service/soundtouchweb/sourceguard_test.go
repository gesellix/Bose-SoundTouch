package soundtouchweb

import (
	"os"
	"strings"
)

// readSourceFile reads a file from this package's directory, for the few
// guards that assert on the shape of the code itself rather than its
// behaviour.
func readSourceFile(name string) (string, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// sliceBetween returns the text from the first occurrence of start up to the
// next occurrence of end, or "" if start is absent.
func sliceBetween(source, start, end string) string {
	i := strings.Index(source, start)
	if i < 0 {
		return ""
	}

	rest := source[i:]

	j := strings.Index(rest, end)
	if j < 0 {
		return rest
	}

	return rest[:j]
}
