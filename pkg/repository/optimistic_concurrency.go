package repository

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
)

// ConcurrencyControlManager manages optimistic concurrency control across aggregates
type ConcurrencyControlManager struct {
	mu               sync.RWMutex
	versionCache     map[string]shared.Version  // aggregateID -> current version
	expectedVersions map[string]shared.Version  // aggregateID -> expected version
	lockTimeouts     map[string]time.Time       // aggregateID -> lock expiry
	conflictResolver ConflictResolver
	metrics          *ConcurrencyMetrics
}

// ConflictResolver defines how to resolve version conflicts
type ConflictResolver interface {
	ResolveConflict(aggregate shared.AggregateRoot, expectedVersion, actualVersion shared.Version) (ResolveAction, error)
	GetResolverName() string
}

// ResolveAction defines the action to take when resolving conflicts
type ResolveAction string

const (
	ResolveActionRetry     ResolveAction = "retry"      // Retry the operation
	ResolveActionAbort     ResolveAction = "abort"      // Abort the operation
	ResolveActionMerge     ResolveAction = "merge"      // Attempt to merge changes
	ResolveActionOverwrite ResolveAction = "overwrite"  // Overwrite with new version
)

// ConcurrencyMetrics tracks concurrency control metrics
type ConcurrencyMetrics struct {
	mu                    sync.RWMutex
	TotalChecks           int64
	ConflictsDetected     int64
	ConflictsResolved     int64
	ConflictResolutionTime time.Duration
	LockTimeouts          int64
	SuccessfulLocks       int64
	LockDuration          time.Duration
}

// VersionCheckResult contains the result of a version check
type VersionCheckResult struct {
	IsValid           bool
	ExpectedVersion   shared.Version
	ActualVersion     shared.Version
	ConflictDetected  bool
	ResolveAction     ResolveAction
	Error             error
}

// NewConcurrencyControlManager creates a new concurrency control manager
func NewConcurrencyControlManager(resolver ConflictResolver) *ConcurrencyControlManager {
	return &ConcurrencyControlManager{
		versionCache:     make(map[string]shared.Version),
		expectedVersions: make(map[string]shared.Version),
		lockTimeouts:     make(map[string]time.Time),
		conflictResolver: resolver,
		metrics: &ConcurrencyMetrics{},
	}
}

// RegisterAggregateVersion registers an aggregate's current version
func (ccm *ConcurrencyControlManager) RegisterAggregateVersion(aggregateID string, version shared.Version) {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	
	ccm.versionCache[aggregateID] = version
	ccm.expectedVersions[aggregateID] = version
	
	log.Printf("Registered aggregate %s with version %d", aggregateID, version.Int())
}

