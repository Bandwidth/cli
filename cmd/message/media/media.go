package media

import "github.com/spf13/cobra"

// Cmd is the `band message media` parent command.
var Cmd = &cobra.Command{
	Use:   "media",
	Short: "Manage MMS media files",
	Long:  "Upload, list, download, and delete the media files served for MMS. Uploaded media is hosted by Bandwidth and referenced by URL when sending an MMS with 'message send --media'.",
}
