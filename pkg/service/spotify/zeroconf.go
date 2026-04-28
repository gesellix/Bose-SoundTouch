package spotify

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"time"
)

// dhPrimeBytes is the 768-bit MODP Group 1 prime from RFC 2409 §6.1.
// Spotify Connect ZeroConf uses this group for the DH key exchange.
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

type zcGetInfoResponse struct {
	PublicKey string `json:"publicKey"`
}

// generateDHKeyPair generates a fresh DH private key and derives the public key.
// Both keys are padded to dhKeySize bytes (big-endian).
func generateDHKeyPair() (privateKey *big.Int, publicKeyBytes []byte, err error) {
	privBytes := make([]byte, dhKeySize)
	if _, err = rand.Read(privBytes); err != nil {
		return
	}
	privateKey = new(big.Int).SetBytes(privBytes)
	pub := new(big.Int).Exp(dhGenerator, privateKey, dhPrime)
	publicKeyBytes = padBigInt(pub, dhKeySize)
	return
}

// computeSharedSecret computes DH(remotePublicKey, privateKey) mod prime.
func computeSharedSecret(privateKey *big.Int, remotePublicKeyBytes []byte) []byte {
	remote := new(big.Int).SetBytes(remotePublicKeyBytes)
	shared := new(big.Int).Exp(remote, privateKey, dhPrime)
	return padBigInt(shared, dhKeySize)
}

// deriveKeys produces a 16-byte AES key and a 20-byte HMAC key from the shared secret.
func deriveKeys(sharedSecret []byte) (encKey, macKey []byte) {
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

// buildCredentialsBlob encodes Spotify login credentials as a minimal protobuf
// LoginCredentials message (username=1, typ=5, auth_data=4).
// typ=4 = AUTHENTICATION_SPOTIFY_TOKEN.
func buildCredentialsBlob(username, accessToken string) []byte {
	var buf bytes.Buffer

	// field 1 (username), wire type 2
	buf.WriteByte(0x0a)
	writeVarint(&buf, uint64(len(username)))
	buf.WriteString(username)

	// field 5 (typ), wire type 0; value 4 = AUTHENTICATION_SPOTIFY_TOKEN
	buf.WriteByte(0x28)
	writeVarint(&buf, 4)

	// field 4 (auth_data), wire type 2
	buf.WriteByte(0x22)
	writeVarint(&buf, uint64(len(accessToken)))
	buf.WriteString(accessToken)

	return buf.Bytes()
}

// encryptBlob encrypts plaintext using AES-128-CTR with an HMAC-SHA1 checksum.
// Returns [16-byte IV][ciphertext][20-byte HMAC].
func encryptBlob(encKey, macKey, plaintext []byte) ([]byte, error) {
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

// decryptBlob reverses encryptBlob: verifies the HMAC then decrypts.
func decryptBlob(encKey, macKey, blob []byte) ([]byte, error) {
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

// ZeroConfGetInfo fetches the speaker's DH public key via GET ?action=getInfo.
func ZeroConfGetInfo(zcBaseURL string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(zcBaseURL + "?action=getInfo")
	if err != nil {
		return nil, fmt.Errorf("getInfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getInfo: status %d", resp.StatusCode)
	}

	var info zcGetInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("getInfo: decode: %w", err)
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

// PushSpotifyCredentials pushes Spotify credentials to a speaker using the full
// Spotify Connect ZeroConf protocol (DH key exchange + encrypted credential blob).
// If getInfo fails (e.g. older firmware without DH support), it falls back to the
// simplified tokenType=accesstoken approach.
// zcBaseURL is the base URL of the ZeroConf endpoint, e.g. "http://192.168.1.10:8200/zc".
func PushSpotifyCredentials(zcBaseURL, username, accessToken string) error {
	speakerPublicKey, err := ZeroConfGetInfo(zcBaseURL)
	if err != nil {
		log.Printf("[ZeroConf] getInfo failed (%v), falling back to simplified token push", err)
		return pushSimplifiedToken(zcBaseURL, username, accessToken)
	}

	privateKey, ourPublicKeyBytes, err := generateDHKeyPair()
	if err != nil {
		return fmt.Errorf("pushSpotifyCredentials: keygen: %w", err)
	}

	sharedSecret := computeSharedSecret(privateKey, speakerPublicKey)
	encKey, macKey := deriveKeys(sharedSecret)

	plaintext := buildCredentialsBlob(username, accessToken)
	encryptedBlob, err := encryptBlob(encKey, macKey, plaintext)
	if err != nil {
		return fmt.Errorf("pushSpotifyCredentials: encrypt: %w", err)
	}

	data := url.Values{}
	data.Set("userName", username)
	data.Set("blob", base64.StdEncoding.EncodeToString(encryptedBlob))
	data.Set("clientKey", base64.StdEncoding.EncodeToString(ourPublicKeyBytes))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm(zcBaseURL+"?action=addUser", data)
	if err != nil {
		return fmt.Errorf("pushSpotifyCredentials: addUser: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pushSpotifyCredentials: addUser status %d: %s", resp.StatusCode, body)
	}
	return nil
}

// pushSimplifiedToken is the fallback for firmware that does not support the DH
// key exchange. It sends the raw OAuth access token directly as the blob with
// tokenType=accesstoken. The token will expire after ~60 minutes.
func pushSimplifiedToken(zcBaseURL, username, accessToken string) error {
	data := url.Values{}
	data.Set("userName", username)
	data.Set("blob", accessToken)
	data.Set("clientKey", "")
	data.Set("tokenType", "accesstoken")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm(zcBaseURL+"?action=addUser", data)
	if err != nil {
		return fmt.Errorf("pushSimplifiedToken: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
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