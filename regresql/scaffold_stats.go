package regresql

import (
	"fmt"
	"regexp"
	"strings"
)

// collectStats retrieves pg_stats data for a table
func (s *Scaffolder) collectStats(schemaName, tableName string) error {
	qualifiedName := schemaName + "." + tableName

	query := `
		SELECT
			attname,
			null_frac,
			n_distinct,
			avg_width,
			correlation,
			most_common_vals::text,
			most_common_freqs::text,
			histogram_bounds::text
		FROM pg_stats
		WHERE schemaname = $1
		  AND tablename = $2
	`

	rows, err := s.db.Query(query, schemaName, tableName)
	if err != nil {
		return fmt.Errorf("failed to query pg_stats: %w", err)
	}
	defer rows.Close()

	tableStats := make(map[string]*ColumnProfile)

	for rows.Next() {
		var (
			attname        string
			nullFrac       float64
			nDistinct      float64
			avgWidth       int
			correlation    *float64
			mcvRaw         *string
			mcfRaw         *string
			histogramRaw   *string
		)

		if err := rows.Scan(&attname, &nullFrac, &nDistinct, &avgWidth, &correlation, &mcvRaw, &mcfRaw, &histogramRaw); err != nil {
			return fmt.Errorf("failed to scan pg_stats row: %w", err)
		}

		profile := &ColumnProfile{
			NullFrac:   nullFrac,
			NDistinct:  nDistinct,
			AvgWidth:   avgWidth,
		}

		if correlation != nil {
			profile.Correlation = *correlation
		}

		// Parse most_common_vals
		if mcvRaw != nil && *mcvRaw != "" {
			profile.MostCommonVals = parsePgArray(*mcvRaw)
		}

		// Parse most_common_freqs
		if mcfRaw != nil && *mcfRaw != "" {
			profile.MostCommonFreqs = parsePgFloatArray(*mcfRaw)
		}

		// Parse histogram_bounds
		if histogramRaw != nil && *histogramRaw != "" {
			profile.HistogramBounds = parsePgArray(*histogramRaw)
		}

		tableStats[attname] = profile
	}

	if err := rows.Err(); err != nil {
		return err
	}

	s.stats[qualifiedName] = tableStats
	return nil
}

// parsePgArray parses PostgreSQL array literal format: {val1,val2,"val with spaces"}
func parsePgArray(s string) []string {
	// Remove outer braces
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")

	if s == "" {
		return nil
	}

	var result []string
	var current strings.Builder
	inQuote := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			current.WriteByte(c)
			escaped = false
			continue
		}

		switch c {
		case '\\':
			escaped = true
		case '"':
			inQuote = !inQuote
		case ',':
			if !inQuote {
				val := current.String()
				result = append(result, cleanPgValue(val))
				current.Reset()
			} else {
				current.WriteByte(c)
			}
		default:
			current.WriteByte(c)
		}
	}

	// Don't forget the last value
	if current.Len() > 0 {
		val := current.String()
		result = append(result, cleanPgValue(val))
	}

	return result
}

// cleanPgValue cleans up a PostgreSQL array value
func cleanPgValue(s string) string {
	// Remove surrounding quotes if present
	s = strings.TrimPrefix(s, "\"")
	s = strings.TrimSuffix(s, "\"")
	// Unescape doubled quotes
	s = strings.ReplaceAll(s, "\"\"", "\"")
	return s
}

// parsePgFloatArray parses PostgreSQL float array: {0.45,0.25,0.15}
func parsePgFloatArray(s string) []float64 {
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")

	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	result := make([]float64, 0, len(parts))

	for _, part := range parts {
		var f float64
		if _, err := fmt.Sscanf(part, "%f", &f); err == nil {
			result = append(result, f)
		}
	}

	return result
}

// formatHistogramBoundsForYAML formats histogram bounds for YAML output
func formatHistogramBoundsForYAML(bounds []string, colType string) []string {
	lowerType := strings.ToLower(colType)

	// For numeric types, keep as-is
	if isNumericType(lowerType) {
		return bounds
	}

	// For timestamps/dates, format nicely
	if strings.Contains(lowerType, "timestamp") || lowerType == "date" {
		formatted := make([]string, len(bounds))
		for i, b := range bounds {
			formatted[i] = formatTimestamp(b)
		}
		return formatted
	}

	return bounds
}

func isNumericType(t string) bool {
	numericTypes := []string{"int", "serial", "bigint", "smallint", "numeric", "decimal", "real", "double", "money", "float"}
	for _, nt := range numericTypes {
		if strings.Contains(t, nt) {
			return true
		}
	}
	return false
}

// formatTimestamp extracts date from PostgreSQL timestamp
func formatTimestamp(s string) string {
	// Try to extract just the date portion for cleaner output
	// PostgreSQL format: "2023-01-15 10:30:00" or "2023-01-15T10:30:00Z"
	dateRe := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})`)
	if match := dateRe.FindStringSubmatch(s); len(match) > 1 {
		return match[1]
	}
	return s
}