// CheckOptimisticConcurrency validates version consistency before saving
func (ccm *ConcurrencyControlManager) CheckOptimisticConcurrency(
	tx *sql.Tx, 
	aggregate shared.AggregateRoot,
) (*VersionCheckResult, error) {
	
	startTime := time.Now()
	defer func() {
		ccm.metrics.mu.Lock()
		ccm.metrics.TotalChecks++
		ccm.metrics.mu.Unlock()
	}()
	
	aggregateID := aggregate.GetID().String()
	currentVersion := aggregate.GetVersion()
	
	ccm.mu.RLock()
	expectedVersion, hasExpected := ccm.expectedVersions[aggregateID]
	ccm.mu.RUnlock()
	
	if !hasExpected {
		// First time seeing this aggregate, register it
		ccm.RegisterAggregateVersion(aggregateID, currentVersion)
		return &VersionCheckResult{
			IsValid:         true,
			ExpectedVersion: currentVersion,
			ActualVersion:   currentVersion,
		}, nil
	}
	
	// Check database for actual current version
	actualVersion, err := ccm.getDatabaseVersion(tx, aggregateID)
	if err != nil {
		return &VersionCheckResult{
			IsValid: false,
			Error:   fmt.Errorf("failed to get database version: %w", err),
		}, err
	}
	
	// Calculate the expected version (version before any new events)
	uncommittedEvents := aggregate.GetUncommittedEvents()
	expectedDBVersion := currentVersion.Int() - int64(len(uncommittedEvents))
	
	result := &VersionCheckResult{
		ExpectedVersion: shared.NewVersionFromInt(expectedDBVersion),
		ActualVersion:   actualVersion,
	}
	
	// Check for conflicts
	if actualVersion.Int() != expectedDBVersion {
		ccm.metrics.mu.Lock()
		ccm.metrics.ConflictsDetected++
		ccm.metrics.mu.Unlock()
		
		result.ConflictDetected = true
		result.IsValid = false
		
		log.Printf("Concurrency conflict detected for aggregate %s: expected %d, actual %d", 
			aggregateID, expectedDBVersion, actualVersion.Int())
		
		// Attempt to resolve the conflict
		if ccm.conflictResolver != nil {
			resolveStart := time.Now()
			action, resolveErr := ccm.conflictResolver.ResolveConflict(aggregate, result.ExpectedVersion, actualVersion)
			
			ccm.metrics.mu.Lock()
			ccm.metrics.ConflictResolutionTime += time.Since(resolveStart)
			ccm.metrics.mu.Unlock()
			
			if resolveErr != nil {
				result.Error = fmt.Errorf("conflict resolution failed: %w", resolveErr)
				return result, resolveErr
			}
			
			result.ResolveAction = action
			
			// Handle resolution action
			switch action {
			case ResolveActionRetry:
				result.IsValid = false // Caller should retry
				ccm.metrics.mu.Lock()
				ccm.metrics.ConflictsResolved++
				ccm.metrics.mu.Unlock()
			case ResolveActionOverwrite:
				result.IsValid = true // Allow overwrite
				result.ExpectedVersion = actualVersion
				ccm.metrics.mu.Lock()
				ccm.metrics.ConflictsResolved++
				ccm.metrics.mu.Unlock()
			case ResolveActionAbort:
				result.IsValid = false
				result.Error = shared.NewDomainError(
					shared.ErrCodeConflict,
					"operation aborted due to concurrency conflict",
					map[string]interface{}{
						"aggregate_id":     aggregateID,
						"expected_version": expectedDBVersion,
						"actual_version":   actualVersion.Int(),
					},
				)
			case ResolveActionMerge:
				// Merge would require additional logic specific to the aggregate type
				result.IsValid = false
				result.Error = fmt.Errorf("merge conflict resolution not implemented")
			}
		} else {
			// No resolver, treat as standard conflict
			result.Error = shared.NewDomainError(
				shared.ErrCodeConflict,
				fmt.Sprintf("concurrency conflict: expected version %d, got %d", expectedDBVersion, actualVersion.Int()),
				map[string]interface{}{
					"aggregate_id":     aggregateID,
					"expected_version": expectedDBVersion,
					"actual_version":   actualVersion.Int(),
				},
			)
		}
	} else {
		result.IsValid = true
	}
	
	// Update our cache with the actual version
	ccm.mu.Lock()
	ccm.versionCache[aggregateID] = actualVersion
	ccm.mu.Unlock()
	
	return result, nil
}

// AcquireOptimisticLock attempts to acquire an optimistic lock on an aggregate
func (ccm *ConcurrencyControlManager) AcquireOptimisticLock(
	aggregateID string, 
	expectedVersion shared.Version, 
	lockDuration time.Duration,
) error {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	
	// Check if already locked
	if expiry, isLocked := ccm.lockTimeouts[aggregateID]; isLocked && time.Now().Before(expiry) {
		ccm.metrics.mu.Lock()
		ccm.metrics.LockTimeouts++
		ccm.metrics.mu.Unlock()
		
		return shared.NewDomainError(
			shared.ErrCodeConflict,
			"aggregate is already locked",
			map[string]interface{}{
				"aggregate_id": aggregateID,
				"lock_expiry":  expiry,
			},
		)
	}
	
	// Verify version matches expected
	if cachedVersion, exists := ccm.versionCache[aggregateID]; exists {
		if cachedVersion.Int() != expectedVersion.Int() {
			return shared.NewDomainError(
				shared.ErrCodeConflict,
				"version mismatch when acquiring lock",
				map[string]interface{}{
					"aggregate_id":     aggregateID,
					"expected_version": expectedVersion.Int(),
					"cached_version":   cachedVersion.Int(),
				},
			)
		}
	}
	
	// Acquire the lock
	ccm.lockTimeouts[aggregateID] = time.Now().Add(lockDuration)
	
	ccm.metrics.mu.Lock()
	ccm.metrics.SuccessfulLocks++
	ccm.metrics.LockDuration += lockDuration
	ccm.metrics.mu.Unlock()
	
	log.Printf("Acquired optimistic lock on aggregate %s for %v", aggregateID, lockDuration)
	return nil
}

