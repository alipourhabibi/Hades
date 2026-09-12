package authorization

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
)

const bindingCacheKeyPrefix = "opa:bindings:"

// hybridStore implements storage.Store.
// Policy (Rego) paths are handled by the embedded inmem delegate.
// Data reads for path /role_bindings/{subject} are served from cache (with DB
// fallback on miss). Writes to /role_bindings/{subject} only invalidate the
// per-subject cache entry; the authoritative store is always the database.
//
// Multi-pod note: in-memory cache is per-process. Bindings written on one pod
// are not visible to others until their cache TTL expires. Use Redis
// (backends.cache: redis) for cross-pod consistency. SQLite is single-process
// by design and pairs naturally with the in-memory cache.
type hybridStore struct {
	inner storage.Store // inmem, holds Rego policy only
	cache cache.Cache
	db    hybridBindingDB
	ttl   time.Duration
	// logger is optional; when set, cache outages are reported rather than
	// silently degrading into a database read on every request.
	logger *log.LoggerWrapper
}

// hybridBindingDB is the subset of opabinding.Storage used by hybridStore.
type hybridBindingDB interface {
	ListBySubject(ctx context.Context, subject string) ([]opabinding.RoleBinding, error)
}

// negativeBindingCacheTTL bounds how long "this subject has no bindings" is
// remembered.
//
// Caching an empty result for the full TTL means a read that races the write
// granting a role pins the negative answer, and the new role does not take
// effect for up to the TTL even on the pod that granted it. A short window
// still absorbs the repeated lookups an unauthorised caller generates without
// making a fresh grant look like it did not happen.
const negativeBindingCacheTTL = 2 * time.Second

func newHybridStore(c cache.Cache, db hybridBindingDB, ttl time.Duration, superAdmins []string) (*hybridStore, error) {
	inner := inmem.New()

	// Seed inner with an empty role_bindings map so OPA policy compilation
	// sees a valid (though empty) data document and does not fail type-checks,
	// and with the superadmin set the policy's bypass clause reads.
	ctx := context.Background()
	txn, err := inner.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		return nil, fmt.Errorf("hybridStore: seed txn: %w", err)
	}
	admins := make([]any, 0, len(superAdmins))
	for _, s := range superAdmins {
		if s != "" {
			admins = append(admins, s)
		}
	}
	seed := map[string]any{
		"role_bindings": map[string]any{},
		"superadmins":   admins,
	}
	if err := inner.Write(ctx, txn, storage.AddOp, storage.MustParsePath("/"), seed); err != nil {
		inner.Abort(ctx, txn)
		return nil, fmt.Errorf("hybridStore: seed write: %w", err)
	}
	if err := inner.Commit(ctx, txn); err != nil {
		return nil, fmt.Errorf("hybridStore: seed commit: %w", err)
	}

	return &hybridStore{
		inner: inner,
		cache: c,
		db:    db,
		ttl:   ttl,
	}, nil
}

func bindingCacheKey(subject string) string {
	return bindingCacheKeyPrefix + subject
}

// opaSubjectBinding is the per-subject binding shape stored in the OPA data
// document as data.role_bindings[subject][i].
type opaSubjectBinding struct {
	Role   string `json:"role"`
	Domain string `json:"domain"`
}

