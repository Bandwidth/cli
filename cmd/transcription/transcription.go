package transcription

import "github.com/spf13/cobra"

// Cmd is the `band transcription` parent command.
var Cmd = &cobra.Command{
	Use:   "transcription",
	Short: "Manage call recording transcriptions",
}
