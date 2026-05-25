package stockholm

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var blockedRequestHeaders = map[string]bool{
	"access-control-request-headers": true,
	"access-control-request-method":  true,
	"connection":                     true,
	"content-length":                 true,
	"cookie":                         true,
	"forwarded":                      true,
	"host":                           true,
	"http2-settings":                 true,
	"keep-alive":                     true,
	"origin":                         true,
	"proxy-authenticate":             true,
	"proxy-authorization":            true,
	"referer":                        true,
	"sec-ch-ua":                      true,
	"sec-ch-ua-mobile":               true,
	"sec-ch-ua-platform":             true,
	"sec-fetch-dest":                 true,
	"sec-fetch-mode":                 true,
	"sec-fetch-site":                 true,
	"sec-fetch-user":                 true,
	"te":                             true,
	"trailer":                        true,
	"transfer-encoding":              true,
	"upgrade":                        true,
	"x-forwarded-for":                true,
	"x-forwarded-host":               true,
	"x-forwarded-port":               true,
	"x-forwarded-proto":              true,
	"x-real-ip":                      true,
	"x-requested-with":               true,
}

var blockedResponseHeaders = map[string]bool{
	"access-control-allow-credentials": true,
	"access-control-allow-headers":     true,
	"access-control-allow-methods":     true,
	"access-control-allow-origin":      true,
	"access-control-expose-headers":    true,
	"access-control-max-age":           true,
	"connection":                       true,
	"content-length":                   true,
	"keep-alive":                       true,
	"proxy-authenticate":               true,
	"proxy-authorization":              true,
	"set-cookie":                       true,
	"set-cookie2":                      true,
	"te":                               true,
	"trailer":                          true,
	"transfer-encoding":                true,
	"upgrade":                          true,
}

var proxyHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return nil // follow redirects
	},
}

// HandleProxy serves the /api/http-proxy endpoint.
func HandleProxy(w http.ResponseWriter, r *http.Request, cfg *Config, state *NativeState) {
	encodedTarget := r.URL.Query().Get("url")
	if encodedTarget == "" {
		http.Error(w, "Missing url query parameter", http.StatusBadRequest)
		return
	}

	decodedTarget, err := url.QueryUnescape(encodedTarget)
	if err != nil {
		http.Error(w, "Invalid proxy target encoding", http.StatusBadRequest)
		return
	}

	target, err := url.Parse(decodedTarget)
	if err != nil || target.Host == "" {
		http.Error(w, "Invalid proxy target", http.StatusBadRequest)
		return
	}

	scheme := strings.ToLower(target.Scheme)
	if scheme != "http" && scheme != "https" {
		http.Error(w, "Unsupported proxy target", http.StatusBadRequest)
		return
	}

	if isProxyLoop(r, target) {
		http.Error(w, "Refusing to proxy proxy endpoint", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadGateway)
		return
	}

	effectiveTarget := applyOverrideURL(target, cfg, state)

	resp, err := executeProxyRequest(r, effectiveTarget, body, cfg, state)
	if err != nil {
		log.Printf("[Stockholm proxy] %s %s failed: %s", r.Method, sanitizeLog(effectiveTarget.String()), sanitizeErr(err))
		http.Error(w, "Proxy request failed", http.StatusBadGateway)

		return
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read proxy response", http.StatusBadGateway)
		return
	}

	// Login retry logic
	if isLoginRequest(r.Method, effectiveTarget) {
		resp, respBody, effectiveTarget = handleLoginRetry(r, effectiveTarget, body, resp, respBody, cfg, state)
		captureSuccessfulLogin(resp, respBody, state)
	}

	captureRefreshedToken(effectiveTarget, resp, cfg, state)
	relayProxyResponse(w, r.Method, resp, respBody)
}

