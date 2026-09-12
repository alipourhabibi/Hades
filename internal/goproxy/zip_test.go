package goproxy

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
)

// fakeBackend records how content was retrieved so the test can assert that the
// zip is built by streaming rather than by materialising every file.
type fakeBackend struct {
	files map[string][]byte

	downloadCalls int
	getFileCalls  int
	maxOpen       int
	open          int
}

func (f *fakeBackend) Upload(context.Context, string, string) (string, error) { return "", nil }

func (f *fakeBackend) Download(_ context.Context, prefix string) ([]*sdkstorage.File, error) {
	f.downloadCalls++
	var out []*sdkstorage.File
	for k, v := range f.files {
		if rel, ok := strings.CutPrefix(k, prefix+"/"); ok {
			out = append(out, &sdkstorage.File{Path: rel, Content: v})
		}
	}
	return out, nil
}

func (f *fakeBackend) ListFiles(_ context.Context, prefix string) ([]string, error) {
	var paths []string
	for k := range f.files {
		if rel, ok := strings.CutPrefix(k, prefix+"/"); ok {
			paths = append(paths, rel)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func (f *fakeBackend) GetFile(_ context.Context, key string) (io.ReadCloser, int64, error) {
	content, ok := f.files[key]
	if !ok {
		return nil, 0, fmt.Errorf("not found: %s", key)
	}
	f.getFileCalls++
	f.open++
	if f.open > f.maxOpen {
		f.maxOpen = f.open
	}
	return &trackedReader{Reader: bytes.NewReader(content), backend: f}, int64(len(content)), nil
}

type trackedReader struct {
	*bytes.Reader
	backend *fakeBackend
	closed  bool
}

func (t *trackedReader) Close() error {
	if !t.closed {
		t.closed = true
		t.backend.open--
	}
	return nil
}

// stubSDKJobs implements sdkjob.Storage; only GetByCommitAndLang is exercised.
type stubSDKJobs struct {
	job *sdkjob.SDKJob
}

func (s *stubSDKJobs) CreateBatch(context.Context, string, string, []config.GeneratorConfig) error {
	return nil
}
func (s *stubSDKJobs) ClaimPending(context.Context, int) ([]*sdkjob.SDKJob, error) { return nil, nil }
func (s *stubSDKJobs) MarkSucceeded(context.Context, string, string) error         { return nil }
func (s *stubSDKJobs) MarkFailed(context.Context, string, string, int) error       { return nil }
func (s *stubSDKJobs) ListByModule(context.Context, string) ([]*sdkjob.SDKJob, error) {
	return nil, nil
}
func (s *stubSDKJobs) ListSucceededByModuleAndLang(context.Context, string, string) ([]*sdkjob.SDKJob, error) {
	return nil, nil
}
func (s *stubSDKJobs) GetByCommitAndLang(context.Context, string, string) (*sdkjob.SDKJob, error) {
	return s.job, nil
}
func (s *stubSDKJobs) RecoverStaleJobs(context.Context, time.Duration) (int64, error) {
	return 0, nil
}

func newZipHandler(t *testing.T, backend *fakeBackend, commit *registryv1.Commit) *Handler {
	t.Helper()
	h := newTestHandler(t, &fakeAuthorizer{}, nil)
	h.backend = backend
	h.commitDB = &fakeCommitDB{commit: commit}
	return h
}

func TestHandleZip_StreamsOneFileAtATime(t *testing.T) {
	backend := &fakeBackend{files: map[string][]byte{
		"alice/mymod/abcdef123456/go/a.pb.go":     []byte("package a"),
		"alice/mymod/abcdef123456/go/b.pb.go":     []byte("package b"),
		"alice/mymod/abcdef123456/go/sub/c.pb.go": []byte("package c"),
		"alice/mymod/abcdef123456/go/go.mod":      []byte("module example.com/x"),
	}}

	commit := &registryv1.Commit{
		Id:         "c-1",
		ModuleId:   "mod-1",
		CommitHash: "abcdef123456",
		CreateTime: timestamppb.Now(),
		Module:     &registryv1.Module{Id: "mod-1", Name: "alice/mymod"},
	}
	h := newZipHandler(t, backend, commit)
	h.sdkJobDB = &stubSDKJobs{job: &sdkjob.SDKJob{ID: "job-1"}}

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/go/x/@v/v0.0.0-20240101000000-abcdef123456.zip", nil)
	h.handleZip(rec, r, &registryv1.Module{Id: "mod-1", Name: "alice/mymod"},
		"example.com/gen/go/alice/mymod", "v0.0.0-20240101000000-abcdef123456")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Never buffers the whole artifact set.
	assert.Zero(t, backend.downloadCalls, "handleZip must not call Download")
	assert.Equal(t, 4, backend.getFileCalls)
	assert.Equal(t, 1, backend.maxOpen, "only one object may be open at a time")
	assert.Zero(t, backend.open, "every reader must be closed")

	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	require.NoError(t, err)

	got := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		_ = rc.Close()
		got[f.Name] = string(content)
	}

	prefix := "example.com/gen/go/alice/mymod@v0.0.0-20240101000000-abcdef123456/"
	assert.Equal(t, "package a", got[prefix+"a.pb.go"])
	assert.Equal(t, "package c", got[prefix+"sub/c.pb.go"])
	assert.Equal(t, "module example.com/x", got[prefix+"go.mod"])
	assert.Len(t, got, 4)
}

func TestHandleZip_InjectsSyntheticGoMod(t *testing.T) {
	backend := &fakeBackend{files: map[string][]byte{
		"alice/mymod/abcdef123456/go/a.pb.go": []byte("package a"),
	}}
	commit := &registryv1.Commit{
		Id: "c-1", ModuleId: "mod-1", CommitHash: "abcdef123456",
		CreateTime: timestamppb.Now(),
		Module:     &registryv1.Module{Id: "mod-1", Name: "alice/mymod"},
	}
	h := newZipHandler(t, backend, commit)
	h.sdkJobDB = &stubSDKJobs{job: &sdkjob.SDKJob{ID: "job-1"}}

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/go/x/@v/v0.0.0-20240101000000-abcdef123456.zip", nil)
	h.handleZip(rec, r, &registryv1.Module{Id: "mod-1", Name: "alice/mymod"},
		"example.com/gen/go/alice/mymod", "v0.0.0-20240101000000-abcdef123456")

	require.Equal(t, http.StatusOK, rec.Code)
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	require.NoError(t, err)

	var goModContent string
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/go.mod") {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			_ = rc.Close()
			goModContent = string(b)
		}
	}
	assert.Contains(t, goModContent, "module example.com/gen/go/alice/mymod")
}

func TestHandleZip_ServesNotModifiedForMatchingETag(t *testing.T) {
	backend := &fakeBackend{files: map[string][]byte{
		"alice/mymod/abcdef123456/go/a.pb.go": []byte("package a"),
	}}
	commit := &registryv1.Commit{
		Id: "c-1", ModuleId: "mod-1", CommitHash: "abcdef123456",
		CreateTime: timestamppb.Now(),
		Module:     &registryv1.Module{Id: "mod-1", Name: "alice/mymod"},
	}
	h := newZipHandler(t, backend, commit)
	h.sdkJobDB = &stubSDKJobs{job: &sdkjob.SDKJob{ID: "job-1"}}

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/go/x/@v/v0.0.0-20240101000000-abcdef123456.zip", nil)
	r.Header.Set("If-None-Match", `"abcdef123456-go"`)
	h.handleZip(rec, r, &registryv1.Module{Id: "mod-1", Name: "alice/mymod"},
		"example.com/gen/go/alice/mymod", "v0.0.0-20240101000000-abcdef123456")

	assert.Equal(t, http.StatusNotModified, rec.Code)
	assert.Zero(t, backend.getFileCalls, "a cache hit must not read any artifact")
}
