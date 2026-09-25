package sample

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	callcmd "github.com/Bandwidth/cli/cmd/call"
	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/ui"
)

var (
	runLanguage   string
	runCallTo     string
	runPort       int
	runOpenAIKey  string
	runTransferTo string
)

var runCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Download and run a sample application",
	Long: `Clones the sample app, wires up Bandwidth credentials from your active band
profile, starts an ngrok tunnel, and launches the app.

Pass --call-to to automatically dial a number once the app is ready.`,
	Example: `  band sample run live-assistant --language python --openai-key sk-...
  band sample run live-assistant --language python --openai-key sk-... --call-to +13367499393`,
	Args: cobra.ExactArgs(1),
	RunE: runSample,
}

func init() {
	runCmd.Flags().StringVarP(&runLanguage, "language", "l", "", "Language variant to run (required)")
	runCmd.Flags().StringVar(&runCallTo, "call-to", "", "Phone number to call after the app is ready (E.164)")
	runCmd.Flags().IntVar(&runPort, "port", 0, "Local port (default: from catalog)")
	runCmd.Flags().StringVar(&runOpenAIKey, "openai-key", "", "OpenAI API key (for AI-powered samples)")
	runCmd.Flags().StringVar(&runTransferTo, "transfer-to", "", "Phone number to transfer calls to (E.164)")
	_ = runCmd.MarkFlagRequired("language")
}

