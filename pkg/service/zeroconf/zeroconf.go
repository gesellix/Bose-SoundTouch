// Package zeroconf implements the Spotify Connect ZeroConf DH key exchange
// protocol used to push OAuth credentials to SoundTouch speakers.
// Both Spotify and Amazon Music use the same protocol with authType 4.
package zeroconf

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AuthTypeOAuthToken is the protobuf auth_type value for OAuth token credentials
// (AUTHENTICATION_SPOTIFY_TOKEN = 4). Both Spotify and Amazon use this value.
const AuthTypeOAuthToken uint64 = 4

// ErrAddUserNoOp signals a benign 404-with-empty-body reply from the speaker's
// ?action=addUser endpoint. SoundTouch firmware uses that exact response shape
// to mean "no transition required" — typically because the requested
// activeUser is already the active one. It is NOT a credential or transport
// failure; the speaker silently kept its current state. Callers that have
// already written the authoritative source record to marge (the path
// presets/playback actually go through) should treat this as success.
var ErrAddUserNoOp = errors.New("zeroconf: addUser no-op (speaker already in target state)")

// dhPrimeBytes is the 768-bit MODP Group 1 prime from RFC 2409 §6.1.
var dhPrimeBytes = []byte{
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xc9, 0x0f, 0xda, 0xa2, 0x21, 0x68, 0xc2, 0x34,
	0xc4, 0xc6, 0x66, 0x28, 0xb8, 0x0d, 0xc1, 0xcd,
	0x12, 0x90, 0x24, 0xe0, 0x88, 0xa6, 0x7c, 0xc7,
	0x40, 0x20, 0xbb, 0xea, 0x63, 0xb1, 0x39, 0xb2,
	0x25, 0x14, 0xa0, 0x87, 0x98, 0xe3, 0x40, 0x4d,
	0xde, 0xf9, 0x51, 0x9b, 0x3c, 0xd3, 0xa4, 0x31,
	0xb3, 0x02, 0xb0, 0xa6, 0xdf, 0x25, 0xf1, 0x43,
	0x74, 0xfe, 0x13, 0x56, 0xd6, 0xd5, 0x1c, 0x24,
	0x5e, 0x48, 0x5b, 0x57, 0x66, 0x25, 0xe7, 0xec,
	0x6f, 0x44, 0xc4, 0x2e, 0x9a, 0x63, 0xa3, 0x62,
	0x0f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
}

var dhPrime = new(big.Int).SetBytes(dhPrimeBytes)
var dhGenerator = big.NewInt(2)

const dhKeySize = 96 // bytes, matches the 768-bit prime

type getInfoResponse struct {
	PublicKey string `json:"publicKey"`
}

// GenerateDHKeyPair generates a fresh DH private key and derives the public key.
// Both keys are padded to dhKeySize bytes (big-endian).
func GenerateDHKeyPair() (privateKey *big.Int, publicKeyBytes []byte, err error) {
	privBytes := make([]byte, dhKeySize)
	if _, err = rand.Read(privBytes); err != nil {
		return
	}

	privateKey = new(big.Int).SetBytes(privBytes)
	pub := new(big.Int).Exp(dhGenerator, privateKey, dhPrime)
	publicKeyBytes = padBigInt(pub, dhKeySize)

	return
}

// ComputeSharedSecret computes DH(remotePublicKey, privateKey) mod prime.
func ComputeSharedSecret(privateKey *big.Int, remotePublicKeyBytes []byte) []byte {
	remote := new(big.Int).SetBytes(remotePublicKeyBytes)
	shared := new(big.Int).Exp(remote, privateKey, dhPrime)

	return padBigInt(shared, dhKeySize)
}

// DeriveKeys produces a 16-byte AES key and a 20-byte HMAC key from the shared secret.
func DeriveKeys(sharedSecret []byte) (encKey, macKey []byte) {
	h := sha1.Sum(sharedSecret)
	baseKey := h[:16]

	hEnc := hmac.New(sha1.New, baseKey)
	hEnc.Write([]byte("encryption"))
	encKey = hEnc.Sum(nil)[:16]

	hMac := hmac.New(sha1.New, baseKey)
	hMac.Write([]byte("checksum"))
	macKey = hMac.Sum(nil)

	return
}

