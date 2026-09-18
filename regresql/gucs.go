package regresql

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Per-query session GUCs, carried as `-- gucs: enable_seqscan=off; work_mem='64kB'`.
type GUC struct {
	Name  string
	Value string
}

// identifier, optionally with an extension prefix (`plpgsql.foo`)
var gucNameRx = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// bare word/number, quoted literal, or comma-separated list of those
var gucValueRx = regexp.MustCompile(
	`^(?:'(?:[^']|'')*'|[A-Za-z0-9_.+-]+)(?:\s*,\s*(?:'(?:[^']|'')*'|[A-Za-z0-9_.+-]+))*$`)

// Semicolon-separated, because a value may itself contain commas.
func (q *Query) GetGUCs() []GUC {
	v, ok := q.GetMetadata("gucs")
	if !ok {
		return nil
	}
	var out []GUC
	for _, part := range strings.Split(v, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if !gucNameRx.MatchString(name) || !gucValueRx.MatchString(value) {
			continue
		}
		out = append(out, GUC{Name: name, Value: value})
	}
	return out
}

// applyGUCsTx sets each GUC inside its own savepoint: a failed SET aborts the
// whole transaction, and a single unknown name must not take out the query.
// Returns the names that would not set, for the caller to surface.
func applyGUCsTx(ctx context.Context, q execer, gucs []GUC) (skipped []string) {
	for _, g := range gucs {
		if _, err := q.ExecContext(ctx, "SAVEPOINT rq_guc"); err != nil {
			return append(skipped, g.Name)
		}
		_, err := q.ExecContext(ctx, "SET LOCAL "+g.Name+" = "+g.Value)
		if err != nil {
			skipped = append(skipped, g.Name)
			if _, rerr := q.ExecContext(ctx, "ROLLBACK TO SAVEPOINT rq_guc"); rerr != nil {
				return skipped
			}
		}
		if _, err := q.ExecContext(ctx, "RELEASE SAVEPOINT rq_guc"); err != nil {
			return skipped
		}
	}
	return skipped
}

// applyGUCsConn is the no-transaction variant for the admit/metamorphic hash
// path: without a transaction a failed SET leaves the connection usable.
func applyGUCsConn(ctx context.Context, q execer, gucs []GUC) (skipped []string) {
	for _, g := range gucs {
		if _, err := q.ExecContext(ctx, "SET "+g.Name+" = "+g.Value); err != nil {
			skipped = append(skipped, g.Name)
		}
	}
	return skipped
}

func gucSkipNote(skipped []string) string {
	if len(skipped) == 0 {
		return ""
	}
	return fmt.Sprintf("gucs not settable on this build: %s", strings.Join(skipped, ", "))
}
