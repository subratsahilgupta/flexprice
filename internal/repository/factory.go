package repository

import (
	"github.com/flexprice/flexprice/internal/clickhouse"
	"github.com/flexprice/flexprice/internal/domain/auth"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/user"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	clickhouseRepo "github.com/flexprice/flexprice/internal/repository/clickhouse"
	postgresRepo "github.com/flexprice/flexprice/internal/repository/postgres"
)

type RepositoryType string

const (
	PostgresRepo   RepositoryType = "postgres"
	ClickHouseRepo RepositoryType = "clickhouse"
)

func NewEventRepository(store *clickhouse.ClickHouseStore, logger *logger.Logger) events.Repository {
	return clickhouseRepo.NewEventRepository(store, logger)
}

func NewMeterRepository(db *postgres.DB, logger *logger.Logger) meter.Repository {
	return postgresRepo.NewMeterRepository(db, logger)
}

func NewUserRepository(db *postgres.DB, logger *logger.Logger) user.Repository {
	return postgresRepo.NewUserRepository(db, logger)
}

func NewAuthRepository(db *postgres.DB, logger *logger.Logger) auth.Repository {
	return postgresRepo.NewAuthRepository(db, logger)
}