func executeProxyRequest(r *http.Request, target *url.URL, body []byte, cfg *Config, state *NativeState) (*http.Response, error) {
	method := strings.ToUpper(r.Method)

	var bodyReader io.Reader
	if method != "GET" && method != "HEAD" && len(body) > 0 {
		bodyReader = strings.NewReader(string(body))
	}

	req, err := http.NewRequestWithContext(r.Context(), method, target.String(), bodyReader)
	if err != nil {
		return nil, err
	}

	// Forward allowed request headers
	for k, vals := range r.Header {
		if blockedRequestHeaders[strings.ToLower(k)] {
			continue
		}

		for _, v := range sanitizeHeaderValues(vals) {
			req.Header.Add(k, v)
		}
	}

	// Inject backend headers
	injectBackendHeaders(req, target, cfg, state)

	return proxyHTTPClient.Do(req)
}

func injectBackendHeaders(req *http.Request, target *url.URL, cfg *Config, state *NativeState) {
	host := target.Hostname()
	path := target.Path

	if cfg.IsBmxTarget(host) {
		injectIfMissing(req, "x-bmx-api-key", cfg.EncryptedBmxToken)
		injectIfMissing(req, "x-software-version", cfg.AppVersion)
	}

	if cfg.IsMargeTarget(host, path) {
		mediaType := cfg.MediaTypeForPath(path)
		injectIfMissing(req, "Accept", mediaType)
		injectIfMissing(req, "Content-Type", mediaType)
		injectIfMissing(req, "ClientType", "SOUNDTOUCH_COMPUTER_APP")
		injectIfMissing(req, "GUID", firstNonEmpty(state.Get("guid"), state.Get("deviceGuid")))
		injectIfMissing(req, "version_NativeFrameVersion", state.Get("nativeFrameVersion"))
		injectIfMissing(req, "version_StockholmVersion", cfg.AppVersion)
		injectIfMissing(req, "version_ProtocolVersion", cfg.ProtocolVersion)

		if cfg.MargeServerKeyHeader != "" && cfg.MargeServerKey != "" {
			injectIfMissing(req, cfg.MargeServerKeyHeader, cfg.MargeServerKey)
		}

		if shouldInjectAuth(path) {
			injectIfMissing(req, "Authorization", state.Get("margeAuthToken"))
		}
	}
}

func shouldInjectAuth(path string) bool {
	p := strings.ToLower(path)
	if strings.HasSuffix(p, "/streaming/account/login") {
		return false
	}

	if p == "/streaming/account" || p == "/streaming/account/" {
		return false
	}

	if strings.Contains(p, "/streaming/account/email/") && strings.HasSuffix(p, "/environment") {
		return false
	}

	if strings.HasPrefix(p, "/customer/account/password/email/") {
		return false
	}

	return true
}

func injectIfMissing(req *http.Request, name, value string) {
	if value == "" {
		return
	}

	if req.Header.Get(name) != "" {
		return
	}

	req.Header.Set(name, value)
}

func sanitizeHeaderValues(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		t := strings.TrimSpace(v)
		if t == "" || strings.EqualFold(t, "null") || strings.EqualFold(t, "undefined") {
			continue
		}

		out = append(out, t)
	}

	return out
}

func handleLoginRetry(
	r *http.Request,
	target *url.URL,
	body []byte,
	resp *http.Response,
	respBody []byte,
	cfg *Config,
	state *NativeState,
) (*http.Response, []byte, *url.URL) {
	if extractXMLStatusCode(respBody) != "4033" {
		return resp, respBody, target
	}

	email, password := parseLoginCredentials(body)
	if email == "" {
		return resp, respBody, target
	}

	env := fetchEnvironment(r, target, email, password, cfg, state)
	if env == nil || env.streamingURL == "" {
		return resp, respBody, target
	}

	state.PutMany(map[string]string{
		"overrideMargeURL":  normalizeBaseURL(env.streamingURL),
		"overrideUpdateURL": normalizeBaseURL(env.updateURL),
	})

	retryTarget := buildURIFromBase(env.streamingURL, target.Path, target.RawQuery)
	if retryTarget == nil {
		return resp, respBody, target
	}

	_ = resp.Body.Close()

	retryResp, err := executeProxyRequest(r, retryTarget, body, cfg, state)
	if err != nil {
		log.Printf("[Stockholm proxy] Login retry failed: %v", err)
		return resp, respBody, target
	}

	retryBody, err := io.ReadAll(retryResp.Body)
	_ = retryResp.Body.Close()

	if err != nil {
		return resp, respBody, target
	}

	return retryResp, retryBody, retryTarget
}

