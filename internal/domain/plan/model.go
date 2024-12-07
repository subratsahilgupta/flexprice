package plan

import (
	"github.com/flexprice/flexprice/internal/types"
)

type Plan struct {
	ID          string `db:"id"`
	Name        string `db:"name"`
	Description string `db:"description"`
	types.BaseModel
}
