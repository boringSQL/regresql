package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	fixturesCmd = &cobra.Command{
		Use:   "fixtures",
		Short: "Manage test fixtures",
		Long:  `Load, validate, and manage declarative test data fixtures`,
	}

	fixturesListCmd = &cobra.Command{
		Use:   "list",
		Short: "List available fixtures",
		RunE:  runFixturesList,
	}

	fixturesValidateCmd = &cobra.Command{
		Use:   "validate [fixture-name]",
		Short: "Validate fixture definitions",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runFixturesValidate,
	}

	fixturesShowCmd = &cobra.Command{
		Use:   "show <fixture-name>",
		Short: "Show fixture details and dependencies",
		Args:  cobra.ExactArgs(1),
		RunE:  runFixturesShow,
	}

	fixturesApplyCmd = &cobra.Command{
		Use:   "apply <fixture-name>",
		Short: "Apply fixture to database (for debugging/setup)",
		Args:  cobra.ExactArgs(1),
		RunE:  runFixturesApply,
	}

	fixturesDepsCmd = &cobra.Command{
		Use:   "deps [fixture-name]",
		Short: "Show dependency graph for fixture",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runFixturesDeps,
	}

	fixturesScaffoldCmd = &cobra.Command{
		Use:   "scaffold",
		Short: "Generate fixture from database schema and statistics",
		Long: `Generate a complete fixture definition from PostgreSQL metadata.

Queries pg_stats for column distributions and pg_constraint for relationships,
then generates a fixture YAML with appropriate generators.

Examples:
  # Scaffold fixtures for specific tables
  regresql fixtures scaffold --connection "$DB_URL" --tables users,orders

  # Scaffold with custom counts
  regresql fixtures scaffold --connection "$DB_URL" --tables users --counts users=500

  # Dry run (preview without writing)
  regresql fixtures scaffold --connection "$DB_URL" --tables users --dry-run`,
		RunE: runFixturesScaffold,
	}

	fixturesCwd       string
	applyForce        bool
	scaffoldConn      string
	scaffoldTables    string
	scaffoldSchema    string
	scaffoldOutput    string
	scaffoldCounts    string
	scaffoldDryRun    bool
)

func init() {
	RootCmd.AddCommand(fixturesCmd)
	fixturesCmd.AddCommand(fixturesListCmd)
	fixturesCmd.AddCommand(fixturesValidateCmd)
	fixturesCmd.AddCommand(fixturesShowCmd)
	fixturesCmd.AddCommand(fixturesApplyCmd)
	fixturesCmd.AddCommand(fixturesDepsCmd)
	fixturesCmd.AddCommand(fixturesScaffoldCmd)

	fixturesCmd.PersistentFlags().StringVarP(&fixturesCwd, "cwd", "C", ".", "Change to Directory")
	fixturesApplyCmd.Flags().BoolVar(&applyForce, "force", false, "Truncate tables before applying fixture")

	// Scaffold flags
	fixturesScaffoldCmd.Flags().StringVar(&scaffoldConn, "connection", "", "PostgreSQL connection string (required)")
	fixturesScaffoldCmd.Flags().StringVar(&scaffoldTables, "tables", "", "Comma-separated list of tables to scaffold (required)")
	fixturesScaffoldCmd.Flags().StringVar(&scaffoldSchema, "schema", "public", "Schema name for unqualified tables")
	fixturesScaffoldCmd.Flags().StringVar(&scaffoldOutput, "output", "", "Output file path (default: regresql/fixtures/scaffold.yaml)")
	fixturesScaffoldCmd.Flags().StringVar(&scaffoldCounts, "counts", "", "Custom row counts (e.g., users=100,orders=500)")
	fixturesScaffoldCmd.Flags().BoolVar(&scaffoldDryRun, "dry-run", false, "Preview generated YAML without writing")
	fixturesScaffoldCmd.MarkFlagRequired("connection")
	fixturesScaffoldCmd.MarkFlagRequired("tables")
}