func runSample(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	name := args[0]

	// ── Catalog lookup ──────────────────────────────────────────────────────
	entry, ok := catalog[name]
	if !ok {
		return fmt.Errorf("unknown sample %q — run `band sample list` to see available samples", name)
	}
	repoURL, ok := entry.Repos[runLanguage]
	if !ok {
		supported := make([]string, 0, len(entry.Repos))
		for l := range entry.Repos {
			supported = append(supported, l)
		}
		return fmt.Errorf("language %q not supported for %q (available: %s)", runLanguage, name, strings.Join(supported, ", "))
	}
	setup, ok := entry.Setup[runLanguage]
	if !ok {
		return fmt.Errorf("no setup defined for %s/%s — open a PR to add it", name, runLanguage)
	}
	port := entry.Port
	if runPort != 0 {
		port = runPort
	}

	// ── Credentials ─────────────────────────────────────────────────────────
	clientID, clientSecret, acctID, err := cmdutil.LoadRawCredentials(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	// ── Extra env var validation ─────────────────────────────────────────────
	extraEnv, err := collectExtraEnv(name, entry)
	if err != nil {
		return err
	}

	// ── Prerequisites ────────────────────────────────────────────────────────
	if err := checkPrerequisites(runLanguage); err != nil {
		return err
	}

	// ── Clone / update repo ──────────────────────────────────────────────────
	cloneDir, err := ensureCloned(ctx, repoURL, name, runLanguage)
	if err != nil {
		return err
	}

	// ── ngrok ────────────────────────────────────────────────────────────────
	ngrokPath, err := exec.LookPath("ngrok")
	if err != nil {
		return fmt.Errorf("ngrok not found — install it with: brew install ngrok/ngrok/ngrok")
	}
	ngrokCmd := exec.CommandContext(ctx, ngrokPath, "http", strconv.Itoa(port), "--log=stdout")
	if err := ngrokCmd.Start(); err != nil {
		return fmt.Errorf("starting ngrok: %w", err)
	}
	defer func() { _ = ngrokCmd.Process.Kill() }()

	spin := ui.NewSpinner("Waiting for ngrok tunnel...")
	spin.Start()
	publicURL, err := waitForNgrok(ctx, 15*time.Second)
	spin.Stop()
	if err != nil {
		return fmt.Errorf("ngrok tunnel not ready: %w", err)
	}
	ui.Successf("ngrok tunnel: %s", ui.ID(publicURL))

	// ── Bandwidth resources ──────────────────────────────────────────────────
	dashClient, _, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}
	voiceClient, _, err := cmdutil.VoiceClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	appID, err := ensureVoiceApp(ctx, dashClient, acctID, "band-sample-"+name,
		publicURL+"/webhooks/bandwidth/voice/initiate",
		publicURL+"/webhooks/bandwidth/voice/status",
	)
	if err != nil {
		return err
	}

	phoneNumber, err := firstPhoneNumber(ctx, dashClient, acctID)
	if err != nil {
		return err
	}
	ui.Successf("Using number: %s", ui.ID(phoneNumber))

	// ── Python venv setup (language-specific) ───────────────────────────────
	if runLanguage == "python" {
		venvSpin := ui.NewSpinner("Creating Python virtual environment...")
		venvSpin.Start()
		venvCmd := exec.CommandContext(ctx, "python3", "-m", "venv", ".venv")
		venvCmd.Dir = cloneDir
		if out, err := venvCmd.CombinedOutput(); err != nil {
			venvSpin.Stop()
			return fmt.Errorf("creating venv: %w\n%s", err, out)
		}
		venvSpin.Stop()
	}

	// ── Install dependencies ─────────────────────────────────────────────────
	depSpin := ui.NewSpinner("Installing dependencies...")
	depSpin.Start()
	depCmd := exec.CommandContext(ctx, setup.InstallCmd[0], setup.InstallCmd[1:]...)
	depCmd.Dir = cloneDir
	if out, err := depCmd.CombinedOutput(); err != nil {
		depSpin.Stop()
		return fmt.Errorf("installing dependencies: %w\n%s", err, out)
	}
	depSpin.Stop()
	ui.Successf("Dependencies installed")

	// ── Write .env file ──────────────────────────────────────────────────────
	envVars := map[string]string{
		"BW_ACCOUNT_ID":    acctID,
		"BW_CLIENT_ID":     clientID,
		"BW_CLIENT_SECRET": clientSecret,
		"BASE_URL":         publicURL,
		"LOCAL_PORT":       strconv.Itoa(port),
		"LOG_LEVEL":        "INFO",
	}
	for k, v := range extraEnv {
		envVars[k] = v
	}
	if err := writeEnvFile(filepath.Join(cloneDir, ".env"), envVars); err != nil {
		return err
	}

	// ── Start the sample app ─────────────────────────────────────────────────
	fmt.Fprintln(os.Stderr, "")
	ui.Headerf("Starting %s (%s)", name, runLanguage)
	fmt.Fprintf(os.Stderr, "  App dir : %s\n", filepath.Join(cloneDir, setup.AppDir))
	fmt.Fprintf(os.Stderr, "  Tunnel  : %s\n", publicURL)
	fmt.Fprintf(os.Stderr, "  Number  : %s\n\n", phoneNumber)

	// Resolve the run executable relative to cloneDir when it's a relative path
	// (e.g. ".venv/bin/python3") — the working dir below is set to AppDir, which
	// may be a subdirectory, so the relative path won't resolve correctly otherwise.
	runExe := setup.RunCmd[0]
	if strings.HasPrefix(runExe, ".") {
		runExe = filepath.Join(cloneDir, runExe)
	}
	appCmd := exec.CommandContext(ctx, runExe, setup.RunCmd[1:]...)
	appCmd.Dir = filepath.Join(cloneDir, setup.AppDir)
	appCmd.Stdout = os.Stdout
	appCmd.Stderr = os.Stderr
	if err := appCmd.Start(); err != nil {
		return fmt.Errorf("starting app: %w", err)
	}

	// ── Optionally make an outbound call once the app is healthy ─────────────
	if runCallTo != "" {
		go func() {
			healthURL := fmt.Sprintf("http://localhost:%d%s", port, setup.HealthPath)
			if err := waitForHealth(ctx, healthURL, 30*time.Second); err != nil {
				fmt.Fprintf(os.Stderr, "\n[sample] app health check failed: %v\n", err)
				return
			}
			time.Sleep(500 * time.Millisecond) // brief pause after health before dialing
			fmt.Fprintf(os.Stderr, "\n[sample] Dialing %s...\n", runCallTo)
			var resp interface{}
			body := callcmd.BuildCreateBody(callcmd.CreateOpts{
				From:      phoneNumber,
				To:        runCallTo,
				AppID:     appID,
				AnswerURL: publicURL + "/webhooks/bandwidth/voice/initiate",
			})
			if err := voiceClient.Post(ctx, fmt.Sprintf("/accounts/%s/calls", acctID), body, &resp); err != nil {
				fmt.Fprintf(os.Stderr, "[sample] call failed: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "[sample] Call initiated — answer your phone!\n")
			}
		}()
	}

	if err := appCmd.Wait(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("app exited with error: %w", err)
	}
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// collectExtraEnv maps flag values and falls back to env vars for each extra
// env var the sample requires. Returns an error if a required value is missing.
func collectExtraEnv(name string, entry *SampleEntry) (map[string]string, error) {
	flagMap := map[string]string{
		"OPENAI_API_KEY": runOpenAIKey,
		"TRANSFER_TO":    runTransferTo,
	}
	result := make(map[string]string)
	var missing []string
	for _, p := range entry.ExtraEnv {
		val := flagMap[p.Key]
		if val == "" {
			val = os.Getenv(p.Key)
		}
		if val == "" {
			missing = append(missing, fmt.Sprintf("  --%s   (%s)", p.Flag, p.Description))
		} else {
			result[p.Key] = val
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("the %q sample requires additional flags:\n%s", name, strings.Join(missing, "\n"))
	}
	return result, nil
}

// checkPrerequisites verifies that git, ngrok, and the language runtime exist.
func checkPrerequisites(lang string) error {
	required := []string{"git", "ngrok"}
	switch lang {
	case "python":
		required = append(required, "python3")
	case "node":
		required = append(required, "node", "npm")
	case "java":
		required = append(required, "java", "mvn")
	}
	var missing []string
	for _, tool := range required {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required tools: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ensureCloned clones repoURL to ~/.band/samples/<name>-<lang>/ or pulls if
// it already exists. Returns the clone directory path.
func ensureCloned(ctx context.Context, repoURL, name, lang string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cloneDir := filepath.Join(home, ".band", "samples", name+"-"+lang)

	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err == nil {
		spin := ui.NewSpinner("Updating sample repo...")
		spin.Start()
		pullCmd := exec.CommandContext(ctx, "git", "pull", "--ff-only")
		pullCmd.Dir = cloneDir
		out, err := pullCmd.CombinedOutput()
		spin.Stop()
		if err != nil {
			fmt.Fprintf(os.Stderr, "git pull failed (%v), proceeding with existing clone\n%s\n", err, out)
		} else {
			ui.Successf("Repo up to date: %s", cloneDir)
		}
		return cloneDir, nil
	}

	if err := os.MkdirAll(filepath.Dir(cloneDir), 0o755); err != nil {
		return "", fmt.Errorf("creating samples directory: %w", err)
	}
	spin := ui.NewSpinner(fmt.Sprintf("Cloning %s...", repoURL))
	spin.Start()
	cloneCmd := exec.CommandContext(ctx, "git", "clone", repoURL, cloneDir)
	out, err := cloneCmd.CombinedOutput()
	spin.Stop()
	if err != nil {
		return "", fmt.Errorf("cloning %s: %w\n%s", repoURL, err, out)
	}
	ui.Successf("Cloned to %s", cloneDir)
	return cloneDir, nil
}

// waitForNgrok polls the ngrok local API until the tunnel URL is available.
func waitForNgrok(ctx context.Context, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		resp, err := http.Get("http://127.0.0.1:4040/api/tunnels")
		if err == nil {
			var data struct {
				Tunnels []struct {
					PublicURL string `json:"public_url"`
					Proto     string `json:"proto"`
				} `json:"tunnels"`
			}
			if json.NewDecoder(resp.Body).Decode(&data) == nil {
				resp.Body.Close()
				for _, t := range data.Tunnels {
					if strings.HasPrefix(t.PublicURL, "https://") {
						return t.PublicURL, nil
					}
				}
			} else {
				resp.Body.Close()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("timed out after %s", timeout)
}

// waitForHealth polls the app's health endpoint until it responds 2xx.
func waitForHealth(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 300 {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("app not healthy after %s", timeout)
}

// ensureVoiceApp find-or-creates a Voice-V2 application named appName with the
// given initiate and status callback URLs. Returns the application ID.
func ensureVoiceApp(ctx context.Context, client *api.Client, acctID, appName, initiateURL, statusURL string) (string, error) {
	var listResp interface{}
	if err := client.Get(ctx, fmt.Sprintf("/accounts/%s/applications", acctID), &listResp); err != nil {
		return "", fmt.Errorf("listing applications: %w", err)
	}
	if id := findAppByName(listResp, appName); id != "" {
		ui.Successf("Voice application (existing): %s", ui.ID(id))
		return id, nil
	}

	spin := ui.NewSpinner("Creating voice application...")
	spin.Start()
	var createResp interface{}
	body := api.XMLBody{
		RootElement: "Application",
		Data: map[string]interface{}{
			"ServiceType":              "Voice-V2",
			"AppName":                  appName,
			"CallInitiatedCallbackUrl": initiateURL,
			"CallStatusCallbackUrl":    statusURL,
		},
	}
	err := client.Post(ctx, fmt.Sprintf("/accounts/%s/applications", acctID), body, &createResp)
	spin.Stop()
	if err != nil {
		return "", fmt.Errorf("creating voice application: %w", err)
	}

	id := extractStringField(createResp, "ApplicationId", "applicationId")
	if id == "" {
		return "", fmt.Errorf("voice application created but no ID in response")
	}
	ui.Successf("Voice application: %s", ui.ID(id))
	return id, nil
}

// firstPhoneNumber returns the first in-service TN on the account.
// The Dashboard client already scopes requests to the account, so the path is
// just /tns (same as `band number list` uses internally).
func firstPhoneNumber(ctx context.Context, client *api.Client, acctID string) (string, error) {
	var resp interface{}
	if err := client.Get(ctx, "/tns?size=1&page=1", &resp); err != nil {
		return "", fmt.Errorf("listing phone numbers: %w", err)
	}
	tn := extractStringField(resp, "FullNumber", "TelephoneNumber", "telephoneNumber")
	if tn == "" {
		return "", fmt.Errorf("no phone numbers found on account %s", acctID)
	}
	if !strings.HasPrefix(tn, "+") {
		tn = "+" + tn
	}
	return tn, nil
}

// writeEnvFile writes env vars in `export KEY="VALUE"` format.
func writeEnvFile(path string, vars map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("writing .env: %w", err)
	}
	defer f.Close()
	for k, v := range vars {
		fmt.Fprintf(f, "export %s=%q\n", k, v)
	}
	return nil
}

// findAppByName searches a listing response for an application with the given name.
func findAppByName(resp interface{}, name string) string {
	data, _ := json.Marshal(resp)
	var list []map[string]interface{}
	if json.Unmarshal(data, &list) == nil {
		for _, item := range list {
			for _, k := range []string{"AppName", "appName"} {
				if v, ok := item[k].(string); ok && v == name {
					for _, idKey := range []string{"ApplicationId", "applicationId"} {
						if id, ok := item[idKey].(string); ok && id != "" {
							return id
						}
					}
				}
			}
		}
	}
	// Also try unwrapping a data envelope
	var envelope map[string]interface{}
	if json.Unmarshal(data, &envelope) == nil {
		if inner, ok := envelope["ApplicationList"].([]interface{}); ok {
			for _, raw := range inner {
				item, _ := raw.(map[string]interface{})
				for _, k := range []string{"AppName", "appName"} {
					if v, ok := item[k].(string); ok && v == name {
						for _, idKey := range []string{"ApplicationId", "applicationId"} {
							if id, ok := item[idKey].(string); ok && id != "" {
								return id
							}
						}
					}
				}
			}
		}
	}
	return ""
}

// extractStringField walks a JSON-marshalled response looking for any of the
// given keys, returning the first non-empty string value it finds.
func extractStringField(resp interface{}, keys ...string) string {
	data, _ := json.Marshal(resp)
	var m map[string]interface{}
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	return searchMap(m, keys...)
}

func searchMap(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	for _, v := range m {
		if nested, ok := v.(map[string]interface{}); ok {
			if found := searchMap(nested, keys...); found != "" {
				return found
			}
		}
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				if nested, ok := item.(map[string]interface{}); ok {
					if found := searchMap(nested, keys...); found != "" {
						return found
					}
				}
			}
		}
	}
	return ""
}
