package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/pyairtable/go-services/plugin-service/internal/config"
	"github.com/pyairtable/go-services/plugin-service/internal/models"
)

// RegistryService manages plugin registries and discovery
type RegistryService struct {
	config          *config.RegistryConfig
	logger          *zap.Logger
	registryRepo    RegistryRepository
	httpClient      *http.Client
	cache           RegistryCache
	syncMutex       sync.RWMutex
	lastSyncTime    time.Time
}

// RegistryRepository interface for registry data persistence
type RegistryRepository interface {
	CreateRegistry(ctx context.Context, registry *models.PluginRegistry) error
	GetRegistry(ctx context.Context, id uuid.UUID) (*models.PluginRegistry, error)
	GetRegistryByName(ctx context.Context, name string) (*models.PluginRegistry, error)
	UpdateRegistry(ctx context.Context, registry *models.PluginRegistry) error
	DeleteRegistry(ctx context.Context, id uuid.UUID) error
	ListRegistries(ctx context.Context) ([]*models.PluginRegistry, error)
	GetEnabledRegistries(ctx context.Context) ([]*models.PluginRegistry, error)
}

// RegistryCache interface for caching registry data
type RegistryCache interface {
	GetPlugin(registryName, pluginName string) (*RegistryPlugin, bool)
	SetPlugin(registryName, pluginName string, plugin *RegistryPlugin, ttl time.Duration)
	GetPluginList(registryName string) ([]*RegistryPlugin, bool)
	SetPluginList(registryName string, plugins []*RegistryPlugin, ttl time.Duration)
	InvalidateRegistry(registryName string)
	Clear()
}

// RegistryPlugin represents a plugin in a registry
type RegistryPlugin struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	Description     string                 `json:"description"`
	Type            models.PluginType      `json:"type"`
	Author          string                 `json:"author"`
	License         string                 `json:"license"`
	Homepage        string                 `json:"homepage"`
	Repository      string                 `json:"repository"`
	Tags            []string               `json:"tags"`
	Categories      []string               `json:"categories"`
	DownloadURL     string                 `json:"download_url"`
	ChecksumSHA256  string                 `json:"checksum_sha256"`
	Size            int64                  `json:"size"`
	Dependencies    []models.PluginDependency `json:"dependencies"`
	Permissions     []models.Permission    `json:"permissions"`
	ResourceLimits  models.ResourceLimits  `json:"resource_limits"`
	MinAPIVersion   string                 `json:"min_api_version"`
	MaxAPIVersion   string                 `json:"max_api_version"`
	PublishedAt     time.Time              `json:"published_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	Downloads       int64                  `json:"downloads"`
	Rating          float64                `json:"rating"`
	ReviewCount     int64                  `json:"review_count"`
	Verified        bool                   `json:"verified"`
	Featured        bool                   `json:"featured"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// RegistrySearchRequest represents a search request
type RegistrySearchRequest struct {
	Query       string             `json:"query,omitempty"`
	Type        *models.PluginType `json:"type,omitempty"`
	Category    string             `json:"category,omitempty"`
	Tag         string             `json:"tag,omitempty"`
	Author      string             `json:"author,omitempty"`
	Featured    *bool              `json:"featured,omitempty"`
	Verified    *bool              `json:"verified,omitempty"`
	MinRating   *float64           `json:"min_rating,omitempty"`
	SortBy      string             `json:"sort_by,omitempty"` // name, downloads, rating, updated_at
	SortOrder   string             `json:"sort_order,omitempty"` // asc, desc
	Limit       int                `json:"limit,omitempty"`
	Offset      int                `json:"offset,omitempty"`
	Registries  []string           `json:"registries,omitempty"` // specific registries to search
}

