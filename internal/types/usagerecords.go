package types

import "time"

// UsageRecordSyncEntry records the outcome of reporting a usage record to one destination (today,
// always a marketplace connection). Stored in usage_records.syncs, keyed by connection_id.
// Marketplace is carried on the entry itself so it reads back self-describing even if the
// connection is later deleted.
//
// An entry is one of two kinds, distinguished by Skipped:
//   - A real acceptance: Skipped is false, ReportingID carries the marketplace's receipt.
//   - A skip: Skipped is true, ReportingID is empty, SkipReason explains why. Used only when
//     sending is deterministically pointless for this provider (e.g. a zero-amount row on Azure,
//     which documents a zero quantity as invalid). This still resolves the connection so the row's
//     synced flag can reach true, without claiming anything was posted.
type UsageRecordSyncEntry struct {
	Marketplace SecretProvider `json:"marketplace"`
	ReportingID string         `json:"reporting_id"`
	SyncedAt    time.Time      `json:"synced_at"`
	Skipped     bool           `json:"skipped,omitempty"`
	SkipReason  string         `json:"skip_reason,omitempty"`
}

// UsageRecordFilter selects rows from usage_records. Tenant and environment are not fields: every
// query is scoped to the caller's context before these filters are applied.
type UsageRecordFilter struct {
	*QueryFilter

	// Bounds created_at, the time the row was written. The usage window the row covers is
	// PeriodStart/PeriodEnd below.
	*TimeRangeFilter

	// Generic predicate and ordering escape hatch, for comparisons that have no dedicated field
	// below — a range such as period_end at or after a cutoff goes here.
	Filters []*FilterCondition `json:"filters,omitempty" form:"filters" validate:"omitempty"`
	Sort    []*SortCondition   `json:"sort,omitempty" form:"sort" validate:"omitempty"`

	SubscriptionID     string `json:"subscription_id,omitempty" form:"subscription_id" validate:"omitempty"`
	CustomerID         string `json:"customer_id,omitempty" form:"customer_id" validate:"omitempty"`
	CustomerExternalID string `json:"customer_external_id,omitempty" form:"customer_external_id" validate:"omitempty"`
	PlanID             string `json:"plan_id,omitempty" form:"plan_id" validate:"omitempty"`
	Currency           string `json:"currency,omitempty" form:"currency" validate:"omitempty"`

	// Exact-match on the window boundaries. Together with SubscriptionID these identify a single row:
	// they are the columns the table's unique index is built on.
	PeriodStart *time.Time `json:"period_start,omitempty" form:"period_start" validate:"omitempty"`
	PeriodEnd   *time.Time `json:"period_end,omitempty" form:"period_end" validate:"omitempty"`

	Synced *bool `json:"synced,omitempty" form:"synced" validate:"omitempty"`
}

// NewUsageRecordFilter creates a new UsageRecordFilter with default pagination.
func NewUsageRecordFilter() *UsageRecordFilter {
	return &UsageRecordFilter{
		QueryFilter: NewDefaultQueryFilter(),
	}
}

// NewNoLimitUsageRecordFilter creates a new UsageRecordFilter with no pagination limits.
func NewNoLimitUsageRecordFilter() *UsageRecordFilter {
	return &UsageRecordFilter{
		QueryFilter: NewNoLimitQueryFilter(),
	}
}

func (f UsageRecordFilter) Validate() error {
	if f.QueryFilter != nil {
		if err := f.QueryFilter.Validate(); err != nil {
			return err
		}
	}
	if f.TimeRangeFilter != nil {
		if err := f.TimeRangeFilter.Validate(); err != nil {
			return err
		}
	}
	for _, c := range f.Filters {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	for _, c := range f.Sort {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// GetLimit implements BaseFilter interface
func (f *UsageRecordFilter) GetLimit() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetLimit()
	}
	return f.QueryFilter.GetLimit()
}

// GetOffset implements BaseFilter interface
func (f *UsageRecordFilter) GetOffset() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOffset()
	}
	return f.QueryFilter.GetOffset()
}

// GetSort implements BaseFilter interface
func (f *UsageRecordFilter) GetSort() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetSort()
	}
	return f.QueryFilter.GetSort()
}

// GetOrder implements BaseFilter interface
func (f *UsageRecordFilter) GetOrder() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOrder()
	}
	return f.QueryFilter.GetOrder()
}

// GetStatus implements BaseFilter interface
func (f *UsageRecordFilter) GetStatus() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetStatus()
	}
	return f.QueryFilter.GetStatus()
}

// IsUnlimited implements BaseFilter interface
func (f *UsageRecordFilter) IsUnlimited() bool {
	if f.QueryFilter == nil {
		return false
	}
	return f.QueryFilter.IsUnlimited()
}
