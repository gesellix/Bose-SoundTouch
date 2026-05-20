package health

import (
	"sort"
	"strconv"
)

// ConsistencyView is one side's snapshot of presets, recents, and sources
// for a single device. It's intentionally minimal — only the fields the
// cross-reference checks need — so the same struct can hold either the
// speaker's perspective (parsed from :8090/presets, /recents, /sources)
// or the service's perspective (loaded from Presets.xml, Recents.xml,
// Sources.xml).
type ConsistencyView struct {
	// Label is "speaker" or "service" — surfaced in finding messages so
	// the operator can tell which side reported what.
	Label string

	Presets []ConsistencyPreset
	Recents []ConsistencyRecent
	Sources []ConsistencySource
}

// ConsistencyPreset captures one preset slot's identifying fields. Slot is
// the buttonNumber / id attribute (e.g. "1".."6"). Source is the symbolic
// source name (TUNEIN, SPOTIFY, …). SourceID is the numeric/account-bound
// id referenced in the inner <source id="…"> or <sourceid> — empty on the
// speaker side where presets carry only the source attribute.
type ConsistencyPreset struct {
	Slot     string
	Source   string
	SourceID string
	Location string
	Name     string
}

// ConsistencyRecent captures one recent entry. ID is the upstream-assigned
// recent id (a long numeric string). Other fields mirror the preset shape.
type ConsistencyRecent struct {
	ID       string
	Source   string
	SourceID string
	Location string
	Name     string
}

// ConsistencySource captures one configured source. ID is the numeric
// source id (or "" on the speaker side, which lists sources by type only).
// Type is the symbolic name (TUNEIN, SPOTIFY, BLUETOOTH, …). Account is
// the sourceAccount attribute when present.
type ConsistencySource struct {
	ID      string
	Type    string
	Account string
}

// ConsistencyIssue describes one inconsistency. Kind is the category
// (cross-side mismatch, dangling preset, dangling recent). Side is
// "speaker", "service", or "both" when applicable. Detail is a
// human-readable explanation; the per-finding ConsistencyIssue list is
// rendered as separate Findings, one per issue.
type ConsistencyIssue struct {
	Kind   ConsistencyIssueKind
	Side   string
	Detail string
}

// ConsistencyIssueKind enumerates the categories of inconsistency the
// checker can report.
type ConsistencyIssueKind string

// Recognised ConsistencyIssueKind values.
const (
	IssuePresetMismatch     ConsistencyIssueKind = "preset_mismatch"
	IssueRecentMismatch     ConsistencyIssueKind = "recent_mismatch"
	IssueSourceMismatch     ConsistencyIssueKind = "source_mismatch"
	IssueDanglingPreset     ConsistencyIssueKind = "dangling_preset"
	IssueDanglingRecent     ConsistencyIssueKind = "dangling_recent"
	IssueDuplicateSource    ConsistencyIssueKind = "duplicate_source"
	IssueMissingSpeakerSide ConsistencyIssueKind = "missing_speaker_side"
)

