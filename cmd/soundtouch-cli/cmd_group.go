package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/speaker"
	"github.com/urfave/cli/v2"
)

// getGroupStatus retrieves and prints the device's current stereo-pair state.
func getGroupStatus(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader("Getting group information", clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	group, err := client.GetGroup()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to get group: %v", err))
		return err
	}

	if group.IsEmpty() {
		fmt.Println("Device is not in a stereo pair")
		return nil
	}

	printGroup(group)

	return nil
}

// createGroup forms a stereo pair by POSTing /addGroup to both speakers in
// parallel. LEFT is the master. Addressing each speaker directly (instead of
// only the master and letting it propagate via marge) sidesteps the
// inter-device round-trip that surfaced as client timeouts in #252.
func createGroup(c *cli.Context) error {
	leftIP := c.String("left")
	rightIP := c.String("right")
	name := c.String("name")

	if net.ParseIP(leftIP) == nil {
		PrintError(fmt.Sprintf("Invalid left IP address: %s", leftIP))
		return fmt.Errorf("invalid left IP: %s", leftIP)
	}

	if net.ParseIP(rightIP) == nil {
		PrintError(fmt.Sprintf("Invalid right IP address: %s", rightIP))
		return fmt.Errorf("invalid right IP: %s", rightIP)
	}

	PrintDeviceHeader(fmt.Sprintf("Creating stereo pair: LEFT=%s RIGHT=%s", leftIP, rightIP), leftIP, speaker.HTTPPort)

	leftInfo, err := fetchDeviceInfo(c, leftIP)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to read LEFT device info: %v", err))
		return err
	}

	rightInfo, err := fetchDeviceInfo(c, rightIP)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to read RIGHT device info: %v", err))
		return err
	}

	if name == "" {
		name = fmt.Sprintf("%s + %s", leftInfo.Name, rightInfo.Name)
	}

	req := &models.Group{
		Name:           name,
		MasterDeviceID: leftInfo.DeviceID,
		Roles: models.GroupRoles{
			Roles: []models.GroupRole{
				{DeviceID: leftInfo.DeviceID, Role: "LEFT", IPAddress: leftIP},
				{DeviceID: rightInfo.DeviceID, Role: "RIGHT", IPAddress: rightIP},
			},
		},
		SenderIPAddress: leftIP,
	}

	leftClient, err := clientForHost(c, leftIP)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client for LEFT: %v", err))
		return err
	}

	rightClient, err := clientForHost(c, rightIP)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client for RIGHT: %v", err))
		return err
	}

	leftOut, rightOut := propagateAddGroup(leftClient, rightClient, leftIP, rightIP, req)

	if leftOut.err != nil {
		PrintError(fmt.Sprintf("LEFT (%s) /addGroup failed: %v", leftIP, leftOut.err))
	}

	if rightOut.err != nil {
		PrintError(fmt.Sprintf("RIGHT (%s) /addGroup failed: %v", rightIP, rightOut.err))
	}

	if leftOut.err != nil || rightOut.err != nil {
		if (leftOut.err == nil) != (rightOut.err == nil) {
			succeeded := leftIP
			if leftOut.err != nil {
				succeeded = rightIP
			}

			PrintError(fmt.Sprintf("Partial group state on %s — clean up with `soundtouch-cli --host %s group remove`", succeeded, succeeded))
		}

		return fmt.Errorf("/addGroup propagation failed")
	}

	// The LEFT (master) response carries the assigned group ID; use it for display.
	PrintSuccess(fmt.Sprintf("Stereo pair created (id=%s)", leftOut.group.ID))
	printGroup(leftOut.group)

	return nil
}

// addGroupOutcome is the per-speaker result of a parallel /addGroup call.
type addGroupOutcome struct {
	host  string
	group *models.Group
	err   error
}

