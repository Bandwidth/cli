package cmdutil

import "github.com/Bandwidth/cli/internal/api"

// ClientFunc builds an authenticated API client for an optional account-id
// override, returning the resolved account ID. The client constructors
// (e.g. VoiceClient) are vars of this type so tests can substitute a fake
// that implements api.Requester. api.Requester is the interface *api.Client
// already satisfies.
type ClientFunc func(accountIDOverride string) (api.Requester, string, error)
