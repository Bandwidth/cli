# Bandwidth CLI — Agent Reference

> Structured reference for AI agents using the `band` CLI.
> Covers command semantics, dependency chains, idempotency, polling patterns, and limitations.

> **Note:** This document is self-contained so agents can operate from a single file. Some content overlaps with README.md by design — auth, exit codes, env vars, and error patterns are duplicated here so an agent never needs to cross-reference.

## Design Principles

These principles guide how the CLI is built. If you're contributing changes, maintain them:

- **`--plain` output must be stable and parseable.** Agents depend on flat JSON. Don't change the shape of `--plain` output without a migration path.
- **`--if-not-exists` for idempotency.** Any create command should support this flag so agents can retry safely.
- **`--wait` for async operations.** Agents can't poll — give them a way to block until the operation completes.
- **Structured exit codes.** Agents use exit codes for control flow, not string parsing. See [Exit Codes](#exit-codes).
- **Update this file.** If you add, remove, or change a command, update this file alongside the README.

## Scope

This CLI handles **provisioning, one-shot API operations, and state queries** against Bandwidth's platform. It can set up accounts, manage infrastructure, initiate calls, send messages, and retrieve results.

It **cannot** receive or respond to mid-call webhook callbacks or message delivery callbacks. An agent can start a call or send a message and later check metadata, but cannot dynamically control the conversation during a call. Real-time call control requires a separate callback-handling server. Message delivery status arrives via webhooks on your application — there is no polling endpoint.

## Authentication

The CLI uses OAuth2 client credentials. An agent can bootstrap itself without human interaction:

```bash
BW_CLIENT_ID=<id> BW_CLIENT_SECRET=<secret> band auth login
```

Login exchanges the credentials for a token, extracts accessible accounts from the JWT, and stores everything. If the credentials access multiple accounts, the first is selected by default. Override with `--account-id` on any command.

```bash
band auth switch <account-id>   # change active account (no re-auth needed)
band auth status                # verify auth state
```

### Credential Profiles

Store multiple credential sets under named profiles — useful when different roles or environments require different client credentials:

```bash
band auth login --profile admin   # store credentials under "admin" profile
band auth profiles                # list all stored profiles
band auth use admin               # switch the active profile
```

If your credentials are not bound to a specific account, the CLI will prompt you to pass `--account-id` explicitly. Always pass `--account-id` on every command in that case.

### Account Type and Capabilities

`band auth status --plain` returns structured JSON describing what the active account can do. The two fields agents care about most:

- **`build: true`** — this is a Bandwidth Build account. Voice-only, credit-based. Messaging, number ordering, sub-accounts, VCPs, 10DLC, and toll-free verification are not available; commands targeting those exit with code 4 and a clear message pointing at the upgrade path.
- **`capabilities`** — a derived map (`voice`, `messaging`, `numbers`, `vcp`, `campaign_management`, `tfv`, `app_management`) flipping `true`/`false` based on the credential's roles. Use this to gate work locally rather than discovering limits via 4xx errors.

Branch on these before attempting feature-gated work. The CLI also fails fast at the moment you try a restricted command, but checking capabilities up front avoids wasted setup.

### Account Hint

When multiple accounts or profiles are active, commands write a hint to stderr so you know which account is being targeted:

```
[account: 9901287 | profile: admin | env: test]
```

The environment is included in the hint only when credentials span multiple environments or the active environment is non-default. Single-environment users (e.g. customers on prod only) won't see it.

This is stderr only — it won't break piped output parsing.

**Environment variables for headless operation:**

| Variable | Purpose |
|----------|---------|
| `BW_CLIENT_ID` | OAuth2 client ID |
| `BW_CLIENT_SECRET` | OAuth2 client secret |
| `BW_ACCOUNT_ID` | Override active account |
| `BW_ENVIRONMENT` | API environment: `prod` (default), `test` |
| `BW_API_URL` | Override API base URL (overrides environment-based default) |
| `BW_VOICE_URL` | Override Voice API base URL (overrides environment-based default) |
| `BW_FORMAT` | Output format override |

**Config file location:** `~/.config/band/config.json` (XDG). Falls back to `~/.band/config.json` if the XDG path doesn't exist.

## Voice Provisioning: Two Paths

Bandwidth has two provisioning models. **Try the Universal Platform path first.** If it fails with a 403 or "Forbidden" error, fall back to the legacy path. If both fail, the account may not be contracted for voice — contact Bandwidth support.

### Universal Platform (default)

Uses Voice Configuration Packages (VCPs). Simpler — no site/location hierarchy needed for voice.

```
auth login
  └─→ app create (voice application with callback URL)
        └─→ vcp create (links to app via --app-id)
              └─→ number search → number order
                    └─→ vcp assign (attach numbers to VCP)
                          └─→ call create (requires --from, --app-id, --answer-url)
```

### Legacy

Uses the sub-account → location → application chain. Required for accounts not on the Universal Platform.

```
auth login
  └─→ subaccount create
        └─→ location create (requires --site)
              └─→ app create (voice application with callback URL)
                    └─→ number search → number order
                          └─→ call create (requires --from, --app-id, --answer-url)
```

### How to detect which path to use

1. Try `band vcp list --plain`. If it succeeds → Universal Platform, use VCPs.
2. If it returns exit code 2 (403 Forbidden) → either legacy account or missing VCP role.
3. Try `band app create --type voice ...`. If it succeeds → legacy path works.
4. If app create returns 409 with "HTTP voice feature is required" → the account doesn't have voice enabled. Contact Bandwidth support.

## Idempotency

**Use `--if-not-exists`** on create commands to make them safe for retries:

```bash
band subaccount create --name "My Site" --if-not-exists
band location create --site <id> --name "My Location" --if-not-exists
band app create --name "My App" --type voice --callback-url <url> --if-not-exists
band vcp create --name "My VCP" --if-not-exists
```

For `number order`, there is no `--if-not-exists` — check `band number list --plain` first.

All read operations (gets, lists, deletes) are safe to retry.

## Async Operations

Use `--wait` to block until completion:

```bash
band number order +19195551234 --wait                           # blocks until number is active (30s default)
band call create --from ... --to ... --wait --timeout 120       # blocks until call completes
band transcription create <call-id> <rec-id> --wait             # blocks until transcription ready (60s default)
```

All `--wait` commands support `--timeout <seconds>`. Exit code 5 on timeout.

## Output

**Always use `--plain` when parsing CLI output.** Default JSON reflects Bandwidth's API structure with deep nesting. `--plain` flattens it:

```bash
band number list --plain        # → ["+19193554167", "+19198234157", ...]
band subaccount list --plain    # → [{"Id":"152681","Name":"Subacct"}]
band app list --plain           # → [{"ApplicationId":"abc-123", ...}, ...]
band app get <id> --plain       # → {"ApplicationId":"abc-123", "AppName":"My App", ...}
```

List commands with `--plain` always return arrays, even for a single result. No type-checking needed.

**Auto-plain when piped:** When stdout is piped to another command (e.g., `band number list | jq ...`), `--plain` is automatically enabled. Agents running in pipelines don't need to pass the flag explicitly.

## Global Flags

| Flag | Purpose |
|------|---------|
| `--plain` | **Recommended for agents.** Flat, simplified JSON output |
| `--format <json\|table>` | Output format (default: json) |
| `--account-id <id>` | Override active account for this command |
| `--environment <name>` | API environment: prod, test |

## Behavioral Notes

For full flag/argument reference, use `band <command> --help`. This section covers non-obvious semantics that affect agent control flow.

### Messaging

- **`message send` runs preflight checks** that block the send when provisioning is wrong. Handle exit code 1 from preflight failures — the error message contains the fix command.
- **`message send` returns 202, not 200.** A 202 means "accepted for processing," not "delivered." An agent must not report delivery success based on a 202. Delivery confirmation arrives via webhooks on the callback server.
- **`message media upload` outputs the media URL to stdout.** Chain it: `MEDIA_URL=$(band message media upload image.png)` then pass to `--media`.
- **`message list` requires at least one filter** (`--to`, `--from`, `--start-date`, or `--end-date`). Calling with no filters returns a 400 error.
- **`message list` date filters require millisecond precision:** `2024-01-01T00:00:00.000Z`, not `2024-01-01T00:00:00Z`.

### Applications

- **`app assign` is required for messaging** — it links a messaging app to a location. Without it, messages silently vanish (202 accepted, never delivered, no error). Voice on UP doesn't need this (VCPs handle it), but messaging always does.
- **`app create --type messaging`** sets `MsgCallbackUrl`, not `CallInitiatedCallbackUrl`. The callback URL receives delivery webhooks.
- **`app update` auto-detects** whether the app is voice or messaging and sets the appropriate callback field.

### Numbers

- **`number order` costs money.** No undo — you must `number release` to give it back.
- **`number search` results are not reserved.** Between search and order, someone else can take the number.

### VCPs

- **`vcp delete` fails if numbers are assigned.** Move them first with `vcp assign <other-vcp-id> <numbers...>`.
- **`vcp assign` is an upsert.** Numbers already on another VCP are moved, not duplicated.

### Quickstart

- **Agents should not use `band quickstart`.** It creates real resources that cost money (orders a phone number), doesn't support `--if-not-exists` (running it twice creates duplicate resources and orders a second number), doesn't return structured output for each step, and can't be partially retried if it fails midway. Use the step-by-step provisioning workflows in the [Agent Workflows](#agent-workflows) section instead.

---

## Timeout Recovery

When `--wait` times out (exit code 5), the operation may have succeeded — the CLI just stopped waiting.

| Command | On timeout | Recovery |
|---------|-----------|----------|
| `number order --wait` | Number may be activating | Check `band number list --plain` — if the number appears, it completed. If not, retry the order. |
| `call create --wait` | Call may still be active | Check `band call get <call-id> --plain` — look at the `state` field. |
| `transcription create --wait` | Transcription may be processing | Check `band transcription get <call-id> <rec-id> --plain`. |

**General rule:** after a timeout, query the resource state before retrying. Don't blindly re-run a create that might have succeeded.

---

## Agent Workflows

### Build Registration: Create a new account from zero

Use when no credentials exist yet. The CLI submits the registration request; the remaining setup happens in the browser. **An agent cannot complete this flow autonomously** — it requires a human (or an agent with web/phone access) to finish.

```bash
band account register --phone +19195551234 --email you@example.com --first-name Jane --last-name Doe --accept-tos
# → registration submitted; remaining steps happen outside the CLI:
#   1. Check email for a registration link from Bandwidth
#   2. Enter the OTP code sent via SMS to verify the phone number
#   3. Set a password and enter the OTP code from the email
#   4. Go to Account > API Credentials to generate OAuth2 credentials
# → once credentials are available:
band auth login --client-id <id> --client-secret <secret>
band auth status   # confirm
```

**Important for agents:** Registration requires accepting the [Bandwidth Build Terms of Service](https://www.bandwidth.com/legal/build-terms-of-service/). Before passing `--accept-tos`, you **must** present the full Terms of Service URL to the user and get their explicit confirmation. Do not accept on the user's behalf without showing them the terms first. The flow should be:

1. Show the user: "Registration requires accepting the Bandwidth Build Terms of Service: https://www.bandwidth.com/legal/build-terms-of-service/"
2. Ask the user to review and confirm they accept
3. Only after confirmation, run the command with `--accept-tos`

After calling `band account register`, stop and tell the user they need to complete setup in their browser. Do not attempt to poll or wait — the next CLI step (`band auth login`) requires credentials that are only available after the human finishes the browser flow.

**After login, the account already has a voice app and a phone number.** Build accounts ship with both pre-provisioned. Run `band app list --plain` to discover the voice app — do **not** call `app create` or `number order` on a fresh Build account, you already have what you need to make a call. (`band number list` doesn't work on Build yet; the pre-provisioned number is reachable via the account portal and already wired to the default voice app.)

---

### Prerequisite Chains

Different operations have different prerequisites. Use this to determine what's needed:

**Voice (Universal Platform):**
```
account + auth
  └─→ app create (voice)
        └─→ vcp create (links to app)
              └─→ number search → number order
                    └─→ vcp assign
                          └─→ call create
```

**Voice (Legacy):**
```
account + auth
  └─→ subaccount create
        └─→ location create
              └─→ app create (voice)
                    └─→ number search → number order
                          └─→ call create
```

**Messaging (all accounts):**
```
account + auth
  └─→ subaccount (check existing first)
        └─→ location (check existing first)
              └─→ app create (messaging, with real callback URL)
                    └─→ app assign (link app to location)
                          └─→ 10DLC campaign (local numbers) or TFV approval (toll-free)
                                └─→ message send
```

**Key difference:** Voice on UP skips the sub-account/location hierarchy. Messaging always needs it, even on UP accounts.

---

### Diagnose: What state am I in?

When inheriting a partially-provisioned account, run these commands to assess what's set up:

```bash
band auth status --plain                    # logged in? which account?
band subaccount list --plain                # any sub-accounts?
band location list --site <site-id> --plain # any locations?
band app list --plain                       # any applications?
band number list --plain                    # any phone numbers?
band vcp list --plain                       # VCPs? (403 = legacy account, use sub-account path)
```

For messaging readiness, also check:
```bash
band tendlc campaigns --plain               # any 10DLC campaigns? (403 = see note below)
band tendlc number <number> --plain         # is a specific number registered?
band tfv get <number> --plain               # toll-free verification status?
```

**If `band tendlc` returns a 403:** This could mean one of three things — your credential lacks the Campaign Management role, your account doesn't have the Registration Center feature enabled, or messaging isn't enabled on the account. Contact your Bandwidth account manager to check your account configuration and request Registration Center access if needed.

---

### Universal Platform: Provision voice from scratch

```bash
band auth status                                                                    # 1. verify auth
band app create --name "Agent Voice" --type voice --callback-url <url> --if-not-exists --plain  # 2. create app
band vcp create --name "Agent VCP" --app-id <app-id> --if-not-exists --plain        # 3. create VCP linked to app
band number list --plain                                                            # 4. check existing numbers
# if no numbers:
band number search --area-code 919 --quantity 1 --plain
band number order <number> --wait                                                   # 5. order number
band vcp assign <vcp-id> <number>                                                   # 6. assign number to VCP
```

If step 2 fails with 409 "HTTP voice feature is required," or step 3 fails with 403, fall back to legacy.

### Legacy: Provision voice from scratch

```bash
band auth status                                                                        # 1. verify auth
band subaccount create --name "Agent Site" --if-not-exists --plain                      # 2. sub-account
band location create --site <site-id> --name "Agent Location" --if-not-exists --plain   # 3. location
band app create --name "Agent Voice" --type voice --callback-url <url> --if-not-exists --plain  # 4. app
band number list --plain                                                                # 5. check numbers
# if no numbers:
band number search --area-code 919 --quantity 1 --plain
band number order <number> --wait                                                       # 6. order number
```

### Provision messaging from scratch

**Messaging uses a different provisioning model than voice.** Voice on UP uses VCPs (no sub-account/location needed). Messaging always requires the sub-account → location → application chain — even on Universal Platform accounts. This is because phone numbers live inside locations (SIP peers), and messaging applications are linked to locations, not directly to numbers. Every number in a location inherits its messaging app. If you just completed the voice UP workflow, don't assume messaging follows the same pattern.

A fresh UP account typically has one sub-account and one location already created. Check before creating new ones:

```bash
band auth status                                                                           # 1. verify auth
band subaccount list --plain                                                               # 2. check existing sites
band location list --site <site-id> --plain                                                # 3. check existing locations

# If no sub-account or location exists (--if-not-exists returns the existing
# resource if one with the same name already exists — same output shape either way,
# so you can always parse the ID from the response):
band subaccount create --name "Agent Site" --if-not-exists --plain
band location create --site <site-id> --name "Agent Location" --if-not-exists --plain

# 4. Create a messaging application with a REAL callback URL
#    The CLI blocks sends if this URL is a placeholder like example.com or localhost.
band app create --name "Agent SMS" --type messaging --callback-url <your-callback-url> --if-not-exists --plain

# 5. Link the app to the location where your numbers live
band app assign <app-id> --site <site-id> --location <location-id>

# 6. Send (CLI checks campaign assignment automatically and blocks if missing)
band message send --from <number> --to <destination> --app-id <app-id> --text "Hello"
```

**Preflight failure recovery.** If step 6 fails, the error message contains the fix:

| Error contains | Cause | Fix |
|---|---|---|
| `"not linked to any location"` | App not assigned to a location | `band app assign <app-id> --site <id> --location <id>` |
| `"no working callback URL"` | Callback URL is placeholder or missing | `band app update <app-id> --callback-url <url>` |
| `"not assigned to any active 10DLC campaign"` | Number not on a campaign | `band tendlc campaigns --plain` to list campaigns; `band tnoption assign <number> --campaign-id <id>` to assign |
| `"toll-free verification status"` | TFV not approved | `band tfv get <number> --plain` to check status |

### Send a message

Once provisioning is set up, sending is straightforward:

```bash
band message send --from +19195551234 --to +15559876543 --app-id abc-123 --text "Hello from the agent"
# → preflight checks pass (app linked, callback URL valid, number on campaign)
# → returns JSON with message id, segmentCount, direction
```

**Message delivery is async and webhook-based.** The CLI cannot verify whether a message was actually delivered. A 202 means "accepted for processing." Delivery confirmations (`message-delivered`, `message-failed`) arrive via webhooks on the app's callback URL. **An agent should not report "message delivered" based on a 202 — only report "message sent."** True delivery status requires a callback server.

**Sending MMS with uploaded media:**

```bash
MEDIA_URL=$(band message media upload image.png)
band message send --from +19195551234 --to +15559876543 --app-id abc-123 --text "Check this out" --media "$MEDIA_URL"
```

**Group messaging** uses the same `send` command with multiple recipients:

```bash
band message send --from +19195551234 --to +15551234567,+15552345678 --app-id abc-123 --text "Team update"
```

**Listing messages** requires at least one filter and **millisecond-precision timestamps** (a common agent mistake):

```bash
# Correct — milliseconds in the timestamp:
band message list --from +19195551234 --start-date 2024-01-01T00:00:00.000Z --plain
# Wrong — this returns a 400:
band message list --from +19195551234 --start-date 2024-01-01T00:00:00Z --plain
```

### Make a call

```bash
band number list --plain                # → ["+19195551234", ...]
band app list --plain                   # → [{"ApplicationId":"abc-123", ...}, ...]
band call create --from +19195551234 --to +15559876543 --app-id abc-123 --answer-url <url>
# → returns JSON with callId

# IMPORTANT: always verify the call actually connected
band call get <call-id> --plain
# Check: state should be "active" or disconnectCause should be "hangup"
# If disconnectCause is "error" or errorMessage is "Service unavailable",
# the call never went out — try a different --from number or re-check provisioning.
```

**Calls can fail silently.** `call create` returns 200 with a callId even when the call fails immediately (e.g., number not properly provisioned, routing error). Always verify with `call get` before reporting success to the user.

### Check call outcome

```bash
band call get <call-id> --plain                                    # check state
band recording list <call-id> --plain                              # recordings
band transcription create <call-id> <rec-id> --wait --plain        # blocks until ready
```

**Interpreting call state:**

| `disconnectCause` | Meaning |
|---|---|
| `hangup` | Call connected and ended normally |
| `busy` | Callee was busy |
| `timeout` | No answer |
| `error` | Call never connected — check `errorMessage` for details |

### Find number-to-app mapping

**Look up a specific number's VCP:**
```bash
band number get +19195551234 --plain    # → shows VCP assignment and voice settings
```

**List all numbers on a VCP:**
```bash
band vcp numbers <vcp-id> --plain       # → numbers assigned to this VCP
```

**Legacy:**
```bash
band app peers <app-id> --plain         # → locations linked to app (includes SiteId)
band number list --plain                # → all numbers on account
```

## Exit Codes

| Code | Meaning | When |
|------|---------|------|
| 0 | Success | Command completed |
| 1 | General error | Missing flags, invalid input, unexpected failures |
| 2 | Auth error | 401 — bad credentials or token expired. Re-authenticate. |
| 3 | Not found | 404 — resource doesn't exist |
| 4 | Conflict / feature limit / payment required | 402, 409, or 403 due to a plan/role gate (e.g., Build account trying to message, missing VCP/Campaign Management/TFV role, out of credits, declined card). Non-retryable — stop and escalate to the user. |
| 5 | Timeout | `--wait` exceeded `--timeout` |
| 7 | Rate limited / quota exceeded | 429 or concurrent-resource ceiling. Back off and retry. |

**Use exit codes for control flow, not string parsing.**

## Error Patterns

| Error | Exit Code | Cause | Fix |
|-------|-----------|-------|-----|
| "not logged in" | 1 | No stored credentials | `BW_CLIENT_ID=x BW_CLIENT_SECRET=y band auth login` |
| "account ID not set" | 1 | No active account | `band auth switch <id>` or pass `--account-id` |
| "credential verification failed" | 2 | Bad client ID or secret | Check credentials |
| "API error 401" | 2 | Token expired or invalid | Re-run `band auth login` |
| "...isn't available on Bandwidth Build accounts" | 4 | Build account hit a feature outside its plan (messaging, numbers, VCPs, 10DLC, TFV) | Stop and tell the user — non-retryable. Upgrade path: https://www.bandwidth.com/talk-to-an-expert/ |
| "credential lacks the X role" | 4 | Credential lacks a role on a non-Build account | Escalate to the user's Bandwidth account manager to assign the role |
| "API error 402" / "Insufficient credits" | 4 | Out of credits, declined card, or no payment method on file | Stop and tell the user — non-retryable; they need to top up or fix billing |
| "API error 403" | 2 | True auth failure (token expired or invalid). Feature/role 403s now surface as exit 4 with a tailored message — see the rows above. | Re-run `band auth login` |
| "API error 404" | 3 | Resource doesn't exist | Verify the ID; check you're on the right account |
| "API error 409" | 4 | Conflict / duplicate | Use `--if-not-exists`; or feature not enabled on account |
| "API error 429" | 7 | Rate limited or quota exceeded | Back off and retry — eventually retryable |
| "HTTP voice feature is required" | 4 | Legacy voice not available | Try VCP path (UP account) or contact support |
| "required flag not set" | 1 | Missing a required flag | Check `--help` for required flags |

### Messaging delivery errors

These are **not CLI errors** — the CLI returns 0 (send was accepted). Delivery failures arrive via webhooks on your messaging application. Key error codes:

| Webhook error code | Meaning | Fix |
|---|---|---|
| **4476** | Source TN not registered to a 10DLC campaign | `band tnoption assign <number> --campaign-id <id> --wait` |
| **4770** | AT&T carrier block | Campaign reputation issue or content violation |
| **5620** | T-Mobile carrier block | Number not registered for 10DLC (T-Mobile blocks even inbound) |
| **5229** | TN-to-campaign provisioning error | Check sub-error: campaign suspended, TN on another campaign, or downstream partner error |

**An agent should never assume a 202 means delivery succeeded.** If delivery confirmation matters, the agent's callback server must listen for `message-delivered` or `message-failed` webhook events.

## 10DLC Registration (Registration Center)

These commands query the Registration Center API for 10DLC campaign and phone number registration status.

**Important:** These commands are for **import customers** — accounts that register campaigns through TCR and import them to Bandwidth. They require the **Campaign Management role** on your API credential and the **Registration Center feature** on your account.

**Direct customers** (accounts that register campaigns directly through Bandwidth) are not yet supported by these commands. Direct registration through the CLI is planned for mid-2026. In the meantime, direct customers should use the Bandwidth App or the existing Campaign Management API.

A 403 from `band tendlc` can mean: credential lacks the Campaign Management role, account doesn't have Registration Center, account is a direct customer, or messaging isn't enabled. The CLI parses the API response and gives a specific message for each case.

### Check if a number is registered for 10DLC

```bash
band tendlc number +19195551234 --plain
# → { "phoneNumber": "+19195551234", "campaignId": "CA3XKE1", "status": "SUCCESS", "brandId": "B1DER2J", ... }
```

Status values: `SUCCESS` (ready to send), `PROCESSING` (pending), `FAILURE` (registration failed).

### List all 10DLC campaigns

```bash
band tendlc campaigns --plain
# → [{ "campaignId": "CA3XKE1", "status": "SUCCESS", "brandId": "B1DER2J", ... }, ...]
```

### List all registered numbers (with filters)

```bash
band tendlc numbers --plain                           # all registered numbers
band tendlc numbers --campaign-id CA3XKE1 --plain     # numbers on a specific campaign
band tendlc numbers --status SUCCESS --plain           # only successfully registered numbers
band tendlc numbers --status FAILURE --plain           # numbers with registration failures
```

### List numbers on a specific campaign

```bash
band tendlc campaigns numbers CA3XKE1 --plain
```

### Diagnose messaging send failures

When `message send` fails with "not assigned to any active 10DLC campaign":

```bash
# 1. Check the specific number's registration
band tendlc number +19195551234 --plain

# 2. If not registered, list available campaigns
band tendlc campaigns --plain

# 3. Assign the number to a campaign
band tnoption assign +19195551234 --campaign-id CA3XKE1 --wait
```

**If `band tendlc` returns 403:** Don't retry — escalate. Tell the user: "Your credential may not have the Campaign Management role, or your account may not have the Registration Center feature enabled. Contact your Bandwidth account manager to check your configuration."

## Toll-Free Verification (TFV)

These commands manage toll-free number verification via the Athena v2 API. A 403 means the TFV role isn't enabled on the credential — contact your Bandwidth account manager to enable it.

### Check verification status

```bash
band tfv get +18005551234 --plain
# → { "status": "VERIFIED", "phoneNumber": "+18005551234", "submission": { ... } }
```

Status values: `VERIFIED` (approved, ready to send), `PENDING` (under review), `REJECTED` (resubmit needed).

### Submit a verification request

```bash
band tfv submit +18005551234 \
  --business-name "Acme Corp" \
  --business-addr "123 Main St" \
  --business-city "Raleigh" \
  --business-state "NC" \
  --business-zip "27606" \
  --contact-first "Jane" \
  --contact-last "Doe" \
  --contact-email "jane@acme.com" \
  --contact-phone "+19195551234" \
  --message-volume 10000 \
  --use-case "2FA" \
  --use-case-summary "Two-factor auth codes for user login" \
  --sample-message "Your Acme code is 123456" \
  --privacy-url "https://acme.com/privacy" \
  --terms-url "https://acme.com/terms" \
  --entity-type "PRIVATE_PROFIT"
```

### Diagnose toll-free messaging failures

When `message send` fails with toll-free verification issues:

```bash
# Check the number's TFV status
band tfv get +18005551234 --plain
# If PENDING → wait for carrier review
# If REJECTED → resubmit with corrected information
# If 404 → no verification request exists, submit one
```

## Short Codes

These commands query the Athena v2 API for short code registration and carrier activation status. Short codes are provisioned through carrier agreements outside the API — these commands are read-only.

### List short codes on the account

```bash
band shortcode list --plain
# → [{ "shortCode": "12345", "status": "ACTIVE", "country": "USA", "carrierStatuses": [...], ... }]
```

### Get details for a specific short code

```bash
band shortcode get 12345 --plain
band shortcode get 12345 --country CAN --plain    # Canadian short code
```

The response includes per-carrier activation status (`carrierStatuses` array), lease info, and which site/location the short code is assigned to. Status values: `ACTIVE`, `EXPIRED`, `SUSPENDED`, `INACTIVE`.

## TN Option Orders

TN Option Orders assign phone numbers to 10DLC campaigns (and can set other per-number options). This is the missing step between "number exists" and "number can send messages."

### Assign a number to a campaign

```bash
band tnoption assign +19195551234 --campaign-id CA3XKE1 --wait --plain
# → order completes when status is COMPLETE
```

Multiple numbers in one order:

```bash
band tnoption assign +19195551234 +19195551235 --campaign-id CA3XKE1 --wait
```

### Check order status

```bash
band tnoption get <order-id> --plain
# → { "ProcessingStatus": "COMPLETE", ... }
```

### List recent orders

```bash
band tnoption list --plain
band tnoption list --status FAILED --plain
band tnoption list --tn +19195551234 --plain
```

### Common errors

| Error code | Message | Cause | Fix |
|---|---|---|---|
| **1022** | "TelephoneNumber is in an invalid format" | Number not in E.164 format | Pass numbers with `+` prefix: `+19195551234` |
| **12220** | "Campaign has been rejected by DCA2" | Campaign failed carrier compliance review | Fix campaign compliance in the Bandwidth App, then retry |
| **5132** | "SMS attribute should be 'ON' for provisioning A2P" | SMS not enabled on the number's SIP peer/location | Enable SMS on the location in the Bandwidth App |
| **5133** | "A2P provisioning requires A2P on corresponding Sip peer" | Location not configured for A2P messaging | Enable A2P on the location in the Bandwidth App |

### Full messaging send-readiness workflow

```bash
# 1. Check if number is on a campaign
band tendlc number +19195551234 --plain

# 2. If not, find an available campaign
band tendlc campaigns --plain

# 3. Assign the number (use full E.164 format with + prefix)
band tnoption assign +19195551234 --campaign-id CA3XKE1 --wait

# 4. If assign fails with 5132/5133, SMS or A2P isn't enabled on the
#    number's location — this must be fixed in the Bandwidth App before retrying

# 5. Verify assignment
band tendlc number +19195551234 --plain
# → status should be SUCCESS

# 6. Now send
band message send --from +19195551234 --to +15559876543 --app-id abc-123 --text "Hello"
```

## Limitations

- **Bandwidth Build accounts are voice-only.** Detect via `band auth status --plain` (`build: true`). On a Build account, only voice and app-management commands work — `message send`, `number search`/`order`, `vcp *`, `subaccount *`, `tendlc *`, `tfv *` all exit 4 with a Build-aware message and an upgrade link. Pre-provisioned voice app and number ship with the account; `band number list` doesn't work yet (the number is reachable via the account portal). Build also has runtime limits not surfaced in `auth status` — verified-number-only outbound on Free Trial, a 30-min cap per call, a 5-concurrent-call ceiling. See [dev.bandwidth.com](https://dev.bandwidth.com/docs/voice/programmable-voice/build-free-trial) for current pricing and limits; treat any 402 (exit 4) as "out of credits, escalate" and any 429 (exit 7) as "back off and retry."
- **No real-time call control.** The CLI can initiate calls and query state, but cannot receive or respond to mid-call callbacks. Dynamic call control requires a separate callback-handling server.
- **No message delivery confirmation.** The CLI verifies your setup is correct before sending (app-location link, callback URL, campaign), but it cannot confirm whether a message was actually delivered. Delivery status (`message-delivered`, `message-failed`) arrives via webhooks on your callback server. The CLI's `message get` and `message list` return metadata only — not delivery status.
- **No message content retrieval.** Bandwidth does not store message bodies. After sending, the message text is gone forever. `message get` and `message list` return timestamps, direction, and segment counts only.
- **10DLC: read + assign only.** The CLI can list campaigns, check number registration status, diagnose failures (`band tendlc`), and assign numbers to campaigns (`band tnoption assign`). It cannot create campaigns or register brands — those require the Bandwidth App. The CLI checks that a number is on a campaign and blocks sends if it's not.
- **TFV is check-and-submit.** The CLI can check toll-free verification status and submit new requests (`band tfv`), but cannot approve or expedite reviews — those happen on the carrier side.
- **10DLC, TFV, and short code commands are role-gated.** A 403 can mean the credential lacks the required role (Campaign Management, TFV), the account doesn't have the Registration Center feature, or messaging isn't enabled. The CLI provides a diagnostic message — if it says "access denied," escalate to the Bandwidth account manager rather than retrying.
- **No batch operations.** Each command operates on one resource (except `vcp assign` which handles multiple numbers and `message send` which supports multiple recipients).
- **Dashboard API uses XML internally.** The CLI handles XML serialization transparently — you always send and receive JSON. Use `--plain` for predictable, flat output.
