package sample

// envPrompt describes an environment variable that a sample app requires beyond
// the standard Bandwidth credentials (which are auto-populated from band auth).
type envPrompt struct {
	Key         string // env var name, read from the environment and written to .env
	Description string // shown when the variable is missing
}

// langSetup describes how to install dependencies and run the app for a given language.
type langSetup struct {
	// AppDir is the working directory to cd into before running.
	AppDir string
	// InstallCmd is the command to install dependencies (run once after clone).
	InstallCmd []string
	// RunCmd is the command to start the app (run in the AppDir).
	RunCmd []string
	// HealthPath is the HTTP path polled to detect a ready server.
	HealthPath string
}

// voiceConfig describes how to wire a sample app up for Programmable Voice callbacks.
type voiceConfig struct {
	// InitiateCallbackPath is appended to the ngrok base URL and used as the voice
	// application's CallInitiatedCallbackUrl.
	InitiateCallbackPath string
	// StatusCallbackPath is appended to the ngrok base URL and used as the voice
	// application's CallStatusCallbackUrl. When empty, InitiateCallbackPath is used.
	StatusCallbackPath string
	// DialAnswerPath is appended to the ngrok base URL and used as the answer URL
	// when `band sample run --call-to` auto-initiates an outbound call.
	DialAnswerPath string
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
	// Voice is non-nil when the sample expects Programmable Voice callbacks.
	Voice *voiceConfig
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
			{Key: "OPENAI_API_KEY", Description: "OpenAI API key with Realtime API access"},
			{Key: "TRANSFER_TO", Description: "Phone number to transfer calls to (E.164, e.g. +19195550100)"},
		},
		Setup: map[string]langSetup{
			"python": {
				AppDir:     "app",
				InstallCmd: []string{".venv/bin/pip", "install", "-q", "-r", "app/requirements.txt"},
				RunCmd:     []string{".venv/bin/python3", "main.py"},
				HealthPath: "/health",
			},
		},
		Voice: &voiceConfig{
			InitiateCallbackPath: "/webhooks/bandwidth/voice/initiate",
			StatusCallbackPath:   "/webhooks/bandwidth/voice/status",
			DialAnswerPath:       "/webhooks/bandwidth/voice/initiate",
		},
		Port: 3000,
	},
	"voice-gather": {
		Description: "In-call DTMF gather example",
		Repos: map[string]string{
			"python": "https://github.com/Bandwidth-Samples/voice-gather-python",
			"node":   "https://github.com/Bandwidth-Samples/voice-gather-js",
			"java":   "https://github.com/Bandwidth-Samples/voice-gather-java",
		},
		Setup: map[string]langSetup{
			"python": {
				AppDir:     ".",
				InstallCmd: []string{".venv/bin/pip", "install", "-q", "-r", "requirements.txt"},
				RunCmd:     []string{".venv/bin/python3", "main.py"},
				HealthPath: "/",
			},
			"node": {
				AppDir:     ".",
				InstallCmd: []string{"npm", "i"},
				RunCmd:     []string{"npm", "start"},
				HealthPath: "/",
			},
			"java": {
				AppDir:     ".",
				InstallCmd: []string{"mvn", "package", "-q"},
				RunCmd:     []string{"mvn", "spring-boot:run"},
				HealthPath: "/",
			},
		},
		Voice: &voiceConfig{
			InitiateCallbackPath: "/callbacks/outbound/voice",
			DialAnswerPath:       "/callbacks/outbound/voice",
		},
		Port: 3000,
	},
	"voice-record": {
		Description: "Record a message from a call (voicemail-style)",
		Repos: map[string]string{
			"python": "https://github.com/Bandwidth-Samples/voice-record-python",
			"node":   "https://github.com/Bandwidth-Samples/voice-record-js",
			"java":   "https://github.com/Bandwidth-Samples/voice-record-java",
		},
		Setup: map[string]langSetup{
			"python": {
				AppDir:     ".",
				InstallCmd: []string{".venv/bin/pip", "install", "-q", "-r", "requirements.txt"},
				RunCmd:     []string{".venv/bin/python3", "main.py"},
				HealthPath: "/",
			},
			"node": {
				AppDir:     ".",
				InstallCmd: []string{"npm", "i"},
				RunCmd:     []string{"npm", "start"},
				HealthPath: "/",
			},
			"java": {
				AppDir:     ".",
				InstallCmd: []string{"mvn", "package", "-q"},
				RunCmd:     []string{"mvn", "spring-boot:run"},
				HealthPath: "/",
			},
		},
		Voice: &voiceConfig{
			InitiateCallbackPath: "/callbacks/inbound/voice",
			DialAnswerPath:       "/callbacks/inbound/voice",
		},
		Port: 3000,
	},
	"voice-bridge": {
		Description: "Mask a call by bridging the caller to a configured destination number",
		Repos: map[string]string{
			"python": "https://github.com/Bandwidth-Samples/voice-bridge-python",
			"node":   "https://github.com/Bandwidth-Samples/voice-bridge-js",
			"java":   "https://github.com/Bandwidth-Samples/voice-bridge-java",
		},
		ExtraEnv: []envPrompt{
			{Key: "USER_NUMBER", Description: "Phone number to bridge calls to (E.164, e.g. +19195550100)"},
		},
		Setup: map[string]langSetup{
			"python": {
				AppDir:     ".",
				InstallCmd: []string{".venv/bin/pip", "install", "-q", "-r", "requirements.txt"},
				RunCmd:     []string{".venv/bin/python3", "main.py"},
				HealthPath: "/",
			},
			"node": {
				AppDir:     ".",
				InstallCmd: []string{"npm", "i"},
				RunCmd:     []string{"npm", "start"},
				HealthPath: "/",
			},
			"java": {
				AppDir:     ".",
				InstallCmd: []string{"mvn", "package", "-q"},
				RunCmd:     []string{"mvn", "spring-boot:run"},
				HealthPath: "/",
			},
		},
		Voice: &voiceConfig{
			InitiateCallbackPath: "/callbacks/inbound/voice",
			DialAnswerPath:       "/callbacks/inbound/voice",
		},
		Port: 3000,
	},
	"voice-redirect-loop": {
		Description: "Indefinitely redirect a call until interrupted (demo of Redirect and UpdateCall)",
		Repos: map[string]string{
			"python": "https://github.com/Bandwidth-Samples/voice-redirect-loop-python",
			"node":   "https://github.com/Bandwidth-Samples/voice-redirect-loop-js",
			"java":   "https://github.com/Bandwidth-Samples/voice-redirect-loop-java",
		},
		Setup: map[string]langSetup{
			"python": {
				AppDir:     ".",
				InstallCmd: []string{".venv/bin/pip", "install", "-q", "-r", "requirements.txt"},
				RunCmd:     []string{".venv/bin/python3", "main.py"},
				HealthPath: "/",
			},
			"node": {
				AppDir:     ".",
				InstallCmd: []string{"npm", "i"},
				RunCmd:     []string{"npm", "start"},
				HealthPath: "/",
			},
			"java": {
				AppDir:     ".",
				InstallCmd: []string{"mvn", "package", "-q"},
				RunCmd:     []string{"mvn", "spring-boot:run"},
				HealthPath: "/",
			},
		},
		Voice: &voiceConfig{
			InitiateCallbackPath: "/callbacks/inbound/voice",
			StatusCallbackPath:   "/callbacks/inbound/voice/disconnect",
			DialAnswerPath:       "/callbacks/inbound/voice",
		},
		Port: 3000,
	},
}
