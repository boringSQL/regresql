package regresql

import "testing"

func sigWithJoins(joins ...string) *PlanSignature {
	return &PlanSignature{
		Relations: make(map[string]ScanInfo),
		JoinTypes: joins,
	}
}

func countRegressions(regs []PlanRegression, t RegressionType) int {
	n := 0
	for _, r := range regs {
		if r.Type == t {
			n++
		}
	}
	return n
}

func TestDetectPlanRegressions_JoinTypes_SameLengthDifferentElements(t *testing.T) {
	baseline := sigWithJoins("Nested Loop", "Hash Join")
	current := sigWithJoins("Hash Join", "Merge Join")

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, JoinTypeChanged); got != 1 {
		t.Fatalf("expected 1 JoinTypeChanged regression, got %d: %+v", got, regs)
	}
}

func TestDetectPlanRegressions_JoinTypes_Identical(t *testing.T) {
	baseline := sigWithJoins("Hash Join", "Merge Join")
	current := sigWithJoins("Hash Join", "Merge Join")

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, JoinTypeChanged); got != 0 {
		t.Fatalf("expected 0 JoinTypeChanged regressions, got %d: %+v", got, regs)
	}
}

func TestDetectPlanRegressions_JoinTypes_DifferentLengths(t *testing.T) {
	baseline := sigWithJoins("Hash Join")
	current := sigWithJoins("Hash Join", "Merge Join")

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, JoinTypeChanged); got != 1 {
		t.Fatalf("expected 1 JoinTypeChanged regression, got %d: %+v", got, regs)
	}
}

func TestDetectPlanRegressions_JoinTypes_BothEmpty(t *testing.T) {
	baseline := sigWithJoins()
	current := sigWithJoins()

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, JoinTypeChanged); got != 0 {
		t.Fatalf("expected 0 JoinTypeChanged regressions, got %d: %+v", got, regs)
	}
}

func TestDetectPlanRegressions_JoinTypes_OneEmpty(t *testing.T) {
	baseline := sigWithJoins()
	current := sigWithJoins("Hash Join")

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, JoinTypeChanged); got != 1 {
		t.Fatalf("expected 1 JoinTypeChanged regression, got %d: %+v", got, regs)
	}
}

// sigWithJoinModes builds a signature whose join nodes carry the same node
// type ("Hash Join") but the supplied Join Type qualifiers. This isolates the
// join-semantics axis (Inner vs Right Anti vs Semi) from the node-type axis so
// we can assert that compareJoinModes fires independently of compareJoinTypes.
func sigWithJoinModes(modes ...string) *PlanSignature {
	joins := make([]string, len(modes))
	for i := range modes {
		joins[i] = "Hash Join"
	}
	return &PlanSignature{
		Relations: make(map[string]ScanInfo),
		JoinTypes: joins,
		JoinModes: modes,
	}
}

// sigWithPartialModes builds a signature carrying only the given partial
// aggregate modes ("Partial"/"Finalize"), leaving every other axis empty so a
// PartialModeChanged regression can be attributed unambiguously.
func sigWithPartialModes(modes ...string) *PlanSignature {
	return &PlanSignature{
		Relations:    make(map[string]ScanInfo),
		PartialModes: modes,
	}
}

// The headline PoC gap: a NOT IN rewrite turns an Inner join into a Right Anti
// join while the node type ("Hash Join") stays put. compareJoinTypes is blind
// to this because the node-type slices are identical; compareJoinModes is the
// field that must catch it.
func TestDetectPlanRegressions_JoinModes_InnerToRightAnti(t *testing.T) {
	baseline := sigWithJoinModes("Inner")
	current := sigWithJoinModes("Right Anti")

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, JoinModeChanged); got != 1 {
		t.Fatalf("expected 1 JoinModeChanged regression, got %d: %+v", got, regs)
	}
	// The node type never changed, so the node-type comparison must stay quiet
	// and not double-report the same structural event.
	if got := countRegressions(regs, JoinTypeChanged); got != 0 {
		t.Fatalf("node type was unchanged; expected 0 JoinTypeChanged, got %d: %+v", got, regs)
	}
}

func TestDetectPlanRegressions_JoinModes_Identical(t *testing.T) {
	baseline := sigWithJoinModes("Inner", "Semi")
	current := sigWithJoinModes("Inner", "Semi")

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, JoinModeChanged); got != 0 {
		t.Fatalf("identical join modes must not regress, got %d: %+v", got, regs)
	}
}

// A qualifier appearing where there was none before (e.g. the planner starts
// emitting "Join Type" on a node that previously omitted it) is still a change
// worth surfacing.
func TestDetectPlanRegressions_JoinModes_OneEmpty(t *testing.T) {
	baseline := sigWithJoinModes()
	current := sigWithJoinModes("Semi")

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, JoinModeChanged); got != 1 {
		t.Fatalf("expected 1 JoinModeChanged regression, got %d: %+v", got, regs)
	}
}

// The eager-aggregation pushdown that the PoC could not see: a plain aggregate
// gains a Partial/Finalize split. With no partial modes on the baseline and a
// Partial/Finalize pair on the current plan, comparePartialModes must flag it.
func TestDetectPlanRegressions_PartialModes_PushdownAppears(t *testing.T) {
	baseline := sigWithPartialModes()
	current := sigWithPartialModes("Partial", "Finalize")

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, PartialModeChanged); got != 1 {
		t.Fatalf("expected 1 PartialModeChanged regression, got %d: %+v", got, regs)
	}
}

func TestDetectPlanRegressions_PartialModes_Identical(t *testing.T) {
	baseline := sigWithPartialModes("Partial", "Finalize")
	current := sigWithPartialModes("Partial", "Finalize")

	regs := DetectPlanRegressions(baseline, current)

	if got := countRegressions(regs, PartialModeChanged); got != 0 {
		t.Fatalf("identical partial modes must not regress, got %d: %+v", got, regs)
	}
}
