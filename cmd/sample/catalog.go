package sample

// envPrompt describes an environment variable that a sample app requires beyond
// the standard Bandwidth credentials (which are auto-populated from band auth).
type envPrompt struct {
	Key         string // env var name written to .env
	Flag        string // CLI flag name (without --)
	Description string // shown in --help
}

// langSetup describes how to install dependencies and run the app for a given language.
type langSetup struct {
	// ReqFile is the path (relative to clone root) of the dependency manifest.
	ReqFile string
	// AppDir is the working directory to cd into before running.
	AppDir string
	// InstallCmd is the command to install dependencies (run once after clone).
	InstallCmd []string
	// RunCmd is the command to start the app (run in the AppDir).
	RunCmd []string
	// HealthPath is the HTTP path polled to detect a ready server.
	HealthPath string
}

// SampleEntry describes one sample app and its variants.
type SampleEntry struct {
	Description string
	// Repos maps language name → GitHub clone URL.
	Repos map[string]string
	// ExtraEnv lists environment variables required beyond BW_* credentials.
	ExtraEnv []envPrompt
	// Setup maps language name → build/run instructions.
	Setup map[string]langSetup
	// Port is the default local port the app listens on.
	Port int
}

// catalog is the authoritative list of runnable Bandwidth sample apps.
var catalog = map[string]*SampleEntry{
	"live-assistant": {
		Description: "Real-time AI voice assistant powered by OpenAI Realtime API",
		Repos: map[string]string{
			"python": "https://github.com/Bandwidth-Samples/openai-live-websockets-python",
		},
		ExtraEnv: []envPrompt{
			{Key: "OPENAI_API_KEY", Flag: "openai-key", Description: "OpenAI API key (must have Realtime API access)"},
			{Key: "TRANSFER_TO", Flag: "transfer-to", Description: "Phone number to transfer calls to (E.164, e.g. +19195551234)"},
		},
		Setup: map[string]langSetup{
			"python": {
				AppDir:     "app",
				InstallCmd: []string{".venv/bin/pip", "install", "-q", "-r", "app/requirements.txt"},
				RunCmd:     []string{".venv/bin/python3", "main.py"},
				HealthPath: "/health",
			},
		},
		Port: 3000,
	},
	"voice-gather": {
		Description: "In-call DTMF gather example",
		Repos: map[string]string{
			"java":   "https://github.com/Bandwidth-Samples/in-call-gather-java",
			"node":   "https://github.com/Bandwidth-Samples/in-call-gather-nodejs",
			"python": "https://github.com/Bandwidth-Samples/in-call-gather-python",
		},
		Setup: map[string]langSetup{
			"python": {
				AppDir:     ".",
				InstallCmd: []string{".venv/bin/pip", "install", "-q", "-r", "requirements.txt"},
				RunCmd:     []string{".venv/bin/python3", "app.py"},
				HealthPath: "/",
			},
			"node": {
				AppDir:     ".",
				InstallCmd: []string{"npm", "install", "--silent"},
				RunCmd:     []string{"node", "index.js"},
				HealthPath: "/",
			},
		},
		Port: 3000,
	},
}