func fetchEnvironment(r *http.Request, loginTarget *url.URL, email, password string, cfg *Config, state *NativeState) *environmentInfo {
	// Build environment URL
	prefix := margePathPrefix(loginTarget.Path)
	envPath := prefix + "/streaming/account/email/" + url.PathEscape(email) + "/environment"

	envTarget := buildURIFromBase(loginTarget.Scheme+"://"+loginTarget.Host+"/", envPath, "")
	if envTarget == nil {
		return nil
	}

	envReq, err := http.NewRequestWithContext(r.Context(), "GET", envTarget.String(), nil)
	if err != nil {
		return nil
	}

	// Copy allowed headers from original request
	for k, vals := range r.Header {
		if blockedRequestHeaders[strings.ToLower(k)] {
			continue
		}

		for _, v := range sanitizeHeaderValues(vals) {
			envReq.Header.Add(k, v)
		}
	}

	raw := email + ":" + password
	envReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(raw)))
	injectBackendHeaders(envReq, envTarget, cfg, state)

	envResp, err := proxyHTTPClient.Do(envReq)
	if err != nil {
		return nil
	}

	defer func() { _ = envResp.Body.Close() }()

	if envResp.StatusCode != 200 {
		return nil
	}

	body, _ := io.ReadAll(envResp.Body)

	return extractEnvironment(body)
}

func captureSuccessfulLogin(resp *http.Response, body []byte, state *NativeState) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}

	updates := make(map[string]string)

	if accountID := extractXMLAccountID(body); accountID != "" {
		updates["margeAccountID"] = accountID
	}

	if creds := resp.Header.Get("Credentials"); creds != "" {
		updates["margeAuthToken"] = creds
	}

	if len(updates) > 0 {
		state.PutMany(updates)
	}
}

func captureRefreshedToken(target *url.URL, resp *http.Response, cfg *Config, state *NativeState) {
	if target == nil || !cfg.IsMargeTarget(target.Hostname(), target.Path) {
		return
	}

	if refreshed := resp.Header.Get("Refresh"); refreshed != "" {
		state.Set("margeAuthToken", refreshed)
	}
}

func relayProxyResponse(w http.ResponseWriter, method string, resp *http.Response, body []byte) {
	for k, vals := range resp.Header {
		kl := strings.ToLower(k)
		if kl == "" || strings.HasPrefix(kl, ":") || blockedResponseHeaders[kl] {
			continue
		}

		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}

	w.Header().Set("Cache-Control", "no-store")

	bodyAllowed := method != "HEAD" && resp.StatusCode != 204 && resp.StatusCode != 304
	if bodyAllowed {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	}

	w.WriteHeader(resp.StatusCode)

	if bodyAllowed {
		_, _ = w.Write(body)
	}
}

func isLoginRequest(method string, target *url.URL) bool {
	return strings.EqualFold(method, "POST") &&
		target != nil &&
		strings.HasSuffix(strings.ToLower(target.Path), "/streaming/account/login")
}

func isProxyLoop(r *http.Request, target *url.URL) bool {
	if !strings.HasPrefix(target.Path, "/api/http-proxy") {
		return false
	}

	host := target.Hostname()
	targetPort := target.Port()

	localHost, localPort, _ := net.SplitHostPort(r.Host)
	if localHost == "" {
		localHost = r.Host
	}

	if targetPort != "" && targetPort == localPort {
		if strings.EqualFold(host, localHost) ||
			strings.EqualFold(host, "localhost") ||
			host == "127.0.0.1" || host == "::1" {
			return true
		}
	}

	// Also check forwarded headers
	extHost := resolveExternalHost(r)
	extPort := resolveExternalPort(r)

	if extHost != "" && strings.EqualFold(host, extHost) {
		tp, _ := parsePort(target.Port(), target.Scheme)
		if tp == extPort {
			return true
		}
	}

	return false
}

