package transcription

import "github.com/spf13/cobra"

// Cmd is the `band transcription` parent command.
var Cmd = &cobra.Command{
	Use:   "transcription",
	Short: "Manage call recording transcriptions",
	Long:  "Request and retrieve text transcriptions of call recordings. Transcription is asynchronous — request it for a recording, then fetch the result once it's ready.",
}
