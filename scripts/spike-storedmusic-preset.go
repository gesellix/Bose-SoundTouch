//go:build ignore

// spike-storedmusic-preset.go — browse a DLNA/UPnP media server through a
// speaker, play a folder, store it as a preset and recall it.
//
// # When this is useful
//
// Reach for it for any question about STORED_MUSIC: what a media server really
// reports, whether a container is playable or presetable, what `/now_playing`
// says after selecting one, and whether a stored preset comes back. It answers
// those against real hardware in one run, and prints a per-item summary so the
// answer does not have to be read out of a wall of XML.
//
// It exists because the repo's only `/navigate` fixture was hand-written and
// wrong in three places, and a feature request was nearly designed around it.
//
// Pair it with `cmd/example-dlna-server`, which serves a real directory tree:
//
//	go run ./cmd/example-dlna-server --media-dir ~/Music/SomeAlbum --name "Spike Library"
//
// That gives a controlled server on your own machine, which is how a failure in
// the media server can be told apart from a failure in the speaker. Both were
// needed here: a NAS-proxied internet radio stream failed to play while a plain
// file-serving library worked.
//
// # What it established (ST-10 "rhino"/sm2, FW 27.0.6, 2026-09-08, two servers)
//
//   - Directories come back `isPresetable="true"` and `Playable="1"`.
//   - The ContentItem in a navigate response has NO `type` attribute; `dir` is
//     only in the sibling `<type>` element, so anything building a ContentItem
//     for /select or /storePreset must add it.
//   - Each <item> carries TWO ContentItems: the one inside <mediaItemContainer>
//     is the parent container and repeats on every item; the item's own is the
//     sibling after it.
//   - After playing a folder, `/now_playing` keeps the FOLDER location and
//     reports `isPresetable="true"`, so the Player's existing star button
//     already stores folders.
//   - A preset stored that way carries no `type` attribute and recalls fine.
//
// # How to use it
//
// Read-only unless you pass a writing flag. Work through it in order; each step
// prints what the next one needs.
//
//	# 1-3. /info, which STORED_MUSIC accounts the speaker knows, and /presets.
//	#      KEEP THE /presets OUTPUT: it is your backup before any write.
//	go run scripts/spike-storedmusic-preset.go --host 192.0.2.10
//
//	# 4. browse a server's root (account is the <UDN>/0 printed in step 2)
//	go run scripts/spike-storedmusic-preset.go --host 192.0.2.10 --account 'uuid:xxxx/0'
//
//	# descend, using a location token from the per-item summary
//	go run scripts/spike-storedmusic-preset.go --host 192.0.2.10 \
//	    --account 'uuid:xxxx/0' --location '4:cont1:20:0:0:' --name Musik
//
//	# 5. play a folder and watch what happens (CHANGES PLAYBACK). This polls
//	#    until playback reaches a terminal state, because BUFFERING_STATE is not
//	#    success: a stream can buffer for twenty seconds and then die.
//	go run scripts/spike-storedmusic-preset.go --host 192.0.2.10 \
//	    --account 'uuid:xxxx/0' --location '1' --name 'Some Album' --play
//
//	# 6. store it. Two variants, and the difference matters:
//	#    --store-nowplaying  stores the current now-playing ContentItem verbatim,
//	#                        exactly as the Player's star button does — note that
//	#                        carries NO type attribute
//	#    --store             builds a ContentItem with an explicit type="dir"
//	go run scripts/spike-storedmusic-preset.go --host 192.0.2.10 --store-nowplaying 4
//
//	# 7. recall it. Switch to another preset first, or "it plays" is
//	#    indistinguishable from "it never stopped".
//	go run scripts/spike-storedmusic-preset.go --host 192.0.2.10 --press 2
//	go run scripts/spike-storedmusic-preset.go --host 192.0.2.10 --press 4
//
// `--store`, `--store-nowplaying` and `--press` OVERWRITE a preset slot or
// change playback. Pick an empty slot from step 3, and keep that output so you
// can restore what you replaced.
//
// # Worth knowing
//
//   - Browsing INTO a leaf (a track's location as the container) never answers;
//     the speaker hangs for the whole request timeout.
//   - Location tokens are opaque and server-specific (`4:cont1:20:0:0:`, `1`,
//     `22$2935` all seen), and index dependent, so a folder preset breaks when
//     the server reindexes.
//   - Run a WebSocket listener alongside for the speaker's own account of what
//     happened; that is where the `errorUpdate` frames naming a failure appear.
//     `scripts/spike-balance-ws.go --listen-only` will do.
//
// Everything it prints is speaker and server output, including device IDs,
// server UUIDs and the names of your media. Sanitise before pasting into a
// public issue. Captures belong under `_/`, which is fully gitignored.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

