package regresql

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// A2 (per-query statement_timeout) has three independent moving parts, tested here:
//
//  1. resolution    — given a per-query metadata override and a global config
//                     default, which timeout actually applies, and when does it
//                     resolve to "no timeout" (0)?
//  2. parsing       — does the `-- regresql: timeout:<dur>` metadata land in
//                     RegressQLOptions.Timeout, and is a malformed duration
//                     ignored rather than fatal?
//  3. detection     — is a PostgreSQL statement_timeout cancellation (SQLSTATE
//                     57014) recognised so the suite can turn it into a
//                     "did-not-complete" verdict instead of a fatal run error?
//
// The actual SET LOCAL execution and the failed-result wiring need a live
// PostgreSQL connection, so they are exercised by manual/integration runs, not
// here. These unit tests pin the pure decision logic that decides *whether* and
// *how long* to bound a query, plus the error-classification that gates the
// whole "treat timeout as divergence" behaviour.

// withGlobalTimeout installs a global config whose `timeout:` field is set to
// the given raw value (e.g. "30s", "" for unset, "garbage" for invalid) and
// restores the previous cached config when the test finishes. resolveTimeout
// and GetStatementTimeout both read this process-global, so every test that
// touches it must isolate itself this way to avoid leaking state into others.
func withGlobalTimeout(t *testing.T, raw string) {
	t.Helper()
	prev := cachedConfig
	t.Cleanup(func() { cachedConfig = prev })
	SetGlobalConfig(config{Timeout: raw})
}

// queryWithMetadata writes a one-off .sql file containing the given metadata
// header and parses it the same way the suite does. Metadata (`-- key: value`)
// is only recognised by the file loader, not by NewQueryFromString, so tests
// that care about `-- regresql:` options must go through a real file.
func queryWithMetadata(t *testing.T, sqlText string) *Query {
	t.Helper()
	path := filepath.Join(t.TempDir(), "q.sql")
	if err := os.WriteFile(path, []byte(sqlText), 0644); err != nil {
		t.Fatalf("write query file: %v", err)
	}
	queries, err := parseQueryFile(path)
	if err != nil {
		t.Fatalf("parse query file: %v", err)
	}
	for _, q := range queries {
		return q
	}
	t.Fatal("parsed query file contained no queries")
	return nil
}

func TestResolveTimeout(t *testing.T) {
	// The whole point of resolveTimeout is precedence: a per-query override is a
	// deliberate, query-specific signal and must win over the project-wide
	// default, while an absent override must transparently fall back to it. The
	// 0/"no timeout" case is load-bearing too — applyStatementTimeout treats 0 as
	// "leave statement_timeout untouched", so an empty config must not silently
	// impose a bound.
	cases := []struct {
		name          string
		globalRaw     string        // config `timeout:` value
		queryOverride string        // `-- regresql: timeout:...` value ("" = none)
		want          time.Duration
	}{
		{"neither set means unbounded", "", "", 0},
		{"global default applies when no override", "30s", "", 30 * time.Second},
		{"per-query override wins over global", "30s", "250ms", 250 * time.Millisecond},
		{"per-query override applies with no global", "", "5s", 5 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withGlobalTimeout(t, tc.globalRaw)

			sqlText := "-- name: q\nselect 1;\n"
			if tc.queryOverride != "" {
				sqlText = "-- name: q\n-- regresql: timeout:" + tc.queryOverride + "\nselect 1;\n"
			}
			q := queryWithMetadata(t, sqlText)

			if got := resolveTimeout(q); got != tc.want {
				t.Errorf("resolveTimeout() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetStatementTimeout(t *testing.T) {
	// GetStatementTimeout parses the config string into a duration. An invalid
	// value must degrade to 0 (no timeout) rather than panicking or aborting the
	// run: a typo in regress.yaml should never be the reason a test suite refuses
	// to run.
	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{"unset", "", 0},
		{"valid seconds", "30s", 30 * time.Second},
		{"valid milliseconds", "500ms", 500 * time.Millisecond},
		{"invalid falls back to none", "not-a-duration", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withGlobalTimeout(t, tc.raw)
			if got := GetStatementTimeout(); got != tc.want {
				t.Errorf("GetStatementTimeout() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetRegressQLOptions_ParsesTimeout(t *testing.T) {
	// The timeout option must coexist with the other comma-separated regresql
	// options (notest, nobaseline, ...) without interfering with them, and an
	// unparseable duration must leave Timeout at zero while the surrounding
	// options still take effect — a bad timeout shouldn't swallow a `notest`.
	cases := []struct {
		name        string
		metadata    string
		wantTimeout time.Duration
		wantNoTest  bool
	}{
		{"timeout alone", "timeout:2s", 2 * time.Second, false},
		{"timeout among other options", "notest, timeout:750ms", 750 * time.Millisecond, true},
		{"invalid duration ignored, siblings survive", "timeout:nope, notest", 0, true},
		{"no timeout option", "notest", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := queryWithMetadata(t, "-- name: q\n-- regresql: "+tc.metadata+"\nselect 1;\n")
			opts := q.GetRegressQLOptions()
			if opts.Timeout != tc.wantTimeout {
				t.Errorf("Timeout = %v, want %v", opts.Timeout, tc.wantTimeout)
			}
			if opts.NoTest != tc.wantNoTest {
				t.Errorf("NoTest = %v, want %v", opts.NoTest, tc.wantNoTest)
			}
		})
	}
}

func TestIsTimeoutError(t *testing.T) {
	// This classifier is the gate for the entire "treat timeout as a divergence"
	// behaviour: only SQLSTATE 57014 (query_canceled, which is what
	// statement_timeout raises) should be swallowed into a did-not-complete
	// verdict. Every other error — including other PgErrors and plain Go errors —
	// must stay fatal so genuine failures still abort the run loudly. errors.As
	// is used in the implementation, so a wrapped 57014 must still be detected.
	canceled := &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"statement_timeout cancellation", canceled, true},
		{"wrapped statement_timeout cancellation", errors.New("exec: " + canceled.Error()), false}, // plain wrap loses the type
		{"errors.Wrap of the PgError is still detected", &wrapErr{canceled}, true},
		{"different SQLSTATE is not a timeout", &pgconn.PgError{Code: "42601"}, false},
		{"plain error is not a timeout", errors.New("boom"), false},
		{"nil error is not a timeout", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTimeoutError(tc.err); got != tc.want {
				t.Errorf("isTimeoutError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// wrapErr is a minimal error wrapper that preserves the wrapped error for
// errors.As, mirroring how the database/sql + pgx stack hands back a PgError
// nested inside higher-level errors.
type wrapErr struct{ inner error }

func (e *wrapErr) Error() string { return "wrapped: " + e.inner.Error() }
func (e *wrapErr) Unwrap() error { return e.inner }
