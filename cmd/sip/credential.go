package sip

import "github.com/spf13/cobra"

func init() { Cmd.AddCommand(credentialCmd) }

var credentialCmd = &cobra.Command{
	Use:     "credential",
	Aliases: []string{"cred"},
	Short:   "Manage SIP digest credentials",
	Long:    "Manage the SIP digest credentials a peer uses to authenticate to a Bandwidth realm. Bandwidth stores only MD5 hashes; the CLI computes them, so passwords are never sent to the API.",
}

// credentialFlags are shared by create and rotate.
type credentialFlags struct {
	realm    string
	stdin    bool
	file     string
	generate bool
}

func addPasswordFlags(cmd *cobra.Command, f *credentialFlags) {
	cmd.Flags().BoolVar(&f.stdin, "password-stdin", false, "Read the password from stdin")
	cmd.Flags().StringVar(&f.file, "password-file", "", "Read the password from a file")
	cmd.Flags().BoolVar(&f.generate, "generate-password", false, "Generate a random password and print it once")
}
