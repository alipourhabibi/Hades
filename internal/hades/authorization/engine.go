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
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
)

// bindingStore is the minimal interface the Engine needs from the binding
// storage.
//
// It does not include ListAll. Nothing has read every binding since the policy
// store stopped being reloaded wholesale: hybridStore serves one subject at a
// time. Declaring it here meant every implementation had to provide a method
// that was never called.
type bindingStore interface {
	Create(ctx context.Context, subject, role, domain string) error
	ListBySubject(ctx context.Context, subject string) ([]opabinding.RoleBinding, error)
	DeleteBySubjectDomain(ctx context.Context, subject, domain string) error
}

//go:embed hades/authz/authz.rego
var policyContent string

// Engine is the in-process OPA authorization engine.
// It is safe to use concurrently. The policy is compiled once at startup;
// role bindings are served per-subject from cache (with DB fallback) by the
// hybridStore: no global reload happens on binding writes.
// The prepared queries are compiled once in the constructor and never
// replaced, so no lock guards them: rego.PreparedEvalQuery is safe for
// concurrent Eval. Allow and BatchAllow are on the hottest path in the system,
// and a mutex that is never write-locked only adds atomic traffic.
type Engine struct {
	query      rego.PreparedEvalQuery // single: data.hades.authz.allow
	batchQuery rego.PreparedEvalQuery // batch:  data.hades.authz.denied_indices
	store      *hybridStore
	db         bindingStore
	cache      cache.Cache
}

// Option configures an Engine at construction time.
type Option func(*engineOptions)

type engineOptions struct {
	superAdmins []string
	logger      *log.LoggerWrapper
}

// WithSuperAdmins seeds the subjects the policy's superadmin bypass matches.
// Without it the clause in authz.rego reads data.superadmins, which is
// undefined, and the rule can never fire.
func WithSuperAdmins(subjects []string) Option {
	return func(o *engineOptions) { o.superAdmins = subjects }
}

// WithLogger lets the binding store report cache outages instead of silently
// falling back to the database on every request.
func WithLogger(l *log.LoggerWrapper) Option {
	return func(o *engineOptions) { o.logger = l }
}

// New creates a new Engine with a cache-backed hybridStore and compiles the
// Rego policy once. DefaultTTL is used when ttl is zero.
func New(ctx context.Context, db opabinding.Storage, c cache.Cache, ttl time.Duration, opts ...Option) (*Engine, error) {
	return newFromStore(ctx, db, c, ttl, opts...)
}

// newFromStore is the internal constructor accepting the bindingStore interface
// so tests can inject a fake implementation.
func newFromStore(ctx context.Context, db bindingStore, c cache.Cache, ttl time.Duration, opts ...Option) (*Engine, error) {
	var o engineOptions
	for _, opt := range opts {
		opt(&o)
	}

	hs, err := newHybridStore(c, db, ttl, o.superAdmins)
	if err != nil {
		return nil, fmt.Errorf("authorization: engine: hybrid store: %w", err)
	}
	hs.logger = o.logger

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

// BatchAllow evaluates all policies in a single OPA Eval() call.
// Returns a []bool slice (true = allowed, false = denied) in the same order
// as inputs. OPA iterates input.policies internally; hybridStore serves each
// unique subject from cache (or DB on miss), so K unique subjects → K reads
// regardless of N total policies.
// constants.Policy JSON tags match the Rego input field names so it is passed
// directly to rego.EvalInput without an intermediate conversion struct.
func (e *Engine) BatchAllow(ctx context.Context, inputs []constants.Policy) ([]bool, error) {
	results := make([]bool, len(inputs))
	if len(inputs) == 0 {
		return results, nil
	}

	rs, err := e.batchQuery.Eval(ctx, rego.EvalInput(map[string]any{"policies": inputs}))
	if err != nil {
		return nil, fmt.Errorf("authorization: engine: batch eval: %w", err)
	}
	// An empty result set or an unexpected value shape means the policy did not
	// produce a decision. That is an error, not a grant: single-policy Allow
	// fails closed under the same conditions, and this path gates uploads.
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil, fmt.Errorf("authorization: engine: batch eval returned no decision")
	}

	// denied_indices is a Rego set; OPA surfaces it as []interface{} via JSON.
	// Each element is a json.Number (OPA uses UseNumber internally); float64 is
	// handled defensively in case the OPA version differs.
	denied, ok := rs[0].Expressions[0].Value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("authorization: engine: batch eval returned %T, want a set of indices", rs[0].Expressions[0].Value)
	}

	// Only once the response shape is confirmed are the entries marked allowed,
	// so a malformed response can never leave the slice all-true.
	for i := range results {
		results[i] = true
	}
	for _, item := range denied {
		idx, ok := toInt(item)
		if !ok {
			return nil, fmt.Errorf("authorization: engine: batch eval returned a non-numeric denied index")
		}
		if idx < 0 || idx >= len(results) {
			return nil, fmt.Errorf("authorization: engine: batch eval returned out-of-range denied index %d", idx)
		}
		results[idx] = false
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
