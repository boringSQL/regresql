package regresql

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FixtureManager handles loading, dependency resolution, and execution of fixtures
type FixtureManager struct {
	root       string
	db         *sql.DB
	tx         *sql.Tx
	fixtures   map[string]*Fixture
	generators *GeneratorRegistry
	schema     *DatabaseSchema
}

// NewFixtureManager creates a new fixture manager
func NewFixtureManager(root string, db *sql.DB) (*FixtureManager, error) {
	fm := &FixtureManager{
		root:       root,
		db:         db,
		fixtures:   make(map[string]*Fixture),
		generators: NewGeneratorRegistry(),
	}

	// Register built-in generators
	if err := fm.registerBuiltinGenerators(); err != nil {
		return nil, fmt.Errorf("failed to register built-in generators: %w", err)
	}

	return fm, nil
}

func (fm *FixtureManager) registerBuiltinGenerators() error {
	basicGens := []Generator{
		NewSequenceGenerator(),
		NewIntGenerator(),
		NewStringGenerator(),
		NewUUIDGenerator(),
		NewEmailGenerator(),
		NewNameGenerator(),
		NewNowGenerator(),
		NewDateBetweenGenerator(),
		NewDecimalGenerator(),
		NewRangeGenerator(),
	}

	for _, gen := range basicGens {
		if err := fm.generators.Register(gen); err != nil {
			return err
		}
	}

	advancedGens := []Generator{
		NewForeignKeyGenerator(fm.db),
		NewPatternGenerator(fm.generators),
		NewTemplateGenerator(fm.generators),
	}

	for _, gen := range advancedGens {
		if err := fm.generators.Register(gen); err != nil {
			return err
		}
	}

	return nil
}

// LoadFixture loads a fixture definition from YAML
func (fm *FixtureManager) LoadFixture(name string) (*Fixture, error) {
	// Check if already loaded
	if fixture, exists := fm.fixtures[name]; exists {
		return fixture, nil
	}

	// Construct path to fixture file
	fixturePath := filepath.Join(fm.root, "regresql", "fixtures", name+".yaml")

	// Read fixture file
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFixtureNotFound(name)
		}
		return nil, fmt.Errorf("failed to read fixture file: %w", err)
	}

	// Parse YAML
	var fixture Fixture
	if err := yaml.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fixture: %w", err)
	}

	// Validate fixture
	if err := fixture.Validate(); err != nil {
		return nil, err
	}

	// Cache the fixture
	fm.fixtures[name] = &fixture

	return &fixture, nil
}

// LoadFixtures loads all fixtures from the fixtures directory
func (fm *FixtureManager) LoadFixtures() error {
	fixturesDir := filepath.Join(fm.root, "regresql", "fixtures")

	// Check if fixtures directory exists
	if _, err := os.Stat(fixturesDir); os.IsNotExist(err) {
		// No fixtures directory, that's okay
		return nil
	}

	// Walk the fixtures directory
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		return fmt.Errorf("failed to read fixtures directory: %w", err)
	}

	// Load each YAML file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .yaml and .yml files
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		// Get fixture name (filename without extension)
		name := entry.Name()[:len(entry.Name())-len(ext)]

		// Load the fixture
		if _, err := fm.LoadFixture(name); err != nil {
			return fmt.Errorf("failed to load fixture '%s': %w", name, err)
		}
	}

	return nil
}

