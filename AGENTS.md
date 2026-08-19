# Bandwidth CLI — Agent Reference

> Structured reference for AI agents using the `band` CLI.
> Covers command semantics, dependency chains, idempotency, polling patterns, and limitations.

> **Note:** This document is self-contained so agents can operate from a single file. Some content overlaps with README.md by design — auth, exit codes, env vars, and error patterns are duplicated here so an agent never needs to cross-reference.

## Design Principles

These principles guide how the CLI is built. If you're contributing changes, maintain them:

- **`--plain` output must be stable and parseable.** Agents depend on flat JSON. Don't change the shape of `--plain` output without a migration path.
- **`--if-not-exists` for idempotency, where a safe natural key exists.** Create commands support this flag when there's a stable identity to retry against. A command that lacks one omits the flag deliberately and documents why (see [Customer Profiles](#customer-profiles)) rather than risk a retry silently reusing the wrong resource.
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
- **`capabilities`** — a derived map (`voice`, `messaging`, `numbers`, `vcp`, `campaign_management`, `tfv`, `app_management`) flipping `true`/`false` based on the credential's roles. Use this to gate work locally rather than discovering limits via 4xx errors. This map is unchanged by SIP support — it stays boolean-only.

Branch on these before attempting feature-gated work. The CLI also fails fast at the moment you try a restricted command, but checking capabilities up front avoids wasted setup.

#### SIP capability (tri-state, not boolean)

SIP provisioning (`band sip realm ...`, `band sip credential ...`) needs **two** things: the `SIP Credentials` role on the credential, and account-level SIP configuration on the backend. Only the role is knowable from the JWT offline — the account-level setting can only be confirmed by calling the API. A plain boolean would conflate "don't know," "missing the role," and "has the role but the account isn't enabled," so `band auth status --plain` reports SIP as its own object instead of folding it into `capabilities`:

```json
"sip": { "status": "unknown", "reason": "role_present_not_probed" }
```

`status` is one of `available`, `unavailable`, or `unknown`. `reason` is a stable enumerated identifier — branch on it, not on prose:

| `reason` | `status` | Meaning |
|----------|----------|---------|
| `role_absent` | `unavailable` | Credential lacks the `SIP Credentials` role. |
| `role_present_not_probed` | `unknown` | Credential has the role, but `auth status` is offline and cannot confirm account-level configuration. |
| `account_not_enabled` | `unavailable` | Only returned by `band sip status` — the account has the role but SIP Credentials isn't enabled on the account. Contact Bandwidth support. |
| `probe_succeeded` | `available` | Only returned by `band sip status` — the account can use SIP provisioning. |
| `probe_failed` | `unknown` | Only returned by `band sip status` — the probe itself failed (e.g. rate limited or a server error); retry later. |

`band auth status` never calls the network, so it can only ever report `role_absent` or `role_present_not_probed` for `sip`. To resolve an `unknown`, run the explicit probe:

```bash
band sip status --plain
```

This issues one cheap `GET /realms` call. A `200` reports `available`/`probe_succeeded` (exit 0). Hitting error code `33004` ("account isn't setup for Sip Credentials") reports `unavailable`/`account_not_enabled` — and **exits 0**, because a successful probe that confirms a negative fact is not a command failure. Auth errors (401/403) exit 2 via the normal error path; rate limiting or server errors exit non-zero with `probe_failed`.

Important: `band sip status` **does not persist** its result anywhere. Run it again any time you need a fresh answer, and don't expect `band auth status` to start reporting anything other than `unknown` for a role-holding credential — that command stays fully offline by design.

#### 10DLC capability (tri-state, not boolean)

`band auth status --plain` reports `tendlc` as `{"status":..., "reason":...}` rather than a
boolean, for the same reason as SIP: access needs both the Campaign Management role and the
account-level Registration Center feature, and only the role is knowable offline.

- `unavailable` / `role_absent` — the credential lacks the Campaign Management role.
- `unknown` / `role_present_not_probed` — run `band tendlc status` to resolve.

`campaign_management` remains a plain boolean meaning "the credential holds the role", and
`customer_profiles` is a separate capability — the Customer Profiles Access role is distinct.

```bash
band tendlc status --plain
# → {"access":"available","mode":{"reason":"not_discoverable","status":"unknown"},"reason":"probe_succeeded"}
```

`band tendlc status` can emit any of these `reason` values. Every 403-derived result
exits **0** — the probe answered the question, even when the answer is negative — so
there is no stderr message to fall back on; this table is the only authoritative place
to learn what to do next. `probe_failed` is the sole exception: the probe couldn't
answer at all, so it exits non-zero and is the only reason worth retrying.

| `reason` | `access` | Exit code | Next action |
|---|---|---|---|
| `probe_succeeded` | `available` | 0 | Registration Center access confirmed. Proceed with `band tendlc` commands. |
| `registration_center_not_enabled` | `unavailable` | 0 | Account doesn't have the Registration Center feature. Escalate to your Bandwidth account manager to enable it — do not retry. |
| `campaign_management_not_enabled` | `unavailable` | 0 | Campaign Management/messaging is not enabled on the account. Escalate to your Bandwidth account manager — do not retry. |
| `role_absent` | `unavailable` | 0 | The credential lacks the Campaign Management role. Have an account manager assign the role — do not retry; a retry hits the same 403. |
| `access_denied` | `unavailable` | 0 | A 403 that didn't match any recognized cause. Escalate to your Bandwidth account manager — do not retry. |
| `probe_failed` | `unknown` | non-zero | The probe itself failed (rate limited, 5xx, or a transport error) — it could not answer the question. This is the only reason where retrying makes sense. |

**`mode` is always `unknown`, by design.** An account either registers campaigns *directly* or
*imports* them from TCR — never both — and that is a property of the account's Bandwidth setup,
not something the API exposes. Do not infer it, and do not try one path to see what happens.
If direct-vs-import was not given to you in the task, stop and ask the operator.

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
| `BW_MESSAGING_URL` | Override Messaging API base URL. Messaging is production-only — `--environment test` does NOT change the host (no test messaging endpoint exists); only this override does. |
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
                          └─→ number activate --voice-inbound (required for inbound)
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

`number order` requires `--subaccount <id>` (the orders API needs a sub-account to order into; see `band subaccount list`). There is no `--if-not-exists` — check `band number list --plain` first.

All read operations (gets, lists, deletes) are safe to retry.

## Async Operations

Use `--wait` to block until completion:

