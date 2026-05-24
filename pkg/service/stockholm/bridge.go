// Package stockholm implements the Stockholm frontend backend: native bridge,
// HTTP proxy, static file serving, SSDP discovery, and state persistence.
package stockholm

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// bridgeMessage is a single message in the runQueue response.
type bridgeMessage struct {
	Result interface{} `json:"result,omitempty"`
	Error  interface{} `json:"error,omitempty"`
	Method string      `json:"method,omitempty"`
	Params interface{} `json:"params,omitempty"`
	ID     interface{} `json:"id"`
}

// appSendRequest is the JSON body of a POST /api/native/appSend call.
type appSendRequest struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
	ID     interface{}            `json:"id"`
}

// Bridge manages per-clientId message queues for the native bridge.
type Bridge struct {
	cfg    *Config
	state  *NativeState
	queues sync.Map // clientId -> *clientQueue
}

type clientQueue struct {
	mu   sync.Mutex
	msgs []bridgeMessage
}

func newBridge(cfg *Config, state *NativeState) *Bridge {
	return &Bridge{cfg: cfg, state: state}
}

// HandleAppSend serves POST /api/native/appSend.
func (b *Bridge) HandleAppSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	clientID := resolveClientID(r)

	var req appSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.enqueueError(clientID, nil, "invalid_request")
		w.WriteHeader(http.StatusNoContent)

		return
	}

	b.dispatch(clientID, req)
	w.WriteHeader(http.StatusNoContent)
}

// HandleRunQueue serves GET /api/native/runQueue.
func (b *Bridge) HandleRunQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	clientID := resolveClientID(r)
	q := b.getOrCreateQueue(clientID)

	q.mu.Lock()
	msgs := q.msgs
	q.msgs = nil
	q.mu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")

	type runQueueResponse struct {
		Messages []bridgeMessage `json:"messages"`
	}

	if err := json.NewEncoder(w).Encode(runQueueResponse{Messages: msgs}); err != nil {
		log.Printf("[Stockholm bridge] Failed to encode runQueue response: %v", err)
	}
}

func (b *Bridge) dispatch(clientID string, req appSendRequest) {
	method := req.Method
	params := req.Params

	if params == nil {
		params = map[string]interface{}{}
	}

	id := req.ID

	log.Printf("[Stockholm bridge] method=%q client=%q", sanitizeLog(method), sanitizeLog(clientID))

	switch method {
	case "locale", "htmlReady", "stopHrmsUpdates":
		// no-op

	case "log":
		if msg, _ := params["msg"].(string); msg != "" {
			log.Printf("[Stockholm:%s] %s", sanitizeLog(clientID), sanitizeLog(msg))
		}

	case "setData":
		name, _ := params["name"].(string)
		if name != "" {
			value := stringifyScalar(params["value"])
			b.state.Set(name, value)
		}

	case "getData":
		name, _ := params["name"].(string)
		b.enqueueResult(clientID, id, b.state.Get(name), "")

	case "getLanStatus":
		b.enqueueResult(clientID, id, true, nil)

	case "getTimeZone":
		b.enqueueResult(clientID, id, map[string]interface{}{
			"timezoneInfo": localTimezoneName(),
			"timeFormat":   "TIME_FORMAT_24HOUR_ID",
		}, "")

	case "getLegalDocPath":
		b.enqueueResult(clientID, id, legalDocPath(params), nil)

	case "getConstant":
		name, _ := params["name"].(string)
		val := b.state.Get("constant." + name)

		if val == "" && name == "kilo" {
			val = kiloDefaultValue
		}

		b.enqueueResult(clientID, id, val, "")

	case "canPerformAutoAPSetup":
		b.enqueueResult(clientID, id, map[string]interface{}{
			"permission": false,
			"location":   false,
		}, "")

	case "getDeviceList":
		go b.runDeviceDiscovery(clientID, id)

	case "getHrmsList":
		go b.runServerDiscovery(clientID, id)

	case "getNetStats", "getSSIDList", "setSSID", "updateSetting", "oauth",
		"downloadNewGui", "installNewGui", "sendLogs",
		"socketCreate", "socketSend", "socketClose":
		b.enqueueError(clientID, id, "unsupported")

	default:
		b.enqueueError(clientID, id, "unsupported")
	}
}

func (b *Bridge) runDeviceDiscovery(clientID string, _ interface{}) {
	expectedAccount := b.state.Get("margeAccountID")

	// The JS "devices" handler reconciles the full list: it removes any device
	// not present in the latest message.  Sending one device at a time would
	// therefore drop the previous device on each update.  Always send the
	// cumulative list so existing entries are preserved.
	var seen []RendererDevice

	devices := DiscoverRenderers(expectedAccount, func(d RendererDevice) {
		seen = append(seen, d)
		b.enqueueMethod(clientID, "devices", seen)
	})

	if len(devices) == 0 {
		b.enqueueMethod(clientID, "devices", []RendererDevice{})
	}
}

func (b *Bridge) runServerDiscovery(clientID string, _ interface{}) {
	servers := DiscoverServers()
	b.enqueueMethod(clientID, "servers", servers)
}

func (b *Bridge) enqueueResult(clientID string, id, result, errVal interface{}) {
	b.enqueue(clientID, bridgeMessage{Result: result, Error: errVal, ID: id})
}

func (b *Bridge) enqueueError(clientID string, id interface{}, errMsg string) {
	b.enqueue(clientID, bridgeMessage{Result: nil, Error: errMsg, ID: id})
}

func (b *Bridge) enqueueMethod(clientID, method string, params interface{}) {
	b.enqueue(clientID, bridgeMessage{Method: method, Params: params, ID: nil})
}

func (b *Bridge) enqueue(clientID string, msg bridgeMessage) {
	q := b.getOrCreateQueue(clientID)

	q.mu.Lock()
	q.msgs = append(q.msgs, msg)
	q.mu.Unlock()
}

func (b *Bridge) getOrCreateQueue(clientID string) *clientQueue {
	v, _ := b.queues.LoadOrStore(clientID, &clientQueue{})
	q, _ := v.(*clientQueue)

	if q == nil {
		q = &clientQueue{}
	}

	return q
}

func resolveClientID(r *http.Request) string {
	if v := r.Header.Get("X-Stockholm-Client-Id"); v != "" {
		return v
	}

	if v := r.URL.Query().Get("clientId"); v != "" {
		decoded, err := url.QueryUnescape(v)
		if err == nil {
			return decoded
		}

		return v
	}

	return "default"
}

func legalDocPath(params map[string]interface{}) string {
	typVal, _ := params["type"].(string)
	lang, _ := params["lang"].(string)

	// "lcns" = third-party platform/GUI licences; the Stockholm zip ships this
	// as gui_licenses_en.txt (no per-language variants exist).
	if typVal == "lcns" {
		return "legal/gui_licenses_en.txt"
	}

	if typVal == "" || typVal == "eula" {
		if lang == "" {
			lang = "en"
		}

		return "legal/eula_" + lang + ".txt"
	}

	// "privacy" and any other types: the Stockholm zip does not include these
	// files, so fall back to the English EULA.
	return "legal/eula_en.txt"
}

func localTimezoneName() string {
	return time.Local.String()
}
