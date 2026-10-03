package client

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestTargetBalanceRejectsWrongDeviceWithoutWrite(t *testing.T) {
	f := newFakeSpeakerWS(t)
	wsClient := f.connect(t)
	var fenceCalls atomic.Int32

	_, err := wsClient.SetBalanceWithBoundsForTarget(
		context.Background(), 2, acousticBalanceBounds(), "OTHER-DEVICE",
		func(send func() error) error {
			fenceCalls.Add(1)
			return send()
		},
	)
	if err == nil || !strings.Contains(err.Error(), "does not match target") {
		t.Fatalf("SetBalanceWithBoundsForTarget error = %v, want target mismatch", err)
	}
	if got := fenceCalls.Load(); got != 0 {
		t.Fatalf("send fence called %d times for wrong physical device, want 0", got)
	}
	assertNoAcousticWrites(t, f)
}

func TestTargetBalanceDeniedFenceDoesNotWrite(t *testing.T) {
	f := newFakeSpeakerWS(t)
	wsClient := f.connect(t)
	denied := errors.New("pair token changed")

	_, err := wsClient.SetBalanceWithBoundsForTarget(
		context.Background(), 2, acousticBalanceBounds(), fakeSpeakerDeviceID,
		func(func() error) error { return denied },
	)
	if !errors.Is(err, denied) {
		t.Fatalf("SetBalanceWithBoundsForTarget error = %v, want %v", err, denied)
	}
	assertNoAcousticWrites(t, f)
}

func TestQueuedTargetBalanceRechecksFenceBeforeWrite(t *testing.T) {
	f := newFakeSpeakerWS(t)
	wsClient := f.connect(t)
	var valid atomic.Bool
	valid.Store(true)

	wsClient.writeMu.Lock()
	locked := true
	defer func() {
		if locked {
			wsClient.writeMu.Unlock()
		}
	}()

	result := startTargetBalanceWrite(context.Background(), wsClient, func(send func() error) error {
		if !valid.Load() {
			return errors.New("pair token changed")
		}
		return send()
	})
	waitForPendingAcousticRequest(t, wsClient)
	valid.Store(false)
	wsClient.writeMu.Unlock()
	locked = false

	if got := <-result; got.err == nil || !strings.Contains(got.err.Error(), "pair token changed") {
		t.Fatalf("queued write result = %+v, want invalidated fence error", got)
	}
	assertNoAcousticWrites(t, f)
}

func TestQueuedTargetBalanceRechecksTransportBeforeWrite(t *testing.T) {
	tests := []struct {
		name       string
		invalidate func(*testing.T, *WebSocketClient)
	}{
		{
			name: "transport closed",
			invalidate: func(t *testing.T, wsClient *WebSocketClient) {
				t.Helper()
				if err := wsClient.Disconnect(); err != nil {
					t.Fatalf("Disconnect: %v", err)
				}
			},
		},
		{
			name: "transport replaced on same client",
			invalidate: func(t *testing.T, wsClient *WebSocketClient) {
				t.Helper()
				wsClient.mu.Lock()
				admitted := wsClient.connection
				if admitted == nil {
					wsClient.mu.Unlock()
					t.Fatal("connected client has nil transport")
				}
				replacementCtx, replacementCancel := context.WithCancel(wsClient.ctx)
				wsClient.connection = &webSocketConnection{
					conn: admitted.conn, ctx: replacementCtx, cancel: replacementCancel,
				}
				wsClient.mu.Unlock()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFakeSpeakerWS(t)
			wsClient := f.connect(t)

			wsClient.writeMu.Lock()
			locked := true
			defer func() {
				if locked {
					wsClient.writeMu.Unlock()
				}
			}()

			result := startTargetBalanceWrite(context.Background(), wsClient, func(send func() error) error {
				return send()
			})
			waitForPendingAcousticRequest(t, wsClient)
			test.invalidate(t, wsClient)
			wsClient.writeMu.Unlock()
			locked = false

			if got := <-result; got.err == nil {
				t.Fatalf("queued write result = %+v, want stale transport error", got)
			}
			assertNoAcousticWrites(t, f)
		})
	}
}

func TestQueuedTargetBalanceHonorsContextCancellationBeforeFence(t *testing.T) {
	f := newFakeSpeakerWS(t)
	wsClient := f.connect(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var fenceCalls atomic.Int32

	wsClient.writeMu.Lock()
	locked := true
	defer func() {
		if locked {
			wsClient.writeMu.Unlock()
		}
	}()

	result := startTargetBalanceWrite(ctx, wsClient, func(send func() error) error {
		fenceCalls.Add(1)
		return send()
	})
	waitForPendingAcousticRequest(t, wsClient)
	cancel()
	wsClient.writeMu.Unlock()
	locked = false

	got := <-result
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("queued write error = %v, want context.Canceled", got.err)
	}
	if calls := fenceCalls.Load(); calls != 0 {
		t.Fatalf("send fence called %d times after queued context cancellation, want 0", calls)
	}
	assertNoAcousticWrites(t, f)
}

