package types

type ApplicationStatus string

const (
	ApplicationStatusScheduled ApplicationStatus = "scheduled"
	ApplicationStatusApplied   ApplicationStatus = "applied"
	ApplicationStatusFailed    ApplicationStatus = "failed"
	ApplicationStatusSkipped   ApplicationStatus = "skipped"
	ApplicationStatusCancelled ApplicationStatus = "cancelled"
)
