//go:build ignore

// spike-balance-ws.go — probe the SoundTouch /balance endpoint over both HTTP
// and the speaker's Gabbo WebSocket.
//
// # When this is useful
//
// Reach for it when you need to know what a speaker ACTUALLY does with an
// endpoint, rather than what a model in this repo assumes. It was written
// because `pkg/models/balance.go` turned out to be a copy of `bass.go` with the
// numbers changed — invented range, invented element names, invented request
// shape — and three test files asserted the same invention, so everything was
// green and everything was wrong.
//
// More generally it is a template for "the REST call does not work, is the
// feature impossible?". Here the answer was no: some endpoints are driven over
// the WebSocket, which is a generic RPC channel and not just an event feed.
// See docs/content/docs/reference/DEVICE-PAIRING-FLOW.md for the envelope, and
// the speaker's own web app (Stockholm) for which endpoints use it.
//
// # What it established (ST-10 "rhino"/sm2, FW 27.0.6, paired, 2026-09-08)
//
//   - Range is -7..+7, default 0, camelCase elements, gated on `lrStereoCapable`
//     from /capabilities. Balance belongs to the stereo-pair GROUP and is
//     addressed to the master. (Not to be confused with a multiroom ZONE.)
//   - The write goes over the WebSocket. Its response frame carries the NEW
//     value, so a write is self-confirming.
//   - `balanceUpdated` is emitted, empty (a refetch signal), and broadcast to
//     every connected client.
//   - HTTP GET /balance can lag a write by about a second.
//   - ws port 8090 refuses the handshake; 8080 is the only WebSocket port.
//
// # How to use it
//
// Point it at the MASTER of a stereo pair. The other member and an unpaired
// speaker are the controls — both should report balance as unavailable.
//
//	# Read-only. Prints /info, /capabilities, /getGroup and /balance, then
//	# dials the socket and asks for balance over it. Writes nothing.
//	go run scripts/spike-balance-ws.go --host 192.0.2.10
//
//	# Write -3 over the WebSocket, the way Stockholm does it, then watch how
//	# long HTTP takes to catch up.
//	go run scripts/spike-balance-ws.go --host 192.0.2.10 --set -3
//
//	# Variants worth trying when a write misbehaves:
//	#   --source-item     add <sourceItem source="SETTINGS"/> (optional in practice)
//	#   --try-http-write  attempt POST /balance too, on a short timeout
//	#   --ws-port 8090    confirm which port actually accepts the handshake
//
//	# Run this in a SECOND terminal, started FIRST, to see whether events reach
//	# clients that did not ask for anything. This is how the broadcast
//	# behaviour of balanceUpdated was established.
//	go run scripts/spike-balance-ws.go --host 192.0.2.10 --listen-only
//
// # Two traps this spike fell into, preserved so the next one does not
//
//  1. gorilla/websocket treats ANY read error as fatal, `SetReadDeadline`
//     timeouts included: it stores the error and every later read returns it
//     instantly. Using a read deadline as a per-request timeout therefore kills
//     the connection at the first quiet moment, after which the peer looks
//     permanently silent. That produced a confidently wrong published finding.
//     Hence the single reader goroutine with no read deadline, and the phase
//     timeout in a select. Timestamps on every line are what exposed it: four
//     consecutive six-second waits finishing in the same millisecond.
//  2. Reading state back too early. A value can be correct in the response and
//     still stale over HTTP a moment later.
//
// Everything it prints is speaker output, including device IDs. Sanitise before
// pasting into a public issue. Reporter-style captures belong under `_/`, which
// is fully gitignored.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	host := flag.String("host", "", "speaker IP or host (required)")
	httpPort := flag.Int("http-port", 8090, "speaker HTTP API port")
	wsPort := flag.Int("ws-port", 8080, "speaker WebSocket port (Stockholm uses 8080; captures also show 8090)")
	set := flag.Int("set", 999, "target balance to write over the WebSocket; omit for a read-only run")
	sourceItem := flag.Bool("source-item", false, `include <sourceItem source="SETTINGS"/> in the write request`)
	tryHTTPWrite := flag.Bool("try-http-write", false, "also attempt POST /balance over HTTP (expected to hang; uses its own short timeout)")
	listen := flag.Duration("listen", 6*time.Second, "how long to keep reading WebSocket frames after each request")
	listenOnly := flag.Bool("listen-only", false, "just connect and print every frame, forever: run this in a second terminal BEFORE the writing run, to see whether events go to other clients but not to the requester")
	control := flag.Bool("control", true, `first send url="info" method="GET" as a control: if THAT gets no response either, the silence is not specific to balance`)
	watch := flag.Duration("watch", 20*time.Second, "after a write, poll HTTP /balance once a second for this long (socket still open), then close the socket and poll again")
	httpTimeout := flag.Duration("http-timeout", 8*time.Second, "per-request HTTP timeout")
	flag.Parse()

	if *host == "" {
		fmt.Fprintln(os.Stderr, "--host is required")
		flag.Usage()
		os.Exit(2)
	}

	base := fmt.Sprintf("http://%s", net.JoinHostPort(*host, fmt.Sprint(*httpPort)))
	hc := &http.Client{Timeout: *httpTimeout}

	// Passive observer. The point is to tell two things apart that the single
	// -connection runs cannot: "the speaker never answers" and "the speaker
	// answers, but not the client that asked". Start this first, in its own
	// terminal, then do the writing run from another.
	if *listenOnly {
		section(fmt.Sprintf("LISTEN ONLY: ws://%s, printing every frame until Ctrl-C",
			net.JoinHostPort(*host, fmt.Sprint(*wsPort))))

		conn := dial(*host, *wsPort)

		defer func() {
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(time.Second))
			_ = conn.Close()
		}()

		for {
			// A long window per call so the "quiet for N" lines stay sparse;
			// frames are printed the moment they arrive regardless.
			drain(60 * time.Second)
		}
	}

	section("1. HTTP GET /info")
	info := httpGet(hc, base+"/info")
	deviceID := between(info, `deviceID="`, `"`)
	fmt.Printf("\n>> deviceID = %q\n", deviceID)
	fmt.Printf(">> type     = %q\n", between(info, "<type>", "</type>"))

	if deviceID == "" {
		log.Fatal("could not read deviceID from /info; cannot build the msg envelope")
	}

	section("2. HTTP GET /capabilities  (looking for lrStereoCapable)")
	caps := httpGet(hc, base+"/capabilities")
	fmt.Printf("\n>> lrStereoCapable present: %v\n", strings.Contains(caps, "lrStereoCapable"))

	section("3. HTTP GET /getGroup  (stereo pair, NOT multiroom /getZone)")
	groupBefore := httpGet(hc, base+"/getGroup")

	section("4. HTTP GET /balance  (STReborn: hangs while the speaker sleeps)")
	httpGet(hc, base+"/balance")

	if *tryHTTPWrite && *set != 999 {
		section("5. HTTP POST /balance  (expected to hang; short timeout)")
		short := &http.Client{Timeout: 5 * time.Second}
		body := fmt.Sprintf("<balance><targetBalance>%d</targetBalance></balance>", *set)
		fmt.Printf("--> %s\n", body)
		httpPost(short, base+"/balance", body)
	}

	// ---- WebSocket ----

	section(fmt.Sprintf("6. WebSocket dial ws://%s (subprotocol gabbo)",
		net.JoinHostPort(*host, fmt.Sprint(*wsPort))))

	conn := dial(*host, *wsPort)
	closed := false

	defer func() {
		if closed {
			return
		}

		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second))
		_ = conn.Close()
	}()

	reqID := 0
	send := func(label, envelope string) {
		fmt.Printf("\n%s  --> %s\n%s\n", stamp(), label, envelope)
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

		if err := conn.WriteMessage(websocket.TextMessage, []byte(envelope)); err != nil {
			log.Fatalf("write %s: %v", label, err)
		}

		drain(*listen)
	}

	// Control: does this speaker answer ANY request on this socket? If /info is
	// silent too, the problem is the connection state or the port, not balance.
	if *control {
		reqID++
		send("info GET  [CONTROL: expect a response frame]", fmt.Sprintf(
			`<msg><header deviceID="%s" url="info" method="GET"><request requestID="%d"><info type="new"/></request></header></msg>`,
			deviceID, reqID))
	}

	// Frames exactly as Stockholm builds them: method is GET when the body is
	// empty and POST when it is not (js/device_controller.js), and the body
	// template is balanceSet (js/message_controller.js).
	reqID++
	send("balance GET", fmt.Sprintf(
		`<msg><header deviceID="%s" url="balance" method="GET"><request requestID="%d"><info type="new"/></request></header></msg>`,
		deviceID, reqID))

	if *set != 999 {
		src := ""
		if *sourceItem {
			src = `<sourceItem source="SETTINGS"/>`
		}

		reqID++
		send("balance POST (balanceSet)", fmt.Sprintf(
			`<msg><header deviceID="%s" url="balance" method="POST"><request requestID="%d"><info mainNode="balanceSet" type="new"/>%s</request></header><body><balance><targetBalance>%d</targetBalance></balance></body></msg>`,
			deviceID, reqID, src, *set))

		reqID++
		send("balance GET (read back)", fmt.Sprintf(
			`<msg><header deviceID="%s" url="balance" method="GET"><request requestID="%d"><info type="new"/></request></header></msg>`,
			deviceID, reqID))

		// The write itself is immediate: it is audible the moment the frame is
		// sent. What lags is the HTTP read, which serves a stale value for a
		// while afterwards. This measures how stale, which is the number that
		// decides whether a UI may read back over HTTP to confirm a write.
		// (It may not; see the findings note.)
		section(fmt.Sprintf("6b. how stale is HTTP? poll for %s, socket still open", *watch))
		poll(hc, base+"/balance", *watch, *set)

		section("6c. close the socket and poll again (does closing shake it loose?)")

		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second))
		_ = conn.Close()
		closed = true

		poll(hc, base+"/balance", *watch, *set)
	}

	// ---- after-state ----

	section("7. HTTP GET /balance  (after)")
	httpGet(hc, base+"/balance")

	section("8. HTTP GET /getGroup  (after — did the write disturb the pairing?)")
	groupAfter := httpGet(hc, base+"/getGroup")

	fmt.Printf("\n>> /getGroup unchanged: %v\n", strings.TrimSpace(groupBefore) == strings.TrimSpace(groupAfter))
	fmt.Println("\nRemember to restore the balance to its previous value if you changed it.")
}