// ReleaseOptimisticLock releases an optimistic lock on an aggregate
func (ccm *ConcurrencyControlManager) ReleaseOptimisticLock(aggregateID string) {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	
	delete(ccm.lockTimeouts, aggregateID)
	log.Printf("Released optimistic lock on aggregate %s", aggregateID)
}

// UpdateVersionAfterSave updates the version cache after a successful save
func (ccm *ConcurrencyControlManager) UpdateVersionAfterSave(aggregateID string, newVersion shared.Version) {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	
	ccm.versionCache[aggregateID] = newVersion
	ccm.expectedVersions[aggregateID] = newVersion
	
	log.Printf("Updated version cache for aggregate %s to version %d", aggregateID, newVersion.Int())
}

// getDatabaseVersion retrieves the current version from the database
func (ccm *ConcurrencyControlManager) getDatabaseVersion(tx *sql.Tx, aggregateID string) (shared.Version, error) {
	var version int64
	err := tx.QueryRow(`
		SELECT COALESCE(MAX(version), 0) 
		FROM domain_events 
		WHERE aggregate_id = $1
	`, aggregateID).Scan(&version)
	
	if err != nil && err != sql.ErrNoRows {
		return shared.NewVersion(), fmt.Errorf("failed to query aggregate version: %w", err)
	}
	
	return shared.NewVersionFromInt(version), nil
}

// CleanupExpiredLocks removes expired locks from the cache
func (ccm *ConcurrencyControlManager) CleanupExpiredLocks() {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	
	now := time.Now()
	expiredCount := 0
	
	for aggregateID, expiry := range ccm.lockTimeouts {
		if now.After(expiry) {
			delete(ccm.lockTimeouts, aggregateID)
			expiredCount++
		}
	}
	
	if expiredCount > 0 {
		log.Printf("Cleaned up %d expired optimistic locks", expiredCount)
	}
}

// GetMetrics returns current concurrency control metrics
func (ccm *ConcurrencyControlManager) GetMetrics() ConcurrencyMetrics {
	ccm.metrics.mu.RLock()
	defer ccm.metrics.mu.RUnlock()
	
	return ConcurrencyMetrics{
		TotalChecks:            ccm.metrics.TotalChecks,
		ConflictsDetected:      ccm.metrics.ConflictsDetected,
		ConflictsResolved:      ccm.metrics.ConflictsResolved,
		ConflictResolutionTime: ccm.metrics.ConflictResolutionTime,
		LockTimeouts:           ccm.metrics.LockTimeouts,
		SuccessfulLocks:        ccm.metrics.SuccessfulLocks,
		LockDuration:           ccm.metrics.LockDuration,
	}
}

// LogMetrics logs current concurrency control metrics
func (ccm *ConcurrencyControlManager) LogMetrics() {
	metrics := ccm.GetMetrics()
	
	log.Printf("Concurrency Control Metrics:")
	log.Printf("  Total Checks: %d", metrics.TotalChecks)
	log.Printf("  Conflicts Detected: %d", metrics.ConflictsDetected)
	log.Printf("  Conflicts Resolved: %d", metrics.ConflictsResolved)
	log.Printf("  Conflict Resolution Time: %v", metrics.ConflictResolutionTime)
	log.Printf("  Lock Timeouts: %d", metrics.LockTimeouts)
	log.Printf("  Successful Locks: %d", metrics.SuccessfulLocks)
	log.Printf("  Average Lock Duration: %v", metrics.LockDuration)
	
	if metrics.TotalChecks > 0 {
		conflictRate := float64(metrics.ConflictsDetected) / float64(metrics.TotalChecks) * 100
		log.Printf("  Conflict Rate: %.2f%%", conflictRate)
	}
}

// Conflict Resolver Implementations

// AbortOnConflictResolver aborts operations on any conflict
type AbortOnConflictResolver struct{}

// ResolveConflict always returns abort action
func (r *AbortOnConflictResolver) ResolveConflict(
	aggregate shared.AggregateRoot, 
	expectedVersion, actualVersion shared.Version,
) (ResolveAction, error) {
	return ResolveActionAbort, nil
}

// GetResolverName returns the resolver name
func (r *AbortOnConflictResolver) GetResolverName() string {
	return "AbortOnConflictResolver"
}

// RetryOnConflictResolver attempts to retry operations on conflicts
type RetryOnConflictResolver struct {
	maxRetries int
	retryCount map[string]int
	mu         sync.RWMutex
}

