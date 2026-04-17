package auth

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/ui"
)

func init() {
	Cmd.AddCommand(switchCmd)
}

var switchCmd = &cobra.Command{
	Use:   "switch [account-id]",
	Short: "Switch the active account",
	Long: `Switch between accounts accessible to your credentials.

  band auth switch           # interactive selection
  band auth switch 9901303   # switch directly`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSwitch,
}

func runSwitch(cmd *cobra.Command, args []string) error {
	configPath, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cfg.ClientID == "" {
		return fmt.Errorf("not logged in — run `band auth login` first")
	}

	if len(cfg.Accounts) == 0 {
		return fmt.Errorf("no accounts available — try `band auth login` to refresh")
	}

	var target string

	if len(args) == 1 {
		// Direct switch
		target = args[0]
		found := false
		for _, a := range cfg.Accounts {
			if a == target {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "Available accounts:\n")
			for _, a := range cfg.Accounts {
				fmt.Fprintf(os.Stderr, "  %s\n", a)
			}
			return fmt.Errorf("account %s not accessible with current credentials", target)
		}
	} else if len(cfg.Accounts) == 1 {
		fmt.Printf("Only one account available: %s\n", cfg.Accounts[0])
		return nil
	} else {
		// Interactive selection
		if !cmdutil.IsInteractive() {
			fmt.Fprintf(os.Stderr, "Available accounts: %s\n", strings.Join(cfg.Accounts, ", "))
			return fmt.Errorf("specify account: band auth switch <account-id>")
		}

		fmt.Fprintf(os.Stderr, "\nAvailable accounts:\n\n")
		for i, a := range cfg.Accounts {
			marker := "  "
			if a == cfg.AccountID {
				marker = "* "
			}
			fmt.Fprintf(os.Stderr, "  %s%d) %s\n", marker, i+1, a)
		}
		fmt.Fprintf(os.Stderr, "\n  * = current\n\n")
		fmt.Fprintf(os.Stderr, "Select account: ")

		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading selection: %w", err)
		}

		choice := strings.TrimSpace(line)
		idx := 0
		if _, err := fmt.Sscanf(choice, "%d", &idx); err != nil || idx < 1 || idx > len(cfg.Accounts) {
			return fmt.Errorf("invalid selection: %s", choice)
		}
		target = cfg.Accounts[idx-1]
	}

	if target == cfg.AccountID {
		ui.Infof("Already using account %s.", ui.ID(target))
		return nil
	}

	cfg.AccountID = target
	if err := config.Save(configPath, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	ui.Successf("Switched to account %s", ui.ID(target))
	return nil
}
