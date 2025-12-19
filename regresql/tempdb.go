package regresql

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type (
	TempDB struct {
		Name      string
		PgUri     string
		AdminUri  string
		cleanedUp bool
	}

	TempDBOptions struct {
		BasePgUri string
		Prefix    string
	}
)

func CreateTempDB(opts TempDBOptions) (*TempDB, error) {
	if opts.Prefix == "" {
		opts.Prefix = "regresql_temp"
	}

	name := fmt.Sprintf("%s_%d", opts.Prefix, time.Now().UnixNano())

	adminUri, tempUri, err := modifyPgUri(opts.BasePgUri, "postgres", name)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	adminDB, err := sql.Open("pgx", adminUri)
	if err != nil {
		return nil, fmt.Errorf("failed to connect for admin operations: %w", err)
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping admin database: %w", err)
	}

	_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", QuoteIdentifier(name)))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp database %q: %w", name, err)
	}

	return &TempDB{
		Name:     name,
		PgUri:    tempUri,
		AdminUri: adminUri,
	}, nil
}

func (t *TempDB) Drop() error {
	if t.cleanedUp {
		return nil
	}

	adminDB, err := sql.Open("pgx", t.AdminUri)
	if err != nil {
		return fmt.Errorf("failed to connect for drop: %w", err)
	}
	defer adminDB.Close()

	_, _ = adminDB.Exec(fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = %s AND pid <> pg_backend_pid()",
		QuoteLiteral(t.Name),
	))

	_, err = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", QuoteIdentifier(t.Name)))
	if err != nil {
		return fmt.Errorf("failed to drop temp database %q: %w", t.Name, err)
	}

	t.cleanedUp = true
	return nil
}

func modifyPgUri(pguri, adminDB, targetDB string) (adminUri, targetUri string, err error) {
	u, err := url.Parse(pguri)
	if err != nil {
		return "", "", fmt.Errorf("invalid connection string: %w", err)
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", "", fmt.Errorf("unsupported scheme %q, expected postgres:// or postgresql://", u.Scheme)
	}

	adminU := *u
	adminU.Path = "/" + adminDB
	adminUri = adminU.String()

	targetU := *u
	targetU.Path = "/" + targetDB
	targetUri = targetU.String()

	return adminUri, targetUri, nil
}

func QuoteIdentifier(name string) string {
	escaped := strings.ReplaceAll(name, `"`, `""`)
	return `"` + escaped + `"`
}

func QuoteLiteral(s string) string {
	tag := ""
	for i := 0; ; i++ {
		candidate := fmt.Sprintf("$q%d$", i)
		if !strings.Contains(s, candidate) {
			tag = candidate
			break
		}
	}
	return tag + s + tag
}