var started = time.Now()

func stamp() string {
	return fmt.Sprintf("%s (+%6s)",
		time.Now().Format("15:04:05.000"),
		time.Since(started).Round(time.Millisecond))
}

func section(title string) {
	fmt.Printf("\n\n=== %s %s\n%s\n", title, strings.Repeat("=", max(0, 62-len(title))), stamp())
}

func main() {
	host := flag.String("host", "", "speaker IP or host (required)")
	port := flag.Int("port", 8090, "speaker HTTP API port")
	account := flag.String("account", "", "STORED_MUSIC sourceAccount, the <UDN>/0 from step 1")
	location := flag.String("location", "", "container location token to browse into; empty browses the root")
	name := flag.String("name", "", "item name to send when playing or storing (cosmetic, but it is what a preset shows)")
	itemType := flag.String("type", "dir", `ContentItem type: "dir" or "track"`)
	play := flag.Bool("play", false, "select the folder, then read /now_playing back (changes what the speaker is playing)")
	store := flag.Int("store", 0, `store the folder into this slot 1-6 with an explicit type="dir" (OVERWRITES it)`)
	storeNP := flag.Int("store-nowplaying", 0, "store the CURRENT now-playing ContentItem verbatim into this slot 1-6 (OVERWRITES it) — this is exactly what the Player's star button does via StoreCurrentAsPreset, and unlike --store it sends NO type attribute, because now-playing does not report one")
	press := flag.Int("press", 0, "press this preset slot and report what starts playing")
	watchPlay := flag.Duration("watch-play", 40*time.Second, "after a select, poll /now_playing until playback settles into a terminal state; BUFFERING_STATE is not one")
	settle := flag.Duration("settle", 4*time.Second, "how long to wait after a select/press before reading /now_playing")
	timeout := flag.Duration("timeout", 15*time.Second, "per-request HTTP timeout (/navigate on a big library is slow)")
	flag.Parse()

	if *host == "" {
		fmt.Fprintln(os.Stderr, "--host is required")
		flag.Usage()
		os.Exit(2)
	}

	base := fmt.Sprintf("http://%s", net.JoinHostPort(*host, fmt.Sprint(*port)))
	hc := &http.Client{Timeout: *timeout}

	section("1. /info")
	httpGet(hc, base+"/info")

	section("2. /sources  — which STORED_MUSIC accounts does this speaker know?")
	sources := httpGet(hc, base+"/sources")

	// Match the exact source, or STORED_MUSIC_MEDIA_RENDERER (a different
	// thing entirely) comes along for the ride.
	for _, line := range strings.Split(sources, "<sourceItem") {
		if !strings.Contains(line, `source="STORED_MUSIC"`) {
			continue
		}

		acct := between(line, `sourceAccount="`, `"`)
		status := between(line, `status="`, `"`)
		label, _, _ := strings.Cut(strings.SplitN(line, ">", 2)[1], "<")

		fmt.Printf("\n>> %-12s %-40q  %s\n", status, acct, label)

		if status != "READY" {
			fmt.Println("   (not READY — browsing this one will likely fail)")
		}
	}

	section("3. /presets  — BEFORE any change; keep this, it is your backup")
	httpGet(hc, base+"/presets")

	// --store-nowplaying and --press act on whatever the speaker is doing now,
	// so they need no account. Only the browse/play path does.
	if *account == "" && *storeNP == 0 && *press == 0 {
		fmt.Println("\nPass --account '<UDN>/0' from step 2 to browse the library,")
		fmt.Println("or use --store-nowplaying / --press, which need no account.")

		return
	}

	if *account == "" {
		fmt.Println("\n(no --account: skipping the browse steps)")
	}

	if *itemType == "track" && *location != "" {
		fmt.Println("\n(skipping /navigate: browsing INTO a track is meaningless and the speaker")
		fmt.Println(" just hangs on it for the full request timeout)")
	}

	section("4. /navigate  — does a dir carry isPresetable?")

	body := fmt.Sprintf(
		`<navigate source="STORED_MUSIC" sourceAccount="%s"><startItem>1</startItem><numItems>50</numItems></navigate>`,
		xmlAttr(*account))

	if *location != "" {
		// Browsing into a container: the request carries the container as an
		// <item>, exactly as models.NewNavigateRequestWithItem builds it.
		body = fmt.Sprintf(
			`<navigate source="STORED_MUSIC" sourceAccount="%s"><startItem>1</startItem><numItems>50</numItems>`+
				`<item Playable="1"><name>%s</name><type>dir</type>`+
				`<ContentItem source="STORED_MUSIC" type="dir" location="%s" sourceAccount="%s"/></item></navigate>`,
			xmlAttr(*account), xmlText(*name), xmlAttr(*location), xmlAttr(*account))
	}

	if *account != "" && (*itemType != "track" || *location == "") {
		fmt.Printf("--> %s\n", body)

		nav := httpPost(hc, base+"/navigate", body)
		reportPresetable(nav)
	}

	if *play {
		if *account == "" || *location == "" {
			fmt.Println("\n--play needs --location")
			return
		}

		section("5. /select the folder  (this changes playback)")

		sel := fmt.Sprintf(
			`<ContentItem source="STORED_MUSIC" type="%s" location="%s" sourceAccount="%s" isPresetable="true"><itemName>%s</itemName></ContentItem>`,
			xmlAttr(*itemType), xmlAttr(*location), xmlAttr(*account), xmlText(*name))

		fmt.Printf("--> %s\n", sel)
		httpPost(hc, base+"/select", sel)

		// BUFFERING_STATE is NOT success. A stream can buffer for 20 s and then
		// die with STORED_MUSIC_AP_TIMEOUT, leaving source=INVALID_SOURCE — which
		// is exactly what a too-early single read reported as a pass.
		section("5b. THE KEY QUESTION: /now_playing after playing a folder")

		np := watchNowPlaying(hc, base, *watchPlay)
		verdict(np, *location)
	}

	if *store > 0 {
		section(fmt.Sprintf("6. /storePreset into slot %d  (OVERWRITES that slot)", *store))

		now := time.Now().Unix()
		p := fmt.Sprintf(
			`<preset id="%d" createdOn="%d" updatedOn="%d">`+
				`<ContentItem source="STORED_MUSIC" type="%s" location="%s" sourceAccount="%s" isPresetable="true"><itemName>%s</itemName></ContentItem>`+
				`</preset>`,
			*store, now, now, xmlAttr(*itemType), xmlAttr(*location), xmlAttr(*account), xmlText(*name))

		fmt.Printf("--> %s\n", p)
		httpPost(hc, base+"/storePreset", p)

		time.Sleep(2 * time.Second)

		section("6b. /presets  — did it persist, and with which type?")
		httpGet(hc, base+"/presets")
	}

	if *storeNP > 0 {
		section(fmt.Sprintf("6np. store now-playing verbatim into slot %d  (OVERWRITES it)", *storeNP))

		np := httpGet(hc, base+"/now_playing")

		ci := ""
		if _, after, found := strings.Cut(np, "<ContentItem"); found {
			if body, _, ok := strings.Cut(after, "</ContentItem>"); ok {
				ci = "<ContentItem" + body + "</ContentItem>"
			} else if attrs, _, ok := strings.Cut(after, "/>"); ok {
				ci = "<ContentItem" + attrs + "/>"
			}
		}

		if ci == "" {
			fmt.Println("\n!! no ContentItem in /now_playing — nothing to store")
		} else {
			now := time.Now().Unix()
			p := fmt.Sprintf(`<preset id="%d" createdOn="%d" updatedOn="%d">%s</preset>`, *storeNP, now, now, ci)

			fmt.Printf("\n>> storing the now-playing item verbatim (note: no type attribute)\n--> %s\n", p)
			httpPost(hc, base+"/storePreset", p)

			time.Sleep(2 * time.Second)

			section("6np-b. /presets  — did it persist, and with which type?")
			httpGet(hc, base+"/presets")
		}
	}

	if *press > 0 {
		section(fmt.Sprintf("7. press PRESET_%d  (this changes playback)", *press))

		key := fmt.Sprintf(`<key state="press" sender="Gabbo">PRESET_%d</key>`, *press)
		httpPost(hc, base+"/key", key)

		key = fmt.Sprintf(`<key state="release" sender="Gabbo">PRESET_%d</key>`, *press)
		httpPost(hc, base+"/key", key)

		fmt.Printf("%s  waiting %s for the speaker to settle…\n", stamp(), *settle)
		time.Sleep(*settle)

		section("7b. /now_playing after the preset press — did the folder start?")
		httpGet(hc, base+"/now_playing")
	}

	fmt.Println("\nIf you stored a preset, restore the slot from the /presets output in step 3.")
}

