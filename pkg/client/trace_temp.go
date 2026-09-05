package client

// TEMPORARY INSTRUMENTATION -- do not merge.
//
// Added to measure the source-selection readback cost described in PR #670
// review finding 2 (three readbacks per source click, each running a full
// UpdateDeviceStatus against the speaker). Delete this file, and the
// traceTransport wiring in NewClient, once the measurement is done.
//
// Off unless AFTERTOUCH_TRACE_SPEAKER=1, so a stray build stays silent.

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var (
	traceSpeakerEnabled = os.Getenv("AFTERTOUCH_TRACE_SPEAKER") == "1"
	traceSpeakerSeq     atomic.Int64
	traceStart          = time.Now()
)

// traceTransport logs every outgoing speaker request with a sequence number,
// the elapsed time since process start, and the round-trip duration.
type traceTransport struct{ base http.RoundTripper }

func (t *traceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	seq := traceSpeakerSeq.Add(1)
	started := time.Now()
	resp, err := base.RoundTrip(req)
	elapsed := time.Since(started)

	status := "ERR"
	if resp != nil {
		status = fmt.Sprintf("%d", resp.StatusCode)
	}

	detail := ""
	if err != nil {
		detail = " err=" + strings.ReplaceAll(err.Error(), "\n", " ")
	}

	log.Printf("[SPEAKER-TRACE] #%04d t=%8.3fs %-4s %-28s host=%-22s status=%-3s took=%6.1fms%s",
		seq, time.Since(traceStart).Seconds(), req.Method, req.URL.Path,
		req.URL.Host, status, float64(elapsed.Microseconds())/1000, detail)

	return resp, err
}

// installSpeakerTrace wraps an http.Client's transport when tracing is on.
func installSpeakerTrace(c *http.Client) *http.Client {
	if !traceSpeakerEnabled || c == nil {
		return c
	}

	c.Transport = &traceTransport{base: c.Transport}

	return c
}
