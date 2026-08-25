package tendlc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Bandwidth/cli/internal/api"
)

// putReplaceWithReadOnlyRetry PUTs body to path and, on a specific and
// recognizable failure, retries exactly once.
//
// Both `brand update` and `campaign update` build body by copying the
// resource the API just returned and stripping a known list of read-only
// keys (brandReadOnlyFields, campaignReadOnlyFields) before overlaying the
// caller's changes. That only works because production currently ACCEPTS
// read-only fields it does not use — measured, and flagged to the API team as
// a behavior this CLI depends on. If that is ever tightened to a 400 without
// warning, both commands break the same day, because the strip lists cannot
// enumerate every field the API might start rejecting.
//
// The failure is recognizable: a 400 whose error pointers name fields we
// actually sent. On exactly that shape, this drops the named fields from a
// copy of body and PUTs once more. Any other 400 — one naming a field we did
// not send, or with no usable pointers at all — is a genuine validation
// failure and passes straight through. So does every non-400 status; a 409
// (conflict) or 422 means something other than "the API rejected a
// read-only field", and looping or guessing there would turn a clear error
// into a confusing double-request.
//
// INVARIANT: the retry may only ever drop a field the CLI does not model at
// all. Anything reachable by an update flag is mutable customer data whether
// or not the caller happened to touch it THIS invocation — an earlier version
// of this invariant was "don't drop what the caller changed this call", and
// that is not sufficient: an unchanged field still holds real, previously-set
// customer data. Concretely: a brand has website "https://example.com", set
// months ago. The caller runs "brand update --display-name 'New Name'",
// touching nothing else. The PUT 400s naming "/website" because the stored
// value no longer passes current validation. website is not read-only, and
// the caller didn't touch it this call — but it is still reachable by
// --website, so it is still real data, and dropping it would silently null
// the brand's site on a request that never mentioned it. The only field that
// is genuinely safe to drop is one the CLI never models at all — the entire
// case this retry was designed for. neverDrop is how the invariant is
// enforced: it is the set of JSON body keys this call must never remove,
// regardless of what the error names. UpdateBrand and UpdateCampaign each
// build it from the WHOLE update flag surface (every JSON key any update flag
// can write, not merely the ones changed this call) and, for campaigns, union
// it with a fixed set of fields that are never reachable by any update flag
// at all but are still real data, not read-only filler (see
// campaignNeverDropFields). Without that second category, a field the CLI
// never lets the caller touch would look, from here, indistinguishable from a
// genuinely-inert read-only field — which is exactly the shape production
// returns for a direct campaign's
// subscriberOptin/subscriberOptout/subscriberHelp attestations.
//
// On success after a retry, a note naming the dropped fields goes to stderr:
// a silent self-heal would hide an API change worth knowing about. On a
// retry that also fails, the ORIGINAL error is returned, not the retry's —
// the first response is the one that describes what the caller actually
// sent.
//
// UpdateBrand and UpdateCampaign share this rather than each rolling their
// own copy: both are already identical PUT-then-parse-envelope shapes, and
// this series has already had one bug from a fix landing on one arm and not
// its twin.
func putReplaceWithReadOnlyRetry(ctx context.Context, client *api.Client, path string, body map[string]any, neverDrop map[string]bool) ([]byte, error) {
	raw, err := client.PutRawJSON(ctx, path, body)
	if err == nil {
		return raw, nil
	}

	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.StatusCode != 400 {
		return nil, err
	}

	named := topLevelPointerFields(apiErr.Body)
	var drop []string
	for _, f := range named {
		if neverDrop[f] {
			// The caller either explicitly asked to set this field this
			// call, or it is a field that is never reachable by any flag but
			// still holds real data (see neverDrop's callers). Either way,
			// dropping it would silently discard something that matters more
			// than the retry's convenience.
			continue
		}
		if _, present := body[f]; present {
			drop = append(drop, f)
		}
	}
	if len(drop) == 0 {
		// Nothing named in the error is a droppable field we sent: this is a
		// genuine validation failure (a field the caller set, a protected
		// field, or a pointer naming only nested data), not the shape this
		// retry exists for.
		return nil, err
	}

	retryBody := make(map[string]any, len(body))
	for k, v := range body {
		retryBody[k] = v
	}
	for _, f := range drop {
		delete(retryBody, f)
	}

	raw2, err2 := client.PutRawJSON(ctx, path, retryBody)
	if err2 != nil {
		// The retry's own failure is discarded on purpose: the original error
		// is the one that describes what the caller did.
		return nil, err
	}

	sort.Strings(drop)
	fmt.Fprintf(os.Stderr, "note: retried after dropping field(s) the API rejected but does not need: %s\n",
		strings.Join(drop, ", "))
	return raw2, nil
}

// topLevelPointerFields parses an API error body shaped like
// {"errors":[{"source":{"POINTER":"/phone"}}], "links":[]} and returns the
// field names named by TOP-LEVEL, single-segment JSON pointers — "/phone"
// contributes "phone". A pointer with more than one segment, e.g.
// "/accounts[0]/customerProfileId", names a field nested inside a structure,
// not a top-level key of the body we sent: dropping the whole top-level key
// on that signal would discard data the caller supplied that has nothing to
// do with the rejected sub-field. Those pointers are excluded here by
// requiring the part after the leading "/" to contain no further "/".
//
// Malformed or unparseable bodies yield no fields, which the caller treats
// as "pass through, no retry" the same as a 400 with no usable pointers.
func topLevelPointerFields(body string) []string {
	var parsed struct {
		Errors []struct {
			Source struct {
				Pointer string `json:"POINTER"`
			} `json:"source"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var out []string
	for _, e := range parsed.Errors {
		p := e.Source.Pointer
		if !strings.HasPrefix(p, "/") {
			continue
		}
		field := p[1:]
		if field == "" || strings.Contains(field, "/") {
			continue
		}
		if seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}
