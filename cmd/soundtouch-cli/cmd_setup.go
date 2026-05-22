package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

// setupCommand assembles the `soundtouch-cli setup …` command group. Each
// subcommand wraps an existing pkg/service/setup helper — there is no new
// business logic in this file, only flag parsing and progress reporting.
//
// The group covers the manual provisioning loop documented in
// docs/analysis/SETUP-WEBSOCKET-EXPERIMENT.md and docs/guides/DEVICE-INITIAL-SETUP.md:
//
//	factory-reset → (manual: connect host to speaker's AP)
//	wifi-push     → (manual: switch host back to home Wi-Fi)
//	wait-online   → discover the speaker's new IP
//	urls          → point the speaker at AfterTouch
//	pair          → drive the WebSocket SETUP state machine
//
// The two "manual" lines are user-side Wi-Fi switches that cannot be
// automated portably (macOS, Linux, and Windows each use a different
// command). The wait-ap and wait-online subcommands poll for those
// switches so the user doesn't need to time them manually.
func setupCommand() *cli.Command {
	return &cli.Command{
		Name:  "setup",
		Usage: "Provision a SoundTouch speaker end-to-end (factory reset, Wi-Fi, URLs, pairing)",
		Subcommands: []*cli.Command{
			setupInspectCmd(),
			setupFactoryResetCmd(),
			setupWiFiPushCmd(),
			setupWaitAPCmd(),
			setupWaitOnlineCmd(),
			setupSSHCheckCmd(),
			setupInstallCACmd(),
			setupMigrateCmd(),
			setupRebootCmd(),
			setupVerifyCmd(),
			setupPlanCmd(),
			setupPairCmd(),
		},
	}
}

func setupInspectCmd() *cli.Command {
	return &cli.Command{
		Name:   "inspect",
		Usage:  "Print a non-destructive snapshot of the speaker (identity, pairing, Wi-Fi, sources, presets)",
		Before: RequireHost,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "telnet", Usage: "Also read runtime URLs via telnet getpdo (slower)"},
		},
		Action: func(c *cli.Context) error {
			cfg := GetClientConfig(c)
			m := setup.NewManager("", nil, nil)

			report := m.Inspect(cfg.Host, setup.InspectOptions{IncludeTelnet: c.Bool("telnet")})
			renderInspectReport(report)

			if report.InfoErr != nil {
				return report.InfoErr
			}

			return nil
		},
	}
}

func renderInspectReport(r *setup.InspectReport) {
	fmt.Printf("Speaker @ %s\n", r.DeviceIP)
	fmt.Println(strings.Repeat("─", 40))

	renderInspectIdentityAndPairing(r)
	renderInspectNetwork(r)
	renderInspectSources(r)
	renderInspectPresets(r)
	renderInspectRuntimeURLs(r)
}

func renderInspectIdentityAndPairing(r *setup.InspectReport) {
	if r.InfoErr != nil {
		PrintError(fmt.Sprintf("/info: %v", r.InfoErr))
		return
	}

	if r.Info == nil {
		return
	}

	i := r.Info

	fmt.Println("Identity")
	fmt.Printf("  deviceID         : %s\n", i.DeviceID)

	if suffix := deviceIDSuffix(i.DeviceID); suffix != "" {
		fmt.Printf("  → use as --match suffix for wait-online: %s\n", suffix)
	}

	fmt.Printf("  name             : %s\n", i.Name)
	fmt.Printf("  type             : %s\n", i.Type)

	for _, comp := range i.Components {
		if comp.SoftwareVersion != "" {
			fmt.Printf("  softwareVersion  : %s (component %s)\n", comp.SoftwareVersion, comp.Category)
		}

		if comp.SerialNumber != "" {
			fmt.Printf("  serialNumber     : %s (component %s)\n", comp.SerialNumber, comp.Category)
		}
	}

	fmt.Println()
	fmt.Println("Pairing")

	if i.MargeAccountUUID == "" {
		PrintWarning("margeAccountUUID is empty — device is unpaired (factory-reset state)")
	} else {
		fmt.Printf("  margeAccountUUID : %s\n", i.MargeAccountUUID)
	}

	fmt.Printf("  margeURL         : %s\n", i.MargeURL)
	fmt.Println()
}

func renderInspectNetwork(r *setup.InspectReport) {
	if r.NetworkErr != nil {
		PrintError(fmt.Sprintf("/networkInfo: %v", r.NetworkErr))
		return
	}

	if r.Network == nil {
		return
	}

	fmt.Println("Network")

	for i := range r.Network.Interfaces.Interfaces {
		iface := &r.Network.Interfaces.Interfaces[i]

		fmt.Printf("  %s\n", iface.Type)
		fmt.Printf("    state          : %s\n", iface.State)

		if iface.IPAddress != "" {
			fmt.Printf("    ipAddress      : %s\n", iface.IPAddress)
		}

		if iface.MacAddress != "" {
			fmt.Printf("    macAddress     : %s\n", iface.MacAddress)
		}

		if iface.SSID != "" {
			fmt.Printf("    ssid           : %s\n", iface.SSID)
			fmt.Printf("    → use as --ssid for wifi-push: %s\n", iface.SSID)
		}

		if iface.Signal != "" {
			fmt.Printf("    signal         : %s\n", iface.Signal)
		}

		if iface.FrequencyKHz != 0 {
			fmt.Printf("    frequency      : %d kHz\n", iface.FrequencyKHz)
		}
	}

	fmt.Println()
}

func renderInspectSources(r *setup.InspectReport) {
	if r.SourcesErr != nil {
		PrintError(fmt.Sprintf("/sources: %v", r.SourcesErr))
		return
	}

	if r.Sources == nil {
		return
	}

	fmt.Printf("Sources (%d)\n", len(r.Sources.SourceItem))
	renderSourceTable(r.Sources.SourceItem)
	fmt.Println()
}

func renderInspectPresets(r *setup.InspectReport) {
	if r.PresetsErr != nil {
		PrintError(fmt.Sprintf("/presets: %v", r.PresetsErr))
		return
	}

	if r.Presets == nil {
		return
	}

	fmt.Printf("Presets (%d)\n", len(r.Presets.Presets))

	for _, p := range r.Presets.Presets {
		fmt.Printf("  [%s] %s (source=%s)\n", p.ID, p.ContentItem.ItemName, p.ContentItem.Source)
	}

	if len(r.Presets.Presets) == 0 {
		fmt.Println("  (none)")
	}

	fmt.Println()
}

func renderInspectRuntimeURLs(r *setup.InspectReport) {
	if r.RuntimeErr != nil {
		PrintError(fmt.Sprintf("telnet getpdo: %v", r.RuntimeErr))
		return
	}

	if r.RuntimeURLs == "" {
		return
	}

	fmt.Println("Runtime URL configuration (telnet getpdo)")

	for _, line := range strings.Split(r.RuntimeURLs, "\n") {
		fmt.Printf("  %s\n", line)
	}

	fmt.Println()
}

