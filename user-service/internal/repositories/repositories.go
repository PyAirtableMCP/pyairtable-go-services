package repositories

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Repositories struct {
	// Add your repository interfaces here
	// UserRepo UserRepositoryInterface
	
	db    *gorm.DB
	redis *redis.Client
}

func New(db *gorm.DB, redis *redis.Client) *Repositories {
	return &Repositories{
		db:    db,
		redis: redis,
		// Initialize your repositories here
		// UserRepo: NewUserRepository(db),
	}
}