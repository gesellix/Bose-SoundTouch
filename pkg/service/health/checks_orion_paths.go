package health

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDOrionPaths is the registry id of the check for Internet Radio
// (LOCAL_INTERNET_RADIO) presets that store an absolute Orion station
// location instead of the relative one. The id predates issue 769, when the
// check only looked for the Bose cloud host; it is kept stable.
const CheckIDOrionPaths = "orion_paths_in_presets"

// FixIDRewriteOrionRelative is the quick fix that stores a device's absolute
// Orion station presets again with the relative location.
const FixIDRewriteOrionRelative = "rewrite_orion_relative"

// boseOrionHost is the shut-down Bose cloud host that presets saved before May
// 2026 carry in their Orion station locations.
const boseOrionHost = "content.api.bose.io"

// orionPathsTroubleshootingURL explains the finding and the fix.
const orionPathsTroubleshootingURL = "https://gesellix.github.io/Bose-SoundTouch/docs/guides/TROUBLESHOOTING/#play-url-presets-after-move"

// OrionPathsDeps are the runtime inputs of the Orion location check. Every
// field is optional; a nil function counts as "unknown".
type OrionPathsDeps struct {
	// ServiceURLs returns this service's configured SERVER_URL and HTTPS URL.
	// Their ports decide whether an absolute location still points here.
	ServiceURLs func() (serverURL, httpsURL string)
	// ExpectedHosts returns the host names this service answers to
	// (SERVER_URL host, HTTPS host, TLS extra hosts), as the speaker_marge_url
	// check uses them.
	ExpectedHosts func() []string
	// DNSEnabled reports whether "Enable DNS Discovery Server" is on.
	DNSEnabled DNSEnabledFunc
	// DNSClientIPs returns the speaker IPs that have resolved an intercepted
	// Bose hostname through AfterTouch's DNS server (the dns_speaker_usage
	// signal).
	DNSClientIPs func() map[string]time.Time
	// NewSpeakerClient returns a client for the speaker at ip. Tests replace
	// it; the default talks to ip:8090 with a short timeout.
	NewSpeakerClient func(ip string) *client.Client
}

// RegisterOrionPathsCheck registers a scan over every device's service-side
// Presets.xml for Internet Radio presets whose Orion station location is
// absolute ("<host>/core02/svc-bmx-adapter-orion/prod/orion/station?data=...")
// instead of relative ("/station?data=...").
//
// Since issue 769 AfterTouch stores the relative form: the speaker resolves
// it against the Orion baseUrl from its BMX registry, so it follows
// AfterTouch to a new address. Absolute locations are classified by host:
//
//   - another host than this service (typically AfterTouch's old address):
//     warning, it stops playing once that host is gone (discussion 708);
//   - content.api.bose.io (saved before the Bose shutdown): it plays only
//     while the speaker resolves that name through AfterTouch's DNS. Info
//     when the speaker has been seen doing so, info marked "not confirmed"
//     while DNS Discovery is on without that evidence, warning when DNS
//     Discovery is off (then it reaches the dead Bose cloud);
//   - this service's current address: info, it plays today but breaks when
//     AfterTouch's address changes.
//
// Each finding offers a quick fix that stores the affected slots again on the
// speaker with the relative location. Other sources and relative locations
// are never touched.
func RegisterOrionPathsCheck(r *Registry, ds *datastore.DataStore, deps OrionPathsDeps) {
	r.Register(Check{
		ID:    CheckIDOrionPaths,
		Title: "Internet Radio presets use relative station locations",
		Run: func() []Finding {
			return runOrionPathsCheck(ds, deps)
		},
	})

	r.RegisterFix(CheckIDOrionPaths, FixIDRewriteOrionRelative, func(target Target) (string, error) {
		return fixRewriteOrionRelative(ds, deps, target)
	})
}

// orionKind classifies an absolute Orion station location by its host.
type orionKind int

