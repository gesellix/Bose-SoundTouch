package soundtouchweb

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/dlna"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

const (
	// storedMusicArtBudget bounds the whole art lookup. It runs inside a
	// user-facing request (saving a preset), so a slow or half-answering media
	// server must cost a moment, not the request.
	storedMusicArtBudget = 4 * time.Second

	// mediaServerCacheTTL is how long a resolved ContentDirectory URL is
	// reused. Servers keep theirs across restarts in practice, and a stale
	// entry only costs one failed lookup, after which the art is simply
	// missing.
	mediaServerCacheTTL = 10 * time.Minute
)

// cachedMediaServer is one resolved media server plus the time it was
// resolved. A miss is cached too (ok=false), so a server that is gone or has
// no ContentDirectory is not re-resolved on every save.
type cachedMediaServer struct {
	server   discovery.MediaServer
	ok       bool
	resolved time.Time
}

// storedMusicObjectID maps a speaker-native STORED_MUSIC location to the DLNA
// object ID it came from.
//
// They are the same string, except that the speaker appends " TRACK" to a
// playable item's ID. Measured against a SoundTouch 10 (FW 27.0.6) on two
// servers, which agreed: MiniDLNA album "1$6$7$2" and its track
// "1$6$7$2$5 TRACK"; a FRITZ!Box container "4:cont2:150:0:0:" and its track
// "5:audio5:part13:3171:5 TRACK". Containers carry no suffix.
func storedMusicObjectID(location string) string {
	return strings.TrimSuffix(location, " TRACK")
}

// storedMusicArtURL returns the album art for one STORED_MUSIC location, or
// "" when it cannot be determined.
//
// The speaker cannot answer this: its /navigate response carries no artwork
// (verified on hardware), which is why a folder saved as a preset used to
// show a generic icon while the same folder playing showed its cover, the
// latter coming from the speaker's own metadata at play time. So we ask the
// media server, whose DIDL-Lite does carry albumArtURI on both album
// containers and tracks.
//
// Every failure path returns "" rather than an error: art is decoration, and
// nothing here may prevent a preset from being stored.
func (app *WebApp) storedMusicArtURL(ctx context.Context, device *webtypes.DeviceConnection, account, location string) string {
	objectID := storedMusicObjectID(location)
	if objectID == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, storedMusicArtBudget)
	defer cancel()

	server, ok := app.mediaServerForAccount(ctx, device, account)
	if !ok {
		return ""
	}

	result, err := dlna.Metadata(ctx, server, objectID)
	if err != nil {
		// The object ID comes from a request body and the error can quote a
		// media server's response, so both are sanitised (CodeQL
		// go/log-injection, same reason as logPlaybackRequest).
		slog.Debug("library art: metadata lookup failed",
			"object", sanitizeLog(objectID), "err", sanitizeLog(err.Error()))

		return ""
	}

	// BrowseMetadata answers with exactly one object, which is a container for
	// a folder and an item for a track.
	for _, c := range result.Containers {
		if c.AlbumArtURL != "" {
			return c.AlbumArtURL
		}
	}

	for i := range result.Items {
		if art := result.Items[i].AlbumArtURL; art != "" {
			return art
		}
	}

	return ""
}

// mediaServerForAccount resolves the ContentDirectory of the server behind a
// STORED_MUSIC sourceAccount ("<udn>/0").
//
// The speaker itself is the directory: its /listMediaServers reports each
// server's UDN together with the URL of its UPnP description, so no SSDP
// sweep is needed. That also keeps the lookup working when the service host
// and the speaker do not see the same network segment, which is the whole
// reason HandleDiscoverLibraryServers queries both paths.
func (app *WebApp) mediaServerForAccount(ctx context.Context, device *webtypes.DeviceConnection, account string) (discovery.MediaServer, bool) {
	udn := normalizeUDN(strings.TrimSuffix(account, "/0"))
	if udn == "" || device == nil || device.Client == nil {
		return discovery.MediaServer{}, false
	}

	app.mediaServersMu.Lock()
	defer app.mediaServersMu.Unlock()

	if cached, found := app.mediaServers[udn]; found && time.Since(cached.resolved) < mediaServerCacheTTL {
		return cached.server, cached.ok
	}

	resolved := cachedMediaServer{resolved: time.Now()}

	if location := app.mediaServerLocation(device, udn); location != "" {
		server, ok, err := discovery.MediaServerAt(ctx, location)
		if err != nil {
			slog.Debug("library art: media server description fetch failed", "err", sanitizeLog(err.Error()))
		}

		resolved.server = server
		resolved.ok = ok && err == nil
	}

	if app.mediaServers == nil {
		app.mediaServers = map[string]cachedMediaServer{}
	}

	app.mediaServers[udn] = resolved

	return resolved.server, resolved.ok
}

// mediaServerLocation asks the speaker where a given media server's UPnP
// description lives. An empty return means the speaker does not know that
// server (or the call failed), which is not an error here: the caller then
// simply has no art to store.
func (app *WebApp) mediaServerLocation(device *webtypes.DeviceConnection, udn string) string {
	resp, err := device.Client.ListMediaServers()
	if err != nil || resp == nil {
		if err != nil {
			slog.Debug("library art: listMediaServers failed", "err", sanitizeLog(err.Error()))
		}

		return ""
	}

	for i := range resp.MediaServers {
		s := &resp.MediaServers[i]
		if normalizeUDN(s.ID) == udn {
			return s.Location
		}
	}

	return ""
}
