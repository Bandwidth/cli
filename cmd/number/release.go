package number

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(releaseCmd)
}

var releaseCmd = &cobra.Command{
	Use:   "release [number]",
	Short: "Release a phone number",
	Args:  cobra.ExactArgs(1),
	RunE:  runRelease,
}

func runRelease(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	bodyData := map[string]interface{}{
		"TelephoneNumberList": map[string]interface{}{
			"TelephoneNumber": []string{args[0]},
		},
	}

	var result interface{}
	if err := client.Post(fmt.Sprintf("/accounts/%s/disconnects", acctID), api.XMLBody{RootElement: "DisconnectTelephoneNumberOrder", Data: bodyData}, &result); err != nil {
		return fmt.Errorf("releasing number: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
