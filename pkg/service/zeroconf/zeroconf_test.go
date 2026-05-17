package zeroconf

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateDHKeyPair(t *testing.T) {
	priv1, pub1, err := GenerateDHKeyPair()
	if err != nil {
		t.Fatalf("GenerateDHKeyPair: %v", err)
	}
	if priv1 == nil || len(pub1) == 0 {
		t.Fatal("expected non-nil private key and non-empty public key")
	}
	if len(pub1) != dhKeySize {
		t.Errorf("public key length = %d, want %d", len(pub1), dhKeySize)
	}

	// Two calls must produce different key pairs.
	_, pub2, err := GenerateDHKeyPair()
	if err != nil {
		t.Fatalf("GenerateDHKeyPair second call: %v", err)
	}
	if string(pub1) == string(pub2) {
		t.Error("two key-pair generations produced identical public keys")
	}
}

func TestDHCommutativity(t *testing.T) {
	// DH shared secret must be symmetric: A's secret == B's secret.
	privA, pubA, err := GenerateDHKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	privB, pubB, err := GenerateDHKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	secretA := ComputeSharedSecret(privA, pubB)
	secretB := ComputeSharedSecret(privB, pubA)

	if string(secretA) != string(secretB) {
		t.Error("DH shared secrets are not equal (commutativity broken)")
	}
}

func TestDeriveKeys(t *testing.T) {
	sharedSecret := make([]byte, dhKeySize)
	for i := range sharedSecret {
		sharedSecret[i] = byte(i)
	}

	encKey, macKey := DeriveKeys(sharedSecret)

	if len(encKey) != 16 {
		t.Errorf("encKey length = %d, want 16", len(encKey))
	}
	if len(macKey) != 20 {
		t.Errorf("macKey length = %d, want 20", len(macKey))
	}

	// Deterministic: same input → same output.
	encKey2, macKey2 := DeriveKeys(sharedSecret)
	if string(encKey) != string(encKey2) || string(macKey) != string(macKey2) {
		t.Error("DeriveKeys is not deterministic")
	}

	// Different secrets → different keys.
	other := make([]byte, dhKeySize)
	encKeyOther, _ := DeriveKeys(other)
	if string(encKey) == string(encKeyOther) {
		t.Error("different secrets produced the same encKey")
	}
}

func TestBuildCredentialsBlob(t *testing.T) {
	blob := BuildCredentialsBlob("alice", "tok123", AuthTypeOAuthToken)

	creds, err := parseCredentialsBlob(blob)
	if err != nil {
		t.Fatalf("parseCredentialsBlob: %v", err)
	}
	if creds.username != "alice" {
		t.Errorf("username = %q, want %q", creds.username, "alice")
	}
	if string(creds.authData) != "tok123" {
		t.Errorf("authData = %q, want %q", string(creds.authData), "tok123")
	}
	if uint64(creds.authType) != AuthTypeOAuthToken {
		t.Errorf("authType = %d, want %d (AuthTypeOAuthToken)", creds.authType, AuthTypeOAuthToken)
	}
}

func TestEncryptDecryptBlob(t *testing.T) {
	sharedSecret := make([]byte, dhKeySize)
	for i := range sharedSecret {
		sharedSecret[i] = byte(42 + i)
	}
	encKey, macKey := DeriveKeys(sharedSecret)

	plaintext := []byte("hello zeroconf world")

	encrypted, err := EncryptBlob(encKey, macKey, plaintext)
	if err != nil {
		t.Fatalf("EncryptBlob: %v", err)
	}

	decrypted, err := DecryptBlob(encKey, macKey, encrypted)
	if err != nil {
		t.Fatalf("DecryptBlob: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("round-trip mismatch: got %q, want %q", decrypted, plaintext)
	}

	// Tampered checksum must fail.
	tampered := make([]byte, len(encrypted))
	copy(tampered, encrypted)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := DecryptBlob(encKey, macKey, tampered); err == nil {
		t.Error("expected error on tampered checksum, got nil")
	}
}

func TestPushCredentials_FullRoundTrip(t *testing.T) {
	speakerPrivate, speakerPublicBytes, err := GenerateDHKeyPair()
	if err != nil {
		t.Fatalf("speaker keygen: %v", err)
	}

	type received struct {
		username string
		authData string
		authType int
	}
	var got received

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getInfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":       101,
				"statusString": "OK",
				"publicKey":    base64.StdEncoding.EncodeToString(speakerPublicBytes),
			})

		case "addUser":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			blobBytes, err := base64.StdEncoding.DecodeString(r.FormValue("blob"))
			if err != nil {
				http.Error(w, "bad blob base64: "+err.Error(), http.StatusBadRequest)
				return
			}
			clientKeyBytes, err := base64.StdEncoding.DecodeString(r.FormValue("clientKey"))
			if err != nil {
				http.Error(w, "bad clientKey base64: "+err.Error(), http.StatusBadRequest)
				return
			}

			shared := ComputeSharedSecret(speakerPrivate, clientKeyBytes)
			encKey, macKey := DeriveKeys(shared)

			plaintext, err := DecryptBlob(encKey, macKey, blobBytes)
			if err != nil {
				http.Error(w, "decrypt failed: "+err.Error(), http.StatusBadRequest)
				return
			}

			creds, err := parseCredentialsBlob(plaintext)
			if err != nil {
				http.Error(w, "parse failed: "+err.Error(), http.StatusBadRequest)
				return
			}

			got.username = creds.username
			got.authData = string(creds.authData)
			got.authType = creds.authType
			w.WriteHeader(http.StatusOK)

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	const wantUsername = "user@example.com"
	const wantToken = "eyJhbGciOiJSUzI1NiJ9.access-token"

	if err := PushCredentials(srv.URL+"/zc", wantUsername, wantToken); err != nil {
		t.Fatalf("PushCredentials: %v", err)
	}

	if got.username != wantUsername {
		t.Errorf("username = %q, want %q", got.username, wantUsername)
	}
	if got.authData != wantToken {
		t.Errorf("authData = %q, want %q", got.authData, wantToken)
	}
	if uint64(got.authType) != AuthTypeOAuthToken {
		t.Errorf("authType = %d, want %d (AuthTypeOAuthToken)", got.authType, AuthTypeOAuthToken)
	}
}