// RegistrySearchResponse represents search results
type RegistrySearchResponse struct {
	Plugins    []*RegistryPlugin `json:"plugins"`
	Total      int64             `json:"total"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
	Registries []string          `json:"registries_searched"`
}

// RegistryMetadata contains registry metadata
type RegistryMetadata struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	BaseURL      string            `json:"base_url"`
	Type         string            `json:"type"`
	LastSync     time.Time         `json:"last_sync"`
	PluginCount  int64             `json:"plugin_count"`
	Categories   []string          `json:"categories"`
	TotalSize    int64             `json:"total_size"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// NewRegistryService creates a new registry service
func NewRegistryService(config *config.RegistryConfig, logger *zap.Logger, repo RegistryRepository, cache RegistryCache) *RegistryService {
	service := &RegistryService{
		config:       config,
		logger:       logger,
		registryRepo: repo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: cache,
	}

	// Initialize default registries if they don't exist
	go service.initializeDefaultRegistries()

	// Start periodic sync if enabled
	if config.SyncInterval > 0 {
		go service.startPeriodicSync()
	}

	return service
}

// SearchPlugins searches for plugins across configured registries
func (s *RegistryService) SearchPlugins(ctx context.Context, req *RegistrySearchRequest) (*RegistrySearchResponse, error) {
	s.logger.Info("Searching plugins", zap.String("query", req.Query))

	// Get enabled registries
	registries, err := s.registryRepo.GetEnabledRegistries(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled registries: %w", err)
	}

	// Filter registries if specified in request
	if len(req.Registries) > 0 {
		filteredRegistries := make([]*models.PluginRegistry, 0)
		for _, registry := range registries {
			for _, reqRegistry := range req.Registries {
				if registry.Name == reqRegistry {
					filteredRegistries = append(filteredRegistries, registry)
					break
				}
			}
		}
		registries = filteredRegistries
	}

	var allPlugins []*RegistryPlugin
	var searchedRegistries []string

	// Search each registry
	for _, registry := range registries {
		plugins, err := s.searchRegistry(ctx, registry, req)
		if err != nil {
			s.logger.Warn("Failed to search registry", 
				zap.String("registry", registry.Name), 
				zap.Error(err))
			continue
		}

		allPlugins = append(allPlugins, plugins...)
		searchedRegistries = append(searchedRegistries, registry.Name)
	}

	// Apply filtering and sorting
	filteredPlugins := s.filterPlugins(allPlugins, req)
	sortedPlugins := s.sortPlugins(filteredPlugins, req.SortBy, req.SortOrder)

	// Apply pagination
	total := int64(len(sortedPlugins))
	if req.Limit == 0 {
		req.Limit = 50 // default limit
	}
	
	start := req.Offset
	end := start + req.Limit
	if end > len(sortedPlugins) {
		end = len(sortedPlugins)
	}
	if start > len(sortedPlugins) {
		start = len(sortedPlugins)
	}

	paginatedPlugins := sortedPlugins[start:end]

	return &RegistrySearchResponse{
		Plugins:    paginatedPlugins,
		Total:      total,
		Limit:      req.Limit,
		Offset:     req.Offset,
		Registries: searchedRegistries,
	}, nil
}

// GetPlugin retrieves a specific plugin from registries
func (s *RegistryService) GetPlugin(ctx context.Context, pluginName string, registryName string) (*RegistryPlugin, error) {
	// Try cache first
	if s.config.CacheEnabled {
		if plugin, found := s.cache.GetPlugin(registryName, pluginName); found {
			return plugin, nil
		}
	}

	// Get registry
	registry, err := s.registryRepo.GetRegistryByName(ctx, registryName)
	if err != nil {
		return nil, fmt.Errorf("registry not found: %s", registryName)
	}

	if !registry.Enabled {
		return nil, fmt.Errorf("registry is disabled: %s", registryName)
	}

	// Fetch plugin from registry
	plugin, err := s.fetchPluginFromRegistry(ctx, registry, pluginName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plugin from registry: %w", err)
	}

	// Cache the result
	if s.config.CacheEnabled {
		s.cache.SetPlugin(registryName, pluginName, plugin, s.config.CacheTTL)
	}

	return plugin, nil
}

// SyncRegistries synchronizes all enabled registries
func (s *RegistryService) SyncRegistries(ctx context.Context) error {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()

	s.logger.Info("Starting registry synchronization")

	registries, err := s.registryRepo.GetEnabledRegistries(ctx)
	if err != nil {
		return fmt.Errorf("failed to get enabled registries: %w", err)
	}

	var syncErrors []error
	for _, registry := range registries {
		if err := s.syncRegistry(ctx, registry); err != nil {
			s.logger.Error("Failed to sync registry", 
				zap.String("registry", registry.Name), 
				zap.Error(err))
			syncErrors = append(syncErrors, err)
		}
	}

	s.lastSyncTime = time.Now()

	if len(syncErrors) > 0 {
		return fmt.Errorf("failed to sync %d registries", len(syncErrors))
	}

	s.logger.Info("Registry synchronization completed")
	return nil
}

// GetRegistryMetadata returns metadata for a registry
func (s *RegistryService) GetRegistryMetadata(ctx context.Context, registryName string) (*RegistryMetadata, error) {
	registry, err := s.registryRepo.GetRegistryByName(ctx, registryName)
	if err != nil {
		return nil, fmt.Errorf("registry not found: %s", registryName)
	}

	metadata, err := s.fetchRegistryMetadata(ctx, registry)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry metadata: %w", err)
	}

	return metadata, nil
}

