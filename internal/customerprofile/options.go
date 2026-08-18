package customerprofile

import (
	"fmt"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// readOnlyFields are rejected or ignored by the API on write. version is
// deliberately NOT in this list: the schema marks it readOnly, but production
// requires it on every update and returns 409 without it.
var readOnlyFields = []string{"id", "accountId", "createdDate", "modifiedDate", "totalCampaigns"}

// CreateOptions is the flag surface of `customer-profile create`.
type CreateOptions struct {
	Name         string
	Website      string
	ContactName  string
	ContactPhone string
	ContactEmail string
	AddressID    string
}

// UpdateOptions is the flag surface of `customer-profile update`. Whether a
// field was explicitly set is tracked separately, in the changed map, because
// an empty string is a legitimate value meaning "clear this".
type UpdateOptions struct {
	Name         string
	Website      string
	ContactName  string
	ContactPhone string
	ContactEmail string
	AddressID    string
}

// ValidateCreate checks what can be checked locally. Semantic rules the API
// owns are left to the API — duplicating them here guarantees drift.
func ValidateCreate(o CreateOptions) error {
	var missing []string
	if o.Name == "" {
		missing = append(missing, "name")
	}
	// The API's contact object requires a name whenever a contact is present.
	if o.ContactName == "" && (o.ContactPhone != "" || o.ContactEmail != "") {
		missing = append(missing, "contact-name")
	}
	if len(missing) > 0 {
		return cmdutil.NewMissingFlagsError(missing)
	}
	return nil
}

// BuildCreateRequest builds the POST body, omitting anything unset. An empty
// string is omitted rather than sent, so create never writes a blank over a
// server-side default.
func BuildCreateRequest(o CreateOptions) map[string]any {
	body := map[string]any{"name": o.Name}
	setIf(body, "website", o.Website)
	setIf(body, "addressId", o.AddressID)

	contact := map[string]any{}
	setIf(contact, "name", o.ContactName)
	setIf(contact, "phoneNumber", o.ContactPhone)
	setIf(contact, "email", o.ContactEmail)
	if len(contact) > 0 {
		body["contact"] = contact
	}
	return body
}

// BuildUpdateRequest produces a full-replacement PUT body that cannot drop
// fields the CLI does not model.
//
// PUT replaces the whole resource, so anything missing from the body is nulled
// server-side. Building the body from a typed struct would therefore delete
// every production field we never modeled — universalEin-style fields exist on
// other resources and will exist here eventually. So the body starts as a copy
// of what the API just gave us, read-only fields are removed, and only
// explicitly-changed flags are overlaid. Validation stays typed; the payload
// stays lossless.
func BuildUpdateRequest(current map[string]any, o UpdateOptions, changed map[string]bool) (map[string]any, error) {
	if current == nil {
		return nil, fmt.Errorf("no current resource to update from")
	}
	if _, ok := current["version"]; !ok {
		return nil, fmt.Errorf("current resource has no version; the API rejects updates without it")
	}

	body := deepCopyMap(current)
	for _, ro := range readOnlyFields {
		delete(body, ro)
	}

	overlayIfChanged(body, changed, "name", "name", o.Name)
	overlayIfChanged(body, changed, "website", "website", o.Website)
	overlayIfChanged(body, changed, "address-id", "addressId", o.AddressID)

	if changed["contact-name"] || changed["contact-phone"] || changed["contact-email"] {
		// body is already a deepCopyMap of current, so body["contact"] (if
		// present) is already an independent copy — safe to mutate in place
		// without recopying it key-by-key into a second new map.
		contact, ok := body["contact"].(map[string]any)
		if !ok {
			contact = map[string]any{}
		}
		overlayIfChanged(contact, changed, "contact-name", "name", o.ContactName)
		overlayIfChanged(contact, changed, "contact-phone", "phoneNumber", o.ContactPhone)
		overlayIfChanged(contact, changed, "contact-email", "email", o.ContactEmail)
		body["contact"] = contact
	}

	if err := ValidateUpdate(body); err != nil {
		return nil, err
	}
	return body, nil
}

// ValidateUpdate checks the fully overlaid PUT body — the object about to go
// over the wire — not the options struct. A struct that looks fine in
// isolation (e.g. an empty --name with no other flags set) can still combine
// with the read profile into a body the API rejects. Catching that here means
// the failure is a local, zero-request FlagError (exit 6) instead of a raw
// 400 surfaced from the API side, measured against production: --name ""
// sends a null name and the API answers 400 "name must not be null".
//
// Kept as its own exported function, called from BuildUpdateRequest, so the
// check is independently testable and update.go's control flow (surface the
// error before calling svc.Update) falls out for free — BuildUpdateRequest
// already runs, and is already checked, before svc.Update.
func ValidateUpdate(body map[string]any) error {
	if name, ok := body["name"].(string); !ok || name == "" {
		return cmdutil.NewFlagError("name must not be empty or null")
	}
	if contact, ok := body["contact"].(map[string]any); ok {
		if name, ok := contact["name"].(string); !ok || name == "" {
			return cmdutil.NewFlagError("contact-name must not be empty when a contact is present")
		}
	}
	return nil
}

// BuildRestoreRequest undoes a soft delete.
//
// Sends softDeleted:false. The published docs say to send {"deleted": false},
// which returns 404 "Customer profile not found" even though GET returns the
// record — measured against production, reported as MV-23429.
func BuildRestoreRequest(current map[string]any) (map[string]any, error) {
	body, err := BuildUpdateRequest(current, UpdateOptions{}, map[string]bool{})
	if err != nil {
		return nil, err
	}
	body["softDeleted"] = false
	delete(body, "deleted")
	return body, nil
}

func setIf(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

// overlayIfChanged writes val only when the caller explicitly set that flag.
// flagName is the CLI flag; field is the JSON key.
//
// An explicitly empty value is written as JSON null, not "". The API rejects
// an empty string on at least website ("size must be between 1 and 500"),
// measured against production, but accepts null and clears the field. Since
// the CLI's documented way to clear a field is passing the flag with an empty
// value, that value must become null on the wire.
func overlayIfChanged(m map[string]any, changed map[string]bool, flagName, field, val string) {
	if !changed[flagName] {
		return
	}
	if val == "" {
		m[field] = nil
		return
	}
	m[field] = val
}

// deepCopyMap copies m so the result shares no mutable structure with it.
// current is read from an api.Envelope the caller may reuse or cache, so a
// shallow copy would leave nested maps (e.g. "contact") aliased between the
// outgoing body and the caller's data — any in-place mutation of one would
// silently corrupt the other. Nested map[string]any values, and []any slices
// that may themselves contain maps, are copied recursively; other values
// (strings, float64, bool, nil) are immutable in Go and safe to share.
func deepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(v any) any {
	switch vv := v.(type) {
	case map[string]any:
		return deepCopyMap(vv)
	case []any:
		out := make([]any, len(vv))
		for i, e := range vv {
			out[i] = deepCopyValue(e)
		}
		return out
	default:
		return v
	}
}
