package proxy

import (
	"regexp"
	"testing"
)

func TestDefaultPatterns(t *testing.T) {
	patterns := DefaultPatterns()
	if len(patterns) != 4 {
		t.Errorf("Expected 4 default patterns, got %d", len(patterns))
	}

	expectedNames := []string{"IPv4", "UUID", "AccountID", "DeviceID"}
	for i, name := range expectedNames {
		if patterns[i].Name != name {
			t.Errorf("Expected pattern %d name %s, got %s", i, name, patterns[i].Name)
		}
	}
}

func TestPathPatterns_Sanitize(t *testing.T) {
	patterns := DefaultPatterns()
	// Need to compile them as DefaultPatterns() in its new form doesn't compile them (main.go or LoadPatterns does it)
	// Wait, actually the new DefaultPatterns() I wrote doesn't compile them.
	// But PathPatterns.Sanitize checks for compiled != nil.

	// Let's manually compile for the test
	for i := range patterns {
		patterns[i].compiled = mustCompile(patterns[i].Regexp)
	}

	tests := []struct {
		segment  string
		wantRepl string
	}{
		{"192.168.1.100", "{ip}"},
		{"1234567", "{accountId}"},
		{"12345", "{accountId}"},
		{"12345678-1234-5678-9012-123456789012", "{uuid}"},
		{"D05FB8A848E5", "{device_id}"},
		{"some-other-segment", ""},
	}

	for _, tt := range tests {
		repl, _ := patterns.Sanitize(tt.segment)
		if tt.wantRepl == "" {
			if repl != tt.segment {
				t.Errorf("Sanitize(%q) = %q, want %q (no change)", tt.segment, repl, tt.segment)
			}
		} else {
			if repl != tt.wantRepl {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.segment, repl, tt.wantRepl)
			}
		}
	}
}

func mustCompile(re string) *regexp.Regexp {
	return regexp.MustCompile(re)
}
