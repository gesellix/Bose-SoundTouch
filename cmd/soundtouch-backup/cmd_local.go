package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/ssh"
	"github.com/urfave/cli/v2"
)

var localEndpoints = []struct {
	path string
	file string
}{
	{"/info", "info.xml"},
	{"/name", "name.xml"},
	{"/presets", "presets.xml"},
	{"/sources", "sources.xml"},
	{"/now_playing", "now_playing.xml"},
	{"/volume", "volume.xml"},
	{"/bass", "bass.xml"},
	{"/balance", "balance.xml"},
	{"/capabilities", "capabilities.xml"},
	{"/networkInfo", "network_info.xml"},
	{"/clockDisplay", "clock_display.xml"},
	{"/getZone", "zone.xml"},
}

// sshFiles lists individual device filesystem paths captured via SSH.
// Paths that may not exist on all devices are silently skipped.
var sshFiles = []string{
	"/etc/hosts",
	"/etc/resolv.conf",
	"/etc/remote_services",
	"/mnt/nv/remote_services",
}

// sshDirs lists device directories whose contents are recursively captured via SSH.
var sshDirs = []string{
	"/opt/Bose/etc",
	"/mnt/nv/BoseApp-Persistence/1",
}

func localCommand() *cli.Command {
	return &cli.Command{
		Name:  "local",
		Usage: "Back up one or more SoundTouch speakers on your local network",
		Flags: append(outputFlags,
			&cli.StringSliceFlag{
				Name:    "host",
				Aliases: []string{"H"},
				Usage:   "Speaker host/IP (repeatable for multiple speakers)",
				EnvVars: []string{"SOUNDTOUCH_HOST"},
			},
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "Speaker HTTP port",
				Value:   8090,
				EnvVars: []string{"SOUNDTOUCH_PORT"},
			},
			&cli.BoolFlag{
				Name:    "discover",
				Aliases: []string{"d"},
				Usage:   "Auto-discover speakers on the local network",
			},
			&cli.DurationFlag{
				Name:  "discover-timeout",
				Usage: "Discovery timeout",
				Value: 5 * time.Second,
			},
			&cli.BoolFlag{
				Name:  "ssh",
				Usage: "Also back up device filesystem files via SSH (root@host:22, no password required)",
				Value: true,
			},
		),
		Action: runLocalBackup,
	}
}

type speakerTarget struct {
	host string
	port int
	name string
}

func runLocalBackup(c *cli.Context) error {
	hosts := c.StringSlice("host")
	port := c.Int("port")
	doDiscover := c.Bool("discover") || len(hosts) == 0
	discoverTimeout := c.Duration("discover-timeout")
	doSSH := c.Bool("ssh")
	output := resolveOutputPath(c.String("output"), c.String("format"))
	format := c.String("format")

	var targets []speakerTarget

	if doDiscover {
		fmt.Printf("Discovering speakers (timeout: %s)...\n", discoverTimeout)

		ctx, cancel := context.WithTimeout(c.Context, discoverTimeout)
		defer cancel()

		cfg, _ := config.LoadFromEnv()
		svc := discovery.NewUnifiedDiscoveryService(cfg)

		found, discErr := svc.DiscoverDevices(ctx)
		if discErr != nil {
			printWarn(fmt.Sprintf("Discovery failed: %v", discErr))
		}

		for _, d := range found {
			targets = append(targets, speakerTarget{host: d.Host, port: d.Port, name: d.Name})
			printOK(fmt.Sprintf("Found: %s (%s:%d)", d.Name, d.Host, d.Port))
		}
	}

	for _, h := range hosts {
		targets = append(targets, speakerTarget{host: h, port: port})
	}

	if len(targets) == 0 {
		return fmt.Errorf("no speakers found — use --host <ip> or --discover")
	}

	hc := &http.Client{Timeout: 10 * time.Second}
	root := archiveRoot()
	files := collectLocalFiles(hc, targets, root, doSSH)

	if len(files) == 0 {
		return fmt.Errorf("no data collected")
	}

	if err := writeArchive(output, format, files); err != nil {
		return fmt.Errorf("writing archive: %w", err)
	}

	fmt.Printf("Archive written: %s (%d files)\n", output, len(files))

	return nil
}

