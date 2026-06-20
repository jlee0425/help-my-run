package api

import (
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

// allowedImageExts is the allowlist of extensions accepted for uploaded images.
var allowedImageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
}

// saveUploadedImage writes the uploaded file to <dir>/<weekStart><ext>, creating
// dir if needed, and returns the absolute saved path. The extension is taken
// from the original filename (allowlisted; default .jpg).
//
// Defense-in-depth: even though callers validate week_start, this helper rejects
// any weekStart that is not a bare filename and verifies the resolved
// destination stays inside dir before writing anything.
func saveUploadedImage(dir, weekStart string, file multipart.File, hdr *multipart.FileHeader) (string, error) {
	// weekStart must be a bare path component (no separators / traversal).
	if filepath.Base(weekStart) != weekStart {
		return "", errors.New("invalid week_start: must not contain path separators")
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if !allowedImageExts[ext] {
		ext = ".jpg"
	}

	dest := filepath.Join(dir, weekStart+ext)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absDest, absDir+string(os.PathSeparator)) {
		return "", errors.New("invalid week_start: resolved path escapes image dir")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		return "", err
	}
	return absDest, nil
}
