package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/ui"
	versionpkg "github.com/Bandwidth/cli/internal/version"

	accountcmd "github.com/Bandwidth/cli/cmd/account"
	appcmd "github.com/Bandwidth/cli/cmd/app"
	authcmd "github.com/Bandwidth/cli/cmd/auth"
	bxmlcmd "github.com/Bandwidth/cli/cmd/bxml"
	callcmd "github.com/Bandwidth/cli/cmd/call"
	locationcmd "github.com/Bandwidth/cli/cmd/location"
	messagecmd "github.com/Bandwidth/cli/cmd/message"
	numbercmd "github.com/Bandwidth/cli/cmd/number"
	quickstartcmd "github.com/Bandwidth/cli/cmd/quickstart"
	recordingcmd "github.com/Bandwidth/cli/cmd/recording"
	shortcodecmd "github.com/Bandwidth/cli/cmd/shortcode"
	sitecmd "github.com/Bandwidth/cli/cmd/site"
	tendlccmd "github.com/Bandwidth/cli/cmd/tendlc"
	tfvcmd "github.com/Bandwidth/cli/cmd/tfv"
	tnoptioncmd "github.com/Bandwidth/cli/cmd/tnoption"
	transcriptioncmd "github.com/Bandwidth/cli/cmd/transcription"
	vcpcmd "github.com/Bandwidth/cli/cmd/vcp"
)

var (
	format      string
	plain       bool
	accountID   string
	environment string

	// version is set by goreleaser via ldflags at build time.
	version = "dev"
)

// updateResult receives the version check result from the background goroutine.
var updateResult chan *versionpkg.CheckResult

var rootCmd = &cobra.Command{
	Use:   "band",
	Short: "Bandwidth CLI — manage voice, messaging, numbers, and more from the command line",
	Long:  "The official Bandwidth CLI. Build and debug voice applications, send messages, manage phone numbers, and control calls.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Kick off version check in background so it doesn't slow down the command.
		updateResult = make(chan *versionpkg.CheckResult, 1)
		go func() {
			updateResult <- versionpkg.Check(version)
		}()
		if !term.IsTerminal(int(os.Stdout.Fd())) {
			// Auto-enable plain mode for non-terminal output (scripts, pipes)
			// unless the user explicitly chose a different format.
			if !cmd.Root().Flag("plain").Changed && !cmd.Root().Flag("format").Changed {
				plain = true
			}
		}

		// Show active account when ambiguous (multiple accounts or system-wide scope).
		// Skip for auth/account/bxml/version commands that don't need an account.
		showAccountHint(cmd)
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if updateResult == nil {
			return
		}
		result := <-updateResult
		if result != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, ui.Warn(result.NoticeMessage()))
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&format, "format", "json", "Output format: json or table")
	rootCmd.PersistentFlags().BoolVar(&plain, "plain", false, "Simplified flat JSON output (recommended for scripts and agents)")
	rootCmd.PersistentFlags().StringVar(&accountID, "account-id", "", "Bandwidth account ID (overrides config)")
	rootCmd.PersistentFlags().StringVar(&environment, "environment", "", "API environment: prod, test (overrides config)")
	rootCmd.AddCommand(authcmd.Cmd)
	rootCmd.AddCommand(accountcmd.Cmd)
	rootCmd.AddCommand(sitecmd.Cmd)
	rootCmd.AddCommand(locationcmd.Cmd)
	rootCmd.AddCommand(appcmd.Cmd)
	rootCmd.AddCommand(numbercmd.Cmd)
	rootCmd.AddCommand(callcmd.Cmd)
	rootCmd.AddCommand(messagecmd.Cmd)
	rootCmd.AddCommand(recordingcmd.Cmd)
	rootCmd.AddCommand(transcriptioncmd.Cmd)
	rootCmd.AddCommand(bxmlcmd.Cmd)
	rootCmd.AddCommand(quickstartcmd.Cmd)
	rootCmd.AddCommand(vcpcmd.Cmd)
	rootCmd.AddCommand(tendlccmd.Cmd)
	rootCmd.AddCommand(shortcodecmd.Cmd)
	rootCmd.AddCommand(tfvcmd.Cmd)
	rootCmd.AddCommand(tnoptioncmd.Cmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("band version %s\n", version)
	},
}

func Execute() error {
	api.Version = version
	return rootCmd.Execute()
}

func GetFormat() string {
	if f := os.Getenv("BW_FORMAT"); f != "" && format == "json" {
		return f
	}
	return format
}

func GetAccountID() string {
	if accountID != "" {
		return accountID
	}
	return os.Getenv("BW_ACCOUNT_ID")
}

// GetPlain returns true if the --plain flag was set.
func GetPlain() bool {
	return plain
}

// showAccountHint prints the active account to stderr when there's ambiguity
// (multiple accounts or system-wide scope). Skips commands that don't need accounts.
func showAccountHint(cmd *cobra.Command) {
	// Skip for commands that don't make API calls requiring an account
	root := cmd.Root()
	if cmd == root {
		return
	}
	name := cmd.Name()
	parent := ""
	if cmd.Parent() != nil && cmd.Parent() != root {
		parent = cmd.Parent().Name()
	}
	// Skip auth, account (registration), bxml, version, help, completion
	skipParents := map[string]bool{"auth": true, "account": true, "bxml": true}
	skipNames := map[string]bool{"version": true, "help": true, "completion": true}
	if skipParents[parent] || skipParents[name] || skipNames[name] {
		return
	}

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return
	}
	p := cfg.ActiveProfileConfig()

	// Only show when ambiguous
	hasMultipleAccounts := len(p.Accounts) > 1
	hasSystemScope := len(p.Accounts) == 0 && p.ClientID != ""
	hasMultipleProfiles := len(cfg.Profiles) > 1

	if !hasMultipleAccounts && !hasSystemScope && !hasMultipleProfiles {
		return
	}

	// Check if --account-id was explicitly passed
	explicitAccount := cmd.Root().Flag("account-id").Value.String()

	profileName := cfg.ActiveProfile
	if profileName == "" {
		profileName = "default"
	}

	acctID := explicitAccount
	if acctID == "" {
		acctID = p.AccountID
	}
	if acctID == "" {
		acctID = "(none)"
	}

	parts := []string{fmt.Sprintf("account: %s", ui.ID(acctID))}
	if hasMultipleProfiles {
		parts = append(parts, fmt.Sprintf("profile: %s", profileName))
	}

	// Show environment when the user operates across multiple environments
	// or is on a non-default one. Customers with only prod don't need the noise.
	env := p.Environment
	if env == "" {
		env = "prod"
	}
	if env != "prod" || cfg.HasMultipleEnvironments() {
		parts = append(parts, fmt.Sprintf("env: %s", env))
	}

	fmt.Fprintf(os.Stderr, "%s\n", ui.Muted("["+strings.Join(parts, " | ")+"]"))
}