// sourceLine is the per-source row before any width-padding decisions
// have been made. We build the whole table in memory so column widths
// can adjust to the widest entry — variable-length displayName and
// sourceAccount values would otherwise misalign every following column.
type sourceLine struct {
	provider string
	label    string
	status   string
	account  string
	flags    string
}

// renderSourceTable prints the inspect Sources block as a single tabular
// view with auto-sized columns. Each row may carry a long displayName
// (e.g. "AMAZON (amzn1.account.AFKTQOUN…)") or sourceAccount; rather
// than truncating, we let the columns grow.
func renderSourceTable(items []models.SourceItem) {
	if len(items) == 0 {
		fmt.Println("  (none)")
		return
	}

	rows := make([]sourceLine, 0, len(items))

	for _, s := range items {
		account := s.SourceAccount
		if account == "" {
			account = "—"
		}

		// DisplayName is the app-facing label. Drop the parenthesised
		// suffix when it equals sourceAccount (pure duplication of the
		// next column) or the source key itself. Keep it for genuinely
		// distinct names like AUX (AUX IN).
		label := s.Source

		display := strings.TrimSpace(s.DisplayName)
		if display != "" &&
			!strings.EqualFold(display, s.Source) &&
			!strings.EqualFold(display, s.SourceAccount) {
			label = fmt.Sprintf("%s (%s)", s.Source, display)
		}

		// providerID is a cloud-catalog concept — not present on the
		// device's /sources response. Synthesize from
		// constants.StaticProviders. Local-only sources (AUX, BLUETOOTH,
		// LOCAL_INTERNET_RADIO) have no catalog entry by design.
		// Labeled "provider#N" so readers don't mistake it for a
		// per-source sourceID — no such field exists on /sources.
		provider := "provider#?"
		if id := lookupProviderID(s.Source); id > 0 {
			provider = fmt.Sprintf("provider#%d", id)
		}

		flags := ""
		if s.IsLocal {
			flags += " local"
		}

		if s.MultiroomAllowed {
			flags += " multiroom"
		}

		rows = append(rows, sourceLine{
			provider: provider,
			label:    label,
			status:   "status=" + string(s.Status),
			account:  "account=" + account,
			flags:    strings.TrimSpace(flags),
		})
	}

	maxProvider, maxLabel, maxStatus, maxAccount := 0, 0, 0, 0
	for _, r := range rows {
		maxProvider = max(maxProvider, len(r.provider))
		maxLabel = max(maxLabel, len(r.label))
		maxStatus = max(maxStatus, len(r.status))
		maxAccount = max(maxAccount, len(r.account))
	}

	for _, r := range rows {
		fmt.Printf("  %-*s  %-*s  %-*s  %-*s",
			maxProvider, r.provider,
			maxLabel, r.label,
			maxStatus, r.status,
			maxAccount, r.account,
		)

		if r.flags != "" {
			fmt.Printf("  %s", r.flags)
		}

		fmt.Println()
	}
}

// lookupProviderID returns the AfterTouch-catalog providerID for the
// given source name (TUNEIN, SPOTIFY, …), or 0 when the name is not in
// constants.StaticProviders. Local-only sources like AUX, BLUETOOTH,
// LOCAL_INTERNET_RADIO have no catalog entry by design — they aren't
// cloud-provisioned.
func lookupProviderID(sourceName string) int {
	for _, p := range constants.StaticProviders {
		if p.Name == sourceName {
			return p.ID
		}
	}

	return 0
}

// deviceIDSuffix returns the last 6 characters of a SoundTouch device ID,
// which is the canonical naming convention Bose uses ("Bose SoundTouch
// DE4803"). Empty when the input is shorter than 6 characters.
func deviceIDSuffix(deviceID string) string {
	if len(deviceID) < 6 {
		return ""
	}

	return deviceID[len(deviceID)-6:]
}

func setupFactoryResetCmd() *cli.Command {
	return &cli.Command{
		Name:   "factory-reset",
		Usage:  "Issue `sys factorydefault` over telnet (wipes account, presets, Wi-Fi)",
		Before: RequireHost,
		Action: func(c *cli.Context) error {
			cfg := GetClientConfig(c)
			m := setup.NewManager("", nil, nil)

			fmt.Printf("Sending factory-default to %s...\n", cfg.Host)

			logs, err := m.FactoryReset(cfg.Host)
			if logs != "" {
				fmt.Print(logs)
			}

			if err != nil {
				PrintError(err.Error())
				return err
			}

			PrintSuccess("Factory reset accepted. The speaker is rebooting into setup mode.")
			fmt.Println()
			fmt.Println("Heads-up: just before resetting, the speaker sends DELETE /streaming/account/{id}/device/{id} to its current marge URL. If that URL still pointed at streaming.bose.com (not AfterTouch), AfterTouch keeps a stale datastore entry — migrate the speaker first if you want a clean account/device record.")
			fmt.Println()
			fmt.Println("Next: connect this host to the speaker's Wi-Fi AP")
			fmt.Println("  - macOS: networksetup -setairportnetwork en0 \"Bose SoundTouch XXXX\"")
			fmt.Println("  - Linux: nmcli device wifi connect \"Bose SoundTouch XXXX\"")
			fmt.Println("Then run: soundtouch-cli setup wait-ap")

			return nil
		},
	}
}

func setupWiFiPushCmd() *cli.Command {
	return &cli.Command{
		Name:  "wifi-push",
		Usage: "POST AddWirelessProfile to the speaker's setup-mode endpoint",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "ssid", Required: true, Usage: "Home Wi-Fi SSID the speaker should join"},
			&cli.StringFlag{Name: "pass", Required: true, Usage: "Home Wi-Fi password"},
			&cli.StringFlag{Name: "security", Value: setup.DefaultWiFiSecurity, Usage: "Security type (wpa_or_wpa2, wep, open)"},
			&cli.StringFlag{Name: "ap-host", Value: setup.SpeakerSetupAP, Usage: "Speaker's setup-mode IP"},
			&cli.DurationFlag{Name: "request-timeout", Value: 30 * time.Second, Usage: "Per-request timeout (the speaker can be slow to ACK before tearing down AP mode; 10 s often races)"},
		},
		Action: func(c *cli.Context) error {
			params := setup.PushWiFiCredentialsParams{
				APHost:   c.String("ap-host"),
				SSID:     c.String("ssid"),
				Password: c.String("pass"),
				Security: c.String("security"),
			}

			ctx, cancel := context.WithTimeout(c.Context, c.Duration("request-timeout"))
			defer cancel()

			fmt.Printf("Pushing Wi-Fi credentials to %s for SSID %q...\n", params.APHost, params.SSID)

			if err := setup.PushWiFiCredentials(ctx, params); err != nil {
				PrintError(err.Error())
				return err
			}

			PrintSuccess("Credentials accepted. The speaker is leaving AP mode.")
			fmt.Println()
			fmt.Println("Next: switch this host back to your home Wi-Fi, then run:")
			fmt.Println("  soundtouch-cli setup wait-online --match=<deviceID-suffix>")

			return nil
		},
	}
}

