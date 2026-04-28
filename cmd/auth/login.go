package auth

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"github.com/spf13/cobra"

	intauth "github.com/Bandwidth/cli/internal/auth"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/ui"
)

// Cmd is the `band auth` parent command.
var Cmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication credentials",
}

func init() {
	Cmd.AddCommand(loginCmd)
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in with Bandwidth OAuth2 client credentials",
	Long: `Authenticate with Bandwidth API client credentials.

Only a client ID and secret are required — the CLI will automatically
discover which accounts the credentials can access.

  band auth login --client-id <id> --client-secret <secret>
  band auth login --client-id <id> --client-secret <secret> --profile admin

Or via environment variables:

  BW_CLIENT_ID=<id> BW_CLIENT_SECRET=<secret> band auth login`,
	RunE: runLogin,
	Example: `  # Interactive login
  band auth login

  # Non-interactive with flags
  band auth login --client-id CLI-abc123 --client-secret mySecret

  # Non-interactive with env vars
  BW_CLIENT_ID=CLI-abc123 BW_CLIENT_SECRET=mySecret band auth login

  # Store under a named profile
  band auth login --client-id CLI-abc123 --client-secret mySecret --profile admin`,
}

func init() {
	loginCmd.Flags().String("client-id", "", "Bandwidth OAuth2 client ID")
	loginCmd.Flags().String("client-secret", "", "Bandwidth OAuth2 client secret")
	loginCmd.Flags().String("profile", "default", "Profile name to store credentials under")
}

func runLogin(cmd *cobra.Command, args []string) error {
	clientID, _ := cmd.Flags().GetString("client-id")
	clientSecret, _ := cmd.Flags().GetString("client-secret")
	profileName, _ := cmd.Flags().GetString("profile")
	environment, _ := cmd.Root().PersistentFlags().GetString("environment")

	if clientID == "" {
		clientID = os.Getenv("BW_CLIENT_ID")
	}
	if clientSecret == "" {
		clientSecret = os.Getenv("BW_CLIENT_SECRET")
	}
	if environment == "" {
		environment = os.Getenv("BW_ENVIRONMENT")
	}

	// Prompt for missing credentials
	if clientID == "" || clientSecret == "" {
		if !cmdutil.IsInteractive() {
			missing := []string{}
			if clientID == "" {
				missing = append(missing, "client-id")
			}
			if clientSecret == "" {
				missing = append(missing, "client-secret")
			}
			return fmt.Errorf("no TTY available and missing: %s\n\n"+
				"Use flags:\n"+
				"  band auth login --client-id ID --client-secret SECRET\n\n"+
				"Or environment variables:\n"+
				"  BW_CLIENT_ID=ID BW_CLIENT_SECRET=SECRET band auth login",
				strings.Join(missing, ", "))
		}

		reader := bufio.NewReader(os.Stdin)

		if clientID == "" {
			fmt.Print("Client ID: ")
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading client ID: %w", err)
			}
			clientID = strings.TrimSpace(line)
		}

		if clientSecret == "" {
			fmt.Print("Client Secret: ")
			secretBytes, err := cmdutil.ReadPassword()
			fmt.Println()
			if err != nil {
				return fmt.Errorf("reading client secret: %w", err)
			}
			clientSecret = string(secretBytes)
		}
	}

	tokenURL := tokenURLForEnvironment(environment)

	// Step 1: Verify credentials
	spin := ui.NewSpinner("Verifying credentials...")
	spin.Start()
	tm := intauth.NewTokenManager(clientID, clientSecret, tokenURL)
	token, err := tm.GetToken()
	spin.Stop()
	if err != nil {
		return fmt.Errorf("credential verification failed: %w", err)
	}
	ui.Successf("Credentials verified")

	// Step 2: Extract accounts from JWT
	claims, err := parseJWTClaims(token)
	if err != nil {
		return fmt.Errorf("reading token claims: %w", err)
	}
	accounts := claims.Accounts

	if len(accounts) == 0 {
		ui.Infof("Your credentials are not bound to a specific account.")
		ui.Infof("Use --account-id on commands to target a specific account.")
	}

	// Step 3: Store secret in keychain (keyed by client ID)
	if err := intauth.StorePassword(clientID, clientSecret); err != nil {
		return fmt.Errorf("storing credentials: %w", err)
	}

	// Step 4: Load config and create/update profile
	configPath, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	profile := &config.Profile{
		ClientID:    clientID,
		Accounts:    accounts,
		Environment: environment,
		Roles:       claims.Roles,
		Express:     claims.Express,
	}

	// Step 5: Select active account
	profile.AccountID = selectAccount(cmd, accounts)

	cfg.SetProfile(profileName, profile)

	if err := config.Save(configPath, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Fprintln(os.Stderr, "")
	ui.Successf("Logged in")
	ui.Infof("Active account: %s", ui.ID(profile.AccountID))
	if len(accounts) > 1 {
		fmt.Fprintf(os.Stderr, "\nYou have access to %d accounts. Use `band auth switch` to change the active account.\n", len(accounts))
	}
	if profileName != "default" {
		fmt.Fprintf(os.Stderr, "Use `band auth use %s` to switch to this profile.\n", profileName)
	}
	return nil
}

// selectAccount picks an account ID from the available accounts.
func selectAccount(cmd *cobra.Command, accounts []string) string {
	override, _ := cmd.Root().PersistentFlags().GetString("account-id")
	if override == "" {
		override = os.Getenv("BW_ACCOUNT_ID")
	}
	if override != "" {
		for _, a := range accounts {
			if a == override {
				return override
			}
		}
		fmt.Fprintf(os.Stderr, "Warning: account %s not found in token. Using it anyway.\n", override)
		return override
	}

	if len(accounts) == 1 {
		return accounts[0]
	}
	if len(accounts) == 0 {
		return ""
	}

	if !cmdutil.IsInteractive() {
		return accounts[0]
	}

	fmt.Fprintf(os.Stderr, "\nYour credentials have access to %d accounts:\n\n", len(accounts))
	for i, a := range accounts {
		fmt.Fprintf(os.Stderr, "    %s) %s\n", ui.Bold(fmt.Sprintf("%d", i+1)), a)
	}
	fmt.Fprintf(os.Stderr, "\nSelect active account %s: ", ui.Muted("[1]"))

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) == "" {
		return accounts[0]
	}

	idx := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(line), "%d", &idx); err != nil || idx < 1 || idx > len(accounts) {
		fmt.Fprintf(os.Stderr, "Invalid selection, using account %s\n", accounts[0])
		return accounts[0]
	}
	return accounts[idx-1]
}

type jwtClaims struct {
	Accounts []string `json:"accounts"`
	Roles    []string `json:"roles"`
	Express  bool     `json:"express"`
}

func parseJWTClaims(token string) (*jwtClaims, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	payload := parts[1]
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("decoding JWT payload: %w", err)
	}

	var claims jwtClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("parsing JWT claims: %w", err)
	}

	return &claims, nil
}

// tokenURLForEnvironment maps an environment name to its OAuth2 base URL.
// Non-production environments can be overridden with BW_API_URL.
func tokenURLForEnvironment(env string) string {
	if v := os.Getenv("BW_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	switch env {
	case "test", "uat":
		return "https://test.api.bandwidth.com"
	default:
		return "https://api.bandwidth.com"
	}
}
