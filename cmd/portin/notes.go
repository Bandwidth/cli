package portin

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	notesCmd.AddCommand(notesAddCmd)
	notesCmd.AddCommand(notesListCmd)
	Cmd.AddCommand(notesCmd)
}

var notesCmd = &cobra.Command{
	Use:   "notes",
	Short: "Add or list notes on a port-in order (used to communicate with Bandwidth's LNP team)",
}

var notesAddCmd = &cobra.Command{
	Use:     "add <order-id> <text>",
	Short:   "Add a note to a port-in order",
	Example: `  band portin notes add b9ef682b-2b42-4287-bfe4-ba03ec57cb07 "Please expedite — customer outage"`,
	Args:    cobra.ExactArgs(2),
	RunE:    runNotesAdd,
}

var notesListCmd = &cobra.Command{
	Use:     "list <order-id>",
	Short:   "List notes on a port-in order",
	Example: `  band portin notes list b9ef682b-2b42-4287-bfe4-ba03ec57cb07`,
	Args:    cobra.ExactArgs(1),
	RunE:    runNotesList,
}

func runNotesAdd(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	body := map[string]interface{}{
		"UserId":      cmdutil.ActiveUserID(),
		"Description": args[1],
	}

	var result interface{}
	if err := client.Post(
		fmt.Sprintf("/accounts/%s/portins/%s/notes", acctID, args[0]),
		api.XMLBody{RootElement: "Note", Data: body},
		&result,
	); err != nil {
		return portinError(err, "adding note to port-in order")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, map[string]interface{}{
			"orderId": args[0],
			"noteId":  digString(result, "NoteId"),
		})
	}
	return output.StdoutAuto(format, plain, result)
}

func runNotesList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(
		fmt.Sprintf("/accounts/%s/portins/%s/notes", acctID, args[0]),
		&result,
	); err != nil {
		return portinError(err, "listing notes for port-in order")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenNotes(result))
	}
	return output.StdoutAuto(format, plain, result)
}

func flattenNotes(result interface{}) []map[string]interface{} {
	out := []map[string]interface{}{}
	walkNotes(result, &out)
	return out
}

func walkNotes(v interface{}, out *[]map[string]interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		// A Note map has at least an Id and a Description.
		_, hasID := val["Id"]
		_, hasDesc := val["Description"]
		if hasID && hasDesc {
			*out = append(*out, map[string]interface{}{
				"noteId":    digString(val, "Id"),
				"timestamp": digString(val, "LastDateModifier"),
				"actor":     digString(val, "UserId"),
				"text":      digString(val, "Description"),
			})
			return
		}
		for _, child := range val {
			walkNotes(child, out)
		}
	case []interface{}:
		for _, item := range val {
			walkNotes(item, out)
		}
	}
}
