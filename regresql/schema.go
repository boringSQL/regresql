package regresql

import (
	"database/sql"
	"fmt"
)

type (
	// DatabaseSchema provides metadata about database structure
	DatabaseSchema struct {
		tables map[string]*TableInfo
	}

	// TableInfo contains metadata about table
	TableInfo struct {
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
	schema := &DatabaseSchema{
		tables: make(map[string]*TableInfo),
	}

	// Get all tables
	tables, err := getTables(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	// For each table, get columns, primary keys, and foreign keys
	for _, tableName := range tables {
		tableInfo := &TableInfo{
			Name:    tableName,
			Columns: make(map[string]*ColumnInfo),
		}

		// Get columns
		columns, err := getColumns(db, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get columns for table '%s': %w", tableName, err)
		}
		tableInfo.Columns = columns

		// Get primary keys
		primaryKeys, err := getPrimaryKeys(db, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get primary keys for table '%s': %w", tableName, err)
		}
		tableInfo.PrimaryKey = primaryKeys

		// Mark primary key columns
		for _, pkCol := range primaryKeys {
			if col, exists := tableInfo.Columns[pkCol]; exists {
				col.IsPrimaryKey = true
			}
		}

		// Get foreign keys
		foreignKeys, err := getForeignKeys(db, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get foreign keys for table '%s': %w", tableName, err)
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
		uniqueCols, err := getUniqueColumns(db, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get unique constraints for table '%s': %w", tableName, err)
		}
		for colName := range uniqueCols {
			if col, exists := tableInfo.Columns[colName]; exists {
				col.IsUnique = true
			}
		}

		schema.tables[tableName] = tableInfo
	}

	return schema, nil
}

// getTables retrieves all table names from the database
func getTables(db *sql.DB) ([]string, error) {
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}

	return tables, rows.Err()
}

// getColumns retrieves column metadata for a table
func getColumns(db *sql.DB, tableName string) (map[string]*ColumnInfo, error) {
	query := `
		SELECT
			column_name,
			data_type,
			is_nullable,
			column_default,
			character_maximum_length
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = $1
		ORDER BY ordinal_position
	`

	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]*ColumnInfo)
	for rows.Next() {
		var (
			columnName  string
			dataType    string
			isNullable  string
			columnDefault sql.NullString
			maxLength   sql.NullInt64
		)

		if err := rows.Scan(&columnName, &dataType, &isNullable, &columnDefault, &maxLength); err != nil {
			return nil, err
		}

		col := &ColumnInfo{
			Name:       columnName,
			Type:       dataType,
			IsNullable: isNullable == "YES",
		}

		if columnDefault.Valid {
			col.Default = &columnDefault.String
		}

		if maxLength.Valid {
			length := int(maxLength.Int64)
			col.MaxLength = &length
		}

		columns[columnName] = col
	}

	return columns, rows.Err()
}

// getPrimaryKeys retrieves primary key column names for a table
func getPrimaryKeys(db *sql.DB, tableName string) ([]string, error) {
	query := `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass
		  AND i.indisprimary
		ORDER BY array_position(i.indkey, a.attnum)
	`

	rows, err := db.Query(query, tableName)
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
func getForeignKeys(db *sql.DB, tableName string) ([]*ForeignKeyInfo, error) {
	query := `
		SELECT
			tc.constraint_name,
			kcu.column_name,
			ccu.table_name AS referenced_table,
			ccu.column_name AS referenced_column
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
		  ON tc.constraint_name = kcu.constraint_name
		  AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
		  ON ccu.constraint_name = tc.constraint_name
		  AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_name = $1
		  AND tc.table_schema = 'public'
	`

	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foreignKeys []*ForeignKeyInfo
	for rows.Next() {
		var fk ForeignKeyInfo
		if err := rows.Scan(&fk.ConstraintName, &fk.ColumnName, &fk.ReferencedTable, &fk.ReferencedColumn); err != nil {
			return nil, err
		}
		foreignKeys = append(foreignKeys, &fk)
	}

	return foreignKeys, rows.Err()
}

func getUniqueColumns(db *sql.DB, tableName string) (map[string]bool, error) {
	query := `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass
		  AND i.indisunique
		  AND NOT i.indisprimary
		  AND array_length(i.indkey, 1) = 1
	`

	rows, err := db.Query(query, tableName)
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

// GetTable retrieves table metadata
func (ds *DatabaseSchema) GetTable(name string) (*TableInfo, error) {
	table, exists := ds.tables[name]
	if !exists {
		return nil, fmt.Errorf("table not found: %s", name)
	}
	return table, nil
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

	// Collect unique referenced tables
	deps := make(map[string]bool)
	for _, fk := range table.ForeignKeys {
		// Don't include self-references as dependencies
		if fk.ReferencedTable != tableName {
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
	_, exists := ds.tables[name]
	return exists
}
