package regresql

import (
	"database/sql"
	"fmt"
	"strings"
)

type (
	// DatabaseSchema provides metadata about database structure
	DatabaseSchema struct {
		tables map[string]*TableInfo
	}

	// TableInfo contains metadata about table
	TableInfo struct {
		Schema      string
		Name        string
		Columns     map[string]*ColumnInfo
		PrimaryKey  []string
		ForeignKeys []*ForeignKeyInfo
	}

	// ColumnInfo provides metadata about a database column
	ColumnInfo struct {
		Name         string
		Type         string
		IsNullable   bool
		IsPrimaryKey bool
		IsForeignKey bool
		IsUnique     bool
		ForeignKey   *ForeignKeyInfo
		Default      *string
		MaxLength    *int
	}

	// ForeignKeyInfo describes a foreign key relationship
	ForeignKeyInfo struct {
		ConstraintName   string
		ColumnName       string
		ReferencedTable  string
		ReferencedColumn string
	}
)

// IntrospectSchema queries the database to build schema metadata
func IntrospectSchema(db *sql.DB) (*DatabaseSchema, error) {
	dbSchema := &DatabaseSchema{
		tables: make(map[string]*TableInfo),
	}

	// Get all tables (schema-qualified names like "auth.users")
	tables, err := getTables(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	// For each table, get columns, primary keys, and foreign keys
	for _, qualifiedName := range tables {
		schemaName, tableName := parseTableName(qualifiedName)

		tableInfo := &TableInfo{
			Schema:  schemaName,
			Name:    tableName,
			Columns: make(map[string]*ColumnInfo),
		}

		// Get columns
		columns, err := getColumns(db, schemaName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get columns for table '%s': %w", qualifiedName, err)
		}
		tableInfo.Columns = columns

		// Get primary keys
		primaryKeys, err := getPrimaryKeys(db, schemaName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get primary keys for table '%s': %w", qualifiedName, err)
		}
		tableInfo.PrimaryKey = primaryKeys

		// Mark primary key columns
		for _, pkCol := range primaryKeys {
			if col, exists := tableInfo.Columns[pkCol]; exists {
				col.IsPrimaryKey = true
			}
		}

		// Get foreign keys
		foreignKeys, err := getForeignKeys(db, schemaName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get foreign keys for table '%s': %w", qualifiedName, err)
		}
		tableInfo.ForeignKeys = foreignKeys

		// Mark foreign key columns and attach FK info
		for _, fk := range foreignKeys {
			if col, exists := tableInfo.Columns[fk.ColumnName]; exists {
				col.IsForeignKey = true
				col.ForeignKey = fk
			}
		}

		// Get unique constraints
		uniqueCols, err := getUniqueColumns(db, schemaName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get unique constraints for table '%s': %w", qualifiedName, err)
		}
		for colName := range uniqueCols {
			if col, exists := tableInfo.Columns[colName]; exists {
				col.IsUnique = true
			}
		}

		dbSchema.tables[qualifiedName] = tableInfo
	}

	return dbSchema, nil
}

// getTables retrieves all table names from the database (all user schemas)
func getTables(db *sql.DB) ([]string, error) {
	query := `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		  AND table_type = 'BASE TABLE'
		ORDER BY table_schema, table_name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var schemaName, tableName string
		if err := rows.Scan(&schemaName, &tableName); err != nil {
			return nil, err
		}
		tables = append(tables, schemaName+"."+tableName)
	}

	return tables, rows.Err()
}

// parseTableName splits a schema-qualified table name into schema and table parts
func parseTableName(name string) (schema, table string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "public", name
}

// getColumns retrieves column metadata for a table
func getColumns(db *sql.DB, schemaName, tableName string) (map[string]*ColumnInfo, error) {
	query := `
		SELECT
			column_name,
			data_type,
			udt_name,
			is_nullable,
			column_default,
			character_maximum_length
		FROM information_schema.columns
		WHERE table_schema = $1
		  AND table_name = $2
		ORDER BY ordinal_position
	`

	rows, err := db.Query(query, schemaName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]*ColumnInfo)
	for rows.Next() {
		var (
			columnName    string
			dataType      string
			udtName       string
			isNullable    string
			columnDefault *string
			maxLength     *int64
		)

		if err := rows.Scan(&columnName, &dataType, &udtName, &isNullable, &columnDefault, &maxLength); err != nil {
			return nil, err
		}

		// Use udt_name for USER-DEFINED types (e.g., hstore, custom enums)
		colType := dataType
		if dataType == "USER-DEFINED" && udtName != "" {
			colType = udtName
		}

		col := &ColumnInfo{
			Name:       columnName,
			Type:       colType,
			IsNullable: isNullable == "YES",
			Default:    columnDefault,
		}

		if maxLength != nil {
			length := int(*maxLength)
			col.MaxLength = &length
		}

		columns[columnName] = col
	}

	return columns, rows.Err()
}

// getPrimaryKeys retrieves primary key column names for a table
func getPrimaryKeys(db *sql.DB, schemaName, tableName string) ([]string, error) {
	// Use schema-qualified name for regclass cast
	qualifiedName := schemaName + "." + tableName
	query := `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass
		  AND i.indisprimary
		ORDER BY array_position(i.indkey, a.attnum)
	`

	rows, err := db.Query(query, qualifiedName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var primaryKeys []string
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err != nil {
			return nil, err
		}
		primaryKeys = append(primaryKeys, columnName)
	}

	return primaryKeys, rows.Err()
}

// getForeignKeys retrieves foreign key constraints for a table
func getForeignKeys(db *sql.DB, schemaName, tableName string) ([]*ForeignKeyInfo, error) {
	query := `
		SELECT
			tc.constraint_name,
			kcu.column_name,
			ccu.table_schema AS referenced_schema,
			ccu.table_name AS referenced_table,
			ccu.column_name AS referenced_column
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
		  ON tc.constraint_name = kcu.constraint_name
		  AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
		  ON ccu.constraint_name = tc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_name = $1
		  AND tc.table_schema = $2
	`

	rows, err := db.Query(query, tableName, schemaName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foreignKeys []*ForeignKeyInfo
	for rows.Next() {
		var fk ForeignKeyInfo
		var refSchema string
		if err := rows.Scan(&fk.ConstraintName, &fk.ColumnName, &refSchema, &fk.ReferencedTable, &fk.ReferencedColumn); err != nil {
			return nil, err
		}
		// Store schema-qualified referenced table name
		fk.ReferencedTable = refSchema + "." + fk.ReferencedTable
		foreignKeys = append(foreignKeys, &fk)
	}

	return foreignKeys, rows.Err()
}

func getUniqueColumns(db *sql.DB, schemaName, tableName string) (map[string]bool, error) {
	// Use schema-qualified name for regclass cast
	qualifiedName := schemaName + "." + tableName
	query := `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass
		  AND i.indisunique
		  AND NOT i.indisprimary
		  AND array_length(i.indkey, 1) = 1
	`

	rows, err := db.Query(query, qualifiedName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	uniqueCols := make(map[string]bool)
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return nil, err
		}
		uniqueCols[colName] = true
	}
	return uniqueCols, rows.Err()
}

// GetTable retrieves table metadata by name (schema-qualified or unqualified)
func (ds *DatabaseSchema) GetTable(name string) (*TableInfo, error) {
	// Try exact match first (for schema-qualified names)
	if table, exists := ds.tables[name]; exists {
		return table, nil
	}
	// If no dot in name, try public schema
	if !strings.Contains(name, ".") {
		if table, exists := ds.tables["public."+name]; exists {
			return table, nil
		}
	}
	return nil, fmt.Errorf("table not found: %s", name)
}

// GetTables returns all table names
func (ds *DatabaseSchema) GetTables() []string {
	tables := make([]string, 0, len(ds.tables))
	for name := range ds.tables {
		tables = append(tables, name)
	}
	return tables
}

// GetForeignKeyDependencies returns tables that must be loaded before the given table
func (ds *DatabaseSchema) GetForeignKeyDependencies(tableName string) ([]string, error) {
	table, err := ds.GetTable(tableName)
	if err != nil {
		return nil, err
	}

	// Build the qualified name for self-reference comparison
	qualifiedName := table.Schema + "." + table.Name

	// Collect unique referenced tables
	deps := make(map[string]bool)
	for _, fk := range table.ForeignKeys {
		// Don't include self-references as dependencies
		if fk.ReferencedTable != qualifiedName {
			deps[fk.ReferencedTable] = true
		}
	}

	// Convert to slice
	result := make([]string, 0, len(deps))
	for dep := range deps {
		result = append(result, dep)
	}

	return result, nil
}

// HasTable checks if a table exists in the schema
func (ds *DatabaseSchema) HasTable(name string) bool {
	if _, exists := ds.tables[name]; exists {
		return true
	}
	// If no dot in name, try public schema
	if !strings.Contains(name, ".") {
		_, exists := ds.tables["public."+name]
		return exists
	}
	return false
}
