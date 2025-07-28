package ent

import (
	"context"
	"errors"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/connection"
	"github.com/flexprice/flexprice/ent/predicate"
	"github.com/flexprice/flexprice/ent/schema"
	"github.com/flexprice/flexprice/internal/cache"
	domainConnection "github.com/flexprice/flexprice/internal/domain/connection"
	"github.com/flexprice/flexprice/internal/dsl"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/lib/pq"
)

type connectionRepository struct {
	client    postgres.IClient
	log       *logger.Logger
	queryOpts ConnectionQueryOptions
	cache     cache.Cache
}

func NewConnectionRepository(client postgres.IClient, log *logger.Logger, cache cache.Cache) domainConnection.Repository {
	return &connectionRepository{
		client:    client,
		log:       log,
		queryOpts: ConnectionQueryOptions{},
		cache:     cache,
	}
}

func (r *connectionRepository) Create(ctx context.Context, c *domainConnection.Connection) error {
	client := r.client.Querier(ctx)

	r.log.Debugw("creating connection",
		"connection_id", c.ID,
		"tenant_id", c.TenantID,
		"connection_code", c.ConnectionCode,
	)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "connection", "create", map[string]interface{}{
		"connection_id":   c.ID,
		"connection_code": c.ConnectionCode,
	})
	defer FinishSpan(span)

	// Set environment ID from context if not already set
	if c.EnvironmentID == "" {
		c.EnvironmentID = types.GetEnvironmentID(ctx)
	}

	connection, err := client.Connection.Create().
		SetID(c.ID).
		SetTenantID(c.TenantID).
		SetName(c.Name).
		SetDescription(c.Description).
		SetConnectionCode(c.ConnectionCode).
		SetProviderType(connection.ProviderType(c.ProviderType)).
		SetMetadata(c.Metadata).
		SetNillableSecretID(&c.SecretID).
		SetStatus(string(c.Status)).
		SetCreatedAt(c.CreatedAt).
		SetUpdatedAt(c.UpdatedAt).
		SetCreatedBy(c.CreatedBy).
		SetUpdatedBy(c.UpdatedBy).
		SetEnvironmentID(c.EnvironmentID).
		Save(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsConstraintError(err) {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) {
				if pqErr.Constraint == schema.Idx_tenant_environment_connection_code_unique {
					return ierr.WithError(err).
						WithHint("A connection with this code already exists").
						WithReportableDetails(map[string]any{
							"connection_code": c.ConnectionCode,
						}).
						Mark(ierr.ErrAlreadyExists)
				}
			}
			return ierr.WithError(err).
				WithHint("Failed to create connection").
				WithReportableDetails(map[string]any{
					"connection_code": c.ConnectionCode,
					"provider_type":   c.ProviderType,
				}).
				Mark(ierr.ErrAlreadyExists)
		}
		return ierr.WithError(err).
			WithHint("Failed to create connection").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	*c = *domainConnection.FromEnt(connection)
	return nil
}

