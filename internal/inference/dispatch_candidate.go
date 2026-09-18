package inference

// DispatchBinding identifies the exact active agent work for which a persisted
// issued route may be prepared. It carries no dispatch authority.
type DispatchBinding struct {
	RequestID       string
	SubjectAgentID  string
	AgentGeneration string
	RunID           string
	GraphID         string
	GraphVersion    string
	NodeID          string
	GoalRef         string
}

// DispatchCandidate is replay-verified routing evidence. It is not dispatch
// authority and carries no method capable of invoking an executor.
type DispatchCandidate struct {
	RouteRecordID string
	RequestID     string
	SurfaceID     string
	ExecutorID    string
	ProviderID    string
	WorkContext   string
	TargetScope   string
}
