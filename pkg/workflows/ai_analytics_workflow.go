package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// AIAnalyticsWorkflow implements AI-powered analytics with Gemini integration
type AIAnalyticsWorkflow struct {
	instance        *WorkflowInstance
	config          *AIAnalyticsConfig
	eventStore      EventStore
	geminiClient    GeminiClient
	dataConnector   DataConnector
	mlEngine        MLEngine
	reportGenerator ReportGenerator
	alertManager    AlertManager
	scheduler       SchedulerService
	mu              sync.RWMutex
}

// GeminiClient interface for Google Gemini AI operations
type GeminiClient interface {
	GenerateContent(ctx context.Context, prompt string, config GeminiConfig) (*AIResponse, error)
	AnalyzeData(ctx context.Context, data interface{}, analysisType AnalysisType, config GeminiConfig) (*AnalysisResult, error)
	ProcessNLQuery(ctx context.Context, query string, schema *DataSchema, config GeminiConfig) (*QueryResult, error)
	GenerateInsights(ctx context.Context, data interface{}, context map[string]interface{}, config GeminiConfig) (*InsightResult, error)
	DetectAnomalies(ctx context.Context, data interface{}, config GeminiConfig) (*AnomalyResult, error)
}

// DataConnector interface for data source operations
type DataConnector interface {
	ConnectToSource(ctx context.Context, source DataSource) (DataConnection, error)
	ExtractData(ctx context.Context, connection DataConnection, query string) (*DataSet, error)
	GetSchema(ctx context.Context, connection DataConnection) (*DataSchema, error)
	ValidateData(ctx context.Context, data *DataSet, rules []ValidationRule) (*ValidationResult, error)
	TransformData(ctx context.Context, data *DataSet, transformations []Transformation) (*DataSet, error)
}

// MLEngine interface for machine learning operations
type MLEngine interface {
	TrainModel(ctx context.Context, config MLModelConfig, data *DataSet) (*MLModel, error)
	PredictWithModel(ctx context.Context, model *MLModel, data *DataSet) (*PredictionResult, error)
	EvaluateModel(ctx context.Context, model *MLModel, testData *DataSet) (*ModelEvaluation, error)
	DetectAnomalies(ctx context.Context, data *DataSet, method string) (*AnomalyDetectionResult, error)
	PerformTimeSeriesAnalysis(ctx context.Context, data *DataSet, config TimeSeriesConfig) (*TimeSeriesResult, error)
}

// ReportGenerator interface for report generation
type ReportGenerator interface {
	GenerateReport(ctx context.Context, config ReportConfig, data *ReportData) (*GeneratedReport, error)
	ScheduleReport(ctx context.Context, config ReportConfig, schedule ScheduleConfig) (*ScheduledReport, error)
	GetReportTemplates(ctx context.Context) ([]*ReportTemplate, error)
	CustomizeTemplate(ctx context.Context, templateID uuid.UUID, customization map[string]interface{}) (*ReportTemplate, error)
}

// AlertManager interface for alert management
type AlertManager interface {
	CreateAlert(ctx context.Context, config AlertConfig) (*Alert, error)
	EvaluateAlerts(ctx context.Context, data *DataSet) ([]*AlertTrigger, error)
	SendAlert(ctx context.Context, alert *Alert, trigger *AlertTrigger) error
	ManageAlertSubscriptions(ctx context.Context, userID uuid.UUID, subscriptions []AlertSubscription) error
}

// SchedulerService interface for scheduling analytics tasks
type SchedulerService interface {
	ScheduleAnalysis(ctx context.Context, config ScheduleConfig, analysisConfig interface{}) (*ScheduledTask, error)
	ExecuteScheduledTask(ctx context.Context, task *ScheduledTask) error
	GetScheduledTasks(ctx context.Context, workflowID uuid.UUID) ([]*ScheduledTask, error)
	CancelScheduledTask(ctx context.Context, taskID uuid.UUID) error
}

// Domain entities
type AIResponse struct {
	Content     string                 `json:"content"`
	Confidence  float64                `json:"confidence"`
	Metadata    map[string]interface{} `json:"metadata"`
	TokensUsed  int                    `json:"tokens_used"`
	Model       string                 `json:"model"`
	GeneratedAt time.Time              `json:"generated_at"`
}

type AnalysisResult struct {
	Type         AnalysisType           `json:"type"`
	Summary      string                 `json:"summary"`
	Insights     []*Insight             `json:"insights"`
	Statistics   map[string]interface{} `json:"statistics"`
	Visualizations []*Visualization     `json:"visualizations"`
	Confidence   float64                `json:"confidence"`
	Recommendations []*Recommendation   `json:"recommendations"`
}

