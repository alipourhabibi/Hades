// Package goproxy implements the Go module proxy protocol (GOPROXY) for Hades
// generated Go SDKs stored in MinIO. Mount it at "/go/" and set GOPROXY:
//
//	export DOMAIN=registry.example.com
//	GOPROXY=https://registry.example.com/go \
//	  go get registry.example.com/gen/go/alice/mymodule@latest
//
// Module paths follow the form: {DOMAIN}/gen/go/{owner}/{module}
// The DOMAIN environment variable determines the registry host used in module
// paths. If unset, the value falls back to the registryHost config field.
package goproxy

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	hserver "github.com/alipourhabibi/Hades/internal/hades/server"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	sdkjobdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/jackc/pgx/v5"
)

// Handler implements the GOPROXY protocol for generated Go SDKs.
type Handler struct {
	moduleDB     moduledb.Storage
	commitDB     commitdb.Storage
	sdkJobDB     sdkjobdb.Storage
	backend      sdkstorage.Backend
	registryHost string // resolved from DOMAIN env var or config
	logger       *log.LoggerWrapper
}

// NewHandler creates a Handler from server Dependencies.
// registryHostFallback is used when the DOMAIN environment variable is not set.
func NewHandler(deps *hserver.Dependencies, registryHostFallback string) *Handler {
	// TODO add to config
	host := strings.TrimRight(os.Getenv("DOMAIN"), "/")
	if host == "" {
		host = strings.TrimRight(registryHostFallback, "/")
	}
	return &Handler{
		moduleDB:     deps.ModuleDB,
		commitDB:     deps.CommitDB,
		sdkJobDB:     deps.SDKJobDB,
		backend:      deps.SDKStorageBackend,
		registryHost: host,
		logger:       deps.Logger,
	}
}

// ServeHTTP routes GOPROXY requests. The handler is mounted at "/go/" so
// r.URL.Path always starts with that prefix.
//
// Supported patterns (module = full Go module path):
//
//	GET /go/{module}/@v/list
//	GET /go/{module}/@v/{version}.info
//	GET /go/{module}/@v/{version}.mod
//	GET /go/{module}/@v/{version}.zip
//	GET /go/{module}/@latest
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Strip the "/go/" mount prefix.
	stripped := strings.TrimPrefix(r.URL.Path, "/go/")

	var modulePath, query string
	if idx := strings.Index(stripped, "/@v/"); idx != -1 {
		// TODO is it memory safe; i mean the index
		modulePath = stripped[:idx]
		query = stripped[idx+4:] // "list", "v0.0.0-ts-hash.zip", …
	} else if strings.HasSuffix(stripped, "/@latest") {
		modulePath = strings.TrimSuffix(stripped, "/@latest")
		query = "@latest"
	} else {
		http.NotFound(w, r)
		return
	}

	owner, modName, err := h.parseModulePath(modulePath)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch {
	case query == "list":
		h.handleList(w, r, owner, modName)
	case query == "@latest":
		h.handleLatest(w, r, owner, modName)
	case strings.HasSuffix(query, ".info"):
		h.handleInfo(w, r, strings.TrimSuffix(query, ".info"))
	case strings.HasSuffix(query, ".mod"):
		h.handleMod(w, r, modulePath, strings.TrimSuffix(query, ".mod"))
	case strings.HasSuffix(query, ".zip"):
		h.handleZip(w, r, modulePath, strings.TrimSuffix(query, ".zip"))
	default:
		http.NotFound(w, r)
	}
}

// parseModulePath extracts owner and module name from a full Go module path.
//
// Expected form: {registryHost}/gen/go/{owner}/{moduleName}
func (h *Handler) parseModulePath(modulePath string) (owner, modName string, err error) {
	prefix := h.registryHost + "/gen/go/"
	inner := strings.TrimPrefix(modulePath, prefix)
	if inner == modulePath {
		return "", "", fmt.Errorf("module path %q does not start with %q", modulePath, prefix)
	}
	parts := strings.SplitN(inner, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid module path %q: expected {host}/gen/go/{owner}/{module}", modulePath)
	}
	return parts[0], parts[1], nil
}

