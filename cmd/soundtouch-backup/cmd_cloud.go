package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/urfave/cli/v2"
)

const (
	streamingBase  = "https://streaming.bose.com"
	streamingCT    = "application/vnd.bose.streaming-v1.1+xml"
	stockholmVer   = "27.0.13-4277+8963611.epdbuild.develop.hepdswbld04.2025-10-02T13:17:00"
	nativeFrameVer = "27.0.2 -3353+4ae7c78.epdbuild.HEAD.ssgbld02.2023-10-12T15:10Z"
	protocolVer    = "67"
	appGUID        = "b94dedd1-a61b-492b-b86b-2bc32c9261f4"
	appUserAgent   = "Mozilla/5.0 (Linux; Android 13; Android SDK built for arm64 Build/TE1A.220922.034; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/101.0.4951.61 Mobile Safari/537.36 Manufacturer/unknown DeviceModel/Android-SDK-built-for-arm64 SOUNDTOUCH_MOBILE_APP/" + appGUID
)

func cloudCommand() *cli.Command {
	return &cli.Command{
		Name:  "cloud",
		Usage: "Back up your Bose SoundTouch cloud account (devices, presets, sources)",
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
		),
		Action: runCloudBackup,
	}
}

func runCloudBackup(c *cli.Context) error {
	output := resolveOutputPath(c.String("output"), c.String("format"))
	format := c.String("format")

	client, err := setupCloudClient(c.String("email"), c.String("password"))
	if err != nil {
		return err
	}

	root := archiveRoot()
	files := collectCloudFiles(client, root)

	if len(files) == 0 {
		return fmt.Errorf("no data fetched")
	}

	if err := writeArchive(output, format, files); err != nil {
		return fmt.Errorf("writing archive: %w", err)
	}

	fmt.Printf("Archive written: %s (%d files)\n", output, len(files))

	return nil
}

// setupCloudClient prompts for missing credentials, then authenticates with the Bose cloud.
func setupCloudClient(email, password string) (*cloudClient, error) {
	if email == "" || password == "" {
		var err error

		email, password, err = promptCredentials(email)
		if err != nil {
			return nil, fmt.Errorf("credentials: %w", err)
		}
	}

	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	fmt.Printf("Authenticating as %s...\n", email)

	client, err := loginToCloud(email, password)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	printOK(fmt.Sprintf("Authenticated (account ID: %s)", client.accountID))

	return client, nil
}

// collectCloudFiles fetches all cloud account data and returns a files map ready for
// archiving. Keys are prefixed with root (e.g. "soundtouch-backup-2026-05-02/cloud/").
func collectCloudFiles(client *cloudClient, root string) map[string][]byte {
	type cloudEndpoint struct {
		label    string
		filename string
		fetch    func(*cloudClient) ([]byte, error)
	}

	endpoints := []cloudEndpoint{
		{"email address", "emailaddress.xml", fetchEmailAddress},
		{"devices", "devices.xml", fetchDevices},
		{"sources", "sources.xml", fetchSources},
		{"presets", "presets.xml", fetchPresets},
		{"full account", "full.xml", fetchFull},
	}

	files := make(map[string][]byte)

	for _, ep := range endpoints {
		data, err := ep.fetch(client)
		if err != nil {
			printFail(fmt.Sprintf("%s: %v", ep.label, err))

			continue
		}

		files[root+"/cloud/"+ep.filename] = data
		printOK(fmt.Sprintf("%s (%d bytes)", ep.label, len(data)))
	}

	return files
}

type cloudClient struct {
	http      *http.Client
	accountID string
	token     string
}

type loginXML struct {
	XMLName  xml.Name `xml:"login"`
	Username string   `xml:"username"`
	Password string   `xml:"password"`
}

var accountIDRe = regexp.MustCompile(`<account\s+id="([^"]+)"`)

func loginToCloud(email, password string) (*cloudClient, error) {
	loginBody, err := xml.Marshal(loginXML{Username: email, Password: password})
	if err != nil {
		return nil, err
	}

	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>`)
	body = append(body, loginBody...)

	req, err := http.NewRequest("POST", streamingBase+"/streaming/account/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	setStreamingHeaders(req, "")

	hc := &http.Client{Timeout: 30 * time.Second}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	token := resp.Header.Get("credentials")
	if token == "" {
		return nil, fmt.Errorf("no credentials in response — check your email and password")
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}

	m := accountIDRe.FindSubmatch(data)
	if len(m) < 2 {
		return nil, fmt.Errorf("could not extract account ID from login response")
	}

	return &cloudClient{http: hc, accountID: string(m[1]), token: token}, nil
}

func setStreamingHeaders(req *http.Request, token string) {
	req.Header.Set("content-type", streamingCT)
	req.Header.Set("accept", streamingCT)
	req.Header.Set("clienttype", "SOUNDTOUCH_MOBILE_APP")
	req.Header.Set("version_stockholmversion", stockholmVer)
	req.Header.Set("version_nativeframeversion", nativeFrameVer)
	req.Header.Set("version_protocolversion", protocolVer)
	req.Header.Set("user-agent", appUserAgent)
	req.Header.Set("guid", appGUID)
	req.Header.Set("x-requested-with", "com.bose.soundtouch")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("cache-control", "no-cache")

	if token != "" {
		req.Header.Set("authorization", token)
	}
}

func (c *cloudClient) get(path string) ([]byte, error) {
	url := fmt.Sprintf("%s%s?_=%d", streamingBase, path, time.Now().UnixMilli())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	setStreamingHeaders(req, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
}

func fetchEmailAddress(c *cloudClient) ([]byte, error) {
	return c.get("/streaming/account/" + c.accountID + "/emailaddress")
}

func fetchDevices(c *cloudClient) ([]byte, error) {
	return c.get("/streaming/account/" + c.accountID + "/devices")
}

func fetchSources(c *cloudClient) ([]byte, error) {
	return c.get("/streaming/account/" + c.accountID + "/sources")
}

func fetchPresets(c *cloudClient) ([]byte, error) {
	return c.get("/streaming/account/" + c.accountID + "/presets/all")
}

func fetchFull(c *cloudClient) ([]byte, error) {
	return c.get("/streaming/account/" + c.accountID + "/full")
}
