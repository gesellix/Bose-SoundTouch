package stockholm

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// NativeState is a flat string key→value store persisted to native-state.json.
// Every Set call writes through to disk.
type NativeState struct {
	mu   sync.RWMutex
	data map[string]string
	path string
}

// NewNativeState creates a NativeState that persists to stateDir/native-state.json.
func NewNativeState(stateDir string) *NativeState {
	return &NativeState{
		data: make(map[string]string),
		path: filepath.Join(stateDir, "native-state.json"),
	}
}

// Load reads the state file from disk. Non-existent file is not an error.
func (s *NativeState) Load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read native-state: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse native-state: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, v := range raw {
		s.data[k] = stringifyScalar(v)
	}

	return nil
}

// SeedFromEnv seeds Marge auth/account from environment variables and
// sets first-run defaults (guid, nativeFrameVersion, authServer, constant.kilo).
func (s *NativeState) SeedFromEnv(cfg *Config) {
	updates := make(map[string]string)

	// Marge session from env
	if v := firstNonEmpty(os.Getenv("margeAuthToken"), os.Getenv("MARGE_AUTH_TOKEN")); v != "" {
		updates["margeAuthToken"] = v
	}

	if v := firstNonEmpty(os.Getenv("margeAccountID"), os.Getenv("MARGE_ACCOUNT_ID")); v != "" {
		updates["margeAccountID"] = v
	}

	if s.Get("constant.kilo") == "" {
		updates["constant.kilo"] = kiloDefaultValue
	}

	// First-run defaults that require a persisted value
	if s.Get("authServer") == "" {
		updates["authServer"] = "0"
	}

	// GUID: use existing or generate new
	existingGUID := firstNonEmpty(s.Get("guid"), s.Get("deviceGuid"))
	if existingGUID == "" {
		existingGUID = randomHexUUID()
	}

	if s.Get("guid") == "" {
		updates["guid"] = existingGUID
	}

	if s.Get("deviceGuid") == "" {
		updates["deviceGuid"] = existingGUID
	}

	// Version info from config
	if cfg != nil {
		fullVersion := firstNonEmpty(s.Get("frame_version"), cfg.AppVersion)
		shortVersion := firstNonEmpty(ExtractVersionPrefix(s.Get("nativeFrameVersion")),
			ExtractVersionPrefix(s.Get("frame_version")),
			ExtractVersionPrefix(cfg.AppVersion))

		if s.Get("nativeFrameVersion") == "" && shortVersion != "" {
			updates["nativeFrameVersion"] = shortVersion
		}

		if s.Get("frame_version") == "" && fullVersion != "" {
			updates["frame_version"] = fullVersion
		}
	}

	if len(updates) > 0 {
		s.putMany(updates)
	}
}

// Get returns the value for key, or "" if absent.
func (s *NativeState) Get(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.data[key]
}

// Set stores key→value and persists to disk.
func (s *NativeState) Set(key, value string) {
	if key == "" {
		return
	}

	s.mu.Lock()
	s.data[key] = value
	s.mu.Unlock()
	s.persist()
}

// PutMany stores multiple key→value pairs and persists once.
func (s *NativeState) PutMany(updates map[string]string) {
	if len(updates) == 0 {
		return
	}

	s.putMany(updates)
}

func (s *NativeState) putMany(updates map[string]string) {
	s.mu.Lock()
	changed := false

	for k, v := range updates {
		if k == "" {
			continue
		}

		if prev, ok := s.data[k]; !ok || prev != v {
			s.data[k] = v
			changed = true
		}
	}
	s.mu.Unlock()

	if changed {
		s.persist()
	}
}

// AuthServer returns the authServer value normalised to "0"–"3".
func (s *NativeState) AuthServer() string {
	v := s.Get("authServer")
	switch v {
	case "0", "1", "2", "3":
		return v
	default:
		return "0"
	}
}

func (s *NativeState) persist() {
	s.mu.RLock()

	snapshot := make(map[string]string, len(s.data))
	for k, v := range s.data {
		snapshot[k] = v
	}

	s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		log.Printf("[Stockholm] Failed to create state dir: %v", err)
		return
	}

	b, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		log.Printf("[Stockholm] Failed to marshal native state: %v", err)
		return
	}

	if err := os.WriteFile(s.path, append(b, '\n'), 0644); err != nil {
		log.Printf("[Stockholm] Failed to persist native state: %v", err)
	}
}

func stringifyScalar(v interface{}) string {
	if v == nil {
		return ""
	}

	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}

		return "false"
	case float64:
		// JSON numbers decode to float64
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}

		return fmt.Sprintf("%g", t)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}

		return string(b)
	}
}
