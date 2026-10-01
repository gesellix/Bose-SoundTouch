package soundtouchweb

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

type registryReplacementResult struct {
	removed bool
	added   bool
}

func TestWithCurrentVolumeWriteFencesRegistryReplacement(t *testing.T) {
	speaker := newVolumeTestSpeaker(t, 10, "")
	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	speaker.onVolumePost = func() {
		close(writeEntered)
		<-releaseWrite
	}

	app := NewWebApp()
	conn := addVolumeTestDevice(app, "speaker", "ORIGINAL", "Office", "SoundTouch 20", speaker, 10, nil, nil)
	topology, current := conn.SnapshotGroupTopology()
	if !current {
		t.Fatal("initial group topology is not confirmed")
	}

	replacementSpeaker := newVolumeTestSpeaker(t, 70, "")
	replacement := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: replacementSpeaker.server.URL}),
		&models.DeviceInfo{DeviceID: "REPLACEMENT", Name: "Replacement", IPAddress: "speaker"},
	)

	type writeResult struct {
		current bool
		err     error
	}
	writeDone := make(chan writeResult, 1)
	go func() {
		var writeErr error
		wasCurrent := app.withCurrentVolumeWrite("speaker", conn, topology, nil, func() {
			writeErr = conn.Client.SetVolume(42)
		})
		writeDone <- writeResult{current: wasCurrent, err: writeErr}
	}()

	select {
	case <-writeEntered:
	case <-time.After(time.Second):
		t.Fatal("volume write did not reach the speaker")
	}

	replacementStarted := make(chan struct{})
	replacementDone := make(chan registryReplacementResult, 1)
	go func() {
		close(replacementStarted)
		removed := app.RemoveDevice("speaker")
		added := app.AddDevice("speaker", replacement)
		replacementDone <- registryReplacementResult{removed: removed, added: added}
	}()
	<-replacementStarted

	select {
	case <-replacementDone:
		t.Fatal("registry replacement completed before the accepted volume write")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseWrite)
	result := <-writeDone
	if !result.current || result.err != nil {
		t.Fatalf("volume write result = current %v, error %v", result.current, result.err)
	}
	select {
	case replacementResult := <-replacementDone:
		if !replacementResult.removed || !replacementResult.added {
			t.Fatalf("registry replacement = %+v", replacementResult)
		}
	case <-time.After(time.Second):
		t.Fatal("registry replacement remained blocked after the volume write")
	}
	if registered, ok := app.GetDevice("speaker"); !ok || registered != replacement {
		t.Fatalf("registered device = %p, want replacement %p", registered, replacement)
	}
	if _, posts, _ := speaker.operations(); fmt.Sprint(posts) != "[42]" {
		t.Fatalf("original speaker posts = %v", posts)
	}
	if _, posts, gets := replacementSpeaker.operations(); len(posts) != 0 || gets != 0 {
		t.Fatalf("replacement speaker performed I/O: posts %v gets %d", posts, gets)
	}
}

func TestCurrentVolumeReadbackMergeFencesRegistryReplacement(t *testing.T) {
	speaker := newVolumeTestSpeaker(t, 10, "")
	app := NewWebApp()
	conn := addVolumeTestDevice(app, "speaker", "ORIGINAL", "Office", "SoundTouch 20", speaker, 10, nil, nil)
	topology, current := conn.SnapshotGroupTopology()
	if !current {
		t.Fatal("initial group topology is not confirmed")
	}
	generation := conn.BeginVolumeRefresh()
	mergeEntered := make(chan struct{})
	releaseMerge := make(chan struct{})
	mergeDone := make(chan bool, 1)
	go func() {
		mergeDone <- app.withCurrentVolumeReadbackMerge(
			"speaker",
			conn,
			topology,
			nil,
			func() bool {
				close(mergeEntered)
				<-releaseMerge

				return conn.ApplyPolledVolume(
					generation,
					&models.Volume{TargetVolume: 55, ActualVolume: 55},
				)
			},
		)
	}()

	select {
	case <-mergeEntered:
	case <-time.After(time.Second):
		t.Fatal("readback did not pass its current-target check")
	}

	replacementSpeaker := newVolumeTestSpeaker(t, 70, "")
	replacement := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: replacementSpeaker.server.URL}),
		&models.DeviceInfo{DeviceID: "REPLACEMENT", Name: "Replacement", IPAddress: "speaker"},
	)
	replacementStarted := make(chan struct{})
	replacementDone := make(chan registryReplacementResult, 1)
	go func() {
		close(replacementStarted)
		removed := app.RemoveDevice("speaker")
		added := app.AddDevice("speaker", replacement)
		replacementDone <- registryReplacementResult{removed: removed, added: added}
	}()
	<-replacementStarted

	select {
	case <-replacementDone:
		t.Fatal("registry replacement completed during the accepted cache merge")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseMerge)
	if applied := <-mergeDone; !applied {
		t.Fatal("current cache merge was rejected")
	}
	select {
	case replacementResult := <-replacementDone:
		if !replacementResult.removed || !replacementResult.added {
			t.Fatalf("registry replacement = %+v", replacementResult)
		}
	case <-time.After(time.Second):
		t.Fatal("registry replacement remained blocked after the cache merge")
	}
	if volume := conn.Status().Volume; volume == nil || volume.ActualVolume != 55 {
		t.Fatalf("accepted connection cache = %+v, want merged volume 55", volume)
	}
	if registered, ok := app.GetDevice("speaker"); !ok || registered != replacement {
		t.Fatalf("registered device = %p, want replacement %p", registered, replacement)
	}
}

func TestApplyZoneVolumeRejectsProjectedHardwareReplacementAndPreservesValidMember(t *testing.T) {
	zone, _ := testZone()
	masterSpeaker := newVolumeTestSpeaker(t, 20, "")
	replacementSpeaker := newVolumeTestSpeaker(t, 80, "")

	app := NewWebApp()
	addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Kitchen", "SoundTouch 30", masterSpeaker, 20, nil, zone)
	addVolumeTestDevice(app, "192.0.2.20", "REPLACEMENT", "Other", "SoundTouch 20", replacementSpeaker, 80, nil, nil)

	result, readable := app.applyZoneVolume(zone, nil, 30)
	if !readable {
		t.Fatal("valid zone master was not readable")
	}
	if !result.Partial || result.Baseline != 20 || result.Delta != 10 {
		t.Fatalf("zone result = %+v", result)
	}
	if _, posts, gets := masterSpeaker.operations(); fmt.Sprint(posts) != "[30]" || gets != 2 {
		t.Fatalf("master operations = posts %v gets %d", posts, gets)
	}
	if _, posts, gets := replacementSpeaker.operations(); len(posts) != 0 || gets != 0 {
		t.Fatalf("replacement speaker performed I/O: posts %v gets %d", posts, gets)
	}

	var rejected *zoneVolumeMemberResult
	for index := range result.Members {
		if strings.TrimSpace(result.Members[index].DeviceID) == "MEMBER" {
			rejected = &result.Members[index]
			break
		}
	}
	if rejected == nil || rejected.Before != nil || rejected.Target != nil || rejected.Error != "speaker unavailable" {
		t.Fatalf("replaced member result = %+v", rejected)
	}
}