const (
	orionRelative orionKind = iota
	orionCurrentHost
	orionOtherHost
	orionBoseCloud
)

// orionHit is one preset slot with an absolute Orion station location.
type orionHit struct {
	slot     string
	label    string
	host     string
	kind     orionKind
	relative string
	preset   models.ServicePreset
}

func runOrionPathsCheck(ds *datastore.DataStore, deps OrionPathsDeps) []Finding {
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

	service := currentServiceAddress(deps)

	dnsEnabled := deps.DNSEnabled != nil && deps.DNSEnabled()

	queriers := map[string]time.Time{}
	if deps.DNSClientIPs != nil {
		queriers = deps.DNSClientIPs()
	}

	var findings []Finding

	for i := range devices {
		dev := &devices[i]
		if dev.AccountID == "" || dev.DeviceID == "" {
			continue
		}

		presets, err := ds.GetPresets(dev.AccountID, dev.DeviceID)
		if err != nil {
			continue
		}

		hits := findOrionHits(presets, service)
		if len(hits) == 0 {
			continue
		}

		_, observed := queriers[dev.IPAddress]
		dnsConfirmed := dev.IPAddress != "" && observed

		findings = append(findings, orionFinding(dev, hits, service, dnsEnabled, dnsConfirmed))
	}

	return findings
}

// serviceAddress is what "this service" means for a location's host: one of
// the names it answers to, on the port of its SERVER_URL or HTTPS URL.
type serviceAddress struct {
	hosts   map[string]bool
	ports   map[string]bool
	display string
}

func currentServiceAddress(deps OrionPathsDeps) serviceAddress {
	addr := serviceAddress{hosts: map[string]bool{}, ports: map[string]bool{}}

	if deps.ExpectedHosts != nil {
		addr.hosts = normaliseHosts(deps.ExpectedHosts())
	}

	if deps.ServiceURLs == nil {
		return addr
	}

	serverURL, httpsURL := deps.ServiceURLs()
	addr.display = strings.TrimRight(serverURL, "/")

	for _, raw := range []string{serverURL, httpsURL} {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Host == "" {
			continue
		}

		addr.hosts[strings.ToLower(u.Hostname())] = true
		addr.ports[effectivePort(u)] = true
	}

	return addr
}

func (a serviceAddress) matches(u *url.URL) bool {
	return a.hosts[strings.ToLower(u.Hostname())] && a.ports[effectivePort(u)]
}

// effectivePort returns u's port, filling in the scheme default.
func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}

	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}

	return "80"
}

// classifyOrionLocation reports the kind of an Orion station location, its
// host (for absolute ones) and its relative form. ok is false for anything
// that is not an Orion station location.
func classifyOrionLocation(location string, service serviceAddress) (kind orionKind, host, relative string, ok bool) {
	relative, ok = models.RelativeOrionLocation(location)
	if !ok {
		return orionRelative, "", "", false
	}

	u, err := url.Parse(strings.TrimSpace(location))
	if err != nil || u.Host == "" {
		return orionRelative, "", relative, true
	}

	host = u.Host

	switch {
	case strings.EqualFold(u.Hostname(), boseOrionHost):
		return orionBoseCloud, host, relative, true
	case service.matches(u):
		return orionCurrentHost, host, relative, true
	default:
		return orionOtherHost, host, relative, true
	}
}

// findOrionHits returns the Internet Radio presets whose Orion station
// location is absolute, in input order. Relative locations, raw stream URLs
// and every other source are left out.
func findOrionHits(presets []models.ServicePreset, service serviceAddress) []orionHit {
	var hits []orionHit

	for i := range presets {
		p := presets[i]
		if !strings.EqualFold(strings.TrimSpace(p.Source), models.OrionLocalInternetRadioSource) {
			continue
		}

		kind, host, relative, ok := classifyOrionLocation(p.Location, service)
		if !ok || kind == orionRelative {
			continue
		}

		slot := presetSlot(p)

		label := slot
		if label == "" {
			label = p.Name
		}

		if label == "" {
			label = "(unnamed)"
		}

		hits = append(hits, orionHit{slot: slot, label: label, host: host, kind: kind, relative: relative, preset: p})
	}

	return hits
}

