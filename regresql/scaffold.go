package regresql

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

type (
	// ScaffoldOptions configures fixture scaffold generation
	ScaffoldOptions struct {
		Connection string
		Tables     []string
		Schema     string
		Output     string
		Counts     map[string]int
		DryRun     bool
	}

	// ScaffoldResult contains the scaffolded fixture
	ScaffoldResult struct {
		Fixture  *Fixture
		YAML     string
		Warnings []string
	}

	// TableScaffold contains scaffold data for a single table
	TableScaffold struct {
		Table       string
		Schema      string
		Count       int
		Columns     []*ColumnScaffold
		PrimaryKey  []string
		ForeignKeys []*ForeignKeyInfo
		DependsOn   []string
	}

	// ColumnScaffold contains scaffold data for a single column
	ColumnScaffold struct {
		Name          string
		Type          string
		IsNullable    bool
		IsPrimaryKey  bool
		IsForeignKey  bool
		IsUnique      bool
		ForeignKey    *ForeignKeyInfo
		Profile       *ColumnProfile
		Generator     string
		GeneratorSpec map[string]any
		Comment       string
		Section       string // "pk", "fk", "unique", "learned", "nullable", "todo"
		tableName     string // qualified table name for constraint lookup
	}

	// ColumnProfile contains learned distribution data from pg_stats
	ColumnProfile struct {
		NullFrac         float64
		NDistinct        float64
		MostCommonVals   []string
		MostCommonFreqs  []float64
		HistogramBounds  []string
		AvgWidth         int
		Correlation      float64
	}

	// Scaffolder generates fixture scaffolds from database metadata
	Scaffolder struct {
		db               *sql.DB
		schema           *DatabaseSchema
		stats            map[string]map[string]*ColumnProfile // table -> column -> profile
		options          *ScaffoldOptions
		multiColChecks   map[string][]multiColumnCheck        // table -> list of multi-column checks
	}

	// multiColumnCheck represents a check constraint involving multiple columns
	multiColumnCheck struct {
		Name       string
		Columns    []string
		Definition string
	}
)

// NewScaffolder creates a new fixture scaffolder
func NewScaffolder(db *sql.DB, options *ScaffoldOptions) *Scaffolder {
	return &Scaffolder{
		db:             db,
		stats:          make(map[string]map[string]*ColumnProfile),
		options:        options,
		multiColChecks: make(map[string][]multiColumnCheck),
	}
}

// Scaffold generates a complete fixture definition from database metadata
func (s *Scaffolder) Scaffold() (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	// Introspect schema
	schema, err := IntrospectSchema(s.db)
	if err != nil {
		return nil, fmt.Errorf("schema introspection failed: %w", err)
	}
	s.schema = schema

	// Resolve table names (handle both schema-qualified and unqualified)
	resolvedTables, err := s.resolveTables()
	if err != nil {
		return nil, err
	}

	// Collect pg_stats and check constraints for all tables
	for _, tableName := range resolvedTables {
		tableInfo, _ := s.schema.GetTable(tableName)
		if err := s.collectStats(tableInfo.Schema, tableInfo.Name); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("could not collect stats for %s: %v", tableName, err))
		}
		if err := s.collectMultiColumnChecks(tableInfo.Schema, tableInfo.Name); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("could not collect check constraints for %s: %v", tableName, err))
		}
	}

	// Build table scaffolds with ordering
	tableScaffolds, err := s.buildTableScaffolds(resolvedTables)
	if err != nil {
		return nil, err
	}

	// Check for missing referenced tables and multi-column constraints
	for _, ts := range tableScaffolds {
		qualifiedName := ts.Schema + "." + ts.Table

		// FK warnings
		for _, fk := range ts.ForeignKeys {
			found := false
			for _, t := range resolvedTables {
				if t == fk.ReferencedTable {
					found = true
					break
				}
			}
			if !found {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Table '%s' referenced by %s.%s but not included in scaffold",
						fk.ReferencedTable, ts.Table, fk.ColumnName))
			}
		}

		// Check constraint warnings (length, pattern, multi-column)
		checkWarnings := s.getCheckConstraintWarnings(qualifiedName)
		result.Warnings = append(result.Warnings, checkWarnings...)
	}

	// Order tables by FK dependencies
	orderedTables, err := s.orderByDependencies(tableScaffolds)
	if err != nil {
		return nil, err
	}

	// Generate YAML
	result.YAML = s.generateYAML(orderedTables, result.Warnings)

	// Build fixture struct
	result.Fixture = s.buildFixture(orderedTables)

	return result, nil
}

