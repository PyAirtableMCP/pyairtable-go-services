package eventreplay

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/pyairtable-compose/go-services/pkg/cqrs"
	"github.com/pyairtable-compose/go-services/pkg/eventstore"
	"github.com/pyairtable-compose/go-services/pkg/outbox"
	"github.com/pyairtable-compose/go-services/pkg/repository"
)

// IntegratedBackendSystem demonstrates how all patterns work together
type IntegratedBackendSystem struct {
	// CQRS Components
	dbConfig         *cqrs.DatabaseConfig
	projectionMgr    *cqrs.ProjectionManager
	cacheManager     *cqrs.CQRSCacheManager
	
	// Outbox Pattern
	outbox           *outbox.TransactionalOutbox
	
	// Repository Pattern with UoW
	uowFactory       *repository.UnitOfWorkFactory
	connectionPool   *repository.ConnectionPool
	
	// Event Replay
	replayEngine     *ReplayEngine
	snapshotStore    SnapshotStore
}

// NewIntegratedBackendSystem creates a complete backend system with all patterns
func NewIntegratedBackendSystem(writeDB, readDB *sql.DB) (*IntegratedBackendSystem, error) {
	// 1. Setup CQRS with database separation
	dbConfig := &cqrs.DatabaseConfig{
		WriteDB: writeDB,
		ReadDB:  readDB,
	}
	
	// 2. Setup connection pooling
	poolConfig := repository.ConnectionPoolConfig{
		MaxOpenConns:       100,
		MaxIdleConns:       50,
		ConnMaxLifetime:    time.Hour,
		ConnMaxIdleTime:    30 * time.Minute,
		ReadPoolSize:       40,
		WritePoolSize:      30,
		LongRunningPool:    10,
		EnableMetrics:      true,
		SlowQueryThreshold: 500 * time.Millisecond,
		HealthCheckInterval: 1 * time.Minute,
	}
	
	connectionPool, err := repository.NewConnectionPool("your-connection-string", poolConfig)
	if err != nil {
		return nil, err
	}
	
	// 3. Setup projection management
	projectionConfig := cqrs.ProjectionConfig{
		Strategy:        cqrs.AsynchronousProjection,
		BatchSize:       100,
		BatchTimeout:    5 * time.Second,
		WorkerCount:     4,
		RetryAttempts:   3,
		RetryBackoff:    time.Second,
		EnableCaching:   true,
		CacheExpiration: 15 * time.Minute,
	}
	
	projectionMgr := cqrs.NewProjectionManager(dbConfig, nil, projectionConfig)
	
	// 4. Setup outbox pattern
	outboxConfig := outbox.OutboxConfig{
		TableName:           "outbox_events",
		BatchSize:           50,
		PollInterval:        2 * time.Second,
		MaxRetries:          5,
		RetryBackoffBase:    time.Second,
		RetryBackoffMax:     30 * time.Second,
		WorkerCount:         3,
		EnableDeduplication: true,
		PublishTimeout:      30 * time.Second,
	}
	
	// Create a composite publisher with multiple targets
	eventBusPublisher := outbox.NewEventBusPublisher(nil) // Would inject real event bus
	kafkaPublisher := outbox.NewKafkaPublisher([]string{"localhost:9092"}, "domain-events")
	
	// Wrap with circuit breaker and retry logic
	circuitBreakerPublisher := outbox.NewCircuitBreakerPublisher(kafkaPublisher, 5, 1*time.Minute)
	retryablePublisher := outbox.NewRetryablePublisher(circuitBreakerPublisher, 3, 2*time.Second)
	
	compositePublisher := outbox.NewCompositePublisher(
		[]outbox.EventPublisher{eventBusPublisher, retryablePublisher},
		outbox.PublishAll,
	)
	
	transactionalOutbox := outbox.NewTransactionalOutbox(writeDB, outboxConfig, compositePublisher)
	
	// 5. Setup Unit of Work
	uowConfig := repository.UnitOfWorkConfig{
		TransactionTimeout: 30 * time.Second,
		IsolationLevel:     sql.LevelReadCommitted,
		EnableOutbox:       true,
		EnableEventStore:   true,
		MaxRetries:         3,
	}
	
	uowFactory := repository.NewUnitOfWorkFactory(writeDB, uowConfig)
	
	// 6. Setup event replay
	replayConfig := ReplayConfig{
		Strategy:           ParallelReplay,
		BatchSize:          100,
		WorkerCount:        4,
		SnapshotInterval:   1000,
		MaxRetries:         3,
		RetryBackoff:       2 * time.Second,
		EnableSnapshots:    true,
		EnableCompression:  true,
		EnableMetrics:      true,
		ReplayTimeout:      10 * time.Minute,
		ParallelismLevel:   8,
	}
	
	eventStore := eventstore.NewPostgresEventStore(writeDB)
	replayEngine := NewReplayEngine(eventStore, writeDB, replayConfig)
	snapshotStore := NewPostgresSnapshotStore(writeDB)
	
	system := &IntegratedBackendSystem{
		dbConfig:       dbConfig,
		projectionMgr:  projectionMgr,
		outbox:         transactionalOutbox,
		uowFactory:     uowFactory,
		connectionPool: connectionPool,
		replayEngine:   replayEngine,
		snapshotStore:  snapshotStore,
	}
	
	// Register projection handlers
	system.registerProjectionHandlers()
	
	// Register replay handlers
	system.registerReplayHandlers()
	
	return system, nil
}