// reportPresetable summarises one <item> per line. It deliberately ignores the
// ContentItem nested in <mediaItemContainer> (that is the PARENT container, and
// every item repeats it) and reports the item's own sibling ContentItem, which
// is the one /select and /storePreset consume.
//
// It also prints the <type> element separately from the ContentItem's type
// attribute, because on real hardware the attribute is ABSENT on navigate
// results and only the element carries "dir".
func reportPresetable(nav string) {
	if nav == "" {
		return
	}

	fmt.Println("\n>> per-item summary:")

	items := strings.Split(nav, "<item ")[1:]
	if len(items) == 0 {
		fmt.Println("   (no <item> elements in the response)")
		return
	}

	for _, it := range items {
		// Drop the parent-container ContentItem so it cannot be mistaken for
		// the item's own.
		own := it
		if i := strings.Index(own, "</mediaItemContainer>"); i >= 0 {
			own = own[i:]
		}

		ci := ""
		if _, after, found := strings.Cut(own, "<ContentItem"); found {
			ci, _, _ = strings.Cut(after, ">")
		}

		fmt.Printf("   <type>%-6s ContentItem type=%-6q isPresetable=%-7q playable=%-3q\n        location=%q\n        name=%q\n",
			between(it, "<type>", "</type>")+"</type>",
			between(ci, `type="`, `"`),
			between(ci, `isPresetable="`, `"`),
			between(it, `Playable="`, `"`),
			between(ci, `location="`, `"`),
			between(own, "<itemName>", "</itemName>"))
	}

	// Scope both questions to the items' OWN ContentItems. The ones inside
	// <mediaItemContainer> echo the container we sent in the request, type
	// attribute included, so counting those answers the wrong question.
	presetable, typed := 0, 0

	for _, it := range items {
		own := it
		if i := strings.Index(own, "</mediaItemContainer>"); i >= 0 {
			own = own[i:]
		}

		ci := ""
		if _, after, found := strings.Cut(own, "<ContentItem"); found {
			ci, _, _ = strings.Cut(after, ">")
		}

		if between(ci, `isPresetable="`, `"`) == "true" {
			presetable++
		}

		if between(ci, `type="`, `"`) != "" {
			typed++
		}
	}

	fmt.Printf("\n>> items whose own ContentItem says isPresetable=true: %d/%d\n", presetable, len(items))
	fmt.Printf(">> items whose own ContentItem has a type attribute:   %d/%d", typed, len(items))

	if typed == 0 {
		fmt.Print("  <- navigate never supplies it; we must add type=\"dir\" ourselves")
	}

	fmt.Println()
}

