package regresql

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type (
	// SnapshotDiffResult contains the results of comparing two snapshots
	SnapshotDiffResult struct {
		FromTag   string
		ToTag     string
		Changed   []QueryDiff
		Unchanged []string
		Errors    []QueryError
	}

	// QueryDiff represents a difference in query output between snapshots
	QueryDiff struct {
		QueryPath  string
		FromRows   int
		ToRows     int
		FromResult *ResultSet
		ToResult   *ResultSet
		Diff       string
	}

	// QueryError represents an error running a query against a snapshot
	QueryError struct {
		QueryPath string
		Error     string
	}

	// diffQuery holds a query and its relative path for diff operations
	diffQuery struct {
		Query   *Query
		RelPath string
		SQL     string
	}
)

// DiffSnapshots compares query outputs between two snapshots
func DiffSnapshots(root string, from, to *SnapshotInfo, queryFilter, runFilter string) (*SnapshotDiffResult, error) {
	config, err := ReadConfig(root)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if config.PgUri == "" {
		return nil, fmt.Errorf("pguri not configured")
	}

	suite := Walk(root, config.Ignore)
	if runFilter != "" {
		suite.SetRunFilter(runFilter)
	}
	if queryFilter != "" {
		suite.SetPathFilters([]string{queryFilter})
	}

	result := &SnapshotDiffResult{
		FromTag: FormatSnapshotRef(from),
		ToTag:   FormatSnapshotRef(to),
	}

	queries, err := collectDiffQueries(suite)
	if err != nil {
		return nil, fmt.Errorf("failed to collect queries: %w", err)
	}

	if len(queries) == 0 {
		return result, nil
	}

	ts := time.Now().UnixNano()
	fromDB := fmt.Sprintf("regresql_diff_from_%d", ts)
	toDB := fmt.Sprintf("regresql_diff_to_%d", ts+1)

	fmt.Printf("Restoring %s to temp database...\n", result.FromTag)
	if err := createAndRestore(config.PgUri, fromDB, from.Path); err != nil {
		return nil, fmt.Errorf("failed to restore 'from' snapshot: %w", err)
	}
	defer dropDatabase(config.PgUri, fromDB)

	fmt.Printf("Restoring %s to temp database...\n", result.ToTag)
	if err := createAndRestore(config.PgUri, toDB, to.Path); err != nil {
		return nil, fmt.Errorf("failed to restore 'to' snapshot: %w", err)
	}
	defer dropDatabase(config.PgUri, toDB)

	fromURI, _ := replaceDatabase(config.PgUri, fromDB)
	toURI, _ := replaceDatabase(config.PgUri, toDB)

	fromConn, err := OpenDB(fromURI)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to 'from' database: %w", err)
	}
	defer fromConn.Close()

	toConn, err := OpenDB(toURI)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to 'to' database: %w", err)
	}
	defer toConn.Close()

	fmt.Printf("Comparing %d queries...\n\n", len(queries))

	for _, q := range queries {
		fromResult, fromErr := executeQueryForDiff(fromConn, q.SQL)
		toResult, toErr := executeQueryForDiff(toConn, q.SQL)

		queryPath := q.RelPath

		if fromErr != nil || toErr != nil {
			errMsg := ""
			if fromErr != nil {
				errMsg = fmt.Sprintf("from: %v", fromErr)
			}
			if toErr != nil {
				if errMsg != "" {
					errMsg += "; "
				}
				errMsg += fmt.Sprintf("to: %v", toErr)
			}
			result.Errors = append(result.Errors, QueryError{
				QueryPath: queryPath,
				Error:     errMsg,
			})
			continue
		}

		if diffResultSetsEqual(fromResult, toResult) {
			result.Unchanged = append(result.Unchanged, queryPath)
		} else {
			diff := formatDiffResultSetDiff(fromResult, toResult)
			result.Changed = append(result.Changed, QueryDiff{
				QueryPath:  queryPath,
				FromRows:   len(fromResult.Rows),
				ToRows:     len(toResult.Rows),
				FromResult: fromResult,
				ToResult:   toResult,
				Diff:       diff,
			})
		}
	}

	return result, nil
}

