package regresql

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type (
	CoverageOptions struct {
		Root         string
		TaxonomyPath string
		Format       string // console | json
		OutputPath   string
	}

	// Taxonomy is the planner-feature coverage matrix: each axis lists the cells
	// (feature/optimization) a corpus should exercise. A query claims a cell with
	// a `-- cell: axis/name` tag.
	Taxonomy struct {
		TargetTotal int                 `json:"target_total"`
		TargetPg19  int                 `json:"target_pg19"`
		Axes        map[string][]string `json:"axes"`
	}

	CoverageReport struct {
		TotalCells  int                 `json:"total_cells"`
		Covered     []string            `json:"covered"`
		Empty       []string            `json:"empty"`
		Untagged    []string            `json:"untagged_queries"`
		Unknown     []string            `json:"unknown_cells"` // tags not in the taxonomy
		CellQueries map[string][]string `json:"cell_queries"`
	}
)

func (t *Taxonomy) targetCells() []string {
	var cells []string
	for axis, names := range t.Axes {
		for _, n := range names {
			cells = append(cells, axis+"/"+n)
		}
	}
	sort.Strings(cells)
	return cells
}

func Coverage(opts CoverageOptions) int {
	tax, err := loadTaxonomy(opts.TaxonomyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "taxonomy: %s\n", err)
		return 2
	}

	// map every query's -- cell: tag; queries without one are untagged
	cellQueries := map[string][]string{}
	var untagged []string
	suite := Walk(opts.Root, nil)
	for _, folder := range suite.Dirs {
		for _, name := range folder.Files {
			data, err := os.ReadFile(filepath.Join(opts.Root, folder.Dir, name))
			if err != nil {
				continue
			}
			qname := strings.TrimSuffix(name, ".sql")
			cell := parseCell(string(data))
			if cell == "" {
				untagged = append(untagged, qname)
				continue
			}
			cellQueries[cell] = append(cellQueries[cell], qname)
		}
	}

	target := tax.targetCells()
	inTaxonomy := map[string]bool{}
	for _, c := range target {
		inTaxonomy[c] = true
	}

	report := CoverageReport{TotalCells: len(target), CellQueries: cellQueries}
	for _, c := range target {
		if len(cellQueries[c]) > 0 {
			report.Covered = append(report.Covered, c)
		} else {
			report.Empty = append(report.Empty, c)
		}
	}
	for cell := range cellQueries {
		if !inTaxonomy[cell] {
			report.Unknown = append(report.Unknown, cell)
		}
	}
	sort.Strings(report.Unknown)
	sort.Strings(untagged)
	report.Untagged = untagged

	renderCoverage(&report, tax, opts.Format, opts.OutputPath)
	return 0
}

func renderCoverage(r *CoverageReport, tax *Taxonomy, format, outputPath string) {
	w, closeFn, err := getWriter(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}
	defer closeFn()

	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(r)
		return
	}

	fmt.Fprintf(w, "== Coverage vs taxonomy (target %d queries, %d pg19) ==\n", tax.TargetTotal, tax.TargetPg19)
	fmt.Fprintf(w, "  cells covered: %d / %d\n", len(r.Covered), r.TotalCells)
	fmt.Fprintf(w, "  covered: %s\n", strings.Join(r.Covered, ", "))
	fmt.Fprintf(w, "\n  EMPTY (%d): %s\n", len(r.Empty), strings.Join(r.Empty, ", "))
	if len(r.Unknown) > 0 {
		fmt.Fprintf(w, "\n  tags not in taxonomy (%d): %s\n", len(r.Unknown), strings.Join(r.Unknown, ", "))
	}
	if len(r.Untagged) > 0 {
		fmt.Fprintf(w, "  untagged queries (%d): %s\n", len(r.Untagged), strings.Join(r.Untagged, ", "))
	}
}

func loadTaxonomy(path string) (*Taxonomy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Taxonomy
	return &t, json.Unmarshal(data, &t)
}

// parseCell pulls "axis/name" from the first `-- cell: ...` line, "" if none.
func parseCell(sql string) string {
	sc := bufio.NewScanner(strings.NewReader(sql))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "-- cell:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
