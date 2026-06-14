package marge

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestRecent_StoredMusicKeepsAccount is a regression test for the recents-replay
// bug: a STORED_MUSIC media server's account ("<UDN>/0") is persisted in
// SourceKey.Account, not Username (which does not round-trip). The recent
// <source> builders emitted Username verbatim, producing an empty <username>;
// the speaker then fell back to the provider id (e.g. "7") as the account and
// replaying the recent failed with INVALID_SOURCE.
//
// The speaker registers a recent by sourceid only (no account in the POST), so
// the served recent's account must come from the matched source. This asserts
// the served <source> carries the real UDN, and never the bare provider id.
func TestRecent_StoredMusicKeepsAccount(t *testing.T) {
	tmp, err := os.MkdirTemp("", "recent-storedmusic-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmp) }()

	ds := datastore.NewDataStore(tmp)
	account := "6919733"
	device := "A81B6A536A98"

	if mkErr := os.MkdirAll(ds.AccountDeviceDir(account, device), 0o755); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}

	const udn = "4d696e69-444c-164e-9d41-72ecda78e4c1/0"

	sm := strconv.Itoa(constants.StoredMusicProviderID)

	srcID, err := AddSource(ds, account, udn, sm, "", "", "AfterTouch Test Library")
	if err != nil {
		t.Fatalf("add source: %v", err)
	}

	// The speaker POSTs a recent referencing the source by id only (matches the
	// captured Bose_Lisa payload: no <source> account/username).
	recXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" ?><recent>`+
		`<lastplayedat>2026-06-14T16:35:30+00:00</lastplayedat>`+
		`<sourceid>%s</sourceid>`+
		`<name>02 - The Raven</name>`+
		`<location>1$4$1 TRACK</location>`+
		`<contentItemType></contentItemType></recent>`, srcID)

	postResp, err := AddRecent(ds, account, device, []byte(recXML))
	if err != nil {
		t.Fatalf("add recent: %v", err)
	}

	// 1. The POST response (formatRecentResponse) must carry the real account.
	assertAccount(t, "AddRecent response", string(postResp), udn)

	// 2. The served recents list (RecentsToXML) must carry it too.
	served, err := RecentsToXML(ds, account, device)
	if err != nil {
		t.Fatalf("RecentsToXML: %v", err)
	}

	assertAccount(t, "RecentsToXML", string(served), udn)
}

func assertAccount(t *testing.T, what, xml, udn string) {
	t.Helper()

	if !strings.Contains(xml, "<username>"+udn+"</username>") {
		t.Errorf("%s: expected <username>%s</username>; got:\n%s", what, udn, xml)
	}

	if strings.Contains(xml, "<username></username>") || strings.Contains(xml, "<username/>") {
		t.Errorf("%s: STORED_MUSIC source has an empty <username> (account lost):\n%s", what, xml)
	}

	if strings.Contains(xml, "<username>7</username>") {
		t.Errorf("%s: STORED_MUSIC username is the bare provider id, not the account:\n%s", what, xml)
	}
}
