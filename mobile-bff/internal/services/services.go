package services

import (
	"log/slog"

	"github.com/Reg-Kris/pyairtable-mobile-bff/internal/config"
	"github.com/Reg-Kris/pyairtable-mobile-bff/internal/repositories"
)

type Services struct {
	// Add your service interfaces here
	// UserService UserServiceInterface
	
	config *config.Config
	logger *slog.Logger
	repos  *repositories.Repositories
}

func New(repos *repositories.Repositories, config *config.Config, logger *slog.Logger) *Services {
	return &Services{
		config: config,
		logger: logger,
		repos:  repos,
		// Initialize your services here
		// UserService: NewUserService(repos.UserRepo, logger),
	}
}