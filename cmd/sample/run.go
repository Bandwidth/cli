package sample

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
	runLanguage string
	runCallTo   string
	runPort     int
)

var runCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Download and run a sample application",
	Long: `Clones the sample app, wires up Bandwidth credentials from your active band
profile, starts an ngrok tunnel, and launches the app.

Samples that need their own configuration read it from the environment, the same
way BW_CLIENT_ID and BW_CLIENT_SECRET work. Run the command without them to see
which variables a given sample expects.

Python samples require Python 3.11+.

Pass --call-to to automatically dial a number once the app is ready. This is
only supported by samples that implement outbound call flows (e.g. voice-gather,
live-assistant). Inbound-only samples (e.g. voice-record) should be tested by
calling your Bandwidth number after the app starts.`,
	Example: `  band sample run live-assistant --language python
  OPENAI_API_KEY=sk-... TRANSFER_TO=+19195550100 band sample run live-assistant --language python
  OPENAI_API_KEY=sk-... TRANSFER_TO=+19195550100 band sample run live-assistant --language python --call-to +19195550101

  band sample run voice-gather --language python --call-to +19195550101
  band sample run voice-redirect-loop --language python
  band sample run voice-record --language python
  USER_NUMBER=+19195550100 band sample run voice-bridge --language python`,
	Args: cobra.ExactArgs(1),
	RunE: runSample,
}

