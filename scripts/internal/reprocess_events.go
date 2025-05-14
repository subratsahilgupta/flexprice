package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/flexprice/flexprice/internal/cache"
	"github.com/flexprice/flexprice/internal/clickhouse"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	chRepo "github.com/flexprice/flexprice/internal/repository/clickhouse"
	entRepo "github.com/flexprice/flexprice/internal/repository/ent"
	"github.com/flexprice/flexprice/internal/sentry"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
)

// ReprocessEventsScript holds all dependencies for the script
type ReprocessEventsScript struct {
	log                        *logger.Logger
	eventPostProcessingService service.EventPostProcessingService
}

// ReprocessEvents triggers reprocessing of events
func ReprocessEvents() error {
	// Get required environment variables
	tenantID := os.Getenv("TENANT_ID")
	environmentID := os.Getenv("ENVIRONMENT_ID")

	if tenantID == "" || environmentID == "" {
		return fmt.Errorf("TENANT_ID and ENVIRONMENT_ID environment variables are required")
	}

	// Get optional environment variables
	externalCustomerID := "672efbfb-91fe-4603-ba3f-e7ce804c9eb7" // os.Getenv("EXTERNAL_CUSTOMER_ID")
	eventName := os.Getenv("EVENT_NAME")
	startTimeStr := os.Getenv("START_TIME") // format: 2006-01-02T15:04:05Z
	endTimeStr := os.Getenv("END_TIME")     // format: 2006-01-02T15:04:05Z
	batchSizeStr := os.Getenv("BATCH_SIZE")

	// Initialize the script
	script, err := newReprocessEventsScript()
	if err != nil {
		return fmt.Errorf("failed to initialize script: %w", err)
	}

	log.Printf("Starting event reprocessing for tenant: %s, environment: %s", tenantID, environmentID)
	if externalCustomerID != "" {
		log.Printf("Filtering by external customer ID: %s", externalCustomerID)
	}
	if eventName != "" {
		log.Printf("Filtering by event name: %s", eventName)
	}

	// Create context with tenant and environment
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.CtxTenantID, tenantID)
	ctx = context.WithValue(ctx, types.CtxEnvironmentID, environmentID)

	// Parse date parameters
	var startTime, endTime time.Time
	if startTimeStr != "" {
		startTime, err = time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			return fmt.Errorf("invalid START_TIME format, use ISO-8601 (2006-01-02T15:04:05Z): %w", err)
		}
	}
	if endTimeStr != "" {
		endTime, err = time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			return fmt.Errorf("invalid END_TIME format, use ISO-8601 (2006-01-02T15:04:05Z): %w", err)
		}
	}

	// Parse batch size
	batchSize := 100 // default
	if batchSizeStr != "" {
		if _, err := fmt.Sscanf(batchSizeStr, "%d", &batchSize); err != nil {
			return fmt.Errorf("invalid BATCH_SIZE, must be an integer: %w", err)
		}
	}

	// Prepare reprocessing parameters
	params := &events.ReprocessEventsParams{
		ExternalCustomerID: externalCustomerID,
		EventName:          eventName,
		StartTime:          startTime,
		EndTime:            endTime,
		BatchSize:          batchSize,
	}

	// Execute reprocessing
	if err := script.eventPostProcessingService.ReprocessEvents(ctx, params); err != nil {
		return fmt.Errorf("event reprocessing failed: %w", err)
	}

	log.Println("Event reprocessing completed successfully")
	return nil
}

// Initialize all services and dependencies
func newReprocessEventsScript() (*ReprocessEventsScript, error) {
	// Load configuration
	cfg, err := config.NewConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize logger
	log, err := logger.NewLogger(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Initialize Postgres client for customer, meter, feature repositories
	entClient, err := postgres.NewEntClient(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	pgClient := postgres.NewClient(entClient, log, sentry.NewSentryService(cfg, log))
	cacheClient := cache.NewInMemoryCache()

	// Initialize ClickHouse client for event repositories
	sentryService := sentry.NewSentryService(cfg, log)
	chStore, err := clickhouse.NewClickHouseStore(cfg, sentryService)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to clickhouse: %w", err)
	}

	// Initialize repositories
	eventRepo := chRepo.NewEventRepository(chStore, log)
	processedEventRepo := chRepo.NewProcessedEventRepository(chStore, log)
	customerRepo := entRepo.NewCustomerRepository(pgClient, log, cacheClient)
	meterRepo := entRepo.NewMeterRepository(pgClient, log, cacheClient)
	priceRepo := entRepo.NewPriceRepository(pgClient, log, cacheClient)
	featureRepo := entRepo.NewFeatureRepository(pgClient, log, cacheClient)

	// Create service parameters
	serviceParams := service.ServiceParams{
		Config:       cfg,
		Logger:       log,
		CustomerRepo: customerRepo,
		MeterRepo:    meterRepo,
		PriceRepo:    priceRepo,
		FeatureRepo:  featureRepo,
	}

	// Initialize event post-processing service
	eventPostProcessingService := service.NewEventPostProcessingService(
		serviceParams,
		eventRepo,
		processedEventRepo,
	)

	return &ReprocessEventsScript{
		log:                        log,
		eventPostProcessingService: eventPostProcessingService,
	}, nil
}