func (r *connectionRepository) Get(ctx context.Context, id string) (*domainConnection.Connection, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "connection", "get", map[string]interface{}{
		"connection_id": id,
	})
	defer FinishSpan(span)

	// Try to get from cache first
	if cachedConnection := r.GetCache(ctx, id); cachedConnection != nil {
		return cachedConnection, nil
	}

	client := r.client.Querier(ctx)
	r.log.Debugw("getting connection", "connection_id", id)

	c, err := client.Connection.Query().
		Where(
			connection.ID(id),
			connection.TenantID(types.GetTenantID(ctx)),
			connection.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		Only(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsNotFound(err) {
			return nil, ierr.WithError(err).
				WithHintf("Connection with ID %s was not found", id).
				WithReportableDetails(map[string]any{
					"connection_id": id,
				}).
				Mark(ierr.ErrNotFound)
		}
		return nil, ierr.WithError(err).
			WithHint("Failed to get connection").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	domainConn := domainConnection.FromEnt(c)

	// Cache the result
	r.SetCache(ctx, domainConn)

	return domainConn, nil
}

func (r *connectionRepository) GetByConnectionCode(ctx context.Context, connectionCode string) (*domainConnection.Connection, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "connection", "get_by_code", map[string]interface{}{
		"connection_code": connectionCode,
	})
	defer FinishSpan(span)

	// Try to get from cache first
	if cachedConnection := r.GetCache(ctx, connectionCode); cachedConnection != nil {
		return cachedConnection, nil
	}

	client := r.client.Querier(ctx)
	r.log.Debugw("getting connection by code", "connection_code", connectionCode)

	c, err := client.Connection.Query().
		Where(
			connection.ConnectionCode(connectionCode),
			connection.TenantID(types.GetTenantID(ctx)),
			connection.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		Only(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsNotFound(err) {
			return nil, ierr.WithError(err).
				WithHintf("Connection with code %s was not found", connectionCode).
				WithReportableDetails(map[string]any{
					"connection_code": connectionCode,
				}).
				Mark(ierr.ErrNotFound)
		}
		return nil, ierr.WithError(err).
			WithHint("Failed to get connection by code").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	domainConn := domainConnection.FromEnt(c)

	// Cache the result
	r.SetCache(ctx, domainConn)

	return domainConn, nil
}

func (r *connectionRepository) GetByEnvironmentAndProvider(ctx context.Context, environmentID string, provider types.SecretProvider) (*domainConnection.Connection, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "connection", "get_by_env_provider", map[string]interface{}{
		"environment_id": environmentID,
		"provider_type":  provider,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	r.log.Debugw("getting connection by environment and provider", 
		"environment_id", environmentID, 
		"provider_type", provider)

	c, err := client.Connection.Query().
		Where(
			connection.EnvironmentID(environmentID),
			connection.ProviderTypeEQ(connection.ProviderType(provider)),
			connection.TenantID(types.GetTenantID(ctx)),
		).
		Only(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsNotFound(err) {
			return nil, ierr.WithError(err).
				WithHintf("Connection with environment ID %s and provider %s was not found", environmentID, provider).
				WithReportableDetails(map[string]any{
					"environment_id": environmentID,
					"provider_type":  provider,
				}).
				Mark(ierr.ErrNotFound)
		}
		return nil, ierr.WithError(err).
			WithHint("Failed to get connection by environment and provider").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	domainConn := domainConnection.FromEnt(c)

	// Cache the result
	r.SetCache(ctx, domainConn)

	return domainConn, nil
}

func (r *connectionRepository) List(ctx context.Context, filter *types.ConnectionFilter) ([]*domainConnection.Connection, error) {
	client := r.client.Querier(ctx)

	query := client.Connection.Query()
	query = r.queryOpts.ApplyTenantFilter(ctx, query)
	query = r.queryOpts.ApplyEnvironmentFilter(ctx, query)
	query = r.queryOpts.ApplyStatusFilter(query, string(types.StatusPublished))

	query, err := r.queryOpts.applyEntityQueryOptions(ctx, filter, query)
	if err != nil {
		return nil, err
	}

	connections, err := query.All(ctx)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list connections").
			Mark(ierr.ErrDatabase)
	}

	var result []*domainConnection.Connection
	for _, c := range connections {
		result = append(result, domainConnection.FromEnt(c))
	}

	return result, nil
}

func (r *connectionRepository) Count(ctx context.Context, filter *types.ConnectionFilter) (int, error) {
	client := r.client.Querier(ctx)

	query := client.Connection.Query()
	query = r.queryOpts.ApplyTenantFilter(ctx, query)
	query = r.queryOpts.ApplyEnvironmentFilter(ctx, query)
	query = r.queryOpts.ApplyStatusFilter(query, string(types.StatusPublished))

	query, err := r.queryOpts.applyEntityQueryOptions(ctx, filter, query)
	if err != nil {
		return 0, err
	}

	count, err := query.Count(ctx)
	if err != nil {
		return 0, ierr.WithError(err).
			WithHint("Failed to count connections").
			Mark(ierr.ErrDatabase)
	}

	return count, nil
}

func (r *connectionRepository) Update(ctx context.Context, c *domainConnection.Connection) error {
	client := r.client.Querier(ctx)

	r.log.Debugw("updating connection",
		"connection_id", c.ID,
		"tenant_id", c.TenantID,
	)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "connection", "update", map[string]interface{}{
		"connection_id": c.ID,
	})
	defer FinishSpan(span)

	connection, err := client.Connection.UpdateOneID(c.ID).
		Where(
			connection.TenantID(c.TenantID),
			connection.EnvironmentID(c.EnvironmentID),
		).
		SetName(c.Name).
		SetDescription(c.Description).
		SetConnectionCode(c.ConnectionCode).
		SetProviderType(connection.ProviderType(c.ProviderType)).
		SetMetadata(c.Metadata).
		SetNillableSecretID(&c.SecretID).
		SetStatus(string(c.Status)).
		SetUpdatedAt(c.UpdatedAt).
		SetUpdatedBy(c.UpdatedBy).
		Save(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsNotFound(err) {
			return ierr.WithError(err).
				WithHintf("Connection with ID %s was not found", c.ID).
				WithReportableDetails(map[string]any{
					"connection_id": c.ID,
				}).
				Mark(ierr.ErrNotFound)
		}

		if ent.IsConstraintError(err) {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) {
				if pqErr.Constraint == schema.Idx_tenant_environment_connection_code_unique {
					return ierr.WithError(err).
						WithHint("A connection with this code already exists").
						WithReportableDetails(map[string]any{
							"connection_code": c.ConnectionCode,
						}).
						Mark(ierr.ErrAlreadyExists)
				}
			}
			return ierr.WithError(err).
				WithHint("Failed to update connection").
				Mark(ierr.ErrAlreadyExists)
		}

		return ierr.WithError(err).
			WithHint("Failed to update connection").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	*c = *domainConnection.FromEnt(connection)

	// Update cache
	r.SetCache(ctx, c)

	return nil
}

func (r *connectionRepository) Delete(ctx context.Context, domainConnection *domainConnection.Connection) error {
	client := r.client.Querier(ctx)

	r.log.Debugw("deleting connection",
		"connection_id", domainConnection.ID,
		"tenant_id", domainConnection.TenantID,
	)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "connection", "delete", map[string]interface{}{
		"connection_id": domainConnection.ID,
	})
	defer FinishSpan(span)

	err := client.Connection.UpdateOneID(domainConnection.ID).
		Where(
			connection.TenantID(domainConnection.TenantID),
			connection.EnvironmentID(domainConnection.EnvironmentID),
		).
		SetStatus(string(types.StatusDeleted)).
		SetUpdatedAt(domainConnection.UpdatedAt).
		SetUpdatedBy(domainConnection.UpdatedBy).
		Exec(ctx)

	if err != nil {
		SetSpanError(span, err)

		if ent.IsNotFound(err) {
			return ierr.WithError(err).
				WithHintf("Connection with ID %s was not found", domainConnection.ID).
				WithReportableDetails(map[string]any{
					"connection_id": domainConnection.ID,
				}).
				Mark(ierr.ErrNotFound)
		}

		return ierr.WithError(err).
			WithHint("Failed to delete connection").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)

	// Delete from cache
	r.DeleteCache(ctx, domainConnection)

	return nil
}

// ConnectionQuery type alias for better readability
type ConnectionQuery = *ent.ConnectionQuery

// ConnectionQueryOptions implements BaseQueryOptions for connection queries
type ConnectionQueryOptions struct{}

func (o ConnectionQueryOptions) ApplyTenantFilter(ctx context.Context, query ConnectionQuery) ConnectionQuery {
	return query.Where(connection.TenantID(types.GetTenantID(ctx)))
}

func (o ConnectionQueryOptions) ApplyEnvironmentFilter(ctx context.Context, query ConnectionQuery) ConnectionQuery {
	environmentID := types.GetEnvironmentID(ctx)
	if environmentID != "" {
		return query.Where(connection.EnvironmentID(environmentID))
	}
	return query
}

func (o ConnectionQueryOptions) ApplyStatusFilter(query ConnectionQuery, status string) ConnectionQuery {
	if status != "" {
		return query.Where(connection.Status(status))
	}
	return query
}

func (o ConnectionQueryOptions) ApplySortFilter(query ConnectionQuery, field string, order string) ConnectionQuery {
	switch field {
	case "created_at":
		if order == "desc" {
			return query.Order(ent.Desc(connection.FieldCreatedAt))
		}
		return query.Order(ent.Asc(connection.FieldCreatedAt))
	case "updated_at":
		if order == "desc" {
			return query.Order(ent.Desc(connection.FieldUpdatedAt))
		}
		return query.Order(ent.Asc(connection.FieldUpdatedAt))
	case "name":
		if order == "desc" {
			return query.Order(ent.Desc(connection.FieldName))
		}
		return query.Order(ent.Asc(connection.FieldName))
	}
	return query
}

func (o ConnectionQueryOptions) ApplyPaginationFilter(query ConnectionQuery, limit int, offset int) ConnectionQuery {
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	return query
}

func (o ConnectionQueryOptions) GetFieldName(field string) string {
	switch field {
	case "created_at":
		return connection.FieldCreatedAt
	case "updated_at":
		return connection.FieldUpdatedAt
	case "name":
		return connection.FieldName
	case "connection_code":
		return connection.FieldConnectionCode
	case "provider_type":
		return connection.FieldProviderType
	case "status":
		return connection.FieldStatus
	default:
		//unknown field
		return ""
	}
}

func (o ConnectionQueryOptions) GetFieldResolver(field string) (string, error) {
	fieldName := o.GetFieldName(field)
	if fieldName == "" {
		return "", ierr.NewErrorf("unknown field name '%s' in connection query", field).
			Mark(ierr.ErrValidation)
	}
	return fieldName, nil
}

func (o ConnectionQueryOptions) applyEntityQueryOptions(_ context.Context, f *types.ConnectionFilter, query ConnectionQuery) (ConnectionQuery, error) {
	var err error
	if f == nil {
		return query, nil
	}

	if f.ConnectionCode != "" {
		query = query.Where(connection.ConnectionCode(f.ConnectionCode))
	}

	if f.ProviderType != "" {
		query = query.Where(connection.ProviderTypeEQ(connection.ProviderType(f.ProviderType)))
	}

	if len(f.ConnectionIDs) > 0 {
		query = query.Where(connection.IDIn(f.ConnectionIDs...))
	}

	if len(f.ConnectionCodes) > 0 {
		query = query.Where(connection.ConnectionCodeIn(f.ConnectionCodes...))
	}

	if f.Filters != nil {
		query, err = dsl.ApplyFilters[ConnectionQuery, predicate.Connection](
			query,
			f.Filters,
			o.GetFieldResolver,
			func(p dsl.Predicate) predicate.Connection { return predicate.Connection(p) },
		)
		if err != nil {
			return nil, err
		}
	}

	// Apply sorts using the generic function
	if f.Sort != nil {
		query, err = dsl.ApplySorts[ConnectionQuery, connection.OrderOption](
			query,
			f.Sort,
			o.GetFieldResolver,
			func(o dsl.OrderFunc) connection.OrderOption { return connection.OrderOption(o) },
		)
		if err != nil {
			return nil, err
		}
	}

	return query, nil
}

func (r *connectionRepository) SetCache(ctx context.Context, connection *domainConnection.Connection) {
	span := cache.StartCacheSpan(ctx, "connection", "set", map[string]interface{}{
		"connection_id": connection.ID,
	})
	defer cache.FinishSpan(span)

	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	// Set both ID and connection code based cache entries
	connIdKey := cache.GenerateKey(cache.PrefixConnection, tenantID, environmentID, connection.ID)
	codeKey := cache.GenerateKey(cache.PrefixConnection, tenantID, environmentID, connection.ConnectionCode)

	r.cache.Set(ctx, connIdKey, connection, cache.ExpiryDefaultInMemory)
	r.cache.Set(ctx, codeKey, connection, cache.ExpiryDefaultInMemory)

	r.log.Debugw("cache set", "id_key", connIdKey, "code_key", codeKey)
}

func (r *connectionRepository) GetCache(ctx context.Context, key string) *domainConnection.Connection {
	span := cache.StartCacheSpan(ctx, "connection", "get", map[string]interface{}{
		"connection_key": key,
	})
	defer cache.FinishSpan(span)

	cacheKey := cache.GenerateKey(cache.PrefixConnection, types.GetTenantID(ctx), types.GetEnvironmentID(ctx), key)
	if value, found := r.cache.Get(ctx, cacheKey); found {
		if connection, ok := value.(*domainConnection.Connection); ok {
			r.log.Debugw("cache hit", "key", cacheKey)
			return connection
		}
	}
	return nil
}

func (r *connectionRepository) DeleteCache(ctx context.Context, connection *domainConnection.Connection) {
	span := cache.StartCacheSpan(ctx, "connection", "delete", map[string]interface{}{
		"connection_id": connection.ID,
	})
	defer cache.FinishSpan(span)

	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	// Delete ID-based cache first
	connIdKey := cache.GenerateKey(cache.PrefixConnection, tenantID, environmentID, connection.ID)
	codeKey := cache.GenerateKey(cache.PrefixConnection, tenantID, environmentID, connection.ConnectionCode)
	r.cache.Delete(ctx, connIdKey)
	r.cache.Delete(ctx, codeKey)
	r.log.Debugw("cache deleted", "id_key", connIdKey, "code_key", codeKey)
}
