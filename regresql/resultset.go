package regresql

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

/*
A ResultSet stores the result of a Query in Filename, with Cols and Rows
separated.
*/
type ResultSet struct {
	Cols     []string        `json:"columns"`
	Rows     [][]interface{} `json:"rows"`
	Filename string          `json:"-"`
}

// TestConnectionString connects to PostgreSQL with pguri and issue a single
// query (select 1"), because some errors (such as missing SSL certificates)
// only happen at query time.
func TestConnectionString(pguri string) error {
	fmt.Printf("Connecting to '%s'… ", pguri)
	db, err := sql.Open("postgres", pguri)

	if err != nil {
		fmt.Println("✗")
		return err
	}
	defer db.Close()

	var args []interface{}
	if _, err := QueryDB(db, "Select 1", args...); err != nil {
		fmt.Println("✗")
		return err
	}
	fmt.Println("✓")

	return nil
}

// QueryDB runs the query against the db database connection, and returns a
// ResultSet
func QueryDB(db *sql.DB, query string, args ...interface{}) (*ResultSet, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	res := make([][]interface{}, 0)

	for rows.Next() {
		container := make([]interface{}, len(cols))
		dest := make([]interface{}, len(cols))
		for i, _ := range container {
			dest[i] = &container[i]
		}
		rows.Scan(dest...)
		r := make([]interface{}, len(cols))
		for i, _ := range cols {
			val := dest[i].(*interface{})
			r[i] = *val
		}

		res = append(res, r)
	}
	return &ResultSet{cols, res, ""}, nil
}

// Println outputs to standard output a Pretty Printed result set.
func (r *ResultSet) Println() {
	fmt.Println(r.PrettyPrint())

}

// PrettyPrint pretty prints a result set and returns it as a string
func (r *ResultSet) PrettyPrint() string {
	var b bytes.Buffer

	cn := len(r.Cols)

	// compute max length of values for each col, including column
	// name (used as an header)
	maxl := make([]int, cn)
	for i, colname := range r.Cols {
		maxl[i] = len(colname)
	}
	for _, row := range r.Rows {
		for i, value := range row {
			l := len(valueToString(value))
			if l > maxl[i] {
				maxl[i] = l
			}
		}
	}
	fmts := make([]string, cn)
	for i, l := range maxl {
		fmts[i] = fmt.Sprintf("%%-%ds", l)
	}

	for i, colname := range r.Cols {
		justify := strings.Repeat(" ", (maxl[i]-len(colname))/2)
		centered := justify + colname
		fmt.Fprintf(&b, fmts[i], centered)
		if i+1 < cn {
			fmt.Fprintf(&b, " | ")
		}
	}
	fmt.Fprintf(&b, "\n")

	for i, l := range maxl {
		fmt.Fprintf(&b, "%s", strings.Repeat("-", l))
		if i+1 < cn {
			fmt.Fprintf(&b, "-+-")
		}
	}
	fmt.Fprintf(&b, "\n")

	for _, row := range r.Rows {
		for i, value := range row {
			s := valueToString(value)
			if i+1 < cn {
				fmt.Fprintf(&b, fmts[i], s)
				fmt.Fprintf(&b, " | ")
			} else {
				fmt.Fprint(&b, s)
			}
		}
		fmt.Fprintf(&b, "\n")
	}
	return b.String()
}

// Writes the Result Set r to filename, overwriting it if already exists
// when overwrite is true
func (r *ResultSet) Write(filename string, overwrite bool) error {
	// Marshal ResultSet to JSON
	jsonBytes, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal result set to JSON: %s\n", err)
	}

	var f *os.File
	if _, err = os.Stat(filename); os.IsNotExist(err) {
		f, err = os.Create(filename)

		if err != nil {
			return fmt.Errorf(
				"Failed to write result set '%s': %s\n",
				filename,
				err)
		}
		defer f.Close()

		_, err = f.Write(jsonBytes)
		if err != nil {
			return fmt.Errorf("Failed to write JSON to file '%s': %s\n", filename, err)
		}
	} else {
		if !overwrite {
			return errors.New("Target file '%s' already exists")
		}
		f, err = os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC, 0644)

		if err != nil {
			return fmt.Errorf(
				"Failed to open result set '%s': %s\n",
				filename,
				err)
		}
		defer f.Close()

		_, err = f.Write(jsonBytes)
		if err != nil {
			return fmt.Errorf("Failed to write JSON to file '%s': %s\n", filename, err)
		}
	}
	return nil
}

// valueToString is an helper function for the Pretty Printer
func valueToString(value interface{}) string {
	var s string
	switch value.(type) {
	case int:
		s = fmt.Sprintf("%d", value)
	case float32:
		s = fmt.Sprintf("%g", value)
	case float64:
		s = fmt.Sprintf("%g", value)
	case time.Time:
		s = fmt.Sprintf("%s", value)
	case []byte:
		s = fmt.Sprintf("%s", value)
	default:
		s = fmt.Sprintf("%v", value)
	}
	return s
}