type QueryResult struct {
	SQL          string                 `json:"sql"`
	Results      *DataSet               `json:"results"`
	Explanation  string                 `json:"explanation"`
	Confidence   float64                `json:"confidence"`
	ExecutionTime time.Duration         `json:"execution_time"`
}

type InsightResult struct {
	Insights     []*Insight             `json:"insights"`
	Trends       []*Trend               `json:"trends"`
	Patterns     []*Pattern             `json:"patterns"`
	Correlations []*Correlation         `json:"correlations"`
	Summary      string                 `json:"summary"`
}

type AnomalyResult struct {
	Anomalies    []*Anomaly             `json:"anomalies"`
	AnomalyScore float64                `json:"anomaly_score"`
	Threshold    float64                `json:"threshold"`
	Method       string                 `json:"method"`
	Confidence   float64                `json:"confidence"`
}

type DataConnection interface {
	Query(ctx context.Context, query string) (*DataSet, error)
	Close() error
	IsValid() bool
	GetConnectionInfo() map[string]interface{}
}

type DataSet struct {
	ID          uuid.UUID              `json:"id"`
	Schema      *DataSchema            `json:"schema"`
	Rows        []map[string]interface{} `json:"rows"`
	Metadata    map[string]interface{} `json:"metadata"`
	RowCount    int                    `json:"row_count"`
	ColumnCount int                    `json:"column_count"`
	Size        int64                  `json:"size"`
	CreatedAt   time.Time              `json:"created_at"`
}

type DataSchema struct {
	Tables  map[string]*TableSchema    `json:"tables"`
	Version string                     `json:"version"`
}

type TableSchema struct {
	Name        string                 `json:"name"`
	Columns     []*ColumnSchema        `json:"columns"`
	PrimaryKeys []string               `json:"primary_keys"`
	ForeignKeys []*ForeignKey          `json:"foreign_keys"`
	Indexes     []*Index               `json:"indexes"`
}

type ColumnSchema struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Nullable     bool   `json:"nullable"`
	DefaultValue string `json:"default_value,omitempty"`
	Description  string `json:"description,omitempty"`
}

type ForeignKey struct {
	Column          string `json:"column"`
	ReferencedTable string `json:"referenced_table"`
	ReferencedColumn string `json:"referenced_column"`
}

type Index struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

type ValidationResult struct {
	IsValid     bool                   `json:"is_valid"`
	Errors      []*ValidationError     `json:"errors"`
	Warnings    []*ValidationWarning   `json:"warnings"`
	Summary     string                 `json:"summary"`
	Score       float64                `json:"score"`
}

type ValidationError struct {
	Rule        string `json:"rule"`
	Message     string `json:"message"`
	Column      string `json:"column,omitempty"`
	Row         int    `json:"row,omitempty"`
	Severity    string `json:"severity"`
}

type ValidationWarning struct {
	Rule        string `json:"rule"`
	Message     string `json:"message"`
	Column      string `json:"column,omitempty"`
	Suggestion  string `json:"suggestion"`
}

type MLModel struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Algorithm   string                 `json:"algorithm"`
	Version     string                 `json:"version"`
	Parameters  map[string]interface{} `json:"parameters"`
	Performance *ModelPerformance      `json:"performance"`
	TrainedAt   time.Time              `json:"trained_at"`
	ModelData   []byte                 `json:"model_data"`
}

type ModelPerformance struct {
	Accuracy    float64                `json:"accuracy"`
	Precision   float64                `json:"precision"`
	Recall      float64                `json:"recall"`
	F1Score     float64                `json:"f1_score"`
	AUC         float64                `json:"auc"`
	RMSE        float64                `json:"rmse,omitempty"`
	MAE         float64                `json:"mae,omitempty"`
	Metrics     map[string]interface{} `json:"metrics"`
}

type PredictionResult struct {
	Predictions  []interface{}          `json:"predictions"`
	Probabilities []float64             `json:"probabilities,omitempty"`
	Confidence   []float64              `json:"confidence"`
	Features     []string               `json:"features"`
	ModelVersion string                 `json:"model_version"`
}

type ModelEvaluation struct {
	Performance     *ModelPerformance      `json:"performance"`
	ConfusionMatrix [][]int                `json:"confusion_matrix,omitempty"`
	FeatureImportance map[string]float64   `json:"feature_importance"`
	CrossValidation *CrossValidationResult `json:"cross_validation"`
}

type AnomalyDetectionResult struct {
	Anomalies       []*DataAnomaly         `json:"anomalies"`
	AnomalyScores   []float64              `json:"anomaly_scores"`
	Threshold       float64                `json:"threshold"`
	Method          string                 `json:"method"`
	TotalAnomalies  int                    `json:"total_anomalies"`
}

