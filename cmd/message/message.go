package message

import (
	"github.com/spf13/cobra"

	mediacmd "github.com/Bandwidth/cli/cmd/message/media"
)

// Cmd is the `band message` parent command.
var Cmd = &cobra.Command{
	Use:   "message",
	Short: "Send and manage SMS/MMS messages",
}

func init() {
	Cmd.AddCommand(mediacmd.Cmd)
}