// registerProjectionHandlers registers handlers for updating read model projections
func (ibs *IntegratedBackendSystem) registerProjectionHandlers() {
	// User projection handler
	userProjectionHandler := &UserProjectionHandler{
		readDB: ibs.dbConfig.GetReadDB(),
	}
	ibs.projectionMgr.RegisterProjection(userProjectionHandler)
	
	// Workspace projection handler
	workspaceProjectionHandler := &WorkspaceProjectionHandler{
		readDB: ibs.dbConfig.GetReadDB(),
	}
	ibs.projectionMgr.RegisterProjection(workspaceProjectionHandler)
	
	// Tenant projection handler
	tenantProjectionHandler := &TenantProjectionHandler{
		readDB: ibs.dbConfig.GetReadDB(),
	}
	ibs.projectionMgr.RegisterProjection(tenantProjectionHandler)
}

// registerReplayHandlers registers handlers for event replay operations
func (ibs *IntegratedBackendSystem) registerReplayHandlers() {
	// Projection rebuild handler
	projectionReplayHandler := NewProjectionReplayHandler(
		"projection_rebuilder",
		[]string{"*"}, // Handle all event types
		ibs.dbConfig.GetReadDB(),
	)
	ibs.replayEngine.RegisterHandler(projectionReplayHandler)
	
	// Aggregate rebuild handler
	aggregateReplayHandler := NewAggregateReplayHandler(
		"aggregate_rebuilder",
		[]string{"user.registered", "user.updated", "workspace.created", "tenant.created"},
	)
	ibs.replayEngine.RegisterHandler(aggregateReplayHandler)
}

// Start initializes and starts all backend components
func (ibs *IntegratedBackendSystem) Start() error {
	log.Println("Starting integrated backend system...")
	
	// Start outbox processing
	if err := ibs.outbox.Start(); err != nil {
		return err
	}
	
	log.Println("Integrated backend system started successfully")
	return nil
}

// Stop gracefully shuts down all components
func (ibs *IntegratedBackendSystem) Stop() error {
	log.Println("Stopping integrated backend system...")
	
	// Stop outbox
	if err := ibs.outbox.Stop(); err != nil {
		log.Printf("Error stopping outbox: %v", err)
	}
	
	// Stop projection manager
	if err := ibs.projectionMgr.Shutdown(); err != nil {
		log.Printf("Error stopping projection manager: %v", err)
	}
	
	// Stop replay engine
	if err := ibs.replayEngine.Shutdown(); err != nil {
		log.Printf("Error stopping replay engine: %v", err)
	}
	
	// Close connection pool
	if err := ibs.connectionPool.Close(); err != nil {
		log.Printf("Error closing connection pool: %v", err)
	}
	
	// Close database connections
	if err := ibs.dbConfig.Close(); err != nil {
		log.Printf("Error closing database connections: %v", err)
	}
	
	log.Println("Integrated backend system stopped")
	return nil
}

// Example Business Operation using all patterns
func (ibs *IntegratedBackendSystem) RegisterUserWithCompleteFlow(email, firstName, lastName, companyName string) error {
	// Use Unit of Work for transaction management
	uow := ibs.uowFactory.Create()
	
	return uow.WithTransaction(func(uow repository.UnitOfWork) error {
		// 1. Create user aggregate (Write side)
		userRepo := uow.UserRepository()
		
		// Check if user already exists
		emailObj, _ := shared.NewEmail(email)
		exists, err := userRepo.ExistsByEmail(emailObj)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("user with email %s already exists", email)
		}
		
		// Create user (this would use your actual user aggregate)
		// userAggregate := user.NewUserAggregate(...)
		// userRepo.Save(userAggregate)
		
		// 2. Domain events are automatically collected by UoW
		// Events will be stored in event store and outbox within the transaction
		
		log.Printf("User registration completed for email: %s", email)
		return nil
	})
}