func TestPushCredentials_FallbackOnGetInfoFailure(t *testing.T) {
	var receivedForm map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getInfo":
			http.Error(w, "not supported", http.StatusNotFound)
		case "addUser":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			receivedForm = map[string]string{
				"userName":  r.FormValue("userName"),
				"blob":      r.FormValue("blob"),
				"clientKey": r.FormValue("clientKey"),
				"tokenType": r.FormValue("tokenType"),
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	const wantUsername = "user@example.com"
	const wantToken = "raw-access-token"

	if err := PushCredentials(srv.URL+"/zc", wantUsername, wantToken); err != nil {
		t.Fatalf("PushCredentials: %v", err)
	}

	if receivedForm == nil {
		t.Fatal("addUser was never called")
	}
	if receivedForm["userName"] != wantUsername {
		t.Errorf("userName = %q, want %q", receivedForm["userName"], wantUsername)
	}
	if receivedForm["blob"] != wantToken {
		t.Errorf("blob = %q, want raw token %q", receivedForm["blob"], wantToken)
	}
	if receivedForm["tokenType"] != "accesstoken" {
		t.Errorf("tokenType = %q, want %q", receivedForm["tokenType"], "accesstoken")
	}
	if receivedForm["clientKey"] != "" {
		t.Errorf("clientKey = %q, want empty for simplified fallback", receivedForm["clientKey"])
	}
}

// parseCredentialsBlob is the inverse of BuildCredentialsBlob, used in tests.
func parseCredentialsBlob(data []byte) (*parsedCredentials, error) {
	var r parsedCredentials
	i := 0
	for i < len(data) {
		tag := data[i]
		i++
		fieldNum := tag >> 3
		wireType := tag & 0x07
		switch wireType {
		case 0: // varint
			val, n := readProtoVarint(data[i:])
			i += n
			if fieldNum == 5 {
				r.authType = int(val)
			}
		case 2: // length-delimited
			length, n := readProtoVarint(data[i:])
			i += n
			value := data[i : i+int(length)]
			i += int(length)
			switch fieldNum {
			case 1:
				r.username = string(value)
			case 4:
				r.authData = value
			}
		default:
			return nil, fmt.Errorf("unsupported wire type %d at offset %d", wireType, i-1)
		}
	}
	return &r, nil
}

type parsedCredentials struct {
	username string
	authType int
	authData []byte
}

func readProtoVarint(data []byte) (uint64, int) {
	var val uint64
	for i, b := range data {
		val |= uint64(b&0x7f) << (7 * uint(i))
		if b&0x80 == 0 {
			return val, i + 1
		}
	}
	return 0, len(data)
}

func TestValidateZcBaseURL(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantOK   bool
		wantHost string // expected u.Host on success
		wantPath string
	}{
		{"loopback", "http://127.0.0.1:8200/zc", true, "127.0.0.1:8200", "/zc"},
		{"loopback no port", "http://127.0.0.1/zc", true, "127.0.0.1", "/zc"},
		// The "private 192" and "strips query" cases must use an
		// RFC-1918 192.168/16 value — validateZcBaseURL only accepts
		// loopback, RFC-1918, and link-local. RFC-5737 doc IPs
		// (which we use as placeholders elsewhere) would be rejected
		// here, so the test uses a generic-but-real 192.168 value.
		{"private 192", "http://192.168.10.10:8200/zc", true, "192.168.10.10:8200", "/zc"},
		{"private 10", "http://10.0.0.5/zc", true, "10.0.0.5", "/zc"},
		{"private 172", "http://172.16.0.1/zc", true, "172.16.0.1", "/zc"},
		{"link-local v4", "http://169.254.10.20/zc", true, "169.254.10.20", "/zc"},
		{"ipv6 loopback", "http://[::1]:8200/zc", true, "[::1]:8200", "/zc"},
		{"ipv6 link-local", "http://[fe80::1]:8200/zc", true, "[fe80::1]:8200", "/zc"},
		{"strips query", "http://192.168.10.10:8200/zc?foo=bar", true, "192.168.10.10:8200", "/zc"},

		{"public IP rejected", "http://1.1.1.1/zc", false, "", ""},
		{"public ipv6 rejected", "http://[2001:db8::1]/zc", false, "", ""},
		{"hostname rejected", "http://myspeaker.local/zc", false, "", ""},
		{"plain hostname rejected", "http://speaker/zc", false, "", ""},
		{"ftp scheme rejected", "ftp://192.0.2.10/zc", false, "", ""},
		{"file scheme rejected", "file:///etc/passwd", false, "", ""},
		{"empty host rejected", "http:///zc", false, "", ""},
		{"unparseable rejected", "::not a url::", false, "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateZcBaseURL(tc.input)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("validateZcBaseURL(%q) returned error %v, want success", tc.input, err)
				}
				if got.Host != tc.wantHost {
					t.Errorf("Host = %q, want %q", got.Host, tc.wantHost)
				}
				if got.Path != tc.wantPath {
					t.Errorf("Path = %q, want %q", got.Path, tc.wantPath)
				}
				if got.RawQuery != "" {
					t.Errorf("RawQuery = %q, want empty (validator should strip query)", got.RawQuery)
				}
			} else if err == nil {
				t.Errorf("validateZcBaseURL(%q) succeeded, want error", tc.input)
			}
		})
	}
}

