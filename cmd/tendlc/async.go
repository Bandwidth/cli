package tendlc

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
)

// pollTarget describes what to poll and how to interpret it.
type pollTarget struct {
	// Noun names the resource in messages: "brand", "vetting".
	Noun string
	// Fetch reads the resource. found=false means the API answered 404, which
	// is "not readable yet" for creates and "done" for deletes — see
	// GoneIsDone. An error is a hard transport or decode failure.
	Fetch func() (obj map[string]any, found bool, err error)
	// Classify maps a fetched resource to an outcome. Called only when found.
	Classify func(obj map[string]any) tendlcsvc.StateClass
	// Remediate returns operator-facing next steps for a business-failure
	// state. Optional; "" prints nothing.
	Remediate func(obj map[string]any) string
	// GoneIsDone makes a 404 the success condition, for delete polls.
	GoneIsDone bool
}

// awaitTerminal polls until the resource settles, then writes stdout itself.
//
// It owns stdout on every path because of the one rule that matters here:
// after a 202 the write may already have succeeded, so the accepted resource's
// ID is the single piece of information that cannot be recovered if this
// command exits without printing it. A wrapped error alone prints no
// structured receipt, which is exactly the failure this function exists to
// prevent — modeled on cmd/quickstart's failWithPartial.
//
// receipt is what gets printed on every non-success outcome. It must already
// contain the ID from the 202 and, where one exists, a resume command.
func awaitTerminal(cmd *cobra.Command, t pollTarget, receipt map[string]any, timeout, interval time.Duration) error {
	format, plain := cmdutil.OutputFlags(cmd)

	// emitReceipt writes the partial-result receipt. A failure to write it is
	// deliberately swallowed in favor of the original error: the caller needs
	// to know the operation failed more than it needs to know stdout was
	// closed, and returning the write error would bury the real cause.
	//
	// This prints via output.Stdout, not output.StdoutAuto, deliberately: the
	// receipt is a value we built ourselves, not a raw API envelope, so it
	// never needs FlattenResponse's envelope-unwrapping. That matters because
	// FlattenResponse treats ANY single-key map as a wrapper and unwraps it to
	// its bare value — a receipt of just {"bandwidthId": "WABC"} would print
	// as the bare string "WABC" instead of a JSON object, which is exactly
	// the "no structured receipt" failure this function exists to prevent.
	emitReceipt := func() {
		_ = output.Stdout(format, receipt)
	}

	result, err := cmdutil.Poll(cmdutil.PollConfig{
		Context:  cmd.Context(),
		Interval: interval,
		Timeout:  timeout,
		Check: func() (bool, interface{}, error) {
			obj, found, err := t.Fetch()
			if err != nil {
				return false, nil, err
			}
			if !found {
				// 404: done for a delete poll, not-ready-yet for anything else.
				return t.GoneIsDone, nil, nil
			}
			if t.GoneIsDone {
				return false, nil, nil
			}
			switch t.Classify(obj) {
			case tendlcsvc.StateSuccess, tendlcsvc.StateFailure:
				return true, obj, nil
			default:
				return false, nil, nil
			}
		},
	})
	if err != nil {
		// Timeout, cancellation, or a transport failure after acceptance. All
		// three get the receipt: the write may have landed.
		emitReceipt()
		return err
	}

	if result == nil {
		// A delete poll that found the resource gone. There is no final
		// resource to print, so the receipt IS the success output.
		emitReceipt()
		return nil
	}

	final, ok := result.(map[string]any)
	if !ok {
		emitReceipt()
		return fmt.Errorf("polling %s returned an unexpected shape (%T)", t.Noun, result)
	}

	if t.Classify(final) == tendlcsvc.StateFailure {
		// final is real API data (not a synthetic receipt), so StdoutAuto's
		// flatten-on-plain behavior is correct here — see emitReceipt above
		// for why receipts print differently.
		//
		// The write error is reported but does not replace the return value:
		// this is a business failure regardless of whether the write
		// succeeded, and returning the bare write error here would drop the
		// ConflictError classification (exit 4) along with the remediation
		// text below, silently downgrading a failed brand/vetting into
		// "something went wrong printing it."
		if err := output.StdoutAuto(format, plain, final); err != nil {
			cmd.PrintErrln(fmt.Sprintf("writing result: %v", err))
		}
		msg := fmt.Sprintf("%s did not complete successfully", t.Noun)
		if t.Remediate != nil {
			if r := t.Remediate(final); r != "" {
				msg = r
			}
		}
		cmd.PrintErrln(msg)
		return &cmdutil.ConflictError{Message: msg}
	}

	// final is real API data, not a synthetic receipt — see emitReceipt above
	// for why receipts go through output.Stdout instead of StdoutAuto.
	return output.StdoutAuto(format, plain, final)
}

// fetchBrand adapts a brand read into pollTarget.Fetch, translating a 404 into
// found=false rather than an error.
func fetchBrand(svc *tendlcsvc.Service, brandID string) func() (map[string]any, bool, error) {
	return func() (map[string]any, bool, error) {
		env, err := svc.GetBrand(brandID)
		if err != nil {
			if isNotFound(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		obj, err := env.Object()
		if err != nil {
			return nil, false, err
		}
		return obj, true, nil
	}
}
