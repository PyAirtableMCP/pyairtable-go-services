package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	"github.com/rs/cors"
	"github.com/sirupsen/logrus"

	"github.com/pyairtable/pyairtable-compose/go-services/user-service/internal/services"
	"github.com/pyairtable/pyairtable-compose/go-services/shared/middleware"
)

// Server represents the GraphQL server
type Server struct {
	schema              *graphql.Schema
	resolver            *Resolver
	logger              *logrus.Logger
	port                string
	enableIntrospection bool
	enablePlayground    bool
}

// Config holds server configuration
type Config struct {
	Port                string
	EnableIntrospection bool
	EnablePlayground    bool
	CORSOrigins         []string
}

// NewServer creates a new GraphQL server
func NewServer(
	userService services.UserService,
	authService services.AuthService,
	permissionService services.PermissionService,
	notificationService services.NotificationService,
	activityService services.ActivityService,
	logger *logrus.Logger,
	config Config,
) (*Server, error) {
	// Read schema file
	schemaString, err := os.ReadFile("graphql/schema.graphql")
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}

	// Create resolver
	resolver := NewResolver(
		userService,
		authService,
		permissionService,
		notificationService,
		activityService,
		logger,
	)

	// Parse schema
	schema, err := graphql.ParseSchema(string(schemaString), resolver, graphql.UseFieldResolvers())
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL schema: %w", err)
	}

	return &Server{
		schema:              schema,
		resolver:            resolver,
		logger:              logger,
		port:                config.Port,
		enableIntrospection: config.EnableIntrospection,
		enablePlayground:    config.EnablePlayground,
	}, nil
}

// Start starts the GraphQL server
func (s *Server) Start() error {
	router := mux.NewRouter()

	// Add middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger(s.logger))
	router.Use(middleware.Recovery(s.logger))
	router.Use(middleware.Metrics())

	// CORS middleware
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // Configure based on environment
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})
	router.Use(c.Handler)

	// Health check endpoint
	router.HandleFunc("/health", s.healthCheckHandler).Methods("GET")

	// Batch endpoints for DataLoader
	router.HandleFunc("/batch/users", s.authMiddleware(s.batchUsersHandler)).Methods("POST")
	router.HandleFunc("/batch/permissions", s.authMiddleware(s.batchPermissionsHandler)).Methods("POST")
	router.HandleFunc("/batch/notifications", s.authMiddleware(s.batchNotificationsHandler)).Methods("POST")

	// GraphQL endpoint
	graphqlHandler := &relay.Handler{Schema: s.schema}
	router.Handle("/graphql", s.authMiddleware(graphqlHandler)).Methods("POST")

	// GraphQL Playground (development only)
	if s.enablePlayground {
		router.HandleFunc("/graphql", s.playgroundHandler).Methods("GET")
	}

	// Schema introspection endpoint (development only)
	if s.enableIntrospection {
		router.HandleFunc("/schema", s.schemaHandler).Methods("GET")
	}

	s.logger.WithField("port", s.port).Info("Starting GraphQL server")

	server := &http.Server{
		Addr:         ":" + s.port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return server.ListenAndServe()
}

// authMiddleware adds authentication context to requests
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract JWT token from Authorization header
		token := r.Header.Get("Authorization")
		if token != "" && len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
			
			// Add token to context (simplified - in real implementation, validate JWT)
			ctx := context.WithValue(r.Context(), "token", token)
			
			// Extract user info from headers (set by gateway)
			userID := r.Header.Get("X-User-ID")
			userRole := r.Header.Get("X-User-Role")
			
			if userID != "" {
				ctx = context.WithValue(ctx, "userID", userID)
				ctx = context.WithValue(ctx, "userRole", userRole)
			}
			
			r = r.WithContext(ctx)
		}
		
		next.ServeHTTP(w, r)
	})
}

