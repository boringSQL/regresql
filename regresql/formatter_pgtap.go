package regresql

import (
	"fmt"
	"io"

	"github.com/mndrix/tap-go"
)

type PgTAPFormatter struct {
	t *tap.T
}

func (f *PgTAPFormatter) Start(w io.Writer) error {
	f.t.Header(0)
	return nil
}

func (f *PgTAPFormatter) AddResult(r TestResult, w io.Writer) error {
	if r.Status == "failed" {
		if r.Type == "output" {
			f.t.Diagnostic(fmt.Sprintf(`Query File: '%s'
Bindings File: '%s'
Bindings Name: '%s'
Query Parameters: '%v'

%s`, r.QueryFile, r.BindingsFile, r.BindingName, r.Parameters, r.Diff))
		} else if r.Type == "cost" {
			diagMsg := fmt.Sprintf("Cost increased by %.1f%% (threshold: %.0f%%)",
				r.PercentIncrease, r.Threshold)

			// Add plan regression information
			if len(r.PlanRegressions) > 0 {
				diagMsg += "\n\nPlan Regressions:"
				for _, reg := range r.PlanRegressions {
					if reg.Table != "" {
						diagMsg += fmt.Sprintf("\n  - [%s] Table '%s': %s → %s",
							reg.Severity, reg.Table, reg.OldScan, reg.NewScan)
					} else {
						diagMsg += fmt.Sprintf("\n  - [%s] %s", reg.Severity, reg.Message)
					}
				}
			}

			f.t.Diagnostic(diagMsg)
		}
	}

	// Add plan warnings as diagnostics
	if len(r.PlanWarnings) > 0 && (r.Status == "passed" || r.Status == "warning") {
		warningMsg := "Plan Quality Warnings:"
		for _, warning := range r.PlanWarnings {
			warningMsg += fmt.Sprintf("\n  - %s", warning.Message)
		}
		f.t.Diagnostic(warningMsg)
	}

	if r.Error != "" {
		f.t.Diagnostic(r.Error)
	}

	switch r.Status {
	case "passed":
		f.t.Ok(true, r.Name)
	case "failed":
		f.t.Ok(false, r.Name)
	case "warning":
		// Treat warnings as passing but with diagnostic info
		f.t.Ok(true, r.Name)
	case "skipped":
		f.t.Skip(1, r.Name)
	}

	return nil
}

func (f *PgTAPFormatter) Finish(summary *TestSummary, w io.Writer) error {
	return nil
}

func init() {
	RegisterFormatter("pgtap", &PgTAPFormatter{t: tap.New()})
}