func createAndRestore(pguri, dbName, snapshotPath string) error {
	adminURI, err := replaceDatabase(pguri, "postgres")
	if err != nil {
		return err
	}

	adminConn, err := OpenDB(adminURI)
	if err != nil {
		return err
	}
	defer adminConn.Close()

	_, err = adminConn.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	targetURI, _ := replaceDatabase(pguri, dbName)
	return RestoreSnapshot(targetURI, RestoreOptions{
		InputPath: snapshotPath,
		Clean:     false,
	})
}

func dropDatabase(pguri, dbName string) {
	adminURI, err := replaceDatabase(pguri, "postgres")
	if err != nil {
		return
	}

	adminConn, err := OpenDB(adminURI)
	if err != nil {
		return
	}
	defer adminConn.Close()

	adminConn.Exec(fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = '%s' AND pid <> pg_backend_pid()
	`, dbName))
	adminConn.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
}

func executeQueryForDiff(db *sql.DB, sqlText string) (*ResultSet, error) {
	return RunQuery(db, sqlText)
}

func diffResultSetsEqual(a, b *ResultSet) bool {
	if len(a.Cols) != len(b.Cols) {
		return false
	}
	for i := range a.Cols {
		if a.Cols[i] != b.Cols[i] {
			return false
		}
	}

	if len(a.Rows) != len(b.Rows) {
		return false
	}

	for i := range a.Rows {
		if len(a.Rows[i]) != len(b.Rows[i]) {
			return false
		}
		for j := range a.Rows[i] {
			if !diffValuesEqual(a.Rows[i][j], b.Rows[i][j]) {
				return false
			}
		}
	}

	return true
}

func diffValuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func formatDiffResultSetDiff(from, to *ResultSet) string {
	var diff strings.Builder

	// Simple row-based diff
	maxRows := len(from.Rows)
	if len(to.Rows) > maxRows {
		maxRows = len(to.Rows)
	}

	for i := 0; i < maxRows; i++ {
		var fromRow, toRow []any
		if i < len(from.Rows) {
			fromRow = from.Rows[i]
		}
		if i < len(to.Rows) {
			toRow = to.Rows[i]
		}

		if !diffRowsEqual(fromRow, toRow) {
			if fromRow != nil && (toRow == nil || !diffRowsEqual(fromRow, toRow)) {
				diff.WriteString(fmt.Sprintf("- %v\n", fromRow))
			}
			if toRow != nil && (fromRow == nil || !diffRowsEqual(fromRow, toRow)) {
				diff.WriteString(fmt.Sprintf("+ %v\n", toRow))
			}
		}
	}

	return diff.String()
}

func diffRowsEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !diffValuesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func collectDiffQueries(s *Suite) ([]*diffQuery, error) {
	var queries []*diffQuery

	for _, folder := range s.Dirs {
		for _, name := range folder.Files {
			qfile := filepath.Join(s.Root, folder.Dir, name)
			relPath := filepath.Join(folder.Dir, name)

			if len(s.pathFilters) > 0 && !s.matchesPathFilter(relPath) {
				continue
			}

			parsedQueries, err := parseQueryFile(qfile)
			if err != nil {
				return nil, err
			}

			for queryName, q := range parsedQueries {
				if !s.matchesRunFilter(name, queryName) {
					continue
				}
				if opts := q.GetRegressQLOptions(); opts.NoTest {
					continue
				}

				queryRelPath := relPath
				if queryName != "default" && queryName != name {
					queryRelPath = filepath.Join(folder.Dir, queryName+".sql")
				}

				queries = append(queries, &diffQuery{
					Query:   q,
					RelPath: queryRelPath,
					SQL:     q.RawQuery(),
				})
			}
		}
	}

	return queries, nil
}
