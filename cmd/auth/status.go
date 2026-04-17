package auth

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	intauth "github.com/Bandwidth/cli/internal/auth"
	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/ui"
)

func init() {
	Cmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	configPath, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p := cfg.ActiveProfileConfig()

	if p.ClientID == "" {
		fmt.Fprintln(os.Stderr, ui.Warn("Not logged in."))
		return nil
	}

	env := p.Environment
	if env == "" {
		env = "prod"
	}

	// Show environment only when it's informative: either the user is on a
	// non-default environment or they have profiles spanning multiple environments.
	showEnv := env != "prod" || cfg.HasMultipleEnvironments()

	_, err = intauth.GetPassword(p.ClientID)
	if err != nil {
		fmt.Printf("Client ID:   %s\n", ui.ID(p.ClientID))
		fmt.Printf("Account:     %s\n", ui.ID(p.AccountID))
		if showEnv {
			fmt.Printf("Environment: %s\n", env)
		}
		fmt.Println("Status:      " + ui.Error("credentials not found in keychain"))
		return nil
	}

	profileName := cfg.ActiveProfile
	if profileName == "" {
		profileName = "default"
	}

	fmt.Printf("Profile:     %s\n", ui.Bold(profileName))
	fmt.Printf("Client ID:   %s\n", ui.ID(p.ClientID))
	if p.AccountID != "" {
		fmt.Printf("Account:     %s\n", ui.ID(p.AccountID))
	} else {
		fmt.Printf("Account:     (none — pass --account-id on commands)\n")
	}
	if len(p.Accounts) > 1 {
		fmt.Printf("Accounts:    %s\n", strings.Join(p.Accounts, ", "))
	} else if len(p.Accounts) == 0 && p.AccountID == "" {
		fmt.Println("Scope:       system-wide (use --account-id to target an account)")
	}
	if showEnv {
		fmt.Printf("Environment: %s\n", env)
	}
	fmt.Println("Status:      " + ui.Success("authenticated"))

	if len(cfg.Profiles) > 1 {
		fmt.Printf("Profiles:    %s\n", strings.Join(cfg.ProfileNames(), ", "))
	}
	return nil
}