// toPseudoVersion converts a commit to a Go pseudo-version:
//
//	v0.0.0-YYYYMMDDHHMMSS-XXXXXXXXXXXX
func toPseudoVersion(commit *registryv1.Commit) string {
	ts := commit.CreateTime.AsTime().UTC().Format("20060102150405")
	hash := commit.CommitHash
	if len(hash) > 12 {
		hash = hash[:12]
	}
	return fmt.Sprintf("v0.0.0-%s-%s", ts, hash)
}

// hashFromVersion extracts the 12-char commit-hash prefix from a pseudo-version.
// Returns ("", false) when ver does not match the pseudo-version format.
func hashFromVersion(ver string) (string, bool) {
	parts := strings.Split(ver, "-")
	if len(parts) == 3 && strings.HasPrefix(parts[0], "v") {
		return parts[2], true
	}
	return "", false
}

// resolveCommit looks up the commit for a pseudo-version string.
// Returns (nil, non-zero status) on error.
func (h *Handler) resolveCommit(r *http.Request, ver string) (*registryv1.Commit, int) {
	hashPfx, ok := hashFromVersion(ver)
	if !ok {
		return nil, http.StatusBadRequest
	}
	commit, err := h.commitDB.GetByHashPrefix(r.Context(), hashPfx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, http.StatusNotFound
		}
		h.logger.Error("goproxy: GetByHashPrefix", "err", err)
		return nil, http.StatusInternalServerError
	}
	return commit, 0
}

// syntheticGoMod returns a minimal go.mod when the worker did not produce one
// (protoc-gen-go does not write a go.mod to the output directory).
func syntheticGoMod(modulePath string) string {
	return fmt.Sprintf("module %s\n\ngo 1.21\n\nrequire google.golang.org/protobuf v1.36.0\n", modulePath)
}

// GoImportHandler serves the ?go-get=1 discovery endpoint that the Go tool
// queries before downloading a module. Mount it at "/gen/go/" in the main mux.
//
// Go sends:  GET /gen/go/{owner}/{module}?go-get=1
// We reply with a minimal HTML page containing the go-import meta tag, telling
// Go to use this server's /go/ tree as the GOPROXY for the module.
func (h *Handler) GoImportHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("go-get") != "1" {
			http.NotFound(w, r)
			return
		}

		// Full module path = registryHost + request path
		// e.g. path="/gen/go/owner/module" → "example.com/gen/go/owner/module"
		modPath := h.registryHost + r.URL.Path

		// Determine scheme: X-Forwarded-Proto (behind reverse proxy) → TLS → http
		scheme := "http"
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else if r.TLS != nil {
			scheme = "https"
		}
		proxyURL := fmt.Sprintf("%s://%s/go", scheme, h.registryHost)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w,
			`<!DOCTYPE html><html><head><meta name="go-import" content="%s mod %s"></head><body></body></html>`,
			modPath, proxyURL)
	})
}

// handleList serves /@v/list - newline-separated pseudo-versions, newest first.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request, owner, modName string) {
	ctx := r.Context()

	mod, err := h.moduleDB.GetModuleByOwnerAndName(ctx, owner, modName)
	if err != nil || mod == nil {
		http.NotFound(w, r)
		return
	}

	jobs, err := h.sdkJobDB.ListSucceededByModuleAndLang(ctx, mod.Id, "go")
	if err != nil {
		h.logger.Error("goproxy: ListSucceededByModuleAndLang", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var versions []string
	for _, job := range jobs {
		commit, err := h.commitDB.GetCommitById(ctx, job.CommitID)
		if err != nil {
			continue
		}
		versions = append(versions, toPseudoVersion(commit))
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, strings.Join(versions, "\n"))
}

// handleInfo serves /@v/{version}.info - JSON version metadata.
func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request, ver string) {
	commit, status := h.resolveCommit(r, ver)
	if status != 0 {
		http.Error(w, http.StatusText(status), status)
		return
	}

	type infoResponse struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(infoResponse{
		Version: toPseudoVersion(commit),
		Time:    commit.CreateTime.AsTime().UTC().Format(time.RFC3339),
	})
}

