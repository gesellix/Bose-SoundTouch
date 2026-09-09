package client

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// balanceMainNode is the mainNode the firmware expects on a balance write.
// Taken from Stockholm, the app Bose ships on the speaker, and confirmed on
// hardware. A write without it is also accepted, but this matches what the
// speaker's own UI sends.
const balanceMainNode = "balanceSet"

// GetBalance reads the stereo pair's balance over the WebSocket.
//
// Prefer this over Client.GetBalance when a socket is already open: the reply
// is correlated to the request, and it does not race the HTTP read's brief lag
// behind a just-completed write.
//
// Either member of the pair answers, with the same value; an unpaired speaker
// answers Available=false.
func (ws *WebSocketClient) GetBalance(ctx context.Context) (*models.Balance, error) {
	body, err := ws.Request(ctx, "balance", "GET", "", RequestOptions{})
	if err != nil {
		return nil, err
	}

	return parseBalanceBody(body)
}

// SetBalance writes the stereo pair's balance over the WebSocket.
//
// This is the only way the write works. POST /balance hangs rather than
// refusing (see Client.SetBalance), and the speaker's own app writes balance
// exclusively over this socket.
//
// The level is validated against the range the device reports, not an assumed
// one, which costs a read first. Callers that already hold a reading should use
// SetBalanceWithBounds to skip it.
//
// Confirmed on hardware (ST-10 pair master, FW 27.0.6): applies at both range
// endpoints, and leaves the pairing untouched — /getGroup is byte-identical
// before and after.
func (ws *WebSocketClient) SetBalance(ctx context.Context, level int) (*models.Balance, error) {
	current, err := ws.GetBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("read balance before writing: %w", err)
	}

	return ws.SetBalanceWithBounds(ctx, level, current)
}

// SetBalanceWithBounds writes level, validated against an already-fetched
// reading. A nil bounds skips validation and lets the device judge.
func (ws *WebSocketClient) SetBalanceWithBounds(ctx context.Context, level int, bounds *models.Balance) (*models.Balance, error) {
	request, err := models.NewBalanceRequest(level, bounds)
	if err != nil {
		return nil, err
	}

	body := fmt.Sprintf(`<balance><targetBalance>%d</targetBalance></balance>`, request.Target)

	responseBody, err := ws.Request(ctx, "balance", "POST", body, RequestOptions{MainNode: balanceMainNode})
	if err != nil {
		return nil, err
	}

	// The response echoes the full balance document, so the write is
	// self-verifying: no read-back round trip, and no risk of reading the
	// stale value the HTTP endpoint briefly reports after a write.
	return parseBalanceBody(responseBody)
}

// parseBalanceBody unmarshals the <balance> document out of a response body.
func parseBalanceBody(body []byte) (*models.Balance, error) {
	var balance models.Balance
	if err := xml.Unmarshal(body, &balance); err != nil {
		return nil, fmt.Errorf("parse balance response: %w", err)
	}

	return &balance, nil
}

// OnBalanceUpdated sets a handler for stereo-pair balance changes.
//
// The event carries no payload — it is a signal to re-read, not a value.
func (ws *WebSocketClient) OnBalanceUpdated(handler models.TypedEventHandler[*models.BalanceUpdatedEvent]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnBalanceUpdated = handler
}
