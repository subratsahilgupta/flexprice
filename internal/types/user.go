package types

type UserFilter struct {
	*QueryFilter
	*TimeRangeFilter

	// filters allows complex filtering based on multiple fields
	Filters []*FilterCondition `json:"filters,omitempty" form:"filters" validate:"omitempty"`
	Sort    []*SortCondition   `json:"sort,omitempty" form:"sort" validate:"omitempty"`
	
	// Specific filters for users
	UserIDs []string `json:"user_ids,omitempty" form:"user_ids" validate:"omitempty"`
	Type    *string  `json:"type,omitempty" form:"type" validate:"omitempty,oneof=user service_account"`
	Roles   []string `json:"roles,omitempty" form:"roles" validate:"omitempty"`
}

