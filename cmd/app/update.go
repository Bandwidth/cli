package app

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	updateCallbackURL string
)

func init() {
	updateCmd.Flags().StringVar(&updateCallbackURL, "callback-url", "", "Callback URL for voice or messaging events")
	Cmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update <app-id>",
	Short: "Update an application's settings",
	Long: `Updates an existing application. Currently supports changing the callback URL.

For messaging apps, this sets the URL where Bandwidth sends delivery status
webhooks (message-delivered, message-failed). Without a working callback URL,
you won't know whether messages were actually delivered.`,
	Example: `  # Update a messaging app's callback URL
  band app update abc-123 --callback-url https://your-server.example.com/callbacks`,
	Args: cobra.ExactArgs(1),
	RunE: runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) error {
	appID := args[0]
	if err := cmdutil.ValidateID(appID); err != nil {
		return err
	}
	if !cmd.Flags().Changed("callback-url") {
		return fmt.Errorf("at least one flag must be set (e.g. --callback-url)")
	}

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	// First, get the existing app to determine its type
	var existing interface{}
	if err := client.Get(fmt.Sprintf("/accounts/%s/applications/%s", acctID, url.PathEscape(appID)), &existing); err != nil {
		return fmt.Errorf("getting application: %w", err)
	}

	appType := detectAppType(existing)
	appName := findAppName(existing)

	var body api.XMLBody
	if appType == "messaging" {
		body = api.XMLBody{
			RootElement: "Application",
			Data: map[string]interface{}{
				"AppName":        appName,
				"ServiceType":    "Messaging-V2",
				"MsgCallbackUrl": updateCallbackURL,
				"CallbackUrl":    updateCallbackURL,
			},
		}
	} else {
		body = api.XMLBody{
			RootElement: "Application",
			Data: map[string]interface{}{
				"AppName":                  appName,
				"ServiceType":              "Voice-V2",
				"CallInitiatedCallbackUrl": updateCallbackURL,
			},
		}
	}

	var result interface{}
	if err := client.Put(fmt.Sprintf("/accounts/%s/applications/%s", acctID, url.PathEscape(appID)), body, &result); err != nil {
		return fmt.Errorf("updating application: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}

// detectAppType returns "messaging" or "voice" based on the app's ServiceType field.
func detectAppType(app interface{}) string {
	m, ok := app.(map[string]interface{})
	if !ok {
		return "voice"
	}
	// Walk nested maps looking for ServiceType
	return findServiceType(m)
}

func findAppName(app interface{}) string {
	m, ok := app.(map[string]interface{})
	if !ok {
		return ""
	}
	return findStringField(m, "AppName")
}

func findStringField(m map[string]interface{}, key string) string {
	for k, v := range m {
		if k == key {
			if s, ok := v.(string); ok {
				return s
			}
		}
		if nested, ok := v.(map[string]interface{}); ok {
			if found := findStringField(nested, key); found != "" {
				return found
			}
		}
	}
	return ""
}

func findServiceType(m map[string]interface{}) string {
	for k, v := range m {
		if k == "ServiceType" {
			if s, ok := v.(string); ok {
				if s == "Messaging-V2" {
					return "messaging"
				}
				return "voice"
			}
		}
		if nested, ok := v.(map[string]interface{}); ok {
			if result := findServiceType(nested); result != "" {
				return result
			}
		}
	}
	return ""
}
