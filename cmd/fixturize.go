package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/boringsql/fixturize/fixturize"
	"github.com/boringsql/regresql/regresql"
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

func runFixturizeExtract(cmd *cobra.Command, args []string) error {
	db, err := fixturize.OpenDB(fzExtractConn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	options := &fixturize.ExtractOptions{
		Connection:       fzExtractConn,
		Root:             fzExtractRoot,
		Schema:           fzExtractSchema,
		Output:           fzExtractOutput,
		Limit:            fzExtractLimit,
		Depth:            fzExtractDepth,
		Include:          parseCommaSeparatedFz(fzExtractInclude),
		Exclude:          parseCommaSeparatedFz(fzExtractExclude),
		Mask:             fzExtractMask,
		StatementTimeout: fzExtractStatementTimeout,
		DryRun:           fzExtractDryRun,
	}

	if !fzExtractDryRun && fzExtractOutput != "" {
		os.Remove(fzExtractOutput)
	}

	extractor := fixturize.NewExtractor(db, options)
	result, err := extractor.Extract()
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	if fzExtractDryRun {
		fmt.Println(string(result.JSON))
		return nil
	}

	outputPath := fzExtractOutput
	if outputPath == "" {
		outputPath = "extracted.json"
	}

	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	if err := os.WriteFile(outputPath, result.JSON, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Fixture written to: %s\n", outputPath)
	return nil
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
	connStr, err := resolveApplyConnection()
	if err != nil {
		return err
	}

	db, err := fixturize.OpenDB(connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	options := &fixturize.ApplyOptions{
		Connection:      connStr,
		Fixture:         args[0],
		Force:           fzApplyForce,
		DryRun:          fzApplyDryRun,
		DisableTriggers: fzApplyDisableTriggers,
	}

	result, err := fixturize.ApplyFixtureFile(db, options)
	if err != nil {
		return err
	}

	totalRows := 0
	for _, count := range result.RowsInserted {
		totalRows += count
	}

	fmt.Printf("Applied %d table(s), %d row(s) total\n", len(result.TablesApplied), totalRows)
	return nil
}

func parseCommaSeparatedFz(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			p := trimSpaceFz(s[start:i])
			if p != "" {
				parts = append(parts, p)
			}
			start = i + 1
		}
	}
	p := trimSpaceFz(s[start:])
	if p != "" {
		parts = append(parts, p)
	}
	return parts
}

func trimSpaceFz(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
