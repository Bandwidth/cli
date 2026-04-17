package call

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	createFrom      string
	createTo        string
	createAppID     string
	createAnswerURL string
	createWait      bool
	createTimeout   time.Duration
)

// terminalCallStates are call states that indicate the call is finished.
// The Voice API uses: queued → initiated → answered → disconnected.
// "disconnected" is the only terminal state; the reason is in disconnectCause.
var terminalCallStates = map[string]bool{
	"disconnected": true,
}

func init() {
	createCmd.Flags().StringVar(&createFrom, "from", "", "Caller ID (required)")
	createCmd.Flags().StringVar(&createTo, "to", "", "Destination number (required)")
	createCmd.Flags().StringVar(&createAppID, "app-id", "", "Application ID (required)")
	createCmd.Flags().StringVar(&createAnswerURL, "answer-url", "", "Answer callback URL (required)")
	createCmd.Flags().BoolVar(&createWait, "wait", false, "Wait until the call reaches a terminal state")
	createCmd.Flags().DurationVar(&createTimeout, "timeout", 120*time.Second, "Maximum time to wait (default 120s)")
	_ = createCmd.MarkFlagRequired("from")
	_ = createCmd.MarkFlagRequired("to")
	_ = createCmd.MarkFlagRequired("app-id")
	_ = createCmd.MarkFlagRequired("answer-url")
	Cmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Make an outbound voice call",
	Long:  "Initiates an outbound voice call. The call starts dialing immediately. Bandwidth will POST to the answer-url for BXML instructions when the call connects.",
	Example: `  # Fire and forget
  band call create --from +19195551234 --to +15559876543 --app-id abc-123 --answer-url https://example.com/voice

  # Wait for call to complete
  band call create --from +19195551234 --to +15559876543 --app-id abc-123 --answer-url https://example.com/voice --wait`,
	RunE: runCreate,
}

// CreateOpts holds the parameters for creating a call.
type CreateOpts struct {
	From      string
	To        string
	AppID     string
	AnswerURL string
}

// BuildCreateBody builds the request body for creating a call.
func BuildCreateBody(opts CreateOpts) map[string]string {
	return map[string]string{
		"from":          opts.From,
		"to":            opts.To,
		"applicationId": opts.AppID,
		"answerUrl":     opts.AnswerURL,
	}
}

func runCreate(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.VoiceClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	reqBody := BuildCreateBody(CreateOpts{
		From:      createFrom,
		To:        createTo,
		AppID:     createAppID,
		AnswerURL: createAnswerURL,
	})

	var result interface{}
	if err := client.Post(fmt.Sprintf("/accounts/%s/calls", acctID), reqBody, &result); err != nil {
		return fmt.Errorf("creating call: %w", err)
	}

	if !createWait {
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, result)
	}

	// Extract the call ID from the response to poll with.
	callID, err := extractCallID(result)
	if err != nil {
		return fmt.Errorf("--wait: could not determine call ID from response: %w", err)
	}

	final, err := cmdutil.Poll(cmdutil.PollConfig{
		Interval: 2 * time.Second,
		Timeout:  createTimeout,
		Check: func() (bool, interface{}, error) {
			var callState interface{}
			if err := client.Get(fmt.Sprintf("/accounts/%s/calls/%s", acctID, url.PathEscape(callID)), &callState); err != nil {
				// The Voice API is eventually consistent — a 404 right after
				// creation means the call record hasn't propagated yet. Retry.
				var apiErr *api.APIError
				if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
					return false, nil, nil
				}
				return false, nil, fmt.Errorf("polling call state: %w", err)
			}
			m, ok := callState.(map[string]interface{})
			if !ok {
				return false, nil, nil
			}
			state, _ := m["state"].(string)
			if terminalCallStates[state] {
				return true, callState, nil
			}
			return false, nil, nil
		},
	})
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, final)
}

// extractCallID pulls the callId field out of a create-call response.
func extractCallID(result interface{}) (string, error) {
	m, ok := result.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected response type %T", result)
	}
	for _, key := range []string{"callId", "id", "CallId"} {
		if v, ok := m[key].(string); ok && v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("callId not found in response")
}
