# band — The Bandwidth CLI

Manage phone numbers, voice calls, and messaging from your terminal. No dashboard clicking, no API wrangling — just straightforward commands that get things done.

```sh
band call create --from +19195551234 --to +15559876543 --app-id abc-123 --answer-url https://example.com/answer
```

Built for humans, but agent-native from day one — every command supports `--plain` for flat JSON, `--if-not-exists` for safe retries, and `--wait` for async operations. If you're building an AI agent that provisions phone numbers or makes calls, this is the interface.

---

## Install

### Homebrew (macOS and Linux)

```sh
brew install Bandwidth/tap/band
```

### Go install

```sh
go install github.com/Bandwidth/cli/cmd/band@latest
```

### Download a binary

Pre-built binaries for macOS, Linux, and Windows are on the [GitHub Releases](https://github.com/Bandwidth/cli/releases) page.

### Docker

```sh
docker run --rm ghcr.io/bandwidth/cli:latest version
```

## Log in

The CLI uses OAuth2 client credentials — a **client ID** and **client secret** from the Bandwidth App.

```sh
band auth login --client-id <your-id> --client-secret <your-secret>
```

That's it. The CLI validates your credentials, figures out which accounts you can access, and stores everything in your OS keychain. If your credentials work with multiple accounts, you'll pick one.

### Switch accounts

```sh
band auth status          # see which account is active and what else is available
band auth switch          # pick a different account interactively
band auth switch 9901287  # or jump straight to one by ID
```

You can also pass `--account-id` to any command to override the active account for a single call.

### Credential profiles

Need to juggle multiple sets of credentials — say, one for each environment or role? Named profiles keep them organized:

```sh
band auth login --profile admin   # store credentials under "admin"
band auth profiles                # list all profiles
band auth use admin               # switch to the "admin" profile
```

When more than one account or profile is active, commands print an `[account: X | profile: Y]` hint to stderr so you always know which account you're targeting. It's stderr only, so it won't break scripts that parse stdout.

### Headless and CI/CD

Set environment variables instead of flags:

```sh
export BW_CLIENT_ID=<your-id>
export BW_CLIENT_SECRET=<your-secret>
band auth login
```

No TTY required. Accounts are auto-discovered from the OAuth2 token.

### Don't have a Bandwidth account yet?

You can sign up for a Bandwidth Build trial account from the CLI:

```sh
band account register --phone +19195551234 --email you@example.com --first-name Jane --last-name Doe
```

You'll be prompted to accept the [Bandwidth Build Terms of Service](https://www.bandwidth.com/legal/build-terms-of-service/) before registration proceeds. For scripted usage, pass `--accept-tos`.

Then complete setup in your browser:

1. Check your email for a registration link from Bandwidth
2. Enter the OTP code sent via SMS to verify your phone number
3. Set your password and enter the OTP code from your email
4. Go to **Account > API Credentials** to generate your OAuth2 credentials

Once your credentials are ready, run `band auth login` and you're off.

**What you get:** Every Build account ships with a voice application and a phone number already provisioned — no need to create them yourself. After login, run `band app list` and `band number list` to see them, and skip straight to [make a call](#make-a-call).

**Important note**:  a Bandwidth Build account is for our Voice API **only**. Usage limits and terms and conditions apply. If you would like to send
messages, order numbers, and more, you will need a full Bandwidth Account. [Talk to an expert](https://www.bandwidth.com/talk-to-an-expert/) to start 
your onboarding process today.

---

## What do I need?

Different tasks have different prerequisites. Here's what's required before you can do the main things:

| I want to... | I need... |
|---|---|
| **Make a call** | Bandwidth Build or full Bandwidth account + auth + voice application + phone number + VCP (or legacy site/location) + callback server |
| **Send a message** | Full Bandwidth account + auth + messaging application + phone number + app-location link + callback server + 10DLC campaign (local) or TFV approval (toll-free) |
| **Order a number** | Full Bandwidth account + auth |
| **Generate BXML** | Nothing — works offline, no auth needed |

If you don't have an account yet, start with `band account register` [above](#dont-have-a-bandwidth-account-yet) to start a free trial. Each walkthrough below builds up from auth.

---

## Your first call in 5 minutes

Already have phone numbers and an application set up? Skip to [make a call](#make-a-call).

Starting from scratch? Here's the full setup — you'll create a voice application, get a phone number, and place a call. This uses the **Universal Platform (VCP) path**, which is the default for new accounts. If your account uses the older sub-account model, see [legacy setup](#legacy-setup).

### 1. Create a voice application

An application tells Bandwidth where to send webhooks when something happens on a call. Point it at your server's callback URL.

```sh
band app create --name "My Voice App" --type voice --callback-url https://your-server.example.com/callbacks
```

### 2. Get a phone number

Search for available numbers, then order one:

```sh
band number search --area-code 919 --quantity 1
band number order +19195551234 --wait
```

The `--wait` flag blocks until the number is active, so you don't have to poll.

### 3. Connect the number to your application

Phone numbers don't do anything on their own — you need to tell Bandwidth how to handle calls to them. That's what a Voice Configuration Package (VCP) does. A VCP is a bundle of voice settings (routing rules, caller ID lookup, call verification) that you apply to a group of numbers. The most important setting is which application receives the webhooks.

```sh
band vcp create --name "My VCP" --app-id <your-app-id>
band vcp assign <vcp-id> +19195551234
```

Now when someone calls that number, Bandwidth routes the call to your application's callback URL.

If `vcp create` fails with a 403, your account uses the older sub-account model instead. See [legacy setup](#legacy-setup) below.

### 4. Make a call

> **Important:** The CLI starts calls and checks their status, but it can't control what happens *during* a call. That's your callback server's job. When Bandwidth reaches your `--answer-url`, your server responds with BXML (Bandwidth XML) — instructions like "say this," "gather digits," or "transfer to this number." If you don't have a callback server running, the call will connect but immediately hang up.

> **`--callback-url` vs `--answer-url`:** The `--callback-url` you set on the application (step 1) receives event webhooks — status changes, recordings, etc. The `--answer-url` you pass to `call create` is where Bandwidth fetches BXML instructions when the call connects. They can be the same URL or different endpoints on the same server.

```sh
band call create \
  --from +19195551234 \
  --to +15559876543 \
  --app-id <your-app-id> \
  --answer-url https://your-server.example.com/answer
```

Need a quick callback server to test with? Here's a minimal one in Node.js:

```js
// server.js — run with: node server.js
const http = require("http");
http.createServer((req, res) => {
  res.writeHead(200, { "Content-Type": "application/xml" });
  res.end(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <SpeakSentence>Hello from Bandwidth. Your call is working.</SpeakSentence>
</Response>`);
}).listen(3000, () => console.log("BXML server on port 3000"));
```

Expose it with a tool like ngrok (`ngrok http 3000`), then use that public URL as your `--answer-url`.

### Generate BXML

BXML (Bandwidth XML) is how you tell a call what to do — speak text, gather key presses, transfer to another number, or start recording. Your callback server responds with BXML, and Bandwidth executes it.

The CLI can generate BXML for you locally. No API calls, no auth required — it just prints XML to stdout.

```sh
band bxml speak "Thanks for calling. How can we help?"
band bxml speak --voice julie "Press 1 for sales."
band bxml gather --url https://example.com/gather --max-digits 1 --prompt "Press a key"
band bxml transfer +19195551234 --caller-id +19195550000
band bxml record --url https://example.com/done --max-duration 60
band bxml raw '<SpeakSentence>Hello</SpeakSentence>'   # validate and pretty-print XML
```

Pipe the output to a file, use it in tests, or serve it directly from your callback server:

```sh
band bxml speak "Hello, thanks for calling." > greeting.xml
```

---

## Your first message

Three steps: create a messaging app, link it to a location, send.

### 1. Create a messaging application

A messaging application tells Bandwidth where to send delivery status webhooks. Point it at your callback server.

```sh
band app create --name "My SMS App" --type messaging --callback-url https://your-server.example.com/callbacks
```

### 2. Link the app to a location

Phone numbers live inside locations, and messaging apps are linked to locations — not directly to numbers. Every number in a location inherits its messaging app.

```sh
band subaccount list                    # find your subaccount ID
band location list --site <site-id>     # find your location ID
band app assign <app-id> --site <site-id> --location <location-id>
```

### 3. Send a message

```sh
band message send --to +15551234567 --from +15559876543 --app-id <app-id> --text "Hello!"
```

The CLI runs preflight checks before sending and blocks you if something is wrong — you'll get a clear error message telling you exactly what to fix. If everything passes, you get a 202 response with the message ID.

> **A 202 means "accepted for processing," not "delivered."** Delivery status arrives via webhooks on your callback server. See [how messaging delivery works](#how-messaging-delivery-works) for details.

---

## Messaging provisioning details

The messaging quickstart above covers the happy path. This section covers what else can go wrong and how to diagnose it.

### Preflight checks

The CLI checks three things before every send:

| Check | What it verifies | What happens if it fails |
|-------|-----------------|-------------------------|
| **App-location link** | Messaging app is assigned to a location | Send blocked — tells you to run `band app assign` |
| **Callback URL** | App has a real callback URL (not `example.com`, `localhost`, etc.) | Send blocked — tells you to run `band app update --callback-url` |
| **Number registration** | 10DLC numbers are on an approved campaign; toll-free numbers have TFV approval | Send blocked — tells you what's missing |

### 10DLC campaigns (local numbers)

If you're sending from a standard 10-digit local number, it must be assigned to an approved 10DLC campaign. Without this, carriers will block your messages. The CLI detects this and blocks the send with a diagnostic message.

You can check registration status with `band tendlc`:

```sh
band tendlc number +19195551234 --plain    # check a specific number
band tendlc campaigns --plain              # list campaigns on your account
```

Campaign and brand registration happen in the Bandwidth App — see [dev.bandwidth.com](https://dev.bandwidth.com/docs/messaging/campaign-management/) for the full guide. Once you have a campaign, assign numbers to it with `band tnoption assign`.

### Toll-free verification (toll-free numbers)

Toll-free numbers must be verified before messages will deliver. Check status or submit a verification request:

```sh
band tfv get +18005551234 --plain          # check verification status
band tfv submit +18005551234 ...           # submit a new request (see band tfv submit --help)
```

### Why messaging needs sub-accounts and locations

Voice on the Universal Platform uses VCPs — no sub-account/location hierarchy needed. Messaging is different: it always goes through the sub-account → location → application chain, even on UP accounts. This is because phone numbers live inside locations (SIP peers), and messaging apps are linked to locations, not directly to numbers.

A fresh UP account typically has one sub-account and one location already created. Check before creating new ones.

---

## Common tasks

### Numbers

```sh
band number list                                              # list your numbers
band number search --area-code 919 --quantity 5               # search available numbers
band number order +19195551234 --wait                         # order (blocks until active)
band number activate +19195551234 --voice-inbound --wait      # turn on inbound voice
band number release +19195551234                              # release a number
```

### Messaging

```sh
# SMS
band message send --to +15551234567 --from +15559876543 --app-id abc-123 --text "Hello"

# MMS with media
band message send --to +15551234567 --from +15559876543 --app-id abc-123 --text "Check this out" --media https://example.com/image.png

# Group message
band message send --to +15551234567,+15552345678 --from +15559876543 --app-id abc-123 --text "Hey everyone"

# Pipe from stdin
echo "Alert: server is back up" | band message send --to +15551234567 --from +15559876543 --app-id abc-123 --stdin

# List messages and media
band message list --from +15559876543 --start-date 2024-01-01T00:00:00.000Z  # milliseconds required
band message media upload image.png     # prints media URL to stdout
```

### Calls

```sh
band call create --from +19195551234 --to +15559876543 --app-id abc-123 --answer-url https://example.com/answer
band call get <call-id>                             # check state
band call hangup <call-id>                          # hang up
band call update <call-id> --redirect-url <url>     # redirect active call
```

### Recordings and transcriptions

```sh
band recording list <call-id>
band recording download <call-id> <rec-id> --output call.wav
band transcription create <call-id> <rec-id> --wait
```

---

## Legacy setup

Some Bandwidth accounts use the older sub-account and location model instead of VCPs. If `band vcp list` returns a 403, you're on this path.

```sh
band subaccount create --name "My Subaccount"
band location create --subaccount <subaccount-id> --name "My Location"
band app create --name "My Voice App" --type voice --callback-url https://your-server.example.com/callbacks
band number search --area-code 919 --quantity 1
band number order +19195551234 --wait
```

Sub-accounts (formerly known as sites) are the top-level container. Locations (formerly known as SIP peers) sit inside sub-accounts and define where numbers get routed. The flow is: sub-account → location → application → number.

> **Want one-command legacy setup?** Run `band quickstart --callback-url <url> --legacy`. The default `band quickstart` uses the VCP path.

---

## Command reference

### Auth

| Command | What it does |
|---------|-------------|
| `band auth login` | Log in with OAuth2 credentials (use `--profile <name>` to store under a named profile) |
| `band auth logout` | Clear stored credentials |
| `band auth status` | Show auth state, active account, and accessible accounts |
| `band auth switch [id]` | Switch to a different account |
| `band auth profiles` | List all stored credential profiles |
| `band auth use <profile>` | Switch the active credential profile |

### Account registration

| Command | What it does |
|---------|-------------|
| `band account register` | Register a new Bandwidth account |

### Applications

| Command | What it does |
|---------|-------------|
| `band app create` | Create a voice or messaging application |
| `band app update <id>` | Update an application (e.g. change callback URL) |
| `band app assign <id>` | Link a messaging app to a location (`--site`, `--location`) |
| `band app list` | List all applications |
| `band app get <id>` | Get application details |
| `band app delete <id>` | Delete an application |
| `band app peers <id>` | Show locations linked to an app |

### Voice configuration packages

| Command | What it does |
|---------|-------------|
| `band vcp create` | Create a VCP |
| `band vcp list` | List all VCPs |
| `band vcp get <id>` | Get VCP details |
| `band vcp update <id>` | Update a VCP (name, description, linked app) |
| `band vcp delete <id>` | Delete a VCP |
| `band vcp assign <id> <number...>` | Assign (or move) numbers to a VCP |
| `band vcp numbers <id>` | List numbers on a VCP |

### Numbers

| Command | What it does |
|---------|-------------|
| `band number search` | Search available numbers by area code |
| `band number order <number...>` | Order numbers |
| `band number get <number>` | Get voice config details (including VCP assignment) |
| `band number activate <number...>` | Activate voice/messaging services (e.g. enable inbound) |
| `band number deactivate <number...>` | Deactivate voice/messaging services |
| `band number list` | List your in-service numbers |
| `band number release <number>` | Release a number |

### Messaging

| Command | What it does |
|---------|-------------|
| `band message send` | Send an SMS or MMS (supports group messaging and stdin) |
| `band message get <id>` | Get message metadata by ID |
| `band message list` | List messages (filter by `--to`, `--from`, `--start-date`, `--end-date`) |
| `band message media list` | List uploaded media files |
| `band message media upload <file>` | Upload a media file for MMS |
| `band message media get <id>` | Download a media file |
| `band message media delete <id>` | Delete a media file |

### Calls

| Command | What it does |
|---------|-------------|
| `band call create` | Start an outbound call |
| `band call list` | List calls |
| `band call get <id>` | Get call state |
| `band call update <id>` | Redirect an active call |
| `band call hangup <id>` | Hang up a call |

### Recordings and transcriptions

| Command | What it does |
|---------|-------------|
| `band recording list <call-id>` | List recordings for a call |
| `band recording get <call-id> <rec-id>` | Get recording metadata |
| `band recording download <call-id> <rec-id>` | Download the audio file |
| `band recording delete <call-id> <rec-id>` | Delete a recording |
| `band transcription create <call-id> <rec-id>` | Request a transcription |
| `band transcription get <call-id> <rec-id>` | Get the transcription |

### Sub-accounts and locations (legacy)

| Command | What it does |
|---------|-------------|
| `band subaccount create` | Create a sub-account (alias: `band site`) |
| `band subaccount list` | List sub-accounts |
| `band subaccount get <id>` | Get sub-account details |
| `band subaccount delete <id>` | Delete a sub-account |
| `band location create` | Create a location under a sub-account |
| `band location list` | List locations for a sub-account |

### TN Option Orders

| Command | What it does |
|---------|-------------|
| `band tnoption assign <number...>` | Assign phone numbers to a 10DLC campaign |
| `band tnoption get <id>` | Check the status of a TN Option Order |
| `band tnoption list` | List TN Option Orders (filter by `--status`, `--tn`) |

### Other

| Command | What it does |
|---------|-------------|
| `band quickstart` | One-command setup: creates app, orders number, wires everything up (use `--legacy` for sub-account path) |
| `band bxml <verb>` | Generate BXML locally (no auth needed) |
| `band version` | Print CLI version |

---

## Useful flags

| Flag | What it does |
|------|-------------|
| `--wait` | Block until an async operation finishes (ordering numbers, calls, transcriptions) |
| `--timeout <seconds>` | Set how long `--wait` should wait before giving up |
| `--if-not-exists` | Skip creating a resource if one with the same name already exists |
| `--plain` | Simplified, flat JSON output — great for scripting and piping |
| `--format json\|table` | Choose output format (default: json) |
| `--account-id <id>` | Override the active account for this command |
| `--environment <name>` | Target a different API environment (prod, test) |

---

## Environment variables

| Variable | What it does |
|----------|-------------|
| `BW_CLIENT_ID` | OAuth2 client ID |
| `BW_CLIENT_SECRET` | OAuth2 client secret |
| `BW_ACCOUNT_ID` | Override the active account |
| `BW_ENVIRONMENT` | API environment (prod, test) |
| `BW_FORMAT` | Default output format |
| `BW_API_URL` | Override the API base URL |
| `BW_VOICE_URL` | Override the Voice API base URL |

---

## Troubleshooting

**First step:** Run `band version` to check which version you're on. Include this when filing issues.

**"not logged in"** — Run `band auth login` with your credentials.

**"account ID not set"** — You're logged in but haven't picked an account. Run `band auth switch <id>` or pass `--account-id`.

**"credential verification failed"** — Your client ID or secret is wrong. Double-check them in the Bandwidth App.

**API error 401** — Your token expired. Run `band auth login` again.

**API error 403** — Your credentials don't have permission for this operation. Check your roles in the Bandwidth App. VCP commands need the VCP role specifically.

**API error 404** — The resource doesn't exist. Verify the ID and make sure you're on the right account.

**API error 409** — You're trying to create something that already exists, or a feature isn't enabled on your account. Use `--if-not-exists` on create commands to handle duplicates gracefully.

**"HTTP voice feature is required"** — Your account doesn't have voice enabled. Try the VCP path instead, or contact Bandwidth support.

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Bad input or unexpected error |
| 2 | Authentication or permission problem |
| 3 | Resource not found |
| 4 | Conflict (duplicate resource or missing feature) |
| 5 | Timed out waiting |

---

## How messaging delivery works

The CLI does everything it can to prevent misconfigured sends — but it **cannot confirm delivery**. Here's why and what to do about it.

When you run `band message send`, the CLI verifies your setup (app link, callback URL, campaign) and then submits the message to Bandwidth. Bandwidth returns 202 ("accepted for processing") and the CLI prints the message ID. That's where the CLI's visibility ends.

**What happens next is entirely between Bandwidth and the carrier.** Your callback server receives one of:
- `message-delivered` — carrier accepted the message
- `message-failed` — delivery failed (includes an error code like 4476, 4770, etc.)

If your callback server isn't running or can't be reached, these events are **lost forever**. There's no way to retroactively look them up. This is why the CLI blocks sends when there's no callback URL.

**For production use:** make sure your callback server logs every delivery event. The `message.id` in the webhook matches the `id` returned by `band message send`.

---

## How it works

- **Auto-plain when piped.** If you pipe `band` output to another command (`band number list | jq ...`), `--plain` is automatically enabled. No flag needed.
- **Color and spinners.** The CLI uses color and spinners for interactive UX. All color output goes to stderr, so it never pollutes piped stdout. Set `NO_COLOR=1` to disable.
- **Config file** lives at `~/.config/band/config.json` (XDG standard). Falls back to `~/.band/config.json` if that doesn't exist.

---

## For AI agents

This CLI is agent-native — not just "agent-compatible." The design principles:

- **`--plain` everywhere.** Flat, stable JSON output. Auto-enabled when stdout is piped, so agents in pipelines don't need the flag.
- **`--if-not-exists` for idempotency.** Create commands can be retried safely without duplicating resources.
- **`--wait` for async operations.** Agents can't poll. `--wait` blocks until the number is active, the call completes, or the transcription is ready.
- **Structured exit codes.** 0 success, 2 auth, 3 not found, 4 conflict, 5 timeout. Use exit codes for control flow, not string parsing.
- **Env-var-driven auth.** `BW_CLIENT_ID` + `BW_CLIENT_SECRET` — no interactive prompts required.

For the full agent reference — dependency chains, provisioning workflows, error patterns, and copy-pasteable scripts — see [AGENTS.md](AGENTS.md).

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, CI details, and guidelines.

Quick start:

```sh
git clone https://github.com/Bandwidth/cli.git
cd cli
make build   # compile → ./band
make test    # run tests
make lint    # run golangci-lint
```

---

For full API docs, visit [dev.bandwidth.com](https://dev.bandwidth.com).
