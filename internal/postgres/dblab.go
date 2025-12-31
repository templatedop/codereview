package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DatabaseLabClient provides integration with Database Lab for testing queries.
type DatabaseLabClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// DatabaseLabConfig holds configuration for Database Lab.
type DatabaseLabConfig struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// NewDatabaseLabClient creates a new Database Lab client.
func NewDatabaseLabClient(cfg DatabaseLabConfig) *DatabaseLabClient {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &DatabaseLabClient{
		baseURL: cfg.BaseURL,
		token:   cfg.Token,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Clone represents a thin database clone.
type Clone struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	ConnectionString string    `json:"connection_string"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
}

// ExplainPlan represents an EXPLAIN ANALYZE result.
type ExplainPlan struct {
	Query         string          `json:"query"`
	Plan          json.RawMessage `json:"plan"`
	ExecutionTime float64         `json:"execution_time_ms"`
	PlanningTime  float64         `json:"planning_time_ms"`
	TotalTime     float64         `json:"total_time_ms"`
	RowsExamined  int64           `json:"rows_examined"`
	SharedHit     int64           `json:"shared_hit_blocks"`
	SharedRead    int64           `json:"shared_read_blocks"`
	TempRead      int64           `json:"temp_read_blocks"`
	TempWritten   int64           `json:"temp_written_blocks"`
	Analysis      *PlanAnalysis   `json:"analysis,omitempty"`
}

// PlanAnalysis contains analyzed execution plan data.
type PlanAnalysis struct {
	HasSeqScan       bool     `json:"has_seq_scan"`
	SeqScanTables    []string `json:"seq_scan_tables,omitempty"`
	HasNestedLoop    bool     `json:"has_nested_loop"`
	HasHashJoin      bool     `json:"has_hash_join"`
	HasMergeJoin     bool     `json:"has_merge_join"`
	HasSort          bool     `json:"has_sort"`
	SortInMemory     bool     `json:"sort_in_memory"`
	EstimatedRows    int64    `json:"estimated_rows"`
	ActualRows       int64    `json:"actual_rows"`
	RowEstimateRatio float64  `json:"row_estimate_ratio"`
	Warnings         []string `json:"warnings,omitempty"`
}

// CreateClone creates a thin database clone.
func (c *DatabaseLabClient) CreateClone(ctx context.Context, name string) (*Clone, error) {
	reqBody := map[string]interface{}{
		"name": name,
		"protected": false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/clones", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create clone failed with status %d: %s", resp.StatusCode, string(body))
	}

	var clone Clone
	if err := json.NewDecoder(resp.Body).Decode(&clone); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &clone, nil
}

// DestroyClone destroys a database clone.
func (c *DatabaseLabClient) DestroyClone(ctx context.Context, cloneID string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+"/api/clones/"+cloneID, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("destroy clone failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// RunExplain runs EXPLAIN ANALYZE on a query using a clone.
func (c *DatabaseLabClient) RunExplain(ctx context.Context, cloneID, query string) (*ExplainPlan, error) {
	reqBody := map[string]interface{}{
		"query": fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) %s", query),
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/clones/"+cloneID+"/query", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("explain failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("no explain plan returned")
	}

	plan := &ExplainPlan{
		Query: query,
		Plan:  result.Rows[0],
	}

	// Parse and analyze the plan
	plan.Analysis = analyzePlan(result.Rows[0])
	plan.extractMetrics()

	return plan, nil
}

// ValidateQuery validates a query using a temporary clone.
func (c *DatabaseLabClient) ValidateQuery(ctx context.Context, query string) (*ExplainPlan, error) {
	// Create temporary clone
	clone, err := c.CreateClone(ctx, fmt.Sprintf("validation-%d", time.Now().Unix()))
	if err != nil {
		return nil, fmt.Errorf("create clone: %w", err)
	}

	// Ensure cleanup
	defer func() {
		_ = c.DestroyClone(context.Background(), clone.ID)
	}()

	// Wait for clone to be ready
	time.Sleep(2 * time.Second)

	// Run explain
	return c.RunExplain(ctx, clone.ID, query)
}

// ValidateQueries validates multiple queries.
func (c *DatabaseLabClient) ValidateQueries(ctx context.Context, queries []string) ([]*ExplainPlan, error) {
	// Create temporary clone
	clone, err := c.CreateClone(ctx, fmt.Sprintf("validation-%d", time.Now().Unix()))
	if err != nil {
		return nil, fmt.Errorf("create clone: %w", err)
	}

	// Ensure cleanup
	defer func() {
		_ = c.DestroyClone(context.Background(), clone.ID)
	}()

	// Wait for clone to be ready
	time.Sleep(2 * time.Second)

	// Run explain for each query
	plans := make([]*ExplainPlan, 0, len(queries))
	for _, query := range queries {
		plan, err := c.RunExplain(ctx, clone.ID, query)
		if err != nil {
			// Log error but continue
			continue
		}
		plans = append(plans, plan)
	}

	return plans, nil
}

func (c *DatabaseLabClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (p *ExplainPlan) extractMetrics() {
	// Parse the JSON plan to extract metrics
	var planData []map[string]interface{}
	if err := json.Unmarshal(p.Plan, &planData); err != nil || len(planData) == 0 {
		return
	}

	plan := planData[0]
	if execTime, ok := plan["Execution Time"].(float64); ok {
		p.ExecutionTime = execTime
	}
	if planTime, ok := plan["Planning Time"].(float64); ok {
		p.PlanningTime = planTime
	}
	p.TotalTime = p.ExecutionTime + p.PlanningTime

	// Extract from Plan node
	if planNode, ok := plan["Plan"].(map[string]interface{}); ok {
		if rows, ok := planNode["Actual Rows"].(float64); ok {
			p.RowsExamined = int64(rows)
		}
		if hit, ok := planNode["Shared Hit Blocks"].(float64); ok {
			p.SharedHit = int64(hit)
		}
		if read, ok := planNode["Shared Read Blocks"].(float64); ok {
			p.SharedRead = int64(read)
		}
	}
}

func analyzePlan(planJSON json.RawMessage) *PlanAnalysis {
	analysis := &PlanAnalysis{}

	var planData []map[string]interface{}
	if err := json.Unmarshal(planJSON, &planData); err != nil || len(planData) == 0 {
		return analysis
	}

	plan := planData[0]
	if planNode, ok := plan["Plan"].(map[string]interface{}); ok {
		analyzeNode(planNode, analysis)
	}

	// Calculate row estimate ratio
	if analysis.EstimatedRows > 0 && analysis.ActualRows > 0 {
		analysis.RowEstimateRatio = float64(analysis.ActualRows) / float64(analysis.EstimatedRows)
		if analysis.RowEstimateRatio > 10 || analysis.RowEstimateRatio < 0.1 {
			analysis.Warnings = append(analysis.Warnings, "Row estimate is significantly off - consider ANALYZE")
		}
	}

	return analysis
}

func analyzeNode(node map[string]interface{}, analysis *PlanAnalysis) {
	nodeType, _ := node["Node Type"].(string)

	switch nodeType {
	case "Seq Scan":
		analysis.HasSeqScan = true
		if tableName, ok := node["Relation Name"].(string); ok {
			analysis.SeqScanTables = append(analysis.SeqScanTables, tableName)
			if rows, ok := node["Actual Rows"].(float64); ok && rows > 10000 {
				analysis.Warnings = append(analysis.Warnings,
					fmt.Sprintf("Sequential scan on %s with %d rows - consider adding index", tableName, int(rows)))
			}
		}
	case "Nested Loop":
		analysis.HasNestedLoop = true
	case "Hash Join":
		analysis.HasHashJoin = true
	case "Merge Join":
		analysis.HasMergeJoin = true
	case "Sort":
		analysis.HasSort = true
		if method, ok := node["Sort Method"].(string); ok {
			analysis.SortInMemory = method == "quicksort" || method == "top-N heapsort"
			if !analysis.SortInMemory {
				analysis.Warnings = append(analysis.Warnings, "Sort spilling to disk - consider increasing work_mem")
			}
		}
	}

	if rows, ok := node["Plan Rows"].(float64); ok {
		analysis.EstimatedRows = int64(rows)
	}
	if rows, ok := node["Actual Rows"].(float64); ok {
		analysis.ActualRows = int64(rows)
	}

	// Recursively analyze child nodes
	if plans, ok := node["Plans"].([]interface{}); ok {
		for _, p := range plans {
			if childNode, ok := p.(map[string]interface{}); ok {
				analyzeNode(childNode, analysis)
			}
		}
	}
}

// IsEnabled checks if Database Lab is configured.
func (c *DatabaseLabClient) IsEnabled() bool {
	return c.baseURL != ""
}