func presetSlot(p models.ServicePreset) string {
	if button := strings.TrimSpace(p.ButtonNumber); button != "" {
		return button
	}

	return strings.TrimSpace(p.ID)
}

func orionFinding(dev *models.ServiceDeviceInfo, hits []orionHit, service serviceAddress, dnsEnabled, dnsConfirmed bool) Finding {
	severity := SeverityInfo

	var parts, details []string

	byKind := map[orionKind][]string{}
	hostsByKind := map[orionKind]map[string]bool{}

	for i := range hits {
		h := &hits[i]
		byKind[h.kind] = append(byKind[h.kind], "slot "+h.label)

		if hostsByKind[h.kind] == nil {
			hostsByKind[h.kind] = map[string]bool{}
		}

		hostsByKind[h.kind][h.host] = true
	}

	if slots := byKind[orionOtherHost]; len(slots) > 0 {
		severity = SeverityWarning

		current := "this service's address"
		if service.display != "" {
			current += " (" + service.display + ")"
		}

		parts = append(parts, fmt.Sprintf("%s point at %s, not at %s", strings.Join(slots, ", "), sortedKeys(hostsByKind[orionOtherHost]), current))
		details = append(details, "A location with another host stops playing once that host is gone, for example after AfterTouch moved to a new address (discussion 708). If it points at another Orion-compatible service on purpose, leave it as it is.")
	}

	if slots := byKind[orionBoseCloud]; len(slots) > 0 {
		switch {
		case !dnsEnabled:
			severity = SeverityWarning

			parts = append(parts, fmt.Sprintf("%s point at the shut-down Bose cloud (%s)", strings.Join(slots, ", "), boseOrionHost))
			details = append(details, "DNS Discovery is off, so the speaker resolves "+boseOrionHost+" through its own resolver and reaches the dead Bose cloud: these presets don't play.")
		case dnsConfirmed:
			parts = append(parts, fmt.Sprintf("%s point at %s, which this speaker resolves through AfterTouch", strings.Join(slots, ", "), boseOrionHost))
			details = append(details, "They play today because the speaker asks AfterTouch's DNS for "+boseOrionHost+". They stop playing if the speaker ever resolves Bose hostnames elsewhere, for example after its DNS migration is undone.")
		default:
			parts = append(parts, fmt.Sprintf("%s point at %s; not yet confirmed that this speaker resolves it through AfterTouch", strings.Join(slots, ", "), boseOrionHost))
			details = append(details, "They play only while the speaker resolves "+boseOrionHost+" through AfterTouch's DNS. The dns_bypass_risk check (\"Test DNS path\") tells whether it does.")
		}
	}

	if slots := byKind[orionCurrentHost]; len(slots) > 0 {
		parts = append(parts, fmt.Sprintf("%s point at this service's current address", strings.Join(slots, ", ")))
		details = append(details, "They play today, but stop when AfterTouch's address changes.")
	}

	details = append(details,
		"Since issue 769 AfterTouch stores Internet Radio stations with the relative location /station?data=..., which the speaker resolves against the Orion address in its BMX registry, so the preset follows AfterTouch wherever it moves. "+
			"The quick fix stores these slots on the speaker again with the relative location (same name, artwork and slot); the speaker then reports them back to AfterTouch. "+
			"It needs the speaker to be reachable. See "+orionPathsTroubleshootingURL+".")

	name := displayName(dev.Name, dev.DeviceID)

	return Finding{
		Severity: severity,
		Target:   Target{Account: dev.AccountID, Device: dev.DeviceID},
		Message: fmt.Sprintf("%d Internet Radio preset(s) on %s store an absolute station location: %s.",
			len(hits), name, strings.Join(parts, "; ")),
		Details: strings.Join(details, " "),
		QuickFixes: []QuickFix{{
			ID:    FixIDRewriteOrionRelative,
			Label: "Store as relative locations",
			Confirm: fmt.Sprintf("Stores %d preset slot(s) on %s again with the relative station location. Names, artwork and slots stay the same; any Internet Radio slot with an absolute station location is rewritten.",
				len(hits), name),
		}},
		ManualCommands: orionManualCommands(dev.IPAddress, hits),
	}
}

func sortedKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return strings.Join(keys, ", ")
}

// orionManualCommands renders one /storePreset call per affected slot, for
// when AfterTouch cannot reach the speaker itself.
func orionManualCommands(ip string, hits []orionHit) []ManualCommand {
	speaker := ip
	if speaker == "" {
		speaker = "<speaker-ip>"
	}

	var out []ManualCommand

	for i := range hits {
		h := &hits[i]
		if _, err := strconv.Atoi(h.slot); err != nil {
			continue
		}

		body := orionStorePresetXML(h.slot, h.preset, h.relative)

		out = append(out, ManualCommand{
			Label: "Store slot " + h.slot + " with the relative location:",
			Command: fmt.Sprintf("curl -s -X POST http://%s/storePreset -H 'Content-Type: application/xml' -d %s",
				net.JoinHostPort(speaker, "8090"), shellQuote(body)),
			Hint: "Run it on a machine that reaches the speaker on port 8090. The speaker reports the new preset back to AfterTouch.",
		})
	}

	return out
}

func orionStorePresetXML(slot string, p models.ServicePreset, relative string) string {
	itemType := p.Type
	if itemType == "" {
		itemType = p.ContentItemType
	}

	if itemType == "" {
		itemType = "stationurl"
	}

	var b strings.Builder

	fmt.Fprintf(&b, `<preset id="%s"><ContentItem source="%s" type="%s" location="%s" sourceAccount="%s" isPresetable="true">`,
		xmlEscape(slot), xmlEscape(models.OrionLocalInternetRadioSource), xmlEscape(itemType), xmlEscape(relative), xmlEscape(p.SourceAccount))
	fmt.Fprintf(&b, "<itemName>%s</itemName>", xmlEscape(p.Name))

	if p.ContainerArt != "" {
		fmt.Fprintf(&b, "<containerArt>%s</containerArt>", xmlEscape(p.ContainerArt))
	}

	b.WriteString("</ContentItem></preset>")

	return b.String()
}

func xmlEscape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func defaultOrionSpeakerClient(ip string) *client.Client {
	cfg := client.DefaultConfig()
	cfg.Host = ip
	cfg.Timeout = 5 * time.Second

	return client.NewClient(cfg)
}

// fixRewriteOrionRelative stores every Internet Radio preset of the target
// device whose Orion station location is absolute again, with the relative
// location. It reads the slots from the speaker itself (the authoritative
// copy, with name, artwork, type and account) and writes each through the
// speaker's /storePreset, so the speaker and AfterTouch stay consistent: the
// speaker reports the new preset back to AfterTouch, and the datastore row is
// updated here as well so the next health run already sees it.
//
// Nothing is written when the speaker can't be reached.
func fixRewriteOrionRelative(ds *datastore.DataStore, deps OrionPathsDeps, target Target) (string, error) {
	if ds == nil || target.Account == "" || target.Device == "" {
		return "", fmt.Errorf("account and device are required")
	}

	ip, name, err := orionFixDevice(ds, target)
	if err != nil {
		return "", err
	}

	newClient := deps.NewSpeakerClient
	if newClient == nil {
		newClient = defaultOrionSpeakerClient
	}

	c := newClient(ip)

	onSpeaker, err := c.GetPresets()
	if err != nil {
		return "", fmt.Errorf("could not read presets from %s (%s), nothing was changed. Retry when it is online, or use the manual commands: %w", name, ip, err)
	}

	nowRelative, rewritten, failed := rewriteOrionOnSpeaker(c, onSpeaker)

	if err := syncOrionDatastore(ds, target, nowRelative); err != nil {
		failed = append(failed, fmt.Sprintf("AfterTouch's Presets.xml (%v); the speaker reports the change on its own", err))
	}

	if len(failed) > 0 {
		return "", fmt.Errorf("stored slot(s) %s on %s with the relative location, but these failed: %s",
			orNone(rewritten), name, strings.Join(failed, "; "))
	}

	if len(rewritten) == 0 {
		return fmt.Sprintf("The Internet Radio presets on %s already use relative locations; AfterTouch's copy was brought in line.", name), nil
	}

	return fmt.Sprintf("Stored slot(s) %s on %s again with the relative station location. They now follow AfterTouch if its address changes.",
		strings.Join(rewritten, ", "), name), nil
}

