package vcp

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(assignCmd)
}

var assignCmd = &cobra.Command{
	Use:   "assign <vcp-id> <number> [number...]",
	Short: "Assign phone numbers to a VCP",
	Long:  "Assigns one or more phone numbers to a Voice Configuration Package. Numbers must be in E.164 format and owned by the account.",
	Example: `  band vcp assign abc-123-def +19195551234
  band vcp assign abc-123-def +19195551234 +19195551235 +19195551236`,
	Args: cobra.MinimumNArgs(2),
	RunE: runAssign,
}

// BuildAssignBody builds the bulk assign request body.
func BuildAssignBody(numbers []string) map[string]interface{} {
	return map[string]interface{}{
		"action":       "ADD",
		"phoneNumbers": numbers,
	}
}

func runAssign(cmd *cobra.Command, args []string) error {
	vcpID := args[0]
	if err := cmdutil.ValidateID(vcpID); err != nil {
		return err
	}

	numbers := args[1:]

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	body := BuildAssignBody(numbers)

	var result interface{}
	if err := client.Post(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages/%s/phoneNumbers/bulk", acctID, vcpID), body, &result); err != nil {
		return fmt.Errorf("assigning numbers to VCP: %w", err)
	}

	// The API returns null on success. Print a confirmation message instead.
	if result == nil {
		plural := "number"
		if len(numbers) > 1 {
			plural = "numbers"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Assigned %d %s to VCP %s.\n", len(numbers), plural, vcpID)
		return nil
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