func setupWaitAPCmd() *cli.Command {
	return &cli.Command{
		Name:  "wait-ap",
		Usage: "Poll the speaker's setup-mode IP until /info responds",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "ap-host", Value: setup.SpeakerSetupAP, Usage: "Speaker's setup-mode IP"},
			&cli.DurationFlag{Name: "interval", Value: 2 * time.Second},
			&cli.DurationFlag{Name: "timeout", Value: 5 * time.Minute},
		},
		Action: func(c *cli.Context) error {
			fmt.Printf("Waiting for %s to come up (interval=%s, timeout=%s)...\n",
				c.String("ap-host"), c.Duration("interval"), c.Duration("timeout"))

			info, err := setup.WaitForAP(
				c.Context,
				c.String("ap-host"),
				setup.PollConfig{Interval: c.Duration("interval"), Timeout: c.Duration("timeout")},
				nil,
			)
			if err != nil {
				PrintError(err.Error())
				return err
			}

			PrintSuccess(fmt.Sprintf("Speaker reachable: deviceID=%s name=%q", info.DeviceID, info.Name))

			return nil
		},
	}
}

func setupWaitOnlineCmd() *cli.Command {
	return &cli.Command{
		Name:  "wait-online",
		Usage: "Poll mDNS until a speaker matching --match comes online",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "match", Usage: "Substring matched against speaker name/serial/IP (empty = first speaker seen)"},
			&cli.DurationFlag{Name: "interval", Value: 3 * time.Second},
			&cli.DurationFlag{Name: "timeout", Value: 5 * time.Minute},
		},
		Action: func(c *cli.Context) error {
			fmt.Printf("Waiting for speaker matching %q via mDNS (interval=%s, timeout=%s)...\n",
				c.String("match"), c.Duration("interval"), c.Duration("timeout"))

			d, err := setup.WaitForOnline(
				c.Context,
				c.String("match"),
				setup.PollConfig{Interval: c.Duration("interval"), Timeout: c.Duration("timeout")},
				nil,
			)
			if err != nil {
				PrintError(err.Error())
				return err
			}

			PrintSuccess(fmt.Sprintf("Speaker discovered: name=%q host=%s serial=%s",
				d.Name, d.Host, d.SerialNo))

			return nil
		},
	}
}

func setupSSHCheckCmd() *cli.Command {
	return &cli.Command{
		Name:   "ssh-check",
		Usage:  "Probe whether port 22 is reachable on the speaker (we never auto-enable SSH on modern firmware)",
		Before: RequireHost,
		Flags: []cli.Flag{
			&cli.DurationFlag{Name: "timeout", Value: 3 * time.Second},
		},
		Action: func(c *cli.Context) error {
			cfg := GetClientConfig(c)
			addr := fmt.Sprintf("%s:22", cfg.Host)

			fmt.Printf("Probing TCP %s (timeout=%s)...\n", addr, c.Duration("timeout"))

			conn, err := net.DialTimeout("tcp", addr, c.Duration("timeout"))
			if err != nil {
				PrintError(fmt.Sprintf("port 22 not reachable: %v", err))
				fmt.Println()
				fmt.Println("Modern SoundTouch firmware (27.x) does not let us enable SSH from")
				fmt.Println("telnet — those commands were removed. To enable SSH on the speaker:")
				fmt.Println("  1. Format a FAT32 USB stick.")
				fmt.Println("  2. Create an empty file named `remote_services` at its root.")
				fmt.Println("  3. Plug the stick into the speaker (rear USB port) while it is on.")
				fmt.Println("  4. Wait ~30 s; the speaker imports the flag and re-enables sshd.")
				fmt.Println("  5. Re-run `soundtouch-cli setup ssh-check` to confirm port 22.")
				fmt.Println("See docs/guides/DEVICE-INITIAL-SETUP.md and docs/analysis/TELNET-COMMAND-REFERENCE.md.")

				return err
			}

			_ = conn.Close()

			PrintSuccess("Port 22 is open — SSH is reachable.")

			return nil
		},
	}
}

func setupInstallCACmd() *cli.Command {
	return &cli.Command{
		Name:   "install-ca",
		Usage:  "Fetch AfterTouch's CA cert and inject it into the speaker's trust store via SSH",
		Before: RequireHost,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "service-url", Required: true, Usage: "AfterTouch base URL"},
			&cli.StringFlag{Name: "auth", Usage: "Basic-auth credentials for AfterTouch as user:pass (omit to be prompted on 401)"},
		},
		Action: func(c *cli.Context) error {
			cfg := GetClientConfig(c)
			serviceURL := strings.TrimRight(c.String("service-url"), "/")

			if err := validateServiceURL(serviceURL); err != nil {
				PrintError(err.Error())
				return err
			}

			certPEM, err := fetchCACert(serviceURL, c.String("auth"))
			if err != nil {
				PrintError(err.Error())
				return err
			}

			fmt.Printf("Fetched %d bytes of CA PEM from %s/setup/ca.crt\n", len(certPEM), serviceURL)

			m := setup.NewManager(serviceURL, nil, nil)

			logs, err := m.TrustCACertFromBytes(cfg.Host, certPEM)
			if logs != "" {
				fmt.Print(logs)
			}

			if err != nil {
				PrintError(err.Error())
				return err
			}

			PrintSuccess("CA certificate installed in the speaker's trust bundle.")

			return nil
		},
	}
}

// fetchCACert pulls AfterTouch's CA bundle from /setup/ca.crt. On HTTP 401
// it prompts interactively for basic-auth credentials (or accepts --auth)
// and retries once.
func fetchCACert(serviceURL, authFlag string) ([]byte, error) {
	url := serviceURL + "/setup/ca.crt"

	doRequest := func(user, pass string) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		if user != "" {
			req.SetBasicAuth(user, pass)
		}

		client := &http.Client{Timeout: 10 * time.Second}

		return client.Do(req)
	}

	user, pass := splitAuth(authFlag)

	resp, err := doRequest(user, pass)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()

		fmt.Printf("%s requires basic auth.\n", url)

		user, pass, err = promptBasicAuth()
		if err != nil {
			return nil, err
		}

		resp, err = doRequest(user, pass)
		if err != nil {
			return nil, fmt.Errorf("GET %s (with auth): %w", url, err)
		}
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET %s returned %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}

	if !strings.Contains(string(body), "BEGIN CERTIFICATE") {
		return nil, fmt.Errorf("response from %s is not a PEM certificate", url)
	}

	return body, nil
}

func splitAuth(spec string) (string, string) {
	if spec == "" {
		return "", ""
	}

	i := strings.Index(spec, ":")
	if i < 0 {
		return spec, ""
	}

	return spec[:i], spec[i+1:]
}

