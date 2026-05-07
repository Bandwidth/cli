package portin

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(uploadLoaCmd)
}

var uploadLoaCmd = &cobra.Command{
	Use:     "upload-loa <order-id> <file>",
	Short:   "Upload an LOA or supporting document to a port-in order",
	Long:    `Uploads a document (LOA, recent invoice, etc.) to a port-in order. Re-runs replace any existing document of the same type.`,
	Example: `  band portin upload-loa b9ef682b-2b42-4287-bfe4-ba03ec57cb07 ./loa.pdf`,
	Args:    cobra.ExactArgs(2),
	RunE:    runUploadLoa,
}

func runUploadLoa(cmd *cobra.Command, args []string) error {
	orderID := args[0]
	filePath := args[1]

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading LOA file: %w", err)
	}

	contentType := detectContentType(filePath)

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/accounts/%s/portins/%s/loas", acctID, orderID)
	if _, err := client.PostMultipart(path, "loaFile", filepath.Base(filePath), data, contentType); err != nil {
		return portinError(err, "uploading LOA")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, map[string]interface{}{
		"orderId":     orderID,
		"file":        filepath.Base(filePath),
		"contentType": contentType,
		"status":      "UPLOADED",
	})
}

// detectContentType guesses the document content type from the file extension.
// PDFs are by far the most common LOA format; images and Word docs are also accepted.
func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}
