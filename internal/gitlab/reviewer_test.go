package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yourorg/code-reviewer/internal/logging"
	"github.com/yourorg/code-reviewer/internal/postgres"
)

func TestReviewer_PostReview_DryRun(t *testing.T) {
	client := NewClient(Config{
		BaseURL: "http://localhost",
		Token:   "test-token",
	})

	reviewer := NewReviewer(client, ReviewerConfig{
		Enabled:            true,
		PostSummary:        true,
		PostInlineComments: true,
		MinSeverity:        "low",
		DryRun:             true,
		Logger:             logging.NewDefault(),
	})

	input := ReviewInput{
		ProjectID: 1,
		MRIID:     1,
		BaseSHA:   "abc123",
		HeadSHA:   "def456",
		PerformanceIssues: []postgres.Issue{
			{
				Severity:   "HIGH",
				Problem:    "Missing index",
				Suggestion: "Add index",
				File:       "test.go",
				Line:       10,
			},
		},
	}

	result, err := reviewer.PostReview(context.Background(), input)
	if err != nil {
		t.Fatalf("PostReview() error = %v", err)
	}

	// In dry run, summary is logged but not actually posted
	if result.SummaryPosted {
		t.Error("SummaryPosted should be false in dry run (nothing actually sent)")
	}
	// No errors in dry run
	if len(result.Errors) > 0 {
		t.Errorf("Errors = %v, want none", result.Errors)
	}
}

func TestReviewer_PostReview_Disabled(t *testing.T) {
	client := NewClient(Config{
		BaseURL: "http://localhost",
		Token:   "test-token",
	})

	reviewer := NewReviewer(client, ReviewerConfig{
		Enabled: false,
		Logger:  logging.NewDefault(),
	})

	input := ReviewInput{
		ProjectID: 1,
		MRIID:     1,
		PerformanceIssues: []postgres.Issue{
			{Severity: "HIGH", Problem: "Test"},
		},
	}

	result, err := reviewer.PostReview(context.Background(), input)
	if err != nil {
		t.Fatalf("PostReview() error = %v", err)
	}

	if result.SummaryPosted || result.InlineComments > 0 {
		t.Error("Nothing should be posted when disabled")
	}
}

func TestReviewer_PostReview_NoIssues(t *testing.T) {
	client := NewClient(Config{
		BaseURL: "http://localhost",
		Token:   "test-token",
	})

	reviewer := NewReviewer(client, ReviewerConfig{
		Enabled: true,
		Logger:  logging.NewDefault(),
	})

	input := ReviewInput{
		ProjectID: 1,
		MRIID:     1,
	}

	result, err := reviewer.PostReview(context.Background(), input)
	if err != nil {
		t.Fatalf("PostReview() error = %v", err)
	}

	if result.SummaryPosted {
		t.Error("SummaryPosted should be false when no issues")
	}
}

