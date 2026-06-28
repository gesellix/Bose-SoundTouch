package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	defaultAuthProbeTTL     = 30 * time.Second
	defaultAuthProbeTimeout = 8 * time.Second
	authProbePollInterval   = 200 * time.Millisecond
)

// authProbe holds the state for a single active DNS-path probe.
type authProbe struct {
	Nonce        string
	DeviceID     string
	TargetIP     string // datastore IP the notification was sent to
	CreatedAt    time.Time
	ExpiresAt    time.Time
	ObservedFrom string    // source IP of the /v1/auth callback (set on hit)
	ObservedAt   time.Time // zero until observed
}

// authProbeRegistry is an in-memory, mutex-guarded registry of active probes.
// Expired entries are pruned opportunistically on each write operation.
type authProbeRegistry struct {
	mu     sync.Mutex
	active map[string]*authProbe // nonce -> probe
	ttl    time.Duration         // default defaultAuthProbeTTL; injectable for tests
}

// newAuthProbeRegistry creates a new registry with the given ttl.
func newAuthProbeRegistry(ttl time.Duration) *authProbeRegistry {
	if ttl <= 0 {
		ttl = defaultAuthProbeTTL
	}

	return &authProbeRegistry{
		active: make(map[string]*authProbe),
		ttl:    ttl,
	}
}

// pruneExpired removes expired entries. Caller must hold r.mu.
func (r *authProbeRegistry) pruneExpired() {
	now := time.Now()

	for nonce, p := range r.active {
		if now.After(p.ExpiresAt) {
			delete(r.active, nonce)
		}
	}
}

// register stores a new probe. Prunes expired entries first.
func (r *authProbeRegistry) register(nonce, deviceID, targetIP string) {
	now := time.Now()
	p := &authProbe{
		Nonce:     nonce,
		DeviceID:  deviceID,
		TargetIP:  targetIP,
		CreatedAt: now,
		ExpiresAt: now.Add(r.ttl),
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.pruneExpired()
	r.active[nonce] = p
}

// observe marks the probe as observed if the nonce matches an active,
// non-expired entry. Returns true if the probe was found and recorded.
// No loopback filtering: every matching callback is a genuine probe
// response by construction, and tests drive it from loopback.
func (r *authProbeRegistry) observe(nonce, sourceIP string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.active[nonce]
	if !ok {
		return false
	}

	if time.Now().After(p.ExpiresAt) {
		return false
	}

	p.ObservedFrom = sourceIP
	p.ObservedAt = time.Now()

	return true
}

// get returns a snapshot of the probe for the given nonce, or false if not present.
func (r *authProbeRegistry) get(nonce string) (*authProbe, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.active[nonce]
	if !ok {
		return nil, false
	}

	// Return a copy so the caller never races on the struct fields.
	snapshot := *p

	return &snapshot, true
}

// deregister removes the probe after it completes.
func (r *authProbeRegistry) deregister(nonce string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.active, nonce)
}

// generateNonce returns a 32-char hex-encoded random nonce prefixed with "atp_".
func generateNonce() (string, error) {
	buf := make([]byte, 16)

	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	return "atp_" + hex.EncodeToString(buf), nil
}

// clientHostFromRemoteAddr extracts the host part from a "host:port" RemoteAddr.
// If parsing fails it returns the raw value unchanged.
func clientHostFromRemoteAddr(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}

	return remoteAddr
}

// clientHost returns the resolved client IP for r: chi's middleware.GetClientIP
// (populated by the ClientIP middleware) when set, falling back to the socket
// peer host from r.RemoteAddr. Returns a bare IP (no port).
func clientHost(r *http.Request) string {
	if ip := middleware.GetClientIP(r.Context()); ip != "" {
		return ip
	}

	return clientHostFromRemoteAddr(r.RemoteAddr)
}

// dnsProbeSpeakerRequest is the JSON body for POST /setup/health/dns-path-probe.
type dnsProbeSpeakerRequest struct {
	DeviceID string `json:"deviceId,omitempty"`
	Host     string `json:"host,omitempty"`
}