// NewRetryOnConflictResolver creates a new retry resolver
func NewRetryOnConflictResolver(maxRetries int) *RetryOnConflictResolver {
	return &RetryOnConflictResolver{
		maxRetries: maxRetries,
		retryCount: make(map[string]int),
	}
}

// ResolveConflict attempts to retry up to maxRetries times
func (r *RetryOnConflictResolver) ResolveConflict(
	aggregate shared.AggregateRoot, 
	expectedVersion, actualVersion shared.Version,
) (ResolveAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	aggregateID := aggregate.GetID().String()
	retries := r.retryCount[aggregateID]
	
	if retries >= r.maxRetries {
		// Max retries exceeded, abort
		delete(r.retryCount, aggregateID)
		return ResolveActionAbort, fmt.Errorf("max retries exceeded (%d)", r.maxRetries)
	}
	
	// Increment retry count and retry
	r.retryCount[aggregateID] = retries + 1
	log.Printf("Retrying aggregate %s operation (attempt %d/%d)", aggregateID, retries+1, r.maxRetries)
	
	return ResolveActionRetry, nil
}

// GetResolverName returns the resolver name
func (r *RetryOnConflictResolver) GetResolverName() string {
	return "RetryOnConflictResolver"
}

// ResetRetryCount resets the retry count for an aggregate (call after successful operation)
func (r *RetryOnConflictResolver) ResetRetryCount(aggregateID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.retryCount, aggregateID)
}

// OverwriteOnConflictResolver overwrites on any conflict (dangerous!)
type OverwriteOnConflictResolver struct {
	allowedAggregateTypes map[string]bool
}

// NewOverwriteOnConflictResolver creates a new overwrite resolver
func NewOverwriteOnConflictResolver(allowedTypes []string) *OverwriteOnConflictResolver {
	typeMap := make(map[string]bool)
	for _, aggregateType := range allowedTypes {
		typeMap[aggregateType] = true
	}
	
	return &OverwriteOnConflictResolver{
		allowedAggregateTypes: typeMap,
	}
}

// ResolveConflict overwrites if the aggregate type is allowed
func (r *OverwriteOnConflictResolver) ResolveConflict(
	aggregate shared.AggregateRoot, 
	expectedVersion, actualVersion shared.Version,
) (ResolveAction, error) {
	aggregateType := fmt.Sprintf("%T", aggregate)
	
	if r.allowedAggregateTypes[aggregateType] {
		log.Printf("WARNING: Overwriting aggregate %s due to conflict (type %s)", 
			aggregate.GetID().String(), aggregateType)
		return ResolveActionOverwrite, nil
	}
	
	return ResolveActionAbort, fmt.Errorf("overwrite not allowed for aggregate type %s", aggregateType)
}

// GetResolverName returns the resolver name
func (r *OverwriteOnConflictResolver) GetResolverName() string {
	return "OverwriteOnConflictResolver"
}

// SmartConflictResolver uses different strategies based on aggregate type and conflict nature
type SmartConflictResolver struct {
	strategies map[string]ConflictResolver // aggregateType -> resolver
	fallback   ConflictResolver
}

// NewSmartConflictResolver creates a new smart resolver
func NewSmartConflictResolver(fallback ConflictResolver) *SmartConflictResolver {
	return &SmartConflictResolver{
		strategies: make(map[string]ConflictResolver),
		fallback:   fallback,
	}
}

// RegisterStrategy registers a conflict resolution strategy for an aggregate type
func (r *SmartConflictResolver) RegisterStrategy(aggregateType string, resolver ConflictResolver) {
	r.strategies[aggregateType] = resolver
}

// ResolveConflict uses type-specific strategies or falls back to default
func (r *SmartConflictResolver) ResolveConflict(
	aggregate shared.AggregateRoot, 
	expectedVersion, actualVersion shared.Version,
) (ResolveAction, error) {
	aggregateType := fmt.Sprintf("%T", aggregate)
	
	if strategy, exists := r.strategies[aggregateType]; exists {
		return strategy.ResolveConflict(aggregate, expectedVersion, actualVersion)
	}
	
	if r.fallback != nil {
		return r.fallback.ResolveConflict(aggregate, expectedVersion, actualVersion)
	}
	
	// Default to abort
	return ResolveActionAbort, nil
}

// GetResolverName returns the resolver name
func (r *SmartConflictResolver) GetResolverName() string {
	return "SmartConflictResolver"
}