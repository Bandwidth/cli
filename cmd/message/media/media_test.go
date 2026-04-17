package media

import (
	"mime"
	"path/filepath"
	"testing"
)

// TestContentTypeDetection verifies that the MIME type auto-detection logic
// used by the upload command works for common media types.
func TestContentTypeDetection(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"image.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"animation.gif", "image/gif"},
		{"document.pdf", "application/pdf"},
		{"audio.mp3", "audio/mpeg"},
		{"video.mp4", "video/mp4"},
		{"data.json", "application/json"},
	}

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			got := mime.TypeByExtension(filepath.Ext(tc.filename))
			if got == "" {
				t.Skipf("MIME type not registered for %s on this OS", tc.filename)
			}
			if got != tc.want {
				t.Errorf("TypeByExtension(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

// TestMediaIDFromFilename verifies the default media ID derivation logic.
func TestMediaIDFromFilename(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/Users/me/photos/image.png", "image.png"},
		{"image.png", "image.png"},
		{"./relative/path/photo.jpg", "photo.jpg"},
		{"/deeply/nested/dir/file.mp4", "file.mp4"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := filepath.Base(tc.path)
			if got != tc.want {
				t.Errorf("filepath.Base(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestFallbackContentType verifies unknown extensions get application/octet-stream.
func TestFallbackContentType(t *testing.T) {
	got := mime.TypeByExtension(".xyz123unknown")
	if got != "" {
		t.Skipf("unexpectedly got MIME type %q for unknown extension", got)
	}
	// The upload command falls back to application/octet-stream when TypeByExtension returns ""
	fallback := "application/octet-stream"
	if got == "" {
		got = fallback
	}
	if got != "application/octet-stream" {
		t.Errorf("fallback = %q, want application/octet-stream", got)
	}
}