// started anchors the elapsed column so every line in a run can be lined up
// against the others, and against what the speaker was audibly doing.
var started = time.Now()

func stamp() string {
	return fmt.Sprintf("%s (+%6s)",
		time.Now().Format("15:04:05.000"),
		time.Since(started).Round(time.Millisecond))
}

func section(title string) {
	fmt.Printf("\n\n=== %s %s\n%s\n", title, strings.Repeat("=", max(0, 62-len(title))), stamp())
}

func dial(host string, port int) *websocket.Conn {
	u := url.URL{Scheme: "ws", Host: net.JoinHostPort(host, fmt.Sprint(port)), Path: "/"}
	d := websocket.Dialer{HandshakeTimeout: 8 * time.Second, Subprotocols: []string{"gabbo"}}

	start := time.Now()

	conn, resp, err := d.Dial(u.String(), nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if err != nil {
		log.Fatalf("dial %s: %v  (try --ws-port 8090)", u.String(), err)
	}

	fmt.Printf("%s  connected in %s, negotiated subprotocol %q\n",
		stamp(), time.Since(start).Round(time.Millisecond), conn.Subprotocol())

	startReader(conn)

	// The speaker greets with <SoundTouchSdkInfo …/>; show it and anything else
	// it volunteers before we ask for something.
	drain(2 * time.Second)

	return conn
}

// frames is fed by a single reader goroutine started at dial time.
//
// This used to be a straight ReadMessage with a per-phase read deadline, which
// was WRONG and produced a false finding. gorilla/websocket treats any read
// error, timeout included, as fatal: it stores the error and every later read
// returns it immediately. So the first expired deadline killed the connection,
// and every subsequent "no further frames" was instant and meaningless — the
// spike had stopped listening without saying so. The giveaway was in the
// timestamps: four consecutive 6-second waits all completing in the same
// millisecond.
//
// One goroutine with no read deadline keeps the connection healthy; the phase
// timeout lives in the select instead.
var frames = make(chan string, 64)
var readerErr = make(chan error, 1)

func startReader(conn *websocket.Conn) {
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				readerErr <- err
				close(frames)

				return
			}

			frames <- strings.TrimSpace(string(data))
		}
	}()
}

