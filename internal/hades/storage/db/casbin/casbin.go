package casbin

import (
	"context"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CasbinStorage is Casbin adapter for PostgreSQL and pgx
type CasbinStorage struct {
	pool *pgxpool.Pool
}

// NewCasbinStorage initializes a new adapter with pgxpool.
func New(pool *pgxpool.Pool) *CasbinStorage {
	return &CasbinStorage{pool: pool}
}

// LoadPolicy loads all policy rules from the database.
func (a *CasbinStorage) LoadPolicy(model model.Model) error {
	query := `
	SELECT ptype, v0, v1, v2, COALESCE(v3, '') AS v3, COALESCE(v4, '') AS v4, COALESCE(v5, '') AS v5
	FROM casbin_rule
`
	rows, err := a.pool.Query(context.Background(), query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var ptype, v0, v1, v2, v3, v4, v5 string
		err := rows.Scan(&ptype, &v0, &v1, &v2, &v3, &v4, &v5)
		if err != nil {
			return err
		}

		rule := []string{ptype, v0, v1, v2, v3}
		rule = removeEmptyStrings(rule)

		line := strings.Join(rule, ", ")
		persist.LoadPolicyLine(line, model)
	}

	return nil
}

// SavePolicy saves all policy rules to the database.
func (a *CasbinStorage) SavePolicy(model model.Model) error {
	_, err := a.pool.Exec(context.Background(), "DELETE FROM casbin_rule") // Clear table
	if err != nil {
		return err
	}

	var rules [][]string
	for ptype, ast := range model["p"] {
		for _, rule := range ast.Policy {
			rules = append(rules, append([]string{ptype}, rule...))
		}
	}
	for ptype, ast := range model["g"] {
		for _, rule := range ast.Policy {
			rules = append(rules, append([]string{ptype}, rule...))
		}
	}

	for _, rule := range rules {
		_, err := a.pool.Exec(context.Background(),
			"INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			getValues(rule)...)
		if err != nil {
			return err
		}
	}

	return nil
}

// AddPolicy adds a policy rule.
func (a *CasbinStorage) AddPolicy(sec string, ptype string, rule []string) error {
	_, err := a.pool.Exec(context.Background(),
		"INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES ($1, $2, $3, $4)",
		getValues(append([]string{ptype}, rule...))...)
	return err
}

// RemovePolicy removes a policy rule.
func (a *CasbinStorage) RemovePolicy(sec string, ptype string, rule []string) error {
	_, err := a.pool.Exec(context.Background(),
		"DELETE FROM casbin_rule WHERE ptype = $1 AND v0 = $2 AND v1 = $3 AND v2 = $4 AND v3 = $5 AND v4 = $6 AND v5 = $7",
		getValues(append([]string{ptype}, rule...))...)
	return err
}

// RemoveFilteredPolicy removes policy rules that match the filter.
func (a *CasbinStorage) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	query := "DELETE FROM casbin_rule WHERE ptype = $1"
	args := []interface{}{ptype}

	for i, value := range fieldValues {
		if value != "" {
			query += fmt.Sprintf(" AND v%d = $%d", fieldIndex+i, len(args)+1)
			args = append(args, value)
		}
	}

	_, err := a.pool.Exec(context.Background(), query, args...)
	return err
}

// Utility function to remove empty strings from a slice.
func removeEmptyStrings(input []string) []string {
	var result []string
	for _, str := range input {
		if str != "" {
			result = append(result, str)
		}
	}
	return result
}

// Utility function to convert a slice to interface{} for query arguments.
func getValues(vals []string) []interface{} {
	result := make([]interface{}, len(vals))
	for i, v := range vals {
		result[i] = v
	}
	return result
}

func (s *CasbinStorage) AddPolicies(sec string, ptype string, rules [][]string) error {
	if len(rules) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, rule := range rules {
		values := getValues(append([]string{ptype}, rule...))
		batch.Queue(
			"INSERT INTO casbin_rule (ptype, v0, v1, v2, v3) VALUES ($1, $2, $3, $4, $5)",
			values...,
		)
	}

	br := s.pool.SendBatch(context.Background(), batch)
	defer br.Close()

	for i := 0; i < len(rules); i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *CasbinStorage) RemovePolicies(sec string, ptype string, rules [][]string) error {
	if len(rules) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, rule := range rules {
		values := getValues(append([]string{ptype}, rule...))
		batch.Queue(
			"DELETE FROM casbin_rule WHERE ptype = $1 AND v0 = $2 AND v1 = $3 AND v2 = $4 AND v3 = $5",
			values...,
		)
	}

	br := s.pool.SendBatch(context.Background(), batch)
	defer br.Close()

	for i := 0; i < len(rules); i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}