func TestReviewer_PostReview_WithServer(t *testing.T) {
	notesCreated := 0
	discussionsCreated := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/notes") {
			notesCreated++
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(Note{ID: 1, Body: "test"})
			return
		}
		if strings.Contains(r.URL.Path, "/discussions") {
			discussionsCreated++
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(Discussion{ID: "d1"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	reviewer := NewReviewer(client, ReviewerConfig{
		Enabled:            true,
		PostSummary:        true,
		PostInlineComments: true,
		MinSeverity:        "low",
		CollapseThreshold:  20,
		DryRun:             false,
		Logger:             logging.NewDefault(),
	})

	input := ReviewInput{
		ProjectID: 1,
		MRIID:     1,
		BaseSHA:   "abc",
		HeadSHA:   "def",
		StartSHA:  "abc",
		PerformanceIssues: []postgres.Issue{
			{
				Severity:   "HIGH",
				Problem:    "Missing index",
				Suggestion: "Add index",
				File:       "test.go",
				Line:       10,
			},
		},
		SecurityIssues: []postgres.Issue{
			{
				Severity:   "CRITICAL",
				Problem:    "SQL injection",
				Suggestion: "Use parameterized queries",
				File:       "db.go",
				Line:       20,
				CWE:        "CWE-89",
			},
		},
	}

	result, err := reviewer.PostReview(context.Background(), input)
	if err != nil {
		t.Fatalf("PostReview() error = %v", err)
	}

	if !result.SummaryPosted {
		t.Error("Summary should be posted")
	}
	if notesCreated != 1 {
		t.Errorf("Notes created = %d, want 1", notesCreated)
	}
	if result.InlineComments != 2 {
		t.Errorf("InlineComments = %d, want 2", result.InlineComments)
	}
	if discussionsCreated != 2 {
		t.Errorf("Discussions created = %d, want 2", discussionsCreated)
	}
}

func TestReviewer_PostReview_SeverityFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Note{ID: 1})
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	reviewer := NewReviewer(client, ReviewerConfig{
		Enabled:     true,
		PostSummary: true,
		MinSeverity: "high", // Only HIGH and above
		DryRun:      true,
		Logger:      logging.NewDefault(),
	})

	input := ReviewInput{
		ProjectID: 1,
		MRIID:     1,
		PerformanceIssues: []postgres.Issue{
			{Severity: "LOW", Problem: "Minor issue"},
			{Severity: "MEDIUM", Problem: "Medium issue"},
			{Severity: "HIGH", Problem: "High issue"},
			{Severity: "CRITICAL", Problem: "Critical issue"},
		},
	}

	result, err := reviewer.PostReview(context.Background(), input)
	if err != nil {
		t.Fatalf("PostReview() error = %v", err)
	}

	// In dry run, nothing is actually posted but we can check there are no errors
	// (issues are filtered but would be logged)
	if len(result.Errors) > 0 {
		t.Errorf("Errors = %v, want none", result.Errors)
	}
}

func TestReviewer_PostReview_CollapseThreshold(t *testing.T) {
	discussionsCreated := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/discussions") {
			discussionsCreated++
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Note{ID: 1})
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	reviewer := NewReviewer(client, ReviewerConfig{
		Enabled:            true,
		PostSummary:        true,
		PostInlineComments: true,
		MinSeverity:        "low",
		CollapseThreshold:  2, // Only post inline if <= 2 issues
		DryRun:             false,
		Logger:             logging.NewDefault(),
	})

	// Create 5 issues - exceeds threshold
	issues := make([]postgres.Issue, 5)
	for i := range issues {
		issues[i] = postgres.Issue{
			Severity: "HIGH",
			Problem:  "Issue",
			File:     "test.go",
			Line:     i + 1,
		}
	}

	input := ReviewInput{
		ProjectID:         1,
		MRIID:             1,
		BaseSHA:           "abc",
		HeadSHA:           "def",
		PerformanceIssues: issues,
	}

	result, err := reviewer.PostReview(context.Background(), input)
	if err != nil {
		t.Fatalf("PostReview() error = %v", err)
	}

	// Should not post inline comments because threshold exceeded
	if result.InlineComments > 0 {
		t.Errorf("InlineComments = %d, want 0 (threshold exceeded)", result.InlineComments)
	}
	if discussionsCreated > 0 {
		t.Errorf("Discussions created = %d, want 0", discussionsCreated)
	}
}

func TestReviewer_formatSummary(t *testing.T) {
	reviewer := NewReviewer(nil, ReviewerConfig{
		Logger: logging.NewDefault(),
	})

	issues := []categorizedIssue{
		{Issue: postgres.Issue{Severity: "HIGH", Problem: "Performance problem"}, Category: "Performance"},
		{Issue: postgres.Issue{Severity: "CRITICAL", Problem: "Security vulnerability", CWE: "CWE-89"}, Category: "Security"},
		{Issue: postgres.Issue{Severity: "MEDIUM", Problem: "Style issue"}, Category: "Standards"},
	}

	summary := reviewer.formatSummary(issues)

	// Check contains expected elements
	checks := []string{
		"Code Review Analysis",
		"Performance",
		"Security",
		"Standards",
		"HIGH",
		"CRITICAL",
		"MEDIUM",
		"Performance problem",
		"Security vulnerability",
		"CWE-89",
	}

	for _, check := range checks {
		if !strings.Contains(summary, check) {
			t.Errorf("Summary should contain %q", check)
		}
	}
}

