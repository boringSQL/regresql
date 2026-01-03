package regresql

import "math"

type (
	// ExplainOutput is the top-level structure from EXPLAIN (FORMAT JSON)
	ExplainOutput struct {
		Plan          PlanNode    `json:"Plan"`
		Planning      BufferStats `json:"Planning,omitempty"`
		PlanningTime  float64     `json:"Planning Time,omitempty"`
		ExecutionTime float64     `json:"Execution Time,omitempty"`
	}

	// PlanNode represents a node in the query execution plan
	PlanNode struct {
		// Core fields (always present)
		NodeType       string  `json:"Node Type"`
		StartupCost    float64 `json:"Startup Cost"`
		TotalCost      float64 `json:"Total Cost"`
		PlanRows       float64 `json:"Plan Rows"`
		PlanWidth      int     `json:"Plan Width"`
		ParallelAware  bool    `json:"Parallel Aware,omitempty"`
		AsyncCapable   bool    `json:"Async Capable,omitempty"`

		// Relationship to parent
		ParentRelationship string `json:"Parent Relationship,omitempty"`

		// Scan node fields
		RelationName  string `json:"Relation Name,omitempty"`
		Alias         string `json:"Alias,omitempty"`
		ScanDirection string `json:"Scan Direction,omitempty"`
		IndexName     string `json:"Index Name,omitempty"`

		// Conditions
		IndexCond   string `json:"Index Cond,omitempty"`
		Filter      string `json:"Filter,omitempty"`
		RecheckCond string `json:"Recheck Cond,omitempty"`
		MergeCond   string `json:"Merge Cond,omitempty"`
		HashCond    string `json:"Hash Cond,omitempty"`
		JoinFilter  string `json:"Join Filter,omitempty"`

		// Join fields
		JoinType    string `json:"Join Type,omitempty"`
		InnerUnique bool   `json:"Inner Unique,omitempty"`

		// Sort fields
		SortKey       []string `json:"Sort Key,omitempty"`
		SortMethod    string   `json:"Sort Method,omitempty"`
		SortSpaceUsed int64    `json:"Sort Space Used,omitempty"`
		SortSpaceType string   `json:"Sort Space Type,omitempty"`

		// Parallel/Gather fields
		WorkersPlanned  int  `json:"Workers Planned,omitempty"`
		WorkersLaunched int  `json:"Workers Launched,omitempty"`
		SingleCopy      bool `json:"Single Copy,omitempty"`

		// Bitmap scan fields
		ExactHeapBlocks int64 `json:"Exact Heap Blocks,omitempty"`
		LossyHeapBlocks int64 `json:"Lossy Heap Blocks,omitempty"`
		HeapFetches     int64 `json:"Heap Fetches,omitempty"`

		// Append fields
		SubplansRemoved int `json:"Subplans Removed,omitempty"`

		// Row removal stats
		RowsRemovedByFilter       int64 `json:"Rows Removed by Filter,omitempty"`
		RowsRemovedByIndexRecheck int64 `json:"Rows Removed by Index Recheck,omitempty"`

		// ANALYZE fields (only present with ANALYZE true)
		ActualStartupTime float64 `json:"Actual Startup Time,omitempty"`
		ActualTotalTime   float64 `json:"Actual Total Time,omitempty"`
		ActualRows        int64   `json:"Actual Rows,omitempty"`
		ActualLoops       int64   `json:"Actual Loops,omitempty"`

		// BUFFERS fields (only present with BUFFERS true)
		SharedHitBlocks     int64   `json:"Shared Hit Blocks,omitempty"`
		SharedReadBlocks    int64   `json:"Shared Read Blocks,omitempty"`
		SharedDirtiedBlocks int64   `json:"Shared Dirtied Blocks,omitempty"`
		SharedWrittenBlocks int64   `json:"Shared Written Blocks,omitempty"`
		LocalHitBlocks      int64   `json:"Local Hit Blocks,omitempty"`
		LocalReadBlocks     int64   `json:"Local Read Blocks,omitempty"`
		LocalDirtiedBlocks  int64   `json:"Local Dirtied Blocks,omitempty"`
		LocalWrittenBlocks  int64   `json:"Local Written Blocks,omitempty"`
		TempReadBlocks      int64   `json:"Temp Read Blocks,omitempty"`
		TempWrittenBlocks   int64   `json:"Temp Written Blocks,omitempty"`
		IOReadTime          float64 `json:"I/O Read Time,omitempty"`
		IOWriteTime         float64 `json:"I/O Write Time,omitempty"`
		TempIOReadTime      float64 `json:"Temp I/O Read Time,omitempty"`
		TempIOWriteTime     float64 `json:"Temp I/O Write Time,omitempty"`

		// Child plans
		Plans []PlanNode `json:"Plans,omitempty"`
	}

	// BufferStats holds buffer I/O statistics from EXPLAIN BUFFERS
	BufferStats struct {
		SharedHitBlocks     int64   `json:"Shared Hit Blocks,omitempty"`
		SharedReadBlocks    int64   `json:"Shared Read Blocks,omitempty"`
		SharedDirtiedBlocks int64   `json:"Shared Dirtied Blocks,omitempty"`
		SharedWrittenBlocks int64   `json:"Shared Written Blocks,omitempty"`
		LocalHitBlocks      int64   `json:"Local Hit Blocks,omitempty"`
		LocalReadBlocks     int64   `json:"Local Read Blocks,omitempty"`
		LocalDirtiedBlocks  int64   `json:"Local Dirtied Blocks,omitempty"`
		LocalWrittenBlocks  int64   `json:"Local Written Blocks,omitempty"`
		TempReadBlocks      int64   `json:"Temp Read Blocks,omitempty"`
		TempWrittenBlocks   int64   `json:"Temp Written Blocks,omitempty"`
		IOReadTime          float64 `json:"I/O Read Time,omitempty"`
		IOWriteTime         float64 `json:"I/O Write Time,omitempty"`
		TempIOReadTime      float64 `json:"Temp I/O Read Time,omitempty"`
		TempIOWriteTime     float64 `json:"Temp I/O Write Time,omitempty"`
	}

	// RowEstimate compares planner estimate vs actual rows
	RowEstimate struct {
		NodeType     string  `json:"node_type"`
		RelationName string  `json:"relation_name,omitempty"`
		PlanRows     float64 `json:"plan_rows"`
		ActualRows   int64   `json:"actual_rows"`
		ActualLoops  int64   `json:"actual_loops"`
		Ratio        float64 `json:"ratio"`
	}

	// RowEstimateAnalysis holds all row estimate comparisons
	RowEstimateAnalysis struct {
		Estimates  []RowEstimate `json:"estimates"`
		WorstOver  *RowEstimate  `json:"worst_overestimate,omitempty"`
		WorstUnder *RowEstimate  `json:"worst_underestimate,omitempty"`
	}

	// PlanMetrics contains performance metrics from EXPLAIN ANALYZE.
	// Timing fields require ANALYZE, buffer fields require BUFFERS option.
	PlanMetrics struct {
		TotalCost       float64 `json:"total_cost"`
		ExecutionTimeMs float64 `json:"execution_time_ms"`
		PlanningTimeMs  float64 `json:"planning_time_ms"`
		ActualRows      int64   `json:"actual_rows"`

		// Buffer stats (root node only)
		SharedHitBlocks   int64 `json:"shared_hit_blocks"`
		SharedReadBlocks  int64 `json:"shared_read_blocks"`
		LocalHitBlocks    int64 `json:"local_hit_blocks"`
		LocalReadBlocks   int64 `json:"local_read_blocks"`
		TempReadBlocks    int64 `json:"temp_read_blocks"`
		TempWrittenBlocks int64 `json:"temp_written_blocks"`
		TotalBuffers      int64 `json:"total_buffers"`
		IOReadTimeMs      float64 `json:"io_read_time_ms"`
		IOWriteTimeMs     float64 `json:"io_write_time_ms"`
	}
)

