package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type OracleExpectation struct {
	ID            string   `json:"id,omitempty"`
	ClaimID       string   `json:"claim_id,omitempty"`
	Statement     string   `json:"statement,omitempty"`
	RequiredTerms []string `json:"required_terms,omitempty"`
	Expected      Status   `json:"expected"`
	BasisRef      string   `json:"basis_ref,omitempty"`
}

// OracleAuthority identifies the qualification context in which an oracle is
// authoritative. The expectation bytes are immutable evidence; this metadata
// is the versioned authority that gives them meaning.
type OracleAuthority struct {
	SchemaVersion        string   `json:"schema_version"`
	OracleID             string   `json:"oracle_id"`
	Generation           int      `json:"generation"`
	Scope                string   `json:"scope"`
	QualificationContext string   `json:"qualification_context"`
	ClaimSetDigest       string   `json:"claim_set_digest"`
	GoalDigest           string   `json:"goal_digest"`
	EffectiveFrom        string   `json:"effective_from"`
	Supersedes           string   `json:"supersedes,omitempty"`
	SupersessionReason   string   `json:"supersession_reason,omitempty"`
	ProvenanceRefs       []string `json:"provenance_refs"`
	OraclePath           string   `json:"oracle_path,omitempty"`
	OracleDigest         string   `json:"oracle_digest,omitempty"`
}

// VersionedOracle is the current wire representation for a qualification
// oracle. Historical array-shaped oracle files remain readable only through a
// separately bound OracleAuthority manifest.
type VersionedOracle struct {
	OracleAuthority
	Expectations []OracleExpectation `json:"expectations"`
}

const (
	OracleSchemaVersion   = "v1"
	OracleScopeHistorical = "historical_replay"
	OracleScopeRelease    = "current_release"
)

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

func (a OracleAuthority) Validate() error {
	if a.SchemaVersion != OracleSchemaVersion || a.OracleID == "" || a.Generation < 1 || a.QualificationContext == "" || a.ClaimSetDigest == "" || a.GoalDigest == "" || a.EffectiveFrom == "" {
		return errors.New("oracle authority is incomplete or has an unsupported schema")
	}
	if a.Scope != OracleScopeHistorical && a.Scope != OracleScopeRelease {
		return fmt.Errorf("unknown oracle scope %q", a.Scope)
	}
	if _, err := time.Parse(time.RFC3339, a.EffectiveFrom); err != nil {
		return fmt.Errorf("oracle effective_from: %w", err)
	}
	if a.Supersedes != "" && a.SupersessionReason == "" {
		return errors.New("oracle supersession reason is required")
	}
	if len(a.ProvenanceRefs) == 0 {
		return errors.New("oracle provenance is required")
	}
	return nil
}

func (o VersionedOracle) ValidateFor(result Result) error {
	if err := o.OracleAuthority.Validate(); err != nil {
		return err
	}
	if result.ClaimSetDigest != o.ClaimSetDigest || result.GoalDigest != o.GoalDigest {
		return errors.New("oracle authority does not bind the frozen result identity")
	}
	if len(o.Expectations) == 0 {
		return errors.New("oracle expectations are required")
	}
	seen := map[string]bool{}
	seenClaims := map[string]bool{}
	findingIDs := map[string]bool{}
	for _, finding := range result.Findings {
		findingIDs[finding.ClaimID] = true
	}
	for _, expectation := range o.Expectations {
		if expectation.ID == "" || seen[expectation.ID] || expectation.BasisRef == "" {
			return errors.New("oracle expectations require unique ids and independent basis references")
		}
		seen[expectation.ID] = true
		if expectation.ClaimID == "" && len(expectation.RequiredTerms) == 0 {
			return errors.New("oracle expectation requires claim identity or semantic terms")
		}
		if expectation.ClaimID != "" && !findingIDs[expectation.ClaimID] {
			return fmt.Errorf("oracle expectation references unknown claim %q", expectation.ClaimID)
		}
		if expectation.ClaimID != "" {
			if seenClaims[expectation.ClaimID] {
				return fmt.Errorf("oracle expectation duplicates claim %q", expectation.ClaimID)
			}
			seenClaims[expectation.ClaimID] = true
		}
		switch expectation.Expected {
		case Satisfied, Unsupported, Contradicted, Indeterminate:
		default:
			return fmt.Errorf("oracle expectation %q has unknown expected status %q", expectation.ID, expectation.Expected)
		}
	}
	if o.Scope == OracleScopeRelease && len(o.Expectations) != len(result.Findings) {
		return fmt.Errorf("release oracle must cover current denominator: got %d expectations for %d findings", len(o.Expectations), len(result.Findings))
	}
	if o.Scope == OracleScopeRelease && len(seenClaims) != len(result.Findings) {
		return errors.New("release oracle must bind each current claim exactly once")
	}
	return nil
}

// LoadOracle reads a versioned oracle envelope. Legacy array-shaped bytes are
// accepted only when an authority manifest binds their exact digest and path;
// this preserves historical replay without granting unversioned bytes release
// authority.
func LoadOracle(root, oraclePath, authorityPath string) (VersionedOracle, error) {
	oracleBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(oraclePath)))
	if err != nil {
		return VersionedOracle{}, err
	}
	trimmed := strings.TrimSpace(string(oracleBytes))
	if strings.HasPrefix(trimmed, "[") {
		if authorityPath == "" {
			return VersionedOracle{}, errors.New("legacy oracle requires an authority manifest")
		}
		manifestBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(authorityPath)))
		if err != nil {
			return VersionedOracle{}, err
		}
		var authority OracleAuthority
		if err := json.Unmarshal(manifestBytes, &authority); err != nil {
			return VersionedOracle{}, fmt.Errorf("decode oracle authority: %w", err)
		}
		if err := authority.Validate(); err != nil {
			return VersionedOracle{}, err
		}
		if authority.OraclePath != filepath.ToSlash(oraclePath) {
			return VersionedOracle{}, errors.New("oracle authority path does not match oracle bytes")
		}
		sum := sha256.Sum256(oracleBytes)
		got := "sha256:" + hex.EncodeToString(sum[:])
		if authority.ClaimSetDigest == "" || authority.OracleDigest == "" {
			return VersionedOracle{}, errors.New("legacy oracle authority must bind claim-set and oracle digests")
		}
		if authority.OracleDigest != got {
			return VersionedOracle{}, fmt.Errorf("legacy oracle digest mismatch: got %s want %s", got, authority.OracleDigest)
		}
		var expectations []OracleExpectation
		if err := json.Unmarshal(oracleBytes, &expectations); err != nil {
			return VersionedOracle{}, fmt.Errorf("decode legacy oracle: %w", err)
		}
		for i := range expectations {
			if expectations[i].BasisRef == "" {
				expectations[i].BasisRef = authority.QualificationContext
			}
		}
		return VersionedOracle{OracleAuthority: authority, Expectations: expectations}, nil
	}
	var oracle VersionedOracle
	if err := json.Unmarshal(oracleBytes, &oracle); err != nil {
		return VersionedOracle{}, fmt.Errorf("decode versioned oracle: %w", err)
	}
	if authorityPath != "" {
		return VersionedOracle{}, errors.New("separate authority manifests are only valid for legacy oracle bytes")
	}
	return oracle, nil
}

func ScoreQualified(result Result, oracle VersionedOracle) (OracleScore, error) {
	if err := VerifyFrozen(result); err != nil {
		return OracleScore{}, err
	}
	if err := oracle.ValidateFor(result); err != nil {
		return OracleScore{}, err
	}
	return ScoreFrozen(result, oracle.Expectations)
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
