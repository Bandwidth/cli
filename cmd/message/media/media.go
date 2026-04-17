package media

import "github.com/spf13/cobra"

// Cmd is the `band message media` parent command.
var Cmd = &cobra.Command{
	Use:   "media",
	Short: "Manage MMS media files",
}