func runFixturesList(cmd *cobra.Command, args []string) error {
	if err := checkDirectory(fixturesCwd); err != nil {
		return err
	}
	root := fixturesCwd

	config, err := regresql.ReadConfig(root)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	db, err := regresql.OpenDB(config.PgUri)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	fm, err := regresql.NewFixtureManager(root, db)
	if err != nil {
		return err
	}

	if err := fm.LoadFixtures(); err != nil {
		return err
	}

	fixtures := fm.ListFixtures()
	if len(fixtures) == 0 {
		fmt.Println("No fixtures found")
		return nil
	}

	fmt.Printf("Found %d fixture(s):\n\n", len(fixtures))
	for _, name := range fixtures {
		fixture, _ := fm.GetFixture(name)
		status := "✓"
		if err := fixture.Validate(); err != nil {
			status = "✗"
		}
		fmt.Printf("  %s %s", status, name)
		if fixture.Description != "" {
			fmt.Printf(" - %s", fixture.Description)
		}
		fmt.Println()
	}

	return nil
}

func runFixturesValidate(cmd *cobra.Command, args []string) error {
	if err := checkDirectory(fixturesCwd); err != nil {
		return err
	}
	root := fixturesCwd

	config, err := regresql.ReadConfig(root)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	db, err := regresql.OpenDB(config.PgUri)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	fm, err := regresql.NewFixtureManager(root, db)
	if err != nil {
		return err
	}

	var fixturesToValidate []string
	if len(args) == 1 {
		fixturesToValidate = []string{args[0]}
	} else {
		if err := fm.LoadFixtures(); err != nil {
			return err
		}
		fixturesToValidate = fm.ListFixtures()
	}

	hasErrors := false
	for _, name := range fixturesToValidate {
		fixture, err := fm.LoadFixture(name)
		if err != nil {
			fmt.Printf("✗ %s.yaml - error: %v\n", name, err)
			hasErrors = true
			continue
		}

		if err := fixture.Validate(); err != nil {
			fmt.Printf("✗ %s.yaml - error: %v\n", name, err)
			hasErrors = true
		} else {
			fmt.Printf("✓ %s.yaml - valid\n", name)
		}
	}

	if hasErrors {
		os.Exit(1)
	}

	return nil
}