// promptBasicAuth asks the user for credentials when AfterTouch returns 401.
// The password is read from stdin without echo so it doesn't end up in shell
// history or screen scrollback.
func promptBasicAuth() (string, string, error) {
	fmt.Fprint(os.Stderr, "Username: ")

	reader := bufio.NewReader(os.Stdin)

	user, err := reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("read username: %w", err)
	}

	user = strings.TrimRight(user, "\r\n")

	fmt.Fprint(os.Stderr, "Password: ")

	// syscall.Stdin is int on Unix but syscall.Handle on Windows; the cast is
	// required for the Windows cross-compile.
	pass, err := term.ReadPassword(int(syscall.Stdin)) //nolint:unconvert

	fmt.Fprintln(os.Stderr)

	if err != nil {
		return "", "", fmt.Errorf("read password: %w", err)
	}

	return user, string(pass), nil
}

func setupMigrateCmd() *cli.Command {
	return &cli.Command{
		Name:   "migrate",
		Usage:  "Apply a migration method (telnet|hosts|resolv|xml) to point the speaker at AfterTouch",
		Before: RequireHost,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "service-url", Required: true, Usage: "AfterTouch base URL"},
			&cli.StringFlag{Name: "method", Value: string(setup.MigrationMethodTelnet), Usage: "telnet | hosts | resolv | xml"},
			&cli.StringFlag{Name: "proxy-url", Usage: "Optional upstream proxy URL (for --method=xml)"},
			&cli.BoolFlag{Name: "skip-preflight", Usage: "Skip the AfterTouch settings preflight (use when AfterTouch's settings endpoint is unreachable)"},
		},
		Action: func(c *cli.Context) error {
			cfg := GetClientConfig(c)
			method := setup.MigrationMethod(c.String("method"))
			serviceURL := c.String("service-url")

			if err := validateServiceURL(serviceURL); err != nil {
				PrintError(err.Error())
				return err
			}

			m := setup.NewManager(serviceURL, nil, nil)

			// For DNS-redirect methods check that AfterTouch's DNS listener
			// is reachable — both from this machine and from the speaker.
			if !c.Bool("skip-preflight") && (method == setup.MigrationMethodResolvConf || method == setup.MigrationMethodHosts) {
				if err := runDNSPreflight(cfg.Host, serviceURL, m.NewSSH); err != nil {
					PrintError(err.Error())
					return err
				}

				// The migration's internal CA-install step uses
				// Manager.Crypto which is nil in the CLI flow. Pre-install
				// the cert via the same HTTP path as `setup install-ca`
				// so the migration finds CACertTrusted=true and skips.
				// Idempotent — TrustCACertFromBytes deduplicates by label.
				if err := preInstallCAForCLI(cfg.Host, serviceURL); err != nil {
					PrintError(err.Error())
					return err
				}
			}

			fmt.Printf("Migrating %s → %s using method=%s\n", cfg.Host, serviceURL, method)

			logs, err := m.MigrateSpeaker(cfg.Host, serviceURL, c.String("proxy-url"), nil, method)
			if logs != "" {
				fmt.Print(logs)
			}

			if err != nil {
				PrintError(err.Error())
				return err
			}

			PrintSuccess(fmt.Sprintf("Migration committed (method=%s). Reboot the speaker to apply the next-boot persistence layer.", method))

			return nil
		},
	}
}

// preInstallCAForCLI fetches AfterTouch's CA cert and SSH-injects it
// into the speaker. Mirrors what `setup install-ca` does but inline, so
// the subsequent setup.MigrateSpeaker call (which would otherwise try
// to install the CA via the nil Manager.Crypto and NPE) finds the cert
// already trusted and skips its internal install step.
//
// All operations are idempotent: TrustCACertFromBytes rebuilds the
// bundle stripping any prior CALabel block before re-appending.
func preInstallCAForCLI(deviceIP, serviceURL string) error {
	certPEM, err := fetchCACert(serviceURL, "")
	if err != nil {
		return fmt.Errorf("pre-install CA: %w", err)
	}

	m := setup.NewManager(serviceURL, nil, nil)

	if _, err := m.TrustCACertFromBytes(deviceIP, certPEM); err != nil {
		return fmt.Errorf("pre-install CA: %w", err)
	}

	return nil
}

// validateServiceURL returns an error if serviceURL cannot be parsed or has no
// hostname. A common mistake is a single-slash scheme (https:/host instead of
// https://host); the error message hints at the correction in that case.
func validateServiceURL(serviceURL string) error {
	parsed, err := url.Parse(serviceURL)
	if err != nil {
		return fmt.Errorf("invalid --service-url %q: %w", serviceURL, err)
	}

	if parsed.Hostname() == "" {
		hint := ""
		if parsed.Scheme != "" && parsed.Opaque != "" {
			hint = fmt.Sprintf(" (did you mean %s://%s?)", parsed.Scheme, strings.TrimPrefix(parsed.Opaque, "/"))
		}

		return fmt.Errorf("invalid --service-url %q: no hostname found%s", serviceURL, hint)
	}

	return nil
}

// dnsCheckResult holds the outcome of one DNS reachability probe.
type dnsCheckResult struct {
	ok      bool
	unknown bool   // SSH unavailable or nslookup not present — result indeterminate
	detail  string // "works" on success, error reason otherwise
}

func (r dnsCheckResult) label() string {
	switch {
	case r.ok:
		return "✓ works"
	case r.unknown:
		return "? " + r.detail
	default:
		return "✗ " + r.detail
	}
}

// cliDNSCheck sends a real DNS query for streaming.bose.com through the
// AfterTouch DNS listener to verify it is alive from this machine.
func cliDNSCheck(dnsHost string) dnsCheckResult {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", net.JoinHostPort(dnsHost, "53"))
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := resolver.LookupHost(ctx, "streaming.bose.com")
	if err != nil {
		return dnsCheckResult{detail: err.Error()}
	}
	if len(ips) == 0 {
		return dnsCheckResult{detail: "no answers for streaming.bose.com — listener may be misconfigured"}
	}
	return dnsCheckResult{ok: true, detail: "works"}
}

// speakerDNSCheck SSHes into the speaker and runs nslookup streaming.bose.com
// against the AfterTouch DNS server to verify reachability from the device.
func speakerDNSCheck(deviceIP, dnsHost string, newSSH func(string) setup.SSHClient) dnsCheckResult {
	addrs, err := net.LookupHost(dnsHost)
	if err != nil || len(addrs) == 0 {
		return dnsCheckResult{unknown: true, detail: fmt.Sprintf("cannot resolve %s locally to run speaker-side check", dnsHost)}
	}
	dnsIP := addrs[0]

	client := newSSH(deviceIP)
	out, sshErr := client.Run(fmt.Sprintf("nslookup streaming.bose.com %s", dnsIP))
	if sshErr != nil {
		if strings.Contains(out, "not found") || strings.Contains(out, "No such file") {
			return dnsCheckResult{unknown: true, detail: "nslookup not available on speaker"}
		}
		if strings.Contains(sshErr.Error(), "dial") || strings.Contains(sshErr.Error(), "connect") {
			return dnsCheckResult{unknown: true, detail: fmt.Sprintf("SSH unavailable: %s", sshErr)}
		}
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = sshErr.Error()
		}
		return dnsCheckResult{detail: msg}
	}
	return dnsCheckResult{ok: true, detail: "works"}
}

