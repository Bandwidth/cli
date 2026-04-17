package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/ui"
)

func init() {
	Cmd.AddCommand(profilesCmd)
	Cmd.AddCommand(useCmd)
}

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List all credential profiles",
	RunE:  runProfiles,
}

func runProfiles(cmd *cobra.Command, args []string) error {
	configPath, err := config.DefaultPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	if len(cfg.Profiles) == 0 {
		// Legacy single-credential config
		if cfg.ClientID != "" {
			marker := ui.Success("*")
			fmt.Printf("  %s %s  %s  (account: %s)\n", marker, ui.Bold("default"), ui.ID(cfg.ClientID), ui.Muted(cfg.AccountID))
		} else {
			fmt.Println("No profiles. Run `band auth login` to create one.")
		}
		return nil
	}

	showEnv := cfg.HasMultipleEnvironments()

	for _, name := range cfg.ProfileNames() {
		p := cfg.Profiles[name]
		marker := "  "
		if name == cfg.ActiveProfile {
			marker = ui.Success("*") + " "
		}
		acctInfo := ui.Muted(p.AccountID)
		if len(p.Accounts) > 1 {
			acctInfo = fmt.Sprintf("%s %s", ui.Muted(p.AccountID), ui.Muted(fmt.Sprintf("(%d accounts)", len(p.Accounts))))
		}
		envTag := ""
		if showEnv {
			env := p.Environment
			if env == "" {
				env = "prod"
			}
			envTag = fmt.Sprintf("  %s", ui.Muted("["+env+"]"))
		}
		fmt.Printf("  %s%-10s  %s  %s%s\n", marker, ui.Bold(name), ui.ID(p.ClientID), acctInfo, envTag)
	}

	fmt.Println("\n  * = active profile")
	return nil
}

var useCmd = &cobra.Command{
	Use:   "use <profile>",
	Short: "Switch to a different credential profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runUse,
}

func runUse(cmd *cobra.Command, args []string) error {
	name := args[0]

	configPath, err := config.DefaultPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	if len(cfg.Profiles) == 0 {
		return fmt.Errorf("no profiles configured — run `band auth login` first")
	}

	p, ok := cfg.Profiles[name]
	if !ok {
		fmt.Printf("Available profiles: %v\n", cfg.ProfileNames())
		return fmt.Errorf("profile %q not found", name)
	}

	cfg.ActiveProfile = name
	// Sync legacy fields
	cfg.ClientID = p.ClientID
	cfg.AccountID = p.AccountID
	cfg.Accounts = p.Accounts
	cfg.Environment = p.Environment

	if err := config.Save(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("Switched to profile %q (account: %s)\n", name, p.AccountID)
	return nil
}
