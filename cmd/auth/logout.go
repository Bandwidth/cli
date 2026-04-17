package auth

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	intauth "github.com/Bandwidth/cli/internal/auth"
	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/ui"
)

func init() {
	Cmd.AddCommand(logoutCmd)
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and remove stored credentials",
	RunE:  runLogout,
}

func runLogout(cmd *cobra.Command, args []string) error {
	configPath, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Collect all client IDs across profiles and legacy fields.
	clientIDs := make(map[string]bool)
	for _, p := range cfg.Profiles {
		if p.ClientID != "" {
			clientIDs[p.ClientID] = true
		}
	}
	if cfg.ClientID != "" {
		clientIDs[cfg.ClientID] = true
	}

	if len(clientIDs) == 0 {
		fmt.Println("Not logged in.")
		return nil
	}

	// Best-effort keychain deletion for every profile.
	for id := range clientIDs {
		if err := intauth.DeletePassword(id); err != nil {
			fmt.Printf("Warning: could not remove keychain entry for %s: %v\n", id, err)
		}
	}

	// Remove the config file entirely for a clean slate.
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing config file: %w", err)
	}

	ui.Successf("Logged out")
	return nil
}
