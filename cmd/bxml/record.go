package bxml

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	recordURL         string
	recordMaxDuration string
)

func init() {
	recordCmd.Flags().StringVar(&recordURL, "url", "", "URL to send recording completion event to")
	recordCmd.Flags().StringVar(&recordMaxDuration, "max-duration", "", "Maximum duration of the recording in seconds")
	Cmd.AddCommand(recordCmd)
}

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Generate a Record BXML verb",
	Args:  cobra.NoArgs,
	RunE:  runRecord,
}

func runRecord(cmd *cobra.Command, args []string) error {
	var attrParts []string
	if recordURL != "" {
		attrParts = append(attrParts, fmt.Sprintf(`recordCompleteUrl=%q`, recordURL))
	}
	if recordMaxDuration != "" {
		attrParts = append(attrParts, fmt.Sprintf(`maxDuration=%q`, recordMaxDuration))
	}

	var element string
	if len(attrParts) > 0 {
		element = fmt.Sprintf("  <Record %s/>", strings.Join(attrParts, " "))
	} else {
		element = "  <Record/>"
	}

	fmt.Fprintf(cmd.OutOrStdout(), "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Response>\n%s\n</Response>\n", element)
	return nil
}