// Health check handler
func (s *Server) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"service":   "user-service",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// Batch users handler for DataLoader
func (s *Server) batchUsersHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		IDs []string `json:"ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	users, err := s.resolver.userService.GetUsersByIDs(r.Context(), request.IDs)
	if err != nil {
		s.logger.WithError(err).Error("Failed to batch load users")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// Batch permissions handler for DataLoader
func (s *Server) batchPermissionsHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		UserIDs []string `json:"userIds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build response map
	response := make(map[string]interface{})
	for _, userID := range request.UserIDs {
		permissions, err := s.resolver.permissionService.GetUserPermissions(r.Context(), userID)
		if err != nil {
			s.logger.WithError(err).WithField("userID", userID).Error("Failed to get user permissions")
			response[userID] = []interface{}{}
			continue
		}
		response[userID] = permissions
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Batch notifications handler for DataLoader
func (s *Server) batchNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		UserIDs []string `json:"userIds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build response map
	response := make(map[string]interface{})
	for _, userID := range request.UserIDs {
		params := &services.NotificationParams{
			UserID: userID,
			Limit:  20,
			Offset: 0,
		}
		
		notifications, err := s.resolver.notificationService.GetUserNotifications(r.Context(), params)
		if err != nil {
			s.logger.WithError(err).WithField("userID", userID).Error("Failed to get user notifications")
			response[userID] = []interface{}{}
			continue
		}
		response[userID] = notifications
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GraphQL Playground handler
func (s *Server) playgroundHandler(w http.ResponseWriter, r *http.Request) {
	playgroundHTML := `<!DOCTYPE html>
<html>
<head>
  <meta charset=utf-8/>
  <meta name="viewport" content="user-scalable=no, initial-scale=1.0, minimum-scale=1.0, maximum-scale=1.0, minimal-ui">
  <title>PyAirtable User Service GraphQL Playground</title>
  <link rel="stylesheet" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/css/index.css" />
  <link rel="shortcut icon" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/favicon.png" />
  <script src="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/js/middleware.js"></script>
</head>
<body>
  <div id="root">
    <style>
      body {
        background-color: rgb(23, 42, 58);
        font-family: Open Sans, sans-serif;
        height: 90vh;
      }
      #root {
        height: 100%;
        width: 100%;
        display: flex;
        align-items: center;
        justify-content: center;
      }
      .loading {
        font-size: 32px;
        font-weight: 200;
        color: rgba(255, 255, 255, .6);
        margin-left: 20px;
      }
      img {
        width: 78px;
        height: 78px;
      }
      .title {
        font-weight: 400;
      }
    </style>
    <img src="//cdn.jsdelivr.net/npm/graphql-playground-react/build/logo.png" alt="">
    <div class="loading"> Loading
      <span class="title">GraphQL Playground</span>
    </div>
  </div>
  <script>window.addEventListener('load', function (event) {
      GraphQLPlayground.init(document.getElementById('root'), {
        endpoint: '/graphql',
        settings: {
          'editor.theme': 'dark',
          'editor.fontSize': 14,
          'editor.fontFamily': '"Source Code Pro", "Consolas", "Inconsolata", "Droid Sans Mono", "Monaco", monospace',
          'editor.cursorShape': 'line',
          'prettier.printWidth': 80,
          'prettier.tabWidth': 2,
          'prettier.useTabs': false,
          'request.credentials': 'include',
        },
        tabs: [
          {
            endpoint: '/graphql',
            query: 'query GetMe {\n  me {\n    id\n    email\n    firstName\n    lastName\n    role\n    status\n  }\n}',
          },
        ],
      })
    })</script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(playgroundHTML))
}

// Schema introspection handler
func (s *Server) schemaHandler(w http.ResponseWriter, r *http.Request) {
	schemaString := s.schema.String()
	
	response := map[string]interface{}{
		"schema": schemaString,
		"service": "user-service",
		"federation": "v2.3",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down GraphQL server")
	// Add cleanup logic here
	return nil
}