func (s *Scaffolder) resolveTables() ([]string, error) {
	var resolved []string
	schemaPrefix := s.options.Schema
	if schemaPrefix == "" {
		schemaPrefix = "public"
	}

	for _, table := range s.options.Tables {
		var qualifiedName string
		if strings.Contains(table, ".") {
			qualifiedName = table
		} else {
			qualifiedName = schemaPrefix + "." + table
		}

		if !s.schema.HasTable(qualifiedName) {
			return nil, fmt.Errorf("table not found: %s", qualifiedName)
		}
		resolved = append(resolved, qualifiedName)
	}

	return resolved, nil
}

func (s *Scaffolder) buildTableScaffolds(tables []string) ([]*TableScaffold, error) {
	var scaffolds []*TableScaffold

	for _, tableName := range tables {
		tableInfo, err := s.schema.GetTable(tableName)
		if err != nil {
			return nil, err
		}

		ts := &TableScaffold{
			Table:       tableInfo.Name,
			Schema:      tableInfo.Schema,
			Count:       s.getCount(tableName, tableInfo.Name),
			PrimaryKey:  tableInfo.PrimaryKey,
			ForeignKeys: tableInfo.ForeignKeys,
		}

		// Build column scaffolds
		for colName, colInfo := range tableInfo.Columns {
			cs := s.buildColumnScaffold(colName, colInfo, tableName)
			ts.Columns = append(ts.Columns, cs)
		}

		// Sort columns by section order
		s.sortColumns(ts.Columns)

		// Find dependencies
		for _, fk := range tableInfo.ForeignKeys {
			// Skip self-references
			if fk.ReferencedTable == tableName {
				continue
			}
			// Check if referenced table is in our list
			for _, t := range tables {
				if t == fk.ReferencedTable {
					ts.DependsOn = append(ts.DependsOn, fk.ReferencedTable)
					break
				}
			}
		}

		scaffolds = append(scaffolds, ts)
	}

	return scaffolds, nil
}

func (s *Scaffolder) buildColumnScaffold(colName string, colInfo *ColumnInfo, tableName string) *ColumnScaffold {
	cs := &ColumnScaffold{
		Name:         colName,
		Type:         colInfo.Type,
		IsNullable:   colInfo.IsNullable,
		IsPrimaryKey: colInfo.IsPrimaryKey,
		IsForeignKey: colInfo.IsForeignKey,
		IsUnique:     colInfo.IsUnique,
		ForeignKey:   colInfo.ForeignKey,
		tableName:    tableName,
	}

	// Get profile if available
	if tableStats, ok := s.stats[tableName]; ok {
		cs.Profile = tableStats[colName]
	}

	// Determine generator based on schema + stats
	s.assignGenerator(cs)

	return cs
}