// runDNSPreflight checks AfterTouch DNS reachability from both the CLI machine
// and the speaker, prints a table, and returns an error only when the speaker
// side definitively cannot reach the DNS listener (CLI-only failures are
// informational — the speaker's perspective is authoritative).
func runDNSPreflight(deviceIP, serviceURL string, newSSH func(string) setup.SSHClient) error {
	parsed, _ := url.Parse(serviceURL)
	dnsHost := parsed.Hostname()

	type result struct {
		cli     dnsCheckResult
		speaker dnsCheckResult
	}
	ch := make(chan result, 1)
	go func() {
		cliCh := make(chan dnsCheckResult, 1)
		speakerCh := make(chan dnsCheckResult, 1)
		go func() { cliCh <- cliDNSCheck(dnsHost) }()
		go func() { speakerCh <- speakerDNSCheck(deviceIP, dnsHost, newSSH) }()
		ch <- result{cli: <-cliCh, speaker: <-speakerCh}
	}()
	r := <-ch

	if r.cli.ok && r.speaker.ok {
		fmt.Printf("DNS preflight (%s:53)   ✓ works\n", dnsHost)
		return nil
	}

	fmt.Printf("DNS preflight (%s:53)\n", dnsHost)
	fmt.Printf("  CLI host  %s\n", r.cli.label())
	fmt.Printf("  Speaker   %s\n", r.speaker.label())
	fmt.Println()

	if !r.speaker.ok && !r.speaker.unknown {
		return fmt.Errorf("AfterTouch DNS unreachable from speaker — %s migration would fail", serviceURL)
	}
	if r.speaker.unknown && !r.cli.ok {
		return fmt.Errorf("cannot confirm DNS reachability (SSH unavailable from speaker, CLI probe also failed) — use --skip-preflight to bypass")
	}
	return nil
}

func setupVerifyCmd() *cli.Command {
	return &cli.Command{
		Name: "verify",
		Usage: "Read-only status probe across all migration axes — doubles as preflight before applying " +
			"and verification after applying. Wraps Manager.GetMigrationSummary.",
		Before: RequireHost,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "service-url", Required: true, Usage: "AfterTouch base URL"},
			&cli.StringFlag{Name: "proxy-url", Usage: "Optional upstream proxy URL (matches the --method=xml path)"},
		},
		Action: func(c *cli.Context) error {
			cfg := GetClientConfig(c)
			serviceURL := c.String("service-url")

			if err := validateServiceURL(serviceURL); err != nil {
				PrintError(err.Error())
				return err
			}

			m := setup.NewManager(serviceURL, nil, nil)

			summary, err := m.GetMigrationSummary(cfg.Host, serviceURL, c.String("proxy-url"), nil)
			if err != nil {
				PrintError(err.Error())
				return err
			}

			renderMigrationSummary(cfg.Host, serviceURL, summary)

			// Non-zero exit when nothing is migrated so this command is
			// usable as a CI gate. Warnings alone don't fail the call.
			if !summary.IsMigrated {
				return fmt.Errorf("speaker is not migrated (no method reports green)")
			}

			return nil
		},
	}
}

// renderMigrationSummary prints a grouped, glyph-prefixed view of a
// MigrationSummary. Each line uses [✓] / [✗] / ⚠ to make scan-reading
// fast. Identical data shape to what the web UI's preflight panel
// renders — see pkg/service/handlers/web/js/script.js.
func renderMigrationSummary(deviceIP, serviceURL string, s *setup.MigrationSummary) {
	checkmark := func(b bool) string {
		if b {
			return "[✓]"
		}

		return "[✗]"
	}

	fmt.Printf("Status @ %s  (target: %s)\n", deviceIP, serviceURL)
	fmt.Println(strings.Repeat("═", 60))

	fmt.Println("Identity")
	fmt.Printf("  deviceID : %s\n", s.DeviceID)
	fmt.Printf("  name     : %s\n", s.DeviceName)
	fmt.Printf("  model    : %s\n", s.DeviceModel)
	fmt.Printf("  firmware : %s\n", s.FirmwareVersion)
	fmt.Printf("  serial   : %s\n", s.DeviceSerial)
	fmt.Println()

	fmt.Println("Pairing")

	if s.IsPaired {
		fmt.Printf("  [✓] paired (margeAccountUUID=%s)\n", s.AccountID)
	} else {
		fmt.Println("  [✗] unpaired — device reports empty margeAccountUUID")
	}

	fmt.Println()

	fmt.Println("Transport")
	fmt.Printf("  %s SSH (port 22, full system access)\n", checkmark(s.SSHSuccess))
	fmt.Printf("  %s Telnet (port 17000, diagnostic shell)\n", checkmark(s.TelnetReachable))

	if s.TelnetBanner != "" {
		fmt.Printf("        banner: %q\n", s.TelnetBanner)
	}

	if s.TelnetProbeError != "" {
		fmt.Printf("        probe error: %s\n", s.TelnetProbeError)
	}

	fmt.Println()

	fmt.Println("SSH enablement (remote_services)")
	fmt.Printf("  %s enabled\n", checkmark(s.RemoteServicesEnabled))
	fmt.Printf("  %s persistent across reboot\n", checkmark(s.RemoteServicesPersistent))

	if len(s.RemoteServicesFound) > 0 {
		fmt.Printf("        found: %s\n", strings.Join(s.RemoteServicesFound, ", "))
	}

	if s.RemoteServicesCheckErr != "" {
		fmt.Printf("        probe error: %s\n", s.RemoteServicesCheckErr)
	}

	fmt.Println()

	fmt.Println("CA certificate")
	fmt.Printf("  %s AfterTouch CA trusted by the speaker\n", checkmark(s.CACertTrusted))

	if s.ServerHTTPSURL != "" {
		fmt.Printf("        HTTPS endpoint: %s\n", s.ServerHTTPSURL)
	}

	fmt.Println()

	fmt.Printf("Migration state (overall: %s migrated)\n", checkmark(s.IsMigrated))
	fmt.Printf("  %s telnet  (envswitch + sys configuration)\n", checkmark(s.TelnetMigrated))
	fmt.Printf("  %s xml     (SoundTouchSdkPrivateCfg.xml on disk)\n", checkmark(s.XMLMigrated))
	fmt.Printf("  %s hosts   (/etc/hosts entries — deprecated)\n", checkmark(s.HostsMigrated))
	fmt.Printf("  %s resolv  (/etc/resolv.conf via DHCP hook)\n", checkmark(s.ResolvMigrated))
	fmt.Println()

	if len(s.Warnings) > 0 {
		fmt.Println("Warnings")

		for _, w := range s.Warnings {
			PrintWarning(w)
		}

		fmt.Println()
	}

	if s.ResolveIPError != "" {
		PrintError("Resolve IP error: " + s.ResolveIPError)
	}

	// Observability for the IP-resolve path. Source tells the user whether
	// the speaker itself was consulted (authoritative) or only the service
	// (best-effort). DurationMS lets us watch the SSH-ping cost trend in
	// the wild — historical comment claimed 2-5 s on firmware 27, worth
	// re-evaluating as data accumulates.
	if s.ResolveIPSource != "" {
		fmt.Printf("Resolve IP source : %s (%d ms)\n", s.ResolveIPSource, s.ResolveIPDurationMS)
	} else if s.ResolveIPDurationMS > 0 {
		fmt.Printf("Resolve IP        : %d ms\n", s.ResolveIPDurationMS)
	}
}

