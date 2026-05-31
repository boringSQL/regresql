package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/boringsql/regresql/v2/regresql"
	"github.com/spf13/cobra"
)

var (
	fixturizeCmd = &cobra.Command{
		Use:   "fixturize",
		Short: "Extract and apply database fixtures",
		Long:  `Extract real data from databases and apply JSON fixtures.`,
	}

	fixturizeExtractCmd = &cobra.Command{
		Use:   "extract",
		Short: "Extract a consistent subgraph of real data from a live database",
		Long: `Extract real data from a PostgreSQL database by following foreign key
relationships from a root table query. Produces a self-contained JSON fixture
with data that satisfies all FK constraints by definition.

Examples:
  regresql fixturize extract --connection "$DB" \
    --root "organizations WHERE id = 42"

  regresql fixturize extract --connection "$DB" \
    --root "organizations ORDER BY random() LIMIT 3" \
    --limit 500

  regresql fixturize extract --connection "$DB" \
    --root "users LIMIT 5" --dry-run`,
		RunE: runFixturizeExtract,
	}

	fixturizeApplyCmd = &cobra.Command{
		Use:   "apply <fixture.json>",
		Short: "Apply a JSON fixture to a database",
		Long: `Apply a previously extracted JSON fixture to a PostgreSQL database.
Inserts rows in FK-dependency order.

If --connection is not provided, use pguri from regress.yaml.

Examples:
  regresql fixturize apply customer_12345.json
  regresql fixturize apply --force customer_12345.json
  regresql fixturize apply --connection "$OTHER_DB" customer_12345.json`,
		Args: cobra.ExactArgs(1),
		RunE: runFixturizeApply,
	}

	fzExtractConn             string
	fzExtractRoot             string
	fzExtractSchema           string
	fzExtractOutput           string
	fzExtractLimit            int
	fzExtractDepth            int
	fzExtractInclude          string
	fzExtractExclude          string
	fzExtractMask             []string
	fzExtractStatementTimeout int
	fzExtractDryRun           bool

	fzApplyConn            string
	fzApplyCwd             string
	fzApplyForce           bool
	fzApplyDryRun          bool
	fzApplyDisableTriggers bool
)

func init() {
	RootCmd.AddCommand(fixturizeCmd)
	fixturizeCmd.AddCommand(fixturizeExtractCmd)
	fixturizeCmd.AddCommand(fixturizeApplyCmd)

	// Extract flags
	fixturizeExtractCmd.Flags().StringVar(&fzExtractConn, "connection", "", "PostgreSQL connection string (required)")
	fixturizeExtractCmd.Flags().StringVar(&fzExtractRoot, "root", "", "Root table + optional WHERE/ORDER BY/LIMIT (required)")
	fixturizeExtractCmd.Flags().StringVar(&fzExtractSchema, "schema", "public", "Default schema for unqualified names")
	fixturizeExtractCmd.Flags().StringVarP(&fzExtractOutput, "output", "o", "", "Output file path (default: extracted.json)")
	fixturizeExtractCmd.Flags().IntVar(&fzExtractLimit, "limit", 0, "Max rows per child table (0 = unlimited)")
	fixturizeExtractCmd.Flags().IntVar(&fzExtractDepth, "depth", 0, "Max FK hops from root (0 = follow everything)")
	fixturizeExtractCmd.Flags().StringVar(&fzExtractInclude, "include", "", "Extra tables to include (comma-separated)")
	fixturizeExtractCmd.Flags().StringVar(&fzExtractExclude, "exclude", "", "Tables to skip (comma-separated)")
	fixturizeExtractCmd.Flags().StringArrayVar(&fzExtractMask, "mask", nil, "Mask column with SQL expression (table.column=expr, repeatable)")
	fixturizeExtractCmd.Flags().IntVar(&fzExtractStatementTimeout, "statement-timeout", 30, "Per-statement timeout in seconds")
	fixturizeExtractCmd.Flags().BoolVar(&fzExtractDryRun, "dry-run", false, "Print JSON to stdout, don't write file")
	fixturizeExtractCmd.MarkFlagRequired("connection")
	fixturizeExtractCmd.MarkFlagRequired("root")

	// Apply flags
	fixturizeApplyCmd.Flags().StringVar(&fzApplyConn, "connection", "", "PostgreSQL connection string (defaults to pguri from regress.yaml)")
	fixturizeApplyCmd.Flags().StringVarP(&fzApplyCwd, "cwd", "C", ".", "Change to directory")
	fixturizeApplyCmd.Flags().BoolVar(&fzApplyForce, "force", false, "Truncate tables before applying fixture")
	fixturizeApplyCmd.Flags().BoolVar(&fzApplyDryRun, "dry-run", false, "Show what would be done without making changes")
	fixturizeApplyCmd.Flags().BoolVar(&fzApplyDisableTriggers, "disable-triggers", false, "Disable triggers during insert (uses replica mode)")
}

