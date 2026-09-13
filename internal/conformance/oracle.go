package conformance

import "errors"

type OracleExpectation struct { ClaimID string; Expected Status }
type OracleScore struct { TruePositive int; TrueNegative int; FalsePositive int; FalseNegative int }

// ScoreFrozen compares an already-frozen result to an external qualification oracle.
// There is deliberately no oracle parameter on Evaluate.
func ScoreFrozen(result Result, oracle []OracleExpectation) (OracleScore,error) {
	if result.Digest==""||result.FrozenAt.IsZero(){return OracleScore{},errors.New("result must be frozen before oracle comparison")}
	actual:=map[string]Status{}; for _,f:=range result.Findings{actual[f.ClaimID]=f.Status}
	var s OracleScore
	for _,o:=range oracle {
		a,ok:=actual[o.ClaimID]; if !ok { s.FalseNegative++; continue }
		expectedFailure:=o.Expected!=Satisfied; actualFailure:=a!=Satisfied
		switch { case expectedFailure&&actualFailure:s.TruePositive++; case !expectedFailure&&!actualFailure:s.TrueNegative++; case !expectedFailure&&actualFailure:s.FalsePositive++; case expectedFailure&&!actualFailure:s.FalseNegative++ }
	}
	return s,nil
}
