package s3

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs/mime"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
)

// fakeMimeDetector records the name it was asked about so the test can tell whether
// the content type was inferred from the file name.
type fakeMimeDetector struct {
	asked []string
}

func (d *fakeMimeDetector) TypeByName(p string) string {
	d.asked = append(d.asked, p)
	if strings.HasSuffix(p, ".zip") {
		return "application/zip"
	}
	return "application/octet-stream"
}

func newTestDriver(t *testing.T, mimeDetector mime.MimeDetector) *Driver {
	t.Helper()

	policy := &ent.StoragePolicy{
		Name:       "s3-test",
		Type:       types.PolicyTypeS3,
		Server:     "127.0.0.1:1",
		BucketName: "bucket",
		AccessKey:  "key",
		SecretKey:  "secret",
		Settings:   &types.PolicySetting{ChunkSize: 5 << 20},
	}

	d, err := New(context.Background(), policy, nil, nil, logging.NewConsoleLogger(logging.LevelError), mimeDetector)
	if err != nil {
		t.Fatalf("failed to build the driver: %s", err)
	}

	return d
}

// A relocation builds an UploadRequest itself, and an S3 policy infers the content type
// from the request URI when none is set. Dereferencing a nil URI there panicked the
// whole task with "invalid memory address or nil pointer dereference", so the inference
// must stay optional.
func TestPutWithoutUriDoesNotPanic(t *testing.T) {
	detector := &fakeMimeDetector{}
	d := newTestDriver(t, detector)

	req := &fs.UploadRequest{
		Props: &fs.UploadProps{
			// Uri deliberately left nil.
			SavePath: "uploads/1/file.zip",
			Size:     4,
		},
		Mode: fs.ModeOverwrite,
		File: &stubReader{},
	}

	// The upload itself cannot succeed against a dead endpoint; only the panic matters.
	_ = d.Put(context.Background(), req)

	if len(detector.asked) != 0 {
		t.Fatalf("the detector must not be consulted without a URI, got %v", detector.asked)
	}
}

// With a URI present the content type is still inferred from the file name.
func TestPutInfersMimeTypeFromUri(t *testing.T) {
	detector := &fakeMimeDetector{}
	d := newTestDriver(t, detector)

	uri, err := fs.NewUriFromString("cloudreve://my/folder/archive.zip")
	if err != nil {
		t.Fatalf("failed to build a uri: %s", err)
	}

	req := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:      uri,
			SavePath: "uploads/1/file.zip",
			Size:     4,
		},
		Mode: fs.ModeOverwrite,
		File: &stubReader{},
	}

	_ = d.Put(context.Background(), req)

	if len(detector.asked) != 1 || detector.asked[0] != "archive.zip" {
		t.Fatalf("expected the name to be inferred from the uri, got %v", detector.asked)
	}
}

// An explicit MimeType wins and the detector is left alone.
func TestPutPrefersExplicitMimeType(t *testing.T) {
	detector := &fakeMimeDetector{}
	d := newTestDriver(t, detector)

	req := &fs.UploadRequest{
		Props: &fs.UploadProps{
			SavePath: "uploads/1/file.bin",
			Size:     4,
			MimeType: "application/x-custom",
		},
		Mode: fs.ModeOverwrite,
		File: &stubReader{},
	}

	_ = d.Put(context.Background(), req)

	if len(detector.asked) != 0 {
		t.Fatalf("an explicit mime type must skip detection, got %v", detector.asked)
	}
}

// stubReader is a minimal driver.Handler upload body.
type stubReader struct {
	read bool
}

func (s *stubReader) Read(p []byte) (int, error) {
	if s.read {
		return 0, errEOF
	}
	s.read = true
	copy(p, "test")
	return 4, nil
}

func (s *stubReader) Close() error { return nil }

var errEOF = &eofError{}

type eofError struct{}

func (e *eofError) Error() string { return "EOF" }

var _ driver.Handler = (*Driver)(nil)
