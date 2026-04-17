package bxml

import (
	"bytes"
	"encoding/xml"
	"fmt"

	"github.com/spf13/cobra"
)

var speakVoice string

func init() {
	speakCmd.Flags().StringVar(&speakVoice, "voice", "", "Voice to use for speech (e.g. Susan)")
	Cmd.AddCommand(speakCmd)
}

var speakCmd = &cobra.Command{
	Use:   "speak <text>",
	Short: "Generate a SpeakSentence BXML verb",
	Example: `  band bxml speak "Hello, welcome to Bandwidth."
  band bxml speak --voice julie "Press 1 for sales."
  band bxml speak "Goodbye." > hangup.xml`,
	Args: cobra.ExactArgs(1),
	RunE: runSpeak,
}

func runSpeak(cmd *cobra.Command, args []string) error {
	text := xmlEscape(args[0])

	var inner string
	if speakVoice != "" {
		inner = fmt.Sprintf(`  <SpeakSentence voice=%q>%s</SpeakSentence>`, speakVoice, text)
	} else {
		inner = fmt.Sprintf("  <SpeakSentence>%s</SpeakSentence>", text)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Response>\n%s\n</Response>\n", inner)
	return nil
}

// xmlEscape escapes special XML characters in s so it is safe to embed in XML
// element content.
func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
