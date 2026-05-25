package stockholm

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// contentTypeFor returns the MIME type for a file based on its extension.
func contentTypeFor(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".html"):
		return "text/html; charset=UTF-8"
	case strings.HasSuffix(n, ".js"):
		return "application/javascript; charset=UTF-8"
	case strings.HasSuffix(n, ".css"):
		return "text/css; charset=UTF-8"
	case strings.HasSuffix(n, ".json"):
		return "application/json; charset=UTF-8"
	case strings.HasSuffix(n, ".xml"):
		return "application/xml; charset=UTF-8"
	case strings.HasSuffix(n, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(n, ".png"):
		return "image/png"
	case strings.HasSuffix(n, ".jpg"), strings.HasSuffix(n, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(n, ".gif"):
		return "image/gif"
	case strings.HasSuffix(n, ".ttf"):
		return "font/ttf"
	case strings.HasSuffix(n, ".otf"):
		return "font/otf"
	case strings.HasSuffix(n, ".txt"):
		return "text/plain; charset=UTF-8"
	default:
		return "application/octet-stream"
	}
}

// ServeStatic handles all static file requests for the Stockholm frontend.
// All file operations are performed via an os.Root anchored at stockholmDir,
// which prevents path traversal at the OS level (go/path-injection, alerts
// 143–145). The URL-path → relative-path mapping is handled by
// resolveStaticRel; os.Root rejects any path that would escape stockholmDir.
func ServeStatic(w http.ResponseWriter, r *http.Request, stockholmDir string, backendCfg *BackendConfig, state *NativeState, cfg *Config) {
	method := strings.ToUpper(r.Method)
	if method != http.MethodGet && method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	root, err := os.OpenRoot(stockholmDir)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = root.Close() }()

	rel := resolveStaticRel(r.URL.Path)

	info, err := root.Stat(rel)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Directory → serve index.html inside it.
	if info.IsDir() {
		rel += "/index.html"

		info, err = root.Stat(rel)
		if err != nil || info.IsDir() {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
	}

	f, err := root.Open(rel)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	body, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ct := contentTypeFor(rel)

	if ct == "text/html; charset=UTF-8" && isBootstrapTarget(rel) {
		body = injectBootstrap(body, state, cfg)
	}

	// Frontend logging cookie
	if backendCfg.ShouldEnableFrontendDebug() {
		w.Header().Add("Set-Cookie", fmt.Sprintf("stockholmFrontendLoggingLevel=%d; Path=/; SameSite=Lax", backendCfg.FrontendLoggingLevel))
	} else {
		w.Header().Add("Set-Cookie", "stockholmFrontendLoggingLevel=; Max-Age=0; Path=/; SameSite=Lax")
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")

	if method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// resolveStaticRel converts a URL path to a relative path for use with an
// os.Root anchored at the Stockholm static-files directory. It handles the
// root-path → index.html default and strips the leading slash; it does not
// validate for traversal because os.Root enforces containment at the OS level.
func resolveStaticRel(rawPath string) string {
	if rawPath == "" || rawPath == "/" {
		return "index.html"
	}

	rel := strings.TrimPrefix(rawPath, "/")
	if rel == "" {
		return "index.html"
	}

	return rel
}

func isBootstrapTarget(relPath string) bool {
	p := strings.ToLower(relPath)
	return p == "index.html" || p == "setup/index.html"
}

func injectBootstrap(html []byte, state *NativeState, cfg *Config) []byte {
	content := string(html)

	if strings.Contains(content, "window.StockholmBrowserBootstrap") {
		return html
	}

	idx := strings.Index(content, "</head>")
	if idx < 0 {
		return html
	}

	script := buildBootstrapScript(state, cfg)
	injected := content[:idx] + script + content[idx:]

	return []byte(injected)
}

func buildBootstrapScript(state *NativeState, cfg *Config) string {
	guid := firstNonEmpty(state.Get("guid"), state.Get("deviceGuid"))
	nativeVersion := firstNonEmpty(state.Get("frame_version"), cfg.AppVersion)
	authServer := state.AuthServer()

	payload := map[string]interface{}{
		"authServer":    authServer,
		"guid":          guid,
		"nativeVersion": nativeVersion,
		"frameConfig":   map[string]interface{}{},
		"basePath":      cfg.BasePath,
	}

	bootstrapJSON, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Stockholm static] Failed to marshal bootstrap payload: %v", err)

		bootstrapJSON = []byte("{}")
	}

	return fmt.Sprintf(`<script>
(function () {
    window.StockholmBrowserBootstrap = %s;
    // __stockholmBase lets the bridge JS files resolve API URLs when Stockholm
    // is mounted under a prefix such as /stockholm.
    window.__stockholmBase = window.StockholmBrowserBootstrap.basePath || "";
    var bootstrap = window.StockholmBrowserBootstrap || {};

    function toBase64(value) {
        return window.btoa(unescape(encodeURIComponent(String(value))));
    }

    function mergeFrameConfig(config) {
        if (!bootstrap.frameConfig || typeof bootstrap.frameConfig !== "object") {
            return config;
        }
        config = config || {};
        config.default = config.default || {};
        Object.keys(bootstrap.frameConfig).forEach(function (key) {
            var value = bootstrap.frameConfig[key];
            if (!/^f\d+$/.test(key) || value === undefined || value === null) {
                return;
            }
            var targetKey = "d" + key.substring(1);
            if (config.default[targetKey] === undefined || config.default[targetKey] === null
                    || config.default[targetKey] === "") {
                config.default[targetKey] = toBase64(value);
            }
        });
        return config;
    }

    var originalGetURLParams = window.getURLParams;
    if (typeof originalGetURLParams === "function") {
        window.getURLParams = function (name, url) {
            var value = originalGetURLParams(name, url);
            if (value !== null && value !== undefined) {
                return value;
            }
            if (name === "native_version" && bootstrap.nativeVersion) {
                return bootstrap.nativeVersion;
            }
            if (name === "authServer" && bootstrap.authServer !== undefined && bootstrap.authServer !== null) {
                return String(bootstrap.authServer);
            }
            if (name === "guid" && bootstrap.guid) {
                return bootstrap.guid;
            }
            return value;
        };
    }

    var originalGetUserAgentValue = window.getUserAgentValue;
    if (typeof originalGetUserAgentValue === "function") {
        window.getUserAgentValue = function (name) {
            var value = originalGetUserAgentValue(name);
            if ((!value || value === "") && name === "_app" && bootstrap.guid) {
                return bootstrap.guid;
            }
            return value;
        };
    }

    if ((!window.guid || window.guid === "") && bootstrap.guid) {
        window.guid = bootstrap.guid;
    }
    if ((!window.frame_version || window.frame_version === "") && bootstrap.nativeVersion) {
        window.frame_version = bootstrap.nativeVersion;
    }
    if ((window.auth_server === undefined || window.auth_server === null || window.auth_server === "")
            && bootstrap.authServer !== undefined && bootstrap.authServer !== null) {
        window.auth_server = bootstrap.authServer;
    }

    var originalSettingsLoad = window.settingsLoad;
    if (typeof originalSettingsLoad === "function") {
        window.settingsLoad = function (config) {
            return originalSettingsLoad(mergeFrameConfig(config));
        };
    }
})();
</script>
`, string(bootstrapJSON))
}