// ReplayUserEvents demonstrates selective event replay
func (ibs *IntegratedBackendSystem) ReplayUserEvents(userID string, fromVersion int64) (*ReplayResult, error) {
	request := ReplayRequest{
		AggregateID:    userID,
		AggregateType:  "UserAggregate",
		FromVersion:    fromVersion,
		Strategy:       SequentialReplay,
		UseSnapshots:   true,
		CreateSnapshot: true,
		BatchSize:      50,
		Timeout:        5 * time.Minute,
	}
	
	return ibs.replayEngine.ReplayEvents(request)
}

// RebuildProjections rebuilds all read model projections
func (ibs *IntegratedBackendSystem) RebuildProjections(fromTime time.Time) (*ReplayResult, error) {
	request := ReplayRequest{
		FromTime:       fromTime,
		Strategy:       ParallelReplay,
		UseSnapshots:   false, // Force rebuild from events
		CreateSnapshot: true,
		WorkerCount:    8,
		BatchSize:      200,
		Timeout:        30 * time.Minute,
	}
	
	return ibs.replayEngine.ReplayEvents(request)
}

// GetSystemHealth returns health status of all components
func (ibs *IntegratedBackendSystem) GetSystemHealth() map[string]interface{} {
	health := make(map[string]interface{})
	
	// Database health
	if err := ibs.dbConfig.HealthCheck(); err != nil {
		health["database"] = map[string]interface{}{
			"status": "unhealthy",
			"error":  err.Error(),
		}
	} else {
		health["database"] = map[string]interface{}{
			"status": "healthy",
		}
	}
	
	// Connection pool health
	poolStats := ibs.connectionPool.GetDetailedStats()
	health["connection_pools"] = poolStats
	
	// Outbox health
	outboxStats := ibs.outbox.GetStats()
	health["outbox"] = map[string]interface{}{
		"total_events":     outboxStats.TotalEvents,
		"published_events": outboxStats.PublishedEvents,
		"failed_events":    outboxStats.FailedEvents,
		"last_processed":   outboxStats.LastProcessedAt,
	}
	
	// Projection status
	projectionStatus := ibs.projectionMgr.GetProjectionStatus()
	health["projections"] = projectionStatus
	
	// Replay engine metrics
	replayMetrics := ibs.replayEngine.GetMetrics()
	health["replay_engine"] = map[string]interface{}{
		"total_events":     replayMetrics.TotalEvents,
		"processed_events": replayMetrics.ProcessedEvents,
		"failed_events":    replayMetrics.FailedEvents,
		"last_replay":      replayMetrics.ReplayEndTime,
	}
	
	// Snapshot statistics
	if snapshotStats, err := ibs.snapshotStore.(*PostgresSnapshotStore).GetSnapshotStats(); err == nil {
		health["snapshots"] = snapshotStats
	}
	
	return health
}

// Example projection handlers

// UserProjectionHandler handles user events for read model updates
type UserProjectionHandler struct {
	readDB *sql.DB
}

func (uph *UserProjectionHandler) GetName() string {
	return "user_projection_handler"
}

func (uph *UserProjectionHandler) GetEventTypes() []string {
	return []string{"user.registered", "user.updated", "user.activated", "user.deactivated"}
}

func (uph *UserProjectionHandler) Handle(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	// Update user projection based on event type
	switch event.EventType() {
	case "user.registered":
		return uph.handleUserRegistered(event, tx)
	case "user.updated":
		return uph.handleUserUpdated(event, tx)
	case "user.activated":
		return uph.handleUserActivated(event, tx)
	case "user.deactivated":
		return uph.handleUserDeactivated(event, tx)
	}
	return nil
}

func (uph *UserProjectionHandler) GetCheckpoint(ctx context.Context, db *sql.DB) (string, error) {
	var checkpoint sql.NullString
	err := db.QueryRow("SELECT last_processed_event_id FROM event_handler_checkpoints WHERE handler_name = $1", 
		uph.GetName()).Scan(&checkpoint)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if checkpoint.Valid {
		return checkpoint.String, nil
	}
	return "", nil
}

