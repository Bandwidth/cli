package media

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/ui"
)

var (
	uploadMediaID     string
	uploadContentType string
)

func init() {
	uploadCmd.Flags().StringVar(&uploadMediaID, "media-id", "", "Media identifier/filename on Bandwidth (defaults to local filename)")
	uploadCmd.Flags().StringVar(&uploadContentType, "content-type", "", "MIME type (auto-detected from file extension if omitted)")
	Cmd.AddCommand(uploadCmd)
}

var uploadCmd = &cobra.Command{
	Use:   "upload <file>",
	Short: "Upload a media file for MMS",
	Long:  "Uploads a local file to Bandwidth's media storage for use in MMS messages. The resulting media URL can be passed to `band message send --media`.",
	Example: `  # Upload with auto-detected content type
  band message media upload image.png

  # Upload with custom media ID
  band message media upload photo.jpg --media-id my-campaign-image.jpg`,
	Args: cobra.ExactArgs(1),
	RunE: runUpload,
}

func runUpload(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	// Determine media ID (default to filename)
	mediaID := uploadMediaID
	if mediaID == "" {
		mediaID = filepath.Base(filePath)
	}
	if err := cmdutil.ValidateID(mediaID); err != nil {
		return fmt.Errorf("invalid media ID: %w", err)
	}

	// Determine content type
	ct := uploadContentType
	if ct == "" {
		ct = mime.TypeByExtension(filepath.Ext(filePath))
		if ct == "" {
			ct = "application/octet-stream"
		}
	}

	client, acctID, err := cmdutil.MessagingClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	if err := client.PutRaw(fmt.Sprintf("/users/%s/media/%s", acctID, mediaID), data, ct); err != nil {
		return fmt.Errorf("uploading media: %w", err)
	}

	// Print the media URL that can be used with `message send --media`
	mediaURL := fmt.Sprintf("https://messaging.bandwidth.com/api/v2/users/%s/media/%s", acctID, mediaID)
	fmt.Fprintln(cmd.OutOrStdout(), mediaURL)
	ui.Successf("Use with: band message send --media %s", mediaURL)
	return nil
}
