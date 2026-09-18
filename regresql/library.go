package regresql

import (
	"context"
	"database/sql"
	"fmt"
)

func (p *Plan) CreateBaselines(ctx context.Context, db *sql.DB, useAnalyze bool) ([]Baseline, []*ExplainOutput, error) {
	baselines := make([]Baseline, len(p.Names))
	fullPlans := make([]*ExplainOutput, len(p.Names))

	for i := range p.Names {
		baseline, fullPlan, err := p.createSingleBaseline(ctx, db, i, useAnalyze)
		if err != nil {
			return nil, nil, err
		}
		baselines[i] = baseline
		fullPlans[i] = fullPlan
	}

	return baselines, fullPlans, nil
}

func (p *Plan) createSingleBaseline(ctx context.Context, db *sql.DB, index int, useAnalyze bool) (Baseline, *ExplainOutput, error) {
	var explainPlan *ExplainOutput
	var err error

	opts := DefaultExplainOptions()
	if useAnalyze {
		opts.Analyze = true
		opts.Buffers = true
	}

	if len(p.Query.Args) == 0 {
		explainPlan, err = ExecuteExplainWithOptions(ctx, db, p.Query.OrdinalQuery, opts)
	} else {
		sql, args := p.Query.Prepare(p.Bindings[index])
		explainPlan, err = ExecuteExplainWithOptions(ctx, db, sql, opts, args...)
	}
	if err != nil {
		return Baseline{}, nil, fmt.Errorf("failed to create baseline for %s: %w", p.Names[index], err)
	}

	filteredPlan := map[string]any{
		"startup_cost": explainPlan.Plan.StartupCost,
		"total_cost":   explainPlan.Plan.TotalCost,
		"plan_rows":    explainPlan.Plan.PlanRows,
	}

	return Baseline{Query: p.Query.Name, Plan: filteredPlan}, explainPlan, nil
}