// collectLocalFiles backs up all targets over HTTP (and optionally SSH) and returns
// a files map ready for archiving. Keys are prefixed with root.
func collectLocalFiles(hc *http.Client, targets []speakerTarget, root string, doSSH bool) map[string][]byte {
	files := make(map[string][]byte)

	for _, t := range targets {
		name, entries, err := backupSpeakerHTTP(hc, t)
		if err != nil {
			printFail(fmt.Sprintf("%s:%d — %v", t.host, t.port, err))

			continue
		}

		dir := root + "/local/" + sanitizeName(name) + "/"

		for filename, data := range entries {
			files[dir+filename] = data
		}

		printOK(fmt.Sprintf("%s: %d files via HTTP", name, len(entries)))

		if doSSH {
			sshEntries := backupSpeakerSSH(t.host, name)

			for filename, data := range sshEntries {
				files[dir+filename] = data
			}

			if len(sshEntries) > 0 {
				printOK(fmt.Sprintf("%s: %d files via SSH", name, len(sshEntries)))
			}
		}
	}

	return files
}

func backupSpeakerHTTP(hc *http.Client, t speakerTarget) (name string, files map[string][]byte, err error) {
	base := fmt.Sprintf("http://%s:%d", t.host, t.port)
	files = make(map[string][]byte)
	name = t.name
	infoFetched := false

	if name == "" {
		data, ferr := fetchRaw(hc, base+"/info")
		if ferr != nil {
			return "", nil, fmt.Errorf("cannot reach %s: %w", base, ferr)
		}

		files["info.xml"] = data
		infoFetched = true

		if extracted := xmlFirst(data, "name"); extracted != "" {
			name = extracted
		} else {
			name = t.host
		}
	}

	for _, ep := range localEndpoints {
		if ep.path == "/info" && infoFetched {
			continue
		}

		data, ferr := fetchRaw(hc, base+ep.path)
		if ferr != nil {
			printWarn(fmt.Sprintf("%s: skipped %s (%v)", name, ep.file, ferr))
			continue
		}

		files[ep.file] = data
	}

	return name, files, nil
}

// backupSpeakerSSH connects to the device via SSH and reads the key filesystem paths.
// Files that don't exist on the device are silently skipped.
// Returned map keys are relative paths within the device backup directory (e.g. "ssh/etc/hosts").
func backupSpeakerSSH(host, deviceName string) map[string][]byte {
	client := ssh.NewClient(host)
	files := make(map[string][]byte)

	for _, remotePath := range sshFiles {
		data, err := client.ReadFile(remotePath)
		if err != nil {
			// Most missing files are expected (e.g. /etc/remote_services only exists post-migration)
			printWarn(fmt.Sprintf("%s: SSH skipped %s (%v)", deviceName, remotePath, err))

			continue
		}

		if len(data) == 0 {
			printWarn(fmt.Sprintf("%s: SSH empty file %s", deviceName, remotePath))
		}

		files["ssh"+remotePath] = data
	}

	for _, remoteDir := range sshDirs {
		dirFiles, err := client.ReadDir(remoteDir)
		if err != nil {
			printWarn(fmt.Sprintf("%s: SSH skipped dir %s (%v)", deviceName, remoteDir, err))
			continue
		}

		for path, data := range dirFiles {
			files["ssh"+path] = data
		}
	}

	return files
}

func fetchRaw(hc *http.Client, url string) ([]byte, error) {
	resp, err := hc.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
}

func xmlFirst(data []byte, field string) string {
	re := regexp.MustCompile(`<` + regexp.QuoteMeta(field) + `[^>]*>([^<]+)</` + regexp.QuoteMeta(field) + `>`)

	m := re.FindSubmatch(data)
	if len(m) >= 2 {
		return strings.TrimSpace(string(m[1]))
	}

	return ""
}
