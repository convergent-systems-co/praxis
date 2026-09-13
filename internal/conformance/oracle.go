package conformance

import "strings"

type OracleExpectation struct {
	ID            string   `json:"id,omitempty"`
	ClaimID       string   `json:"claim_id,omitempty"`
	Statement     string   `json:"statement,omitempty"`
	RequiredTerms []string `json:"required_terms,omitempty"`
	Expected      Status   `json:"expected"`
}

type OracleMatch struct {
	ExpectationID string `json:"expectation_id"`
	ClaimID       string `json:"claim_id,omitempty"`
	Matched       bool   `json:"matched"`
	Rationale     string `json:"rationale"`
}

type OracleScore struct {
	TruePositive  int           `json:"true_positives"`
	TrueNegative  int           `json:"true_negatives"`
	FalsePositive int           `json:"false_positives"`
	FalseNegative int           `json:"false_negatives"`
	Matches       []OracleMatch `json:"matches"`
}

// ScoreFrozen compares an already-frozen result to an external qualification
// oracle. There is deliberately no oracle parameter on Evaluate.
func ScoreFrozen(result Result, oracle []OracleExpectation) (OracleScore, error) {
	if err := VerifyFrozen(result); err != nil {
		return OracleScore{}, err
	}
	var score OracleScore
	for _, expectation := range oracle {
		finding, ok := matchFinding(result.Findings, expectation)
		if !ok {
			score.FalseNegative++
			score.Matches = append(score.Matches, OracleMatch{ExpectationID: expectation.ID, Rationale: "no frozen finding matched the withheld semantic expectation"})
			continue
		}
		expectedFailure := expectation.Expected != Satisfied
		actualFailure := finding.Status != Satisfied
		switch {
		case expectedFailure && actualFailure:
			score.TruePositive++
		case !expectedFailure && !actualFailure:
			score.TrueNegative++
		case !expectedFailure && actualFailure:
			score.FalsePositive++
		case expectedFailure && !actualFailure:
			score.FalseNegative++
		}
		score.Matches = append(score.Matches, OracleMatch{ExpectationID: expectation.ID, ClaimID: finding.ClaimID, Matched: true, Rationale: "frozen claim statement contains every withheld semantic term; status=" + string(finding.Status)})
	}
	return score, nil
}

func matchFinding(findings []Finding, expectation OracleExpectation) (Finding, bool) {
	if expectation.ClaimID != "" {
		for _, finding := range findings {
			if finding.ClaimID == expectation.ClaimID {
				return finding, true
			}
		}
		return Finding{}, false
	}
	for _, finding := range findings {
		statement := strings.ToLower(finding.Statement)
		matched := len(expectation.RequiredTerms) > 0
		for _, term := range expectation.RequiredTerms {
			if !strings.Contains(statement, strings.ToLower(term)) {
				matched = false
				break
			}
		}
		if matched {
			return finding, true
		}
	}
	return Finding{}, false
}