// watchNowPlaying polls until the play status stops changing into something
// terminal, and returns the LAST reading. It exists because the first version of
// this spike read once after four seconds, caught BUFFERING_STATE, and called a
// stream that died twenty seconds later a success.
func watchNowPlaying(c *http.Client, base string, d time.Duration) string {
	start := time.Now()
	last, lastLine := "", ""

	for time.Since(start) < d {
		np := httpGetQuiet(c, base+"/now_playing")
		status := between(np, "<playStatus>", "</playStatus>")
		src := between(np, `source="`, `"`)

		// Key on both, or a run where playStatus never appears prints nothing
		// at all until it fails, which is what happened the first time.
		line := fmt.Sprintf("playStatus=%-16s source=%s", status, src)
		if line != lastLine {
			fmt.Printf("%s  %6s  %s\n", stamp(), time.Since(start).Round(100*time.Millisecond), line)

			lastLine = line
		}

		last = np

		if src == "INVALID_SOURCE" {
			fmt.Printf("  >> PLAYBACK FAILED after %s: the speaker fell back to INVALID_SOURCE\n",
				time.Since(start).Round(100*time.Millisecond))

			break
		}

		if status == "PLAY_STATE" && time.Since(start) > 15*time.Second {
			fmt.Printf("  >> still playing after %s — treating as success\n",
				time.Since(start).Round(100*time.Millisecond))

			break
		}

		time.Sleep(2 * time.Second)
	}

	fmt.Printf("\n%s\n", strings.TrimSpace(last))

	return last
}

