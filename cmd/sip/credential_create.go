package sip

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
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
	if err := sipsvc.ValidateUsername(credCreateUsername); err != nil {
		return err
	}
	svc, err := service(cmd)
	if err != nil {
		return err
	}
	realm, err := svc.GetRealm(credCreate.realm)
	if err != nil {
		return faultExit(err)
	}
	if realm.Status != "ACTIVE" {
		return fmt.Errorf("realm %s is %s — credentials can only be created on ACTIVE realms; retry after 'band sip realm get %s' reports ACTIVE", realm.Name, realm.Status, realm.Name)
	}

	password, generated, err := readPassword(cmd, credCreate.stdin, credCreate.file, credCreate.generate)
	if err != nil {
		return err
	}
	hash1, hash1b := sipsvc.ComputeHashes(credCreateUsername, realm.Hostname, password)

	cred, err := svc.CreateCredential(realm.ID, credCreateUsername, hash1, hash1b, credCreateAppID)
	if err != nil {
		var fault *sipsvc.APIFault
		if credCreateIfNotExists && errors.As(err, &fault) && fault.Code == "23026" {
			return reuseCredential(cmd, svc, realm, hash1, hash1b, password, generated)
		}
		return faultExit(err)
	}
	return emitCredential(cmd, cred, password, generated)
}

// reuseCredential implements --if-not-exists after a 23026 duplicate. Identity
// is realm + username; desired state is both hashes plus the app binding.
func reuseCredential(cmd *cobra.Command, svc *sipsvc.Service, realm *sipsvc.Realm, hash1, hash1b, password string, generated bool) error {
	existing, err := svc.FindCredentialByUsername(realm.ID, credCreateUsername)
	if err != nil {
		return faultExit(err)
	}
	if existing.AppID != credCreateAppID {
		return fmt.Errorf("credential %q exists but is bound to a different application (%q) — delete and recreate it", credCreateUsername, existing.AppID)
	}
	if generated {
		return &cmdutil.SecretUnavailableError{
			Message: fmt.Sprintf("credential %q already exists and its password cannot be recovered — rotate it: band sip credential rotate %s --realm %s --generate-password", credCreateUsername, existing.ID, realm.Name),
		}
	}
	match, err := svc.CredentialHashesMatch(realm.ID, existing.ID, hash1, hash1b)
	if err != nil {
		return faultExit(err)
	}
	if !match {
		return fmt.Errorf("credential %q exists with a different password — rotate it: band sip credential rotate %s --realm %s --password-stdin", credCreateUsername, existing.ID, realm.Name)
	}
	return emitCredential(cmd, existing, password, false)
}

// emitCredential prints the credential. A generated password is included exactly
// once; a caller-supplied one is omitted because the caller already has it.
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
	return emit(format, plain, out)
}