type TimeSeriesResult struct {
	Forecast        []*ForecastPoint       `json:"forecast"`
	Seasonality     *SeasonalityAnalysis   `json:"seasonality"`
	Trends          []*TrendAnalysis       `json:"trends"`
	ChangePoints    []*ChangePoint         `json:"change_points"`
	Accuracy        *ForecastAccuracy      `json:"accuracy"`
}

type ReportData struct {
	DataSets        []*DataSet             `json:"datasets"`
	Analysis        []*AnalysisResult      `json:"analysis"`
	Visualizations  []*Visualization       `json:"visualizations"`
	Insights        []*Insight             `json:"insights"`
	Metadata        map[string]interface{} `json:"metadata"`
}

type GeneratedReport struct {
	ID          uuid.UUID              `json:"id"`
	Title       string                 `json:"title"`
	Content     string                 `json:"content"`
	Format      string                 `json:"format"`
	Data        []byte                 `json:"data"`
	GeneratedAt time.Time              `json:"generated_at"`
	Recipients  []string               `json:"recipients"`
	Status      string                 `json:"status"`
}

type ScheduledReport struct {
	ID          uuid.UUID              `json:"id"`
	ReportConfig ReportConfig          `json:"report_config"`
	Schedule    ScheduleConfig         `json:"schedule"`
	Status      string                 `json:"status"`
	NextRun     *time.Time             `json:"next_run,omitempty"`
	LastRun     *time.Time             `json:"last_run,omitempty"`
}

type ReportTemplate struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	Template    string                 `json:"template"`
	Variables   []string               `json:"variables"`
	Popular     bool                   `json:"popular"`
}

type Alert struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Condition   string                 `json:"condition"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type AlertTrigger struct {
	AlertID     uuid.UUID              `json:"alert_id"`
	Value       float64                `json:"value"`
	Threshold   float64                `json:"threshold"`
	Condition   string                 `json:"condition"`
	Data        map[string]interface{} `json:"data"`
	TriggeredAt time.Time              `json:"triggered_at"`
}

type AlertSubscription struct {
	UserID      uuid.UUID `json:"user_id"`
	AlertID     uuid.UUID `json:"alert_id"`
	Channel     string    `json:"channel"`
	Enabled     bool      `json:"enabled"`
}

type ScheduledTask struct {
	ID          uuid.UUID              `json:"id"`
	WorkflowID  uuid.UUID              `json:"workflow_id"`
	Type        string                 `json:"type"`
	Config      json.RawMessage        `json:"config"`
	Schedule    ScheduleConfig         `json:"schedule"`
	Status      string                 `json:"status"`
	NextRun     *time.Time             `json:"next_run,omitempty"`
	LastRun     *time.Time             `json:"last_run,omitempty"`
}

type Insight struct {
	ID          uuid.UUID              `json:"id"`
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Value       interface{}            `json:"value"`
	Confidence  float64                `json:"confidence"`
	Priority    int                    `json:"priority"`
	Tags        []string               `json:"tags"`
}

type Trend struct {
	Direction   string    `json:"direction"`
	Strength    float64   `json:"strength"`
	Period      string    `json:"period"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date"`
	Description string    `json:"description"`
}

type Pattern struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Confidence  float64                `json:"confidence"`
	Data        map[string]interface{} `json:"data"`
}

type Correlation struct {
	Variable1   string  `json:"variable1"`
	Variable2   string  `json:"variable2"`
	Coefficient float64 `json:"coefficient"`
	PValue      float64 `json:"p_value"`
	Significance string `json:"significance"`
}

type Anomaly struct {
	ID          uuid.UUID              `json:"id"`
	Type        string                 `json:"type"`
	Score       float64                `json:"score"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
	Description string                 `json:"description"`
}

type Visualization struct {
	ID          uuid.UUID              `json:"id"`
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Data        interface{}            `json:"data"`
	Config      map[string]interface{} `json:"config"`
	Description string                 `json:"description"`
}

type Recommendation struct {
	ID          uuid.UUID `json:"id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Action      string    `json:"action"`
	Priority    int       `json:"priority"`
	Impact      string    `json:"impact"`
}

type TimeSeriesConfig struct {
	Frequency       string            `json:"frequency"`
	SeasonalPeriods []int             `json:"seasonal_periods"`
	ForecastHorizon int               `json:"forecast_horizon"`
	ConfidenceLevel float64           `json:"confidence_level"`
	Method          string            `json:"method"`
}

type ForecastPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	Value          float64   `json:"value"`
	UpperBound     float64   `json:"upper_bound"`
	LowerBound     float64   `json:"lower_bound"`
	Confidence     float64   `json:"confidence"`
}

