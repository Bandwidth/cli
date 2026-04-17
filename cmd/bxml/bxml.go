// Package bxml provides local commands for generating Bandwidth XML (BXML).
// No API calls are made — output is printed directly to stdout.
package bxml

import "github.com/spf13/cobra"

// Cmd is the `band bxml` parent command.
var Cmd = &cobra.Command{
	Use:   "bxml",
	Short: "Generate Bandwidth XML (BXML) snippets",
	Long:  "Generate BXML verb snippets locally. No API calls are made.",
}
