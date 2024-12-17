package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// Metadata represents a JSONB field for storing key-value pairs
type Metadata map[string]string

// Scan implements the sql.Scanner interface for Metadata
func (m *Metadata) Scan(value interface{}) error {
	if value == nil {
		*m = make(Metadata)
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal JSONB value: %v", value)
	}

	result := make(Metadata)
	err := json.Unmarshal(bytes, &result)
	*m = result
	return err
}

// Value implements the driver.Valuer interface for Metadata
func (m Metadata) Value() (driver.Value, error) {
	if m == nil {
		return json.Marshal(make(Metadata))
	}
	return json.Marshal(m)
}
