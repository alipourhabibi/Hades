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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	hserver "github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	sdkjobdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/utils/log"
)

// authorizer resolves credentials and enforces module read access. It is
// satisfied by *authorization.Server; the interface keeps this package
// testable and free of an import cycle.
type authorizer interface {
	// UserFromToken validates a session token or PAT and returns its user plus
	// the scopes it carries (empty means unrestricted).
	UserFromToken(ctx context.Context, rawToken string) (*identityv1.User, []string, error)
	// CheckReadAccess returns an error for the first module the caller may not
	// read. user may be nil for anonymous callers.
	CheckReadAccess(ctx context.Context, user *identityv1.User, modules []*registryv1.Module) error
}

// Handler implements the GOPROXY protocol for generated Go SDKs.
type Handler struct {
	moduleDB     moduledb.Storage
	commitDB     commitdb.Storage
	sdkJobDB     sdkjobdb.Storage
	backend      sdkstorage.Backend
	authz        authorizer
	registryHost string // resolved from DOMAIN env var or config
	logger       *log.LoggerWrapper
}

// NewHandler creates a Handler from server Dependencies.
// registryHostFallback is used when the DOMAIN environment variable is not set.
func NewHandler(deps *hserver.Dependencies, registryHostFallback string) *Handler {
	// DOMAIN env var overrides the config-supplied fallback so the same binary
	// works in all environments without recompilation.
	host := strings.TrimRight(os.Getenv("DOMAIN"), "/")
	if host == "" {
		host = strings.TrimRight(registryHostFallback, "/")
	}
	return &Handler{
		moduleDB:     deps.ModuleDB,
		commitDB:     deps.CommitDB,
		sdkJobDB:     deps.SDKJobDB,
		backend:      deps.SDKStorageBackend,
		authz:        deps.Authorization,
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

	// This handler is mounted outside the Connect interceptor chain, so it
	// authenticates and authorises the request itself. Everything below serves
	// module content, so no route may run before this gate. The authorised
	// module is threaded through so version lookups can be pinned to it.
	mod, status := h.authorize(r, owner, modName)
	if status != 0 {
		http.Error(w, http.StatusText(status), status)
		return
	}

	switch {
	case query == "list":
		h.handleList(w, r, mod)
	case query == "@latest":
		h.handleLatest(w, r, mod)
	case strings.HasSuffix(query, ".info"):
		h.handleInfo(w, r, mod, strings.TrimSuffix(query, ".info"))
	case strings.HasSuffix(query, ".mod"):
		h.handleMod(w, r, mod, modulePath, strings.TrimSuffix(query, ".mod"))
	case strings.HasSuffix(query, ".zip"):
		h.handleZip(w, r, mod, modulePath, strings.TrimSuffix(query, ".zip"))
	default:
		http.NotFound(w, r)
	}
}

// credentialFromRequest extracts a Hades credential from the request.
//
// The Go toolchain sends credentials from ~/.netrc as HTTP Basic auth, so a
// PAT arrives as the password (the username is ignored, matching how the buf
// CLI and GitHub treat netrc entries). Bearer is also accepted for direct
// curl-style use. Returns "" when the request is anonymous.
func credentialFromRequest(r *http.Request) string {
	if _, password, ok := r.BasicAuth(); ok && password != "" {
		return strings.TrimSpace(password)
	}
	authHeader := r.Header.Get("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// authorize resolves the caller and checks read access to the requested
// module. It returns the module and 0 when the request may proceed, otherwise
// nil and the HTTP status to send.
//
// A missing or private module and an unauthorised caller all produce 404, so
// the endpoint never reveals that a module the caller cannot read exists.
func (h *Handler) authorize(r *http.Request, owner, modName string) (*registryv1.Module, int) {
	ctx := r.Context()

	// Fail closed: without an authorizer nothing can be checked, so nothing is
	// served.
	if h.authz == nil {
		h.logger.Error("goproxy: no authorizer configured, refusing request")
		return nil, http.StatusServiceUnavailable
	}

	var user *identityv1.User
	if cred := credentialFromRequest(r); cred != "" {
		u, scopes, err := h.authz.UserFromToken(ctx, cred)
		if err != nil {
			h.logger.Debug("goproxy: credential rejected", "err", err)
			return nil, http.StatusUnauthorized
		}
		// A scoped PAT must carry module:read to fetch module content.
		if !authorization.ScopesAllow(scopes, string(constants.ResourceModule), string(constants.ActionRead)) {
			return nil, http.StatusForbidden
		}
		user = u
	}

	mod, err := h.moduleDB.GetModuleByOwnerAndName(ctx, owner, modName)
	if err != nil || mod == nil {
		return nil, http.StatusNotFound
	}
	if err := h.authz.CheckReadAccess(ctx, user, []*registryv1.Module{mod}); err != nil {
		return nil, http.StatusNotFound
	}
	return mod, 0
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

// resolveCommit looks up the commit for a pseudo-version string and pins it to
// mod, the module the caller was authorised for. Without that check a caller
// could ask a module they can read for a commit hash belonging to a module they
// cannot, since hashes are resolved globally.
// Returns (nil, non-zero status) on error.
func (h *Handler) resolveCommit(r *http.Request, mod *registryv1.Module, ver string) (*registryv1.Commit, int) {
	hashPfx, ok := hashFromVersion(ver)
	if !ok {
		return nil, http.StatusBadRequest
	}
	commit, err := h.commitDB.GetByHashPrefix(r.Context(), hashPfx)
	if err != nil {
		var ce *connect.Error
		if errors.As(err, &ce) && ce.Code() == connect.CodeNotFound {
			return nil, http.StatusNotFound
		}
		h.logger.Error("goproxy: GetByHashPrefix", "err", err)
		return nil, http.StatusInternalServerError
	}
	if commit.ModuleId != mod.Id {
		return nil, http.StatusNotFound
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
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request, mod *registryv1.Module) {
	ctx := r.Context()

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
func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request, mod *registryv1.Module, ver string) {
	commit, status := h.resolveCommit(r, mod, ver)
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
func (h *Handler) handleMod(w http.ResponseWriter, r *http.Request, mod *registryv1.Module, modulePath, ver string) {
	commit, status := h.resolveCommit(r, mod, ver)
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
// Files are loaded from MinIO and written into the zip on the fly.
// Large modules hold the full file set in memory; a streaming approach would
// require the backend to expose a per-file reader instead of []byte slices.
func (h *Handler) handleZip(w http.ResponseWriter, r *http.Request, mod *registryv1.Module, modulePath, ver string) {
	ctx := r.Context()

	commit, status := h.resolveCommit(r, mod, ver)
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
func (h *Handler) handleLatest(w http.ResponseWriter, r *http.Request, mod *registryv1.Module) {
	ctx := r.Context()

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
