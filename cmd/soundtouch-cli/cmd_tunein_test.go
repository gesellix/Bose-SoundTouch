package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

// newCtx wires a *cli.Context with the kind-selection flags the resolver
// reads, plus whatever values the test wants set. Empty-string values are
// the default (flag not provided).
func newCtx(t *testing.T, kv map[string]string) *cli.Context {
	t.Helper()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, name := range []string{"station", "episode", "program", "id"} {
		fs.String(name, "", "")
	}

	for k, v := range kv {
		if err := fs.Set(k, v); err != nil {
			t.Fatalf("fs.Set(%q, %q): %v", k, v, err)
		}
	}

	return cli.NewContext(nil, fs, nil)
}

func TestResolveTuneInKind_Station(t *testing.T) {
	c := newCtx(t, map[string]string{"station": "s14991"})

	k, id, err := resolveTuneInKind(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if k.flag != "station" || k.itemType != "stationurl" {
		t.Errorf("wrong kind: %+v", k)
	}

	if id != "s14991" {
		t.Errorf("wrong id: %q", id)
	}
}

func TestResolveTuneInKind_Episode(t *testing.T) {
	c := newCtx(t, map[string]string{"episode": "e789012"})

	k, id, err := resolveTuneInKind(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if k.flag != "episode" || k.itemType != "stationurl" {
		t.Errorf("wrong kind: %+v", k)
	}

	if id != "e789012" {
		t.Errorf("wrong id: %q", id)
	}

	if !strings.Contains(k.location, "/v1/playback/episode/") {
		t.Errorf("wrong location template: %q", k.location)
	}
}

func TestResolveTuneInKind_Program(t *testing.T) {
	c := newCtx(t, map[string]string{"program": "p123456"})

	k, id, err := resolveTuneInKind(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if k.flag != "program" || k.itemType != "tracklisturl" {
		t.Errorf("wrong kind: %+v", k)
	}

	if id != "p123456" {
		t.Errorf("wrong id: %q", id)
	}

	if !strings.Contains(k.location, "/v1/playback/episodes/") {
		t.Errorf("wrong location template: %q", k.location)
	}
}

func TestResolveTuneInKind_IDPrefixAutoDetect(t *testing.T) {
	cases := []struct {
		id       string
		wantFlag string
	}{
		{"s14991", "station"},
		{"e789012", "episode"},
		{"p123456", "program"},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			c := newCtx(t, map[string]string{"id": tc.id})

			k, id, err := resolveTuneInKind(c)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if k.flag != tc.wantFlag {
				t.Errorf("auto-detect picked %q; want %q", k.flag, tc.wantFlag)
			}

			if id != tc.id {
				t.Errorf("id round-tripped wrong: got %q want %q", id, tc.id)
			}
		})
	}
}

func TestResolveTuneInKind_NoFlags(t *testing.T) {
	c := newCtx(t, nil)

	_, _, err := resolveTuneInKind(c)
	if err == nil {
		t.Fatal("expected error when no flags are set")
	}

	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error message should mention required flag: %v", err)
	}
}

func TestResolveTuneInKind_ConflictingFlags(t *testing.T) {
	c := newCtx(t, map[string]string{"station": "s14991", "episode": "e789012"})

	_, _, err := resolveTuneInKind(c)
	if err == nil {
		t.Fatal("expected error when conflicting flags are set")
	}

	if !strings.Contains(err.Error(), "only one of") {
		t.Errorf("error message should mention exclusivity: %v", err)
	}
}

func TestResolveTuneInKind_UnknownPrefix(t *testing.T) {
	c := newCtx(t, map[string]string{"id": "x999"})

	_, _, err := resolveTuneInKind(c)
	if err == nil {
		t.Fatal("expected error for unknown ID prefix")
	}

	if !strings.Contains(err.Error(), "no recognised TuneIn prefix") {
		t.Errorf("error message should explain prefix mismatch: %v", err)
	}
}
