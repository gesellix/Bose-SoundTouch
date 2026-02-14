package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// PathPattern defines a regex and its replacement for sanitizing URL paths.
type PathPattern struct {
	Name        string `json:"name"`
	Regexp      string `json:"regexp"`
	Replacement string `json:"replacement"`
	compiled    *regexp.Regexp
}

// PathPatterns is a collection of PathPattern.
type PathPatterns []PathPattern

// LoadPatterns loads path patterns from a JSON file.
func LoadPatterns(path string) (PathPatterns, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PathPatterns{}, nil
		}
		return nil, err
	}

	var patterns PathPatterns
	if err := json.Unmarshal(data, &patterns); err != nil {
		return nil, err
	}

	for i := range patterns {
		re, err := regexp.Compile(patterns[i].Regexp)
		if err != nil {
			return nil, fmt.Errorf("invalid regex in pattern %s: %w", patterns[i].Name, err)
		}
		patterns[i].compiled = re
	}

	return patterns, nil
}

// Sanitize sanitizes a segment using the configured patterns.
func (pp PathPatterns) Sanitize(segment string) (string, string) {
	for _, p := range pp {
		if p.compiled != nil && p.compiled.MatchString(segment) {
			return p.Replacement, p.Replacement
		}
	}
	return segment, ""
}

// DefaultPatterns returns the default set of path patterns.
func DefaultPatterns() PathPatterns {
	p := PathPattern{
		Name:        "IPv4",
		Regexp:      `^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`,
		Replacement: "{ip}",
	}
	re, _ := regexp.Compile(p.Regexp)
	p.compiled = re
	return PathPatterns{p}
}
