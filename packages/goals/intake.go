package goals

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// intakeAssumption discloses on the Baseline itself that its fields were
// derived, not authored, so a reviewer and the owner see the derivation.
const intakeAssumption = "Baseline derived by goals-lifecycle intake from a prose Goal document: refined_outcome is the document's intent text, not a human-refined statement; rigor and recommendation mode are intake defaults."

// BaselineFromProse derives a root GoalBaseline from a prose Goal document.
// It is a pure, deterministic transformation: it never persists, decides, or
// grants anything. The result enters authority only by crossing the explicit
// import boundary (ADR-068), and a WorkPlan can attach only through the
// propose/review/request/decide/accept/attach topology.
//
// Document convention:
//
//	# Optional title
//
//	Intent text (everything before the first "## " heading) is the refined outcome.
//
//	## Scope            free text
//	## Non-goals        bullet list
//	## Constraints      bullet list
//	## Success criteria bullet list
//
// Any other "## " section is preserved verbatim in the original intent and
// derives nothing. The whole document is the original intent, and its digest
// is bound into the Baseline as evidence. A section that is recognized but
// empty, malformed, or repeated fails closed rather than being dropped.
func BaselineFromProse(goalID, goalVersion string, document []byte) (GoalBaseline, error) {
	if goalID == "" || goalVersion == "" {
		return GoalBaseline{}, errors.New("prose intake requires an exact Goal id and generation version")
	}
	text := string(document)
	if strings.TrimSpace(text) == "" {
		return GoalBaseline{}, errors.New("prose Goal document is empty")
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i < len(lines) && strings.HasPrefix(lines[i], "# ") {
		i++
	}
	var intent []string
	for ; i < len(lines) && !isSectionHeading(lines[i]); i++ {
		intent = append(intent, lines[i])
	}
	refined := strings.TrimSpace(strings.Join(intent, "\n"))
	if refined == "" {
		return GoalBaseline{}, errors.New("prose Goal document has no intent text before its first section")
	}

	bodies := map[string][]string{}
	current := ""
	for ; i < len(lines); i++ {
		if isSectionHeading(lines[i]) {
			name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(lines[i], "## ")))
			current = ""
			if recognizedIntakeSection(name) {
				if _, seen := bodies[name]; seen {
					return GoalBaseline{}, fmt.Errorf("prose Goal document repeats the %q section", name)
				}
				bodies[name] = []string{}
				current = name
			}
			continue
		}
		if current != "" {
			bodies[current] = append(bodies[current], lines[i])
		}
	}

	baseline := GoalBaseline{ID: goalID, Version: goalVersion, OriginalIntent: text, RefinedOutcome: refined, Rigor: RigorStructured, RecommendationMode: RecommendationReviewAll, Assumptions: []string{intakeAssumption}}
	sum := sha256.Sum256(document)
	baseline.EvidenceRefs = []string{"goal-document:sha256:" + hex.EncodeToString(sum[:])}
	for name, body := range bodies {
		switch name {
		case "scope":
			scope := strings.TrimSpace(strings.Join(body, "\n"))
			if scope == "" {
				return GoalBaseline{}, errors.New("prose Goal document has an empty scope section")
			}
			baseline.Scope = scope
		default:
			items, err := intakeBullets(name, body)
			if err != nil {
				return GoalBaseline{}, err
			}
			switch name {
			case "non-goals":
				baseline.NonGoals = items
			case "constraints":
				baseline.Constraints = items
			case "success criteria":
				baseline.SuccessCriteria = items
			}
		}
	}
	if err := baseline.Validate(); err != nil {
		return GoalBaseline{}, err
	}
	return baseline, nil
}

func isSectionHeading(line string) bool { return strings.HasPrefix(line, "## ") }

func recognizedIntakeSection(name string) bool {
	switch name {
	case "scope", "non-goals", "constraints", "success criteria":
		return true
	}
	return false
}

// intakeBullets reads a bullet list. An indented line continues the previous
// bullet; anything else in a list section is refused so nothing is silently
// lost between the prose and the Baseline.
func intakeBullets(section string, body []string) ([]string, error) {
	var items []string
	for _, raw := range body {
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			items = append(items, strings.TrimSpace(line[2:]))
		case (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(items) > 0:
			items[len(items)-1] += " " + strings.TrimSpace(line)
		default:
			return nil, fmt.Errorf("prose Goal document section %q must be a bullet list; found %q", section, strings.TrimSpace(line))
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("prose Goal document has an empty %q section", section)
	}
	return items, nil
}