```bash
band number order +19195551234 --subaccount <subaccount-id> --wait   # blocks until number is active (30s default)
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

### SIP

- **A generated SIP password is shown exactly once.** `sip credential create --generate-password` prints a
  `password` field with `passwordShownOnce: true`. It cannot be retrieved later — `sip credential get` never
  returns it. If it is lost, run `sip credential rotate <credential-id> --realm <realm>`; this **preserves the
  credential ID**, so peers referencing it keep working.
- **Prefer `--password-stdin`.** When the caller owns the secret, retries are safe and the password is not
  echoed back (`passwordShownOnce: false`, no `password` field in the response). With `--generate-password`,
  `--if-not-exists` against an existing credential exits **8** (`ExitSecretUnavailable`) because the stored
  password cannot be recovered — an agent must not treat that as success.
- **A stdout write failure after a successful write also exits 8.** If the API accepts the create/rotate but the
  generated password cannot be written to stdout — a full pipe, a closed pipe, or a short write — the credential
  exists and its password is unrecoverable, which is exactly the exit-8 state and not the generic 1 a write
  error would otherwise produce. Recovery is the same as any exit 8: if you don't already have the credential
  ID, `band sip credential list --realm <realm> --plain` to find it, then
  `band sip credential rotate <credential-id> --realm <realm> --generate-password`. On rotate the ID is always
  known, so the error names the exact command to re-run. With a caller-supplied password nothing is lost, so a
  write failure there stays a plain write error.
- **The generated `password` is the FIRST key in JSON output.** `sip credential create` and
  `sip credential rotate` emit `password` ahead of `id`, `hostname`, and `appId` specifically so a stdout write
  that gets truncated part-way still delivers the only copy of the secret. This is the one place in `band sip`
  that does not route its JSON through the shared `emit` helper — every other SIP command does, and gets Go's
  alphabetical map-key order. The exception is deliberate: `emit` normalizes the payload to a map, and
  alphabetical ordering would put `appId` ahead of `password`. Redaction still runs on this path, explicitly,
  and the human-facing table output still goes through `emit`. Don't "fix" this back to `emit`, and don't rely
  on key order for any other command.
- **`--app-id` is validated as a UUID before anything else happens.** `sip credential create --app-id` must be
  a canonical 8-4-4-4-12 UUID. The check runs ahead of the realm lookup and ahead of reading or generating the
  password, so an invalid value fails deterministically with exit **1**, **zero HTTP requests**, and — the part
  that matters — **no password generated**. A typo in `--app-id` cannot burn a write-once secret. Get real IDs
  from `band app list --plain`. Omitting `--app-id` (or passing an empty value) is valid and means "unbound".
- **Exactly one of `--password-stdin`, `--password-file`, or `--generate-password` is required.** There is
  deliberately no `--password` flag — passing a secret via argv leaks it through shell history, process
  listings, CI logs, and agent transcripts.
- **A credential's username and application binding are immutable.** Changing either requires delete +
  recreate; `sip credential rotate` only replaces the password.
- **`sip realm delete` fails while credentials exist** (error 12666) — delete the credentials first. The
  account's **default realm cannot be deleted** (error 33006) — promote another realm first with
  `sip realm update <other-realm> --default=true`.
- **Realm `ACTIVE` does not guarantee the FQDN resolves in public DNS yet.** If the far end reports an
  unresolvable host immediately after creation, retry shortly.
- **`sip realm delete` without `--wait` reports `accepted: true, deleted: false`.** A 202 means the delete
  was accepted, not that it completed. Only `--wait` promotes `deleted` to `true`, after confirming the realm
  is actually gone. Don't treat a bare delete as teardown-complete.
- **`sip realm delete` returns the realm's canonical ID, not the ref you passed.** The argument accepts an ID,
  a short name, or an FQDN, but the `--plain` `id` field is always the resolved numeric realm ID:
  `band sip realm delete vapi --plain` returns that ID, not `"vapi"`. The command resolves the ref with a GET
  before issuing the delete, so this costs one extra request on the delete path — the trade for an `id` field
  that always means an ID. Don't match the returned `id` against the string you passed in.
- **`sip realm create --if-not-exists` does not always silently reuse.** If a realm with that name exists but
  its `default` or `description` differs from what was requested, the command exits **4** instead of reusing
  it. Reconcile with `sip realm update <realm> --description <value>` (or promote it with `--default=true`)
  rather than retrying create. Realm names match case-insensitively, so `--name VAPI` reuses an existing
  `vapi`.
- **`sip realm create --if-not-exists --wait` is safe to combine.** A reused realm that is still
  `CREATE_PENDING` is polled to `ACTIVE` before the command returns, so a re-run after a `--wait` timeout
  cannot hand back exit 0 with a realm that is not yet usable.
- **`sip credential list` warns on stderr if it may be truncated.** Pagination is not implemented; a realm
  with more than 500 credentials returns only the first page, and the CLI says so on stderr. `--plain` stdout
  stays clean JSON.

### Quickstart

- **Agents should prefer the step-by-step provisioning workflows over `band quickstart`.** Quickstart creates real resources that cost money (it orders a phone number). The default (VCP) path is idempotent — re-running reuses existing resources via find-or-create and will not order a second number — and on failure it prints the resource IDs created so far (`status: partial`, see below). Re-running reuses the app/VCP/sub-account/location — but a number that was ordered and then failed to assign to the VCP is NOT auto-reassigned; finish it with `band vcp assign <vcp-id> <number>`. The `--legacy` path is NOT idempotent (re-running it may order an additional number). Because quickstart bundles several steps behind one command, prefer the step-by-step provisioning workflows in the [Agent Workflows](#agent-workflows) section when you need per-step structured output or fine-grained control.

- **`band quickstart` output `status` values** (VCP path only — `--legacy` is not idempotent):
  - `complete` — all resources created and number assigned; ready to use.
  - `complete_no_number` — resources created but no number was available in the requested area code; re-run with `--area-code` to try a different code.
  - `partial` — quickstart stopped after a failure but printed the resource IDs it created so far (app, VCP, sub-account, location, and possibly an ordered phone number). Re-running reuses the app/VCP/sub-account/location via idempotency checks. **Caveat:** if a number was ordered but its VCP assignment failed, the number is printed under `phoneNumber` but is NOT auto-reassigned on re-run (a re-run would order a *new* number) — finish the existing one with `band vcp assign <vcp-id> <phoneNumber>`.

---

## Timeout Recovery

When `--wait` times out (exit code 5), the operation may have succeeded — the CLI just stopped waiting.

| Command | On timeout | Recovery |
|---------|-----------|----------|
| `number order --wait` | Number may be activating | Check `band number list --plain` — if the number appears, it completed. If not, retry the order. |
| `number activate --wait` / `number deactivate --wait` | Service activation order may still be RECEIVED/PROCESSING | Check `band number get <number> --plain` — the `inboundActivated` / `outbound*Activated` flags reflect the terminal state. Re-running the same activate is idempotent. |
| `call create --wait` | Call may still be active | Check `band call get <call-id> --plain` — look at the `state` field. |
| `transcription create --wait` | Transcription may be processing | Check `band transcription get <call-id> <rec-id> --plain`. |
| `sip realm create --wait` | Realm may still be CREATE_PENDING | Check `band sip realm get <id> --plain` — `status` reflects the terminal state. Re-running create with `--if-not-exists` is safe. |
| `sip realm delete --wait` | Delete may still be in progress | Check `band sip realm get <id> --plain`; a not-found result means the delete completed. |
| `portin validate-tf --wait` | TF validation order may still be PROCESSING | Check `band portin validate-tf <numbers> --plain` again — caching means a re-run is cheap. |
| `portin submit --wait` | Order may still be in VALIDATE_TFNS | Check `band portin get <order-id> --plain` — look at the `status` field. |
| `portin supp` | Supp's propagation poll timed out | Check `band portin get <order-id> --plain`. The CLI's silent-fail check (error code 7300) only runs against the GET it observed before timeout — re-run the GET before retrying the supp. |
| `portin bulk get-tns --wait` | TN list may still be VALIDATE_DRAFT_TNS | Re-run `band portin bulk get-tns <id> --plain`. |

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
                          └─→ number activate --voice-inbound
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

**SIP interconnect (third-party voice-AI platform):**
```
account + auth
  └─→ sip realm create --wait            (returns the realm FQDN + id)
        └─→ sip credential create --realm <id> --username <u> --password-stdin
              └─→ vcp create|update --route-endpoint <partner-fqdn> --route-endpoint-type FQDN
                    └─→ vcp assign <vcp-id> <number>
                          └─→ number activate --voice-inbound