// drain prints every frame that arrives within d. It never stops early: a
// silent speaker is itself a result, and so is a second frame arriving after
// the response (balanceUpdated).
func drain(d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()

	seen := 0

	for {
		select {
		case f, ok := <-frames:
			if !ok {
				fmt.Printf("%s      connection closed by peer\n", stamp())
				return
			}

			seen++

			fmt.Printf("%s  <-- %s\n", stamp(), f)
		case err := <-readerErr:
			fmt.Printf("%s      read error: %v\n", stamp(), err)

			return
		case <-timer.C:
			if seen == 0 {
				fmt.Printf("%s      (no frames within %s)\n", stamp(), d)
			} else {
				fmt.Printf("%s      (%d frame(s), then quiet for %s)\n", stamp(), seen, d)
			}

			return
		}
	}
}

// poll reads the endpoint once a second and prints only the transitions, so a
// 20-second watch is three lines rather than twenty. It reports how long the
// speaker took to expose the value we asked for.
func poll(c *http.Client, u string, d time.Duration, want int) {
	start := time.Now()
	last := ""
	target := fmt.Sprintf("<targetBalance>%d</targetBalance>", want)
	// The first two reads go back to back, with no sleep between them. If the
	// second one is already correct, the staleness is refreshed BY READING
	// rather than by the passage of time — which is what the evidence so far
	// suggests, and which decides whether a UI could paper over it with a
	// double read (it should not; it should just not read back at all).
	reads := 0

	for time.Since(start) < d {
		resp, err := c.Get(u) //nolint:noctx // spike
		if err != nil {
			fmt.Printf("%s  %6s  !! %v\n", stamp(), time.Since(start).Round(100*time.Millisecond), err)
			time.Sleep(time.Second)

			continue
		}

		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		cur := fmt.Sprintf("target=%s actual=%s",
			between(string(body), "<targetBalance>", "</targetBalance>"),
			between(string(body), "<actualBalance>", "</actualBalance>"))

		if cur != last {
			fmt.Printf("%s  %6s  %s\n", stamp(), time.Since(start).Round(100*time.Millisecond), cur)
			last = cur
		}

		reads++

		if strings.Contains(string(body), target) {
			fmt.Printf("  >> reached the requested value after %s, on read #%d\n",
				time.Since(start).Round(100*time.Millisecond), reads)

			return
		}

		if reads > 1 {
			time.Sleep(time.Second)
		}
	}

	fmt.Printf("  >> did NOT reach the requested value within %s\n", d)
}

