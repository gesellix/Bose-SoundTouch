// Package main provides the soundtouch-cli balance control commands.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/urfave/cli/v2"
)

// balanceWriteTimeout caps a single balance write, including the read that
// validates it against the device's range.
const balanceWriteTimeout = 15 * time.Second

// Balance belongs to a stereo PAIR — two SoundTouch 10s taking the LEFT and
// RIGHT channel — not to a multiroom zone and not to one speaker. Measured on
// hardware, either member answers with the same value and a write to either is
// reflected by both, so these commands work against whichever member is
// addressed. An unpaired speaker reports balanceAvailable=false.
//
// Writes go over the WebSocket, never HTTP: POST /balance hangs rather than
// refusing, and the app Bose ships on the speaker writes balance exclusively
// over the socket (GH-699).

// getBalance retrieves the current balance level from the device
func getBalance(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader("Getting balance level", clientConfig.Host, clientConfig.Port)

	soundTouchClient, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	balance, err := soundTouchClient.GetBalance()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to get balance: %v", err))
		return err
	}

	printBalance(balance)

	return nil
}

// printBalance renders a reading, including the unavailable case, which is
// what a speaker that is not in a stereo pair reports.
func printBalance(balance *models.Balance) {
	if !balance.Available {
		fmt.Println("Balance: not available on this speaker")
		fmt.Println("  Balance exists only while a speaker is in a stereo pair (two SoundTouch 10s).")
		fmt.Println("  Pair them, then either member accepts the setting.")

		return
	}

	fmt.Printf("Current balance level: %d\n", balance.Actual)

	if balance.Target != balance.Actual {
		fmt.Printf("Target balance level: %d\n", balance.Target)
	}

	fmt.Printf("Range: %d to %d (default %d)\n", balance.Min, balance.Max, balance.Default)
	fmt.Printf("Balance direction: %s\n", balance.LevelName(balance.Actual))
}

// setBalance sets the balance level on the device
func setBalance(c *cli.Context) error {
	level := c.Int("level")
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Setting balance level to %d", level), clientConfig.Host, clientConfig.Port)

	return writeBalance(clientConfig, func(_ *models.Balance) int { return level })
}

// balanceLeft shifts balance to the left
func balanceLeft(c *cli.Context) error {
	amount := c.Int("amount")
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Shifting balance left by %d", amount), clientConfig.Host, clientConfig.Port)

	return writeBalance(clientConfig, func(current *models.Balance) int {
		return current.Clamp(current.Actual - amount)
	})
}

// balanceRight shifts balance to the right
func balanceRight(c *cli.Context) error {
	amount := c.Int("amount")
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Shifting balance right by %d", amount), clientConfig.Host, clientConfig.Port)

	return writeBalance(clientConfig, func(current *models.Balance) int {
		return current.Clamp(current.Actual + amount)
	})
}

// balanceCenter centers the balance, at whatever the device calls its default.
func balanceCenter(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader("Centering balance", clientConfig.Host, clientConfig.Port)

	return writeBalance(clientConfig, func(current *models.Balance) int { return current.Default })
}

// writeBalance opens a WebSocket, reads the current balance so the target can
// be computed and validated against the DEVICE-reported range, writes, and
// prints the value the speaker echoes back.
//
// The echoed response is used rather than a read-back: HTTP GET /balance lags
// briefly behind a write, so re-reading to confirm can report the old value.
func writeBalance(clientConfig *ClientConfig, target func(current *models.Balance) int) error {
	soundTouchClient, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	wsClient := soundTouchClient.NewWebSocketClient(&client.WebSocketConfig{
		Logger: &SilentLogger{},
	})

	if connectErr := wsClient.Connect(); connectErr != nil {
		PrintError(fmt.Sprintf("Failed to open the WebSocket: %v", connectErr))
		return connectErr
	}

	defer func() { _ = wsClient.Disconnect() }()

	ctx, cancel := context.WithTimeout(context.Background(), balanceWriteTimeout)
	defer cancel()

	current, err := wsClient.GetBalance(ctx)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to read the current balance: %v", err))
		return err
	}

	if !current.Available {
		printBalance(current)

		return fmt.Errorf("balance is not available on this speaker")
	}

	updated, err := wsClient.SetBalanceWithBounds(ctx, target(current), current)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to set balance: %v", err))
		return err
	}

	PrintSuccess(fmt.Sprintf("Balance set to %d (%s)", updated.Target, updated.LevelName(updated.Target)))

	return nil
}
