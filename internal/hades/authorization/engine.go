// Package authorization provides an in-process OPA policy engine that
// evaluates role-based access control decisions. Role bindings are
// stored in the database and served per-subject through a cache-backed
// hybridStore, avoiding global reloads on every write.
package authorization

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
)

// bindingStore is the minimal interface the Engine needs from the binding storage.
type bindingStore interface {
	Create(ctx context.Context, subject, role, domain string) error
	ListAll(ctx context.Context) ([]opabinding.RoleBinding, error)
	ListBySubject(ctx context.Context, subject string) ([]opabinding.RoleBinding, error)
	DeleteBySubjectDomain(ctx context.Context, subject, domain string) error
}

//go:embed hades/authz/authz.rego
var policyContent string

// Engine is the OPA authorization engine. It runs inside this process.
// It is safe to use from many goroutines at once.
//
// The policy is compiled once at startup. Role bindings are read per subject by
// the hybridStore, from the cache first and the database on a miss. Writing a
// binding does not reload the policy.
type Engine struct {
	mu         sync.RWMutex
	query      rego.PreparedEvalQuery // single: data.hades.authz.allow
	batchQuery rego.PreparedEvalQuery // batch:  data.hades.authz.denied_indices
	store      *hybridStore
	db         bindingStore
	cache      cache.Cache
}

// New creates a new Engine with a cache-backed hybridStore and compiles the
// Rego policy once. DefaultTTL is used when ttl is zero.
func New(ctx context.Context, db opabinding.Storage, c cache.Cache, ttl time.Duration) (*Engine, error) {
	return newFromStore(ctx, db, c, ttl)
}

// newFromStore is the internal constructor accepting the bindingStore interface
// so tests can inject a fake implementation.
func newFromStore(ctx context.Context, db bindingStore, c cache.Cache, ttl time.Duration) (*Engine, error) {
	hs, err := newHybridStore(c, db, ttl)
	if err != nil {
		return nil, fmt.Errorf("authorization: engine: hybrid store: %w", err)
	}

	e := &Engine{
		store: hs,
		db:    db,
		cache: c,
	}

	pq, err := newQuery(ctx, hs)
	if err != nil {
		return nil, err
	}
	e.query = pq

	bq, err := newBatchQuery(ctx, hs)
	if err != nil {
		return nil, err
	}
	e.batchQuery = bq

	return e, nil
}

// newQuery compiles the single-eval query against the given store.
func newQuery(ctx context.Context, s storage.Store) (rego.PreparedEvalQuery, error) {
	pq, err := rego.New(
		rego.Query("data.hades.authz.allow"),
		rego.Module("authz.rego", policyContent),
		rego.Store(s),
	).PrepareForEval(ctx)
	if err != nil {
		return rego.PreparedEvalQuery{}, fmt.Errorf("authorization: engine: prepare query: %w", err)
	}
	return pq, nil
}

// newBatchQuery compiles the batch query (denied_indices) against the given store.
func newBatchQuery(ctx context.Context, s storage.Store) (rego.PreparedEvalQuery, error) {
	pq, err := rego.New(
		rego.Query("data.hades.authz.denied_indices"),
		rego.Module("authz.rego", policyContent),
		rego.Store(s),
	).PrepareForEval(ctx)
	if err != nil {
		return rego.PreparedEvalQuery{}, fmt.Errorf("authorization: engine: prepare batch query: %w", err)
	}
	return pq, nil
}

// Allow evaluates the OPA policy for the given policy and returns true if the
// action is permitted. Role bindings for policy.Subject are fetched from cache
// (or DB on miss) by the hybridStore during evaluation.
func (e *Engine) Allow(ctx context.Context, input constants.Policy) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rs, err := e.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return false, fmt.Errorf("authorization: engine: eval: %w", err)
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return false, nil
	}
	allowed, ok := rs[0].Expressions[0].Value.(bool)
	if !ok {
		return false, nil
	}
	return allowed, nil
}

// BatchAllow checks many policies in one OPA call.
// It returns one bool for each input, in the same order. True means allowed.
//
// OPA loops over input.policies itself. hybridStore reads each subject once,
// from the cache or from the database. So the number of reads follows the
// number of different subjects, not the number of policies.
//
// constants.Policy has JSON tags that match the Rego field names, so we pass it
// straight to rego.EvalInput with no extra struct in between.
func (e *Engine) BatchAllow(ctx context.Context, inputs []constants.Policy) ([]bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	results := make([]bool, len(inputs))
	for i := range results {
		results[i] = true
	}
	if len(inputs) == 0 {
		return results, nil
	}

	rs, err := e.batchQuery.Eval(ctx, rego.EvalInput(map[string]any{"policies": inputs}))
	if err != nil {
		return nil, fmt.Errorf("authorization: engine: batch eval: %w", err)
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return results, nil
	}

	// denied_indices is a Rego set; OPA surfaces it as []interface{} via JSON.
	// Each element is a json.Number (OPA uses UseNumber internally).
	// Also handle float64 defensively in case OPA version differs.
	raw := rs[0].Expressions[0].Value
	switch v := raw.(type) {
	case []interface{}:
		for _, item := range v {
			idx, ok := toInt(item)
			if ok && idx >= 0 && idx < len(results) {
				results[idx] = false
			}
		}
	}

	return results, nil
}

// toInt converts a json.Number or float64 to int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	case float64:
		return int(n), true
	}
	return 0, false
}

// AddBinding inserts a single role binding into the database and invalidates
// the per-subject cache entry so the next Allow() call fetches fresh data.
func (e *Engine) AddBinding(ctx context.Context, subject, role, domain string) error {
	if err := e.db.Create(ctx, subject, role, domain); err != nil {
		return err
	}
	return e.cache.Delete(ctx, bindingCacheKey(subject))
}

// DeleteBinding removes role bindings for subject+domain from the database
// and invalidates the per-subject cache entry.
func (e *Engine) DeleteBinding(ctx context.Context, subject, domain string) error {
	if err := e.db.DeleteBySubjectDomain(ctx, subject, domain); err != nil {
		return err
	}
	return e.cache.Delete(ctx, bindingCacheKey(subject))
}