func runFixturesShow(cmd *cobra.Command, args []string) error {
	if err := checkDirectory(fixturesCwd); err != nil {
		return err
	}
	root := fixturesCwd

	config, err := regresql.ReadConfig(root)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	db, err := regresql.OpenDB(config.PgUri)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	fm, err := regresql.NewFixtureManager(root, db)
	if err != nil {
		return err
	}

	fixture, err := fm.LoadFixture(args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Fixture: %s\n", fixture.Name)
	if fixture.Description != "" {
		fmt.Printf("Description: %s\n", fixture.Description)
	}

	if len(fixture.DependsOn) > 0 {
		fmt.Println("Dependencies:")
		for _, dep := range fixture.DependsOn {
			fmt.Printf("  → %s\n", dep)
		}
	}

	if len(fixture.Data) > 0 {
		fmt.Println("\nStatic Data:")
		for _, td := range fixture.Data {
			fmt.Printf("  • %s (%d rows)\n", td.Table, len(td.Rows))
		}
	}

	if len(fixture.Generate) > 0 {
		fmt.Println("\nGenerated Data:")
		for _, gs := range fixture.Generate {
			fmt.Printf("  • %s (%d rows)\n", gs.Table, gs.Count)
		}
	}

	return nil
}

func runFixturesApply(cmd *cobra.Command, args []string) error {
	if err := checkDirectory(fixturesCwd); err != nil {
		return err
	}
	root := fixturesCwd

	config, err := regresql.ReadConfig(root)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	db, err := regresql.OpenDB(config.PgUri)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	fm, err := regresql.NewFixtureManager(root, db)
	if err != nil {
		return err
	}

	if err := fm.IntrospectSchema(); err != nil {
		return err
	}

	fixtureName := args[0]
	fixture, err := fm.LoadFixture(fixtureName)
	if err != nil {
		return err
	}

	fmt.Printf("Loading fixture: %s\n", fixtureName)

	// Truncate tables if --force is set
	if applyForce {
		tables := fixture.GetTables()
		if len(tables) > 0 {
			fmt.Printf("Truncating tables: %v\n", tables)
			for _, table := range tables {
				query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
				if _, err := db.Exec(query); err != nil {
					return fmt.Errorf("failed to truncate table '%s': %w", table, err)
				}
			}
		}
	}

	if err := fm.BeginTransaction(); err != nil {
		return err
	}

	if err := fm.ApplyFixtures([]string{fixtureName}); err != nil {
		fm.Rollback()
		errMsg := err.Error()

		// Check for GENERATED ALWAYS AS IDENTITY conflict
		if containsAny(errMsg, []string{"cannot insert a non-DEFAULT value", "SQLSTATE 428C9"}) {
			cmd.SilenceUsage = true
			fmt.Fprintln(os.Stderr, "\nError: Cannot insert explicit ID into GENERATED ALWAYS AS IDENTITY column")
			fmt.Fprintln(os.Stderr, "\nThis fixture provides explicit IDs, but your schema uses GENERATED ALWAYS AS IDENTITY.")
			fmt.Fprintln(os.Stderr, "\nSuggestions:")
			fmt.Fprintln(os.Stderr, "  - Use a fixture without explicit IDs (e.g., users_identity.yaml)")
			fmt.Fprintln(os.Stderr, "  - Change schema to GENERATED BY DEFAULT AS IDENTITY")
			fmt.Fprintln(os.Stderr, "  - Use SQL fixture with OVERRIDING SYSTEM VALUE")
			fmt.Fprintln(os.Stderr, "")
			os.Exit(1)
		}

		// Check for duplicate key error
		if containsAny(errMsg, []string{"duplicate key", "unique constraint", "SQLSTATE 23505"}) {
			cmd.SilenceUsage = true
			fmt.Fprintln(os.Stderr, "\nError: Fixture data already exists (duplicate key violation)")
			fmt.Fprintln(os.Stderr, "\nSuggestions:")
			fmt.Fprintln(os.Stderr, "  - Use --force to truncate tables and reload")
			fmt.Fprintf(os.Stderr, "  - Manual cleanup: TRUNCATE %s CASCADE;\n", joinTables(fixture.GetTables()))
			fmt.Fprintln(os.Stderr, "")
			os.Exit(1)
		}

		return err
	}

	for _, td := range fixture.Data {
		fmt.Printf("  ✓ Inserted %d rows into %s\n", len(td.Rows), td.Table)
	}

	for _, gs := range fixture.Generate {
		fmt.Printf("  ✓ Generated %d rows into %s\n", gs.Count, gs.Table)
	}

	if err := fm.Commit(); err != nil {
		return err
	}

	fmt.Println("Fixture applied successfully")

	return nil
}

func runFixturesDeps(cmd *cobra.Command, args []string) error {
	if err := checkDirectory(fixturesCwd); err != nil {
		return err
	}
	root := fixturesCwd

	config, err := regresql.ReadConfig(root)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	db, err := regresql.OpenDB(config.PgUri)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	fm, err := regresql.NewFixtureManager(root, db)
	if err != nil {
		return err
	}

	if err := fm.LoadFixtures(); err != nil {
		return err
	}

	var fixturesToShow []string
	if len(args) == 1 {
		fixturesToShow = []string{args[0]}
	} else {
		fixturesToShow = fm.ListFixtures()
	}

	for _, name := range fixturesToShow {
		fixture, err := fm.GetFixture(name)
		if err != nil {
			return err
		}

		fmt.Println(name)
		printDeps(fm, fixture, "  ", make(map[string]bool))
		fmt.Println()
	}

	return nil
}

func printDeps(fm *regresql.FixtureManager, fixture *regresql.Fixture, indent string, visited map[string]bool) {
	if len(fixture.DependsOn) == 0 {
		return
	}

	for i, depName := range fixture.DependsOn {
		isLast := i == len(fixture.DependsOn)-1
		prefix := "├─"
		if isLast {
			prefix = "└─"
		}

		fmt.Printf("%s%s %s\n", indent, prefix, depName)

		if visited[depName] {
			fmt.Printf("%s   (circular reference)\n", indent)
			continue
		}

		dep, err := fm.GetFixture(depName)
		if err != nil {
			continue
		}

		newVisited := make(map[string]bool)
		for k, v := range visited {
			newVisited[k] = v
		}
		newVisited[depName] = true

		newIndent := indent + "   "
		if !isLast {
			newIndent = indent + "│  "
		}

		printDeps(fm, dep, newIndent, newVisited)
	}
}

// containsAny checks if the string contains any of the substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// joinTables joins table names for display in error messages
func joinTables(tables []string) string {
	if len(tables) == 0 {
		return ""
	}
	result := tables[0]
	for i := 1; i < len(tables); i++ {
		result += ", " + tables[i]
	}
	return result
}

func runFixturesScaffold(cmd *cobra.Command, args []string) error {
	// Parse tables
	tables := parseCommaSeparated(scaffoldTables)
	if len(tables) == 0 {
		return fmt.Errorf("--tables is required")
	}

	// Parse counts
	counts := make(map[string]int)
	if scaffoldCounts != "" {
		var err error
		counts, err = parseCounts(scaffoldCounts)
		if err != nil {
			return fmt.Errorf("invalid --counts: %w", err)
		}
	}

	// Connect to database
	db, err := regresql.OpenDB(scaffoldConn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Create scaffolder
	options := &regresql.ScaffoldOptions{
		Connection: scaffoldConn,
		Tables:     tables,
		Schema:     scaffoldSchema,
		Output:     scaffoldOutput,
		Counts:     counts,
		DryRun:     scaffoldDryRun,
	}

	scaffolder := regresql.NewScaffolder(db, options)
	result, err := scaffolder.Scaffold()
	if err != nil {
		return fmt.Errorf("scaffold failed: %w", err)
	}

	// Print warnings
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Dry run - just print
	if scaffoldDryRun {
		fmt.Println(result.YAML)
		return nil
	}

	// Determine output path
	outputPath := scaffoldOutput
	if outputPath == "" {
		if err := checkDirectory(fixturesCwd); err != nil {
			return err
		}
		outputPath = filepath.Join(fixturesCwd, "regresql", "fixtures", "scaffold.yaml")
	}

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(outputPath, []byte(result.YAML), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Scaffold written to: %s\n", outputPath)
	fmt.Printf("Tables scaffolded: %d\n", len(tables))

	if len(result.Warnings) > 0 {
		fmt.Printf("\nReview %d warning(s) above and fix TODOs in the generated file.\n", len(result.Warnings))
	}

	return nil
}

func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, p := range splitComma(s) {
		p = trimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitComma(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
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

func parseCounts(s string) (map[string]int, error) {
	result := make(map[string]int)
	pairs := parseCommaSeparated(s)
	for _, pair := range pairs {
		eqIdx := -1
		for i := 0; i < len(pair); i++ {
			if pair[i] == '=' {
				eqIdx = i
				break
			}
		}
		if eqIdx == -1 {
			return nil, fmt.Errorf("invalid count format %q, expected table=count", pair)
		}
		table := pair[:eqIdx]
		countStr := pair[eqIdx+1:]
		var count int
		if _, err := fmt.Sscanf(countStr, "%d", &count); err != nil {
			return nil, fmt.Errorf("invalid count %q for table %q", countStr, table)
		}
		if count <= 0 {
			return nil, fmt.Errorf("count must be positive for table %q", table)
		}
		result[table] = count
	}
	return result, nil
}
