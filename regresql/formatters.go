package regresql

import (
	"fmt"
	"io"
	"os"
	"time"
)

type (
	TestResult struct {
		Name     string
		Type     string // "output", "cost", or "plan_quality"
		Status   string // "passed", "failed", "skipped", "warning"
		Duration float64
		Error    string

		// Output comparisons
		Diff           string
		StructuredDiff *StructuredDiff // nil if not computed

		// Cost comparisons
		ExpectedCost    float64
		ActualCost      float64
		PercentIncrease float64
		Threshold       float64
		PlanChanged      bool
		PlanRegressions  []PlanRegression
		PlanWarnings     []PlanWarning

		// Diagnostics
		QueryFile    string
		BindingsFile string
		BindingName  string
		Parameters   map[string]any
	}

	TestSummary struct {
		Total     int
		Passed    int
		Failed    int
		Skipped   int
		Duration  float64
		Results   []TestResult
		StartTime time.Time
	}

	OutputFormatter interface {
		Start(w io.Writer) error
		AddResult(result TestResult, w io.Writer) error
		Finish(summary *TestSummary, w io.Writer) error
	}
)

var formatters = make(map[string]OutputFormatter)

func RegisterFormatter(name string, formatter OutputFormatter) {
	formatters[name] = formatter
}

func GetFormatter(name string) (OutputFormatter, error) {
	f, ok := formatters[name]
	if !ok {
		return nil, fmt.Errorf("unknown formatter: %s", name)
	}
	return f, nil
}

func NewTestSummary() *TestSummary {
	return &TestSummary{
		StartTime: time.Now(),
	}
}

func (s *TestSummary) AddResult(r TestResult) {
	s.Results = append(s.Results, r)
	s.Total++
	s.Duration += r.Duration

	switch r.Status {
	case "passed":
		s.Passed++
	case "failed":
		s.Failed++
	case "skipped":
		s.Skipped++
	}
}

func getWriter(path string) (io.Writer, func() error, error) {
	if path == "" {
		return os.Stdout, func() error { return nil }, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create output file: %w", err)
	}
	return f, f.Close, nil
}

type MultiFormatter struct {
	formatters []OutputFormatter
	writers    []io.Writer
	closers    []func() error
}

func NewMultiFormatter(specs []struct{ Format, Path string }) (*MultiFormatter, error) {
	mf := &MultiFormatter{
		formatters: make([]OutputFormatter, 0, len(specs)),
		writers:    make([]io.Writer, 0, len(specs)),
		closers:    make([]func() error, 0, len(specs)),
	}

	for _, spec := range specs {
		f, err := GetFormatter(spec.Format)
		if err != nil {
			return nil, err
		}
		w, close, err := getWriter(spec.Path)
		if err != nil {
			return nil, err
		}
		mf.formatters = append(mf.formatters, f)
		mf.writers = append(mf.writers, w)
		mf.closers = append(mf.closers, close)
	}

	return mf, nil
}

func (mf *MultiFormatter) Start(w io.Writer) error {
	for i, f := range mf.formatters {
		if err := f.Start(mf.writers[i]); err != nil {
			return err
		}
	}
	return nil
}

func (mf *MultiFormatter) AddResult(r TestResult, w io.Writer) error {
	for i, f := range mf.formatters {
		if err := f.AddResult(r, mf.writers[i]); err != nil {
			return err
		}
	}
	return nil
}

func (mf *MultiFormatter) Finish(s *TestSummary, w io.Writer) error {
	for i, f := range mf.formatters {
		if err := f.Finish(s, mf.writers[i]); err != nil {
			return err
		}
	}
	for _, close := range mf.closers {
		if err := close(); err != nil {
			return err
		}
	}
	return nil
}
