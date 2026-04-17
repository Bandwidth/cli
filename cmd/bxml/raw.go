package bxml

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	Cmd.AddCommand(rawCmd)
}

var rawCmd = &cobra.Command{
	Use:   "raw <xml-string>",
	Short: "Validate and pretty-print a BXML string",
	Args:  cobra.ExactArgs(1),
	RunE:  runRaw,
}

func runRaw(cmd *cobra.Command, args []string) error {
	input := args[0]

	// Validate and pretty-print by round-tripping through the XML decoder/encoder.
	decoder := xml.NewDecoder(strings.NewReader(input))

	var tokens []xml.Token
	for {
		tok, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("invalid XML: %w", err)
		}
		tokens = append(tokens, xml.CopyToken(tok))
	}

	var buf strings.Builder
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	for _, tok := range tokens {
		if err := encoder.EncodeToken(tok); err != nil {
			return fmt.Errorf("encoding XML: %w", err)
		}
	}
	if err := encoder.Flush(); err != nil {
		return fmt.Errorf("encoding XML: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), buf.String())
	return nil
}
