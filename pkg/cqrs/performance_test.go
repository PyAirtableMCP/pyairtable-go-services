package cqrs

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"github.com/pyairtable-compose/go-services/pkg/cache"
)

// PerformanceBenchmark conducts comprehensive performance testing for CQRS
type PerformanceBenchmark struct {
	writeDB         *sql.DB
	readDB          *sql.DB
	cqrsService     *CQRSService
	queryService    *QueryService
	redisClient     *cache.RedisClient
	testData        []TestUser
	baseline        map[string]time.Duration
}

// TestUser represents test user data
type TestUser struct {
	ID          string
	Email       string
	FirstName   string
	LastName    string
	TenantID    string
	IsActive    bool
}

// NewPerformanceBenchmark creates a new performance benchmark suite
func NewPerformanceBenchmark(writeDB, readDB *sql.DB, redisClient *cache.RedisClient) *PerformanceBenchmark {
	// Initialize CQRS service for testing
	cqrsConfig := DefaultCQRSServiceConfig()
	cqrsService, _ := NewCQRSService(cqrsConfig, redisClient)
	
	queryService := NewQueryService(readDB, cqrsService.CacheManager, cqrsService.QueryOptimizer)

	return &PerformanceBenchmark{
		writeDB:      writeDB,
		readDB:       readDB,
		cqrsService:  cqrsService,
		queryService: queryService,
		redisClient:  redisClient,
		testData:     generateTestUsers(1000),
		baseline:     make(map[string]time.Duration),
	}
}

// generateTestUsers creates test user data
func generateTestUsers(count int) []TestUser {
	users := make([]TestUser, count)
	tenantIDs := []string{"tenant-1", "tenant-2", "tenant-3", "tenant-4", "tenant-5"}
	
	for i := 0; i < count; i++ {
		users[i] = TestUser{
			ID:        fmt.Sprintf("user-%d", i+1),
			Email:     fmt.Sprintf("user%d@example.com", i+1),
			FirstName: fmt.Sprintf("User%d", i+1),
			LastName:  "Test",
			TenantID:  tenantIDs[rand.Intn(len(tenantIDs))],
			IsActive:  rand.Float32() > 0.2, // 80% active users
		}
	}
	
	return users
}

