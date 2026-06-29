package site

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	createName        string
	createDescription string
	createIfNotExists bool
)

func init() {
	createCmd.Flags().StringVar(&createName, "name", "", "Sub-account name (required)")
	createCmd.Flags().StringVar(&createDescription, "description", "", "Sub-account description")
	createCmd.Flags().BoolVar(&createIfNotExists, "if-not-exists", false, "Return existing sub-account if one with the same name already exists")
	_ = createCmd.MarkFlagRequired("name")
	Cmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new sub-account",
	Long:  "Creates a new sub-account under the active account. Use --if-not-exists to make the command idempotent, returning the existing sub-account instead of erroring when one with the same name already exists.",
	Example: `  band subaccount create --name "Production"
  band subaccount create --name "Production" --description "Prod traffic"

  # Idempotent — safe to re-run
  band subaccount create --name "Production" --if-not-exists`,
	RunE: runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)

	if createIfNotExists {
		var listResult interface{}
		if err := client.Get(fmt.Sprintf("/accounts/%s/sites", acctID), &listResult); err != nil {
			return fmt.Errorf("listing sub-accounts: %w", err)
		}
		if existing := output.FindByName(listResult, "Name", createName); existing != nil {
			return output.StdoutAuto(format, plain, existing)
		}
	}

	bodyData := map[string]interface{}{
		"Name": createName,
	}
	if createDescription != "" {
		bodyData["Description"] = createDescription
	}

	var result interface{}
	if err := client.Post(fmt.Sprintf("/accounts/%s/sites", acctID), api.XMLBody{RootElement: "Site", Data: bodyData}, &result); err != nil {
		return fmt.Errorf("creating sub-account: %w", err)
	}

	return output.StdoutAuto(format, plain, result)
}
