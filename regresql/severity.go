package regresql

import "fmt"

type (
	Severity string
)

const (
	SevInfo     Severity = "info"
	SevWarning  Severity = "warning"
	SevError    Severity = "error"
	SevCritical Severity = "critical"
)

func (s Severity) Rank() int {
	switch s {
	case SevInfo:
		return 0
	case SevWarning:
		return 1
	case SevError:
		return 2
	case SevCritical:
		return 3
	default:
		return -1
	}
}

func SeverityAtLeast(s string, floor Severity) bool {
	r, f := Severity(s).Rank(), floor.Rank()
	return r >= 0 && f >= 0 && r >= f
}

func ParseSeverity(s string) (Severity, error) {
	sev := Severity(s)
	if sev.Rank() < 0 {
		return "", fmt.Errorf("unknown severity %q (accepted: info, warning, error, critical)", s)
	}
	return sev, nil
}