// ResolveDependencies orders fixtures based on dependencies
// Returns fixtures in the order they should be applied
func (fm *FixtureManager) ResolveDependencies(fixtureNames []string) ([]*Fixture, error) {
	// Load all requested fixtures
	fixtures := make(map[string]*Fixture)
	for _, name := range fixtureNames {
		fixture, err := fm.LoadFixture(name)
		if err != nil {
			return nil, err
		}
		fixtures[name] = fixture
	}

	// Build dependency graph and perform topological sort
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var result []*Fixture
	var path []string

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}

		if visiting[name] {
			// Circular dependency detected
			path = append(path, name)
			return ErrCircularDependency(path)
		}

		visiting[name] = true
		path = append(path, name)

		fixture := fixtures[name]
		if fixture == nil {
			// Load dependency if not already loaded
			var err error
			fixture, err = fm.LoadFixture(name)
			if err != nil {
				return err
			}
			fixtures[name] = fixture
		}

		// Visit dependencies first
		for _, dep := range fixture.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}

		visiting[name] = false
		visited[name] = true
		result = append(result, fixture)
		path = path[:len(path)-1]

		return nil
	}

	// Visit all requested fixtures
	for _, name := range fixtureNames {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// BeginTransaction starts a new transaction for fixture application
func (fm *FixtureManager) BeginTransaction() error {
	if fm.tx != nil {
		return fmt.Errorf("transaction already in progress")
	}

	tx, err := fm.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	fm.tx = tx
	return nil
}

// ApplyFixtures loads and applies fixtures in dependency order within a transaction
func (fm *FixtureManager) ApplyFixtures(fixtureNames []string) error {
	if len(fixtureNames) == 0 {
		return nil
	}

	// Resolve dependencies
	fixtures, err := fm.ResolveDependencies(fixtureNames)
	if err != nil {
		return err
	}

	// Ensure we have a transaction
	if fm.tx == nil {
		if err := fm.BeginTransaction(); err != nil {
			return err
		}
	}

	// Apply each fixture in order
	for _, fixture := range fixtures {
		if err := fm.applyFixture(fixture); err != nil {
			return ErrFixtureApplication(fixture.Name, err)
		}
	}

	return nil
}

func (fm *FixtureManager) applyFixture(fixture *Fixture) error {
	for _, sqlSpec := range fixture.SQL {
		if err := fm.executeSQL(sqlSpec); err != nil {
			return fmt.Errorf("failed to execute SQL: %w", err)
		}
	}

	for _, tableData := range fixture.Data {
		if err := fm.insertTableData(tableData); err != nil {
			return fmt.Errorf("failed to insert data into table '%s': %w", tableData.Table, err)
		}
	}

	for _, genSpec := range fixture.Generate {
		if err := fm.generateTableData(genSpec); err != nil {
			return fmt.Errorf("failed to generate data for table '%s': %w", genSpec.Table, err)
		}
	}

	return nil
}

func (fm *FixtureManager) executeSQL(sqlSpec SQLSpec) error {
	var sqlContent string

	if sqlSpec.File != "" {
		filePath := filepath.Join(fm.root, "regresql", "fixtures", sqlSpec.File)
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read SQL file '%s': %w", sqlSpec.File, err)
		}
		sqlContent = string(data)
	} else {
		sqlContent = sqlSpec.Inline
	}

	if _, err := fm.tx.Exec(sqlContent); err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}
	return nil
}

func (fm *FixtureManager) insertTableData(tableData TableData) error {
	if len(tableData.Rows) == 0 {
		return nil
	}

	// Get columns from first row (assuming all rows have same columns)
	firstRow := tableData.Rows[0]
	columns := make([]string, 0, len(firstRow))
	for col := range firstRow {
		columns = append(columns, col)
	}

	// Build INSERT statement
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES ", tableData.Table, joinColumns(columns))

	// Build value placeholders and collect values
	var values []any
	valuePlaceholders := make([]string, 0, len(tableData.Rows))

	for i, row := range tableData.Rows {
		rowValues := make([]string, len(columns))
		for j, col := range columns {
			values = append(values, row[col])
			rowValues[j] = fmt.Sprintf("$%d", len(values))
		}
		valuePlaceholders = append(valuePlaceholders, fmt.Sprintf("(%s)", joinStrings(rowValues, ", ")))

		// PostgreSQL has a limit on the number of parameters (typically 65535)
		// Insert in batches if needed
		if (i+1)%1000 == 0 || i == len(tableData.Rows)-1 {
			batchQuery := query + joinStrings(valuePlaceholders, ", ")
			if _, err := fm.tx.Exec(batchQuery, values...); err != nil {
				return err
			}
			// Reset for next batch
			values = nil
			valuePlaceholders = nil
		}
	}

	return nil
}

// generateTableData generates and inserts data based on generation spec
func (fm *FixtureManager) generateTableData(genSpec GenerateSpec) error {
	if fm.schema == nil {
		return fmt.Errorf("schema not introspected - call IntrospectSchema first")
	}

	// Set FK generator to use transaction if available
	if fkGen, err := fm.generators.Get("fk"); err == nil {
		if fk, ok := fkGen.(*ForeignKeyGenerator); ok {
			if fm.tx != nil {
				fk.SetQuerier(fm.tx)
			} else {
				fk.SetQuerier(fm.db)
			}
		}
	}

	// Get table info from schema
	tableInfo, err := fm.schema.GetTable(genSpec.Table)
	if err != nil {
		return err
	}

	// Auto-detect foreign keys for columns without explicit generators
	if err := fm.autoDetectForeignKeys(&genSpec, tableInfo); err != nil {
		return fmt.Errorf("failed to auto-detect foreign keys: %w", err)
	}

	// Validate all generators exist
	for colName, colGenSpec := range genSpec.Columns {
		if !fm.generators.Has(colGenSpec.Generator) {
			return ErrGeneratorNotFound(colGenSpec.Generator)
		}

		// Validate generator params
		colInfo := tableInfo.Columns[colName]
		if colInfo == nil {
			return fmt.Errorf("column '%s' not found in table '%s'", colName, genSpec.Table)
		}

		gen, _ := fm.generators.Get(colGenSpec.Generator)
		if err := gen.Validate(colGenSpec.Params, colInfo); err != nil {
			return fmt.Errorf("invalid params for generator '%s' on column '%s': %w", colGenSpec.Generator, colName, err)
		}
	}

	// Generate and insert data in batches
	const batchSize = 1000
	for i := 0; i < genSpec.Count; i += batchSize {
		count := batchSize
		if i+count > genSpec.Count {
			count = genSpec.Count - i
		}

		if err := fm.generateAndInsertBatch(genSpec, tableInfo, count); err != nil {
			return fmt.Errorf("failed to generate batch at row %d: %w", i, err)
		}
	}

	return nil
}