type SeasonalityAnalysis struct {
	HasSeasonality bool                   `json:"has_seasonality"`
	Periods        []int                  `json:"periods"`
	Strength       float64                `json:"strength"`
	Components     map[string]interface{} `json:"components"`
}

type TrendAnalysis struct {
	Direction   string    `json:"direction"`
	Slope       float64   `json:"slope"`
	Strength    float64   `json:"strength"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date"`
}

type ChangePoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Confidence  float64   `json:"confidence"`
	Magnitude   float64   `json:"magnitude"`
	Direction   string    `json:"direction"`
}

type ForecastAccuracy struct {
	MAE  float64 `json:"mae"`
	MAPE float64 `json:"mape"`
	RMSE float64 `json:"rmse"`
	R2   float64 `json:"r2"`
}

type DataAnomaly struct {
	ID          uuid.UUID              `json:"id"`
	Row         int                    `json:"row"`
	Column      string                 `json:"column"`
	Value       interface{}            `json:"value"`
	Score       float64                `json:"score"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
}

type CrossValidationResult struct {
	Folds       int                    `json:"folds"`
	Scores      []float64              `json:"scores"`
	MeanScore   float64                `json:"mean_score"`
	StdScore    float64                `json:"std_score"`
	Method      string                 `json:"method"`
}

// NewAIAnalyticsWorkflow creates a new AI Analytics workflow instance
func NewAIAnalyticsWorkflow(
	instance *WorkflowInstance,
	eventStore EventStore,
	geminiClient GeminiClient,
	dataConnector DataConnector,
	mlEngine MLEngine,
	reportGenerator ReportGenerator,
	alertManager AlertManager,
	scheduler SchedulerService,
) (*AIAnalyticsWorkflow, error) {
	var config AIAnalyticsConfig
	if err := json.Unmarshal(instance.Configuration, &config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal AI analytics configuration")
	}

	return &AIAnalyticsWorkflow{
		instance:        instance,
		config:          &config,
		eventStore:      eventStore,
		geminiClient:    geminiClient,
		dataConnector:   dataConnector,
		mlEngine:        mlEngine,
		reportGenerator: reportGenerator,
		alertManager:    alertManager,
		scheduler:       scheduler,
	}, nil
}

// Execute runs the complete AI Analytics workflow
func (w *AIAnalyticsWorkflow) Execute(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Emit workflow started event
	if err := w.emitEvent(ctx, EventWorkflowStarted, map[string]interface{}{
		"workflow_type": WorkflowTypeAIAnalytics,
		"config":        w.config,
	}); err != nil {
		return errors.Wrap(err, "failed to emit workflow started event")
	}

	// Update workflow status
	w.instance.Status = WorkflowStatusRunning
	now := time.Now()
	w.instance.StartedAt = &now

	// Execute workflow steps
	steps := []AnalyticsStepFunc{
		w.validateConfiguration,
		w.connectToDataSources,
		w.extractAndValidateData,
		w.performDataAnalysis,
		w.trainMLModels,
		w.generateInsights,
		w.createReports,
		w.setupAlerts,
		w.scheduleRecurringAnalysis,
	}

	for i, step := range steps {
		w.instance.CurrentStepIndex = i
		stepName := w.getStepName(i)
		
		if err := w.executeStep(ctx, stepName, step); err != nil {
			w.instance.Status = WorkflowStatusFailed
			w.instance.Error = &WorkflowError{
				Code:        "AI_ANALYTICS_STEP_FAILED",
				Message:     fmt.Sprintf("Step %s failed: %v", stepName, err),
				Timestamp:   time.Now(),
				Recoverable: w.isRecoverableError(err),
			}
			
			w.emitEvent(ctx, EventWorkflowFailed, map[string]interface{}{
				"error": w.instance.Error,
				"step":  stepName,
			})
			
			return err
		}

		// Update progress
		w.instance.Progress.CompletedSteps = i + 1
		w.instance.Progress.PercentComplete = float64(i+1) / float64(len(steps)) * 100
	}

	// Mark as completed
	w.instance.Status = WorkflowStatusCompleted
	completedAt := time.Now()
	w.instance.CompletedAt = &completedAt

	// Emit workflow completed event
	return w.emitEvent(ctx, EventWorkflowCompleted, map[string]interface{}{
		"duration": completedAt.Sub(*w.instance.StartedAt).String(),
		"stats":    w.getAnalyticsStats(),
	})
}

type AnalyticsStepFunc func(ctx context.Context) error

// Analytics workflow step implementations
func (w *AIAnalyticsWorkflow) validateConfiguration(ctx context.Context) error {
	if len(w.config.DataSources) == 0 {
		return errors.New("no data sources configured")
	}
	if w.config.GeminiConfig.APIKey == "" {
		return errors.New("Gemini API key not configured")
	}
	if len(w.config.AnalysisTypes) == 0 {
		return errors.New("no analysis types specified")
	}

	// Validate Gemini configuration
	if w.config.GeminiConfig.Temperature < 0 || w.config.GeminiConfig.Temperature > 1 {
		return errors.New("invalid Gemini temperature setting")
	}

	return nil
}

func (w *AIAnalyticsWorkflow) connectToDataSources(ctx context.Context) error {
	connections := make(map[uuid.UUID]DataConnection)
	
	for _, source := range w.config.DataSources {
		connection, err := w.dataConnector.ConnectToSource(ctx, source)
		if err != nil {
			return errors.Wrapf(err, "failed to connect to data source %s", source.ID)
		}
		
		connections[source.ID] = connection
		
		// Store connection in workflow context
		w.instance.Context[fmt.Sprintf("connection_%s", source.ID.String())] = connection.GetConnectionInfo()
	}

	// Store connections for later use
	w.instance.Metadata["data_connections"] = connections

	return nil
}

func (w *AIAnalyticsWorkflow) extractAndValidateData(ctx context.Context) error {
	dataSets := make(map[uuid.UUID]*DataSet)
	
	for _, source := range w.config.DataSources {
		connectionInfo, exists := w.instance.Context[fmt.Sprintf("connection_%s", source.ID.String())]
		if !exists {
			return errors.Errorf("no connection found for data source %s", source.ID)
		}

		// Extract data based on source configuration
		var query string
		if sourceConfig, ok := source.Config.(map[string]interface{}); ok {
			if q, exists := sourceConfig["query"]; exists {
				query = q.(string)
			}
		}

		// Get connection from metadata (in real implementation, you'd properly cast this)
		connections := w.instance.Metadata["data_connections"].(map[uuid.UUID]DataConnection)
		connection := connections[source.ID]

		dataSet, err := w.dataConnector.ExtractData(ctx, connection, query)
		if err != nil {
			return errors.Wrapf(err, "failed to extract data from source %s", source.ID)
		}

		// Validate data quality
		validationResult, err := w.dataConnector.ValidateData(ctx, dataSet, []ValidationRule{
			{
				ID:    uuid.New(),
				Field: "*",
				Type:  "completeness",
				Parameters: map[string]interface{}{
					"min_completeness": 0.8,
				},
			},
		})
		if err != nil {
			return errors.Wrap(err, "data validation failed")
		}

		if !validationResult.IsValid {
			log.Printf("Data quality issues detected: %+v", validationResult.Errors)
		}

		dataSets[source.ID] = dataSet

		// Update source with last refresh time
		source.LastRefresh = &time.Time{}
		*source.LastRefresh = time.Now()
	}

	// Store datasets for later use
	w.instance.Metadata["datasets"] = dataSets

	return nil
}

func (w *AIAnalyticsWorkflow) performDataAnalysis(ctx context.Context) error {
	dataSets := w.instance.Metadata["datasets"].(map[uuid.UUID]*DataSet)
	analysisResults := make(map[AnalysisType]*AnalysisResult)

	for _, analysisType := range w.config.AnalysisTypes {
		// Combine all datasets for analysis
		combinedData := w.combineDataSets(dataSets)

		// Perform analysis using Gemini
		result, err := w.geminiClient.AnalyzeData(ctx, combinedData, analysisType, w.config.GeminiConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to perform %s analysis", analysisType)
		}

		analysisResults[analysisType] = result

		// Emit analysis completed event
		w.emitEvent(ctx, "analytics.analysis.completed", map[string]interface{}{
			"analysis_type": analysisType,
			"insights":      len(result.Insights),
			"confidence":    result.Confidence,
		})
	}

	// Store analysis results
	w.instance.Metadata["analysis_results"] = analysisResults

	return nil
}

func (w *AIAnalyticsWorkflow) trainMLModels(ctx context.Context) error {
	if len(w.config.MLModels) == 0 {
		return nil // Skip if no ML models configured
	}

	dataSets := w.instance.Metadata["datasets"].(map[uuid.UUID]*DataSet)
	trainedModels := make(map[uuid.UUID]*MLModel)

	for _, modelConfig := range w.config.MLModels {
		// Get training data
		trainingData, exists := dataSets[modelConfig.TrainingData.ID]
		if !exists {
			return errors.Errorf("training data not found for model %s", modelConfig.Name)
		}

		// Train the model
		model, err := w.mlEngine.TrainModel(ctx, modelConfig, trainingData)
		if err != nil {
			return errors.Wrapf(err, "failed to train model %s", modelConfig.Name)
		}

		trainedModels[modelConfig.ID] = model

		// Evaluate model performance
		evaluation, err := w.mlEngine.EvaluateModel(ctx, model, trainingData)
		if err != nil {
			log.Printf("Failed to evaluate model %s: %v", modelConfig.Name, err)
		} else {
			model.Performance = evaluation.Performance
		}

		// Update last trained time
		modelConfig.LastTrained = &model.TrainedAt

		// Emit model trained event
		w.emitEvent(ctx, "analytics.model.trained", map[string]interface{}{
			"model_id":    model.ID,
			"model_name":  model.Name,
			"performance": model.Performance,
		})
	}

	// Store trained models
	w.instance.Metadata["trained_models"] = trainedModels

	return nil
}

func (w *AIAnalyticsWorkflow) generateInsights(ctx context.Context) error {
	dataSets := w.instance.Metadata["datasets"].(map[uuid.UUID]*DataSet)
	analysisResults := w.instance.Metadata["analysis_results"].(map[AnalysisType]*AnalysisResult)

	// Combine all data for insight generation
	combinedData := w.combineDataSets(dataSets)
	contextData := map[string]interface{}{
		"analysis_results": analysisResults,
		"timestamp":        time.Now(),
		"user_id":          w.instance.UserID,
		"tenant_id":        w.instance.TenantID,
	}

	// Generate insights using Gemini
	insightResult, err := w.geminiClient.GenerateInsights(ctx, combinedData, contextData, w.config.GeminiConfig)
	if err != nil {
		return errors.Wrap(err, "failed to generate insights")
	}

	// Detect anomalies
	anomalyResult, err := w.geminiClient.DetectAnomalies(ctx, combinedData, w.config.GeminiConfig)
	if err != nil {
		log.Printf("Failed to detect anomalies: %v", err)
	}

	// Store insights and anomalies
	w.instance.Metadata["insights"] = insightResult
	if anomalyResult != nil {
		w.instance.Metadata["anomalies"] = anomalyResult
	}

	// Emit insights generated event
	w.emitEvent(ctx, "analytics.insights.generated", map[string]interface{}{
		"insights_count":    len(insightResult.Insights),
		"trends_count":      len(insightResult.Trends),
		"patterns_count":    len(insightResult.Patterns),
		"anomalies_count":   len(anomalyResult.Anomalies),
	})

	return nil
}

func (w *AIAnalyticsWorkflow) createReports(ctx context.Context) error {
	if len(w.config.Reports) == 0 {
		return nil // Skip if no reports configured
	}

	// Prepare report data
	reportData := &ReportData{
		DataSets:       w.getDataSetsFromMetadata(),
		Analysis:       w.getAnalysisResultsFromMetadata(),
		Visualizations: w.generateVisualizations(),
		Insights:       w.getInsightsFromMetadata(),
		Metadata:       w.instance.Metadata,
	}

	generatedReports := make([]*GeneratedReport, 0)

	for _, reportConfig := range w.config.Reports {
		// Generate report
		report, err := w.reportGenerator.GenerateReport(ctx, reportConfig, reportData)
		if err != nil {
			return errors.Wrapf(err, "failed to generate report %s", reportConfig.Name)
		}

		generatedReports = append(generatedReports, report)

		// Schedule report if needed
		if reportConfig.Schedule.Enabled {
			scheduledReport, err := w.reportGenerator.ScheduleReport(ctx, reportConfig, reportConfig.Schedule)
			if err != nil {
				log.Printf("Failed to schedule report %s: %v", reportConfig.Name, err)
			} else {
				w.instance.Metadata[fmt.Sprintf("scheduled_report_%s", reportConfig.ID)] = scheduledReport
			}
		}

		// Emit report generated event
		w.emitEvent(ctx, "analytics.report.generated", map[string]interface{}{
			"report_id":   report.ID,
			"report_name": reportConfig.Name,
			"format":      report.Format,
			"recipients":  len(report.Recipients),
		})
	}

	// Store generated reports
	w.instance.Metadata["generated_reports"] = generatedReports

	return nil
}

func (w *AIAnalyticsWorkflow) setupAlerts(ctx context.Context) error {
	if len(w.config.Alerts) == 0 {
		return nil // Skip if no alerts configured
	}

	dataSets := w.instance.Metadata["datasets"].(map[uuid.UUID]*DataSet)
	createdAlerts := make([]*Alert, 0)

	for _, alertConfig := range w.config.Alerts {
		// Create alert
		alert, err := w.alertManager.CreateAlert(ctx, alertConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to create alert %s", alertConfig.Name)
		}

		createdAlerts = append(createdAlerts, alert)

		// Evaluate alert against current data
		combinedData := w.combineDataSets(dataSets)
		triggers, err := w.alertManager.EvaluateAlerts(ctx, combinedData)
		if err != nil {
			log.Printf("Failed to evaluate alert %s: %v", alertConfig.Name, err)
		} else {
			// Send alerts if triggered
			for _, trigger := range triggers {
				if trigger.AlertID == alert.ID {
					if err := w.alertManager.SendAlert(ctx, alert, trigger); err != nil {
						log.Printf("Failed to send alert %s: %v", alert.Name, err)
					}
				}
			}
		}

		// Emit alert created event
		w.emitEvent(ctx, "analytics.alert.created", map[string]interface{}{
			"alert_id":   alert.ID,
			"alert_name": alertConfig.Name,
			"condition":  alertConfig.Condition,
		})
	}

	// Store created alerts
	w.instance.Metadata["created_alerts"] = createdAlerts

	return nil
}

func (w *AIAnalyticsWorkflow) scheduleRecurringAnalysis(ctx context.Context) error {
	if !w.config.Schedule.Enabled {
		return nil // Skip if scheduling is disabled
	}

	// Schedule recurring analysis
	task, err := w.scheduler.ScheduleAnalysis(ctx, w.config.Schedule, w.config)
	if err != nil {
		return errors.Wrap(err, "failed to schedule recurring analysis")
	}

	// Store scheduled task
	w.instance.Metadata["scheduled_analysis"] = task

	// Emit scheduling event
	w.emitEvent(ctx, "analytics.analysis.scheduled", map[string]interface{}{
		"task_id":    task.ID,
		"cron_expr":  w.config.Schedule.CronExpr,
		"next_run":   task.NextRun,
	})

	return nil
}

// Helper methods
func (w *AIAnalyticsWorkflow) executeStep(ctx context.Context, stepName string, stepFunc AnalyticsStepFunc) error {
	// Create step record
	step := &WorkflowStep{
		ID:           uuid.New(),
		WorkflowID:   w.instance.ID,
		Name:         stepName,
		Status:       WorkflowStatusRunning,
		StartedAt:    &time.Time{},
		MaxRetries:   3,
	}
	*step.StartedAt = time.Now()

	// Emit step started event
	w.emitEvent(ctx, EventStepStarted, map[string]interface{}{
		"step": step,
	})

	// Execute step with retry logic
	var err error
	for attempt := 0; attempt <= step.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(attempt*attempt) * time.Second
			time.Sleep(delay)
		}

		err = stepFunc(ctx)
		if err == nil {
			break
		}

		step.RetryCount = attempt
		if attempt < step.MaxRetries {
			w.emitEvent(ctx, EventStepRetried, map[string]interface{}{
				"step":    step,
				"attempt": attempt + 1,
				"error":   err.Error(),
			})
		}
	}

	// Update step status
	completedAt := time.Now()
	step.CompletedAt = &completedAt
	duration := completedAt.Sub(*step.StartedAt)
	step.Duration = &duration

	if err != nil {
		step.Status = WorkflowStatusFailed
		step.Error = &WorkflowError{
			Code:        "AI_ANALYTICS_STEP_ERROR",
			Message:     err.Error(),
			Timestamp:   time.Now(),
			Recoverable: w.isRecoverableError(err),
		}

		w.emitEvent(ctx, EventStepFailed, map[string]interface{}{
			"step":  step,
			"error": step.Error,
		})
	} else {
		step.Status = WorkflowStatusCompleted
		w.emitEvent(ctx, EventStepCompleted, map[string]interface{}{
			"step":     step,
			"duration": duration.String(),
		})
	}

	// Add step to workflow instance
	w.instance.Steps = append(w.instance.Steps, *step)

	return err
}

func (w *AIAnalyticsWorkflow) emitEvent(ctx context.Context, eventType string, data map[string]interface{}) error {
	eventData, _ := json.Marshal(data)
	
	event := &WorkflowEvent{
		ID:         uuid.New(),
		WorkflowID: w.instance.ID,
		Type:       eventType,
		Version:    1,
		Data:       eventData,
		Metadata: map[string]interface{}{
			"workflow_type": string(w.instance.Type),
			"tenant_id":     w.instance.TenantID.String(),
			"user_id":       w.instance.UserID.String(),
		},
		Timestamp: time.Now(),
		UserID:    w.instance.UserID,
		TenantID:  w.instance.TenantID,
	}

	return w.eventStore.SaveEvent(ctx, event)
}

func (w *AIAnalyticsWorkflow) getStepName(index int) string {
	stepNames := []string{
		"validate_configuration",
		"connect_to_data_sources",
		"extract_and_validate_data",
		"perform_data_analysis",
		"train_ml_models",
		"generate_insights",
		"create_reports",
		"setup_alerts",
		"schedule_recurring_analysis",
	}
	
	if index < len(stepNames) {
		return stepNames[index]
	}
	return fmt.Sprintf("step_%d", index)
}

func (w *AIAnalyticsWorkflow) isRecoverableError(err error) bool {
	errorStr := err.Error()
	recoverableErrors := []string{
		"timeout",
		"rate limit",
		"temporary",
		"network",
		"connection",
		"quota",
	}

	for _, recoverable := range recoverableErrors {
		if contains(errorStr, recoverable) {
			return true
		}
	}
	return false
}

func (w *AIAnalyticsWorkflow) combineDataSets(dataSets map[uuid.UUID]*DataSet) *DataSet {
	if len(dataSets) == 0 {
		return nil
	}

	// Simple combination - in practice, this would be more sophisticated
	combined := &DataSet{
		ID:          uuid.New(),
		Rows:        make([]map[string]interface{}, 0),
		Metadata:    make(map[string]interface{}),
		CreatedAt:   time.Now(),
	}

	totalRows := 0
	for _, dataSet := range dataSets {
		combined.Rows = append(combined.Rows, dataSet.Rows...)
		totalRows += dataSet.RowCount
	}

	combined.RowCount = totalRows
	return combined
}

func (w *AIAnalyticsWorkflow) getDataSetsFromMetadata() []*DataSet {
	if dataSets, exists := w.instance.Metadata["datasets"]; exists {
		dataSetMap := dataSets.(map[uuid.UUID]*DataSet)
		result := make([]*DataSet, 0, len(dataSetMap))
		for _, dataSet := range dataSetMap {
			result = append(result, dataSet)
		}
		return result
	}
	return nil
}

func (w *AIAnalyticsWorkflow) getAnalysisResultsFromMetadata() []*AnalysisResult {
	if analysisResults, exists := w.instance.Metadata["analysis_results"]; exists {
		resultMap := analysisResults.(map[AnalysisType]*AnalysisResult)
		result := make([]*AnalysisResult, 0, len(resultMap))
		for _, analysisResult := range resultMap {
			result = append(result, analysisResult)
		}
		return result
	}
	return nil
}

func (w *AIAnalyticsWorkflow) getInsightsFromMetadata() []*Insight {
	if insights, exists := w.instance.Metadata["insights"]; exists {
		insightResult := insights.(*InsightResult)
		return insightResult.Insights
	}
	return nil
}

func (w *AIAnalyticsWorkflow) generateVisualizations() []*Visualization {
	// Generate basic visualizations based on analysis results
	visualizations := make([]*Visualization, 0)
	
	// This would be implemented based on the actual data and analysis results
	// For now, return empty slice
	
	return visualizations
}

func (w *AIAnalyticsWorkflow) getAnalyticsStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_steps":       len(w.instance.Steps),
		"completed_steps":   w.instance.Progress.CompletedSteps,
		"failed_steps":      w.instance.Progress.FailedSteps,
		"success_rate":      float64(w.instance.Progress.CompletedSteps) / float64(len(w.instance.Steps)),
	}

	// Add specific analytics stats
	if dataSets := w.getDataSetsFromMetadata(); dataSets != nil {
		totalRows := 0
		for _, dataSet := range dataSets {
			totalRows += dataSet.RowCount
		}
		stats["total_records_processed"] = totalRows
		stats["data_sources_count"] = len(w.config.DataSources)
	}

	if analysisResults := w.getAnalysisResultsFromMetadata(); analysisResults != nil {
		stats["analysis_types_count"] = len(analysisResults)
		
		totalInsights := 0
		avgConfidence := 0.0
		for _, result := range analysisResults {
			totalInsights += len(result.Insights)
			avgConfidence += result.Confidence
		}
		
		stats["total_insights"] = totalInsights
		if len(analysisResults) > 0 {
			stats["average_confidence"] = avgConfidence / float64(len(analysisResults))
		}
	}

	if trainedModels, exists := w.instance.Metadata["trained_models"]; exists {
		modelMap := trainedModels.(map[uuid.UUID]*MLModel)
		stats["models_trained"] = len(modelMap)
		
		avgAccuracy := 0.0
		modelCount := 0
		for _, model := range modelMap {
			if model.Performance != nil {
				avgAccuracy += model.Performance.Accuracy
				modelCount++
			}
		}
		
		if modelCount > 0 {
			stats["average_model_accuracy"] = avgAccuracy / float64(modelCount)
		}
	}

	if generatedReports, exists := w.instance.Metadata["generated_reports"]; exists {
		reports := generatedReports.([]*GeneratedReport)
		stats["reports_generated"] = len(reports)
	}

	if createdAlerts, exists := w.instance.Metadata["created_alerts"]; exists {
		alerts := createdAlerts.([]*Alert)
		stats["alerts_created"] = len(alerts)
	}

	return stats
}