package handlers

import (
	"net/url"
	"reflect"
	"testing"
)

func TestParseMigrationOptions_AllowsXMLAndTelnetKeys(t *testing.T) {
	q := url.Values{
		"marge":         []string{"self"},
		"stats":         []string{"proxied"},
		"sw_update":     []string{"original"},
		"bmx":           []string{"self"},
		"marge_url":     []string{"http://example:8000/marge"},
		"stats_url":     []string{"http://example:8000"},
		"sw_update_url": []string{"http://example:8000/updates/soundtouch"},
		"bmx_url":       []string{"http://example:8000/bmx/registry/v1/services"},
	}

	got := parseMigrationOptions(q)

	want := map[string]string{
		"marge":         "self",
		"stats":         "proxied",
		"sw_update":     "original",
		"bmx":           "self",
		"marge_url":     "http://example:8000/marge",
		"stats_url":     "http://example:8000",
		"sw_update_url": "http://example:8000/updates/soundtouch",
		"bmx_url":       "http://example:8000/bmx/registry/v1/services",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseMigrationOptions = %v\nwant %v", got, want)
	}
}

func TestParseMigrationOptions_DropsUnknownKeys(t *testing.T) {
	q := url.Values{
		"marge":      []string{"self"},
		"target_url": []string{"http://example:8000"}, // not an option
		"method":     []string{"telnet"},              // not an option
		"random":     []string{"value"},               // attacker-controlled noise
	}

	got := parseMigrationOptions(q)

	if _, ok := got["target_url"]; ok {
		t.Errorf("target_url leaked into options map: %v", got)
	}

	if _, ok := got["method"]; ok {
		t.Errorf("method leaked into options map: %v", got)
	}

	if _, ok := got["random"]; ok {
		t.Errorf("random key leaked into options map: %v", got)
	}

	if got["marge"] != "self" {
		t.Errorf("marge = %q, want self", got["marge"])
	}
}

func TestParseMigrationOptions_EmptyQueryReturnsEmptyMap(t *testing.T) {
	got := parseMigrationOptions(url.Values{})
	if len(got) != 0 {
		t.Errorf("got %v, want empty map", got)
	}
}