func httpGetQuiet(c *http.Client, u string) string {
	resp, err := c.Get(u) //nolint:noctx // spike
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	b, _ := io.ReadAll(resp.Body)

	return string(b)
}

// verdict answers question 1 in one line, because that is the whole point.
func verdict(np, folderLocation string) {
	// The now-playing ContentItem is the only one in the document, so the
	// attributes up to the first ">" after it are what we want.
	item := ""
	if _, after, found := strings.Cut(np, "<ContentItem"); found {
		item, _, _ = strings.Cut(after, ">")
	}

	presetable := between(item, `isPresetable="`, `"`)
	loc := between(item, `location="`, `"`)

	fmt.Printf("\n>> now-playing isPresetable = %q\n", presetable)
	fmt.Printf(">> now-playing location     = %q\n", loc)
	fmt.Printf(">> folder location we sent  = %q\n", folderLocation)

	if strings.Contains(np, `source="INVALID_SOURCE"`) {
		fmt.Println(">> INCONCLUSIVE: playback collapsed to INVALID_SOURCE, so now-playing no longer")
		fmt.Println(">>               describes the item at all — the empty location above is the")
		fmt.Println(">>               fallback item, not evidence about what was stored.")
		fmt.Println(">>               Re-run against content that actually plays on this speaker.")

		return
	}

	switch {
	case presetable == "true" && loc == folderLocation:
		fmt.Println(">> VERDICT: now-playing keeps the FOLDER location and is presetable, so the")
		fmt.Println(">>          existing star button would store the folder. Whether it PLAYS is a")
		fmt.Println(">>          separate question — see the playStatus trace above.")
	case presetable == "true":
		fmt.Println(">> VERDICT: presetable, but the location moved on to the track — the star button would")
		fmt.Println(">>          store a single track, not the folder. An explicit ContentItem store is needed.")
	default:
		fmt.Println(">> VERDICT: not presetable from now-playing; an explicit ContentItem store is needed.")
	}
}

func httpGet(c *http.Client, u string) string {
	start := time.Now()

	resp, err := c.Get(u) //nolint:noctx // spike
	if err != nil {
		fmt.Printf("%s  !! %s failed after %s: %v\n", stamp(), u, time.Since(start).Round(time.Millisecond), err)
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	b, _ := io.ReadAll(resp.Body)
	fmt.Printf("%s  %s  [%s in %s]\n%s\n",
		stamp(), u, resp.Status, time.Since(start).Round(time.Millisecond), strings.TrimSpace(string(b)))

	return string(b)
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
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	b, _ := io.ReadAll(resp.Body)
	fmt.Printf("%s  POST %s  [%s in %s]\n%s\n",
		stamp(), u, resp.Status, time.Since(start).Round(time.Millisecond), strings.TrimSpace(string(b)))

	return string(b)
}

func between(s, open, closing string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}

	s = s[i+len(open):]

	j := strings.Index(s, closing)
	if j < 0 {
		return ""
	}

	return s[:j]
}

func xmlAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func xmlText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
