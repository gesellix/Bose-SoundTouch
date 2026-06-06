package health

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CheckIDServerURLReachable is the registry id of the server-URL
// self-reachability check.
const CheckIDServerURLReachable = "server_url_reachable"

// RegisterServerURLReachableCheck registers the server_url_reachable
// check. It probes GET {serverURL}/api/setup/version from inside the
// service. If the request fails or returns a non-200 status, the
// configured server URL doesn't route back to AfterTouch — speakers
// pointing their margeURL at that address will receive errors instead
// of marge responses.
//
// The most common cause is a missing port number in the server URL:
// e.g. "http://192.0.2.1" implies port 80, but if another process
// (such as the Bose firmware's PtsServer) already occupies port 80,
// AfterTouch runs on its default port (8000) instead — and all marge
// calls from speakers silently hit the wrong process.
//
// getServerURL is a closure so the check picks up config changes
// without re-registration.
func RegisterServerURLReachableCheck(r *Registry, getServerURL func() string) {
	r.Register(Check{
		ID:    CheckIDServerURLReachable,
		Title: "Service is reachable at configured server URL",
		Run: func() []Finding {
			return runServerURLReachableCheck(getServerURL())
		},
	})
}

func runServerURLReachableCheck(serverURL string) []Finding {
	if strings.TrimSpace(serverURL) == "" {
		return nil
	}

	probeURL := strings.TrimRight(serverURL, "/") + "/api/setup/version"
	res := ProbeGet(context.Background(), probeURL, 2*time.Second)

	if res.Reachable && res.Status == 200 {
		return nil
	}

	errDetail := res.Err
	if errDetail == "" && res.Status != 0 {
		errDetail = fmt.Sprintf("HTTP %d", res.Status)
	}

	details := fmt.Sprintf(
		"Probe: GET %s → %s. "+
			"The most common cause is a missing port in the server URL. "+
			"Example: \"http://192.0.2.1\" implies port 80; if another process "+
			"(e.g. the Bose firmware's PtsServer) occupies port 80, AfterTouch runs on its "+
			"default port 8000 instead. Fix: add the explicit port, e.g. \"http://192.0.2.1:8000\", "+
			"then restart the service and re-apply the migration for each speaker to push the "+
			"updated margeURL.",
		probeURL, errDetail,
	)

	return []Finding{{
		Severity: SeverityWarning,
		Message: fmt.Sprintf(
			"Configured server URL %q is not reachable from inside the service (probe: %s). "+
				"Marge calls from speakers will fail — check that the URL and port match "+
				"the service's actual listening port.",
			serverURL, errDetail,
		),
		Details: details,
		ManualCommands: []ManualCommand{
			{
				Label:   "Check what port AfterTouch is listening on:",
				Command: "ss -tlnp | grep soundtouch",
				Hint:    "Compare the listening port with the port implied by the configured server URL.",
			},
			{
				Label:   "Or set the correct URL via the web UI:",
				Command: "Settings tab → Server URL (Target Domain) → add explicit port (e.g. :8000) → Save → restart",
			},
		},
	}}
}
