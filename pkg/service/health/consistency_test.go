package health

import (
	"strings"
	"testing"
)

func TestCheckInternalConsistency_DanglingPreset(t *testing.T) {
	view := ConsistencyView{
		Label: "service",
		Presets: []ConsistencyPreset{
			{Slot: "1", Source: "TUNEIN", SourceID: "9999999", Name: "Orphan"},
			{Slot: "2", Source: "TUNEIN", SourceID: "10004", Name: "Resolved"},
		},
		Sources: []ConsistencySource{
			{ID: "10004", Type: "TUNEIN"},
		},
	}

	got := CheckInternalConsistency(view)

	var sawOrphan bool

	for _, iss := range got {
		if iss.Kind == IssueDanglingPreset && strings.Contains(iss.Detail, "9999999") {
			sawOrphan = true
		}
	}

	if !sawOrphan {
		t.Errorf("expected dangling-preset finding for sourceid=9999999, got: %+v", got)
	}

	for _, iss := range got {
		if iss.Kind == IssueDanglingPreset && strings.Contains(iss.Detail, "10004") {
			t.Errorf("did not expect dangling finding for resolved preset; got %s", iss.Detail)
		}
	}
}

func TestCheckInternalConsistency_DanglingRecent(t *testing.T) {
	view := ConsistencyView{
		Label: "service",
		Recents: []ConsistencyRecent{
			{ID: "rec-1", Source: "SPOTIFY", SourceID: "missing"},
		},
		Sources: []ConsistencySource{
			{ID: "10004", Type: "TUNEIN"},
		},
	}

	got := CheckInternalConsistency(view)

	var sawDangling bool

	for _, iss := range got {
		if iss.Kind == IssueDanglingRecent && strings.Contains(iss.Detail, "rec-1") {
			sawDangling = true
		}
	}

	if !sawDangling {
		t.Errorf("expected dangling-recent finding, got: %+v", got)
	}
}

func TestCheckInternalConsistency_DuplicateSourceType(t *testing.T) {
	view := ConsistencyView{
		Label: "service",
		Sources: []ConsistencySource{
			{ID: "10004", Type: "TUNEIN"},
			{ID: "14774275", Type: "TUNEIN"},
		},
	}

	got := CheckInternalConsistency(view)

	var sawDup bool

	for _, iss := range got {
		if iss.Kind == IssueDuplicateSource && strings.Contains(iss.Detail, "TUNEIN") {
			sawDup = true
		}
	}

	if !sawDup {
		t.Errorf("expected duplicate-source finding, got: %+v", got)
	}
}

func TestCheckCrossSide_PresetSourceMismatch_GH343(t *testing.T) {
	// Speaker reports TUNEIN; service reports RADIOPLAYER — the
	// GH-343 footprint, where /full rebinding silently rewrote the
	// preset's source attribute.
	speaker := ConsistencyView{
		Label: "speaker",
		Presets: []ConsistencyPreset{
			{Slot: "1", Source: "TUNEIN", Location: "/v1/playback/station/s6634", Name: "MDR JUMP"},
		},
		Sources: []ConsistencySource{
			{Type: "TUNEIN"},
		},
	}

	service := ConsistencyView{
		Label: "service",
		Presets: []ConsistencyPreset{
			{Slot: "1", Source: "RADIOPLAYER", SourceID: "10004", Location: "/v1/playback/station/s6634", Name: "MDR JUMP"},
		},
		Sources: []ConsistencySource{
			{ID: "10004", Type: "RADIOPLAYER"},
		},
	}

	got := CheckCrossSide(speaker, service)

	var sawMismatch bool

	for _, iss := range got {
		if iss.Kind == IssuePresetMismatch && iss.Side == "both" {
			sawMismatch = true

			if !strings.Contains(iss.Detail, "TUNEIN") || !strings.Contains(iss.Detail, "RADIOPLAYER") {
				t.Errorf("expected detail to name both source attributions; got: %s", iss.Detail)
			}
		}
	}

	if !sawMismatch {
		t.Errorf("expected preset-mismatch (both) finding, got: %+v", got)
	}
}

func TestCheckCrossSide_PresetMissingOnOneSide(t *testing.T) {
	speaker := ConsistencyView{
		Label: "speaker",
		Presets: []ConsistencyPreset{
			{Slot: "1", Source: "TUNEIN"},
			{Slot: "3", Source: "TUNEIN"},
		},
	}

	service := ConsistencyView{
		Label: "service",
		Presets: []ConsistencyPreset{
			{Slot: "1", Source: "TUNEIN"},
		},
	}

	got := CheckCrossSide(speaker, service)

	var sawSpeakerOnly bool

	for _, iss := range got {
		if iss.Kind == IssuePresetMismatch && iss.Side == "speaker" && strings.Contains(iss.Detail, "slot 3") {
			sawSpeakerOnly = true
		}
	}

	if !sawSpeakerOnly {
		t.Errorf("expected speaker-only finding for slot 3, got: %+v", got)
	}
}

func TestCheckCrossSide_AgreesWhenIdentical(t *testing.T) {
	identical := func() ConsistencyView {
		return ConsistencyView{
			Presets: []ConsistencyPreset{
				{Slot: "1", Source: "TUNEIN", Location: "/v1/playback/station/s6634"},
			},
			Sources: []ConsistencySource{
				{Type: "TUNEIN"},
			},
		}
	}

	got := CheckCrossSide(identical(), identical())
	if len(got) != 0 {
		t.Errorf("expected no cross-side issues for identical views, got %d: %+v", len(got), got)
	}
}
