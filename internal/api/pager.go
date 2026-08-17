package api

import "fmt"

// PageFetcher retrieves one page. Implementations are usually a closure over
// a service method, e.g. func(l, o int) (*Envelope, error) { return svc.List(l, o, filters) }.
type PageFetcher func(limit, offset int) (*Envelope, error)

// ForEachPage walks every page and hands each batch to fn.
//
// Termination is driven by the page block's totalElements, never by observing
// a short page: a full final page is indistinguishable from "more to come"
// otherwise, and a short page in the middle is legal. Callers therefore do not
// track a cumulative count themselves — getting that wrong is the failure mode
// Page.Truncated's doc comment warns about, and this loop exists so nobody has
// to get it right more than once.
//
// Fails closed when a response carries no page metadata. Returning the first
// page as if it were the whole result would look like success.
func ForEachPage(fetch PageFetcher, pageSize int, fn func([]any) error) error {
	if pageSize <= 0 {
		pageSize = 50
	}
	seen := 0
	for {
		env, err := fetch(pageSize, seen)
		if err != nil {
			return err
		}
		if env.Page == nil {
			return fmt.Errorf("response has no page metadata; refusing to report a partial result as complete")
		}
		batch, err := env.List()
		if err != nil {
			return err
		}
		if err := fn(batch); err != nil {
			return err
		}
		seen += len(batch)
		if !env.Page.Truncated(seen) {
			return nil
		}
		// A page that returns nothing while claiming more remain would spin forever.
		if len(batch) == 0 {
			return fmt.Errorf("page at offset %d returned no items but %d of %d were expected",
				seen, seen, env.Page.TotalElements)
		}
	}
}