// TestPushCredentials_AddUserNoOp covers the firmware quirk we observed in
// production: ?action=addUser sometimes returns 404 with an empty body when
// the speaker already has the requested user as its active one. That is NOT a
// failure — the speaker silently kept its state. PushCredentials must signal
// this via ErrAddUserNoOp so the watchdog can demote it from "Failed to prime"
// to a benign success.
func TestPushCredentials_AddUserNoOp(t *testing.T) {
	_, speakerPublicBytes, err := GenerateDHKeyPair()
	if err != nil {
		t.Fatalf("speaker keygen: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getInfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":    101,
				"publicKey": base64.StdEncoding.EncodeToString(speakerPublicBytes),
			})
		case "addUser":
			// Firmware no-op: 404 + empty body, no Server / Content-Type header.
			w.WriteHeader(http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err = PushCredentials(srv.URL+"/zc", "gesellix", "fresh-access-token")
	if !errors.Is(err, ErrAddUserNoOp) {
		t.Fatalf("PushCredentials: got %v, want ErrAddUserNoOp", err)
	}
}

// TestPushCredentials_AddUserNoOpInSimplifiedPath asserts the same narrow
// pattern is recognised on the simplified-token fallback (firmware that
// 404s getInfo entirely).
func TestPushCredentials_AddUserNoOpInSimplifiedPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getInfo":
			http.Error(w, "not supported", http.StatusNotFound)
		case "addUser":
			w.WriteHeader(http.StatusNotFound) // empty body
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := PushCredentials(srv.URL+"/zc", "gesellix", "raw-access-token")
	if !errors.Is(err, ErrAddUserNoOp) {
		t.Fatalf("PushCredentials (simplified path): got %v, want ErrAddUserNoOp", err)
	}
}

// TestPushCredentials_AddUserRealError_NotMisclassified guards the narrowness
// of isAddUserNoOp: a 404 *with* a body (or any non-404 error) must still
// surface as a regular error, not the benign sentinel. Otherwise we'd silently
// swallow genuine credential rejections that happen to come back as 4xx.
func TestPushCredentials_AddUserRealError_NotMisclassified(t *testing.T) {
	_, speakerPublicBytes, err := GenerateDHKeyPair()
	if err != nil {
		t.Fatalf("speaker keygen: %v", err)
	}

	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"404 with body should NOT be no-op", http.StatusNotFound, "spotifyError=12 invalid_token"},
		{"400 empty body should NOT be no-op", http.StatusBadRequest, ""},
		{"500 empty body should NOT be no-op", http.StatusInternalServerError, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Query().Get("action") {
				case "getInfo":
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"status":    101,
						"publicKey": base64.StdEncoding.EncodeToString(speakerPublicBytes),
					})
				case "addUser":
					w.WriteHeader(tc.status)
					if tc.body != "" {
						_, _ = w.Write([]byte(tc.body))
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			err := PushCredentials(srv.URL+"/zc", "gesellix", "fresh-access-token")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if errors.Is(err, ErrAddUserNoOp) {
				t.Errorf("got ErrAddUserNoOp, want a real failure for status=%d body=%q", tc.status, tc.body)
			}
		})
	}
}