// BuildCredentialsBlob encodes login credentials as a minimal protobuf
// LoginCredentials message (username=1, typ=5, auth_data=4).
// Pass AuthTypeOAuthToken for both Spotify and Amazon OAuth flows.
func BuildCredentialsBlob(username, accessToken string, authType uint64) []byte {
	var buf bytes.Buffer

	// field 1 (username), wire type 2
	buf.WriteByte(0x0a)
	writeVarint(&buf, uint64(len(username)))
	buf.WriteString(username)

	// field 5 (typ), wire type 0
	buf.WriteByte(0x28)
	writeVarint(&buf, authType)

	// field 4 (auth_data), wire type 2
	buf.WriteByte(0x22)
	writeVarint(&buf, uint64(len(accessToken)))
	buf.WriteString(accessToken)

	return buf.Bytes()
}

// EncryptBlob encrypts plaintext using AES-128-CTR with an HMAC-SHA1 checksum.
// Returns [16-byte IV][ciphertext][20-byte HMAC].
func EncryptBlob(encKey, macKey, plaintext []byte) ([]byte, error) {
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}

	ciphertext := make([]byte, len(plaintext))
	cipher.NewCTR(block, iv).XORKeyStream(ciphertext, plaintext)

	mac := hmac.New(sha1.New, macKey)
	mac.Write(ciphertext)

	out := make([]byte, 0, aes.BlockSize+len(ciphertext)+20)
	out = append(out, iv...)
	out = append(out, ciphertext...)
	out = append(out, mac.Sum(nil)...)

	return out, nil
}

// DecryptBlob reverses EncryptBlob: verifies the HMAC then decrypts.
func DecryptBlob(encKey, macKey, blob []byte) ([]byte, error) {
	const overhead = aes.BlockSize + 20 // IV + HMAC
	if len(blob) < overhead {
		return nil, fmt.Errorf("blob too short (%d bytes)", len(blob))
	}

	iv := blob[:aes.BlockSize]
	ciphertext := blob[aes.BlockSize : len(blob)-20]
	gotMAC := blob[len(blob)-20:]

	mac := hmac.New(sha1.New, macKey)
	mac.Write(ciphertext)

	if !hmac.Equal(mac.Sum(nil), gotMAC) {
		return nil, fmt.Errorf("blob HMAC verification failed")
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}

	plaintext := make([]byte, len(ciphertext))
	cipher.NewCTR(block, iv).XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}