func (uph *UserProjectionHandler) UpdateCheckpoint(ctx context.Context, eventID string, tx *sql.Tx) error {
	_, err := tx.Exec(`
		INSERT INTO event_handler_checkpoints (handler_name, last_processed_event_id, last_processed_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (handler_name)
		DO UPDATE SET last_processed_event_id = EXCLUDED.last_processed_event_id, 
					  last_processed_at = EXCLUDED.last_processed_at
	`, uph.GetName(), eventID)
	return err
}

func (uph *UserProjectionHandler) handleUserRegistered(event shared.DomainEvent, tx *sql.Tx) error {
	// Extract user data from event and insert into user_projections table
	log.Printf("Updating user projection for user registration: %s", event.AggregateID())
	return nil
}

func (uph *UserProjectionHandler) handleUserUpdated(event shared.DomainEvent, tx *sql.Tx) error {
	// Update user projection
	log.Printf("Updating user projection for user update: %s", event.AggregateID())
	return nil
}

func (uph *UserProjectionHandler) handleUserActivated(event shared.DomainEvent, tx *sql.Tx) error {
	// Update user status to active
	log.Printf("Updating user projection for user activation: %s", event.AggregateID())
	return nil
}

func (uph *UserProjectionHandler) handleUserDeactivated(event shared.DomainEvent, tx *sql.Tx) error {
	// Update user status to inactive
	log.Printf("Updating user projection for user deactivation: %s", event.AggregateID())
	return nil
}

// Similar implementations for WorkspaceProjectionHandler and TenantProjectionHandler...

type WorkspaceProjectionHandler struct {
	readDB *sql.DB
}

func (wph *WorkspaceProjectionHandler) GetName() string {
	return "workspace_projection_handler"
}

func (wph *WorkspaceProjectionHandler) GetEventTypes() []string {
	return []string{"workspace.created", "workspace.updated", "workspace.member_added", "workspace.member_removed"}
}

func (wph *WorkspaceProjectionHandler) Handle(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	log.Printf("Updating workspace projection for event: %s, aggregate: %s", 
		event.EventType(), event.AggregateID())
	return nil
}

func (wph *WorkspaceProjectionHandler) GetCheckpoint(ctx context.Context, db *sql.DB) (string, error) {
	var checkpoint sql.NullString
	err := db.QueryRow("SELECT last_processed_event_id FROM event_handler_checkpoints WHERE handler_name = $1", 
		wph.GetName()).Scan(&checkpoint)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if checkpoint.Valid {
		return checkpoint.String, nil
	}
	return "", nil
}

func (wph *WorkspaceProjectionHandler) UpdateCheckpoint(ctx context.Context, eventID string, tx *sql.Tx) error {
	_, err := tx.Exec(`
		INSERT INTO event_handler_checkpoints (handler_name, last_processed_event_id, last_processed_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (handler_name)
		DO UPDATE SET last_processed_event_id = EXCLUDED.last_processed_event_id, 
					  last_processed_at = EXCLUDED.last_processed_at
	`, wph.GetName(), eventID)
	return err
}

type TenantProjectionHandler struct {
	readDB *sql.DB
}

func (tph *TenantProjectionHandler) GetName() string {
	return "tenant_projection_handler"
}

func (tph *TenantProjectionHandler) GetEventTypes() []string {
	return []string{"tenant.created", "tenant.updated", "tenant.plan_changed"}
}

func (tph *TenantProjectionHandler) Handle(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	log.Printf("Updating tenant projection for event: %s, aggregate: %s", 
		event.EventType(), event.AggregateID())
	return nil
}

func (tph *TenantProjectionHandler) GetCheckpoint(ctx context.Context, db *sql.DB) (string, error) {
	var checkpoint sql.NullString
	err := db.QueryRow("SELECT last_processed_event_id FROM event_handler_checkpoints WHERE handler_name = $1", 
		tph.GetName()).Scan(&checkpoint)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if checkpoint.Valid {
		return checkpoint.String, nil
	}
	return "", nil
}

func (tph *TenantProjectionHandler) UpdateCheckpoint(ctx context.Context, eventID string, tx *sql.Tx) error {
	_, err := tx.Exec(`
		INSERT INTO event_handler_checkpoints (handler_name, last_processed_event_id, last_processed_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (handler_name)
		DO UPDATE SET last_processed_event_id = EXCLUDED.last_processed_event_id, 
					  last_processed_at = EXCLUDED.last_processed_at
	`, tph.GetName(), eventID)
	return err
}