func httpGet(c *http.Client, u string) string {
	start := time.Now()

	resp, err := c.Get(u) //nolint:noctx // spike
	if err != nil {
		fmt.Printf("%s  !! %s failed after %s: %v\n", stamp(), u, time.Since(start).Round(time.Millisecond), err)
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("%s  %s  [%s in %s]\n%s\n",
		stamp(), u, resp.Status, time.Since(start).Round(time.Millisecond), strings.TrimSpace(string(body)))

	return string(body)
}

func httpPost(c *http.Client, u, body string) string {
	start := time.Now()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u, strings.NewReader(body))
	if err != nil {
		fmt.Printf("!! build request: %v\n", err)
		return ""
	}

	req.Header.Set("Content-Type", "text/xml")

	resp, err := c.Do(req)
	if err != nil {
		fmt.Printf("%s  !! POST %s failed after %s: %v\n", stamp(), u, time.Since(start).Round(time.Millisecond), err)
		fmt.Println("   (a timeout here is the reported hang, and is itself the finding)")

		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	out, _ := io.ReadAll(resp.Body)
	fmt.Printf("%s  POST %s  [%s in %s]\n%s\n",
		stamp(), u, resp.Status, time.Since(start).Round(time.Millisecond), strings.TrimSpace(string(out)))

	return string(out)
}

func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}

	s = s[i+len(open):]

	j := strings.Index(s, close)
	if j < 0 {
		return ""
	}

	return s[:j]
}
