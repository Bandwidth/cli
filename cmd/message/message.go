package message

import (
	"github.com/spf13/cobra"

	mediacmd "github.com/Bandwidth/cli/cmd/message/media"
)

// Cmd is the `band message` parent command.
var Cmd = &cobra.Command{
	Use:   "message",
	Short: "Send and manage SMS/MMS messages",
	Long:  "Send SMS and MMS messages, look up message metadata, and manage the media files used in MMS. Delivery is asynchronous — status arrives via webhook callbacks on your messaging application.",
}

func init() {
	Cmd.AddCommand(mediacmd.Cmd)
}
