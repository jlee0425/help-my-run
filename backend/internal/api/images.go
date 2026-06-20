package api

import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

// saveUploadedImage writes the uploaded file to <dir>/<weekStart><ext>, creating
// dir if needed, and returns the absolute saved path. The extension is taken
// from the original filename (default .jpg).
func saveUploadedImage(dir, weekStart string, file multipart.File, hdr *multipart.FileHeader) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if ext == "" {
		ext = ".jpg"
	}
	dest := filepath.Join(dir, weekStart+ext)
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return dest, nil
	}
	return abs, nil
}