func TestTargetBalanceCancellationWhileSendWaitsForTransportLockDoesNotWrite(t *testing.T) {
	f := newFakeSpeakerWS(t)
	wsClient := f.connect(t)
	baseContext, cancel := context.WithCancel(context.Background())
	ctx := &transportLockCancellationContext{Context: baseContext, preTransportCheck: make(chan struct{})}
	defer cancel()

	wsClient.writeMu.Lock()
	writeLocked := true
	defer func() {
		if writeLocked {
			wsClient.writeMu.Unlock()
		}
	}()

	result := startTargetBalanceWrite(ctx, wsClient, func(send func() error) error {
		return send()
	})
	waitForPendingAcousticRequest(t, wsClient)

	wsClient.mu.Lock()
	transportLocked := true
	defer func() {
		if transportLocked {
			wsClient.mu.Unlock()
		}
	}()
	wsClient.writeMu.Unlock()
	writeLocked = false
	<-ctx.preTransportCheck

	cancel()
	wsClient.mu.Unlock()
	transportLocked = false

	got := <-result
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("write blocked on transport lock returned %v, want context.Canceled", got.err)
	}
	assertNoAcousticWrites(t, f)
}

func TestTargetBalanceFenceEndsBeforeReplyDispatch(t *testing.T) {
	f := newFakeSpeakerWS(t)
	f.reply = func(route, id, _ string) []string {
		return []string{
			`<updates deviceID="DEVICEID01"><balanceUpdated></balanceUpdated></updates>`,
			okResponse(route, id, balanceDocument(3, 3)),
		}
	}
	wsClient := f.connect(t)

	var fenceMu sync.Mutex
	replyCallbackAcquiredFence := make(chan struct{}, 1)
	wsClient.OnBalanceUpdated(func(*models.BalanceUpdatedEvent) {
		fenceMu.Lock()
		replyCallbackAcquiredFence <- struct{}{}
		fenceMu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	updated, err := wsClient.SetBalanceWithBoundsForTarget(
		ctx, 3, acousticBalanceBounds(), fakeSpeakerDeviceID,
		func(send func() error) error {
			fenceMu.Lock()
			defer fenceMu.Unlock()
			return send()
		},
	)
	if err != nil {
		t.Fatalf("SetBalanceWithBoundsForTarget: %v", err)
	}
	if updated.Target != 3 || updated.Actual != 3 || updated.DeviceID != fakeSpeakerDeviceID {
		t.Fatalf("echoed balance = %+v, want target and actual 3 for %s", updated, fakeSpeakerDeviceID)
	}
	select {
	case <-replyCallbackAcquiredFence:
	default:
		t.Fatal("event callback did not acquire fence mutex before the correlated reply completed")
	}

	requests := f.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("recorded %d requests, want exactly one balance write: %v", len(requests), requests)
	}
	for _, want := range []string{
		`deviceID="DEVICEID01"`,
		`url="balance"`,
		`method="POST"`,
		`<info mainNode="balanceSet" type="new"/>`,
		`<balance><targetBalance>3</targetBalance></balance>`,
	} {
		if !strings.Contains(requests[0], want) {
			t.Errorf("balance write missing %q in: %s", want, requests[0])
		}
	}
}

type targetBalanceResult struct {
	balance *models.Balance
	err     error
}

func startTargetBalanceWrite(
	ctx context.Context,
	wsClient *WebSocketClient,
	fence SendFence,
) <-chan targetBalanceResult {
	result := make(chan targetBalanceResult, 1)
	go func() {
		balance, err := wsClient.SetBalanceWithBoundsForTarget(
			ctx, 2, acousticBalanceBounds(), fakeSpeakerDeviceID, fence,
		)
		result <- targetBalanceResult{balance: balance, err: err}
	}()

	return result
}

func waitForPendingAcousticRequest(t *testing.T, wsClient *WebSocketClient) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		wsClient.pending.mu.Lock()
		pending := len(wsClient.pending.waiting)
		wsClient.pending.mu.Unlock()
		if pending != 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("target balance request did not reach the writer queue")
		}
		runtime.Gosched()
	}
}

func acousticBalanceBounds() *models.Balance {
	return &models.Balance{
		DeviceID: fakeSpeakerDeviceID, Available: true, Min: -7, Max: 7, Default: 0,
	}
}

func assertNoAcousticWrites(t *testing.T, f *fakeSpeakerWS) {
	t.Helper()

	if requests := f.recordedRequests(); len(requests) != 0 {
		t.Fatalf("recorded %d WebSocket requests, want zero: %v", len(requests), requests)
	}
}

// Capture the successful pre-lock check before releasing the test thread.
// Cancellation then cannot be caught by either earlier Err call, so rejecting
// the write requires the subsequent check under the transport lock.
type transportLockCancellationContext struct {
	context.Context
	checks            atomic.Int32
	preTransportCheck chan struct{}
}

func (c *transportLockCancellationContext) Err() error {
	err := c.Context.Err()
	if c.checks.Add(1) == 2 {
		close(c.preTransportCheck)
	}
	return err
}
