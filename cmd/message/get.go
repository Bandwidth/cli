package message

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get [messageId]",
	Short: "Get message metadata by ID",
	Long:  "Retrieves metadata for a specific message. Note: Bandwidth does not store message content — only metadata (timestamps, direction, segment count) is returned.",
	Args:  cobra.ExactArgs(1),
	RunE:  runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}
	client, acctID, err := cmdutil.MessagingClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(fmt.Sprintf("/users/%s/messages?messageId=%s", acctID, url.QueryEscape(args[0])), &result); err != nil {
		return fmt.Errorf("getting message: %w", err)
	}

	// The API returns a search wrapper { "messages": [...], "pageInfo": {}, "totalCount": 1 }.
	// Unwrap to return just the single message object.
	if m, ok := result.(map[string]interface{}); ok {
		if msgs, ok := m["messages"].([]interface{}); ok && len(msgs) == 1 {
			result = msgs[0]
		}
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