// dnsProbeSpeakerResponse is the JSON response body for the dns-path probe.
type dnsProbeSpeakerResponse struct {
	Success      bool    `json:"success"`
	ObservedFrom string  `json:"observedFrom,omitempty"`
	LatencyMs    float64 `json:"latencyMs,omitempty"`
	NATOrProxy   bool    `json:"natOrProxy,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	Remediation  string  `json:"remediation,omitempty"`
}

// runDNSPathProbe contains the core probe logic: resolve the target IP,
// register the nonce, send the PlayURL notification, poll for the /v1/auth
// callback up to the configured timeout, deregister the nonce, and return the
// result. It is the shared implementation used by both HandleDNSPathProbe and
// the health QuickFix registered in server.go.
func (s *Server) runDNSPathProbe(deviceID, host string) (dnsProbeSpeakerResponse, error) {
	// Reuse resolveTTSHost for SSRF-safe target resolution (always a datastore IP).
	targetIP, err := s.resolveTTSHost(ttsSpeakRequest{
		DeviceID: deviceID,
		Host:     host,
	})
	if err != nil {
		return dnsProbeSpeakerResponse{}, err
	}

	nonce, err := generateNonce()
	if err != nil {
		return dnsProbeSpeakerResponse{}, fmt.Errorf("failed to generate probe nonce: %w", err)
	}

	// Register BEFORE sending the notification so a fast callback matches.
	s.authProbes.register(nonce, deviceID, targetIP)

	defer s.authProbes.deregister(nonce)

	// Build the probe URL using the service's own base URL. The speaker
	// refuses the notification on 403 before fetching this URL.
	probeURL := s.serverURL + "/media/tts/dns-probe"

	c := client.NewClientFromHost(targetIP)

	if err := c.PlayURL(probeURL, nonce, "AfterTouch DNS probe", "DNS path probe", ""); err != nil {
		return dnsProbeSpeakerResponse{
			Success: false,
			Reason:  "speaker unreachable or notification not accepted: " + err.Error(),
		}, nil
	}

	// Determine the polling timeout: use injected value or default.
	timeout := s.authProbeTimeout()
	deadline := time.Now().Add(timeout)

	for {
		p, ok := s.authProbes.get(nonce)
		if ok && !p.ObservedAt.IsZero() {
			latency := p.ObservedAt.Sub(p.CreatedAt).Seconds() * 1000

			return dnsProbeSpeakerResponse{
				Success:      true,
				ObservedFrom: p.ObservedFrom,
				LatencyMs:    latency,
				NATOrProxy:   p.ObservedFrom != targetIP,
			}, nil
		}

		if time.Now().After(deadline) {
			break
		}

		time.Sleep(authProbePollInterval)
	}

	// Timeout path: notification was accepted but no /v1/auth callback arrived.
	return dnsProbeSpeakerResponse{
		Success: false,
		Reason:  "notification accepted but no /v1/auth callback received within timeout",
		Remediation: "The speaker likely resolves Bose hostnames via a different DNS resolver, " +
			"not AfterTouch's DNS server. " +
			"To fix: point speakers at AfterTouch's DNS server via DHCP option 6, " +
			"configure the upstream resolvers to forward Bose zones to AfterTouch, " +
			"or re-run migration with the resolv method.",
	}, nil
}

// HandleDNSPathProbe actively tests whether a specific speaker resolves Bose
// hostnames through AfterTouch's DNS by sending a /speaker notification with
// a per-probe nonce as the app_key and waiting for the speaker to call back
// at GET /v1/auth with that nonce in the Apikeyheader header.
//
// POST /setup/health/dns-path-probe
// Body: {"deviceId":"...","host":"..."}
func (s *Server) HandleDNSPathProbe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req dnsProbeSpeakerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	resp, err := s.runDNSPathProbe(req.DeviceID, req.Host)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// authProbeTimeout returns the configured probe timeout for polling. It reads
// the unexported field so tests can inject a short value without a setter.
// Falls back to the default when the field is zero.
func (s *Server) authProbeTimeout() time.Duration {
	if s.authProbeTimeoutOverride > 0 {
		return s.authProbeTimeoutOverride
	}

	return defaultAuthProbeTimeout
}
