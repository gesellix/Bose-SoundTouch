package spotify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/zeroconf"
)

// TestPushSpotifyCredentials_FullRoundTrip starts a mock "speaker" ZeroConf server,
// has it generate its own DH key pair, and verifies that the client correctly
// encrypts and delivers the Spotify credentials.
func TestPushSpotifyCredentials_FullRoundTrip(t *testing.T) {
	speakerPrivate, speakerPublicBytes, err := zeroconf.GenerateDHKeyPair()
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

			shared := zeroconf.ComputeSharedSecret(speakerPrivate, clientKeyBytes)
			encKey, macKey := zeroconf.DeriveKeys(shared)

			plaintext, err := zeroconf.DecryptBlob(encKey, macKey, blobBytes)
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

	const wantUsername = "spotifyuser@example.com"
	const wantToken = "eyJhbGciOiJSUzI1NiJ9.access-token"

	if err := PushSpotifyCredentials(srv.URL+"/zc", wantUsername, wantToken); err != nil {
		t.Fatalf("PushSpotifyCredentials: %v", err)
	}

	if got.username != wantUsername {
		t.Errorf("username = %q, want %q", got.username, wantUsername)
	}
	if got.authData != wantToken {
		t.Errorf("authData = %q, want %q", got.authData, wantToken)
	}
	if got.authType != 4 {
		t.Errorf("authType = %d, want 4 (AUTHENTICATION_SPOTIFY_TOKEN)", got.authType)
	}
}

// TestPushSpotifyCredentials_FallbackOnGetInfoFailure verifies that when getInfo
// returns a non-200 response (older firmware without DH support), PushSpotifyCredentials
// falls back to the simplified tokenType=accesstoken POST.
func TestPushSpotifyCredentials_FallbackOnGetInfoFailure(t *testing.T) {
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

	const wantUsername = "spotifyuser@example.com"
	const wantToken = "raw-access-token"

	if err := PushSpotifyCredentials(srv.URL+"/zc", wantUsername, wantToken); err != nil {
		t.Fatalf("PushSpotifyCredentials: %v", err)
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

type parsedCredentials struct {
	username string
	authType int
	authData []byte
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
