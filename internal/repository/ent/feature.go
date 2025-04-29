package ent

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/feature"
	"github.com/flexprice/flexprice/ent/predicate"
	domainFeature "github.com/flexprice/flexprice/internal/domain/feature"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type featureRepository struct {
	client    postgres.IClient
	log       *logger.Logger
	queryOpts FeatureQueryOptions
}

func NewFeatureRepository(client postgres.IClient, log *logger.Logger) domainFeature.Repository {
	return &featureRepository{
		client:    client,
		log:       log,
		queryOpts: FeatureQueryOptions{},
	}
}

func (r *featureRepository) Create(ctx context.Context, f *domainFeature.Feature) error {
	client := r.client.Querier(ctx)

	r.log.Debugw("creating feature",
		"feature_id", f.ID,
		"tenant_id", f.TenantID,
		"name", f.Name,
		"lookup_key", f.LookupKey,
	)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "feature", "create", map[string]interface{}{
		"feature_id": f.ID,
		"name":       f.Name,
		"lookup_key": f.LookupKey,
	})
	defer FinishSpan(span)

	// Set environment ID from context if not already set
	if f.EnvironmentID == "" {
		f.EnvironmentID = types.GetEnvironmentID(ctx)
	}

	feature, err := client.Feature.Create().
		SetID(f.ID).
		SetName(f.Name).
		SetType(string(f.Type)).
		SetDescription(f.Description).
		SetLookupKey(f.LookupKey).
		SetStatus(string(f.Status)).
		SetUnitSingular(f.UnitSingular).
		SetUnitPlural(f.UnitPlural).
		SetMetadata(map[string]string(f.Metadata)).
		SetMeterID(f.MeterID).
		SetTenantID(f.TenantID).
		SetCreatedAt(f.CreatedAt).
		SetUpdatedAt(f.UpdatedAt).
		SetCreatedBy(f.CreatedBy).
		SetUpdatedBy(f.UpdatedBy).
		SetEnvironmentID(f.EnvironmentID).
		Save(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsConstraintError(err) {
			return ierr.WithError(err).
				WithHint("A feature with this lookup key already exists").
				WithReportableDetails(map[string]any{
					"lookup_key": f.LookupKey,
				}).
				Mark(ierr.ErrAlreadyExists)
		}
		return ierr.WithError(err).
			WithHint("Failed to create feature").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	*f = *domainFeature.FromEnt(feature)
	return nil
}

func (r *featureRepository) Get(ctx context.Context, id string) (*domainFeature.Feature, error) {
	client := r.client.Querier(ctx)

	r.log.Debugw("getting feature",
		"feature_id", id,
		"tenant_id", types.GetTenantID(ctx),
	)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "feature", "get", map[string]interface{}{
		"feature_id": id,
	})
	defer FinishSpan(span)

	f, err := client.Feature.Query().
		Where(
			feature.ID(id),
			feature.TenantID(types.GetTenantID(ctx)),
		).
		Only(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsNotFound(err) {
			return nil, ierr.WithError(err).
				WithHintf("Feature with ID %s was not found", id).
				WithReportableDetails(map[string]any{
					"feature_id": id,
				}).
				Mark(ierr.ErrNotFound)
		}
		return nil, ierr.WithError(err).
			WithHintf("Failed to get feature with ID %s", id).
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return domainFeature.FromEnt(f), nil
}

