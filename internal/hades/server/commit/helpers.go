package commit

import (
	"context"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
)

// GetCommits returns commits for the given refs. Used by the buf/commits adapter.
func (h *Handler) GetCommits(ctx context.Context, commitIDs []string, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	var result []*registryv1.Commit

	for _, id := range commitIDs {
		cmt, err := h.commitDBStorage.GetCommitById(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := h.authz.CheckReadAccess(ctx, user, []*registryv1.Module{cmt.Module}); err != nil {
			return nil, err
		}
		result = append(result, cmt)
	}

	if len(moduleRefs) > 0 {
		modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, moduleRefs...)
		if err != nil {
			return nil, err
		}
		if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
			return nil, err
		}
		commits, err := h.commitDBStorage.GetCommitByOwnerModule(ctx, moduleRefs)
		if err != nil {
			return nil, err
		}
		result = append(result, commits...)
	}

	return result, nil
}

// GetGraph returns commits forming the dependency graph for the given refs. Used by the buf/graph adapter.
func (h *Handler) GetGraph(ctx context.Context, commitIDs []string, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	var result []*registryv1.Commit

	for _, id := range commitIDs {
		cmt, err := h.commitDBStorage.GetCommitById(ctx, id)
		if err != nil {
			return nil, err
		}
		modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, &registryv1.ModuleRef{Id: cmt.ModuleId})
		if err != nil {
			return nil, err
		}
		if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
			return nil, err
		}
		result = append(result, cmt)
	}

	if len(moduleRefs) > 0 {
		modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, moduleRefs...)
		if err != nil {
			return nil, err
		}
		if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
			return nil, err
		}
		commits, err := h.commitDBStorage.GetCommitByOwnerModule(ctx, moduleRefs)
		if err != nil {
			return nil, err
		}
		result = append(result, commits...)
	}

	return result, nil
}