func setupRebootCmd() *cli.Command {
	return &cli.Command{
		Name:   "reboot",
		Usage:  "Reboot the speaker (forces the envswitch parallel-persistence layer to apply)",
		Before: RequireHost,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "method",
				Value: string(setup.RebootMethodTelnet),
				Usage: "ssh | telnet — telnet works without SSH on modern firmware",
			},
		},
		Action: func(c *cli.Context) error {
			cfg := GetClientConfig(c)
			method := setup.RebootMethod(c.String("method"))

			m := setup.NewManager("", nil, nil)

			fmt.Printf("Rebooting %s via %s...\n", cfg.Host, method)

			logs, err := m.Reboot(cfg.Host, method)
			if logs != "" {
				fmt.Print(logs)
			}

			if err != nil {
				PrintError(err.Error())
				return err
			}

			PrintSuccess("Reboot signal sent. The speaker will be unreachable for ~30 s.")

			return nil
		},
	}
}

// planStep is one recommended action in the output of `setup plan`.
// title and reason go to the human; cmd is the exact CLI line to run.
// manual=true means the user has to do it themselves (network switch).
type planStep struct {
	title  string
	cmd    string
	reason string
	manual bool
}

func setupPlanCmd() *cli.Command {
	return &cli.Command{
		Name: "plan",
		Usage: "Recommend the next setup/migration steps based on inspect + verify state. " +
			"Use --reset to plan a full factory-reset + re-provisioning sequence.",
		Before: RequireHost,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "service-url", Required: true, Usage: "AfterTouch base URL"},
			&cli.BoolFlag{Name: "reset", Usage: "Plan a full factory-reset → Wi-Fi → migrate → pair flow"},
			&cli.StringFlag{Name: "wifi-ssid", Usage: "Home Wi-Fi SSID for the wifi-push step (default: re-use the SSID inspect found)"},
			&cli.BoolFlag{Name: "include-pair", Value: true, Usage: "Include the pair step (disable if you'll pair manually)"},
		},
		Action: func(c *cli.Context) error {
			cfg := GetClientConfig(c)
			serviceURL := c.String("service-url")
			reset := c.Bool("reset")
			wifiSSID := c.String("wifi-ssid")
			includePair := c.Bool("include-pair")

			if err := validateServiceURL(serviceURL); err != nil {
				PrintError(err.Error())
				return err
			}

			m := setup.NewManager(serviceURL, nil, nil)

			fmt.Printf("Probing %s …\n\n", cfg.Host)

			inspectReport := m.Inspect(cfg.Host, setup.InspectOptions{})
			summary, summaryErr := m.GetMigrationSummary(cfg.Host, serviceURL, "", nil)

			renderPlanState(cfg.Host, inspectReport, summary, summaryErr)
			fmt.Println()

			if inspectReport.InfoErr != nil && !reset {
				PrintError("Speaker not reachable on :8090 — can't plan a non-reset migration.")
				fmt.Println("Either check the network / IP, or run again with --reset for the factory-reset path.")

				return inspectReport.InfoErr
			}

			steps := buildPlanSteps(cfg.Host, serviceURL, wifiSSID, includePair, reset, inspectReport, summary)

			if reset {
				renderPreResetNote()
			}

			renderPlanSteps(steps, includePair)

			return nil
		},
	}
}

// renderPlanState prints the brief one-screen status used at the top of
// `setup plan`. It's a deliberately compact summary — for the full
// inspect output use `setup inspect`, for the full verify output use
// `setup verify`.
func renderPlanState(deviceIP string, inspect *setup.InspectReport, summary *setup.MigrationSummary, summaryErr error) {
	check := func(b bool) string {
		if b {
			return "[✓]"
		}

		return "[✗]"
	}

	fmt.Printf("State @ %s\n", deviceIP)
	fmt.Println(strings.Repeat("─", 40))

	if inspect.Info != nil {
		i := inspect.Info
		fmt.Printf("  deviceID=%s name=%q firmware=%s\n", i.DeviceID, i.Name, firmwareOf(i))
		fmt.Printf("  margeURL=%s\n", i.MargeURL)
	} else {
		PrintError(fmt.Sprintf("/info: %v", inspect.InfoErr))
	}

	if summaryErr != nil {
		PrintError(fmt.Sprintf("verify: %v", summaryErr))
		return
	}

	currentSSID := ""

	if inspect.Network != nil {
		for i := range inspect.Network.Interfaces.Interfaces {
			if ssid := inspect.Network.Interfaces.Interfaces[i].SSID; ssid != "" {
				currentSSID = ssid
				break
			}
		}
	}

	if currentSSID != "" {
		fmt.Printf("  ssid=%q\n", currentSSID)
	}

	fmt.Printf("  %s SSH (port 22)   %s Telnet (port 17000)   %s CA trusted\n",
		check(summary.SSHSuccess), check(summary.TelnetReachable), check(summary.CACertTrusted))
	fmt.Printf("  %s paired           %s migrated (telnet=%s xml=%s hosts=%s resolv=%s)\n",
		check(summary.IsPaired), check(summary.IsMigrated),
		yesNo(summary.TelnetMigrated), yesNo(summary.XMLMigrated),
		yesNo(summary.HostsMigrated), yesNo(summary.ResolvMigrated))
}

func firmwareOf(info *setup.DeviceInfoXML) string {
	for _, c := range info.Components {
		if c.SoftwareVersion != "" {
			return c.SoftwareVersion
		}
	}

	return ""
}

func yesNo(b bool) string {
	if b {
		return "y"
	}

	return "n"
}

