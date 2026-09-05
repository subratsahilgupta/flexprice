package types

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
)

func TestCustomerFilter_GetStatus(t *testing.T) {
	statusInPublishedAndArchived := []*FilterCondition{
		{
			Field:    lo.ToPtr("status"),
			Operator: lo.ToPtr(IN),
			DataType: lo.ToPtr(DataTypeArray),
			Value:    &Value{Array: []string{string(StatusPublished), string(StatusArchived)}},
		},
	}

	t.Run("defaults to published when no status is specified", func(t *testing.T) {
		f := NewCustomerFilter()
		assert.Equal(t, string(StatusPublished), f.GetStatus())
	})

	t.Run("skips default when DSL filters status so archived can be included", func(t *testing.T) {
		f := NewCustomerFilter()
		f.Filters = statusInPublishedAndArchived
		assert.Equal(t, "", f.GetStatus())
	})

	t.Run("honors explicit query status over DSL", func(t *testing.T) {
		f := NewCustomerFilter()
		f.Status = lo.ToPtr(StatusArchived)
		f.Filters = statusInPublishedAndArchived
		assert.Equal(t, string(StatusArchived), f.GetStatus())
	})

	t.Run("empty query status is treated as unset", func(t *testing.T) {
		f := NewCustomerFilter()
		f.Status = lo.ToPtr(Status(""))
		assert.Equal(t, string(StatusPublished), f.GetStatus())
	})
}
