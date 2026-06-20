package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// multipartReq builds a request with a file field "image" and text fields.
func multipartReq(t *testing.T, fieldFile, filename string, content []byte, textFields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(fieldFile, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write file: %v", err)
	}
	for k, v := range textFields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/crossfit/parse", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestSaveUploadedImage(t *testing.T) {
	dir := t.TempDir()
	req := multipartReq(t, "image", "schedule.png", []byte("PNGDATA"), map[string]string{"week_start": "2026-06-22"})
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	file, hdr, err := req.FormFile("image")
	if err != nil {
		t.Fatalf("FormFile: %v", err)
	}
	defer file.Close()

	path, err := saveUploadedImage(dir, "2026-06-22", file, hdr)
	if err != nil {
		t.Fatalf("saveUploadedImage: %v", err)
	}
	if !strings.HasPrefix(path, dir) || !strings.Contains(path, "2026-06-22") {
		t.Errorf("path = %q, want under %q with week_start", path, dir)
	}
	if filepath.Ext(path) != ".png" {
		t.Errorf("ext = %q, want .png (preserved from upload)", filepath.Ext(path))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved: %v", err)
	}
	if string(got) != "PNGDATA" {
		t.Errorf("saved content = %q, want PNGDATA", got)
	}
}

func TestSaveUploadedImageCreatesDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "nested", "crossfit")
	req := multipartReq(t, "image", "x.jpg", []byte("J"), map[string]string{"week_start": "2026-06-22"})
	_ = req.ParseMultipartForm(10 << 20)
	file, hdr, _ := req.FormFile("image")
	defer file.Close()
	path, err := saveUploadedImage(base, "2026-06-22", file, hdr)
	if err != nil {
		t.Fatalf("saveUploadedImage (mkdir): %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("saved file missing: %v", err)
	}
}
