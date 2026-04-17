package app

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	"github.com/Bandwidth/cli/internal/ui"
)

var (
	assignSite     string
	assignLocation string
)

func init() {
	assignCmd.Flags().StringVar(&assignSite, "site", "", "Sub-account ID (required)")
	assignCmd.Flags().StringVar(&assignLocation, "location", "", "Location (SIP peer) ID (required)")
	_ = assignCmd.MarkFlagRequired("site")
	_ = assignCmd.MarkFlagRequired("location")
	Cmd.AddCommand(assignCmd)
}

var assignCmd = &cobra.Command{
	Use:   "assign <app-id>",
	Short: "Link a messaging application to a location",
	Long: `Assigns a messaging application to a location (SIP peer). All phone numbers
in that location will use this application for messaging.

This is required before you can send messages — the from number must be in a
location that has a messaging application assigned to it.`,
	Example: `  # Assign messaging app to a location
  band app assign abc-123 --site 152681 --location 970014

  # Find your site and location IDs first
  band subaccount list
  band location list --site <site-id>`,
	Args: cobra.ExactArgs(1),
	RunE: runAssign,
}

func runAssign(cmd *cobra.Command, args []string) error {
	appID := args[0]
	if err := cmdutil.ValidateID(appID); err != nil {
		return err
	}

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	body := api.XMLBody{
		RootElement: "ApplicationsSettings",
		Data: map[string]interface{}{
			"HttpMessagingV2AppId": appID,
		},
	}

	path := fmt.Sprintf("/accounts/%s/sites/%s/sippeers/%s/products/messaging/applicationSettings",
		acctID, url.PathEscape(assignSite), url.PathEscape(assignLocation))

	var result interface{}
	if err := client.Put(path, body, &result); err != nil {
		return fmt.Errorf("assigning application to location: %w", err)
	}

	ui.Successf("Application %s assigned to location %s (site %s)", appID, assignLocation, assignSite)

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
