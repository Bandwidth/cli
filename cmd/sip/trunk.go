package sip

import "github.com/spf13/cobra"

func init() { Cmd.AddCommand(trunkCmd) }

var trunkCmd = &cobra.Command{
	Use:   "trunk",
	Short: "Opt a Build account in to SIP trunks",
	Long:  "Provision the SIP trunks of a Bandwidth Build account through the Build API.",
}