// generateAndInsertBatch generates and inserts a batch of rows
func (fm *FixtureManager) generateAndInsertBatch(genSpec GenerateSpec, tableInfo *TableInfo, count int) error {
	// Collect column names
	columns := make([]string, 0, len(genSpec.Columns))
	for colName := range genSpec.Columns {
		columns = append(columns, colName)
	}

	// Build INSERT statement
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES ", genSpec.Table, joinColumns(columns))

	// Generate rows
	var values []any
	valuePlaceholders := make([]string, 0, count)

	for i := 0; i < count; i++ {
		rowValues := make([]string, len(columns))

		for j, colName := range columns {
			genSpecCol := genSpec.Columns[colName]
			colInfo := tableInfo.Columns[colName]

			gen, err := fm.generators.Get(genSpecCol.Generator)
			if err != nil {
				return err
			}

			value, err := gen.Generate(genSpecCol.Params, colInfo)
			if err != nil {
				return fmt.Errorf("failed to generate value for column '%s' (row %d): %w", colName, i, err)
			}

			values = append(values, value)
			rowValues[j] = fmt.Sprintf("$%d", len(values))
		}

		valuePlaceholders = append(valuePlaceholders, fmt.Sprintf("(%s)", joinStrings(rowValues, ", ")))
	}

	// Execute insert
	finalQuery := query + joinStrings(valuePlaceholders, ", ")
	if _, err := fm.tx.Exec(finalQuery, values...); err != nil {
		return err
	}

	return nil
}

// Cleanup performs cleanup based on the fixture's cleanup strategy
func (fm *FixtureManager) Cleanup(fixture *Fixture) error {
	cleanup := fixture.GetCleanup()

	switch cleanup {
	case CleanupRollback:
		return fm.Rollback()

	case CleanupTruncate:
		// Truncate all tables used by this fixture
		tables := fixture.GetTables()
		for _, table := range tables {
			query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
			if _, err := fm.tx.Exec(query); err != nil {
				return fmt.Errorf("failed to truncate table '%s': %w", table, err)
			}
		}
		return fm.Commit()

	case CleanupNone:
		return fm.Commit()

	default:
		return fmt.Errorf("unknown cleanup strategy: %s", cleanup)
	}
}

// Rollback rolls back the current transaction
func (fm *FixtureManager) Rollback() error {
	if fm.tx == nil {
		return nil
	}

	err := fm.tx.Rollback()
	fm.tx = nil
	return err
}

// Commit commits the current transaction
func (fm *FixtureManager) Commit() error {
	if fm.tx == nil {
		return nil
	}

	err := fm.tx.Commit()
	fm.tx = nil
	return err
}

// IntrospectSchema introspects the database schema
func (fm *FixtureManager) IntrospectSchema() error {
	schema, err := IntrospectSchema(fm.db)
	if err != nil {
		return ErrSchemaIntrospection(err)
	}
	fm.schema = schema
	return nil
}

// GetSchema returns the introspected database schema
func (fm *FixtureManager) GetSchema() *DatabaseSchema {
	return fm.schema
}

// ListFixtures returns names of all loaded fixtures
func (fm *FixtureManager) ListFixtures() []string {
	names := make([]string, 0, len(fm.fixtures))
	for name := range fm.fixtures {
		names = append(names, name)
	}
	return names
}

// GetFixture returns a loaded fixture by name
func (fm *FixtureManager) GetFixture(name string) (*Fixture, error) {
	if fixture, exists := fm.fixtures[name]; exists {
		return fixture, nil
	}
	return nil, ErrFixtureNotFound(name)
}

// autoDetectForeignKeys automatically adds FK generators for columns that are foreign keys
func (fm *FixtureManager) autoDetectForeignKeys(genSpec *GenerateSpec, tableInfo *TableInfo) error {
	if genSpec.Columns == nil {
		genSpec.Columns = make(map[string]GeneratorSpec)
	}

	for _, fk := range tableInfo.ForeignKeys {
		// Skip if user already specified a generator for this column
		if _, hasGenerator := genSpec.Columns[fk.ColumnName]; hasGenerator {
			continue
		}

		// Skip nullable FKs (let them be NULL)
		colInfo := tableInfo.Columns[fk.ColumnName]
		if colInfo != nil && colInfo.IsNullable {
			continue
		}

		// Auto-add FK generator
		genSpec.Columns[fk.ColumnName] = GeneratorSpec{
			Generator: "fk",
			Params: map[string]any{
				"table":    fk.ReferencedTable,
				"column":   fk.ReferencedColumn,
				"strategy": "random",
			},
		}
	}

	return nil
}

// Helper functions

func joinColumns(columns []string) string {
	return joinStrings(columns, ", ")
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}