// ExtractMetrics extracts performance metrics from the root plan node.
func (e *ExplainOutput) ExtractMetrics() PlanMetrics {
	return PlanMetrics{
		TotalCost:         e.Plan.TotalCost,
		ExecutionTimeMs:   e.ExecutionTime,
		PlanningTimeMs:    e.PlanningTime,
		ActualRows:        e.Plan.ActualRows,
		SharedHitBlocks:   e.Plan.SharedHitBlocks,
		SharedReadBlocks:  e.Plan.SharedReadBlocks,
		LocalHitBlocks:    e.Plan.LocalHitBlocks,
		LocalReadBlocks:   e.Plan.LocalReadBlocks,
		TempReadBlocks:    e.Plan.TempReadBlocks,
		TempWrittenBlocks: e.Plan.TempWrittenBlocks,
		TotalBuffers:      e.Plan.SharedHitBlocks + e.Plan.SharedReadBlocks,
		IOReadTimeMs:      e.Plan.IOReadTime,
		IOWriteTimeMs:     e.Plan.IOWriteTime,
	}
}

// GetBufferStats returns buffer statistics for a plan node
func (n *PlanNode) GetBufferStats() BufferStats {
	return BufferStats{
		SharedHitBlocks:     n.SharedHitBlocks,
		SharedReadBlocks:    n.SharedReadBlocks,
		SharedDirtiedBlocks: n.SharedDirtiedBlocks,
		SharedWrittenBlocks: n.SharedWrittenBlocks,
		LocalHitBlocks:      n.LocalHitBlocks,
		LocalReadBlocks:     n.LocalReadBlocks,
		LocalDirtiedBlocks:  n.LocalDirtiedBlocks,
		LocalWrittenBlocks:  n.LocalWrittenBlocks,
		TempReadBlocks:      n.TempReadBlocks,
		TempWrittenBlocks:   n.TempWrittenBlocks,
		IOReadTime:          n.IOReadTime,
		IOWriteTime:         n.IOWriteTime,
		TempIOReadTime:      n.TempIOReadTime,
		TempIOWriteTime:     n.TempIOWriteTime,
	}
}

