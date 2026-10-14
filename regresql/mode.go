package regresql

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type (
	Mode string

	EnvStatus struct {
		Mode       Mode
		Connected  bool
		PgRegresql string
		GUCs       string
		PGMajor    string
		Tripwire   bool
		ProbeError string
		Detail     []string
	}
)

const (
	ModeBuffers    Mode = "buffers"
	ModePlanChoice Mode = "plan_choice"
)

const (
	envLoaded   = "loaded"
	envMissing  = "missing"
	envUnknown  = "unknown"
	envMatch    = "match"
	envMismatch = "mismatch"
	envGUCsNot  = "not"
)

const (
	statsHeaderScanLines = 20
	envProbeTimeout      = 10 * time.Second
)

const installHint = "install pg_regresql into $libdir (or $libdir/plugins for non-superusers); see pg_ext/"

var statsHeaderRe = regexp.MustCompile(`^-- Dumped from database version (\d+)`)

func CurrentMode() Mode {
	if injectedStats {
		return ModePlanChoice
	}
	return ModeBuffers
}

func (m Mode) Label() string {
	if m == ModePlanChoice {
		return "plan choice (injected stats)"
	}
	return "buffers"
}

func (e EnvStatus) AbortRun() bool {
	return e.Mode == ModePlanChoice && e.PgRegresql == envMissing
}

func (e EnvStatus) Failed() bool {
	return e.AbortRun() || e.Tripwire || e.PGMajor == envMismatch
}

func (e EnvStatus) AbortMessage() string {
	return fmt.Sprintf("Error: stats are configured but pg_regresql could not be loaded: %s\n%s", e.ProbeError, installHint)
}

func (e EnvStatus) Fields() map[string]any {
	return map[string]any{
		"mode":        string(e.Mode),
		"mode_label":  e.Mode.Label(),
		"pg_regresql": e.PgRegresql,
		"gucs":        e.GUCs,
		"pg_major":    e.PGMajor,
	}
}

func writeEnvHeader(w io.Writer, e EnvStatus) {
	fmt.Fprintf(w, "mode: %s | pg_regresql: %s | GUCs: %s | PG major: %s\n",
		e.Mode.Label(), e.PgRegresql, e.GUCs, e.PGMajor)
}

func statsFileMajor(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for i := 0; i < statsHeaderScanLines && sc.Scan(); i++ {
		if m := statsHeaderRe.FindStringSubmatch(sc.Text()); m != nil {
			major, _ := strconv.Atoi(m[1])
			return major
		}
	}
	return 0
}

func snapshotMajor(meta *SnapshotMetadata) int {
	if meta == nil || meta.Current == nil || meta.Current.Server == nil || meta.Current.Server.VersionNum == 0 {
		return 0
	}
	return meta.Current.Server.MajorVersion()
}

func CollectEnv(pguri, statsFile string, cfg config, root string, verify bool) EnvStatus {
	e := EnvStatus{
		Mode:       CurrentMode(),
		PgRegresql: envUnknown,
		GUCs:       envGUCsNot,
		PGMajor:    envUnknown,
		Detail:     []string{},
	}

	if e.Mode == ModePlanChoice && cfg.Analyze != nil && cfg.Analyze.Comparison == "buffers" {
		e.Detail = append(e.Detail, "warning: analyze.comparison: buffers is ignored in plan choice mode")
	}

	db, err := OpenDB(pguri)
	if err != nil {
		e.Detail = append(e.Detail, fmt.Sprintf("connection: %s", err))
		return e
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), envProbeTimeout)
	defer cancel()

	conn, err := db.Conn(ctx)
	if err != nil {
		e.Detail = append(e.Detail, fmt.Sprintf("connection: %s", err))
		return e
	}
	defer conn.Close()
	e.Connected = true

	if _, err := conn.ExecContext(ctx, "LOAD 'pg_regresql'"); err != nil {
		e.PgRegresql = envMissing
		e.ProbeError = err.Error()
		e.Detail = append(e.Detail, fmt.Sprintf("pg_regresql missing or not permitted (%s); %s", err, installHint))
	} else {
		e.PgRegresql = envLoaded
		var active string
		if err := conn.QueryRowContext(ctx, "SHOW pg_regresql.active").Scan(&active); err == nil {
			e.Detail = append(e.Detail, fmt.Sprintf("pg_regresql.active = %s", active))
		}
	}

	if sc, err := CaptureServerContext(db); err == nil {
		server := sc.MajorVersion()
		switch recorded := statsFileMajor(statsFile); {
		case statsFile == "" || recorded == 0:
			e.PGMajor = envUnknown
		case recorded == server:
			e.PGMajor = envMatch
		default:
			e.PGMajor = envMismatch
			e.Detail = append(e.Detail, fmt.Sprintf("stats file was dumped from PostgreSQL %d, server is %d", recorded, server))
		}

		if meta, err := ReadSnapshotMetadata(GetSnapshotsDir(root)); err == nil {
			if sm := snapshotMajor(meta); sm != 0 && sm != server {
				e.Detail = append(e.Detail, fmt.Sprintf("snapshot was built on PostgreSQL %d, server is %d", sm, server))
			}
		}
	} else {
		e.Detail = append(e.Detail, fmt.Sprintf("server version: %s", err))
	}

	if verify && e.Mode == ModePlanChoice && e.PgRegresql == envLoaded {
		sdb, err := openSessionDB(pguri)
		if err == nil {
			defer sdb.Close()
			err = VerifyInjection(sdb)
		}
		if err != nil {
			e.Tripwire = true
			e.Detail = append(e.Detail, fmt.Sprintf("injection check: %s", strings.TrimSpace(err.Error())))
		}
	}

	return e
}
