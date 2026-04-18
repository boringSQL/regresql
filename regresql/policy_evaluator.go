package regresql

type PolicyDecision struct {
	Rule         string
	Table        string
	FromSeverity string
	ToSeverity   string
	Reason       string
}

// ApplyPolicies re-maps severities of existing PlanWarning and
// PlanRegression records on result according to p.Severity, recording
// each change as a PolicyDecision on result.PolicyApplied.
//
// No-op when p is nil or p.Severity is empty.
func ApplyPolicies(result *TestResult, p *PoliciesConfig) {
	if p == nil || len(p.Severity) == 0 {
		return
	}

	for i := range result.PlanWarnings {
		w := &result.PlanWarnings[i]
		rule := string(w.Type)
		newSev, ok := p.Severity[rule]
		if !ok || newSev == w.Severity {
			continue
		}
		result.PolicyApplied = append(result.PolicyApplied, PolicyDecision{
			Rule:         rule,
			Table:        w.Table,
			FromSeverity: w.Severity,
			ToSeverity:   newSev,
			Reason:       p.Reasons[rule],
		})
		w.Severity = newSev
	}

	for i := range result.PlanRegressions {
		r := &result.PlanRegressions[i]
		rule := string(r.Type)
		newSev, ok := p.Severity[rule]
		if !ok || newSev == r.Severity {
			continue
		}
		result.PolicyApplied = append(result.PolicyApplied, PolicyDecision{
			Rule:         rule,
			Table:        r.Table,
			FromSeverity: r.Severity,
			ToSeverity:   newSev,
			Reason:       p.Reasons[rule],
		})
		r.Severity = newSev
	}
}
