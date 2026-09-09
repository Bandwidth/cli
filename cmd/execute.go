package cmd

import (
	"context"
	"errors"

	"github.com/Bandwidth/cli/internal/auth"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// executeCommand owns error rendering so runtime failures do not print usage.
// Track entry into RunE, because Cobra validates required flags after pre-runs.
func executeCommand(ctx context.Context, root *cobra.Command) error {
	started := false
	var restores []func()
	var wrap func(*cobra.Command)
	wrap = func(c *cobra.Command) {
		if run := c.RunE; run != nil {
			c.RunE = func(cmd *cobra.Command, args []string) error {
				started = true
				return run(cmd, args)
			}
			restores = append(restores, func() { c.RunE = run })
		}
		for _, child := range c.Commands() {
			wrap(child)
		}
	}
	wrap(root)
	silentErrors, silentUsage := root.SilenceErrors, root.SilenceUsage
	root.SilenceErrors, root.SilenceUsage = true, true
	defer func() {
		root.SilenceErrors, root.SilenceUsage = silentErrors, silentUsage
		for _, restore := range restores {
			restore()
		}
	}()
	command, err := root.ExecuteContextC(ctx)
	if err == nil {
		return nil
	}
	if command == nil {
		command = root
	}
	if !silentErrors && (command == root || !command.SilenceErrors) {
		// OAuth failures can acquire several API wrappers. Render the actionable
		// underlying error while returning the original chain for exit mapping.
		display := err
		var tokenErr *auth.TokenError
		var credentialErr *auth.CredentialError
		if errors.As(err, &tokenErr) {
			display = tokenErr
		}
		if errors.As(err, &credentialErr) {
			display = credentialErr
		}
		command.PrintErrln("Error:", display)
	}
	var flagErr *cmdutil.FlagError
	if !silentUsage && (!started || errors.As(err, &flagErr)) {
		command.PrintErr(command.UsageString())
	}
	return err
}
