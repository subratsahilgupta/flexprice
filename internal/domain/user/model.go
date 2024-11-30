package user

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/google/uuid"
)

type User struct {
	ID    string `db:"id" json:"id"`
	Email string `db:"email" json:"email"`
	types.BaseModel
}

func NewUser(email, tenantID string) *User {
	return &User{
		ID:    uuid.New().String(),
		Email: email,
		BaseModel: types.BaseModel{
			TenantID:  tenantID,
			CreatedBy: types.DefaultUserID,
			UpdatedBy: types.DefaultUserID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
}