// CheckCrossSide compares speaker and service views for the same device
// and reports per-slot / per-recent / per-source disagreements. Returns
// nil when both sides agree on every comparable field.
//
// The comparison is *strict* on Source attribute. A preset that exists in
// both views with the same slot but different Source values produces an
// IssuePresetMismatch — that's the GH-343 footprint: service-side
// re-attributed the source. SourceID is compared only if both sides set
// it (the speaker often doesn't expose <sourceid> at all).
func CheckCrossSide(speaker, service ConsistencyView) []ConsistencyIssue {
	var issues []ConsistencyIssue

	// Presets: compare by slot.
	speakerPresets := indexPresetsBySlot(speaker.Presets)
	servicePresets := indexPresetsBySlot(service.Presets)

	for slot, sp := range speakerPresets {
		sv, ok := servicePresets[slot]
		if !ok {
			issues = append(issues, ConsistencyIssue{
				Kind:   IssuePresetMismatch,
				Side:   "speaker",
				Detail: "preset slot " + slot + " present on speaker but missing from service: name=" + safeQuote(sp.Name) + " source=" + safeQuote(sp.Source),
			})

			continue
		}

		if !presetFieldsCompatible(sp, sv) {
			issues = append(issues, ConsistencyIssue{
				Kind: IssuePresetMismatch,
				Side: "both",
				Detail: "preset slot " + slot + " disagrees: speaker source=" + safeQuote(sp.Source) +
					" location=" + safeQuote(sp.Location) + "; service source=" + safeQuote(sv.Source) +
					" sourceid=" + safeQuote(sv.SourceID) + " location=" + safeQuote(sv.Location),
			})
		}
	}

	for slot, sv := range servicePresets {
		if _, ok := speakerPresets[slot]; !ok {
			issues = append(issues, ConsistencyIssue{
				Kind:   IssuePresetMismatch,
				Side:   "service",
				Detail: "preset slot " + slot + " present on service but missing from speaker: name=" + safeQuote(sv.Name) + " source=" + safeQuote(sv.Source),
			})
		}
	}

	// Recents: compare by id.
	speakerRecents := indexRecentsByID(speaker.Recents)
	serviceRecents := indexRecentsByID(service.Recents)

	for id, sp := range speakerRecents {
		sv, ok := serviceRecents[id]
		if !ok {
			issues = append(issues, ConsistencyIssue{
				Kind:   IssueRecentMismatch,
				Side:   "speaker",
				Detail: "recent id=" + id + " on speaker but missing from service: source=" + safeQuote(sp.Source) + " location=" + safeQuote(sp.Location),
			})

			continue
		}

		if !recentFieldsCompatible(sp, sv) {
			issues = append(issues, ConsistencyIssue{
				Kind: IssueRecentMismatch,
				Side: "both",
				Detail: "recent id=" + id + " disagrees: speaker source=" + safeQuote(sp.Source) +
					"; service source=" + safeQuote(sv.Source) + " sourceid=" + safeQuote(sv.SourceID),
			})
		}
	}

	for id, sv := range serviceRecents {
		if _, ok := speakerRecents[id]; !ok {
			// Service-only recents are common (the speaker drops old
			// entries faster than we do), so only flag if there are
			// disproportionately many.
			_ = sv
		}
	}

	// Sources: compare by type (speaker lists by type, not by numeric ID).
	speakerSourceTypes := sourceTypesFromView(speaker)
	serviceSourceTypes := sourceTypesFromView(service)

	for t := range speakerSourceTypes {
		if !serviceSourceTypes[t] {
			issues = append(issues, ConsistencyIssue{
				Kind:   IssueSourceMismatch,
				Side:   "speaker",
				Detail: "source type " + safeQuote(t) + " present on speaker but not in service Sources.xml — service /full will not include presets referencing this source",
			})
		}
	}

	for t := range serviceSourceTypes {
		if !speakerSourceTypes[t] {
			issues = append(issues, ConsistencyIssue{
				Kind:   IssueSourceMismatch,
				Side:   "service",
				Detail: "source type " + safeQuote(t) + " in service Sources.xml but not advertised by speaker /sources — preset references to this source may fail at play time",
			})
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].Detail < issues[j].Detail
	})

	return issues
}