func (s *Scaffolder) assignGenerator(cs *ColumnScaffold) {
	spec := make(map[string]any)

	// Primary key - check column type to determine generator
	if cs.IsPrimaryKey {
		cs.Section = "pk"
		lowerType := strings.ToLower(cs.Type)
		switch {
		case lowerType == "uuid":
			cs.Generator = "uuid"
		case isIntegerType(lowerType):
			cs.Generator = "sequence"
		default:
			// Text or other types - use template
			cs.Generator = "template"
			spec["template"] = cs.Name + "_{{.Index}}"
		}
		cs.GeneratorSpec = spec
		return
	}

	// Foreign key
	if cs.IsForeignKey && cs.ForeignKey != nil {
		cs.Generator = "fk"
		// Preserve schema for non-public tables
		refSchema, refTable := parseTableName(cs.ForeignKey.ReferencedTable)
		if refSchema != "" && refSchema != "public" {
			spec["table"] = refSchema + "." + refTable
		} else {
			spec["table"] = refTable
		}
		spec["column"] = cs.ForeignKey.ReferencedColumn
		// Unique FK = one-to-one relationship, use sequential to avoid duplicates
		if cs.IsUnique {
			spec["strategy"] = "sequential"
		}
		cs.Section = "fk"
		cs.GeneratorSpec = spec
		return
	}

	// Unique columns
	if cs.IsUnique {
		cs.Section = "unique"
		s.assignUniqueGenerator(cs, spec)
		return
	}

	lowerType := strings.ToLower(cs.Type)

	// Skip learned distributions only for complex types
	skipLearned := lowerType == "json" || lowerType == "jsonb" || lowerType == "hstore" ||
		strings.HasPrefix(lowerType, "_") || strings.Contains(lowerType, "[]")

	// Use learned distributions from pg_stats when available
	if !skipLearned && cs.Profile != nil && s.hasLearnedDistribution(cs.Profile) {
		cs.Section = "learned"
		s.assignLearnedGenerator(cs, spec)
		return
	}

	// High null fraction - only if column is actually nullable
	if cs.IsNullable && cs.Profile != nil && cs.Profile.NullFrac > 0.5 {
		cs.Generator = "null"
		cs.Section = "nullable"
		cs.Comment = fmt.Sprintf("%.0f%% null in production", cs.Profile.NullFrac*100)
		cs.GeneratorSpec = spec
		return
	}

	// Type-based defaults
	cs.Section = "todo"
	s.assignTypeDefault(cs, spec)

	// Safety: ensure generator is always set
	if cs.Generator == "" {
		if cs.IsNullable {
			cs.Generator = "null"
			cs.Comment = "nullable column, no stats available"
		} else {
			cs.Generator = "string"
			cs.Comment = fmt.Sprintf("TODO: configure generator for type %s", cs.Type)
		}
		cs.GeneratorSpec = spec
	}
}

func (s *Scaffolder) assignUniqueGenerator(cs *ColumnScaffold, spec map[string]any) {
	lowerType := strings.ToLower(cs.Type)
	lowerName := strings.ToLower(cs.Name)

	switch {
	case isIntegerType(lowerType):
		cs.Generator = "sequence"
	case lowerType == "uuid":
		cs.Generator = "uuid"
	case strings.Contains(lowerName, "email"):
		cs.Generator = "email"
	default:
		cs.Generator = "uuid"
	}
	cs.GeneratorSpec = spec
}

func (s *Scaffolder) hasLearnedDistribution(p *ColumnProfile) bool {
	return len(p.MostCommonVals) > 0 || len(p.HistogramBounds) > 0
}

func (s *Scaffolder) assignLearnedGenerator(cs *ColumnScaffold, spec map[string]any) {
	p := cs.Profile

	// Categorical data with most_common_vals
	if len(p.MostCommonVals) > 0 && len(p.MostCommonVals) == len(p.MostCommonFreqs) {
		cs.Generator = "weighted"
		values := make(map[string]int)
		for i, val := range p.MostCommonVals {
			weight := max(1, int(p.MostCommonFreqs[i]*100))
			values[val] = weight
		}
		spec["values"] = values
		if p.NullFrac > 0 && cs.IsNullable {
			spec["null_probability"] = p.NullFrac
		}
		cs.Comment = s.formatDistributionComment(p)
		cs.GeneratorSpec = spec
		return
	}

	// Histogram for numeric/timestamp
	if len(p.HistogramBounds) > 0 {
		cs.Generator = "histogram"
		spec["bounds"] = p.HistogramBounds
		if p.NullFrac > 0 && cs.IsNullable {
			spec["null_probability"] = p.NullFrac
		}
		cs.Comment = s.formatHistogramComment(p)
		cs.GeneratorSpec = spec
		return
	}

	// Fallback to type default
	cs.Section = "todo"
	s.assignTypeDefault(cs, spec)
}

