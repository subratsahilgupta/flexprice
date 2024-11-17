package types

// Status is a type for the status of a resource (e.g. meter, event) in the Database
// This is used to track the lifecycle of a resource and to determine if it should be included in queries
// Any changes to this type should be reflected in the database schema by running migrations
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
	StatusDeleted  Status = "deleted"
)
