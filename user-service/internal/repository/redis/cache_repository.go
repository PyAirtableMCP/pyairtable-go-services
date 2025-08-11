package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	
	"github.com/Reg-Kris/pyairtable-user-service/internal/models"
	"github.com/redis/go-redis/v9"
)

type CacheRepository struct {
	client *redis.Client
	ttl    time.Duration
}

func NewCacheRepository(client *redis.Client) *CacheRepository {
	return &CacheRepository{
		client: client,
		ttl:    15 * time.Minute, // Default TTL
	}
}

// User caching
func (r *CacheRepository) GetUser(ctx context.Context, id string) (*models.User, error) {
	key := fmt.Sprintf("user:%s", id)
	
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, err
	}
	
	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, err
	}
	
	return &user, nil
}

func (r *CacheRepository) SetUser(ctx context.Context, user *models.User) error {
	key := fmt.Sprintf("user:%s", user.ID)
	
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	
	return r.client.Set(ctx, key, data, r.ttl).Err()
}

func (r *CacheRepository) DeleteUser(ctx context.Context, id string) error {
	key := fmt.Sprintf("user:%s", id)
	return r.client.Del(ctx, key).Err()
}

// User list caching
func (r *CacheRepository) GetUserList(ctx context.Context, key string) ([]*models.User, error) {
	cacheKey := fmt.Sprintf("userlist:%s", key)
	
	data, err := r.client.Get(ctx, cacheKey).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, err
	}
	
	var users []*models.User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, err
	}
	
	return users, nil
}

func (r *CacheRepository) SetUserList(ctx context.Context, key string, users []*models.User, ttl int) error {
	cacheKey := fmt.Sprintf("userlist:%s", key)
	
	data, err := json.Marshal(users)
	if err != nil {
		return err
	}
	
	duration := time.Duration(ttl) * time.Second
	if duration == 0 {
		duration = r.ttl
	}
	
	return r.client.Set(ctx, cacheKey, data, duration).Err()
}

func (r *CacheRepository) DeleteUserList(ctx context.Context, key string) error {
	cacheKey := fmt.Sprintf("userlist:%s", key)
	return r.client.Del(ctx, cacheKey).Err()
}

// Stats caching
func (r *CacheRepository) GetStats(ctx context.Context, key string) (*models.UserStats, error) {
	cacheKey := fmt.Sprintf("stats:%s", key)
	
	data, err := r.client.Get(ctx, cacheKey).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, err
	}
	
	var stats models.UserStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}
	
	return &stats, nil
}

func (r *CacheRepository) SetStats(ctx context.Context, key string, stats *models.UserStats, ttl int) error {
	cacheKey := fmt.Sprintf("stats:%s", key)
	
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	
	duration := time.Duration(ttl) * time.Second
	if duration == 0 {
		duration = 1 * time.Hour // Stats can be cached longer
	}
	
	return r.client.Set(ctx, cacheKey, data, duration).Err()
}

func (r *CacheRepository) DeleteStats(ctx context.Context, key string) error {
	cacheKey := fmt.Sprintf("stats:%s", key)
	return r.client.Del(ctx, cacheKey).Err()
}

// Cache invalidation
func (r *CacheRepository) InvalidateUserCache(ctx context.Context, userID string) error {
	// Delete user cache
	if err := r.DeleteUser(ctx, userID); err != nil {
		return err
	}
	
	// Delete all user lists that might contain this user
	// In production, we'd track which lists contain which users
	pattern := "userlist:*"
	var cursor uint64
	for {
		var keys []string
		var err error
		keys, cursor, err = r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		
		if len(keys) > 0 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		
		if cursor == 0 {
			break
		}
	}
	
	return nil
}

func (r *CacheRepository) InvalidateTenantCache(ctx context.Context, tenantID string) error {
	// Delete tenant-specific caches
	patterns := []string{
		fmt.Sprintf("userlist:tenant:%s:*", tenantID),
		fmt.Sprintf("stats:tenant:%s", tenantID),
	}
	
	for _, pattern := range patterns {
		var cursor uint64
		for {
			var keys []string
			var err error
			keys, cursor, err = r.client.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				return err
			}
			
			if len(keys) > 0 {
				if err := r.client.Del(ctx, keys...).Err(); err != nil {
					return err
				}
			}
			
			if cursor == 0 {
				break
			}
		}
	}
	
	return nil
}

func (r *CacheRepository) FlushAll(ctx context.Context) error {
	// In production, we'd only flush specific key patterns
	// to avoid affecting other services using the same Redis
	patterns := []string{"user:*", "userlist:*", "stats:*"}
	
	for _, pattern := range patterns {
		var cursor uint64
		for {
			var keys []string
			var err error
			keys, cursor, err = r.client.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				return err
			}
			
			if len(keys) > 0 {
				if err := r.client.Del(ctx, keys...).Err(); err != nil {
					return err
				}
			}
			
			if cursor == 0 {
				break
			}
		}
	}
	
	return nil
}