// buildPlanSteps is the recommendation engine. Decision order:
//
//  1. --reset path adds factory-reset → manual AP switch → wait-ap →
//     wifi-push → manual home-Wi-Fi switch → wait-online at the top.
//  2. If already migrated and paired (and not --reset): empty plan.
//  3. Otherwise pick a migration method based on capabilities:
//     telnet first (no SSH required), resolv second (DNS-based, needs
//     SSH + CA), USB-stick advice when no transport works.
//  4. Append pair step unless --include-pair=false or already paired.
func buildPlanSteps(
	host, serviceURL, wifiSSID string,
	includePair, reset bool,
	inspect *setup.InspectReport,
	summary *setup.MigrationSummary,
) []planStep {
	var steps []planStep

	if reset {
		steps = append(steps, resetSteps(host, wifiSSID, inspect)...)
		host = "<NEW_IP>" // subsequent commands target the discovered IP
	}

	if !reset && summary != nil && summary.IsMigrated && (!includePair || summary.IsPaired) {
		return steps
	}

	if reset || summary == nil || !summary.IsMigrated {
		steps = append(steps, migrationSteps(host, serviceURL, summary, reset)...)
	}

	if includePair && (reset || (summary != nil && !summary.IsPaired)) {
		steps = append(steps, planStep{
			title:  "Pair the device with an AfterTouch account",
			cmd:    fmt.Sprintf("soundtouch-cli --host=%s setup pair --service-url=%s", host, serviceURL),
			reason: "Required for preset persistence, streaming services, multi-room zones.",
		})
	}

	return steps
}

// resetSteps composes the factory-reset → AP-switch → wait-ap →
// wifi-push → home-switch → wait-online prefix that --reset adds.
func resetSteps(host, wifiSSID string, inspect *setup.InspectReport) []planStep {
	ssidArg := wifiSSID
	if ssidArg == "" {
		if currentSSID := inspectedSSID(inspect); currentSSID != "" {
			ssidArg = currentSSID
		} else {
			ssidArg = "<HOME_SSID>"
		}
	}

	match := ""
	if inspect != nil && inspect.Info != nil {
		match = deviceIDSuffix(inspect.Info.DeviceID)
	}

	if match == "" {
		match = "<deviceID-suffix>"
	}

	return []planStep{
		{
			title:  "Factory-reset the speaker",
			cmd:    fmt.Sprintf("soundtouch-cli --host=%s setup factory-reset", host),
			reason: "Wipes account pairing, presets, Wi-Fi — gives a clean baseline for the SETUP state machine.",
		},
		{
			title:  "Connect this host to the speaker's setup AP",
			cmd:    `# macOS: networksetup -setairportnetwork en0 "Bose SoundTouch XXXX"`,
			reason: "After reset the speaker broadcasts its own Wi-Fi at 192.0.2.1.",
			manual: true,
		},
		{
			title: "Wait for the speaker's setup-mode IP to respond",
			cmd:   "soundtouch-cli setup wait-ap",
		},
		{
			title:  "Push home Wi-Fi credentials to the speaker",
			cmd:    fmt.Sprintf(`soundtouch-cli setup wifi-push --ssid=%q --pass=<HOME_PASS>`, ssidArg),
			reason: "Speaker leaves AP mode and joins your home network within ~30 s.",
		},
		{
			title:  "Switch this host back to home Wi-Fi",
			cmd:    fmt.Sprintf(`# macOS: networksetup -setairportnetwork en0 %q <HOME_PASS>`, ssidArg),
			reason: "Required so wait-online's mDNS browse reaches the right network segment.",
			manual: true,
		},
		{
			title: "Discover the speaker's new IP via mDNS",
			cmd:   fmt.Sprintf("soundtouch-cli setup wait-online --match=%s", match),
		},
	}
}

// migrationSteps composes the migrate + reboot pair (plus optional
// install-ca prelude for DNS-redirect methods). In --reset mode the
// summary is nil and we default to method=telnet; otherwise we let the
// recommender pick based on the speaker's current capabilities.
func migrationSteps(host, serviceURL string, summary *setup.MigrationSummary, reset bool) []planStep {
	var steps []planStep

	method, methodReason := recommendMigrationMethod(serviceURL, summary)

	if method == "" {
		if reset {
			method = setup.MigrationMethodTelnet
			methodReason = "Default: envswitch — works on most firmware-27 devices without SSH."
		} else {
			return []planStep{{
				title:  "Enable SSH on the speaker (USB-stick procedure)",
				cmd:    "# See `soundtouch-cli setup ssh-check` output for the USB-stick steps.",
				reason: "Telnet won't respond and SSH is closed — no transport available to apply a migration.",
				manual: true,
			}}
		}
	}

	dnsRedirect := method == setup.MigrationMethodResolvConf || method == setup.MigrationMethodHosts
	if dnsRedirect && summary != nil && !summary.CACertTrusted {
		steps = append(steps, planStep{
			title:  "Install AfterTouch's CA cert on the speaker",
			cmd:    fmt.Sprintf("soundtouch-cli --host=%s setup install-ca --service-url=%s", host, serviceURL),
			reason: "DNS-redirect methods keep using https://*.bose.com URLs — the device needs to trust AfterTouch's cert.",
		})
	}

	steps = append(steps, planStep{
		title:  fmt.Sprintf("Apply URL migration using method=%s", method),
		cmd:    fmt.Sprintf("soundtouch-cli --host=%s setup migrate --service-url=%s --method=%s", host, serviceURL, method),
		reason: methodReason,
	})

	steps = append(steps, planStep{
		title:  "Reboot the speaker",
		cmd:    fmt.Sprintf("soundtouch-cli --host=%s setup reboot", host),
		reason: "The envswitch parallel-persistence layer only fully wins on next boot; reboot now to lock the new URLs in before pairing.",
	})

	return steps
}

// recommendMigrationMethod picks a migration method from the speaker's
// current capabilities. Returns "" when no transport is available.
//
// Preference order:
//
//  1. telnet (envswitch + sys configuration) — no SSH required, works on
//     firmware-27 devices that we've tested. Doesn't need CA when the
//     service URL is HTTP.
//  2. resolv (DNS-redirect via /etc/resolv.conf) — needs SSH + CA, but is
//     the only method that doesn't depend on telnet `envswitch` being
//     accepted.
//
// We deliberately don't recommend xml (legacy SSH XML rewrite — fragile)
// or hosts (deprecated — superseded by resolv).
func recommendMigrationMethod(serviceURL string, summary *setup.MigrationSummary) (setup.MigrationMethod, string) {
	if summary != nil && summary.TelnetReachable {
		reason := "Telnet (port 17000) responds — simplest path, no SSH or CA cert required."

		if strings.HasPrefix(serviceURL, "https://") {
			reason += " Note: an HTTPS service URL still needs the speaker to trust AfterTouch's cert; run `setup install-ca` first."
		}

		return setup.MigrationMethodTelnet, reason
	}

	if summary != nil && summary.SSHSuccess {
		return setup.MigrationMethodResolvConf, "Telnet unavailable but SSH works — DNS redirect via /etc/resolv.conf is the most flexible alternative."
	}

	if summary == nil {
		return setup.MigrationMethodTelnet, "Speaker not reachable yet — telnet path will be tried after wifi-push."
	}

	return "", ""
}

func inspectedSSID(r *setup.InspectReport) string {
	if r == nil || r.Network == nil {
		return ""
	}

	for i := range r.Network.Interfaces.Interfaces {
		if ssid := r.Network.Interfaces.Interfaces[i].SSID; ssid != "" {
			return ssid
		}
	}

	return ""
}