func (r *featureRepository) List(ctx context.Context, filter *types.FeatureFilter) ([]*domainFeature.Feature, error) {
	if filter == nil {
		filter = &types.FeatureFilter{
			QueryFilter: types.NewDefaultQueryFilter(),
		}
	}

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "feature", "list", map[string]interface{}{
		"filter": filter,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	query := client.Feature.Query()

	// Apply entity-specific filters
	query = r.queryOpts.applyEntityQueryOptions(ctx, filter, query)

	// Apply common query options
	query = ApplyQueryOptions(ctx, query, filter, r.queryOpts)

	features, err := query.All(ctx)
	if err != nil {
		SetSpanError(span, err)
		return nil, ierr.WithError(err).
			WithHint("Failed to list features").
			WithReportableDetails(map[string]any{
				"filter": filter,
			}).
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return domainFeature.FromEntList(features), nil
}

func (r *featureRepository) Count(ctx context.Context, filter *types.FeatureFilter) (int, error) {
	client := r.client.Querier(ctx)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "feature", "count", map[string]interface{}{
		"filter": filter,
	})
	defer FinishSpan(span)

	query := client.Feature.Query()

	query = ApplyBaseFilters(ctx, query, filter, r.queryOpts)
	query = r.queryOpts.applyEntityQueryOptions(ctx, filter, query)

	count, err := query.Count(ctx)
	if err != nil {
		SetSpanError(span, err)
		return 0, ierr.WithError(err).
			WithHint("Failed to count features").
			WithReportableDetails(map[string]any{
				"filter": filter,
			}).
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return count, nil
}

func (r *featureRepository) ListAll(ctx context.Context, filter *types.FeatureFilter) ([]*domainFeature.Feature, error) {
	if filter == nil {
		filter = types.NewNoLimitFeatureFilter()
	}

	if filter.QueryFilter == nil {
		filter.QueryFilter = types.NewNoLimitQueryFilter()
	}

	if !filter.IsUnlimited() {
		filter.QueryFilter.Limit = nil
	}

	if err := filter.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid filter parameters").
			Mark(ierr.ErrValidation)
	}

	return r.List(ctx, filter)
}

func (r *featureRepository) Update(ctx context.Context, f *domainFeature.Feature) error {
	client := r.client.Querier(ctx)

	r.log.Debugw("updating feature",
		"feature_id", f.ID,
		"tenant_id", f.TenantID,
	)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "feature", "update", map[string]interface{}{
		"feature_id": f.ID,
	})
	defer FinishSpan(span)

	_, err := client.Feature.Update().
		Where(
			feature.ID(f.ID),
			feature.TenantID(f.TenantID),
		).
		SetName(f.Name).
		SetDescription(f.Description).
		SetStatus(string(f.Status)).
		SetUnitSingular(f.UnitSingular).
		SetUnitPlural(f.UnitPlural).
		SetMetadata(map[string]string(f.Metadata)).
		SetMeterID(f.MeterID).
		SetUpdatedAt(time.Now().UTC()).
		SetUpdatedBy(types.GetUserID(ctx)).
		Save(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsNotFound(err) {
			return ierr.WithError(err).
				WithHintf("Feature with ID %s was not found", f.ID).
				WithReportableDetails(map[string]any{
					"feature_id": f.ID,
				}).
				Mark(ierr.ErrNotFound)
		}
		return ierr.WithError(err).
			WithHint("Failed to update feature").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return nil
}

func (r *featureRepository) Delete(ctx context.Context, id string) error {
	client := r.client.Querier(ctx)

	r.log.Debugw("deleting feature",
		"feature_id", id,
		"tenant_id", types.GetTenantID(ctx),
	)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "feature", "delete", map[string]interface{}{
		"feature_id": id,
	})
	defer FinishSpan(span)

	_, err := client.Feature.Update().
		Where(
			feature.ID(id),
			feature.TenantID(types.GetTenantID(ctx)),
		).
		SetStatus(string(types.StatusArchived)).
		SetUpdatedAt(time.Now().UTC()).
		SetUpdatedBy(types.GetUserID(ctx)).
		Save(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsNotFound(err) {
			return ierr.WithError(err).
				WithHintf("Feature with ID %s was not found", id).
				WithReportableDetails(map[string]any{
					"feature_id": id,
				}).
				Mark(ierr.ErrNotFound)
		}
		return ierr.WithError(err).
			WithHint("Failed to delete feature").
			WithReportableDetails(map[string]any{
				"feature_id": id,
			}).
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return nil
}

// ListByIDs retrieves features by their IDs
func (r *featureRepository) ListByIDs(ctx context.Context, featureIDs []string) ([]*domainFeature.Feature, error) {
	if len(featureIDs) == 0 {
		return []*domainFeature.Feature{}, nil
	}

	r.log.Debugw("listing features by IDs", "feature_ids", featureIDs)

	// Create a filter with feature IDs
	filter := &types.FeatureFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
		FeatureIDs:  featureIDs,
	}

	// Use the existing List method
	return r.List(ctx, filter)
}

// FeatureQuery type alias for better readability
type FeatureQuery = *ent.FeatureQuery

// FeatureQueryOptions implements query options for feature filtering and sorting
type FeatureQueryOptions struct{}

// FieldPredicates defines the available predicates for each field type
type FieldPredicates struct {
	// String predicates
	Eq       func(string) predicate.Feature
	Neq      func(string) predicate.Feature
	Contains func(string) predicate.Feature
	Prefix   func(string) predicate.Feature
	Suffix   func(string) predicate.Feature
	In       func(...string) predicate.Feature
	NotIn    func(...string) predicate.Feature

	// Date predicates
	DateEq  func(time.Time) predicate.Feature
	DateNeq func(time.Time) predicate.Feature
	DateGt  func(time.Time) predicate.Feature
	DateLt  func(time.Time) predicate.Feature
}

func (o FeatureQueryOptions) ApplyTenantFilter(ctx context.Context, query FeatureQuery) FeatureQuery {
	return query.Where(feature.TenantID(types.GetTenantID(ctx)))
}

func (o FeatureQueryOptions) ApplyEnvironmentFilter(ctx context.Context, query FeatureQuery) FeatureQuery {
	environmentID := types.GetEnvironmentID(ctx)
	if environmentID != "" {
		return query.Where(feature.EnvironmentIDEQ(environmentID))
	}
	return query
}

func (o FeatureQueryOptions) ApplyStatusFilter(query FeatureQuery, status string) FeatureQuery {
	if status == "" {
		return query.Where(feature.StatusNotIn(string(types.StatusDeleted)))
	}
	return query.Where(feature.Status(status))
}

