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
	GetCommitById(ctx context.Context, id string) (*registryv1.Commit, error)
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

// GetCommits returns commits for the given refs. commitIDs are fetched directly;
// moduleRefs resolve to the latest commit for each module.
func (h *Handler) GetCommits(ctx context.Context, commitIDs []string, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*registryv1.User)

	var result []*registryv1.Commit

	for _, id := range commitIDs {
		cmt, err := h.commitDB.GetCommitById(ctx, id)
		if err != nil {
			return nil, err
		}
		modules, err := h.moduleDB.GetModulesByRefs(ctx, &registryv1.ModuleRef{Id: cmt.ModuleId})
		if err != nil {
			return nil, err
		}
		if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
			return nil, err
		}
		result = append(result, cmt)
	}

	if len(moduleRefs) > 0 {
		modules, err := h.moduleDB.GetModulesByRefs(ctx, moduleRefs...)
		if err != nil {
			return nil, err
		}
		if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
			return nil, err
		}
		commits, err := h.commitDB.GetCommitByOwnerModule(ctx, moduleRefs)
		if err != nil {
			return nil, err
		}
		result = append(result, commits...)
	}

	return result, nil
}
