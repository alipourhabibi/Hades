// Package commits provides commit query logic using internal proto types.
// The buf adapter (bufcommits) wraps this to expose the buf.build wire
// protocol; the internal registry API uses it directly.
package commits

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
)

// moduleQuerier is the subset of ModuleStorage used by the Handler.
type moduleQuerier interface {
	GetModulesByRefs(ctx context.Context, refs ...*registryv1.ModuleRef) ([]*registryv1.Module, error)
}

// commitQuerier is the subset of CommitStorage used by the Handler.
type commitQuerier interface {
	GetCommitByOwnerModule(ctx context.Context, refs []*registryv1.ModuleRef) ([]*registryv1.Commit, error)
}

// readAccessChecker is the subset of the authorization Server used by the Handler.
type readAccessChecker interface {
	CheckReadAccess(ctx context.Context, user *registryv1.User, modules []*registryv1.Module) error
}

// Handler provides commit queries using own proto types.
// Buf adapters (bufcommits, bufgraph) wrap this to expose the buf.build wire protocol.
type Handler struct {
	moduleDB moduleQuerier
	commitDB commitQuerier
	authz    readAccessChecker
}

func New(deps *server.Dependencies) *Handler {
	return &Handler{
		moduleDB: deps.ModuleDB,
		commitDB: deps.CommitDB,
		authz:    deps.Authorization,
	}
}

// GetCommits returns the latest commits for the given module refs after checking
// read access. user may be nil (anonymous); public modules are served without
// auth, private ones return NotFound.
func (h *Handler) GetCommits(ctx context.Context, refs []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*registryv1.User) // nil for anonymous
	modules, err := h.moduleDB.GetModulesByRefs(ctx, refs...)
	if err != nil {
		return nil, err
	}
	if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
		return nil, err
	}
	return h.commitDB.GetCommitByOwnerModule(ctx, refs)
}