func init() {
	runCmd.Flags().StringVarP(&runLanguage, "language", "l", "", "Language variant to run (required)")
	runCmd.Flags().StringVar(&runCallTo, "call-to", "", "Phone number to call after the app is ready (E.164)")
	runCmd.Flags().IntVar(&runPort, "port", 0, "Local port (default: from catalog)")
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
	if entry.Voice == nil {
		return fmt.Errorf("sample %q is not runnable yet — open a PR to add its provisioning and runtime wiring", name)
	}
	repoURL, ok := entry.Repos[runLanguage]
	if !ok {
		supported := make([]string, 0, len(entry.Repos))
		for l := range entry.Repos {
			supported = append(supported, l)
		}
		sort.Strings(supported)
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
	extraEnv, err := collectExtraEnv(name, runLanguage, entry)
	if err != nil {
		return err
	}

	// ── Prerequisites ────────────────────────────────────────────────────────
	pythonExe, err := checkPrerequisites(runLanguage, setup.MinPython)
	if err != nil {
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
		return fmt.Errorf("ngrok not found — install it with: brew install ngrok")
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

	statusPath := entry.Voice.StatusCallbackPath
	if statusPath == "" {
		statusPath = entry.Voice.InitiateCallbackPath
	}
	appID, err := ensureVoiceApp(ctx, dashClient, acctID, "band-sample-"+name,
		publicURL+entry.Voice.InitiateCallbackPath,
		publicURL+statusPath,
	)
	if err != nil {
		return err
	}

	phoneNumber, err := firstPhoneNumber(ctx, dashClient, acctID)
	if err != nil {
		return err
	}
	ui.Successf("Using number: %s", ui.ID(phoneNumber))

	if err := ensureVoiceAppOnNumberLocation(ctx, dashClient, acctID, phoneNumber, appID); err != nil {
		return err
	}

	// Inbound-only samples don't implement answer webhooks, so we can't auto-dial
	// them; the correct flow is to call the Bandwidth number after the app starts.
	if runCallTo != "" && entry.Voice.DialAnswerPath == "" {
		return fmt.Errorf("%q does not support --call-to (it handles inbound calls only). Start the app, then call %s", name, phoneNumber)
	}

	// ── Python venv setup (language-specific) ───────────────────────────────
	if runLanguage == "python" {
		if pythonExe == "" {
			return fmt.Errorf("python interpreter not resolved")
		}
		venvSpin := ui.NewSpinner("Creating Python virtual environment...")
		venvSpin.Start()
		venvCmd := exec.CommandContext(ctx, pythonExe, "-m", "venv", ".venv")
		venvCmd.Dir = cloneDir
		if out, err := venvCmd.CombinedOutput(); err != nil {
			venvSpin.Stop()
			return fmt.Errorf("creating venv: %w\n%s", err, out)
		}
		venvSpin.Stop()

		pipSpin := ui.NewSpinner("Upgrading pip...")
		pipSpin.Start()
		upgradeCmd := exec.CommandContext(ctx, filepath.Join(cloneDir, ".venv", "bin", "pip"), "install", "-q", "--upgrade", "pip", "setuptools", "wheel")
		upgradeCmd.Dir = cloneDir
		if out, err := upgradeCmd.CombinedOutput(); err != nil {
			pipSpin.Stop()
			return fmt.Errorf("upgrading pip: %w\n%s", err, out)
		}
		pipSpin.Stop()
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
		"BW_ACCOUNT_ID":           acctID,
		"BW_CLIENT_ID":            clientID,
		"BW_CLIENT_SECRET":        clientSecret,
		"BASE_URL":                publicURL,
		"BASE_CALLBACK_URL":       publicURL,
		"BW_VOICE_APPLICATION_ID": appID,
		"BW_NUMBER":               phoneNumber,
		"LOCAL_PORT":              strconv.Itoa(port),
		"LOG_LEVEL":               "INFO",
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
	appCmd.Env = os.Environ()
	for k, v := range envVars {
		appCmd.Env = append(appCmd.Env, k+"="+v)
	}
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
			answerURL := publicURL + entry.Voice.DialAnswerPath
			body := callcmd.BuildCreateBody(callcmd.CreateOpts{
				From:      phoneNumber,
				To:        runCallTo,
				AppID:     appID,
				AnswerURL: answerURL,
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

// collectExtraEnv reads each extra environment variable the sample requires.
// Values come from the environment rather than flags so that secrets are not
// exposed in the process argument list. Returns an error if any are missing.
func collectExtraEnv(name, lang string, entry *SampleEntry) (map[string]string, error) {
	result := make(map[string]string)
	var missing, missingKeys []string
	for _, p := range entry.ExtraEnv {
		val := os.Getenv(p.Key)
		if val == "" {
			missing = append(missing, fmt.Sprintf("  %s   (%s)", p.Key, p.Description))
			missingKeys = append(missingKeys, p.Key+"=...")
			continue
		}
		result[p.Key] = val
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf(
			"the %q sample requires these environment variables:\n%s\n\nset them in the environment, for example:\n  %s band sample run %s --language %s",
			name,
			strings.Join(missing, "\n"),
			strings.Join(missingKeys, " "),
			name,
			lang,
		)
	}
	return result, nil
}

// checkPrerequisites verifies that git, ngrok, and the language runtime exist.
// Returns the resolved Python interpreter path when lang=="python".
func checkPrerequisites(lang string, minPython string) (string, error) {
	required := []string{"git", "ngrok"}
	var pythonExe string
	switch lang {
	case "python":
		var version string
		var err error
		pythonExe, version, err = resolvePython(minPython)
		if err != nil {
			return "", err
		}
		ui.Successf("Using Python: %s (%s)", ui.ID(pythonExe), version)
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
		return "", fmt.Errorf("missing required tools: %s", strings.Join(missing, ", "))
	}
	return pythonExe, nil
}

type pythonVersion struct {
	Major int
	Minor int
	Patch int
}

func (v pythonVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func parsePythonVersion(s string) (pythonVersion, bool) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) < 2 {
		return pythonVersion{}, false
	}
	atoi := func(x string) (int, bool) {
		n, err := strconv.Atoi(x)
		return n, err == nil
	}
	maj, ok := atoi(parts[0])
	if !ok {
		return pythonVersion{}, false
	}
	min, ok := atoi(parts[1])
	if !ok {
		return pythonVersion{}, false
	}
	patch := 0
	if len(parts) >= 3 {
		if p, ok := atoi(parts[2]); ok {
			patch = p
		}
	}
	return pythonVersion{Major: maj, Minor: min, Patch: patch}, true
}

func (v pythonVersion) less(o pythonVersion) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}

func parseMinPython(min string) (pythonVersion, bool) {
	if min == "" {
		return pythonVersion{}, false
	}
	v, ok := parsePythonVersion(min + ".0")
	return v, ok
}

type pythonCandidate struct {
	Path    string
	Version pythonVersion
}

// resolvePython picks the newest python3.x on PATH that satisfies minPython (major.minor).
// If minPython is empty, it picks the newest python3.x available.
func resolvePython(minPython string) (string, string, error) {
	min, hasMin := parseMinPython(minPython)
	if minPython != "" && !hasMin {
		return "", "", fmt.Errorf("invalid minimum python version %q (expected major.minor, e.g. 3.11)", minPython)
	}

	// Collect candidate executable names from PATH: python3 plus any python3.N we can see.
	candidates := map[string]struct{}{"python3": {}}
	re := regexp.MustCompile(`^python3\.(\d+)$`)
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if name == "python3" || re.MatchString(name) {
				candidates[name] = struct{}{}
			}
		}
	}

	var found []pythonCandidate
	for name := range candidates {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		out, err := exec.Command(path, "-c", `import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}")`).CombinedOutput()
		if err != nil {
			continue
		}
		v, ok := parsePythonVersion(string(out))
		if !ok {
			continue
		}
		// Only consider python3.x
		if v.Major != 3 {
			continue
		}
		found = append(found, pythonCandidate{Path: path, Version: v})
	}

	if len(found) == 0 {
		return "", "", fmt.Errorf("python3 not found — install Python 3.11+")
	}

	sort.Slice(found, func(i, j int) bool { return found[j].Version.less(found[i].Version) }) // desc

	for _, c := range found {
		// Choose the newest version that satisfies the minimum.
		if !hasMin || !c.Version.less(min) {
			return c.Path, c.Version.String(), nil
		}
	}

	best := found[0]
	if hasMin {
		return "", "", fmt.Errorf("python %d.%d+ required, found %s at %s", min.Major, min.Minor, best.Version.String(), best.Path)
	}
	return best.Path, best.Version.String(), nil
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
		remoteCmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
		remoteCmd.Dir = cloneDir
		remoteOut, remoteErr := remoteCmd.CombinedOutput()
		if remoteErr == nil {
			remote := strings.TrimSpace(string(remoteOut))
			if remote != "" && remote != repoURL {
				backupDir := cloneDir + ".bak-" + strconv.FormatInt(time.Now().Unix(), 10)
				if err := os.Rename(cloneDir, backupDir); err != nil {
					return "", fmt.Errorf("existing sample clone has origin %q (expected %q), and moving it aside failed: %w", remote, repoURL, err)
				}
				ui.Successf("Existing clone origin mismatch; moved aside: %s", backupDir)
			}
		}
		if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err != nil {
			// cloneDir was moved aside above; fall through to clone below
		} else {
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
			// Many samples don't have a dedicated health endpoint; treat any
			// non-5xx response as "the server is up".
			if resp.StatusCode < 500 {
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
		// Keep the app but update its callback URLs for the current ngrok tunnel.
		spin := ui.NewSpinner("Updating voice application...")
		spin.Start()
		var result interface{}
		body := api.XMLBody{
			RootElement: "Application",
			Data: map[string]interface{}{
				"ServiceType":              "Voice-V2",
				"AppName":                  appName,
				"CallInitiatedCallbackUrl": initiateURL,
				"CallStatusCallbackUrl":    statusURL,
			},
		}
		err := client.Put(ctx, fmt.Sprintf("/accounts/%s/applications/%s", acctID, url.PathEscape(id)), body, &result)
		spin.Stop()
		if err != nil {
			return "", fmt.Errorf("updating voice application: %w", err)
		}
		ui.Successf("Voice application (updated): %s", ui.ID(id))
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

// ensureVoiceAppOnNumberLocation assigns the voice application to the phone
// number's current location (SIP peer) and enables HTTP origination on that
// location. This is the missing piece that makes inbound calls to the number
// reach the application's callbacks.
func ensureVoiceAppOnNumberLocation(ctx context.Context, client *api.Client, acctID, phoneNumber, appID string) error {
	siteID, peerID, err := numberLocation(ctx, client, phoneNumber)
	if err != nil {
		return err
	}

	body := api.XMLBody{
		RootElement: "SipPeerOriginationSettings",
		Data: map[string]interface{}{
			"VoiceProtocol": "HTTP",
			"HttpSettings": map[string]interface{}{
				"HttpVoiceV2AppId": appID,
			},
		},
	}
	path := fmt.Sprintf("/accounts/%s/sites/%s/sippeers/%s/products/origination/settings",
		acctID, url.PathEscape(siteID), url.PathEscape(peerID))

	spin := ui.NewSpinner("Assigning voice application to location...")
	spin.Start()
	var result interface{}
	if err := client.Post(ctx, path, body, &result); err != nil {
		spin.Stop()
		return fmt.Errorf("assigning voice application to location: %w", err)
	}
	spin.Stop()
	ui.Successf("Voice routing set on location: %s (site %s)", ui.ID(peerID), ui.ID(siteID))
	return nil
}

func numberLocation(ctx context.Context, client *api.Client, phoneNumber string) (siteID string, peerID string, err error) {
	var result interface{}
	if err := client.Get(ctx, fmt.Sprintf("/tns/%s/tndetails", url.PathEscape(phoneNumber)), &result); err != nil {
		return "", "", fmt.Errorf("getting number details for %s: %w", phoneNumber, err)
	}

	siteID = extractStringField(result, "SiteId", "SubAccountId", "SubaccountId", "siteId", "subAccountId", "subaccountId")
	peerID = extractStringField(result, "SipPeerId", "SippeerId", "SipPeerID", "sipPeerId", "sippeersId", "LocationId", "locationId")
	if siteID == "" || peerID == "" {
		return "", "", fmt.Errorf("could not determine site/location for %s (siteId=%q, sippeerId=%q)", phoneNumber, siteID, peerID)
	}
	return siteID, peerID, nil
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

// writeEnvFile writes env vars in .env format.
func writeEnvFile(path string, vars map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("writing .env: %w", err)
	}
	defer f.Close()
	for k, v := range vars {
		fmt.Fprintf(f, "%s=%s\n", k, strconv.Quote(v))
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