// SetupTestData populates the database with test data
func (pb *PerformanceBenchmark) SetupTestData(ctx context.Context) error {
	log.Println("Setting up test data for performance benchmark...")
	
	// Clear existing test data
	_, err := pb.readDB.ExecContext(ctx, "DELETE FROM read_schema.user_projections WHERE email LIKE '%@example.com'")
	if err != nil {
		return fmt.Errorf("failed to clear test data: %w", err)
	}

	// Insert test users into read model
	for _, user := range pb.testData {
		_, err := pb.readDB.ExecContext(ctx, `
			INSERT INTO read_schema.user_projections (
				id, email, first_name, last_name, display_name, tenant_id, 
				status, email_verified, is_active, created_at, updated_at, event_version
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, 
			user.ID, user.Email, user.FirstName, user.LastName, 
			fmt.Sprintf("%s %s", user.FirstName, user.LastName),
			user.TenantID, "active", true, user.IsActive,
			time.Now(), time.Now(), 1,
		)
		if err != nil {
			return fmt.Errorf("failed to insert test user %s: %w", user.ID, err)
		}
	}

	log.Printf("Inserted %d test users", len(pb.testData))
	return nil
}

// RunBaselineQueries measures traditional query performance (without CQRS)
func (pb *PerformanceBenchmark) RunBaselineQueries(ctx context.Context) error {
	log.Println("Running baseline performance tests...")

	// Single user query baseline
	pb.baseline["single_user_query"] = pb.measureTraditionalSingleUserQuery(ctx, 100)
	
	// Tenant users query baseline
	pb.baseline["tenant_users_query"] = pb.measureTraditionalTenantUsersQuery(ctx, 50)
	
	// Concurrent queries baseline
	pb.baseline["concurrent_queries"] = pb.measureTraditionalConcurrentQueries(ctx, 50, 10)

	log.Printf("Baseline measurements: %+v", pb.baseline)
	return nil
}

// RunCQRSQueries measures CQRS query performance
func (pb *PerformanceBenchmark) RunCQRSQueries(ctx context.Context) (map[string]time.Duration, error) {
	log.Println("Running CQRS performance tests...")

	results := make(map[string]time.Duration)
	
	// Single user query with CQRS
	results["single_user_query"] = pb.measureCQRSSingleUserQuery(ctx, 100)
	
	// Tenant users query with CQRS
	results["tenant_users_query"] = pb.measureCQRSTenantUsersQuery(ctx, 50)
	
	// Concurrent queries with CQRS
	results["concurrent_queries"] = pb.measureCQRSConcurrentQueries(ctx, 50, 10)

	log.Printf("CQRS measurements: %+v", results)
	return results, nil
}

// measureTraditionalSingleUserQuery measures traditional single user query performance
func (pb *PerformanceBenchmark) measureTraditionalSingleUserQuery(ctx context.Context, iterations int) time.Duration {
	start := time.Now()
	
	for i := 0; i < iterations; i++ {
		userID := pb.testData[rand.Intn(len(pb.testData))].ID
		
		var user UserProjection
		row := pb.readDB.QueryRowContext(ctx, `
			SELECT id, email, first_name, last_name, display_name, tenant_id, 
				   status, email_verified, is_active, created_at, updated_at, event_version
			FROM read_schema.user_projections 
			WHERE id = $1
		`, userID)
		
		err := row.Scan(
			&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.DisplayName,
			&user.TenantID, &user.Status, &user.EmailVerified, &user.IsActive,
			&user.CreatedAt, &user.UpdatedAt, &user.EventVersion,
		)
		
		if err != nil && err != sql.ErrNoRows {
			log.Printf("Error in traditional query: %v", err)
		}
	}
	
	return time.Since(start)
}

// measureCQRSSingleUserQuery measures CQRS single user query performance
func (pb *PerformanceBenchmark) measureCQRSSingleUserQuery(ctx context.Context, iterations int) time.Duration {
	start := time.Now()
	
	for i := 0; i < iterations; i++ {
		testUser := pb.testData[rand.Intn(len(pb.testData))]
		
		_, err := pb.queryService.GetUserByID(ctx, testUser.ID, testUser.TenantID)
		if err != nil {
			log.Printf("Error in CQRS query: %v", err)
		}
	}
	
	return time.Since(start)
}

// measureTraditionalTenantUsersQuery measures traditional tenant users query performance
func (pb *PerformanceBenchmark) measureTraditionalTenantUsersQuery(ctx context.Context, iterations int) time.Duration {
	start := time.Now()
	
	tenantIDs := []string{"tenant-1", "tenant-2", "tenant-3", "tenant-4", "tenant-5"}
	
	for i := 0; i < iterations; i++ {
		tenantID := tenantIDs[rand.Intn(len(tenantIDs))]
		
		rows, err := pb.readDB.QueryContext(ctx, `
			SELECT id, email, first_name, last_name, display_name, tenant_id, 
				   status, email_verified, is_active, created_at, updated_at, event_version
			FROM read_schema.user_projections 
			WHERE tenant_id = $1 
			ORDER BY created_at DESC 
			LIMIT 20
		`, tenantID)
		
		if err != nil {
			log.Printf("Error in traditional tenant query: %v", err)
			continue
		}
		
		var users []UserProjection
		for rows.Next() {
			var user UserProjection
			err := rows.Scan(
				&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.DisplayName,
				&user.TenantID, &user.Status, &user.EmailVerified, &user.IsActive,
				&user.CreatedAt, &user.UpdatedAt, &user.EventVersion,
			)
			if err != nil {
				log.Printf("Error scanning row: %v", err)
				continue
			}
			users = append(users, user)
		}
		rows.Close()
	}
	
	return time.Since(start)
}

// measureCQRSTenantUsersQuery measures CQRS tenant users query performance
func (pb *PerformanceBenchmark) measureCQRSTenantUsersQuery(ctx context.Context, iterations int) time.Duration {
	start := time.Now()
	
	tenantIDs := []string{"tenant-1", "tenant-2", "tenant-3", "tenant-4", "tenant-5"}
	
	for i := 0; i < iterations; i++ {
		tenantID := tenantIDs[rand.Intn(len(tenantIDs))]
		
		_, err := pb.queryService.GetUsersByTenant(ctx, tenantID, 20, 0, false)
		if err != nil {
			log.Printf("Error in CQRS tenant query: %v", err)
		}
	}
	
	return time.Since(start)
}

// measureTraditionalConcurrentQueries measures traditional concurrent query performance
func (pb *PerformanceBenchmark) measureTraditionalConcurrentQueries(ctx context.Context, queries, concurrent int) time.Duration {
	start := time.Now()
	
	var wg sync.WaitGroup
	queryChan := make(chan TestUser, queries)
	
	// Fill query channel
	for i := 0; i < queries; i++ {
		queryChan <- pb.testData[rand.Intn(len(pb.testData))]
	}
	close(queryChan)
	
	// Run concurrent workers
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for testUser := range queryChan {
				var user UserProjection
				row := pb.readDB.QueryRowContext(ctx, `
					SELECT id, email, first_name, last_name, display_name, tenant_id, 
						   status, email_verified, is_active, created_at, updated_at, event_version
					FROM read_schema.user_projections 
					WHERE id = $1 AND tenant_id = $2
				`, testUser.ID, testUser.TenantID)
				
				err := row.Scan(
					&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.DisplayName,
					&user.TenantID, &user.Status, &user.EmailVerified, &user.IsActive,
					&user.CreatedAt, &user.UpdatedAt, &user.EventVersion,
				)
				
				if err != nil && err != sql.ErrNoRows {
					log.Printf("Error in concurrent traditional query: %v", err)
				}
			}
		}()
	}
	
	wg.Wait()
	return time.Since(start)
}

// measureCQRSConcurrentQueries measures CQRS concurrent query performance
func (pb *PerformanceBenchmark) measureCQRSConcurrentQueries(ctx context.Context, queries, concurrent int) time.Duration {
	start := time.Now()
	
	var wg sync.WaitGroup
	queryChan := make(chan TestUser, queries)
	
	// Fill query channel
	for i := 0; i < queries; i++ {
		queryChan <- pb.testData[rand.Intn(len(pb.testData))]
	}
	close(queryChan)
	
	// Run concurrent workers
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for testUser := range queryChan {
				_, err := pb.queryService.GetUserByID(ctx, testUser.ID, testUser.TenantID)
				if err != nil {
					log.Printf("Error in concurrent CQRS query: %v", err)
				}
			}
		}()
	}
	
	wg.Wait()
	return time.Since(start)
}

// CalculatePerformanceImprovement calculates performance improvement ratios
func (pb *PerformanceBenchmark) CalculatePerformanceImprovement(cqrsResults map[string]time.Duration) map[string]float64 {
	improvements := make(map[string]float64)
	
	for queryType, cqrsTime := range cqrsResults {
		if baseline, exists := pb.baseline[queryType]; exists {
			improvementRatio := float64(baseline.Nanoseconds()) / float64(cqrsTime.Nanoseconds())
			improvements[queryType] = improvementRatio
		}
	}
	
	return improvements
}

// VerifyPerformanceImprovement checks if we achieve at least 10x improvement
func (pb *PerformanceBenchmark) VerifyPerformanceImprovement(improvements map[string]float64) bool {
	const target = 10.0 // 10x improvement target
	
	for queryType, improvement := range improvements {
		log.Printf("%s improvement: %.2fx", queryType, improvement)
		
		if improvement < target {
			log.Printf("WARNING: %s did not meet 10x improvement target (got %.2fx)", queryType, improvement)
			return false
		}
	}
	
	return true
}

// CleanupTestData removes test data
func (pb *PerformanceBenchmark) CleanupTestData(ctx context.Context) error {
	_, err := pb.readDB.ExecContext(ctx, "DELETE FROM read_schema.user_projections WHERE email LIKE '%@example.com'")
	if err != nil {
		return fmt.Errorf("failed to cleanup test data: %w", err)
	}
	
	// Clear cache
	pb.redisClient.FlushDB()
	
	log.Println("Test data cleaned up")
	return nil
}

// Test function for integration testing
func TestCQRSPerformanceImprovement(t *testing.T) {
	// This test would be run in an integration test environment
	// with actual database and Redis connections
	
	t.Skip("Integration test - requires database and Redis setup")
	
	// Mock setup for demonstration
	var writeDB, readDB *sql.DB
	var redisClient *cache.RedisClient
	
	benchmark := NewPerformanceBenchmark(writeDB, readDB, redisClient)
	ctx := context.Background()
	
	// Setup test data
	require.NoError(t, benchmark.SetupTestData(ctx))
	defer benchmark.CleanupTestData(ctx)
	
	// Run baseline tests
	require.NoError(t, benchmark.RunBaselineQueries(ctx))
	
	// Run CQRS tests
	cqrsResults, err := benchmark.RunCQRSQueries(ctx)
	require.NoError(t, err)
	
	// Calculate improvements
	improvements := benchmark.CalculatePerformanceImprovement(cqrsResults)
	
	// Verify we achieve 10x improvement
	meetsTarget := benchmark.VerifyPerformanceImprovement(improvements)
	assert.True(t, meetsTarget, "CQRS implementation should achieve at least 10x performance improvement")
	
	// Log detailed results
	for queryType, improvement := range improvements {
		t.Logf("%s: %.2fx improvement", queryType, improvement)
	}
}

// BenchmarkCQRSVsTraditional provides Go benchmark tests
func BenchmarkCQRSVsTraditional(b *testing.B) {
	b.Skip("Benchmark test - requires database and Redis setup")
	
	// This would run actual benchmark comparisons
	// between traditional and CQRS query approaches
}