func marshalSubjectBindings(rows []opabinding.RoleBinding) (string, error) {
	out := make([]opaSubjectBinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, opaSubjectBinding{Role: r.Role, Domain: r.Domain})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalSubjectBindings(raw string) ([]opaSubjectBinding, error) {
	var out []opaSubjectBinding
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// toOPASlice converts []opaSubjectBinding to []any with map[string]any entries,
// which is the shape OPA expects from store.Read.
func toOPASlice(bindings []opaSubjectBinding) []any {
	out := make([]any, len(bindings))
	for i, b := range bindings {
		out[i] = map[string]any{
			"role":   b.Role,
			"domain": b.Domain,
		}
	}
	return out
}

// --- storage.Store interface ---

// Read intercepts path /role_bindings/{subject} and serves from cache or DB.
// All other paths are delegated to the inner inmem store (which holds Rego policy).
func (s *hybridStore) Read(ctx context.Context, txn storage.Transaction, path storage.Path) (any, error) {
	if len(path) == 2 && path[0] == "role_bindings" {
		subject := path[1]
		key := bindingCacheKey(subject)

		raw, ok, err := s.cache.Get(ctx, key)
		if err != nil {
			// Falling through to the database is the right answer here: the
			// authoritative store still works. It is logged because it is a
			// load transfer onto the database on every single request, and a
			// silent one is a load transfer nobody is alerted to.
			if s.logger != nil {
				s.logger.Error("binding cache unavailable, reading from database",
					"error", err, "subject", subject)
			}
		} else if ok {
			bindings, err := unmarshalSubjectBindings(raw)
			if err != nil {
				return nil, fmt.Errorf("hybridStore: cache unmarshal: %w", err)
			}
			return toOPASlice(bindings), nil
		}

		rows, err := s.db.ListBySubject(ctx, subject)
		if err != nil {
			return nil, fmt.Errorf("hybridStore: list by subject: %w", err)
		}

		bindings := make([]opaSubjectBinding, 0, len(rows))
		for _, r := range rows {
			bindings = append(bindings, opaSubjectBinding{Role: r.Role, Domain: r.Domain})
		}

		if encoded, err := marshalSubjectBindings(rows); err == nil {
			ttl := s.ttl
			if len(rows) == 0 && negativeBindingCacheTTL < ttl {
				ttl = negativeBindingCacheTTL
			}
			_ = s.cache.Set(ctx, key, encoded, ttl)
		}

		return toOPASlice(bindings), nil
	}

	return s.inner.Read(ctx, txn, path)
}

// Write intercepts path /role_bindings/{subject} to invalidate the cache.
// Actual DB writes are performed by the Engine before calling this.
// All other paths (policy etc.) are delegated to inner.
func (s *hybridStore) Write(ctx context.Context, txn storage.Transaction, op storage.PatchOp, path storage.Path, value any) error {
	if len(path) >= 1 && path[0] == "role_bindings" {
		if len(path) >= 2 {
			_ = s.cache.Delete(ctx, bindingCacheKey(path[1]))
		}
		return nil
	}
	return s.inner.Write(ctx, txn, op, path, value)
}

// NewTransaction delegates to inner; transactions are only meaningful for policy paths.
func (s *hybridStore) NewTransaction(ctx context.Context, params ...storage.TransactionParams) (storage.Transaction, error) {
	return s.inner.NewTransaction(ctx, params...)
}

// Commit delegates to inner.
func (s *hybridStore) Commit(ctx context.Context, txn storage.Transaction) error {
	return s.inner.Commit(ctx, txn)
}

// Abort delegates to inner.
func (s *hybridStore) Abort(ctx context.Context, txn storage.Transaction) {
	s.inner.Abort(ctx, txn)
}

// Truncate delegates to inner.
func (s *hybridStore) Truncate(ctx context.Context, txn storage.Transaction, params storage.TransactionParams, it storage.Iterator) error {
	return s.inner.Truncate(ctx, txn, params, it)
}

// Register delegates to inner.
func (s *hybridStore) Register(ctx context.Context, txn storage.Transaction, config storage.TriggerConfig) (storage.TriggerHandle, error) {
	return s.inner.Register(ctx, txn, config)
}

// ListPolicies delegates to inner.
func (s *hybridStore) ListPolicies(ctx context.Context, txn storage.Transaction) ([]string, error) {
	return s.inner.ListPolicies(ctx, txn)
}

// GetPolicy delegates to inner.
func (s *hybridStore) GetPolicy(ctx context.Context, txn storage.Transaction, id string) ([]byte, error) {
	return s.inner.GetPolicy(ctx, txn, id)
}

// UpsertPolicy delegates to inner.
func (s *hybridStore) UpsertPolicy(ctx context.Context, txn storage.Transaction, id string, bs []byte) error {
	return s.inner.UpsertPolicy(ctx, txn, id, bs)
}

// DeletePolicy delegates to inner.
func (s *hybridStore) DeletePolicy(ctx context.Context, txn storage.Transaction, id string) error {
	return s.inner.DeletePolicy(ctx, txn, id)
}

var _ storage.Store = (*hybridStore)(nil)
