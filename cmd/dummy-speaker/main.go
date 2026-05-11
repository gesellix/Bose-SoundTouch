// Command dummy-speaker runs an HTTP-only fake SoundTouch speaker and
// optionally registers it with a running soundtouch-service so the web UI
// has a device to display.
//
// Intended for documentation screenshots and local UI smoke checks. Do not
// use against a real network — the fixture payload is synthetic and would
// confuse other tooling that expects live device data.
//
// Example:
//
//	dummy-speaker --port 8090 --register http://localhost:8000
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/testing/fakespeaker"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8090", "bind address for the fake speaker's HTTP API")
	telnetListen := flag.String("telnet-listen", "127.0.0.1:17000", "bind address for the fake speaker's telnet diagnostic shell (empty to disable)")
	register := flag.String("register", "", "service base URL (e.g. http://localhost:8000) to self-register with via POST /setup/devices")
	registerAs := flag.String("register-as", "", "address to send to /setup/devices (defaults to --listen)")

	flag.Parse()

	s, err := fakespeaker.Start(fakespeaker.Config{
		HTTPListen:   *listen,
		TelnetListen: *telnetListen,
	})
	if err != nil {
		log.Fatalf("start fake speaker: %v", err)
	}

	log.Printf("fake speaker HTTP listening on http://%s", s.HTTPAddr())

	if addr := s.TelnetAddr(); addr != "" {
		log.Printf("fake speaker telnet listening on tcp://%s", addr)
	}

	if *register != "" {
		target := *registerAs
		if target == "" {
			target = s.HTTPAddr()
		}

		if err := registerWithService(*register, target); err != nil {
			log.Printf("self-register failed: %v (continuing anyway)", err)
		} else {
			log.Printf("registered %s with service at %s", target, *register)
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Printf("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := s.Stop(ctx); err != nil {
		log.Printf("stop: %v", err)
	}
}

func registerWithService(serviceURL, deviceAddr string) error {
	body, err := json.Marshal(map[string]string{"ip": deviceAddr})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serviceURL+"/setup/devices", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("service responded %s", resp.Status)
	}

	return nil
}
