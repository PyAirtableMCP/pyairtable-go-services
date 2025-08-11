package integration

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type IntegrationTestSuite struct {
	suite.Suite
	app *fiber.App
}

func (suite *IntegrationTestSuite) SetupSuite() {
	// Setup test environment
	os.Setenv("GO_ENV", "test")
	os.Setenv("DB_NAME", "api_gateway_db_test")
	
	// Initialize your app here
	// suite.app = setupTestApp()
}

func (suite *IntegrationTestSuite) TearDownSuite() {
	// Cleanup
}

func (suite *IntegrationTestSuite) TestHealthEndpoint() {
	// Skip if app is not initialized
	if suite.app == nil {
		suite.T().Skip("App not initialized")
	}

	req, _ := http.NewRequest("GET", "/health", nil)
	resp, err := suite.app.Test(req, -1)
	
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 200, resp.StatusCode)
}

func (suite *IntegrationTestSuite) TestMetricsEndpoint() {
	// Skip if app is not initialized
	if suite.app == nil {
		suite.T().Skip("App not initialized")
	}

	req, _ := http.NewRequest("GET", "/metrics", nil)
	resp, err := suite.app.Test(req, -1)
	
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 200, resp.StatusCode)
}

func TestIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}
	
	suite.Run(t, new(IntegrationTestSuite))
}