// orionFixDevice returns the IP and display name of the target device, or an
// error when AfterTouch has no address to reach it at.
func orionFixDevice(ds *datastore.DataStore, target Target) (ip, name string, err error) {
	name = target.Device

	devices, err := ds.ListAllDevices()
	if err != nil {
		return "", "", fmt.Errorf("could not enumerate devices: %w", err)
	}

	for i := range devices {
		if devices[i].AccountID == target.Account && devices[i].DeviceID == target.Device {
			ip = devices[i].IPAddress
			name = displayName(devices[i].Name, devices[i].DeviceID)

			break
		}
	}

	if ip == "" {
		return "", name, fmt.Errorf("no IP address known for %s, so AfterTouch can't reach it; nothing was changed. Use the manual commands, or rediscover the speaker and retry", name)
	}

	return ip, name, nil
}

// rewriteOrionOnSpeaker stores every Internet Radio slot with an absolute
// Orion station location again with the relative one. nowRelative maps each
// slot that holds a relative Orion location afterwards (rewritten or already
// relative) to that location.
func rewriteOrionOnSpeaker(c *client.Client, onSpeaker *models.Presets) (nowRelative map[string]string, rewritten, failed []string) {
	nowRelative = map[string]string{}

	for i := range onSpeaker.Preset {
		p := onSpeaker.Preset[i]
		if p.ContentItem == nil || !strings.EqualFold(p.ContentItem.Source, models.OrionLocalInternetRadioSource) {
			continue
		}

		relative, ok := models.RelativeOrionLocation(p.ContentItem.Location)
		if !ok {
			continue
		}

		slot := strconv.Itoa(p.ID)

		if relative == strings.TrimSpace(p.ContentItem.Location) {
			// Already relative on the speaker; the datastore may still lag.
			nowRelative[slot] = relative

			continue
		}

		item := *p.ContentItem
		item.Location = relative

		if err := c.StorePreset(p.ID, &item); err != nil {
			failed = append(failed, fmt.Sprintf("slot %s (%v)", slot, err))

			continue
		}

		nowRelative[slot] = relative
		rewritten = append(rewritten, slot)
	}

	return nowRelative, rewritten, failed
}

// syncOrionDatastore brings AfterTouch's copy of the slots in nowRelative in
// line with the speaker, without waiting for the speaker's own report. Only a
// row for the same station (same canonical location) is changed.
func syncOrionDatastore(ds *datastore.DataStore, target Target, nowRelative map[string]string) error {
	if len(nowRelative) == 0 {
		return nil
	}

	_, err := ds.MutatePresets(target.Account, target.Device, func(current []models.ServicePreset) ([]models.ServicePreset, error) {
		for i := range current {
			relative, ok := nowRelative[presetSlot(current[i])]
			if !ok || !strings.EqualFold(current[i].Source, models.OrionLocalInternetRadioSource) {
				continue
			}

			if models.CanonicalContentLocation(current[i].Source, current[i].Location) == relative {
				current[i].Location = relative
			}
		}

		return current, nil
	})

	return err
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "(none)"
	}

	return strings.Join(s, ", ")
}