// validateZcBaseURL parses zcBaseURL and ensures the URL points at a
// non-routable host on the LAN. Speakers live on the local network; rejecting
// non-local hosts prevents the upstream caller from being tricked into
// making outbound requests to arbitrary hosts (server-side request forgery).
//
// The validator is strict on purpose:
//   - the scheme must be http or https,
//   - the host must be a *literal IP* (no DNS / mDNS hostnames — see note
//     below) that is loopback, RFC1918 private, or IPv4/IPv6 link-local,
//   - the returned URL is rebuilt from validated components so the
//     subsequent String() call no longer carries the original tainted host
//     value, which CodeQL recognises as taint sanitisation.
//
// Note on hostnames: SoundTouch speakers announce themselves with
// IP-based zeroconf URLs in the captures we have. If a future deployment
// needs mDNS support, the right place to add it is in the caller — resolve
// the hostname to an IP and pass the IP-form URL in here. Doing the lookup
// inside the validator would re-introduce the very SSRF surface CodeQL is
// flagging, because malicious DNS could point a *.local name at a
// public host between the lookup and the request.
func validateZcBaseURL(zcBaseURL string) (*url.URL, error) {
	u, err := url.Parse(zcBaseURL)
	if err != nil {
		return nil, fmt.Errorf("zeroconf URL %q: parse: %w", zcBaseURL, err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("zeroconf URL %q: scheme %q not allowed — must be http or https", zcBaseURL, u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("zeroconf URL %q: missing host", zcBaseURL)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf(
			"zeroconf URL %q: host %q must be a literal IP — resolve the hostname to a private-network IP first "+
				"(e.g. `getent hosts %s` or `dig +short %s`) and retry with the resolved address",
			zcBaseURL, host, host, host)
	}

	if !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() {
		return nil, fmt.Errorf(
			"zeroconf URL %q: host %q is not on a local network — only loopback (127.0.0.0/8, ::1), "+
				"RFC1918 private (10/8, 172.16/12, 192.168/16) and link-local (169.254/16, fe80::/10) "+
				"addresses are accepted",
			zcBaseURL, host)
	}

	// Build a fresh URL from validated components only — the IP literal,
	// the original port, the original path. Pre-existing ?query and
	// #fragment are stripped so callers can attach their own cleanly.
	hostPort := ip.String()
	if port := u.Port(); port != "" {
		hostPort = net.JoinHostPort(ip.String(), port)
	}

	return &url.URL{
		Scheme: u.Scheme,
		Host:   hostPort,
		Path:   u.Path,
	}, nil
}

// withAction returns the validated base URL with ?action=<action> appended.
func withAction(base *url.URL, action string) string {
	u := *base
	q := u.Query()
	q.Set("action", action)
	u.RawQuery = q.Encode()

	return u.String()
}

// GetInfo fetches the speaker's DH public key via GET ?action=getInfo.
func GetInfo(zcBaseURL string) ([]byte, error) {
	base, err := validateZcBaseURL(zcBaseURL)
	if err != nil {
		return nil, fmt.Errorf("getInfo: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(withAction(base, "getInfo"))
	if err != nil {
		return nil, fmt.Errorf("getInfo: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getInfo: status %d", resp.StatusCode)
	}

	var info getInfoResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&info); decodeErr != nil {
		return nil, fmt.Errorf("getInfo: decode: %w", decodeErr)
	}

	if info.PublicKey == "" {
		return nil, fmt.Errorf("getInfo: empty publicKey")
	}

	// Accept both standard and URL-safe base64.
	pubKey, err := base64.StdEncoding.DecodeString(info.PublicKey)
	if err != nil {
		pubKey, err = base64.URLEncoding.DecodeString(info.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("getInfo: invalid base64 publicKey: %w", err)
		}
	}

	return pubKey, nil
}

// PushCredentials pushes OAuth credentials to a speaker using the ZeroConf DH
// key exchange protocol. If getInfo fails (older firmware without DH support),
// it falls back to the simplified tokenType=accesstoken approach.
// zcBaseURL is the base URL of the ZeroConf endpoint, e.g. "http://192.168.10.10:8200/zc".
func PushCredentials(zcBaseURL, username, accessToken string) error {
	base, err := validateZcBaseURL(zcBaseURL)
	if err != nil {
		return fmt.Errorf("pushCredentials: %w", err)
	}

	speakerPublicKey, err := GetInfo(zcBaseURL)
	if err != nil {
		log.Printf("[ZeroConf] getInfo failed (%v), falling back to simplified token push", err)
		return pushSimplifiedToken(zcBaseURL, username, accessToken)
	}

	privateKey, ourPublicKeyBytes, err := GenerateDHKeyPair()
	if err != nil {
		return fmt.Errorf("pushCredentials: keygen: %w", err)
	}

	sharedSecret := ComputeSharedSecret(privateKey, speakerPublicKey)
	encKey, macKey := DeriveKeys(sharedSecret)

	plaintext := BuildCredentialsBlob(username, accessToken, AuthTypeOAuthToken)

	encryptedBlob, err := EncryptBlob(encKey, macKey, plaintext)
	if err != nil {
		return fmt.Errorf("pushCredentials: encrypt: %w", err)
	}

	data := url.Values{}
	data.Set("userName", username)
	data.Set("blob", base64.StdEncoding.EncodeToString(encryptedBlob))
	data.Set("clientKey", base64.StdEncoding.EncodeToString(ourPublicKeyBytes))

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.PostForm(withAction(base, "addUser"), data)
	if err != nil {
		return fmt.Errorf("pushCredentials: addUser: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if isAddUserNoOp(resp.StatusCode, body) {
			logAddUserNoOp("DH", base, username, resp)

			return ErrAddUserNoOp
		}

		logAddUserFailure("DH", base, username, resp, body)

		return fmt.Errorf("pushCredentials: addUser status %d: %s", resp.StatusCode, body)
	}

	return nil
}

// isAddUserNoOp recognises the narrow firmware pattern (status 404, empty body)
// that signals "no transition required". Anything else — including 404 with a
// body, or any other non-2xx — falls through to the real failure path so we
// don't silently swallow genuine errors.
func isAddUserNoOp(status int, body []byte) bool {
	return status == http.StatusNotFound && len(bytes.TrimSpace(body)) == 0
}

// logAddUserNoOp emits a single line marking the benign no-op explicitly —
// kept visible (not Debug-level) so the operator can correlate it with priming
// runs, but worded so it's clearly not a failure.
func logAddUserNoOp(path string, base *url.URL, username string, resp *http.Response) {
	log.Printf("[ZeroConf] addUser produced expected no-op via %s path (speaker already has activeUser=%q or equivalent state): url=%s status=%d body=<empty> — marge source registration is authoritative for preset/playback",
		path, sanitizeLog(username), withAction(base, "addUser"), resp.StatusCode)
}

// logAddUserFailure emits a single diagnostic line capturing what the speaker
// said about an `?action=addUser` rejection. Bose firmware often returns 4xx
// with an empty body, so the headers (libspotify version, content-type,
// content-length) are the only clue about whether the speaker refused the
// transition, the credential, or the action entirely. Kept verbose on purpose —
// these failures are rare and worth grepping for.
func logAddUserFailure(path string, base *url.URL, username string, resp *http.Response, body []byte) {
	ct := resp.Header.Get("Content-Type")
	cl := resp.Header.Get("Content-Length")
	server := resp.Header.Get("Server")

	bodySummary := strings.TrimSpace(string(body))
	if bodySummary == "" {
		bodySummary = "<empty>"
	}

	log.Printf("[ZeroConf] addUser rejected via %s path: url=%s userName=%q status=%d server=%q content-type=%q content-length=%q body=%q",
		path, withAction(base, "addUser"), sanitizeLog(username), resp.StatusCode, sanitizeLog(server), sanitizeLog(ct), sanitizeLog(cl), sanitizeLog(bodySummary))
}

// pushSimplifiedToken is the fallback for firmware that does not support DH
// key exchange. It sends the raw OAuth access token directly as the blob.
func pushSimplifiedToken(zcBaseURL, username, accessToken string) error {
	base, err := validateZcBaseURL(zcBaseURL)
	if err != nil {
		return fmt.Errorf("pushSimplifiedToken: %w", err)
	}

	data := url.Values{}
	data.Set("userName", username)
	data.Set("blob", accessToken)
	data.Set("clientKey", "")
	data.Set("tokenType", "accesstoken")

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.PostForm(withAction(base, "addUser"), data)
	if err != nil {
		return fmt.Errorf("pushSimplifiedToken: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if isAddUserNoOp(resp.StatusCode, body) {
			logAddUserNoOp("simplified", base, username, resp)

			return ErrAddUserNoOp
		}

		logAddUserFailure("simplified", base, username, resp, body)

		return fmt.Errorf("pushSimplifiedToken: status %d: %s", resp.StatusCode, body)
	}

	return nil
}

func padBigInt(n *big.Int, size int) []byte {
	b := n.Bytes()
	if len(b) >= size {
		return b
	}

	out := make([]byte, size)
	copy(out[size-len(b):], b)

	return out
}

func writeVarint(buf *bytes.Buffer, v uint64) {
	for v >= 0x80 {
		buf.WriteByte(byte(v) | 0x80)
		v >>= 7
	}

	buf.WriteByte(byte(v))
}
