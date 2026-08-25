package number

import (
	"fmt"
	"net/url"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// listOptions carries every `band number list` flag that selects an endpoint.
type listOptions struct {
	Status       string
	NpaNxx       string
	State        string
	RateCenter   string
	Lata         string
	Subaccount   string
	Location     string
	Disconnected bool
}

// geoFiltered reports whether any NANP geography filter is set.
func (o listOptions) geoFiltered() bool {
	return o.NpaNxx != "" || o.State != "" || o.RateCenter != "" || o.Lata != ""
}

// validate rejects flag combinations the API cannot serve. It runs before
// authentication so misuse fails fast with a FlagError (exit 6).
func (o listOptions) validate() error {
	if o.Disconnected && (o.Status != "" || o.geoFiltered() || o.Subaccount != "" || o.Location != "") {
		return cmdutil.NewFlagError("--disconnected cannot be combined with other filters")
	}
	if o.Location != "" && o.Subaccount == "" {
		return cmdutil.NewFlagError("--location requires --subaccount")
	}
	if o.RateCenter != "" && o.State == "" {
		return cmdutil.NewFlagError("--ratecenter requires --state (API constraint)")
	}
	if o.Status != "" && (o.geoFiltered() || o.Subaccount != "") {
		return cmdutil.NewFlagError("--status cannot be combined with geography or sub-account filters (filtered lists return in-service numbers only)")
	}
	if o.Location != "" && o.geoFiltered() {
		return cmdutil.NewFlagError("geography filters cannot be combined with --location")
	}
	return nil
}

// pageStyle names the pagination dialect a list endpoint speaks. The
// Dashboard read endpoints disagree: inserviceNumbers and discnumbers define
// `page` as the 1-based ID of the FIRST ELEMENT of the page (1, 1001, 2001,
// ...), while the sippeer tns endpoint defines it as an ordinary page number
// (1, 2, 3, ...).
type pageStyle int

const (
	pageByFirstElementID pageStyle = iota
	pageByNumber
)

// listQuery describes one page-parameterized list request. Page and size are
// appended by the fetch loop, not here.
type listQuery struct {
	Path      string
	Query     url.Values
	PageStyle pageStyle
}

// buildListQuery maps validated options onto the Dashboard list endpoints.
// nil means "use the default /tns path" (the historical behavior, preserved
// because /tns works for credentials without the inservice role).
func buildListQuery(acctID string, o listOptions) *listQuery {
	if o.Disconnected {
		return &listQuery{Path: fmt.Sprintf("/accounts/%s/discnumbers", acctID), Query: url.Values{}, PageStyle: pageByFirstElementID}
	}

	q := url.Values{}
	if o.NpaNxx != "" {
		q.Set("npaNxx", o.NpaNxx)
	}
	if o.State != "" {
		q.Set("state", o.State)
	}
	if o.Lata != "" {
		q.Set("lata", o.Lata)
	}

	switch {
	case o.Subaccount != "" && o.Location != "":
		return &listQuery{Path: fmt.Sprintf("/accounts/%s/sites/%s/sippeers/%s/tns", acctID, o.Subaccount, o.Location), Query: q, PageStyle: pageByNumber}
	case o.Subaccount != "":
		// The site-level endpoint documents this parameter as "rateCenter";
		// the account-level endpoint documents it as "ratecenter". Follow
		// each endpoint's published casing.
		if o.RateCenter != "" {
			q.Set("rateCenter", o.RateCenter)
		}
		return &listQuery{Path: fmt.Sprintf("/accounts/%s/sites/%s/inserviceNumbers", acctID, o.Subaccount), Query: q, PageStyle: pageByFirstElementID}
	case o.geoFiltered():
		if o.RateCenter != "" {
			q.Set("ratecenter", o.RateCenter)
		}
		return &listQuery{Path: fmt.Sprintf("/accounts/%s/inserviceNumbers", acctID), Query: q, PageStyle: pageByFirstElementID}
	default:
		return nil
	}
}