func (s *Scaffolder) formatDistributionComment(p *ColumnProfile) string {
	if len(p.MostCommonVals) == 0 {
		return ""
	}

	parts := make([]string, 0, len(p.MostCommonVals))
	for i := 0; i < len(p.MostCommonVals) && i < 4; i++ {
		parts = append(parts, fmt.Sprintf("%s %.0f%%", p.MostCommonVals[i], p.MostCommonFreqs[i]*100))
	}

	if len(p.MostCommonVals) > 4 {
		return fmt.Sprintf("Production: %s, +%d more", strings.Join(parts, ", "), len(p.MostCommonVals)-4)
	}
	return fmt.Sprintf("Production: %s", strings.Join(parts, ", "))
}

func (s *Scaffolder) formatHistogramComment(p *ColumnProfile) string {
	if len(p.HistogramBounds) < 2 {
		return ""
	}
	return fmt.Sprintf("Production range: %s to %s (%d buckets)",
		p.HistogramBounds[0], p.HistogramBounds[len(p.HistogramBounds)-1], len(p.HistogramBounds)-1)
}

func (s *Scaffolder) assignTypeDefault(cs *ColumnScaffold, spec map[string]any) {
	lowerType := strings.ToLower(cs.Type)

	switch {
	case lowerType == "json" || lowerType == "jsonb":
		if cs.IsNullable {
			cs.Generator = "null"
		} else {
			cs.Generator = "constant"
			spec["value"] = "{}"
		}

	case lowerType == "hstore":
		if cs.IsNullable {
			cs.Generator = "null"
		} else {
			cs.Generator = "constant"
			spec["value"] = ""
		}

	case strings.HasPrefix(lowerType, "_") || strings.Contains(lowerType, "[]"):
		if cs.IsNullable {
			cs.Generator = "null"
		} else {
			cs.Generator = "constant"
			spec["value"] = "{}"
		}

	case lowerType == "uuid":
		cs.Generator = "uuid"

	case lowerType == "boolean" || lowerType == "bool":
		cs.Generator = "bool"
		spec["probability"] = 0.5

	case isIntegerType(lowerType):
		cs.Generator = "int"
		spec["min"] = 1
		spec["max"] = 1000

	case strings.Contains(lowerType, "numeric") || strings.Contains(lowerType, "decimal") ||
		lowerType == "money" || lowerType == "real" || lowerType == "double precision":
		cs.Generator = "decimal"
		spec["min"] = 0.0
		spec["max"] = 1000.0
		spec["precision"] = 2

	case strings.Contains(lowerType, "timestamp"):
		cs.Generator = "now"

	case lowerType == "date":
		cs.Generator = "date_between"
		spec["start"] = "2020-01-01"
		spec["end"] = "2026-12-31"

	case strings.Contains(lowerType, "char") || lowerType == "text":
		lowerName := strings.ToLower(cs.Name)
		switch {
		case strings.Contains(lowerName, "password") || strings.Contains(lowerName, "secret") || strings.Contains(lowerName, "token"):
			cs.Generator = "constant"
			spec["value"] = "$2b$10$placeholder"
		case strings.Contains(lowerName, "email"):
			cs.Generator = "email"
		case strings.Contains(lowerName, "name") && !strings.Contains(lowerName, "file"):
			cs.Generator = "name"
		case strings.Contains(lowerName, "url") || strings.Contains(lowerName, "link"):
			cs.Generator = "template"
			spec["template"] = "https://example.com/{{.Index}}"
		default:
			cs.Generator = "string"
			spec["length"] = 32
		}

	default:
		if cs.IsNullable {
			cs.Generator = "null"
		} else {
			cs.Generator = "string"
			spec["length"] = 16
		}
	}
	cs.GeneratorSpec = spec
}

func (s *Scaffolder) sortColumns(columns []*ColumnScaffold) {
	sectionOrder := map[string]int{
		"pk":       0,
		"fk":       1,
		"unique":   2,
		"learned":  3,
		"nullable": 4,
		"todo":     5,
	}

	sort.Slice(columns, func(i, j int) bool {
		oi := sectionOrder[columns[i].Section]
		oj := sectionOrder[columns[j].Section]
		if oi != oj {
			return oi < oj
		}
		return columns[i].Name < columns[j].Name
	})
}