// propagateAddGroup POSTs /addGroup to both speakers concurrently and returns
// the (LEFT, RIGHT) outcomes. A non-GROUP_OK Status in the response is
// reported as an error so callers don't have to re-inspect the body.
func propagateAddGroup(left, right *client.Client, leftIP, rightIP string, req *models.Group) (addGroupOutcome, addGroupOutcome) {
	var (
		wg                sync.WaitGroup
		leftOut, rightOut addGroupOutcome
	)

	wg.Add(2)

	go func() {
		defer wg.Done()

		leftOut = postAddGroup(left, leftIP, req)
	}()

	go func() {
		defer wg.Done()

		rightOut = postAddGroup(right, rightIP, req)
	}()

	wg.Wait()

	return leftOut, rightOut
}

func postAddGroup(cli *client.Client, host string, req *models.Group) addGroupOutcome {
	out := addGroupOutcome{host: host}

	g, err := cli.AddGroup(req)
	if err != nil {
		out.err = err
		return out
	}

	out.group = g

	if g != nil && g.Status != "" && g.Status != "GROUP_OK" {
		out.err = fmt.Errorf("device returned status %q (want GROUP_OK)", g.Status)
	}

	return out
}

// renameGroup updates the name of the existing stereo pair. The device
// requires the full structure on every update, so we fetch the current
// state first.
func renameGroup(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	newName := c.String("name")

	if newName == "" {
		PrintError("--name is required")
		return fmt.Errorf("name is required")
	}

	PrintDeviceHeader(fmt.Sprintf("Renaming stereo pair to %q", newName), clientConfig.Host, clientConfig.Port)

	stClient, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	current, err := stClient.GetGroup()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to read current group: %v", err))
		return err
	}

	if current.IsEmpty() {
		PrintError("Device is not in a stereo pair — nothing to rename")
		return fmt.Errorf("no group configured")
	}

	// Status is read-only on the device side; don't echo it back.
	current.Status = ""
	current.Name = newName

	result, err := stClient.UpdateGroup(current)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to rename group: %v", err))
		return err
	}

	PrintSuccess(fmt.Sprintf("Stereo pair renamed to %q", result.Name))
	printGroup(result)

	return nil
}

// removeGroup tears down the device's stereo pair.
func removeGroup(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader("Removing stereo pair", clientConfig.Host, clientConfig.Port)

	stClient, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	if err := stClient.RemoveGroup(); err != nil {
		PrintError(fmt.Sprintf("Failed to remove group: %v", err))
		return err
	}

	PrintSuccess("Stereo pair removed")

	return nil
}

// fetchDeviceInfo builds a one-off client for the given IP and reads /info.
// Reused for both halves of a `create` invocation so the caller doesn't have
// to babysit two host/port pairs.
func fetchDeviceInfo(c *cli.Context, host string) (*models.DeviceInfo, error) {
	stClient, err := clientForHost(c, host)
	if err != nil {
		return nil, err
	}

	return stClient.GetDeviceInfo()
}

// clientForHost mirrors CreateSoundTouchClient but overrides the host so we
// can talk to a speaker other than the one named in --host.
func clientForHost(c *cli.Context, host string) (*client.Client, error) {
	cfg, err := loadConfig(c.Duration("timeout"))
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return client.NewClient(&client.Config{
		Host:      host,
		Port:      speaker.HTTPPort,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}), nil
}

func printGroup(g *models.Group) {
	fmt.Println("Stereo Pair Configuration:")
	fmt.Printf("  ID:        %s\n", g.ID)
	fmt.Printf("  Name:      %s\n", g.Name)
	fmt.Printf("  Master:    %s\n", g.MasterDeviceID)

	if g.Status != "" {
		fmt.Printf("  Status:    %s\n", g.Status)
	}

	for _, r := range g.Roles.Roles {
		fmt.Printf("  %-5s     %s", r.Role, r.DeviceID)

		if r.IPAddress != "" {
			fmt.Printf(" (IP: %s)", r.IPAddress)
		}

		fmt.Println()
	}
}