// renderPreResetNote emits a one-liner explaining the DELETE-on-reset
// semantic. Surfaced both here (in plan output) and in the factory-reset
// command so users see it regardless of which verb they ran first.
func renderPreResetNote() {
	fmt.Println("Note: when factory-reset runs, the speaker sends DELETE /streaming/account/{id}/device/{id} to its current marge URL just before wiping. If that URL still points at streaming.bose.com, AfterTouch never sees it and keeps a stale entry — run migrate (steps below) before reset for a clean datastore record.")
	fmt.Println()
}

func renderPlanSteps(steps []planStep, includePair bool) {
	if len(steps) == 0 {
		if includePair {
			PrintSuccess("Speaker is already migrated and paired. No action required.")
		} else {
			PrintSuccess("Speaker is already migrated. No action required.")
		}

		return
	}

	fmt.Println("Recommended steps")
	fmt.Println(strings.Repeat("─", 40))

	for i, step := range steps {
		marker := "•"
		if step.manual {
			marker = "✋"
		}

		fmt.Printf("  %d. %s %s\n", i+1, marker, step.title)

		if step.reason != "" {
			fmt.Printf("       └─ %s\n", step.reason)
		}

		fmt.Printf("     $ %s\n\n", step.cmd)
	}

	fmt.Println("Run them in order. Manual lines (✋) require you to switch Wi-Fi networks on this host before proceeding.")
}

func setupPairCmd() *cli.Command {
	return &cli.Command{
		Name:   "pair",
		Usage:  "Pair the speaker with an account via WebSocket SETUP state machine",
		Before: RequireHost,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "account", Usage: "7-digit account ID (empty = generate)"},
			&cli.StringFlag{Name: "mode", Value: "full", Usage: "full (state machine) or bare (setMargeAccount only — experimental)"},
			&cli.StringFlag{Name: "service-url", Value: "http://aftertouch.local:8000", Usage: "AfterTouch base URL (also populates <boseServer>/<updateServer> in setMargeAccount)"},
			&cli.StringFlag{Name: "name", Usage: "Speaker name to set during pairing (empty = keep current)"},
			&cli.IntFlag{Name: "language", Value: setup.LanguageEnglish, Usage: "sysLanguage code (2 = English)"},
			&cli.DurationFlag{Name: "step-timeout", Value: 8 * time.Second},
			&cli.StringFlag{Name: "token", Usage: "userAuthToken value (empty = use built-in placeholder matching the Bose app token shape)"},
		},
		Action: func(c *cli.Context) error {
			cfg := GetClientConfig(c)
			mode := c.String("mode")
			accountID := c.String("account")

			if accountID == "" {
				generated, err := setup.GenerateAccountID(nil)
				if err != nil {
					return fmt.Errorf("generate account id: %w", err)
				}

				accountID = generated

				fmt.Printf("Generated account id: %s\n", accountID)
			}

			if !setup.IsValidAccountID(accountID) {
				return fmt.Errorf("invalid account id %q: must be 7 digits", accountID)
			}

			switch mode {
			case "bare":
				return runPairBare(c, cfg.Host, accountID)
			case "full":
				return runPairFull(c, cfg.Host, accountID)
			default:
				return fmt.Errorf("unknown --mode=%q (want full or bare)", mode)
			}
		},
	}
}

func runPairBare(c *cli.Context, deviceIP, accountID string) error {
	m := setup.NewManager("", nil, nil)

	info, err := m.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		return fmt.Errorf("read /info: %w", err)
	}

	fmt.Printf("pre  /info deviceID=%s margeAccountUUID=%q margeURL=%q\n",
		info.DeviceID, info.MargeAccountUUID, info.MargeURL)

	// Service URL drives the extended <PairDeviceWithAccount> payload
	// (boseServer/updateServer/accountEmail). When empty, the session
	// falls back to the minimal historical shape (accountId +
	// userAuthToken only).
	serviceURL := c.String("service-url")

	var extras setup.MargePairingExtras
	if serviceURL != "" {
		extras = setup.MargePairingExtras{BoseServer: serviceURL}
	}

	session, err := setup.DialSession(deviceIP, info.DeviceID, setup.SessionConfig{
		StepTimeout:   c.Duration("step-timeout"),
		PairingExtras: extras,
	})
	if err != nil {
		return fmt.Errorf("dial WS: %w", err)
	}

	defer func() { _ = session.Close() }()

	ctx, cancel := context.WithTimeout(c.Context, c.Duration("step-timeout")+2*time.Second)
	defer cancel()

	if serviceURL != "" {
		fmt.Printf("→ setMargeAccount accountID=%s (extended: boseServer=%s)\n", accountID, serviceURL)
	} else {
		fmt.Printf("→ setMargeAccount accountID=%s (minimal payload, no SETUP bracket)\n", accountID)
	}

	if pairErr := session.SetMargeAccount(ctx, accountID, c.String("token")); pairErr != nil {
		PrintError(fmt.Sprintf("setMargeAccount: %v", pairErr))
		return pairErr
	}

	time.Sleep(2 * time.Second)

	post, err := m.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		return fmt.Errorf("read /info post: %w", err)
	}

	fmt.Printf("post /info margeAccountUUID=%q (want %q)\n", post.MargeAccountUUID, accountID)

	if post.MargeAccountUUID == accountID {
		PrintSuccess("Device accepted bare pairing.")
	} else {
		PrintWarning("Device did NOT persist the pairing — bare path likely refused silently.")
	}

	return nil
}

func runPairFull(c *cli.Context, deviceIP, accountID string) error {
	m := setup.NewManager(c.String("service-url"), nil, nil)

	plan := setup.InitPlan{
		DeviceIP:       deviceIP,
		ServiceURL:     c.String("service-url"),
		AccountID:      accountID,
		Language:       c.Int("language"),
		DeviceName:     c.String("name"),
		SkipURLRewrite: true,
		StepTimeout:    c.Duration("step-timeout"),
	}

	ctx, cancel := context.WithTimeout(c.Context, 60*time.Second)
	defer cancel()

	_, err := m.ExecuteInitPlan(ctx, plan, func(e setup.StepEvent) {
		switch e.Status {
		case setup.StatusOK:
			fmt.Printf("[%d] %s — ok\n", e.Kind, e.Name)
		case setup.StatusSkipped:
			fmt.Printf("[%d] %s — skipped\n", e.Kind, e.Name)
		case setup.StatusFailed:
			fmt.Printf("[%d] %s — FAILED: %v\n", e.Kind, e.Name, e.Err)
		case setup.StatusRunning:
			fmt.Printf("[%d] %s — ...\n", e.Kind, e.Name)
		}
	})
	if err != nil {
		PrintError(err.Error())
		return err
	}

	PrintSuccess(fmt.Sprintf("Pairing complete: accountID=%s", accountID))

	return nil
}
