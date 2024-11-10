package aggregation

type Type string

const (
	Count       Type = "COUNT"
	Sum         Type = "SUM"
	Avg         Type = "AVG"
	Max         Type = "MAX"
	Min         Type = "MIN"
	CountUnique Type = "COUNT_UNIQUE"
	Latest      Type = "LATEST"
)

// Validate checks if the aggregation type is valid
func (t Type) Validate() bool {
	switch t {
	case Count, Sum, Avg, Max, Min, CountUnique, Latest:
		return true
	default:
		return false
	}
}

// RequiresField returns true if the aggregation type requires a field
func (t Type) RequiresField() bool {
	switch t {
	case Count:
		return false
	default:
		return true
	}
}