func TestReviewer_formatInlineComment(t *testing.T) {
	reviewer := NewReviewer(nil, ReviewerConfig{
		Logger: logging.NewDefault(),
	})

	issue := categorizedIssue{
		Issue: postgres.Issue{
			Severity:    "HIGH",
			Problem:     "Missing index on users.email",
			Impact:      "Slow queries",
			Suggestion:  "CREATE INDEX idx_users_email ON users(email)",
			Reference:   "https://example.com/docs",
		},
		Category: "Performance",
	}

	comment := reviewer.formatInlineComment(issue)

	checks := []string{
		"HIGH",
		"Performance Issue",
		"Missing index",
		"Slow queries",
		"CREATE INDEX",
		"https://example.com/docs",
	}

	for _, check := range checks {
		if !strings.Contains(comment, check) {
			t.Errorf("Comment should contain %q", check)
		}
	}
}

func TestSeverityIcon(t *testing.T) {
	tests := []struct {
		severity string
		want     string
	}{
		{"CRITICAL", "🔴"},
		{"critical", "🔴"},
		{"HIGH", "🟠"},
		{"MEDIUM", "🟡"},
		{"LOW", "🔵"},
		{"unknown", "⚪"},
	}

	for _, tt := range tests {
		got := severityIcon(tt.severity)
		if got != tt.want {
			t.Errorf("severityIcon(%q) = %q, want %q", tt.severity, got, tt.want)
		}
	}
}

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"CRITICAL", 4},
		{"critical", 4},
		{"HIGH", 3},
		{"ERROR", 3},
		{"MEDIUM", 2},
		{"WARNING", 2},
		{"LOW", 1},
		{"INFO", 1},
		{"unknown", 0},
	}

	for _, tt := range tests {
		got := parseSeverity(tt.input)
		if got != tt.want {
			t.Errorf("parseSeverity(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestDefaultReviewerConfig(t *testing.T) {
	cfg := DefaultReviewerConfig()

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if !cfg.PostSummary {
		t.Error("PostSummary should be true")
	}
	if !cfg.PostInlineComments {
		t.Error("PostInlineComments should be true")
	}
	if cfg.MinSeverity != "low" {
		t.Errorf("MinSeverity = %q, want %q", cfg.MinSeverity, "low")
	}
	if cfg.CollapseThreshold != 20 {
		t.Errorf("CollapseThreshold = %d, want 20", cfg.CollapseThreshold)
	}
	if cfg.DryRun {
		t.Error("DryRun should be false")
	}
}

func TestReviewer_PostReview_SkipNoLine(t *testing.T) {
	reviewer := NewReviewer(nil, ReviewerConfig{
		Enabled:            true,
		PostSummary:        false,
		PostInlineComments: true,
		MinSeverity:        "low",
		CollapseThreshold:  10,
		DryRun:             true,
		Logger:             logging.NewDefault(),
	})

	input := ReviewInput{
		ProjectID: 1,
		MRIID:     1,
		BaseSHA:   "abc",
		HeadSHA:   "def",
		PerformanceIssues: []postgres.Issue{
			{Severity: "HIGH", Problem: "No line", Line: 0, File: "test.go"},
			{Severity: "HIGH", Problem: "No file", Line: 10, File: ""},
			{Severity: "HIGH", Problem: "Has both", Line: 10, File: "test.go"},
		},
	}

	result, err := reviewer.PostReview(context.Background(), input)
	if err != nil {
		t.Fatalf("PostReview() error = %v", err)
	}

	// Only the issue with both line and file should be posted
	if result.SkippedComments != 2 {
		t.Errorf("SkippedComments = %d, want 2", result.SkippedComments)
	}
}
