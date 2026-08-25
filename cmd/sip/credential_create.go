package sip

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	sipsvc "github.com/Bandwidth/cli/internal/sip"
)

var (
	credCreate            credentialFlags
	credCreateUsername    string
	credCreateAppID       string
	credCreateIfNotExists bool
)

func init() {
	credentialCmd.AddCommand(credentialCreateCmd)
	f := credentialCreateCmd.Flags()
	f.StringVar(&credCreate.realm, "realm", "", "Realm ID, name, or FQDN (required)")
	f.StringVar(&credCreateUsername, "username", "", "SIP username (required)")
	f.StringVar(&credCreateAppID, "app-id", "", "Bind the credential to a voice application ID")
	f.BoolVar(&credCreateIfNotExists, "if-not-exists", false, "Return the existing credential if one with the same username and settings exists")
	addPasswordFlags(credentialCreateCmd, &credCreate)
	credentialCreateCmd.MarkFlagRequired("realm")
	credentialCreateCmd.MarkFlagRequired("username")
}

var credentialCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a SIP digest credential",
	Long: "Creates a SIP digest credential on a realm. Bandwidth never accepts or returns a password — " +
		"the CLI computes the MD5 digest hashes from the realm's FQDN. With --generate-password the password " +
		"is printed exactly once and cannot be recovered later; use 'band sip credential rotate' if it is lost.",
	Example: `  # Caller-owned secret (recommended for agents and retries)
  printf '%s' "$SIP_PASSWORD" | band sip credential create --realm vapi --username vapi-agent --password-stdin

  # Let the CLI generate one (shown once)
  band sip credential create --realm vapi --username vapi-agent --generate-password`,
	RunE: runCredentialCreate,
}

func runCredentialCreate(cmd *cobra.Command, args []string) error {
	// Both input validations run BEFORE the service is built, before the realm
	// lookup, and before any password is read or generated: an invalid --app-id
	// must cost zero HTTP requests and, more importantly, must never generate a
	// write-once secret that the live API is going to reject anyway.
	if err := sipsvc.ValidateUsername(credCreateUsername); err != nil {
		return err
	}
	if err := sipsvc.ValidateAppID(credCreateAppID); err != nil {
		return err
	}
	svc, err := service(cmd)
	if err != nil {
		return err
	}
	realm, err := svc.GetRealm(cmd.Context(), credCreate.realm)
	if err != nil {
		return faultExit(err)
	}
	if realm.Status != "ACTIVE" {
		return conflict(nil, "realm %s is %s — credentials can only be created on ACTIVE realms; retry after 'band sip realm get %s' reports ACTIVE", realm.Name, realm.Status, realm.Name)
	}

	password, generated, err := readPassword(cmd, credCreate.stdin, credCreate.file, credCreate.generate)
	if err != nil {
		return err
	}
	hash1, hash1b := sipsvc.ComputeHashes(credCreateUsername, realm.Hostname, password)

	cred, err := svc.CreateCredential(cmd.Context(), realm.ID, credCreateUsername, hash1, hash1b, credCreateAppID)
	if err != nil {
		var fault *sipsvc.APIFault
		if credCreateIfNotExists && errors.As(err, &fault) && fault.Code == "23026" {
			return reuseCredential(cmd, svc, realm, hash1, hash1b, password, generated)
		}
		// Exit 8 is reserved for GENUINE ambiguity: a decode failure or a
		// mid-flight transport error, where the POST may have committed and the
		// generated password is unrecoverable. An *APIFault means the server
		// parsed the request and rejected it, so nothing was written — reporting
		// 8 there would tell an agent "a credential you can't use may exist"
		// when the correct signal is the fault's own code (e.g. 7, back off and
		// retry, for a 429). That is what --password-stdin already reports for
		// the identical response.
		if generated && !errors.As(err, &fault) {
			return &cmdutil.SecretUnavailableError{Message: fmt.Sprintf(
				"the write may have been applied but the generated password was not printed and cannot be recovered — check 'band sip credential list --realm %s --plain' and rotate the credential if it exists: %v",
				realm.Name, err)}
		}
		return faultExit(err)
	}
	// The POST has committed. If stdout is full, closed, or short-writes now, the
	// credential exists and nobody will ever know its password — the same
	// unrecoverable state a lost response leaves behind, so it must report the
	// same exit code (8) rather than the generic 1 a write error maps to. With a
	// caller-supplied password there is nothing to lose: return the write error
	// as-is.
	if err := emitCredential(cmd, cred, password, generated); err != nil {
		if generated {
			return &cmdutil.SecretUnavailableError{Message: fmt.Sprintf(
				"the credential was created but the generated password could not be written to stdout and cannot be recovered — find it with 'band sip credential list --realm %s --plain', then rotate it: band sip credential rotate <credential-id> --realm %s --generate-password: %v",
				realm.Name, realm.Name, err)}
		}
		return err
	}
	return nil
}

