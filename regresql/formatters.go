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
		Type     string // "output" or "cost"
		Status   string // "passed", "failed", "skipped"
		Duration float64
		Error    string

		// Output comparisons
		Diff string

		// Cost comparisons
		ExpectedCost    float64
		ActualCost      float64
		PercentIncrease float64
		Threshold       float64

		// Diagnostics
		QueryFile    string
		BindingsFile string
		BindingName  string
		Parameters   map[string]string
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
