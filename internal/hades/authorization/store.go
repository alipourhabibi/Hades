package authorization

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
)

const bindingCacheKeyPrefix = "opa:bindings:"

// hybridStore implements storage.Store.
//
// The Rego policy stays in the inmem store. Reads of /role_bindings/{subject}
// come from the cache, and from the database when the cache misses. A write
// only clears that one subject from the cache. The database is always the
// source of truth.
//
// The memory cache is per process. A binding written on one pod is not seen by
// the other pods until the TTL runs out. Use Redis if you run more than one
// pod. SQLite runs in one process, so the memory cache fits it well.
type hybridStore struct {
	inner storage.Store // inmem store, holds the Rego policy only
	cache cache.Cache
	db    hybridBindingDB
	ttl   time.Duration
}

// hybridBindingDB is the subset of opabinding.Storage used by hybridStore.
type hybridBindingDB interface {
	ListBySubject(ctx context.Context, subject string) ([]opabinding.RoleBinding, error)
}

func newHybridStore(c cache.Cache, db hybridBindingDB, ttl time.Duration) (*hybridStore, error) {
	inner := inmem.New()

	// Seed inner with an empty role_bindings map so OPA policy compilation
	// sees a valid (though empty) data document and does not fail type-checks.
	ctx := context.Background()
	txn, err := inner.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		return nil, fmt.Errorf("hybridStore: seed txn: %w", err)
	}
	seed := map[string]any{"role_bindings": map[string]any{}}
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

		if raw, ok := s.cache.Get(ctx, key); ok {
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

		raw, err := marshalSubjectBindings(rows)
		if err == nil {
			_ = s.cache.Set(ctx, key, raw, s.ttl)
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