func (s *Scaffolder) getCount(qualifiedName, tableName string) int {
	// Check options for explicit count
	if count, ok := s.options.Counts[qualifiedName]; ok {
		return count
	}
	if count, ok := s.options.Counts[tableName]; ok {
		return count
	}
	// Default count
	return 100
}

func (s *Scaffolder) orderByDependencies(scaffolds []*TableScaffold) ([]*TableScaffold, error) {
	// Build adjacency map
	byName := make(map[string]*TableScaffold)
	for _, ts := range scaffolds {
		qualifiedName := ts.Schema + "." + ts.Table
		byName[qualifiedName] = ts
	}

	// Topological sort
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var result []*TableScaffold
	var path []string

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			path = append(path, name)
			return fmt.Errorf("circular FK dependency: %s", strings.Join(path, " -> "))
		}

		visiting[name] = true
		path = append(path, name)

		ts := byName[name]
		if ts != nil {
			for _, dep := range ts.DependsOn {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}

		visiting[name] = false
		visited[name] = true
		if ts != nil {
			result = append(result, ts)
		}
		path = path[:len(path)-1]

		return nil
	}

	for _, ts := range scaffolds {
		qualifiedName := ts.Schema + "." + ts.Table
		if err := visit(qualifiedName); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (s *Scaffolder) buildFixture(tables []*TableScaffold) *Fixture {
	fixture := &Fixture{
		Name: "scaffold",
	}

	for _, ts := range tables {
		// Use schema-qualified name for non-public schemas
		tableName := ts.Table
		if ts.Schema != "" && ts.Schema != "public" {
			tableName = ts.Schema + "." + ts.Table
		}

		genSpec := GenerateSpec{
			Table:   tableName,
			Count:   ts.Count,
			Columns: make(map[string]GeneratorSpec),
		}

		for _, cs := range ts.Columns {
			genSpec.Columns[cs.Name] = GeneratorSpec{
				Generator: cs.Generator,
				Params:    cs.GeneratorSpec,
			}
		}

		fixture.Generate = append(fixture.Generate, genSpec)
	}

	return fixture
}

func (s *Scaffolder) generateYAML(tables []*TableScaffold, warnings []string) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# regresql/fixtures/scaffold.yaml\n")
	sb.WriteString("# AUTO-GENERATED by: regresql fixture scaffold\n")
	sb.WriteString(fmt.Sprintf("# Generated: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString("#\n")
	sb.WriteString("# REVIEW CHECKLIST:\n")
	sb.WriteString("# [ ] Adjust row counts for your test scenarios\n")
	sb.WriteString("# [ ] Fix TODO items (passwords, JSON structures, etc.)\n")
	sb.WriteString("# [ ] Add business logic constraints not in schema\n")
	sb.WriteString("# [ ] Remove columns you don't need\n")
	sb.WriteString("\n")
	sb.WriteString("fixture: scaffold\n")
	sb.WriteString("\n")
	sb.WriteString("generate:\n")

	for i, ts := range tables {
		s.writeTableYAML(&sb, ts, i == 0)
	}

	// Warnings section
	if len(warnings) > 0 {
		sb.WriteString("\n")
		sb.WriteString("#============================================================\n")
		sb.WriteString("# WARNINGS\n")
		sb.WriteString("#============================================================\n")
		for _, w := range warnings {
			sb.WriteString(fmt.Sprintf("# - %s\n", w))
		}
	}

	return sb.String()
}

func (s *Scaffolder) writeTableYAML(sb *strings.Builder, ts *TableScaffold, isFirst bool) {
	if !isFirst {
		sb.WriteString("\n")
	}

	// Table header
	deps := ""
	if len(ts.DependsOn) > 0 {
		depNames := make([]string, len(ts.DependsOn))
		for i, d := range ts.DependsOn {
			_, name := parseTableName(d)
			depNames[i] = name
		}
		deps = fmt.Sprintf(" (depends on: %s)", strings.Join(depNames, ", "))
	}
	// Use schema-qualified name for non-public schemas
	tableName := ts.Table
	if ts.Schema != "" && ts.Schema != "public" {
		tableName = ts.Schema + "." + ts.Table
	}

	sb.WriteString(fmt.Sprintf("  #============================================================\n"))
	sb.WriteString(fmt.Sprintf("  # TABLE: %s%s\n", tableName, deps))
	sb.WriteString(fmt.Sprintf("  #============================================================\n"))

	sb.WriteString(fmt.Sprintf("  - table: %s\n", tableName))
	sb.WriteString(fmt.Sprintf("    count: %d\n", ts.Count))
	sb.WriteString("    columns:\n")

	currentSection := ""
	for _, cs := range ts.Columns {
		// Section header
		if cs.Section != currentSection {
			currentSection = cs.Section
			sectionName := s.getSectionName(cs.Section)
			if sectionName != "" {
				sb.WriteString(fmt.Sprintf("      #-- %s --\n", sectionName))
			}
		}

		s.writeColumnYAML(sb, cs)
	}
}

func (s *Scaffolder) getSectionName(section string) string {
	switch section {
	case "pk":
		return "Primary Key"
	case "fk":
		return "Foreign Keys"
	case "unique":
		return "Unique Columns"
	case "learned":
		return "Learned from pg_stats"
	case "nullable":
		return "Nullable (high null rate)"
	case "todo":
		return "TODO: Configure these"
	}
	return ""
}

func (s *Scaffolder) writeColumnYAML(sb *strings.Builder, cs *ColumnScaffold) {
	sb.WriteString(fmt.Sprintf("      %s:\n", cs.Name))
	// Quote "null" to avoid YAML parsing it as null value
	genName := cs.Generator
	if genName == "null" || genName == "true" || genName == "false" {
		genName = fmt.Sprintf("%q", genName)
	}
	sb.WriteString(fmt.Sprintf("        generator: %s\n", genName))

	// Write generator params
	for key, val := range cs.GeneratorSpec {
		s.writeYAMLValue(sb, key, val, 8)
	}

	// Comment
	if cs.Comment != "" {
		sb.WriteString(fmt.Sprintf("        # %s\n", cs.Comment))
	}
}

func (s *Scaffolder) writeYAMLValue(sb *strings.Builder, key string, val any, indent int) {
	indentStr := strings.Repeat(" ", indent)

	switch v := val.(type) {
	case map[string]int:
		sb.WriteString(fmt.Sprintf("%s%s:\n", indentStr, key))
		for k, vv := range v {
			quotedKey := quoteYAMLKey(k)
			sb.WriteString(fmt.Sprintf("%s  %s: %d\n", indentStr, quotedKey, vv))
		}
	case []string:
		sb.WriteString(fmt.Sprintf("%s%s: [", indentStr, key))
		for i, item := range v {
			if i > 0 {
				sb.WriteString(", ")
			}
			// Quote strings that might be parsed as other types
			if needsQuoting(item) {
				sb.WriteString(fmt.Sprintf("%q", item))
			} else {
				sb.WriteString(item)
			}
		}
		sb.WriteString("]\n")
	case string:
		if needsQuoting(v) {
			sb.WriteString(fmt.Sprintf("%s%s: %q\n", indentStr, key, v))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", indentStr, key, v))
		}
	case float64:
		sb.WriteString(fmt.Sprintf("%s%s: %g\n", indentStr, key, v))
	case int:
		sb.WriteString(fmt.Sprintf("%s%s: %d\n", indentStr, key, v))
	case int64:
		sb.WriteString(fmt.Sprintf("%s%s: %d\n", indentStr, key, v))
	case bool:
		sb.WriteString(fmt.Sprintf("%s%s: %t\n", indentStr, key, v))
	default:
		sb.WriteString(fmt.Sprintf("%s%s: %v\n", indentStr, key, v))
	}
}

func needsQuoting(s string) bool {
	// Quote if it could be parsed as number, bool, or contains special chars
	// YAML boolean variants: true/false, yes/no, on/off, y/n, t/f
	lower := strings.ToLower(s)
	yamlBools := map[string]bool{
		"true": true, "false": true,
		"yes": true, "no": true,
		"on": true, "off": true,
		"y": true, "n": true,
		"t": true, "f": true,
		"null": true, "~": true,
		"": true,
	}
	if yamlBools[lower] {
		return true
	}
	// Check if it's a number
	for i, c := range s {
		if i == 0 && (c == '-' || c == '+') {
			continue
		}
		if c == '.' {
			continue
		}
		if c < '0' || c > '9' {
			// Not a simple number, check for special characters
			if strings.ContainsAny(s, ":#{}[]|>*&!%@`\"'=") {
				return true
			}
			return false
		}
	}
	// It's a number, needs quoting
	return true
}

// quoteYAMLKey ensures a YAML map key is properly quoted
func quoteYAMLKey(s string) string {
	if s == "" {
		return `""`
	}
	if needsQuoting(s) {
		// Use single quotes, escape internal single quotes by doubling
		escaped := strings.ReplaceAll(s, "'", "''")
		return "'" + escaped + "'"
	}
	return s
}

// isIntegerType checks if a PostgreSQL type is an integer type
func isIntegerType(typ string) bool {
	intTypes := map[string]bool{
		"smallint": true, "int2": true,
		"integer": true, "int": true, "int4": true,
		"bigint": true, "int8": true,
		"serial": true, "serial4": true,
		"bigserial": true, "serial8": true,
		"smallserial": true, "serial2": true,
	}
	return intTypes[typ]
}

// collectMultiColumnChecks queries check constraints (both single and multi-column)
func (s *Scaffolder) collectMultiColumnChecks(schema, table string) error {
	qualifiedName := schema + "." + table

	query := `
		SELECT
			con.conname,
			pg_get_constraintdef(con.oid) as definition,
			array_agg(att.attname::text ORDER BY att.attname) as columns
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
		JOIN LATERAL unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ord) ON true
		JOIN pg_attribute att ON att.attrelid = rel.oid AND att.attnum = cols.attnum
		WHERE con.contype = 'c'
		  AND nsp.nspname = $1
		  AND rel.relname = $2
		GROUP BY con.conname, con.oid
	`

	rows, err := s.db.Query(query, schema, table)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, definition string
		var columns []string
		if err := rows.Scan(&name, &definition, (*pgStringArray)(&columns)); err != nil {
			return err
		}

		check := multiColumnCheck{
			Name:       name,
			Columns:    columns,
			Definition: definition,
		}
		s.multiColChecks[qualifiedName] = append(s.multiColChecks[qualifiedName], check)
	}

	return rows.Err()
}

// pgStringArray helper for scanning PostgreSQL text[] into []string
type pgStringArray []string

func (a *pgStringArray) Scan(src any) error {
	if src == nil {
		*a = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		*a = parsePgArray(string(v))
	case string:
		*a = parsePgArray(v)
	default:
		return fmt.Errorf("cannot scan %T into pgStringArray", src)
	}
	return nil
}

// getCheckConstraintWarnings returns warnings for columns involved in check constraints
func (s *Scaffolder) getCheckConstraintWarnings(tableName string) []string {
	var warnings []string
	checks := s.multiColChecks[tableName]

	for _, check := range checks {
		// Multi-column constraints need coordinated generators
		if len(check.Columns) > 1 {
			warnings = append(warnings, fmt.Sprintf(
				"TODO: %s has multi-column check constraint '%s' on %v - coordinate generators: %s",
				tableName, check.Name, check.Columns, check.Definition))
			continue
		}

		// Single-column constraints - warn about length/format checks
		lowerDef := strings.ToLower(check.Definition)
		if strings.Contains(lowerDef, "length") || strings.Contains(lowerDef, "char_length") ||
			strings.Contains(lowerDef, "octet_length") {
			warnings = append(warnings, fmt.Sprintf(
				"TODO: %s.%s has length constraint '%s' - ensure generator produces valid length: %s",
				tableName, check.Columns[0], check.Name, check.Definition))
		} else if strings.Contains(lowerDef, "~~") || strings.Contains(lowerDef, "like") ||
			strings.Contains(lowerDef, "~") {
			warnings = append(warnings, fmt.Sprintf(
				"TODO: %s.%s has pattern constraint '%s' - ensure generator matches pattern: %s",
				tableName, check.Columns[0], check.Name, check.Definition))
		}
	}

	return warnings
}
