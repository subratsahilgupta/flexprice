package ent

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
	_ "github.com/lib/pq"
	"go.uber.org/fx"
)

// Module provides an fx.Option to integrate Ent client with the application
func Module() fx.Option {
	return fx.Options(
		fx.Provide(
			NewEntClient,
		),
	)
}

// NewEntClient creates a new Ent client
func NewEntClient(config *config.Configuration, logger *logger.Logger) (*ent.Client, error) {
	// Get DSN from config
	dsn := config.Postgres.GetDSN()

	// Open PostgreSQL connection
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(config.Postgres.MaxOpenConns)
	db.SetMaxIdleConns(config.Postgres.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(config.Postgres.ConnMaxLifetimeMinutes) * time.Minute)

	// Create driver
	drv := entsql.OpenDB(dialect.Postgres, db)

	// Create client with options
	opts := []ent.Option{
		ent.Driver(drv),
		ent.Debug(), // Enable debug logging
	}

	client := ent.NewClient(opts...)

	// Run the auto migration tool if enabled
	if config.Postgres.AutoMigrate {
		if err := client.Schema.Create(context.Background()); err != nil {
			return nil, fmt.Errorf("failed creating schema resources: %w", err)
		}
	}

	return client, nil
}
