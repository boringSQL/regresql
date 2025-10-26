package regresql

type (
	Fixture struct {
		Name        string          `yaml:"fixture" json:"fixture"`
		Description string          `yaml:"description,omitempty" json:"description,omitempty"`
		Cleanup     CleanupStrategy `yaml:"cleanup" json:"cleanup"`
		DependsOn   []string        `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
		Data        []TableData     `yaml:"data,omitempty" json:"data,omitempty"`
		Generate    []GenerateSpec  `yaml:"generate,omitempty" json:"generate,omitempty"`
		SQL         []SQLSpec       `yaml:"sql,omitempty" json:"sql,omitempty"`
	}

	CleanupStrategy string

	TableData struct {
		Table string                   `yaml:"table" json:"table"`
		Rows  []map[string]interface{} `yaml:"rows" json:"rows"`
	}

	GenerateSpec struct {
		Table   string                   `yaml:"table" json:"table"`
		Count   int                      `yaml:"count" json:"count"`
		Columns map[string]GeneratorSpec `yaml:"columns" json:"columns"`
	}

	GeneratorSpec struct {
		Generator string                 `yaml:"generator" json:"generator"`
		Params    map[string]interface{} `yaml:",inline" json:",inline"`
	}

	SQLSpec struct {
		File   string `yaml:"file,omitempty" json:"file,omitempty"`
		Inline string `yaml:"inline,omitempty" json:"inline,omitempty"`
	}
)

const (
	CleanupTruncate CleanupStrategy = "truncate"
	CleanupRollback CleanupStrategy = "rollback"
	CleanupNone     CleanupStrategy = "none"
)

// Validate checks if the fixture definition is valid
func (f *Fixture) Validate() error {
	if f.Name == "" {
		return ErrInvalidFixture("fixture name is required")
	}

	// Validate cleanup strategy
	switch f.Cleanup {
	case CleanupTruncate, CleanupRollback, CleanupNone, "":
		// Valid strategies (empty defaults to rollback)
	default:
		return ErrInvalidFixture("invalid cleanup strategy: %s (must be truncate, rollback, or none)", f.Cleanup)
	}

	if len(f.Data) == 0 && len(f.Generate) == 0 && len(f.SQL) == 0 {
		return ErrInvalidFixture("fixture must specify 'data', 'generate', or 'sql'")
	}

	// Validate table data
	for i, td := range f.Data {
		if td.Table == "" {
			return ErrInvalidFixture("data[%d]: table name is required", i)
		}
		if len(td.Rows) == 0 {
			return ErrInvalidFixture("data[%d]: at least one row is required", i)
		}
	}

	// Validate generate specs
	for i, gs := range f.Generate {
		if gs.Table == "" {
			return ErrInvalidFixture("generate[%d]: table name is required", i)
		}
		if gs.Count <= 0 {
			return ErrInvalidFixture("generate[%d]: count must be positive", i)
		}
		if len(gs.Columns) == 0 {
			return ErrInvalidFixture("generate[%d]: at least one column must be specified", i)
		}
		for col, spec := range gs.Columns {
			if spec.Generator == "" {
				return ErrInvalidFixture("generate[%d].columns.%s: generator is required", i, col)
			}
		}
	}

	for i, sqlSpec := range f.SQL {
		if sqlSpec.File == "" && sqlSpec.Inline == "" {
			return ErrInvalidFixture("sql[%d]: must specify either 'file' or 'inline'", i)
		}
		if sqlSpec.File != "" && sqlSpec.Inline != "" {
			return ErrInvalidFixture("sql[%d]: cannot specify both 'file' and 'inline'", i)
		}
	}

	return nil
}

// GetCleanup returns the cleanup strategy, defaulting to rollback
func (f *Fixture) GetCleanup() CleanupStrategy {
	if f.Cleanup == "" {
		return CleanupRollback
	}
	return f.Cleanup
}

// GetTables returns all tables referenced in this fixture
func (f *Fixture) GetTables() []string {
	tables := make(map[string]bool)

	// Add tables from static data
	for _, td := range f.Data {
		tables[td.Table] = true
	}

	// Add tables from generate specs
	for _, gs := range f.Generate {
		tables[gs.Table] = true
	}

	// Convert to slice
	result := make([]string, 0, len(tables))
	for table := range tables {
		result = append(result, table)
	}

	return result
}