// InstallFromRegistry installs a plugin from a registry
func (s *RegistryService) InstallFromRegistry(ctx context.Context, registryName, pluginName, version string, userID, workspaceID uuid.UUID) (*models.PluginInstallation, error) {
	// Get plugin info from registry
	registryPlugin, err := s.GetPlugin(ctx, pluginName, registryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get plugin from registry: %w", err)
	}

	// Download plugin binary
	binary, err := s.downloadPlugin(ctx, registryPlugin.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download plugin: %w", err)
	}

	// Verify checksum
	if err := s.verifyChecksum(binary, registryPlugin.ChecksumSHA256); err != nil {
		return nil, fmt.Errorf("checksum verification failed: %w", err)
	}

	// Create plugin model
	plugin := &models.Plugin{
		Name:            registryPlugin.Name,
		Version:         registryPlugin.Version,
		Type:            registryPlugin.Type,
		Description:     registryPlugin.Description,
		Tags:            registryPlugin.Tags,
		Categories:      registryPlugin.Categories,
		WasmBinary:      binary,
		Dependencies:    registryPlugin.Dependencies,
		Permissions:     registryPlugin.Permissions,
		ResourceLimits:  registryPlugin.ResourceLimits,
		DeveloperID:     userID, // The user installing becomes the developer
		WorkspaceID:     &workspaceID,
	}

	// This would integrate with the main PluginService
	// For now, return a placeholder
	installation := &models.PluginInstallation{
		ID:          uuid.New(),
		PluginID:    uuid.New(), // Would be set after creating plugin
		WorkspaceID: workspaceID,
		UserID:      userID,
		Version:     version,
		Status:      models.PluginStatusInstalled,
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
	}

	return installation, nil
}

// Private methods

func (s *RegistryService) searchRegistry(ctx context.Context, registry *models.PluginRegistry, req *RegistrySearchRequest) ([]*RegistryPlugin, error) {
	// Check cache first
	if s.config.CacheEnabled {
		if plugins, found := s.cache.GetPluginList(registry.Name); found {
			return s.filterPluginsLocal(plugins, req), nil
		}
	}

	// Fetch from registry API
	url := fmt.Sprintf("%s/api/v1/plugins/search", registry.URL)
	plugins, err := s.fetchPluginsFromAPI(ctx, url, req)
	if err != nil {
		return nil, err
	}

	// Cache the results
	if s.config.CacheEnabled {
		s.cache.SetPluginList(registry.Name, plugins, s.config.CacheTTL)
	}

	return plugins, nil
}

