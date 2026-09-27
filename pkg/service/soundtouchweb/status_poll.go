package soundtouchweb

import (
	"log/slog"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// Background status polling runs only while someone is watching (issue 766).
//
// The poll exists for an open page: it is the backstop for changes the
// speaker does not report as events (Spotify Connect track changes) and for
// state missed while the speaker WebSocket reconnects. An unwatched player
// used to poll every speaker regardless, seven or eight requests per speaker
// every 30 s for as long as the service ran. Now the poll starts when the
// first browser WebSocket connects (either pool) and stops when the last one
// leaves; REST clients that never open a WebSocket get the cache refreshed on
// demand instead (refreshStaleDeviceStatuses).
const (
	// statusPollInterval is how often every registered speaker is refreshed
	// while at least one browser WebSocket is connected.
	statusPollInterval = 30 * time.Second

	// statusCacheTTL is how old a speaker's cached status may be before a
	// REST endpoint answering from the cache refreshes it first, and before
	// a newly opened page triggers a refresh. Short enough that a client
	// polling the REST API (a third-party player, Home Assistant) sees
	// current state, long enough that a page issuing several requests in a
	// row, or several clients at once, costs the speaker one round.
	statusCacheTTL = 10 * time.Second

	// statusRefreshWait bounds how long a REST request waits for an
	// on-demand refresh. A healthy speaker answers a full round well within
	// it; a slow one must not hold the response hostage, so the request then
	// answers from the cache and the round lands for the next one.
	statusRefreshWait = 5 * time.Second
)

func (app *WebApp) pollInterval() time.Duration {
	if app.statusPollInterval > 0 {
		return app.statusPollInterval
	}

	return statusPollInterval
}

func (app *WebApp) cacheTTL() time.Duration {
	if app.statusCacheTTL > 0 {
		return app.statusCacheTTL
	}

	return statusCacheTTL
}

// watcherJoinedLocked counts a newly registered browser WebSocket and starts
// the status poll when it is the first. Callers hold the mutex of the pool
// they just inserted into, so a removal of the same connection cannot be
// counted before its registration (lock order: pool mutex, then watchMu).
func (app *WebApp) watcherJoinedLocked() {
	app.watchMu.Lock()
	defer app.watchMu.Unlock()

	app.watchers++
	if app.watchers != 1 {
		return
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	app.pollStop = stop
	app.pollDone = done

	slog.Debug("player: a browser is watching, starting the speaker status poll",
		"interval", app.pollInterval().String())

	go app.runStatusPoll(stop, done)
}

// watcherLeftLocked is watcherJoinedLocked's counterpart, called with the
// pool mutex held after a connection was actually removed from it.
func (app *WebApp) watcherLeftLocked() {
	app.watchMu.Lock()
	defer app.watchMu.Unlock()

	if app.watchers == 0 {
		return
	}

	app.watchers--
	if app.watchers != 0 {
		return
	}

	close(app.pollStop)
	app.pollStop = nil
	app.pollDone = nil

	slog.Debug("player: no browser is watching any more, stopping the speaker status poll")
}

// runStatusPoll is the status poll for one watched period. It first brings
// every speaker up to date (unless its cache is still fresh), so a page that
// just opened is not left on a stale snapshot until the first tick.
func (app *WebApp) runStatusPoll(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)

	app.refreshAllDeviceStatuses(app.cacheTTL())

	ticker := time.NewTicker(app.pollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			app.refreshAllDeviceStatuses(0)
		}
	}
}

// refreshAllDeviceStatuses starts a shared refresh for every registered
// speaker whose cache is older than maxAge (maxAge <= 0: all of them). It
// does not wait: each speaker's round runs on its own, so an unreachable one
// never delays the rest or the next tick.
func (app *WebApp) refreshAllDeviceStatuses(maxAge time.Duration) {
	for _, entry := range app.DeviceSnapshot() {
		app.refreshDeviceStatusShared(entry.ID, entry.Device, maxAge)
	}
}

// refreshDeviceStatusShared runs a full status round for conn, or joins the
// one already in flight, unless its cache is younger than maxAge. It returns
// a channel closed when the round ends, or nil when there is nothing to wait
// for (fresh cache, no client, device removed).
func (app *WebApp) refreshDeviceStatusShared(
	deviceID string,
	conn *webtypes.DeviceConnection,
	maxAge time.Duration,
) <-chan struct{} {
	if conn == nil || conn.Client == nil {
		return nil
	}

	select {
	case <-conn.Done():
		return nil
	default:
	}

	done, lead := conn.BeginSharedStatusRefresh(time.Now(), maxAge)
	if lead {
		go func() {
			defer conn.EndSharedStatusRefresh()

			app.UpdateDeviceStatus(deviceID, conn)
		}()
	}

	return done
}

// refreshStaleDeviceStatuses is the on-demand refresh for REST endpoints that
// answer from the cache. It refreshes each entry whose cache is older than
// the TTL (sharing a round already in flight) and waits, up to
// statusRefreshWait, for those rounds to end. A speaker already classified
// offline is refreshed but not waited for: its round would most likely run
// into the client timeout, and the caller is better served by the cached
// "offline" now than by the same answer seconds later.
func (app *WebApp) refreshStaleDeviceStatuses(entries []DeviceEntry) {
	pending := make([]<-chan struct{}, 0, len(entries))

	for _, entry := range entries {
		done := app.refreshDeviceStatusShared(entry.ID, entry.Device, app.cacheTTL())
		if done == nil {
			continue
		}

		if status := entry.Device.Status(); status != nil && status.Connectivity == webtypes.ConnectivityOffline {
			continue
		}

		pending = append(pending, done)
	}

	if len(pending) == 0 {
		return
	}

	timer := time.NewTimer(statusRefreshWait)
	defer timer.Stop()

	for _, done := range pending {
		select {
		case <-done:
		case <-timer.C:
			return
		}
	}
}

// refreshStaleDeviceStatus is refreshStaleDeviceStatuses for one device.
func (app *WebApp) refreshStaleDeviceStatus(deviceID string, conn *webtypes.DeviceConnection) {
	app.refreshStaleDeviceStatuses([]DeviceEntry{{ID: deviceID, Device: conn}})
}
