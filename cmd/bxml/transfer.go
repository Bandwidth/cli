package bxml

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var transferCallerID string

func init() {
	transferCmd.Flags().StringVar(&transferCallerID, "caller-id", "", "Caller ID to use for the transfer")
	Cmd.AddCommand(transferCmd)
}

var transferCmd = &cobra.Command{
	Use:   "transfer <phone-number>",
	Short: "Generate a Transfer BXML verb",
	Args:  cobra.ExactArgs(1),
	RunE:  runTransfer,
}

func runTransfer(cmd *cobra.Command, args []string) error {
	phoneNumber := args[0]

	var attrs string
	if transferCallerID != "" {
		attrs = fmt.Sprintf(` transferCallerId=%q`, transferCallerID)
	}

	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	sb.WriteString("<Response>\n")
	fmt.Fprintf(&sb, "  <Transfer%s>\n", attrs)
	fmt.Fprintf(&sb, "    <PhoneNumber>%s</PhoneNumber>\n", xmlEscape(phoneNumber))
	sb.WriteString("  </Transfer>\n")
	sb.WriteString("</Response>\n")

	fmt.Fprint(cmd.OutOrStdout(), sb.String())
	return nil
}