// handleMod serves /@v/{version}.mod - the go.mod content.
// If the worker did not upload a go.mod, a minimal one is generated on the fly.
func (h *Handler) handleMod(w http.ResponseWriter, r *http.Request, modulePath, ver string) {
	commit, status := h.resolveCommit(r, ver)
	if status != 0 {
		http.Error(w, http.StatusText(status), status)
		return
	}

	// S3 key: "{owner/module}/{commit_hash}/go/go.mod"
	s3Key := fmt.Sprintf("%s/%s/go/go.mod", commit.Module.Name, commit.CommitHash)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rc, _, err := h.backend.GetFile(r.Context(), s3Key)
	if err == nil {
		defer rc.Close()
		_, _ = io.Copy(w, rc)
		return
	}

	fmt.Fprint(w, syntheticGoMod(modulePath))
}

// handleZip serves /@v/{version}.zip - the module zip consumed by `go get`.
// Files are streamed from MinIO and written into the zip on the fly.
// TODO is it good for memory? I mean the stream from minio or gitaly to user without extra memory creation and etc...
func (h *Handler) handleZip(w http.ResponseWriter, r *http.Request, modulePath, ver string) {
	ctx := r.Context()

	commit, status := h.resolveCommit(r, ver)
	if status != 0 {
		http.Error(w, http.StatusText(status), status)
		return
	}

	// Verify a succeeded Go SDK job exists for this commit.
	job, err := h.sdkJobDB.GetByCommitAndLang(ctx, commit.Id, "go")
	if err != nil {
		h.logger.Error("goproxy: GetByCommitAndLang", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "Go SDK not yet generated for this commit", http.StatusNotFound)
		return
	}

	// Fetch all files from MinIO at "{owner/module}/{commit_hash}/go/".
	s3Prefix := fmt.Sprintf("%s/%s/go", commit.Module.Name, commit.CommitHash)
	files, err := h.backend.Download(ctx, s3Prefix)
	if err != nil {
		h.logger.Error("goproxy: Download from S3", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(files) == 0 {
		http.Error(w, "no Go SDK files found in storage", http.StatusNotFound)
		return
	}

	// Each file path inside the zip must be prefixed with "{module}@{version}/".
	dirPrefix := modulePath + "@" + ver + "/"

	hasGoMod := false
	for _, f := range files {
		if f.Path == "go.mod" {
			hasGoMod = true
			break
		}
	}

	w.Header().Set("Content-Type", "application/zip")
	zw := zip.NewWriter(w)

	// Inject a synthetic go.mod when the worker did not produce one.
	if !hasGoMod {
		if f, err := zw.Create(dirPrefix + "go.mod"); err == nil {
			_, _ = fmt.Fprint(f, syntheticGoMod(modulePath))
		}
	}

	for _, file := range files {
		f, err := zw.Create(dirPrefix + file.Path)
		if err != nil {
			h.logger.Error("goproxy: zip Create", "path", file.Path, "err", err)
			continue
		}
		_, _ = f.Write(file.Content)
	}

	_ = zw.Close()
}

// handleLatest serves /@latest - version info for the newest Go SDK.
func (h *Handler) handleLatest(w http.ResponseWriter, r *http.Request, owner, modName string) {
	ctx := r.Context()

	mod, err := h.moduleDB.GetModuleByOwnerAndName(ctx, owner, modName)
	if err != nil || mod == nil {
		http.NotFound(w, r)
		return
	}

	jobs, err := h.sdkJobDB.ListSucceededByModuleAndLang(ctx, mod.Id, "go")
	if err != nil || len(jobs) == 0 {
		http.NotFound(w, r)
		return
	}

	commit, err := h.commitDB.GetCommitById(ctx, jobs[0].CommitID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	type infoResponse struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(infoResponse{
		Version: toPseudoVersion(commit),
		Time:    commit.CreateTime.AsTime().UTC().Format(time.RFC3339),
	})
}
