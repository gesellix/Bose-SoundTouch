package stockholm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeState_SetGet(t *testing.T) {
	state := NewNativeState(t.TempDir())

	state.Set("foo", "bar")

	if got := state.Get("foo"); got != "bar" {
		t.Errorf("Get(foo) = %q, want bar", got)
	}
}

func TestNativeState_GetMissing_ReturnsEmpty(t *testing.T) {
	state := NewNativeState(t.TempDir())

	if got := state.Get("does-not-exist"); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestNativeState_SetEmpty_IsNoOp(t *testing.T) {
	state := NewNativeState(t.TempDir())
	state.Set("", "value")

	// Empty key should not be stored
	if got := state.Get(""); got != "" {
		t.Errorf("expected empty string for empty key, got %q", got)
	}
}

func TestNativeState_PersistsAndLoads(t *testing.T) {
	dir := t.TempDir()
	state := NewNativeState(dir)

	state.Set("key1", "val1")
	state.Set("key2", "val2")

	// Reload from disk
	state2 := NewNativeState(dir)
	if err := state2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := state2.Get("key1"); got != "val1" {
		t.Errorf("after reload, key1 = %q, want val1", got)
	}

	if got := state2.Get("key2"); got != "val2" {
		t.Errorf("after reload, key2 = %q, want val2", got)
	}
}

func TestNativeState_Load_MissingFile_IsOK(t *testing.T) {
	state := NewNativeState(t.TempDir())

	if err := state.Load(); err != nil {
		t.Errorf("Load on missing file should not error, got %v", err)
	}
}

func TestNativeState_Load_CorruptFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "native-state.json")
	_ = os.WriteFile(statePath, []byte("not json"), 0644)

	state := NewNativeState(dir)

	if err := state.Load(); err == nil {
		t.Error("expected error for corrupt JSON, got nil")
	}
}

func TestNativeState_PutMany(t *testing.T) {
	state := NewNativeState(t.TempDir())

	state.PutMany(map[string]string{
		"a": "1",
		"b": "2",
	})

	if got := state.Get("a"); got != "1" {
		t.Errorf("expected a=1, got %q", got)
	}

	if got := state.Get("b"); got != "2" {
		t.Errorf("expected b=2, got %q", got)
	}
}

func TestNativeState_PutMany_Empty_IsNoOp(t *testing.T) {
	dir := t.TempDir()
	state := NewNativeState(dir)

	state.PutMany(nil)
	state.PutMany(map[string]string{})

	// State file should not be created for no-op
	_, err := os.Stat(filepath.Join(dir, "native-state.json"))
	if err == nil {
		t.Error("expected no state file to be created for empty PutMany")
	}
}

func TestNativeState_AuthServer_ValidValues(t *testing.T) {
	state := NewNativeState(t.TempDir())

	for _, v := range []string{"0", "1", "2", "3"} {
		state.Set("authServer", v)

		if got := state.AuthServer(); got != v {
			t.Errorf("AuthServer() = %q, want %q", got, v)
		}
	}
}

func TestNativeState_AuthServer_InvalidDefault(t *testing.T) {
	state := NewNativeState(t.TempDir())
	state.Set("authServer", "99")

	if got := state.AuthServer(); got != "0" {
		t.Errorf("expected default 0 for invalid authServer, got %q", got)
	}
}

func TestNativeState_SeedFromEnv_SetsGUIDAndDefaults(t *testing.T) {
	state := NewNativeState(t.TempDir())

	state.SeedFromEnv(nil)

	if got := state.Get("guid"); got == "" {
		t.Error("expected guid to be seeded")
	}

	if got := state.Get("deviceGuid"); got == "" {
		t.Error("expected deviceGuid to be seeded")
	}

	if got := state.Get("authServer"); got != "0" {
		t.Errorf("expected authServer=0, got %q", got)
	}

	if got := state.Get("constant.kilo"); got != "a7928d7b43dcd49f0af31e5aeed26458" {
		t.Errorf("unexpected kilo value: %q", got)
	}
}

func TestNativeState_SeedFromEnv_GUIDConsistent(t *testing.T) {
	state := NewNativeState(t.TempDir())

	state.SeedFromEnv(nil)

	guid := state.Get("guid")
	deviceGuid := state.Get("deviceGuid")

	if guid != deviceGuid {
		t.Errorf("expected guid == deviceGuid, got %q vs %q", guid, deviceGuid)
	}
}

func TestNativeState_SeedFromEnv_DoesNotOverwriteExistingGUID(t *testing.T) {
	state := NewNativeState(t.TempDir())
	state.Set("guid", "existing-guid")

	state.SeedFromEnv(nil)

	if got := state.Get("guid"); got != "existing-guid" {
		t.Errorf("expected existing guid to be preserved, got %q", got)
	}
}

func TestNativeState_SeedFromEnv_SetsVersionFromConfig(t *testing.T) {
	state := NewNativeState(t.TempDir())
	cfg := &Config{AppVersion: "27.0.13-release"}

	state.SeedFromEnv(cfg)

	if got := state.Get("nativeFrameVersion"); got != "27.0.13" {
		t.Errorf("expected nativeFrameVersion=27.0.13, got %q", got)
	}
}

func TestNativeState_PersistFileContainsJSON(t *testing.T) {
	dir := t.TempDir()
	state := NewNativeState(dir)
	state.Set("hello", "world")

	data, err := os.ReadFile(filepath.Join(dir, "native-state.json"))
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("state file is not valid JSON: %v", err)
	}

	if m["hello"] != "world" {
		t.Errorf("expected hello=world in state file, got %q", m["hello"])
	}
}

// ---- stringifyScalar ----

func TestStringifyScalar(t *testing.T) {
	cases := []struct {
		input interface{}
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{true, "true"},
		{false, "false"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{[]int{1, 2}, `[1,2]`},
	}

	for _, tc := range cases {
		if got := stringifyScalar(tc.input); got != tc.want {
			t.Errorf("stringifyScalar(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
