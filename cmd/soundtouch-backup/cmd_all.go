package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"github.com/urfave/cli/v2"
)

func allCommand() *cli.Command {
	return &cli.Command{
		Name:  "all",
		Usage: "Back up cloud account then all paired speakers in one go",
		Description: "Authenticates with the Bose cloud, backs up account data, then reads" +
			" the device IP addresses from the cloud device list and backs up each reachable" +
			" speaker over HTTP (and optionally SSH).",
		Flags: append(outputFlags,
			&cli.StringFlag{
				Name:    "email",
				Aliases: []string{"e"},
				Usage:   "Bose account email",
				EnvVars: []string{"BOSE_EMAIL"},
			},
			&cli.StringFlag{
				Name:    "password",
				Aliases: []string{"pw"},
				Usage:   "Bose account password",
				EnvVars: []string{"BOSE_PASSWORD"},
			},
			&cli.BoolFlag{
				Name:  "ssh",
				Usage: "Also back up device filesystem files via SSH (root@host:22, no password required)",
				Value: true,
			},
		),
		Action: runAllBackup,
	}
}

func runAllBackup(c *cli.Context) error {
	doSSH := c.Bool("ssh")
	output := resolveOutputPath(c.String("output"), c.String("format"))
	format := c.String("format")

	// 1. Cloud backup
	client, err := setupCloudClient(c.String("email"), c.String("password"))
	if err != nil {
		return err
	}

	root := archiveRoot()
	files := collectCloudFiles(client, root)

	if len(files) == 0 {
		return fmt.Errorf("no cloud data fetched")
	}

	// 2. Resolve speakers from devices.xml, then back each one up
	devicesData := files[root+"/cloud/devices.xml"]
	if devicesData == nil {
		printWarn("devices.xml not available — skipping local backup")
	} else {
		targets := parseDevicesXML(devicesData)
		if len(targets) == 0 {
			printWarn("no device IP addresses found in devices.xml")
		} else {
			fmt.Printf("Found %d device(s) in cloud account, attempting local backup...\n", len(targets))
		}

		hc := &http.Client{Timeout: 10 * time.Second}

		for k, v := range collectLocalFiles(hc, targets, root, doSSH) {
			files[k] = v
		}
	}

	if err := writeArchive(output, format, files); err != nil {
		return fmt.Errorf("writing archive: %w", err)
	}

	fmt.Printf("Archive written: %s (%d files)\n", output, len(files))

	return nil
}

type xmlDevice struct {
	Name      string `xml:"name"`
	IPAddress string `xml:"ipaddress"`
}

type xmlDevices struct {
	XMLName xml.Name    `xml:"devices"`
	Devices []xmlDevice `xml:"device"`
}

// parseDevicesXML extracts speaker targets from a devices.xml cloud response.
func parseDevicesXML(data []byte) []speakerTarget {
	var d xmlDevices

	if err := xml.Unmarshal(data, &d); err != nil {
		return nil
	}

	var targets []speakerTarget

	for _, dev := range d.Devices {
		if dev.IPAddress == "" {
			continue
		}

		// Pass name as a hint for error messages; backupSpeakerHTTP re-fetches
		// from /info to get the current name and include info.xml in the archive.
		targets = append(targets, speakerTarget{host: dev.IPAddress, port: 8090, name: dev.Name})
	}

	return targets
}
