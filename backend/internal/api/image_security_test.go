package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/store"
)

// newTestServerWithImageDir is like newTestServer but lets the caller inspect
// the image dir so traversal tests can assert nothing escaped it.
func newTestServerWithImageDir(t *testing.T, imageDir string) (http.Handler, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	deps := Deps{
		Store:    s,
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string) {
			return "ok", 0, nil
		},
		Coach:    &fakeCoach{},
		ImageDir: imageDir,
	}
	return NewRouter(deps), s
}

// TestCrossfitParseRejectsTraversalWeekStart asserts a traversal week_start is
// rejected at the handler boundary with 400 and that NO file is written outside
// the configured image dir.
func TestCrossfitParseRejectsTraversalWeekStart(t *testing.T) {
	root := t.TempDir()
	imageDir := filepath.Join(root, "images")
	// A sibling dir that an attacker would try to escape into.
	target := filepath.Join(root, "etc")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	h, _ := newTestServerWithImageDir(t, imageDir)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("image", "schedule.png")
	_, _ = fw.Write([]byte("PNG"))
	_ = mw.WriteField("week_start", "../etc/passwd")
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/crossfit/parse", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	// Nothing must have been written into the sibling target dir.
	if _, err := os.Stat(filepath.Join(target, "passwd")); !os.IsNotExist(err) {
		t.Errorf("file escaped image dir into %s (err=%v)", target, err)
	}
	// And nothing must have been written into the image dir under that name.
	if _, err := os.Stat(filepath.Join(imageDir, "passwd")); !os.IsNotExist(err) {
		t.Errorf("unexpected file written into image dir (err=%v)", err)
	}
}

// TestSaveUploadedImageRejectsTraversal is a direct unit test on the hardened
// helper: it must error for a traversal week_start and succeed for a valid one.
func TestSaveUploadedImageRejectsTraversal(t *testing.T) {
	dir := t.TempDir()

	// Traversal: must error and write nothing outside dir.
	file := &fakeMultipartFile{r: bytes.NewReader([]byte("PNG"))}
	hdr := &multipart.FileHeader{Filename: "x.png"}
	if _, err := saveUploadedImage(dir, "../../x", file, hdr); err == nil {
		t.Fatalf("saveUploadedImage(traversal) = nil error, want error")
	}

	// Valid ISO week_start: must succeed and write inside dir.
	file2 := &fakeMultipartFile{r: bytes.NewReader([]byte("PNG"))}
	hdr2 := &multipart.FileHeader{Filename: "schedule.png"}
	got, err := saveUploadedImage(dir, "2026-06-22", file2, hdr2)
	if err != nil {
		t.Fatalf("saveUploadedImage(valid) error = %v", err)
	}
	absDir, _ := filepath.Abs(dir)
	if !bytes.HasPrefix([]byte(got), []byte(absDir+string(os.PathSeparator))) {
		t.Errorf("saved path %q not inside dir %q", got, absDir)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-06-22.png")); err != nil {
		t.Errorf("expected file 2026-06-22.png inside dir: %v", err)
	}
}

// fakeMultipartFile adapts a bytes.Reader to multipart.File for unit tests.
type fakeMultipartFile struct {
	r *bytes.Reader
}

func (f *fakeMultipartFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *fakeMultipartFile) Close() error               { return nil }
func (f *fakeMultipartFile) Seek(o int64, w int) (int64, error) {
	return f.r.Seek(o, w)
}
func (f *fakeMultipartFile) ReadAt(p []byte, o int64) (int, error) {
	return f.r.ReadAt(p, o)
}