// reuseCredential implements --if-not-exists after a 23026 duplicate. Identity
// is realm + username; desired state is both hashes plus the app binding.
func reuseCredential(cmd *cobra.Command, svc *sipsvc.Service, realm *sipsvc.Realm, hash1, hash1b, password string, generated bool) error {
	found, err := svc.FindCredentialByUsername(cmd.Context(), realm.ID, credCreateUsername)
	if err != nil {
		return faultExit(err)
	}
	// Re-read the single credential rather than trusting FindCredentialByUsername's
	// list-derived AppID: the collection response's app-binding field has not
	// been confirmed to round-trip the same shape as the single-item GET.
	existing, err := svc.GetCredential(cmd.Context(), realm.ID, found.ID)
	if err != nil {
		return faultExit(err)
	}
	if existing.AppID != credCreateAppID {
		if existing.AppID == "" {
			return conflict(nil, "credential %q exists but is not bound to an application (wanted %q) — delete and recreate it", credCreateUsername, credCreateAppID)
		}
		return conflict(nil, "credential %q exists but is bound to a different application (%q) — delete and recreate it", credCreateUsername, existing.AppID)
	}
	if generated {
		return &cmdutil.SecretUnavailableError{
			Message: fmt.Sprintf("credential %q already exists and its password cannot be recovered — rotate it: band sip credential rotate %s --realm %s --generate-password", credCreateUsername, existing.ID, realm.Name),
		}
	}
	match, err := svc.CredentialHashesMatch(cmd.Context(), realm.ID, existing.ID, hash1, hash1b)
	if err != nil {
		return faultExit(err)
	}
	if !match {
		return conflict(nil, "credential %q exists with a different password — rotate it: band sip credential rotate %s --realm %s --password-stdin", credCreateUsername, existing.ID, realm.Name)
	}
	return emitCredential(cmd, existing, password, false)
}

// emitCredential prints the credential. A generated password is included exactly
// once; a caller-supplied one is omitted because the caller already has it.
//
// The JSON paths do NOT go through emit, for one reason: the spec requires the
// generated password to be the FIRST thing written on success, so a stdout write
// that is truncated part-way still delivers the only copy of the secret. emit
// normalizes payloads to a map and Go's encoder writes map keys alphabetically,
// which puts appId, hostname, and id ahead of password. This function performs
// emit's two meaningful steps itself — normalize, then redact — and only then
// wraps the result in passwordFirstPayload, whose MarshalJSON fixes the order.
// Wrapping has to come last: output.RedactSecrets only walks
// map[string]interface{} / []interface{}, so a wrapped payload would slip past
// the redaction net entirely.
//
// The table path keeps using emit: output.printTable has no case for a
// json.Marshaler and would print a Go struct dump, and a human reading a table
// has no ordering guarantee to lose.
func emitCredential(cmd *cobra.Command, cred *sipsvc.Credential, password string, generated bool) error {
	format, plain := cmdutil.OutputFlags(cmd)
	out := map[string]interface{}{
		"id":                cred.ID,
		"realmId":           cred.RealmID,
		"username":          cred.Username,
		"hostname":          cred.Hostname,
		"appId":             cred.AppID,
		"passwordShownOnce": generated,
	}
	if generated {
		out["password"] = password
	}
	if format == "table" && !plain {
		return emit(format, plain, out)
	}
	normalized, err := normalizeForOutput(out)
	if err != nil {
		return err
	}
	redacted, ok := output.RedactSecrets(normalized).(map[string]interface{})
	if !ok {
		// Unreachable: out is a map, and RedactSecrets maps a map to a map.
		return emit(format, plain, out)
	}
	return output.StdoutAuto(format, plain, passwordFirstPayload(redacted))
}

// passwordFirstPayload is a JSON object that marshals "password" first and every
// other key in sorted order. It exists so the write-once generated password
// leads the output (see emitCredential), and it is a named map type rather than
// a struct so it carries whatever field set the payload has without a second
// place to keep in sync.
//
// It is deliberately opaque to output.FlattenResponse and output.RedactSecrets,
// which both type-assert on the unnamed map[string]interface{}: redaction has
// already run by the time a payload is wrapped, and this payload is flat, so
// there is nothing for flattening to unwrap.
type passwordFirstPayload map[string]interface{}

func (p passwordFirstPayload) MarshalJSON() ([]byte, error) {
	keys := make([]string, 0, len(p))
	for k := range p {
		if k != "password" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if _, ok := p["password"]; ok {
		keys = append([]string{"password"}, keys...)
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(p[k])
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
