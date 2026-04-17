package location

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	createSiteID      string
	createName        string
	createIfNotExists bool
)

func init() {
	createCmd.Flags().StringVar(&createSiteID, "site", "", "Sub-account ID (required)")
	createCmd.Flags().StringVar(&createName, "name", "", "Location name (required)")
	createCmd.Flags().BoolVar(&createIfNotExists, "if-not-exists", false, "Return existing location if one with the same name already exists")
	_ = createCmd.MarkFlagRequired("site")
	_ = createCmd.MarkFlagRequired("name")
	Cmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new location (SIP peer) under a sub-account",
	RunE:  runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)

	if createIfNotExists {
		var listResult interface{}
		listPath := fmt.Sprintf("/accounts/%s/sites/%s/sippeers", acctID, createSiteID)
		if err := client.Get(listPath, &listResult); err != nil {
			return fmt.Errorf("listing locations: %w", err)
		}
		if existing := output.FindByName(listResult, "PeerName", createName); existing != nil {
			return output.StdoutAuto(format, plain, existing)
		}
	}

	bodyData := map[string]interface{}{
		"PeerName": createName,
	}

	var result interface{}
	path := fmt.Sprintf("/accounts/%s/sites/%s/sippeers", acctID, createSiteID)
	if err := client.Post(path, api.XMLBody{RootElement: "SipPeer", Data: bodyData}, &result); err != nil {
		return fmt.Errorf("creating location: %w", err)
	}

	return output.StdoutAuto(format, plain, result)
}

