package regresql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	genericStmt = "regresql_generic"

	// executions before the planner's custom-vs-generic decision
	genericWarmup = 6
)

var (
	skippableCodes = map[string]bool{"42601": true, "42883": true, "42725": true, "42P18": true, "42804": true}

	errGenericNoExtension = errors.New("pg_regresql is not active on the connection")
)

type genericOutcome struct {
	Failed  bool
	Skipped bool
	Message string
}

func checkGeneric(ctx context.Context, db *sql.DB, q *Query, names []string, bindings []map[string]any) (genericOutcome, error) {
	if len(q.Args) == 0 {
		return genericOutcome{Skipped: true, Message: "no parameters"}, nil
	}
	if len(bindings) == 0 {
		return genericOutcome{Skipped: true, Message: "no bindings"}, nil
	}
	if len(names) != len(bindings) {
		return genericOutcome{}, fmt.Errorf("%d binding names for %d bindings", len(names), len(bindings))
	}
	for _, b := range bindings {
		for name, v := range b {
			if v != nil && reflect.TypeOf(v).Kind() == reflect.Slice {
				return genericOutcome{Skipped: true, Message: fmt.Sprintf("binding %q is a list", name)}, nil
			}
		}
	}
	types, err := q.GetParamTypes()
	if err != nil {
		return genericOutcome{Skipped: true, Message: err.Error()}, nil
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return genericOutcome{}, err
	}
	defer conn.Close()

	var out genericOutcome
	var poisoned bool
	err = conn.Raw(func(driverConn any) error {
		var runErr error
		out, poisoned, runErr = runGeneric(ctx, driverConn.(*stdlib.Conn).Conn(), q, types, names, bindings)
		return runErr
	})
	if poisoned {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	return out, err
}

func runGeneric(ctx context.Context, c *pgx.Conn, q *Query, types, names []string, bindings []map[string]any) (out genericOutcome, poisoned bool, err error) {
	simple := pgx.QueryExecModeSimpleProtocol
	exec := func(stmt string) error {
		_, err := c.Exec(ctx, stmt, simple)
		return err
	}
	prepared := false
	defer func() {
		if exec("ROLLBACK") != nil || (prepared && exec("DEALLOCATE "+genericStmt) != nil) {
			poisoned = true
		}
	}()

	if err := exec("BEGIN"); err != nil {
		return out, false, err
	}
	if d := resolveTimeout(q); d > 0 {
		if err := exec(fmt.Sprintf("SET LOCAL statement_timeout = %d", d.Milliseconds())); err != nil {
			return out, false, err
		}
	}

	var active *string
	if err := c.QueryRow(ctx, "SELECT current_setting('pg_regresql.active', true)", simple).Scan(&active); err != nil {
		return out, false, err
	}
	if active == nil || *active != "on" {
		return out, false, errGenericNoExtension
	}

	sqlText, _ := q.Prepare(nil)
	decl := genericStmt
	if len(types) > 0 {
		decl += "(" + strings.Join(types, ", ") + ")"
	}
	if err := exec("PREPARE " + decl + " AS " + sqlText); err != nil {
		return failOrSkip(err)
	}
	prepared = true

	run := func(verbose bool, args []any) (*PlanNode, error) {
		opts := "FORMAT JSON"
		if verbose {
			opts = "VERBOSE, " + opts
		}
		return explainExecute(ctx, c, opts, args)
	}
	argsFor := func(i int) []any {
		_, params := q.Prepare(bindings[i])
		return params
	}

	if err := exec("SET LOCAL plan_cache_mode = auto"); err != nil {
		return out, false, err
	}
	for i := 0; i < genericWarmup; i++ {
		if _, err := run(false, argsFor(min(i, len(bindings)-1))); err != nil {
			return failOrSkip(err)
		}
	}
	var generics int
	if err := c.QueryRow(ctx, "SELECT generic_plans FROM pg_prepared_statements WHERE name = '"+genericStmt+"'", simple).Scan(&generics); err != nil {
		return out, false, err
	}

	if err := exec("SET LOCAL plan_cache_mode = force_custom_plan"); err != nil {
		return out, false, err
	}
	custom := make([]*PlanNode, len(bindings))
	for i := range bindings {
		p, err := run(true, argsFor(i))
		if err != nil {
			return failOrSkip(err)
		}
		custom[i] = p
	}
	if err := exec("SET LOCAL plan_cache_mode = force_generic_plan"); err != nil {
		return out, false, err
	}
	generic, err := run(true, argsFor(0))
	if err != nil {
		return failOrSkip(err)
	}

	relpages, err := readRelPages(ctx, c, generic)
	if err != nil {
		return out, false, err
	}
	flips := map[string][]genericFlip{}
	for i, name := range names {
		flips[name] = genericScanFlips(*custom[i], *generic, relpages)
	}
	failed, msg := genericVerdict(generics > 0, names, flips)
	return genericOutcome{Failed: failed, Message: msg}, false, nil
}

func explainExecute(ctx context.Context, c *pgx.Conn, opts string, args []any) (*PlanNode, error) {
	holders := make([]string, len(args))
	for i := range args {
		holders[i] = fmt.Sprintf("$%d", i+1)
	}
	stmt := fmt.Sprintf("EXPLAIN (%s) EXECUTE %s(%s)", opts, genericStmt, strings.Join(holders, ", "))

	var raw string
	if err := c.QueryRow(ctx, stmt, append([]any{pgx.QueryExecModeSimpleProtocol}, args...)...).Scan(&raw); err != nil {
		return nil, err
	}
	var plans []ExplainOutput
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) == 0 {
		return nil, fmt.Errorf("unreadable EXPLAIN output")
	}
	return &plans[0].Plan, nil
}

func readRelPages(ctx context.Context, c *pgx.Conn, generic *PlanNode) (map[string]int64, error) {
	relpages := map[string]int64{}
	for k, nodeType := range collectScans(*generic) {
		if nodeType != "Seq Scan" || k.Schema == "" {
			continue
		}
		var pages int64
		err := c.QueryRow(ctx,
			"SELECT relpages FROM pg_class WHERE oid = to_regclass(quote_ident($1) || '.' || quote_ident($2))",
			pgx.QueryExecModeSimpleProtocol, k.Schema, k.Relation).Scan(&pages)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		relpages[relPagesKey(k.Schema, k.Relation)] = pages
	}
	return relpages, nil
}

// timeout fails, parameter type and value errors skip, anything else surfaces
func failOrSkip(err error) (genericOutcome, bool, error) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return genericOutcome{}, false, err
	}
	switch {
	case pgErr.Code == pgQueryCanceled:
		return genericOutcome{Failed: true, Message: "generic: statement timeout"}, false, nil
	case strings.HasPrefix(pgErr.Code, "22"), skippableCodes[pgErr.Code]:
		return genericOutcome{Skipped: true, Message: pgErr.Message}, false, nil
	}
	return genericOutcome{}, false, err
}
