package main

import (
	"fmt"

	"github.com/gesellix/bose-soundtouch/pkg/service/updatecheck"
	"github.com/urfave/cli/v2"
)

// updateCheckRepo is the GitHub repo checked for newer releases, matching
// soundtouch-service's periodic background check (#591,
// _/i591/design-update-check.md).
const updateCheckRepo = "gesellix/Bose-SoundTouch"

// updateCheckCommand assembles the on-demand `soundtouch-backup
// update-check` command, the CLI-side answer to that design doc's open
// question 2 (CLI-only users get no update notice from the service's
// background checker). Unlike the service's opt-in periodic check, running
// this command *is* the opt-in: no config flag, no persisted state, just
// one GitHub API request each time it's invoked.
func updateCheckCommand() *cli.Command {
	return &cli.Command{
		Name:   "update-check",
		Usage:  "Check GitHub for a newer soundtouch-backup release",
		Action: runUpdateCheck,
	}
}

func runUpdateCheck(c *cli.Context) error {
	checker := updatecheck.NewChecker(nil, updateCheckRepo, version)

	result, err := checker.CheckNow(c.Context)
	if err != nil {
		return fmt.Errorf("update check failed: %w", err)
	}

	printUpdateCheckResult(result)

	return nil
}

func printUpdateCheckResult(result updatecheck.Result) {
	if result.LatestVersion == "" {
		fmt.Printf("Running %s, not a released version, skipping comparison.\n", result.CurrentVersion)
		return
	}

	if result.Available {
		fmt.Printf("A newer version is available: %s (you're on %s)\n", result.LatestVersion, result.CurrentVersion)
		fmt.Println(result.ReleaseURL)

		return
	}

	fmt.Printf("You're on the latest version (%s).\n", result.CurrentVersion)
}
