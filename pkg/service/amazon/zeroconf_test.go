package amazon

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/zeroconf"
)

func TestPushAmazonCredentials_FullRoundTrip(t *testing.T) {
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

			// Minimal protobuf parse: field 1 = username, field 4 = authData, field 5 = authType
			i := 0
			for i < len(plaintext) {
				tag := plaintext[i]
				i++
				fieldNum := tag >> 3
				wireType := tag & 0x07
				switch wireType {
				case 0:
					val, n := readVarint(plaintext[i:])
					i += n
					if fieldNum == 5 {
						got.authType = int(val)
					}
				case 2:
					length, n := readVarint(plaintext[i:])
					i += n
					value := plaintext[i : i+int(length)]
					i += int(length)
					switch fieldNum {
					case 1:
						got.username = string(value)
					case 4:
						got.authData = string(value)
					}
				default:
					http.Error(w, "unexpected wire type", http.StatusBadRequest)
					return
				}
			}
			w.WriteHeader(http.StatusOK)

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	const wantUsername = "amazonuser@example.com"
	const wantToken = "Atza|access-token"

	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	if err := PushAmazonCredentials(host, port, wantUsername, wantToken); err != nil {
		t.Fatalf("PushAmazonCredentials: %v", err)
	}

	if got.username != wantUsername {
		t.Errorf("username = %q, want %q", got.username, wantUsername)
	}
	if got.authData != wantToken {
		t.Errorf("authData = %q, want %q", got.authData, wantToken)
	}
	if uint64(got.authType) != zeroconf.AuthTypeOAuthToken {
		t.Errorf("authType = %d, want %d (AuthTypeOAuthToken)", got.authType, zeroconf.AuthTypeOAuthToken)
	}
}

func TestPushAmazonCredentials_FallbackOnGetInfoFailure(t *testing.T) {
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

	const wantUsername = "amazonuser@example.com"
	const wantToken = "Atza|raw-access-token"

	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	if err := PushAmazonCredentials(host, port, wantUsername, wantToken); err != nil {
		t.Fatalf("PushAmazonCredentials: %v", err)
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

func readVarint(data []byte) (uint64, int) {
	var val uint64
	for i, b := range data {
		val |= uint64(b&0x7f) << (7 * uint(i))
		if b&0x80 == 0 {
			return val, i + 1
		}
	}
	return 0, len(data)
}
