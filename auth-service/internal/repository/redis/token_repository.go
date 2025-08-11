package redis

import (
	"context"
	"fmt"
	"time"
	
	"github.com/redis/go-redis/v9"
)

type TokenRepository struct {
	client *redis.Client
	ctx    context.Context
}

func NewTokenRepository(client *redis.Client) *TokenRepository {
	return &TokenRepository{
		client: client,
		ctx:    context.Background(),
	}
}

func (r *TokenRepository) StoreRefreshToken(token, userID string, ttl time.Duration) error {
	key := fmt.Sprintf("refresh_token:%s", token)
	err := r.client.Set(r.ctx, key, userID, ttl).Err()
	if err != nil {
		return err
	}
	
	// Also store in user's token set for easy invalidation
	userKey := fmt.Sprintf("user_tokens:%s", userID)
	err = r.client.SAdd(r.ctx, userKey, token).Err()
	if err != nil {
		return err
	}
	
	// Set expiration on the set too
	r.client.Expire(r.ctx, userKey, ttl)
	
	return nil
}

func (r *TokenRepository) ValidateRefreshToken(token string) (string, error) {
	// First check if token is blacklisted
	blacklistKey := fmt.Sprintf("blacklist_token:%s", token)
	_, err := r.client.Get(r.ctx, blacklistKey).Result()
	if err == nil {
		return "", fmt.Errorf("token has been revoked")
	}
	
	// Check if token exists and is valid
	key := fmt.Sprintf("refresh_token:%s", token)
	userID, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("token not found or expired")
	}
	if err != nil {
		return "", err
	}
	
	return userID, nil
}

func (r *TokenRepository) InvalidateRefreshToken(token string) error {
	key := fmt.Sprintf("refresh_token:%s", token)
	
	// Get user ID first
	userID, err := r.client.Get(r.ctx, key).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	
	// Add to blacklist before deletion (for audit and security)
	blacklistKey := fmt.Sprintf("blacklist_token:%s", token)
	r.client.Set(r.ctx, blacklistKey, userID, 7*24*time.Hour) // Keep blacklist for 7 days
	
	// Delete the token
	err = r.client.Del(r.ctx, key).Err()
	if err != nil {
		return err
	}
	
	// Remove from user's token set
	if userID != "" {
		userKey := fmt.Sprintf("user_tokens:%s", userID)
		r.client.SRem(r.ctx, userKey, token)
	}
	
	return nil
}

func (r *TokenRepository) InvalidateAllUserTokens(userID string) error {
	userKey := fmt.Sprintf("user_tokens:%s", userID)
	
	// Get all tokens for the user
	tokens, err := r.client.SMembers(r.ctx, userKey).Result()
	if err != nil {
		return err
	}
	
	// Blacklist and delete each token
	for _, token := range tokens {
		// Add to blacklist
		blacklistKey := fmt.Sprintf("blacklist_token:%s", token)
		r.client.Set(r.ctx, blacklistKey, userID, 7*24*time.Hour)
		
		// Delete the token
		key := fmt.Sprintf("refresh_token:%s", token)
		r.client.Del(r.ctx, key)
	}
	
	// Delete the user's token set
	return r.client.Del(r.ctx, userKey).Err()
}

func (r *TokenRepository) CleanupExpiredTokens() error {
	// Redis automatically handles expiration, so this is a no-op
	// But we could implement additional cleanup logic if needed
	return nil
}

// IsTokenBlacklisted checks if a refresh token has been blacklisted
func (r *TokenRepository) IsTokenBlacklisted(token string) (bool, error) {
	blacklistKey := fmt.Sprintf("blacklist_token:%s", token)
	_, err := r.client.Get(r.ctx, blacklistKey).Result()
	if err == redis.Nil {
		return false, nil // Not blacklisted
	}
	if err != nil {
		return false, err // Error checking
	}
	return true, nil // Blacklisted
}

// GetBlacklistedTokensForUser returns all blacklisted tokens for a user (for audit)
func (r *TokenRepository) GetBlacklistedTokensForUser(userID string) ([]string, error) {
	pattern := "blacklist_token:*"
	var blacklistedTokens []string
	
	iter := r.client.Scan(r.ctx, 0, pattern, 0).Iterator()
	for iter.Next(r.ctx) {
		key := iter.Val()
		// Check if this blacklisted token belongs to the user
		tokenUserID, err := r.client.Get(r.ctx, key).Result()
		if err != nil {
			continue
		}
		if tokenUserID == userID {
			// Extract token from key (remove "blacklist_token:" prefix)
			token := key[len("blacklist_token:"):]
			blacklistedTokens = append(blacklistedTokens, token)
		}
	}
	
	return blacklistedTokens, iter.Err()
}

// GetActiveTokenCount returns the number of active refresh tokens for a user
func (r *TokenRepository) GetActiveTokenCount(userID string) (int, error) {
	userKey := fmt.Sprintf("user_tokens:%s", userID)
	count, err := r.client.SCard(r.ctx, userKey).Result()
	return int(count), err
}