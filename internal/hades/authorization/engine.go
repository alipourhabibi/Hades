// Package authorization provides an in-process OPA policy engine that
// evaluates role-based access control decisions. Role bindings are
// stored in PostgreSQL and synced to an OPA in-memory store on demand.
package authorization

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
)

// bindingStore is the minimal interface the Engine needs from the binding storage.
// *opabinding.OPABindingStorage satisfies it; tests can provide a fake.
type bindingStore interface {
	ListAll(ctx context.Context) ([]opabinding.RoleBinding, error)
	Create(ctx context.Context, subject, role, domain string) error
}

//go:embed hades/authz/authz.rego
var policyContent string

// Input is the authorization context passed to OPA for each policy evaluation.
type Input struct {
	// Subject is the username of the caller.
	Subject string `json:"subject"`
	// Domain is the resource identifier, e.g. "alice/mymodule".
	Domain string `json:"domain"`
	// ResourceType is one of "module", "label", "commit".
	ResourceType string `json:"resource_type"`
	// Action is one of "create", "read", "list", "update", "push", "delete", "admin", "transfer".
	Action string `json:"action"`
	// Visibility is "public" or "private".
	Visibility string `json:"visibility"`
}

// opaData mirrors the top-level data document stored in the OPA in-memory store.
type opaData struct {
	RoleBindings []opaBinding `json:"role_bindings"`
	Superadmins  []string     `json:"superadmins"`
}

type opaBinding struct {
	Subject string `json:"subject"`
	Role    string `json:"role"`
	Domain  string `json:"domain"`
}

// Engine is the in-process OPA authorization engine.
// It is safe to use concurrently. The policy is compiled once at startup;
// data (role bindings) is refreshed on demand via Reload.
type Engine struct {
	mu    sync.RWMutex
	query rego.PreparedEvalQuery
	store storage.Store
	db    bindingStore
}

// New creates a new Engine, seeds the in-memory store from the database, and
// compiles the Rego policy once.
func New(ctx context.Context, db opabinding.Storage) (*Engine, error) {
	return newFromStore(ctx, db)
}

// newFromStore is the internal constructor that accepts the bindingStore
// interface, allowing tests to pass a fake implementation.
func newFromStore(ctx context.Context, db bindingStore) (*Engine, error) {
	e := &Engine{
		store: inmem.New(),
		db:    db,
	}

	// Seed the OPA store from the database before compiling the query.
	if err := e.reload(ctx); err != nil {
		return nil, fmt.Errorf("authorization: engine: initial reload: %w", err)
	}

	// Compile the policy once; the PreparedEvalQuery is safe to reuse.
	pq, err := newQuery(ctx, e.store)
	if err != nil {
		return nil, err
	}
	e.query = pq

	return e, nil
}

// newQuery compiles the Rego policy against the given store.
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

// Allow evaluates the OPA policy for the given input and returns true if the
// action is permitted.
func (e *Engine) Allow(ctx context.Context, input Input) (bool, error) {
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

// Reload re-reads all role bindings from the database and updates the OPA
// in-memory store. Callers should invoke this after committing new bindings.
func (e *Engine) Reload(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.reload(ctx)
}

// reload is the internal (lock-free) reload implementation.
func (e *Engine) reload(ctx context.Context) error {
	rows, err := e.db.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("authorization: engine: reload: list bindings: %w", err)
	}

	data := opaData{
		RoleBindings: make([]opaBinding, 0, len(rows)),
		Superadmins:  []string{},
	}
	for _, r := range rows {
		data.RoleBindings = append(data.RoleBindings, opaBinding{
			Subject: r.Subject,
			Role:    r.Role,
			Domain:  r.Domain,
		})
	}

	dataMap, err := toMap(data)
	if err != nil {
		return fmt.Errorf("authorization: engine: reload: marshal data: %w", err)
	}

	txn, err := e.store.NewTransaction(ctx, storage.WriteParams)
	if err != nil {
		return fmt.Errorf("authorization: engine: reload: new txn: %w", err)
	}

	if err := e.store.Write(ctx, txn, storage.ReplaceOp, storage.MustParsePath("/"), dataMap); err != nil {
		e.store.Abort(ctx, txn)
		return fmt.Errorf("authorization: engine: reload: write: %w", err)
	}

	if err := e.store.Commit(ctx, txn); err != nil {
		return fmt.Errorf("authorization: engine: reload: commit: %w", err)
	}

	return nil
}

// AddBinding inserts a single role binding into the database and reloads the
// OPA store so the change takes effect immediately.
func (e *Engine) AddBinding(ctx context.Context, subject, role, domain string) error {
	if err := e.db.Create(ctx, subject, role, domain); err != nil {
		return err
	}
	return e.Reload(ctx)
}

// AddBindingInTx inserts a single role binding using the transaction injected
// into ctx by UnitOfWork.Do. The OPA store is NOT reloaded here; the caller
// must call Reload after the transaction commits.
func (e *Engine) AddBindingInTx(ctx context.Context, subject, role, domain string) error {
	return e.db.Create(ctx, subject, role, domain)
}

// toMap round-trips a value through JSON to get a plain map[string]any,
// which is what the OPA storage layer expects.
func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
