package main

import (
	"testing"

	"github.com/urfave/cli/v2"
)

// TestLibraryCommand_Registered checks that the library command and its three
// subcommands are wired up with the expected names and flags. No live multicast
// or real speaker calls are made.
func TestLibraryCommand_Registered(t *testing.T) {
	cmd := libraryCommand()

	if cmd.Name != "library" {
		t.Errorf("top-level command name = %q; want %q", cmd.Name, "library")
	}

	// Index subcommands by name for easy lookup.
	sub := make(map[string]interface{})

	for _, sc := range cmd.Subcommands {
		sub[sc.Name] = sc
	}

	for _, name := range []string{"servers", "browse", "play"} {
		if _, ok := sub[name]; !ok {
			t.Errorf("expected subcommand %q to be registered", name)
		}
	}
}

// TestLibraryServersFlags checks the flags on `library servers`.
func TestLibraryServersFlags(t *testing.T) {
	cmd := libraryCommand()

	for _, s := range cmd.Subcommands {
		if s.Name != "servers" {
			continue
		}

		flags := flagNames(s.Flags)

		for _, want := range []string{"timeout", "via-speaker"} {
			if !contains(flags, want) {
				t.Errorf("servers subcommand missing flag %q; got %v", want, flags)
			}
		}

		return
	}

	t.Fatal("servers subcommand not found")
}

// TestLibraryBrowseFlags checks the flags on `library browse`.
func TestLibraryBrowseFlags(t *testing.T) {
	cmd := libraryCommand()

	for _, s := range cmd.Subcommands {
		if s.Name != "browse" {
			continue
		}

		flags := flagNames(s.Flags)

		for _, want := range []string{"udn", "object", "start", "count", "timeout"} {
			if !contains(flags, want) {
				t.Errorf("browse subcommand missing flag %q; got %v", want, flags)
			}
		}

		return
	}

	t.Fatal("browse subcommand not found")
}

// TestLibraryPlayFlags checks the flags on `library play`.
func TestLibraryPlayFlags(t *testing.T) {
	cmd := libraryCommand()

	for _, s := range cmd.Subcommands {
		if s.Name != "play" {
			continue
		}

		flags := flagNames(s.Flags)

		// source-account and location are required; name, type, art are optional.
		for _, want := range []string{"source-account", "location", "name", "type", "art"} {
			if !contains(flags, want) {
				t.Errorf("play subcommand missing flag %q; got %v", want, flags)
			}
		}

		// Old URL-mode flags must no longer be present.
		for _, gone := range []string{"url", "mode"} {
			if contains(flags, gone) {
				t.Errorf("play subcommand should not have flag %q", gone)
			}
		}

		return
	}

	t.Fatal("play subcommand not found")
}

// flagNames extracts the primary Name from each flag in a slice.
func flagNames(flags []cli.Flag) []string {
	names := make([]string, 0, len(flags))

	for _, f := range flags {
		names = append(names, getFlagName(f))
	}

	return names
}

// contains reports whether needle is in haystack.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}

	return false
}
