package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// uuidToString converts a pgtype.UUID to its string representation.
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

// parsePipelineIssueID converts a pipeline issue ID (string or []byte) to pgtype.UUID.
func parsePipelineIssueID(id any) pgtype.UUID {
	var uuid pgtype.UUID
	switch v := id.(type) {
	case string:
		if err := uuid.Scan(v); err != nil {
			return pgtype.UUID{}
		}
	case []byte:
		if len(v) == 16 {
			copy(uuid.Bytes[:], v)
			uuid.Valid = true
		} else {
			if err := uuid.Scan(string(v)); err != nil {
				return pgtype.UUID{}
			}
		}
	}
	return uuid
}

// extractIssuePrefix returns a short identifier for branch naming.
// Uses issue number if available, otherwise first 8 chars of UUID.
func extractIssuePrefix(issue db.Issue) string {
	if issue.Number > 0 {
		return fmt.Sprintf("MUL-%d", issue.Number)
	}
	uuidStr := fmt.Sprintf("%x", issue.ID.Bytes)
	if len(uuidStr) > 8 {
		return uuidStr[:8]
	}
	return uuidStr
}

// parseScoreFromResult extracts score information from task result JSONB.
func parseScoreFromResult(result []byte) *Score {
	if result == nil {
		return &Score{Total: 50} // default score when no result
	}

	var res map[string]any
	if err := json.Unmarshal(result, &res); err != nil {
		return &Score{Total: 50}
	}

	score := &Score{}

	// Try to extract lint/test results from output
	if output, ok := res["output"].(string); ok {
		// Simple heuristic parsing — will be improved
		if strings.Contains(output, "lint") || strings.Contains(output, "eslint") {
			if !strings.Contains(output, "error") {
				score.LintPass = true
			}
		}
		// Count test results if present
		if strings.Contains(output, "passing") || strings.Contains(output, "passed") {
			score.TestPassRate = 0.8 // placeholder
		}
	}

	// Compute total score
	score.Total = computeScore(score)
	return score
}

// computeScore calculates total score from individual metrics.
func computeScore(s *Score) float64 {
	total := 0.0

	// Test pass rate: 35% weight
	total += s.TestPassRate * 35.0

	// Lint: 20% weight
	if s.LintPass {
		total += 20.0
	}

	// Diff precision: 20% weight (less is better, capped at 200 lines)
	diffLines := float64(s.DiffAdded + s.DiffRemoved)
	if diffLines > 0 && diffLines < 200 {
		total += (1.0 - diffLines/200.0) * 20.0
	} else if diffLines == 0 {
		total += 10.0 // partial credit if we don't know
	}

	// Token efficiency: 15% weight (less is better, capped at 10000)
	if s.TokenConsumed > 0 && s.TokenConsumed < 10000 {
		total += (1.0 - float64(s.TokenConsumed)/10000.0) * 15.0
	} else if s.TokenConsumed == 0 {
		total += 7.5
	}

	// Time efficiency: 10% weight (less is better, capped at 60s)
	if s.DurationMs > 0 && s.DurationMs < 60000 {
		total += (1.0 - float64(s.DurationMs)/60000.0) * 10.0
	} else if s.DurationMs == 0 {
		total += 5.0
	}

	return total
}

// marshalEdgeMeta creates the metadata JSONB for an edge including score.
func marshalEdgeMeta(score *Score, pctx *PipelineContext) []byte {
	meta := map[string]any{
		"transition": fmt.Sprintf("%s → next", pctx.Stage),
		"variant":    pctx.Variant,
		"branch":     pctx.BranchName,
	}
	if score != nil {
		meta["score"] = score
	}
	data, _ := json.Marshal(meta)
	return data
}