func (s *RegistryService) fetchPluginsFromAPI(ctx context.Context, url string, req *RegistrySearchRequest) ([]*RegistryPlugin, error) {
	// Create HTTP request with search parameters
	apiReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add query parameters
	q := apiReq.URL.Query()
	if req.Query != "" {
		q.Add("q", req.Query)
	}
	if req.Type != nil {
		q.Add("type", string(*req.Type))
	}
	if req.Category != "" {
		q.Add("category", req.Category)
	}
	if req.Tag != "" {
		q.Add("tag", req.Tag)
	}
	apiReq.URL.RawQuery = q.Encode()

	// Execute request
	resp, err := s.httpClient.Do(apiReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry API returned status %d", resp.StatusCode)
	}

	// Parse response
	var searchResp struct {
		Plugins []*RegistryPlugin `json:"plugins"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	return searchResp.Plugins, nil
}

func (s *RegistryService) fetchPluginFromRegistry(ctx context.Context, registry *models.PluginRegistry, pluginName string) (*RegistryPlugin, error) {
	url := fmt.Sprintf("%s/api/v1/plugins/%s", registry.URL, pluginName)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("plugin not found: %s", pluginName)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry API returned status %d", resp.StatusCode)
	}

	var plugin RegistryPlugin
	if err := json.NewDecoder(resp.Body).Decode(&plugin); err != nil {
		return nil, err
	}

	return &plugin, nil
}

func (s *RegistryService) fetchRegistryMetadata(ctx context.Context, registry *models.PluginRegistry) (*RegistryMetadata, error) {
	url := fmt.Sprintf("%s/api/v1/metadata", registry.URL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry API returned status %d", resp.StatusCode)
	}

	var metadata RegistryMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func (s *RegistryService) downloadPlugin(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (s *RegistryService) verifyChecksum(data []byte, expectedChecksum string) error {
	// Implementation would verify SHA-256 checksum
	// This is a placeholder
	if len(expectedChecksum) == 0 {
		return fmt.Errorf("no checksum provided")
	}
	return nil
}

func (s *RegistryService) filterPlugins(plugins []*RegistryPlugin, req *RegistrySearchRequest) []*RegistryPlugin {
	var filtered []*RegistryPlugin

	for _, plugin := range plugins {
		if s.matchesFilter(plugin, req) {
			filtered = append(filtered, plugin)
		}
	}

	return filtered
}

func (s *RegistryService) filterPluginsLocal(plugins []*RegistryPlugin, req *RegistrySearchRequest) []*RegistryPlugin {
	// Apply local filtering to cached results
	return s.filterPlugins(plugins, req)
}

func (s *RegistryService) matchesFilter(plugin *RegistryPlugin, req *RegistrySearchRequest) bool {
	if req.Type != nil && plugin.Type != *req.Type {
		return false
	}

	if req.Featured != nil && plugin.Featured != *req.Featured {
		return false
	}

	if req.Verified != nil && plugin.Verified != *req.Verified {
		return false
	}

	if req.MinRating != nil && plugin.Rating < *req.MinRating {
		return false
	}

	if req.Author != "" && plugin.Author != req.Author {
		return false
	}

	if req.Category != "" {
		found := false
		for _, category := range plugin.Categories {
			if category == req.Category {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if req.Tag != "" {
		found := false
		for _, tag := range plugin.Tags {
			if tag == req.Tag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (s *RegistryService) sortPlugins(plugins []*RegistryPlugin, sortBy, sortOrder string) []*RegistryPlugin {
	// Implementation would sort plugins based on criteria
	// This is a placeholder that returns the original slice
	return plugins
}

func (s *RegistryService) syncRegistry(ctx context.Context, registry *models.PluginRegistry) error {
	s.logger.Info("Syncing registry", zap.String("registry", registry.Name))

	// Invalidate cache for this registry
	if s.config.CacheEnabled {
		s.cache.InvalidateRegistry(registry.Name)
	}

	// Fetch latest plugin list
	req := &RegistrySearchRequest{Limit: 1000} // Get all plugins
	plugins, err := s.fetchPluginsFromAPI(ctx, fmt.Sprintf("%s/api/v1/plugins/search", registry.URL), req)
	if err != nil {
		return err
	}

	// Cache the updated plugin list
	if s.config.CacheEnabled {
		s.cache.SetPluginList(registry.Name, plugins, s.config.CacheTTL)
	}

	// Update registry metadata
	registry.UpdatedAt = time.Now()
	if err := s.registryRepo.UpdateRegistry(ctx, registry); err != nil {
		s.logger.Error("Failed to update registry", zap.Error(err))
	}

	s.logger.Info("Registry sync completed", 
		zap.String("registry", registry.Name),
		zap.Int("plugins", len(plugins)))

	return nil
}

func (s *RegistryService) initializeDefaultRegistries() {
	ctx := context.Background()

	// Check if official registry exists
	if _, err := s.registryRepo.GetRegistryByName(ctx, "official"); err != nil {
		// Create official registry
		official := &models.PluginRegistry{
			ID:      uuid.New(),
			Name:    "official",
			URL:     s.config.OfficialRegistry,
			Type:    "official",
			Enabled: true,
			TrustedKeys: []string{}, // Would contain actual public keys
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.registryRepo.CreateRegistry(ctx, official); err != nil {
			s.logger.Error("Failed to create official registry", zap.Error(err))
		}
	}
}

func (s *RegistryService) startPeriodicSync() {
	ticker := time.NewTicker(s.config.SyncInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		if err := s.SyncRegistries(ctx); err != nil {
			s.logger.Error("Periodic registry sync failed", zap.Error(err))
		}
	}
}