func (o FeatureQueryOptions) ApplySortFilter(query FeatureQuery, field string, order string) FeatureQuery {
	orderFunc := ent.Desc
	if order == "asc" {
		orderFunc = ent.Asc
	}
	return query.Order(orderFunc(o.GetFieldName(field)))
}

func (o FeatureQueryOptions) ApplyPaginationFilter(query FeatureQuery, limit int, offset int) FeatureQuery {
	query = query.Limit(limit)
	if offset > 0 {
		query = query.Offset(offset)
	}
	return query
}

func (o FeatureQueryOptions) GetFieldName(field string) string {
	switch field {
	case "created_at":
		return feature.FieldCreatedAt
	case "updated_at":
		return feature.FieldUpdatedAt
	case "name":
		return feature.FieldName
	case "lookup_key":
		return feature.FieldLookupKey
	case "type":
		return feature.FieldType
	case "status":
		return feature.FieldStatus
	default:
		return field
	}
}

func (o FeatureQueryOptions) applyEntityQueryOptions(ctx context.Context, f *types.FeatureFilter, query FeatureQuery) FeatureQuery {
	if f == nil {
		return query
	}

	// Apply feature IDs filter if specified
	if len(f.FeatureIDs) > 0 {
		query = query.Where(feature.IDIn(f.FeatureIDs...))
	}

	// Apply meter IDs filter if specified
	if len(f.MeterIDs) > 0 {
		query = query.Where(feature.MeterIDIn(f.MeterIDs...))
	}

	// Apply key filter if specified
	if f.LookupKey != "" {
		query = query.Where(feature.LookupKey(f.LookupKey))
	}

	if f.NameContains != "" {
		query = query.Where(feature.NameContainsFold(f.NameContains))
	}

	// Apply time range filters if specified
	if f.TimeRangeFilter != nil {
		if f.StartTime != nil {
			query = query.Where(feature.CreatedAtGTE(*f.StartTime))
		}
		if f.EndTime != nil {
			query = query.Where(feature.CreatedAtLTE(*f.EndTime))
		}
	}

	// apply filters
	if f.Filters != nil {
		query = o.applyFilterConditions(ctx, query, f.Filters)
	}

	// apply sort
	if f.Sort != nil {
		query = o.applySortConditions(ctx, query, f.Sort)
	}

	return query
}

// applyFilterConditions applies filter conditions to the query
func (o FeatureQueryOptions) applyFilterConditions(_ context.Context, query FeatureQuery, filters []*types.FilterCondition) FeatureQuery {
	if len(filters) == 0 {
		return query
	}

	for _, filter := range filters {
		if filter == nil {
			continue
		}

		fieldName := o.GetFieldName(filter.Field)
		if fieldName == "" {
			continue
		}

		// Convert value to appropriate type
		var value interface{}
		switch filter.DataType {
		case types.DataTypeString:
			value = filter.ValueString
		case types.DataTypeDate:
			value = filter.ValueDate
		case types.DataTypeArray:
			value = filter.ValueArray
		}

		// Apply the appropriate predicate based on operator
		switch filter.Operator {
		case types.EQUAL:
			query = query.Where(predicate.Feature(sql.FieldEQ(fieldName, value)))
		case types.CONTAINS:
			if strValue, ok := value.(string); ok {
				query = query.Where(predicate.Feature(sql.FieldContains(fieldName, strValue)))
			}
		case types.GREATER_THAN:
			query = query.Where(predicate.Feature(sql.FieldGT(fieldName, value)))
		case types.LESS_THAN:
			query = query.Where(predicate.Feature(sql.FieldLT(fieldName, value)))
		case types.IS_ANY_OF:
			if arrValue, ok := value.([]interface{}); ok {
				query = query.Where(predicate.Feature(sql.FieldIn(fieldName, arrValue...)))
			}
		case types.IS_NOT_ANY_OF:
			if arrValue, ok := value.([]interface{}); ok {
				query = query.Where(predicate.Feature(sql.FieldNotIn(fieldName, arrValue...)))
			}
		case types.BEFORE:
			if dateValue, ok := value.(time.Time); ok {
				query = query.Where(predicate.Feature(sql.FieldLT(fieldName, dateValue)))
			}
		case types.AFTER:
			if dateValue, ok := value.(time.Time); ok {
				query = query.Where(predicate.Feature(sql.FieldGT(fieldName, dateValue)))
			}
		}
	}

	return query
}

// applySortConditions applies sort conditions to the query
func (o FeatureQueryOptions) applySortConditions(_ context.Context, query FeatureQuery, sort []*types.SortCondition) FeatureQuery {
	if len(sort) == 0 {
		return query
	}

	for _, s := range sort {
		if s == nil {
			continue
		}

		fieldName := o.GetFieldName(s.Field)
		if fieldName == "" {
			continue
		}

		orderFunc := ent.Desc
		if s.Direction == types.SortDirectionAsc {
			orderFunc = ent.Asc
		}

		query = query.Order(orderFunc(fieldName))
	}

	return query
}
