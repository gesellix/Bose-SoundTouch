package health

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDRefreshSources is the registry id of the per-device
// "force a sources refresh" affordance.
const CheckIDRefreshSources = "refresh_sources"

// FixIDPostSourcesUpdated is the quick-fix that POSTs a
// sourcesUpdated notification to the speaker's :8090/notification
// endpoint.
const FixIDPostSourcesUpdated = "post_sources_updated"

// RegisterRefreshSourcesCheck registers a per-device affordance
// that POSTs <updates><sourcesUpdated/></updates> to the
// speaker's /notification endpoint without waiting for the
// sources_xml_diff check to find drift first. Useful after any
// service-side Sources.xml change (manual edit, fixed via the
// sources_xml_present quick fix, etc.) to push the new state
// onto the speaker without a reboot.
func RegisterRefreshSourcesCheck(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDRefreshSources,
		Title: "Refresh sources on each speaker",
		Run: func() []Finding {
			return runRefreshSourcesCheck(ds)
		},
	})

	r.RegisterFix(CheckIDRefreshSources, FixIDPostSourcesUpdated, func(target Target) (string, error) {
		return postSourcesUpdated(ds, target)
	})
}

func runRefreshSourcesCheck(ds *datastore.DataStore) []Finding {
	if ds == nil {
		return nil
	}

	devices, err := ds.ListAllDevices()
	if err != nil {
		return []Finding{{
			Severity: SeverityError,
			Message:  "Could not enumerate devices: " + err.Error(),
		}}
	}

	out := make([]Finding, 0, len(devices))

	for i := range devices {
		dev := &devices[i]
		if dev.IPAddress == "" || dev.DeviceID == "" {
			continue
		}

		out = append(out, Finding{
			Severity: SeverityInfo,
			Target:   Target{Account: dev.AccountID, Device: dev.DeviceID},
			Message:  fmt.Sprintf("Force a sources refresh on %s.", displayName(dev.Name, dev.DeviceID)),
			Details:  "POSTs <updates><sourcesUpdated/></updates> to the speaker so it re-fetches /streaming/account/<id>/full. Cheaper than power-cycling when the service-side Sources.xml has been updated.",
			QuickFixes: []QuickFix{{
				ID:    FixIDPostSourcesUpdated,
				Label: "Refresh sources",
			}},
			ManualCommands: []ManualCommand{{
				Label:   "Or trigger from the LAN:",
				Command: sourcesUpdatedCurlCommand(dev.IPAddress, dev.DeviceID),
				Hint:    "Run on a host that can reach the speaker on port 8090. A reboot is sometimes still required for new source *types* (vs. updated metadata for existing types) to take effect.",
			}},
		})
	}

	return out
}

func postSourcesUpdated(ds *datastore.DataStore, target Target) (string, error) {
	if target.Device == "" {
		return "", fmt.Errorf("device is required")
	}

	dev, err := ds.GetDeviceInfo(target.Account, target.Device)
	if err != nil || dev == nil {
		return "", fmt.Errorf("device %s not found in datastore", target.Device)
	}

	if dev.IPAddress == "" {
		return "", fmt.Errorf("device %s has no IP address recorded", target.Device)
	}

	body := sourcesUpdatedXML(target.Device)
	notifyURL := fmt.Sprintf("http://%s:8090/notification", dev.IPAddress)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, notifyURL, bytes.NewReader([]byte(body)))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("post to speaker: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return "", fmt.Errorf("speaker returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return fmt.Sprintf("Sent sourcesUpdated to %s.", displayName(dev.Name, target.Device)), nil
}

func sourcesUpdatedXML(deviceID string) string {
	return fmt.Sprintf(`<updates deviceID="%s"><sourcesUpdated/></updates>`, xmlAttrEscape(deviceID))
}

func sourcesUpdatedCurlCommand(speakerIP, deviceID string) string {
	body := sourcesUpdatedXML(deviceID)

	return fmt.Sprintf(
		"curl -sS -X POST 'http://%s:8090/notification' -H 'Content-Type: application/xml' -d '%s'",
		speakerIP, strings.ReplaceAll(body, "'", `'\''`),
	)
}