func resolveExternalHost(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-Host"); v != "" {
		if idx := strings.LastIndexByte(v, ':'); idx >= 0 {
			return v[:idx]
		}

		return v
	}

	if v := r.Host; v != "" {
		if idx := strings.LastIndexByte(v, ':'); idx >= 0 {
			return v[:idx]
		}

		return v
	}

	return ""
}

func resolveExternalPort(r *http.Request) int {
	if v := r.Header.Get("X-Forwarded-Port"); v != "" {
		if p, err := parsePort(v, ""); err == nil {
			return p
		}
	}

	if v := r.Header.Get("X-Forwarded-Host"); v != "" {
		if idx := strings.LastIndexByte(v, ':'); idx >= 0 {
			if p, err := parsePort(v[idx+1:], ""); err == nil {
				return p
			}
		}
	}

	if v := r.Host; v != "" {
		if idx := strings.LastIndexByte(v, ':'); idx >= 0 {
			if p, err := parsePort(v[idx+1:], ""); err == nil {
				return p
			}
		}
	}

	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return 443
	}

	return 80
}

func parsePort(portStr, scheme string) (int, error) {
	if portStr != "" {
		var p int

		_, err := fmt.Sscanf(portStr, "%d", &p)

		return p, err
	}

	switch strings.ToLower(scheme) {
	case "https":
		return 443, nil
	case "http":
		return 80, nil
	}

	return 0, fmt.Errorf("no port")
}

func applyOverrideURL(target *url.URL, cfg *Config, state *NativeState) *url.URL {
	if !cfg.IsMargeTarget(target.Hostname(), target.Path) {
		return target
	}

	override := normalizeBaseURL(state.Get("overrideMargeURL"))
	if override == "" {
		return target
	}

	result := buildURIFromBase(override, target.Path, target.RawQuery)
	if result == nil {
		return target
	}

	return result
}

func buildURIFromBase(base, path, query string) *url.URL {
	base = normalizeBaseURL(base)
	if base == "" {
		return nil
	}

	u, err := url.Parse(base)
	if err != nil {
		return nil
	}

	u.Path = path
	u.RawQuery = query

	return u
}

func margePathPrefix(path string) string {
	p := strings.ToLower(path)

	idx := strings.Index(p, "/streaming/")
	if idx <= 0 {
		return ""
	}

	return path[:idx]
}

// XML helpers for login retry

type xmlStatusCode struct {
	StatusCode string `xml:"status-code"`
}

type xmlAccountAttr struct {
	ID string `xml:"id,attr"`
}

type xmlLoginBody struct {
	Username string `xml:"username"`
	Password string `xml:"password"`
}

type xmlEnvironment struct {
	StreamingURL string `xml:"streamingURL"`
	UpdateURL    string `xml:"updateURL"`
}

type environmentInfo struct {
	streamingURL string
	updateURL    string
}

func extractXMLStatusCode(body []byte) string {
	var v xmlStatusCode
	if err := xml.Unmarshal(body, &v); err == nil && v.StatusCode != "" {
		return v.StatusCode
	}
	// Try finding in any wrapper element
	type wrapper struct {
		StatusCode string `xml:"status-code"`
	}

	var w wrapper

	_ = xml.Unmarshal(body, &w)

	return w.StatusCode
}

func parseLoginCredentials(body []byte) (email, password string) {
	var login xmlLoginBody
	if err := xml.Unmarshal(body, &login); err != nil {
		return "", ""
	}

	return strings.TrimSpace(login.Username), login.Password
}

func extractXMLAccountID(body []byte) string {
	type accountWrapper struct {
		Account xmlAccountAttr `xml:"account"`
	}

	var w accountWrapper
	if err := xml.Unmarshal(body, &w); err == nil && w.Account.ID != "" {
		return w.Account.ID
	}

	return ""
}

func extractEnvironment(body []byte) *environmentInfo {
	var env xmlEnvironment
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil
	}

	if env.StreamingURL == "" && env.UpdateURL == "" {
		return nil
	}

	return &environmentInfo{streamingURL: env.StreamingURL, updateURL: env.UpdateURL}
}