func checkFixturizeBinary() error {
	if _, err := exec.LookPath("fixturize"); err != nil {
		return fmt.Errorf("fixturize binary not found in PATH\nSee https://github.com/boringSQL/fixturize for install instructions")
	}
	return nil
}

func runFixturizeExtract(cmd *cobra.Command, args []string) error {
	if err := checkFixturizeBinary(); err != nil {
		return err
	}
	fzArgs := []string{"extract", "--connection", fzExtractConn, "--root", fzExtractRoot}
	if fzExtractSchema != "" {
		fzArgs = append(fzArgs, "--schema", fzExtractSchema)
	}
	if fzExtractOutput != "" {
		fzArgs = append(fzArgs, "--output", fzExtractOutput)
	}
	if fzExtractLimit > 0 {
		fzArgs = append(fzArgs, "--limit", strconv.Itoa(fzExtractLimit))
	}
	if fzExtractDepth > 0 {
		fzArgs = append(fzArgs, "--depth", strconv.Itoa(fzExtractDepth))
	}
	if fzExtractInclude != "" {
		fzArgs = append(fzArgs, "--include", fzExtractInclude)
	}
	if fzExtractExclude != "" {
		fzArgs = append(fzArgs, "--exclude", fzExtractExclude)
	}
	for _, m := range fzExtractMask {
		fzArgs = append(fzArgs, "--mask", m)
	}
	if fzExtractStatementTimeout > 0 {
		fzArgs = append(fzArgs, "--statement-timeout", strconv.Itoa(fzExtractStatementTimeout))
	}
	if fzExtractDryRun {
		fzArgs = append(fzArgs, "--dry-run")
	}
	c := exec.Command("fixturize", fzArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func resolveApplyConnection() (string, error) {
	if fzApplyConn != "" {
		return fzApplyConn, nil
	}
	cfg, err := regresql.ReadConfig(fzApplyCwd)
	if err != nil {
		return "", fmt.Errorf("no --connection flag and cannot read config: %w", err)
	}
	if cfg.PgUri == "" {
		return "", fmt.Errorf("no --connection flag and pguri is empty in regress.yaml")
	}
	return cfg.PgUri, nil
}

func runFixturizeApply(cmd *cobra.Command, args []string) error {
	if err := checkFixturizeBinary(); err != nil {
		return err
	}
	connStr, err := resolveApplyConnection()
	if err != nil {
		return err
	}
	fzArgs := []string{"apply", "--connection", connStr}
	if fzApplyForce {
		fzArgs = append(fzArgs, "--force")
	}
	if fzApplyDryRun {
		fzArgs = append(fzArgs, "--dry-run")
	}
	if fzApplyDisableTriggers {
		fzArgs = append(fzArgs, "--disable-triggers")
	}
	fzArgs = append(fzArgs, args[0])
	c := exec.Command("fixturize", fzArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

