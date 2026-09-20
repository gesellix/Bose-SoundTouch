// Package catalog keeps the preset and source entries AfterTouch has seen for
// a service, so a slot that gets emptied can be picked again instead of being
// recovered from hand-edited XML (issue 754).
//
// It is the same shape as recents (a source plus a content item, plus name and
// artwork), with three differences that are the whole point: we manage it, the
// speaker never trims it, and dropping the oldest entry is our decision rather
// than a side effect of the speaker's own bookkeeping.
//
// The catalog says what we have seen, not when a given slot changed. A
// per-slot history would need change events per device, which is a different
// structure; it is deliberately out of scope until someone needs it.
package catalog

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// SeenGranularity is how much a sighting's timestamp has to move before it is
// worth rewriting the catalog for. A speaker re-posts its whole preset list on
// every sync, so without this every one of those would rewrite catalog.json
// with nothing changed but a clock reading -- write amplification on a
// speaker's flash volume, for no information gained.
const SeenGranularity = time.Hour

// DefaultSize is the entry cap when nothing is configured.
//
// Measured on a real install: 30 entries came to 13.5 KB of pretty-printed
// JSON, about 450 bytes each, the largest field being a 214-character Spotify
// location. So 100 entries costs roughly 45 KB -- nothing on a normal host,
// and about a third of that on a speaker's UBIFS /mnt/nv, which compresses
// this kind of text well.
//
// The first default of 30 was set before anything had been measured, and it
// turned out to be too small to keep the promise the feature rests on: a
// household with several speakers filled it with recents alone, so an emptied
// slot's station could already be gone from the pick list. The cost of the
// file is not what should decide that; write frequency is, and the
// SeenGranularity debounce is what handles it.
const DefaultSize = 100

// Origin records which write the entry was learned from. It is informational:
// a preset entry is content someone deliberately kept, a recent one is content
// that merely played, and the editor can say so.
const (
	OriginPreset = "preset"
	OriginRecent = "recent"
)

