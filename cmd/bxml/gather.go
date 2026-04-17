package bxml

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	gatherURL       string
	gatherMaxDigits string
	gatherPrompt    string
)

func init() {
	gatherCmd.Flags().StringVar(&gatherURL, "url", "", "URL to send gathered digits to (required)")
	gatherCmd.Flags().StringVar(&gatherMaxDigits, "max-digits", "", "Maximum number of digits to gather")
	gatherCmd.Flags().StringVar(&gatherPrompt, "prompt", "", "Prompt to speak before gathering input")
	_ = gatherCmd.MarkFlagRequired("url")
	Cmd.AddCommand(gatherCmd)
}

var gatherCmd = &cobra.Command{
	Use:   "gather",
	Short: "Generate a Gather BXML verb",
	Args:  cobra.NoArgs,
	RunE:  runGather,
}

func runGather(cmd *cobra.Command, args []string) error {
	attrs := fmt.Sprintf(`gatherUrl=%q`, gatherURL)
	if gatherMaxDigits != "" {
		attrs += fmt.Sprintf(` maxDigits=%q`, gatherMaxDigits)
	}

	var inner string
	if gatherPrompt != "" {
		inner = fmt.Sprintf("\n    <SpeakSentence>%s</SpeakSentence>\n  ", xmlEscape(gatherPrompt))
	}

	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	sb.WriteString("<Response>\n")
	fmt.Fprintf(&sb, "  <Gather %s>%s</Gather>\n", attrs, inner)
	sb.WriteString("</Response>\n")

	fmt.Fprint(cmd.OutOrStdout(), sb.String())
	return nil
}