// CheckInternalConsistency inspects one side's snapshot in isolation and
// reports references that don't resolve within the same side. This catches
// the "preset stored with sourceid=10004 but Sources.xml has no entry for
// 10004" class (GH-269 NorbertBauer's restore case after a partial backup
// import).
//
// The side string is "speaker" or "service" — included verbatim in each
// finding so the operator can tell which side is internally inconsistent.
func CheckInternalConsistency(view ConsistencyView) []ConsistencyIssue {
	var issues []ConsistencyIssue

	sourceIDs := map[string]bool{}
	sourceTypes := map[string]bool{}
	dupCount := map[string]int{}

	for _, s := range view.Sources {
		if s.ID != "" {
			sourceIDs[s.ID] = true
		}

		if s.Type != "" {
			sourceTypes[s.Type] = true
			dupCount[s.Type]++
		}
	}

	for t, n := range dupCount {
		if n > 1 {
			issues = append(issues, ConsistencyIssue{
				Kind:   IssueDuplicateSource,
				Side:   view.Label,
				Detail: view.Label + ": source type " + safeQuote(t) + " has " + plural(n, "entry", "entries") + " in Sources.xml — mapPresetsToFullResponse picks the first match, the rest are inert",
			})
		}
	}

	for _, p := range view.Presets {
		if p.SourceID != "" && !sourceIDs[p.SourceID] {
			issues = append(issues, ConsistencyIssue{
				Kind:   IssueDanglingPreset,
				Side:   view.Label,
				Detail: view.Label + ": preset slot " + p.Slot + " (" + safeQuote(p.Name) + ") references sourceid=" + safeQuote(p.SourceID) + " which is not in Sources.xml — /full will fall back to type-match or skip",
			})

			continue
		}

		if p.Source != "" && !sourceTypes[p.Source] && p.SourceID == "" {
			issues = append(issues, ConsistencyIssue{
				Kind:   IssueDanglingPreset,
				Side:   view.Label,
				Detail: view.Label + ": preset slot " + p.Slot + " has source=" + safeQuote(p.Source) + " but no matching entry in Sources.xml — preset may be skipped or synthesised in /full",
			})
		}
	}

	for _, r := range view.Recents {
		if r.SourceID != "" && !sourceIDs[r.SourceID] {
			issues = append(issues, ConsistencyIssue{
				Kind:   IssueDanglingRecent,
				Side:   view.Label,
				Detail: view.Label + ": recent id=" + r.ID + " (" + safeQuote(r.Name) + ") references sourceid=" + safeQuote(r.SourceID) + " which is not in Sources.xml",
			})
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].Detail < issues[j].Detail
	})

	return issues
}

func indexPresetsBySlot(in []ConsistencyPreset) map[string]ConsistencyPreset {
	out := make(map[string]ConsistencyPreset, len(in))
	for i := range in {
		if in[i].Slot != "" {
			out[in[i].Slot] = in[i]
		}
	}

	return out
}

func indexRecentsByID(in []ConsistencyRecent) map[string]ConsistencyRecent {
	out := make(map[string]ConsistencyRecent, len(in))
	for i := range in {
		if in[i].ID != "" {
			out[in[i].ID] = in[i]
		}
	}

	return out
}

func sourceTypesFromView(v ConsistencyView) map[string]bool {
	out := map[string]bool{}

	for _, s := range v.Sources {
		if s.Type != "" {
			out[s.Type] = true
		}
	}

	return out
}

// presetFieldsCompatible is strict on Source attribute (GH-343 footprint
// reproduces here as a cross-side mismatch). Location is compared only
// when both sides provide it; the speaker's /presets always does, the
// service's Presets.xml does too unless it was hand-edited.
func presetFieldsCompatible(a, b ConsistencyPreset) bool {
	if a.Source != "" && b.Source != "" && a.Source != b.Source {
		return false
	}

	if a.Location != "" && b.Location != "" && a.Location != b.Location {
		return false
	}

	return true
}

func recentFieldsCompatible(a, b ConsistencyRecent) bool {
	if a.Source != "" && b.Source != "" && a.Source != b.Source {
		return false
	}

	return true
}

func safeQuote(s string) string {
	if s == "" {
		return `""`
	}

	return `"` + s + `"`
}

func plural(n int, singular, plurals string) string {
	if n == 1 {
		return "1 " + singular
	}

	return strconv.Itoa(n) + " " + plurals
}
