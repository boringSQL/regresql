package cmd

import (
	"fmt"
	"os"

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

	fixturesCwd  string
	applyCleanup string
)

func init() {
	RootCmd.AddCommand(fixturesCmd)
	fixturesCmd.AddCommand(fixturesListCmd)
	fixturesCmd.AddCommand(fixturesValidateCmd)
	fixturesCmd.AddCommand(fixturesShowCmd)
	fixturesCmd.AddCommand(fixturesApplyCmd)
	fixturesCmd.AddCommand(fixturesDepsCmd)

	fixturesCmd.PersistentFlags().StringVarP(&fixturesCwd, "cwd", "C", ".", "Change to Directory")
	fixturesApplyCmd.Flags().StringVar(&applyCleanup, "cleanup", "", "Override cleanup strategy (none|rollback|truncate)")
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
	fmt.Printf("Cleanup: %s\n", fixture.GetCleanup())

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

	if applyCleanup != "" {
		fixture.Cleanup = regresql.CleanupStrategy(applyCleanup)
	}

	fmt.Printf("Loading fixture: %s\n", fixtureName)

	if err := fm.BeginTransaction(); err != nil {
		return err
	}

	if err := fm.ApplyFixtures([]string{fixtureName}); err != nil {
		fm.Rollback()
		return err
	}

	for _, td := range fixture.Data {
		fmt.Printf("  ✓ Inserted %d rows into %s\n", len(td.Rows), td.Table)
	}

	for _, gs := range fixture.Generate {
		fmt.Printf("  ✓ Generated %d rows into %s\n", gs.Count, gs.Table)
	}

	if err := fm.Cleanup(fixture); err != nil {
		return err
	}

	switch fixture.GetCleanup() {
	case regresql.CleanupNone:
		fmt.Println("Fixture applied successfully (kept in database)")
	case regresql.CleanupRollback:
		fmt.Println("Fixture applied successfully (rolled back)")
	case regresql.CleanupTruncate:
		fmt.Println("Fixture applied successfully (truncated)")
	}

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