// Entry is one thing we have seen playing or stored in a slot.
type Entry struct {
	Source        string `json:"source"`
	SourceAccount string `json:"source_account,omitempty"`
	Location      string `json:"location,omitempty"`
	Type          string `json:"type,omitempty"`
	Name          string `json:"name,omitempty"`
	ContainerArt  string `json:"container_art,omitempty"`
	IsPresetable  string `json:"is_presetable,omitempty"`
	SourceID      string `json:"source_id,omitempty"`

	// Origin is the strongest origin seen for this entry: once something has
	// been a preset, a later recents sighting does not demote it.
	Origin string `json:"origin,omitempty"`
	// DeviceID is the device the entry was last seen on.
	DeviceID  string    `json:"device_id,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// normalizeAccount drops the placeholder the speaker writes when a source has
// no real account: recents echo the source name back as sourceAccount
// ("TUNEIN", "RADIO_BROWSER") while presets for the same station leave it
// empty. Comparing the raw values would file one station twice. This is the
// same normalisation the player's recents-artwork matching uses.
func normalizeAccount(source, account string) string {
	if account == source {
		return ""
	}

	return account
}

// Identity keys an entry by what actually identifies the content: source, the
// normalised account, and location. Item names are deliberately excluded --
// the same name can belong to two different stations on two sources.
//
// The parts are quoted rather than joined on a separator, so that a location
// containing the separator cannot collide with a different entry. The exact
// format is internal: it is never persisted or sent anywhere, and the player
// builds its own key for its own lookups.
func Identity(source, sourceAccount, location string) string {
	return strings.Join([]string{
		strconv.Quote(source),
		strconv.Quote(normalizeAccount(source, sourceAccount)),
		strconv.Quote(location),
	}, ":")
}

// Identity returns e's identity key.
func (e Entry) Identity() string {
	return Identity(e.Source, e.SourceAccount, e.Location)
}

// Catalog is the persisted set of entries for one service.
type Catalog struct {
	Entries []Entry `json:"entries"`
}

// Record files e, merging it into an existing entry with the same identity.
// The catalog is kept at most size entries, newest sighting first; a size of
// zero or less disables it and drops everything already held.
//
// It reports whether the catalog changed, so a caller can skip the write.
func (c *Catalog) Record(e Entry, size int) bool {
	if size <= 0 {
		changed := len(c.Entries) > 0
		c.Entries = nil

		return changed
	}

	if e.Source == "" || e.Location == "" {
		// Nothing we could pick again later.
		return false
	}

	if e.LastSeen.IsZero() {
		e.LastSeen = time.Now().UTC()
	}

	if e.FirstSeen.IsZero() {
		e.FirstSeen = e.LastSeen
	}

	if existing := c.indexOf(e.Identity()); existing >= 0 {
		old := c.Entries[existing]
		merged := merge(old, e)

		// Nothing new: same content, same metadata, and a timestamp that has
		// not moved far enough to be worth a write. Leaving the stored entry
		// untouched is what keeps a speaker's repeated syncs from rewriting
		// the file.
		if sameEntry(old, merged) && merged.LastSeen.Sub(old.LastSeen) < SeenGranularity {
			return false
		}

		c.Entries[existing] = merged
	} else {
		c.Entries = append(c.Entries, e)
	}

	c.sortAndTrim(size)

	return true
}

// sameEntry compares two entries on everything except LastSeen.
func sameEntry(a, b Entry) bool {
	return a.Source == b.Source &&
		a.SourceAccount == b.SourceAccount &&
		a.Location == b.Location &&
		a.Type == b.Type &&
		a.Name == b.Name &&
		a.ContainerArt == b.ContainerArt &&
		a.IsPresetable == b.IsPresetable &&
		a.SourceID == b.SourceID &&
		a.Origin == b.Origin &&
		a.DeviceID == b.DeviceID &&
		a.FirstSeen.Equal(b.FirstSeen)
}

// Trim applies a (possibly reduced) size limit without recording anything, so
// lowering the configured cap takes effect on the next write rather than only
// once the catalog happens to overflow again.
func (c *Catalog) Trim(size int) bool {
	before := len(c.Entries)

	if size <= 0 {
		c.Entries = nil

		return before > 0
	}

	c.sortAndTrim(size)

	return len(c.Entries) != before
}

// List returns the entries, newest sighting first.
func (c *Catalog) List() []Entry {
	out := make([]Entry, len(c.Entries))
	copy(out, c.Entries)
	sortByLastSeen(out)

	return out
}

func (c *Catalog) indexOf(identity string) int {
	for i := range c.Entries {
		if c.Entries[i].Identity() == identity {
			return i
		}
	}

	return -1
}

func (c *Catalog) sortAndTrim(size int) {
	sortByLastSeen(c.Entries)

	if len(c.Entries) <= size {
		return
	}

	// Evict recents before presets, oldest first within each.
	//
	// Plain oldest-first eviction is wrong for a pick list: a preset carries
	// the speaker's own createdOn, which for a station someone has kept for
	// years is older than anything that merely played last week. The entry
	// most worth offering back would be the first to go, and a slot cleared
	// afterwards could not be refilled from the list -- which is the promise
	// the whole feature rests on.
	//
	// Display order is unaffected: the surviving entries are re-sorted by
	// sighting afterwards.
	kept := make([]Entry, 0, size)
	presets := make([]Entry, 0, len(c.Entries))

	for i := range c.Entries {
		if c.Entries[i].Origin == OriginPreset {
			presets = append(presets, c.Entries[i])
		}
	}

	kept = append(kept, presets...)
	if len(kept) > size {
		kept = kept[:size]
	}

	for i := range c.Entries {
		if len(kept) >= size {
			break
		}

		if c.Entries[i].Origin != OriginPreset {
			kept = append(kept, c.Entries[i])
		}
	}

	sortByLastSeen(kept)
	c.Entries = kept
}

func sortByLastSeen(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].LastSeen.After(entries[j].LastSeen)
	})
}

// merge folds a new sighting into what we already knew. Fields that the new
// sighting leaves empty keep their old value: recents arrive without artwork
// (the speaker never puts any there), and losing the art a preset sighting
// contributed would make the pick list worse the moment a station plays.
func merge(old, seen Entry) Entry {
	merged := seen
	merged.FirstSeen = earliest(old.FirstSeen, seen.FirstSeen)

	if merged.LastSeen.Before(old.LastSeen) {
		merged.LastSeen = old.LastSeen
	}

	merged.Name = firstNonEmpty(seen.Name, old.Name)
	merged.ContainerArt = firstNonEmpty(seen.ContainerArt, old.ContainerArt)
	merged.Type = firstNonEmpty(seen.Type, old.Type)
	merged.SourceID = firstNonEmpty(seen.SourceID, old.SourceID)
	merged.IsPresetable = firstNonEmpty(seen.IsPresetable, old.IsPresetable)
	merged.DeviceID = firstNonEmpty(seen.DeviceID, old.DeviceID)

	// A preset sighting outranks a recents one: it is content someone chose
	// to keep, and that does not stop being true because it later played.
	merged.Origin = seen.Origin
	if old.Origin == OriginPreset {
		merged.Origin = OriginPreset
	}

	return merged
}

// earliest returns the older of two timestamps, ignoring zero values.
func earliest(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	case b.Before(a):
		return b
	default:
		return a
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}

	return ""
}
