package goals

import "errors"

// BaselineFromProse is not implemented yet: this is the RED predicate stub.
func BaselineFromProse(goalID, goalVersion string, document []byte) (GoalBaseline, error) {
	return GoalBaseline{}, errors.New("not implemented")
}