// CompareRowEstimates walks the plan tree and compares Plan Rows vs Actual Rows
func (e *ExplainOutput) CompareRowEstimates() *RowEstimateAnalysis {
	analysis := &RowEstimateAnalysis{}
	collectNodeRowEstimates(&e.Plan, analysis)

	for i := range analysis.Estimates {
		est := &analysis.Estimates[i]
		if est.Ratio > 1 {
			if analysis.WorstUnder == nil || est.Ratio > analysis.WorstUnder.Ratio {
				analysis.WorstUnder = est
			}
		} else if est.Ratio < 1 && est.Ratio > 0 {
			if analysis.WorstOver == nil || est.Ratio < analysis.WorstOver.Ratio {
				analysis.WorstOver = est
			}
		}
	}

	return analysis
}

func collectNodeRowEstimates(node *PlanNode, analysis *RowEstimateAnalysis) {
	if node.ActualLoops > 0 {
		ratio := 0.0
		if node.PlanRows > 0 {
			ratio = float64(node.ActualRows) / node.PlanRows
		} else if node.ActualRows > 0 {
			ratio = math.Inf(1)
		}

		est := RowEstimate{
			NodeType:     node.NodeType,
			RelationName: node.RelationName,
			PlanRows:     node.PlanRows,
			ActualRows:   node.ActualRows,
			ActualLoops:  node.ActualLoops,
			Ratio:        ratio,
		}
		analysis.Estimates = append(analysis.Estimates, est)
	}

	for i := range node.Plans {
		collectNodeRowEstimates(&node.Plans[i], analysis)
	}
}

func toInt64(v any) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	default:
		return 0
	}
}