```

The realm's `hostname` is the outbound SIP address the far end needs — hand it to
the third-party platform so their SIP trunk can reach your account. Credentials
can only be created on an `ACTIVE` realm — without `--wait`, `sip realm create`
may still be `CREATE_PENDING` when the next step runs, and `sip credential create`
fails its own client-side check with `"realm <name> is <status> — credentials can
only be created on ACTIVE realms; retry after 'band sip realm get <name>' reports
ACTIVE"` and exits **1**. (In the narrow case where the API itself rejects the
create in a race, error 23022 surfaces instead — see the errors table below.)

**10DLC (Registration Center, direct):**
```
account + auth (Registration Center feature + Campaign Management role)
  └─→ customer-profile create          (one profile per brand — 1:1, not reusable)
        └─→ tendlc brand create --wait  (must reach VERIFIED or VETTED_VERIFIED)
              └─→ tendlc campaign create        (PR 4 — not yet in the CLI)
```

See [10DLC Brands](#10dlc-brands) for `brand create`'s flag matrix, `--wait`
semantics, and the customer-profile pre-flight; [10DLC Vettings](#10dlc-vettings)
for the optional vetting step some brand/campaign combinations require.

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
band number order <number> --subaccount <subaccount-id> --wait                                                   # 5. order number
band vcp assign <vcp-id> <number>                                                   # 6. assign number to VCP
band number activate <number> --voice-inbound --wait                                # 7. enable inbound voice
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
band number order <number> --subaccount <subaccount-id> --wait                                                       # 6. order number
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

### Port a number into Bandwidth

Six end-to-end flows are completable via the public API. Anything outside this list (port-out, manual toll-free, internal toll-free, NASC overrides, international ports) requires Bandwidth ops or the Dashboard — `band portin` will not let you start those flows.

**1. Check toll-free portability before submitting an order:**

```bash
band portin validate-tf +18005551234 --wait --plain
# → [{"telephoneNumber":"+18005551234","portable":true,"respOrgId":"TST51","reason":""}]
# Exits 1 with the per-number reason if any number is non-portable.
```

**2. On-net domestic port-in (BWC000) end-to-end:**

```bash
band portin create \
  --numbers +19195551234,+19195551235 \
  --site <site-id> --peer <peer-id> \
  --foc 2026-06-01T15:30:00Z \
  --loa-authorizing-person "Jane Doe" \
  --loa ./loa.pdf \
  --customer-order-id agent-run-42 --if-not-exists --plain
# → {"orderId":"...","status":"DRAFT","numbers":["+19195551234","+19195551235"], ...}

ORDER_ID=$(... extract from above ...)
band portin submit $ORDER_ID --wait --plain
# Blocks until status leaves VALIDATE_TFNS — SUBMITTED means validation passed and
# the order is with the vendor; INVALID_TFNS means one or more TFNs are not portable.

band portin get $ORDER_ID --plain
# Re-poll later for FOC progression. Don't try to --wait for FOC; can take days.
```

**3. Toll-free Phase 1 port-in:** Same shape as on-net. The TF validation phase runs automatically. Requires `TOLL_FREE_AUTOMATION_PHASE_1` enabled on the account — without it, `create` exits 4 with a message naming the gate. Don't retry on exit 4; escalate to the Bandwidth account manager.

**4. Bulk port-in:**

```bash
band portin bulk create --numbers-file ./tns.txt --site <id> --peer <id> \
  --customer-order-id agent-bulk-42 --if-not-exists --plain
# → {"bulkOrderId":"...","status":"VALIDATE_DRAFT_TNS","childOrderIds":[], ...}

band portin bulk get-tns <bulk-order-id> --wait --plain
# Blocks until VALID_DRAFT_TNS or INVALID_DRAFT_TNS. childOrderIds populates with
# one ID per validated group — drive each through `band portin get`/`submit`.
```

`bulk create` is two API calls under the hood: a template POST (`/bulkPortins`)
followed by a TN list PUT (`/bulkPortins/{id}/tnList`). If the second call
fails, the template order still exists as a TN-less `DRAFT` — unsubmitted
drafts expire after 2 days. Re-run the same command with
`--customer-order-id <id> --if-not-exists` to resume: the CLI finds the
stranded `DRAFT` and attaches the TN list to it instead of creating a new
template.

**5. Modify an existing order (supp):**

```bash
band portin supp <order-id> --foc 2026-07-01T15:30:00Z
# supp requires a full FOC timestamp (YYYY-MM-DDTHH:MM:SSZ); date-only values are rejected.
# `supp` always polls for propagation by default — it captures the order's
# pre-PUT lastModifiedDate, then waits until either that timestamp advances
# (real propagation) or error code 7300 appears (silent failure on the
# Bandwidth side, typically wireless_to_wireless after FOC). Exits 1 with
# a clear message on 7300; exits 5 on timeout. Do not assume success
# without running this command — a raw PUT can succeed without propagating.
```

**6. Lifecycle ops:**

```bash
band portin upload-loa <order-id> ./loa.pdf       # post-creation document upload
band portin notes add <order-id> "Please expedite — customer outage"
band portin notes list <order-id> --plain
band portin history <order-id> --plain            # state change audit
band portin cancel <order-id>                      # typically irreversible
```

**Idempotency.** `create` and `bulk create` accept `--customer-order-id <id> --if-not-exists`. On retry, an existing order with the same ID is returned with the same `--plain` shape — safe inside an agent reconciliation loop. For `bulk create`, if the retried order is a TN-less `DRAFT` (a stranded template from a prior step-2 failure), the CLI attaches the TN list instead of just returning it.

**Out of scope (will not work via API):**

| Flow | What happens if you try | Where to go instead |
|---|---|---|
| Port-out | No `band portout` exists; not a public API | Bandwidth Dashboard; ops |
| Toll-free Phase 2 / non-automated | `create` exits 4 with the Phase 1 gate message | Bandwidth ops |
| Toll-free internal port (BW account → BW account) | `create` will succeed creating a draft, but FOC requires manual provisioning | Bandwidth ops |
| International / non-NANP | Country-specific manual forms | Per-country ops process |
| NASC manual override | Email Somos Helpdesk | Internal ops process |

### Porting reference

**`--plain` shapes (v1, locked).** Field names will not change without a `--plain-version` migration.

| Command | Shape |
|---|---|
| `validate-tf` | `[{telephoneNumber, portable, respOrgId, reason}]` — array always, even for one TN |
| `create` / `get` / `submit` / `supp` | `{orderId, status, focDate, numbers, customerOrderId, errorCode}` |
| `list` | array of the create/get shape |
| `history` | `[{state, timestamp, actor}]` |
| `notes add` | `{orderId, noteId, location}` |
| `notes list` | `[{noteId, timestamp, actor, text}]` |
| `cancel` | `{orderId, status}` (always `status: "CANCELLED"`) |
| `upload-loa` | `{orderId, file, contentType, status}` (always `status: "UPLOADED"`) |
| `bulk create` / `bulk get` / `bulk get-tns` | `{bulkOrderId, status, childOrderIds, portableNumbers, nonPortable}` where `nonPortable: [{number, code, reason}]` |
| `bulk list` | array of the bulk create/get shape |

**Port-in state machine.** Poll `status` from `band portin get`:

```
DRAFT
  → VALIDATE_DRAFT_TFNS (TF validation running)
      → VALID_DRAFT_TFNS    (ready for submit)
      → INVALID_DRAFT_TFNS  (terminal — fix TNs, recreate)
  → (after `submit`)
      → SUBMITTED → VALIDATE_TFNS
          → SUBMITTED       (TFN validation passed)
          → INVALID_TFNS    (terminal — fix TNs)
      → PENDING_DOCUMENTS / PENDING_CARRIER_APPROVAL → FOC → COMPLETE
                                                (success path; FOC takes days)
      → EXCEPTION                              (terminal — read errorCode)
  → CANCELLED / REQUESTED_CANCEL  (terminal — from explicit `cancel`)
```

`band portin submit --wait` blocks until the order leaves `VALIDATE_TFNS`: `SUBMITTED`, `INVALID_TFNS`, or a later state. It does **not** wait for `COMPLETE` — that requires the FOC date to arrive, which is days to weeks out.

**Reconciliation idiom.** Tag every create with a unique customer-order-id; retries are then idempotent:

```bash
COID="agent-run-$(uuidgen)"
band portin create --numbers +1... --site <id> --peer <id> --foc <date> \
  --customer-order-id "$COID" --if-not-exists --plain
# On retry: returns the existing order's plain shape, same orderId. No duplicate.
```

**Common error codes encountered on porting endpoints:**

| Code | Where | Meaning | Fix |
|---|---|---|---|
| 1022 | any | TN format invalid | Pass numbers in full E.164 with country code (`+18005551234`, not `8005551234`) |
| 5217 | `notes add` | UserId required | Auto-handled by the CLI — should not surface unless config is corrupted |
| 7300 | `supp` (verifying GET) | Supp accepted by API but not propagated to Neustar | Order is in a state where supps are blocked (e.g., wireless_to_wireless past FOC). The CLI exits 1 — do not retry blindly; investigate the order state |
| 7615 | `validate-tf`, `create` (TF) | Invalid toll-free number | TN is malformed or out of TF range |
| 7626 | `validate-tf` | Toll-free vendor timeout (300s) | Transient — retry the validation |
| 7640 | `upload-loa` | documentType not specified | The CLI defaults to `documentType=LOA` — should not surface |
| 7642 | `validate-tf`, `bulk` | TF in spare status, not portable | Number must be acquired through ordering, not porting |
| 7643 | `validate-tf`, `bulk` | TF in unavailable status | Reserved by SOMOS — not portable |
| 7671 | `get`, `list` | Order was cancelled | Visible in `errorCode` on a cancelled order; not actionable |

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
| 8 | Secret unavailable | A resource exists but its secret cannot be recovered. Three producers: (1) `sip credential create --if-not-exists --generate-password` against an existing credential (the credential ID is known — the error names it directly); (2) a generated-password write whose response was lost; (3) a generated-password write that the API accepted but whose password could not be written to stdout — full pipe, closed pipe, or short write — leaving a credential nobody holds the password for. Not retryable as-is: rotate the credential (`sip credential rotate <credential-id> --realm <realm>`) to get a usable password. When the ID isn't known yet — the lost-response and failed-write cases on create; the command's own error message says so — run `band sip credential list --realm <realm> --plain` first to find it, then rotate. On rotate the ID is always known, so the error names the exact rotate command. |

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
| "toll-free port-ins via the API require Phase 1 automation" | 4 | Account doesn't have `TOLL_FREE_AUTOMATION_PHASE_1` enabled | Stop — escalate to the Bandwidth account manager. The number must be ported through the Dashboard or ops. |
| "supplement was accepted by the API but did not propagate to Neustar" | 1 | `band portin supp` detected error code 7300 on the verifying GET | The supp did NOT take effect. Typical cause: order is past FOC for wireless_to_wireless, or attempting a SUP-3 field change. Adjust strategy — don't blindly retry. |
| "one or more numbers are not portable" | 1 | `band portin validate-tf` returned `portable: false` for at least one TN | Inspect the per-number `reason` in the JSON; do not proceed to `create` for those numbers. |

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

**Direct customers** (accounts that register campaigns directly through Bandwidth) are not supported by the commands on this page — those remain import-only. Direct customers register brands and order vettings through the CLI via [`band tendlc brand`](#10dlc-brands) and [`band tendlc vetting`](#10dlc-vettings). Campaign registration for direct customers is not yet in the CLI (planned for a later PR); in the meantime, use the Bandwidth App or the existing Campaign Management API for that step.

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

## Customer Profiles

A customer profile is required to register a 10DLC brand, and **a profile backs
exactly one brand** — reusing a profile ID on a second brand fails with
`cannot be assigned to another brand`. Create a fresh profile per brand. The
prerequisite chain is **customer profile → brand → campaign**: brand
registration now happens in the CLI via [`band tendlc brand create`](#10dlc-brands);
campaign registration still happens in the Bandwidth App (or a later CLI PR).

Requires the **Customer Profiles Access role** — check with `band auth status --plain`.

### Create, list, and get

```bash
band customer-profile create --name "Acme Corp" --plain
# → {"accountId":"9900000","addressId":null,"contact":null,"createdDate":"...","id":"ExampleProfileId000002","modifiedDate":"...","name":"Acme Corp","softDeleted":false,"totalCampaigns":0,"version":0,"website":null}

band customer-profile list --all --plain    # walks every page; cannot combine with --offset
band customer-profile get ExampleProfileId000002 --plain
```

Keys come back alphabetical because the payload is a Go map — don't expect a
"nicer" ordering; the docs match reality, not a prettified version of it.

`list` excludes soft-deleted profiles; `get` still returns them, reporting
`softDeleted: true`. Without `--all`, a truncated page warns on stderr.

**`create` deliberately has no `--if-not-exists`** (see the [Design
Principles](#design-principles) exception). A profile has no safe natural key
to match on, and it's strictly 1:1 with a brand — a retry that silently reused
an existing profile could link an old brand's profile to a new brand's data.
Each brand needs its own freshly created profile; a retry must create, not reuse.

**`create` is also non-idempotent, which cuts the other way after an
ambiguous failure.** If a `create` call fails ambiguously — e.g. the
connection drops after the POST reached the server but before the response
reached you — do not blindly retry. A blind retry can create a *second*,
duplicate profile if the first one actually succeeded. Instead, run `band
customer-profile list --plain` and reconcile the results against what you
just submitted (name, website, contact) to determine whether the first call
already created a profile. If you cannot establish uniqueness that way, stop
and escalate rather than guessing.

### Update is read-modify-write

The API replaces the whole record, so `update` reads the profile first and
re-sends it with your changes applied. Fields you do not pass are preserved.
**Passing a flag with an empty value clears that field** — it sends JSON
`null`, not an empty string, because the API rejects empty strings. A
concurrent edit between the read and the write is caught by the API's version
check and exits **4** — retry the command.

```bash
band customer-profile update ExampleProfileId000002 --name "New Name" --plain
band customer-profile update ExampleProfileId000002 --website "" --plain   # clears the website
```

### Delete is a soft delete

`delete` requires `--confirm`. That's a flag, never a prompt, so agents and
humans share one contract. The record leaves listings but stays retrievable by
ID with `softDeleted: true`, and `restore` brings it back — no confirm needed.

```bash
band customer-profile delete ExampleProfileId000002 --confirm --plain
# → {"deleted":true,"id":"ExampleProfileId000002","restore":"band customer-profile restore ExampleProfileId000002"}
band customer-profile restore ExampleProfileId000002 --plain
```

Note these are two different fields on two different resources, not a typo of
each other: `deleted: true` is the delete command's own receipt, confirming the
204 completed synchronously. `softDeleted: true` is a field on the profile
itself, seen when you `get` it afterward — there is no `deleted` field on the
profile, and no `softDeleted` field on the receipt.

### Version history

`history list` and `history get` return a `{data, metadata}` envelope, newest
first — the profile snapshot lives under `data`, and `version`, `operation`,
`userName`, `createdDate` live under `metadata`. So the version is at
`metadata.version`, NOT top-level the way it is on `customer-profile get`.
Observed `metadata.operation` values: `CREATED`, `UPDATED`, `DELETED`.

```bash
band customer-profile history list ExampleProfileId000002 --plain
band customer-profile history get ExampleProfileId000002 1 --plain
```

## 10DLC Brands

`band tendlc brand` registers and manages 10DLC brands for **direct** customers
(accounts that register with TCR through Bandwidth directly, not through
import). A brand needs a customer profile first — see
[Customer Profiles](#customer-profiles) — and a profile backs exactly one
brand. Requires the **Registration Center** feature and the **Campaign
Management** role.

### Eligibility

Check before doing anything else:

```bash
band tendlc status --plain
```

This is a probe, not a live brand/vetting command, and it is deliberately
gentle: every 403 it can classify — role missing, Registration Center not
enabled, campaign management not enabled, or an unrecognized 403 — is a
**successful answer to the question "can I do this?"**, so it exits **0** with
`{"access":"unavailable", ...}`. See [10DLC capability](#10dlc-capability-tri-state-not-boolean)
for the full `reason` table. Only `probe_failed` (rate limit, 5xx, transport
error) is worth retrying.

That gentle handling is specific to `status`. Once you move on to an actual
`brand`/`vetting` command, a 403 there is **not** a probe answer — it is the
command failing — and it maps to exit **4** via the same
`FeatureLimitError`/`roleGateError` path used everywhere else in this CLI.
Escalate to the account manager; do not retry.

### The two IDs

Every brand has two identifiers, and they are not interchangeable in what
they guarantee:

- **`bandwidthId`** exists from the moment of the 202 that accepts the create.
- **`brandId`** is assigned by TCR once registration completes, and is `null`
  until then. A brand that never registers never gets one.

Every `brand`/`vetting` command that takes an ID accepts either. This matters
for anything that keys off `brandId`: a script or agent that reads `brandId`
right after `create` and treats a missing/`null` value as an error will
misfire on every brand still mid-registration — `bandwidthId` is the only ID
guaranteed to exist immediately.

```bash
band tendlc brand get BGJR2BA --plain       # TCR brandId
band tendlc brand get WET8JUY8H0 --plain    # bandwidthId — same brand
```

Both return the same 46-key object, including `bandwidthId`, `brandId`,
`brandIdentityStatus`, `brandRelationship`, `universalEin`, `imported`, and
`vertical` (a live value observed was `PROFESSIONAL`, which is absent from the
published enum).

### `create`

```bash
band tendlc brand create --customer-profile-id CP123 --brand-type PRIVATE_PROFIT \
  --display-name "Acme Corp" --company-name "Acme Corporation" \
  --street "123 Main St" --city Raleigh --state NC --postal-code 27601 \
  --country-code-a3 USA --phone +18885551234 --email ops@acme.com \
  --vertical RETAIL --ein 123456789 --ein-issuing-country-code-a3 USA --wait
```

There is **no `--country` flag** — the API derives `country` server-side from
`--country-code-a3`, so passing one would just be dead input. Likewise there
is no separate country flag for the EIN; `--ein-issuing-country-code-a3` is
the only knob.

**Every brand type** requires these 9 fields plus `--brand-type` itself:
`--customer-profile-id`, `--display-name`, `--street`, `--city`, `--state`,
`--postal-code`, `--country-code-a3`, `--phone`, `--email`. Beyond that, the
required set is per-`brandType`. This matrix is sourced from
`internal/tendlc/brandoptions.go`, which is the measured truth — the API's
own schema gets required-ness wrong in both directions, so this is derived
from the API's actual 400 responses, not the published spec:

| `--brand-type` | Additional required flags |
|---|---|
| `PRIVATE_PROFIT` | `--company-name`, `--vertical`, `--ein`, `--ein-issuing-country-code-a3` |
| `NON_PROFIT` | same four as `PRIVATE_PROFIT` |
| `GOVERNMENT` | same four as `PRIVATE_PROFIT` |
| `PUBLIC_PROFIT` | the same four, **plus** `--stock-symbol`, `--stock-exchange`, `--website`, `--business-contact-email` |
| `SOLE_PROPRIETOR` | none enforced by the CLI beyond the 9 common fields (see note) |

**`SOLE_PROPRIETOR` is deliberately under-validated.** Its field rules sit
behind an account-level gate no available test account could get past, so
they were never observable. The CLI does not invent rules for it — doing so
risks rejecting a request the API would actually accept. Expect the API's own
400 to enforce anything beyond the 9 common fields for this type. The
`--first-name`, `--last-name`, `--mobile-phone`, and `--ip-address` flags
exist and are almost certainly what a sole-proprietor registration needs, but
they are optional at the CLI layer.

Validation aggregates every violation into one error, the way the API
reports every violation in one 400 — no request is made:

```
$ band tendlc brand create --plain
Error: missing required flags: --brand-type, --city, --country-code-a3, --customer-profile-id, --display-name, --email, --phone, --postal-code, --state, --street

$ band tendlc brand create --brand-type PUBLIC_PROFIT <all common fields + company/vertical/ein> --plain
Error: missing required flags: --business-contact-email, --stock-exchange, --stock-symbol, --website
```

**The `customerProfileId` trap.** Measured against production: `POST /brands`
does **not** reject an invalid or typo'd `customerProfileId`. It silently
discards it and returns **202**, creating an orphan brand with no profile
association — a brand that can never verify. To prevent this, `brand create`
reads the customer profile named by `--customer-profile-id` before it submits
anything. Only a definitive 404 stops the create:

```
$ band tendlc brand create --customer-profile-id NOT_A_REAL_PROFILE_ID --brand-type PRIVATE_PROFIT ... --plain
Error: customer profile "NOT_A_REAL_PROFILE_ID" not found — run 'band customer-profile list' to see valid profile IDs (API error 404: {"errors":[{"type":"not found","description":"Customer profile not found, id: NOT_A_REAL_PROFILE_ID","source":{"POINTER":"/id"}}],"links":[]})
$ echo $?
3
```

Brand count was measured 10 before and 10 after this refusal — nothing was
created. If the pre-flight itself **cannot run** — most commonly because the
caller has Campaign Management but not the separate **Customer Profiles
Access** role — the CLI does not block the create on a check it has no
permission to perform. It warns on stderr and proceeds:

```
warning: could not verify customer profile "CP123" before creating the brand (...); proceeding anyway
```

An agent that sees this warning should not assume the profile association
took — verify it afterward by checking `accounts[0].customerProfileId` on
the resulting brand (`band tendlc brand get <id> --plain`).

**`--wait` semantics.** `brandIdentityStatus` reads `UNVERIFIED` for the
entire registration window; `REGISTERING`, the enum's documented in-progress
value, was never observed on the read path in live testing. That means
`UNVERIFIED` cannot distinguish "TCR hasn't answered yet" from "TCR rejected
it" — so the CLI keeps polling on `UNVERIFIED` rather than treating it as a
failure, and only surfaces it at timeout, with the last-seen status attached.
Two brands submitted with byte-identical payloads measured very differently:

- `WOVNQBAVI2`: read `UNVERIFIED` at t=3s, flipped to `VERIFIED` at t=46s
- `WAR2FRJPVQ`: read `UNVERIFIED` at t=3s, still `UNVERIFIED` at t=275s, no TCR response in its history at all

**A timeout (exit 5) is not a failure.** Tell the caller plainly: re-check
with `band tendlc brand get <id>` (or `brand history <id>` for the raw TCR
timeline) rather than treating the timeout as a rejected brand. The only
state that *is* a business failure is `ERROR`, which exits **4** — everything
else not in `{VERIFIED, VETTED_VERIFIED, SELF_DECLARED, ERROR}` is treated as
still-pending.

| Outcome | Exit | What's on stdout |
|---|---|---|
| Reached `VERIFIED` / `VETTED_VERIFIED` / `SELF_DECLARED` | 0 | The full brand object (46 keys), including `bandwidthId` |
| Reached `ERROR` | 4 | The full brand object, plus a remediation message on stderr pointing at `brand refresh` |
| `--wait` exceeded `--timeout`, still pending | 5 | The synthetic receipt (below), with `lastSeenStatus` |
| Transport/decode failure mid-poll | Whatever `ExitCodeForError` maps the underlying error to (not necessarily 5 — only an actual deadline-exceeded times out as 5) | The synthetic receipt (below) |
| No `--wait` | 0 | The synthetic receipt, immediately after the 202 |

**The receipt guarantee.** On every path that reaches a 202 accept, stdout
carries `bandwidthId` somewhere in valid JSON — that ID is the one thing that
cannot be recovered any other way if the command exits without printing it.
Without `--wait`, or on a timeout/transport failure with `--wait`, that's the
literal synthetic receipt: `{"bandwidthId": "...", "status": "accepted",
"resume": "band tendlc brand get <id>", "brandId": "..." (if already known)}`
— plus, on a `--wait` timeout/error, `"note"` (pointing at `get`/`history`
for a brand parked at `UNVERIFIED`) and `"lastSeenStatus"` (if a status was
ever successfully read). On success or business failure (`ERROR`), stdout
instead carries the **real, full brand object** returned by the API — not
the synthetic receipt — but `bandwidthId` is still one of its keys either
way. An example timeout receipt:

```json
{
  "bandwidthId": "WAR2FRJPVQ",
  "status": "accepted",
  "resume": "band tendlc brand get WAR2FRJPVQ",
  "note": "if this timed out at UNVERIFIED, the brand may still be registering with TCR rather than having failed. Check 'band tendlc brand get WAR2FRJPVQ' for its current status, or 'band tendlc brand history WAR2FRJPVQ' for the full history.",
  "lastSeenStatus": "UNVERIFIED"
}
```

### `--confirm`-gated writes, and what each costs

`--confirm` is a flag, never a prompt — agents and humans share one
contract. Missing it is exit **6** (`FlagError`) with **zero** HTTP requests.

| Command | Cost / consequence |
|---|---|
| `brand delete <id> --confirm` | Permanent. Requires every campaign on the brand to be deactivated first. See the cascade correction below. |
| `brand reverify <id> --confirm` | $4 fee. Resets `brandIdentityStatus` toward re-registration (documented as `REGISTERING`; in practice this reads back as `UNVERIFIED`, per the `--wait` note above). |
| `brand update <id> --confirm ...` | Only required when an identity-affecting field changes: `--company-name`, `--brand-type`, `--ein`, or `--ein-issuing-country-code-a3` (possible $4 fee + reset to re-registration, and rejected outright if the brand has an active campaign or an active Standard/Enhanced/Political vetting), `--mobile-phone` (sets identity status to `UNVERIFIED`), or `--business-contact-email` on a `PUBLIC_PROFIT` brand (revokes Auth+ compliance — regaining it needs a new `AUTHPLUS` vetting and another 2FA email round-trip). |
| `vetting request <brand-id> --confirm` | Billable order placed with an external vetting provider (see [10DLC Vettings](#10dlc-vettings)). |

Example refusals, all exit 6, all zero requests:

```
$ band tendlc brand delete BGJR2BA --plain
Error: this permanently deletes brand BGJR2BA. It cannot be undone, it deletes the brand in TCR for direct accounts, and it requires every campaign on the brand to be deactivated first. It does NOT delete the associated customer profile (measured against production — the documented cascade does not happen); remove that separately with 'band customer-profile delete <id>' if you no longer need it. Pass --confirm to proceed.

$ band tendlc brand reverify BGJR2BA --plain
Error: reverifying brand BGJR2BA incurs a $4 fee and resets brandIdentityStatus to REGISTERING. Pass --confirm to proceed.

$ band tendlc brand update BGJR2BA --company-name "New Name" --plain
Error: changing company-name on brand BGJR2BA resubmits it for identity verification: this may incur a $4 fee and resets brandIdentityStatus to REGISTERING. If the brand has an active campaign or an active Standard/Enhanced/Political vetting, the API will reject the change outright. Pass --confirm to proceed.
```

`brand resend-2fa` and `brand refresh` take no `--confirm` — neither is
destructive nor billable.

**The delete cascade correction.** The endpoint's own docs say deleting a
brand cascades to delete its backing customer profile. Measured against
production: **it does not.** Two test brands were deleted and both backing
profiles remained retrievable afterward with `softDeleted: false`. Delete the
profile separately with `band customer-profile delete <id>` if you don't need
it — otherwise it's an orphan that can never back another brand (a profile
is 1:1 with a brand for life). Also note: deletion takes roughly **40
seconds** to be reflected in `brand list`; `delete --wait` polls until the
brand is actually gone (the one poll in this whole command set where a 404
on the follow-up read means success, not "not ready yet").

```
$ band tendlc brand delete WOVNQBAVI2 --confirm --plain
{
  "deleted": true,
  "id": "WOVNQBAVI2",
  "status": "accepted"
}
$ echo $?
0
```

### `update` is read-modify-write, with no version field

The API replaces the whole record on update, so the CLI reads the brand
first and re-sends it with your changes applied — fields you don't pass are
preserved. Unlike customer profiles, **brands have no optimistic-locking
token at all**: there is no version check on the PUT. A concurrent edit that
lands between the CLI's read and its write is silently lost — whichever
write reaches the API last wins, with no conflict error to catch it.

```bash
band tendlc brand update BGJR2BA --website "https://acme.example" --plain
```

### `list` vs `get`: projection, not nullability

`brand list` returns a **13-key summary projection**; `brand get` returns
**46 keys**. A field missing from `list` output is not null on the brand —
it's simply outside the listing projection. Use `get` for the full resource.

```
$ band tendlc brand list --plain --limit 2
showing 2 of 10 brands; pass --all to fetch every page      # <- stderr
[
  {
    "accounts": [{"accountId": "9901287", "customerProfileId": "59eZSI61xzcW5j1LSAxfPM"}],
    "authenticationStatus": "ACTIVE",
    "bandwidthId": "WGVL458T5W",
    "brandId": "BLLIGLJ",
    "brandIdentityStatus": "VERIFIED",
    "brandType": "PUBLIC_PROFIT",
    "businessContactEmail": "kshah@bandwidth.com",
    "companyName": "Bandwidth",
    "createdDate": "2026-05-28T21:00:16.048Z",
    "displayName": "Another Auth+ Test, Bandwidth",
    "modifiedDate": "2026-05-28T21:00:16.048Z",
    "website": "bandwidth.com"
  }
]
```

There is deliberately **no `--bandwidth-id` filter**. Measured against
production: the API accepts a `bandwidthId[eq]` filter and silently ignores
it, returning every brand rather than filtering — so the CLI doesn't expose
a flag that would lie about filtering. Use `brand get <bandwidth-id>` to
fetch one directly instead.

`brand history <id>` is a free-text activity log, newest first, with no
version-per-entry the way customer profiles have:

```
$ band tendlc brand history BGJR2BA --plain --limit 2
showing 2 of 7 history entries; pass --all to fetch every page      # <- stderr
[
  {"createdDate": "2026-06-17T19:37:16.927Z", "message": "Successfully updated brand BGJR2BA for account 9901287"},
  {"createdDate": "2026-06-17T18:10:48.526Z", "message": "BRAND_IDENTITY_STATUS_UPDATE received from TCR with new status UNVERIFIED"}
]
```

A real registration timeline, from live testing, newest first:

```
{"createdDate": "2026-08-19T13:39:04.542Z", "message": "BRAND_IDENTITY_STATUS_UPDATE received from TCR with new status VERIFIED"}
{"createdDate": "2026-08-19T13:38:24.988Z", "message": "Brand billed for account 9901287 brandId B5XBU3K sku A2PLC-NRC-BRANDFEE"}
{"createdDate": "2026-08-19T13:38:18.725Z", "message": "Successfully created brand B5XBU3K for account 9901287 and bandwidthId WOVNQBAVI2"}
{"createdDate": "2026-08-19T13:38:17.589Z", "message": "Registering brand for account 9901287 bandwidthId WOVNQBAVI2"}
```

Note the fee lands ~6 seconds after creation — long before verification
resolves either way. **The brand fee is billed at creation, not at
verification**, and is charged even for a brand that never ends up
verifying. Factor that into any retry logic: a create you retry blind after
an ambiguous failure risks a second bill, not just a second brand.

## 10DLC Vettings

`band tendlc vetting` orders and records third-party vettings against a
brand. Requires the same Registration Center feature and Campaign Management
role as `brand`.

**Vettings are brand-scoped, not campaign-scoped.** Every command here takes
a **brand ID** as its first positional, not a vetting ID — there is no
campaign vetting endpoint in either spec. A campaign only exposes a
read-only, derived `vettingStatus`; re-evaluating a campaign directly is a
different, separate future command (`nudge`), not covered here.

### `list`

```
$ band tendlc vetting list B53K4I0 --plain
[
  {
    "bandwidthId": "WE4DHNXJZ9",
    "createdDate": "2026-06-17T17:55:13Z",
    "evpId": "AEGIS",
    "reasons": [
      "Submitted Address Line 1 cannot be verified against government or business sources.",
      "Submitted Postal Code cannot be verified against government or business sources.",
      "Relevant liens against the submitted company were reported."
    ],
    "vettedDate": "2026-06-17T17:55:13Z",
    "vettingClass": "STANDARD",
    "vettingDetails": {},
    "vettingId": "978de74a-7191-4656-e1a7-08dec82125d2",
    "vettingScore": 88,
    "vettingStatus": "ACTIVE",
    "vettingToken": "eyJhbGciOiJSUzI1NiIsImtpZCI6..."
  }
]
```

Note `vettingStatus: "ACTIVE"` — the live success value, and it appears in
**neither** the brand-vetting enum nor the campaign `vettingStatus` enum.
Unknown/undocumented statuses are treated as still-pending everywhere in
this command set: only `ACTIVE` classifies as success and only `FAILED` /
`EXPIRED` classify as business failure, so a status this CLI has never seen
keeps polling under `--wait` rather than being reported as either outcome.

**A naming quirk to handle, not normalize.** `list` returns the ID under
`bandwidthId`; the `POST .../vettings` 202 accept returns it as
`vettingBandwidthId` instead. The CLI preserves whichever key name the API
actually used in its own receipts rather than renaming it to one canonical
field — so when you read a vetting receipt, check for both key names.

### `request` — billable, `--confirm`-gated

```bash
band tendlc vetting request BGJR2BA --evp AEGIS --class STANDARD --confirm --plain
band tendlc vetting request BGJR2BA --evp AEGIS --class STANDARD --confirm --wait --plain
```

`--evp` accepts `AEGIS`, `CV`, `WMC` — a small, stable, fully documented enum.

`--class` accepts `STANDARD`, `ENHANCED`, `POLITICAL`, `AUTHPLUS`, and
**`RCS`**. `RCS` is absent from the published `enumVettingClass`, but
production accepts it — confirmed by pairing each class with an invalid
`evpId` and observing which produced a class-level error rather than an
evp-level one. Do not drop it to match the spec; production honors it.

This places a real, billable order with an external vetting provider, so
`--confirm` is required — missing it is exit 6, zero requests:

```
$ band tendlc vetting request BGJR2BA --evp AEGIS --class STANDARD --plain
Error: requesting a STANDARD vetting from AEGIS for brand BGJR2BA is a billable order placed with an external vetting provider. Pass --confirm to proceed.
```

With `--wait`, the receipt is `{<idField>: id, "brandId": ..., "status":
"accepted", "check": "band tendlc vetting list <brand-id>"}`, printed
immediately without `--wait` or on a timeout/transport error with it. Unlike
`brand create --wait`, there is no `lastSeenStatus` carried on a vetting
timeout — the vetting poll target doesn't track a last-observed status, so a
timed-out `vetting request --wait` tells you to re-check with `vetting list`
rather than showing you the last status inline.

### `import` — not billable, no `--confirm`

```bash
band tendlc vetting import BGJR2BA V123 --evp AEGIS --plain
band tendlc vetting import BGJR2BA V123 --evp AEGIS --vetting-token TOK123 --plain
```

Recording a vetting that was already performed outside Bandwidth costs
nothing and places no new order, so — unlike `request` — this takes no
`--confirm`.

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

### Test SIP Trunking end-to-end

Use this workflow to verify that a SIP realm and credential authenticate correctly by placing a real call through Bandwidth. The pattern creates an ephemeral realm and credential so nothing existing is disrupted, dials a number via a local SIP UA, then tears everything down.

**Prerequisites:** a SIP UA installed locally (baresip works; any UA that supports digest auth and reads an accounts file will do), and a Bandwidth phone number to call from.

```bash
# 1. Pick a from number — must be on your account and voice-capable
FROM=$(band number list --plain | jq -r '.[0]')
# On Bandwidth Build accounts, band number list is not available.
# Pass the pre-provisioned number manually: FROM=+19195551234

# 2. Create an ephemeral realm (never the default — it cannot be deleted if it is)
REALM=$(band sip realm create --name sip-test --default=false --wait --plain)
REALM_ID=$(echo "$REALM" | jq -r '.id')
REALM_FQDN=$(echo "$REALM" | jq -r '.hostname')

# 3. Create a credential and capture the password — printed exactly once
CRED=$(band sip credential create \
  --realm "$REALM_ID" \
  --username sip-test-agent \
  --generate-password \
  --plain)
CRED_ID=$(echo "$CRED" | jq -r '.id')
SIP_PASS=$(echo "$CRED" | jq -r '.password')

# 4. Write a temp accounts file — 600 permissions, never passed on the command line
TMPDIR=$(mktemp -d)
chmod 700 "$TMPDIR"
printf '<sip:%s@%s;transport=udp>;regint=0;audio_codecs=pcmu/8000,pcma/8000;auth_user=sip-test-agent;auth_pass=%s\n' \
  "$FROM" "$REALM_FQDN" "$SIP_PASS" > "$TMPDIR/accounts"
chmod 600 "$TMPDIR/accounts"
unset SIP_PASS   # clear from environment immediately after writing

# Copy any existing baresip config (codec/module paths) into the temp dir.
# This ensures the SIP UA can find its audio modules.
# Skip this line if you prefer baresip to generate a fresh default config.
cp ~/.baresip/config "$TMPDIR/config" 2>/dev/null || true

# 5. Dial — replace <TO> with the destination E.164 number
TO=+15559876543
baresip -f "$TMPDIR" -e "d sip:${TO}@${REALM_FQDN}" -t 30

# 6. Clean up — always run, even if the call fails
rm -rf "$TMPDIR"
band sip credential delete "$CRED_ID" --realm "$REALM_ID"
band sip realm delete "$REALM_ID" --wait
```

**Interpreting baresip output:**

| Signal | Meaning |
|--------|---------|
| `407 Proxy Authentication Required` → re-INVITE | Auth challenge is working. Wait for the response to the re-INVITE. |
| `180 Ringing` or `183 Session Progress` | Call reached the PSTN. |
| `200 OK` + "Call established" | Call connected. SIP auth is fully functional. |
| `403 Forbidden` after the re-INVITE | Credential mismatch — the password written to the accounts file does not match the stored hashes. Rotate the credential and retry from step 3. |
| `503 Service Unavailable` or `480` | Routing issue. Check that the realm is `ACTIVE` (`band sip realm get sip-test --plain`) and that the FQDN has propagated in DNS — new realms may take a moment. |

**Always clean up in step 6**, even on failure. A leftover credential is not a security risk (Bandwidth never stores or returns the plaintext password), but unused credentials and realms should not accumulate.

**Why a new realm instead of an existing one.** Rotating a credential on an existing realm invalidates the password for any other system using that credential. A fresh realm + fresh credential is purely additive — nothing else depends on it, and teardown leaves no side effects.

---

## SIP Trunk Authentication

SIP realms provide the FQDN a SIP peer uses as its outbound address for SIP trunk digest authentication. This is separate from inbound call routing, which is configured with `band vcp`.

### Create a realm

```bash
band sip realm create --name vapi --default=false --wait --plain
# → { "id": "...", "name": "vapi", "hostname": "vapi-3efeaa.auth.bandwidth.com", "status": "ACTIVE", ... }
```

`--default` is **required** — the API rejects creates without it (error 1003). Creation is asynchronous (`CREATE_PENDING` → `ACTIVE`); use `--wait` to block until the realm is `ACTIVE`, or `--if-not-exists` for a safe retry.

### List and inspect realms

```bash
band sip realm list --plain
band sip realm get vapi --plain
```

`get` accepts a realm ID, name, or FQDN.

### Update a realm

Two fields are updatable: `--default=true` and `--description`. Pass either or both; an omitted field is preserved (the update reads the realm first, because the API's `PUT` is a full replace).

The API refuses to delete the default realm, and a realm's `default` flag can only be set to `true` (never back to `false`). To retire a default realm, promote another one first:

```bash
band sip realm update backup-realm --default=true
band sip realm delete old-default-realm --wait
```

`--description` is the remediation for a `--if-not-exists` description mismatch:

```bash
band sip realm update vapi --description "Vapi production trunk"
```

### Delete a realm

```bash
band sip realm delete vapi --wait
```

Deletion is asynchronous (the API returns 202). A realm cannot be deleted while it still has SIP credentials (error 12666) or while it is the account default (error 33006).

### Common errors

| Error code | Message | Cause | Fix |
|---|---|---|---|
| **33006** | Cannot delete the default realm | Realm is the account default | Promote another realm first: `band sip realm update <other-realm> --default=true` |
| **12666** | Realm still has SIP credentials | Realm has credentials attached | Delete the credentials first |
| **33002** | Realm already exists | Name collision | Use `--if-not-exists`, or pick a different `--name` |
| **23022** | Realm is not active yet | Realm hasn't finished provisioning | Retry with `--wait` |
| **23026** | Credential already exists | A credential with that username is already on the realm | Use `--if-not-exists` to reuse it, or change its password with `band sip credential rotate <credential-id> --realm <realm>` |
| **33004** | Account isn't set up for SIP credentials | Account lacks `SipCredentialSettings`. Not a role problem — the credential can hold the SIP Credentials role and still get this | Contact Bandwidth support to enable it. Check up front with `band sip status --plain` |

Every error in this table exits **4**, regardless of the HTTP status the API used to report it (these arrive variously as 400, 409, and even 201 with an error envelope). Branch on the exit code, then read the message for the remediation.

## Limitations

- **Bandwidth Build accounts are voice-only.** Detect via `band auth status --plain` (`build: true`). On a Build account, only voice and app-management commands work — `message send`, `number search`/`order`, `vcp *`, `subaccount *`, `tendlc *`, `tfv *` all exit 4 with a Build-aware message and an upgrade link. Pre-provisioned voice app and number ship with the account; `band number list` doesn't work yet (the number is reachable via the account portal). Build also has runtime limits not surfaced in `auth status` — verified-number-only outbound on Free Trial, a 30-min cap per call, a 5-concurrent-call ceiling. See [dev.bandwidth.com](https://dev.bandwidth.com/docs/voice/programmable-voice/build-free-trial) for current pricing and limits; treat any 402 (exit 4) as "out of credits, escalate" and any 429 (exit 7) as "back off and retry."
- **No real-time call control.** The CLI can initiate calls and query state, but cannot receive or respond to mid-call callbacks. Dynamic call control requires a separate callback-handling server.
- **No message delivery confirmation.** The CLI verifies your setup is correct before sending (app-location link, callback URL, campaign), but it cannot confirm whether a message was actually delivered. Delivery status (`message-delivered`, `message-failed`) arrives via webhooks on your callback server. The CLI's `message get` and `message list` return metadata only — not delivery status.
- **No message content retrieval.** Bandwidth does not store message bodies. After sending, the message text is gone forever. `message get` and `message list` return timestamps, direction, and segment counts only.
- **10DLC: read + assign only.** The CLI can list campaigns, check number registration status, diagnose failures (`band tendlc`), and assign numbers to campaigns (`band tnoption assign`). It cannot create campaigns or register brands — those require the Bandwidth App. The CLI checks that a number is on a campaign and blocks sends if it's not.
- **TFV is check-and-submit.** The CLI can check toll-free verification status and submit new requests (`band tfv`), but cannot approve or expedite reviews — those happen on the carrier side.
- **Porting is port-IN only.** `band portin` covers the six end-to-end flows that complete via the public API: TF validation, on-net domestic, automated off-net (Level 3), TF Phase 1 (gated), bulk, and lifecycle ops (notes, supp, cancel, history, doc upload). Out of scope: port-out (no public API), manual TF, internal TF, NASC manual override, and international ports — these need ops or the Dashboard. `band portin create` exits 4 if the account doesn't have `TOLL_FREE_AUTOMATION_PHASE_1` for a TF order. `band portin supp` defends against the documented Bandwidth API behavior where a supp returns 200 on PUT but error code 7300 on the next GET (Neustar never received it) — exits 1 with a clear message rather than silently succeeding.
- **10DLC, TFV, and short code commands are role-gated.** A 403 can mean the credential lacks the required role (Campaign Management, TFV), the account doesn't have the Registration Center feature, or messaging isn't enabled. The CLI provides a diagnostic message — if it says "access denied," escalate to the Bandwidth account manager rather than retrying.
- **No batch operations.** Each command operates on one resource (except `vcp assign` which handles multiple numbers and `message send` which supports multiple recipients).
- **Dashboard API uses XML internally.** The CLI handles XML serialization transparently — you always send and receive JSON. Use `--plain` for predictable, flat output.
