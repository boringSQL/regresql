package regresql

import (
	"encoding/xml"
	"fmt"
	"io"
)

type (
	JUnitTestSuites struct {
		XMLName xml.Name         `xml:"testsuites"`
		Suites  []JUnitTestSuite `xml:"testsuite"`
	}

	JUnitTestSuite struct {
		Name     string          `xml:"name,attr"`
		Tests    int             `xml:"tests,attr"`
		Failures int             `xml:"failures,attr"`
		Skipped  int             `xml:"skipped,attr"`
		Time     float64         `xml:"time,attr"`
		Cases    []JUnitTestCase `xml:"testcase"`
	}

	JUnitTestCase struct {
		Name      string              `xml:"name,attr"`
		Classname string              `xml:"classname,attr"`
		Time      float64             `xml:"time,attr"`
		Failure   *JUnitFailure       `xml:"failure,omitempty"`
		Skipped   *JUnitSkipped       `xml:"skipped,omitempty"`
	}

	JUnitFailure struct {
		Message string `xml:"message,attr"`
		Type    string `xml:"type,attr"`
		Content string `xml:",chardata"`
	}

	JUnitSkipped struct {
		Message string `xml:"message,attr,omitempty"`
	}
)

type JUnitFormatter struct {
	results []TestResult
}

func (f *JUnitFormatter) Start(w io.Writer) error {
	f.results = make([]TestResult, 0)
	return nil
}

func (f *JUnitFormatter) AddResult(r TestResult, w io.Writer) error {
	f.results = append(f.results, r)
	return nil
}

func (f *JUnitFormatter) Finish(s *TestSummary, w io.Writer) error {
	cases := make([]JUnitTestCase, 0, len(f.results))

	for _, r := range f.results {
		tc := JUnitTestCase{
			Name:      r.Name,
			Classname: "regresql." + r.Type,
			Time:      r.Duration,
		}

		if r.Status == "failed" {
			msg := "Test failed"
			content := r.Error
			if r.Type == "cost" {
				msg = fmt.Sprintf("Cost increased by %.1f%% (expected: %.2f, actual: %.2f)",
					r.PercentIncrease, r.ExpectedCost, r.ActualCost)
			} else if r.Type == "output" {
				msg = "Output differs from expected"
				content = r.Diff
			}
			tc.Failure = &JUnitFailure{
				Message: msg,
				Type:    r.Type,
				Content: content,
			}
		} else if r.Status == "skipped" {
			tc.Skipped = &JUnitSkipped{
				Message: r.Error,
			}
		}

		cases = append(cases, tc)
	}

	suite := JUnitTestSuite{
		Name:     "regresql",
		Tests:    s.Total,
		Failures: s.Failed,
		Skipped:  s.Skipped,
		Time:     s.Duration,
		Cases:    cases,
	}

	suites := JUnitTestSuites{
		Suites: []JUnitTestSuite{suite},
	}

	output, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "%s%s\n", xml.Header, output)
	return nil
}

func init() {
	RegisterFormatter("junit", &JUnitFormatter{})
}
