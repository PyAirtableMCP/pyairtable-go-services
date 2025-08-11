// Package services contains domain services for complex business operations
package services

import (
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/tenant"
	"github.com/pyairtable-compose/go-services/domain/user"
	"github.com/pyairtable-compose/go-services/domain/workspace"
)

// TenantManagementService handles complex tenant operations including
// subscription management, quota enforcement, and tenant lifecycle
type TenantManagementService struct {
	*shared.BaseDomainService
	tenantRepo    TenantRepository
	userRepo      UserRepository
	workspaceRepo WorkspaceRepository
	eventBus      shared.EventBus
}

// TenantUsageReport represents current tenant usage across all resources
type TenantUsageReport struct {
	TenantID            shared.TenantID
	CurrentUsers        int
	CurrentWorkspaces   int
	CurrentProjects     int
	CurrentStorageGB    float64
	CurrentAPICallsThisMonth int
	CurrentIntegrations int
	QuotaViolations     []string
	GeneratedAt         shared.Timestamp
}

// SubscriptionChangeRequest represents a request to change subscription
type SubscriptionChangeRequest struct {
	TenantID         shared.TenantID
	NewPlan          tenant.SubscriptionPlan
	PaymentMethodID  string
	ProrationPolicy  ProrationPolicy
	EffectiveDate    *shared.Timestamp
}

// ProrationPolicy determines how billing changes are handled
type ProrationPolicy string

const (
	ProrationPolicyImmediate ProrationPolicy = "immediate"
	ProrationPolicyNextCycle ProrationPolicy = "next_cycle"
	ProrationPolicyNone      ProrationPolicy = "none"
)

// TenantSuspensionRequest represents a request to suspend a tenant
type TenantSuspensionRequest struct {
	TenantID     shared.TenantID
	Reason       string
	SuspendedBy  shared.UserID
	AutoReactivate *shared.Timestamp
}

// NewTenantManagementService creates a new tenant management service
func NewTenantManagementService(
	tenantRepo TenantRepository,
	userRepo UserRepository,
	workspaceRepo WorkspaceRepository,
	eventBus shared.EventBus,
) *TenantManagementService {
	return &TenantManagementService{
		BaseDomainService: shared.NewBaseDomainService("TenantManagementService"),
		tenantRepo:        tenantRepo,
		userRepo:          userRepo,
		workspaceRepo:     workspaceRepo,
		eventBus:          eventBus,
	}
}

// CollectTenantUsage collects current usage statistics for a tenant
func (s *TenantManagementService) CollectTenantUsage(tenantID shared.TenantID) (*TenantUsageReport, error) {
	// Retrieve tenant
	tenantAgg, err := s.tenantRepo.GetByID(tenantID)
	if err != nil {
		return nil, shared.NewDomainError(shared.ErrCodeNotFound, "tenant not found", err)
	}

	// Collect usage from various sources
	userCount, err := s.countTenantUsers(tenantID)
	if err != nil {
		return nil, err
	}

	workspaces, err := s.workspaceRepo.GetByTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	workspaceCount := len(workspaces)

	projectCount := 0
	for _, workspace := range workspaces {
		projectCount += len(workspace.ListProjects())
	}

	// In real implementation, these would query actual storage and API usage
	storageGB := s.calculateStorageUsage(tenantID)
	apiCalls := s.calculateAPIUsage(tenantID)
	integrations := s.countIntegrations(tenantID)

	usage := tenant.TenantUsage{
		CurrentUsers:             userCount,
		CurrentWorkspaces:        workspaceCount,
		CurrentProjects:          projectCount,
		CurrentStorageGB:         storageGB,
		CurrentAPICallsThisMonth: apiCalls,
		CurrentIntegrations:      integrations,
		LastUpdated:              shared.NewTimestamp(),
	}

	// Update tenant usage
	if err := tenantAgg.UpdateUsage(usage); err != nil {
		return nil, err
	}

	// Check for quota violations
	violations := tenantAgg.CheckQuotas()

	// Save updated tenant
	if err := s.tenantRepo.Save(tenantAgg); err != nil {
		return nil, shared.NewDomainError(shared.ErrCodeInternal, "failed to update tenant usage", err)
	}

	return &TenantUsageReport{
		TenantID:                 tenantID,
		CurrentUsers:             userCount,
		CurrentWorkspaces:        workspaceCount,
		CurrentProjects:          projectCount,
		CurrentStorageGB:         storageGB,
		CurrentAPICallsThisMonth: apiCalls,
		CurrentIntegrations:      integrations,
		QuotaViolations:          violations,
		GeneratedAt:              shared.NewTimestamp(),
	}, nil
}

// ChangeSubscription changes a tenant's subscription plan
func (s *TenantManagementService) ChangeSubscription(request SubscriptionChangeRequest) error {
	// Retrieve tenant
	tenantAgg, err := s.tenantRepo.GetByID(request.TenantID)
	if err != nil {
		return shared.NewDomainError(shared.ErrCodeNotFound, "tenant not found", err)
	}

	// Validate subscription change
	if err := s.validateSubscriptionChange(tenantAgg, request); err != nil {
		return err
	}

	// Check if tenant usage fits within new plan limits
	newQuotas := request.NewPlan.GetPlanLimits()
	currentUsage := tenantAgg.GetUsage()
	
	if currentUsage.CurrentUsers > newQuotas.MaxUsers {
		return shared.NewDomainError(
			shared.ErrCodeBusinessRule,
			"current user count exceeds new plan limit",
			nil,
		).WithContext("current_users", currentUsage.CurrentUsers).
		  WithContext("max_users", newQuotas.MaxUsers)
	}

	if currentUsage.CurrentWorkspaces > newQuotas.MaxWorkspaces {
		return shared.NewDomainError(
			shared.ErrCodeBusinessRule,
			"current workspace count exceeds new plan limit",
			nil,
		)
	}

	// Process subscription change
	if err := tenantAgg.ChangePlan(request.NewPlan, request.PaymentMethodID); err != nil {
		return err
	}

	// Handle proration if needed
	if err := s.handleProration(tenantAgg, request); err != nil {
		return err
	}

	// Save tenant
	if err := s.tenantRepo.Save(tenantAgg); err != nil {
		return shared.NewDomainError(shared.ErrCodeInternal, "failed to save tenant", err)
	}

	// Publish events
	for _, event := range tenantAgg.GetUncommittedEvents() {
		if err := s.eventBus.Publish(event); err != nil {
			// Log error but don't fail the operation
		}
	}
	tenantAgg.ClearUncommittedEvents()

	return nil
}

// SuspendTenant suspends a tenant for policy violations or payment issues
func (s *TenantManagementService) SuspendTenant(request TenantSuspensionRequest) error {
	// Retrieve tenant
	tenantAgg, err := s.tenantRepo.GetByID(request.TenantID)
	if err != nil {
		return shared.NewDomainError(shared.ErrCodeNotFound, "tenant not found", err)
	}

	// Suspend tenant
	if err := tenantAgg.Suspend(request.Reason); err != nil {
		return err
	}

	// Deactivate all tenant users (business rule)
	if err := s.deactivateTenantUsers(request.TenantID, request.Reason); err != nil {
		return err
	}

	// Save tenant
	if err := s.tenantRepo.Save(tenantAgg); err != nil {
		return shared.NewDomainError(shared.ErrCodeInternal, "failed to save tenant", err)
	}

	// Schedule auto-reactivation if specified
	if request.AutoReactivate != nil {
		if err := s.scheduleAutoReactivation(request.TenantID, *request.AutoReactivate); err != nil {
			// Log error but don't fail suspension
		}
	}

	// Publish events
	for _, event := range tenantAgg.GetUncommittedEvents() {
		if err := s.eventBus.Publish(event); err != nil {
			// Log error but don't fail the operation
		}
	}
	tenantAgg.ClearUncommittedEvents()

	return nil
}

// ReactivateTenant reactivates a suspended tenant
func (s *TenantManagementService) ReactivateTenant(tenantID shared.TenantID) error {
	// Retrieve tenant
	tenantAgg, err := s.tenantRepo.GetByID(tenantID)
	if err != nil {
		return shared.NewDomainError(shared.ErrCodeNotFound, "tenant not found", err)
	}

	// Check if reactivation is allowed
	if err := s.validateReactivation(tenantAgg); err != nil {
		return err
	}

	// Reactivate tenant
	if err := tenantAgg.Reactivate(); err != nil {
		return err
	}

	// Reactivate eligible tenant users
	if err := s.reactivateTenantUsers(tenantID); err != nil {
		return err
	}

	// Save tenant
	if err := s.tenantRepo.Save(tenantAgg); err != nil {
		return shared.NewDomainError(shared.ErrCodeInternal, "failed to save tenant", err)
	}

	return nil
}

// EnforceQuotas checks and enforces tenant quotas
func (s *TenantManagementService) EnforceQuotas(tenantID shared.TenantID) error {
	// Collect current usage
	usageReport, err := s.CollectTenantUsage(tenantID)
	if err != nil {
		return err
	}

	// If there are violations, take action
	if len(usageReport.QuotaViolations) > 0 {
		return s.handleQuotaViolations(tenantID, usageReport.QuotaViolations)
	}

	return nil
}

// HandleTrialExpiration handles when a tenant's trial period expires
func (s *TenantManagementService) HandleTrialExpiration(tenantID shared.TenantID) error {
	// Retrieve tenant
	tenantAgg, err := s.tenantRepo.GetByID(tenantID)
	if err != nil {
		return shared.NewDomainError(shared.ErrCodeNotFound, "tenant not found", err)
	}

	// Check if trial is actually expired
	if !tenantAgg.IsTrialExpired() {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "trial has not expired", nil)
	}

	// Check if tenant has payment method for automatic conversion
	billing := tenantAgg.GetBilling()
	hasPaymentMethod := len(billing.PaymentMethods) > 0

	if hasPaymentMethod {
		// Convert to paid plan automatically
		subscription := tenantAgg.GetSubscription()
		return s.ChangeSubscription(SubscriptionChangeRequest{
			TenantID:        tenantID,
			NewPlan:         subscription.Plan, // Keep current plan
			PaymentMethodID: billing.PaymentMethods[0].ID,
			ProrationPolicy: ProrationPolicyImmediate,
		})
	} else {
		// Suspend tenant due to expired trial
		return s.SuspendTenant(TenantSuspensionRequest{
			TenantID: tenantID,
			Reason:   "Trial expired without payment method",
		})
	}
}

// Helper methods

func (s *TenantManagementService) countTenantUsers(tenantID shared.TenantID) (int, error) {
	// In real implementation, this would query the user repository
	// For now, return a placeholder
	return 1, nil
}

func (s *TenantManagementService) calculateStorageUsage(tenantID shared.TenantID) float64 {
	// In real implementation, this would calculate actual storage usage
	return 0.5
}

func (s *TenantManagementService) calculateAPIUsage(tenantID shared.TenantID) int {
	// In real implementation, this would calculate API call usage for current month
	return 100
}

func (s *TenantManagementService) countIntegrations(tenantID shared.TenantID) int {
	// In real implementation, this would count active integrations
	return 1
}

func (s *TenantManagementService) validateSubscriptionChange(
	tenantAgg *tenant.TenantAggregate,
	request SubscriptionChangeRequest,
) error {
	// Validate tenant is active
	if tenantAgg.GetStatus() != tenant.TenantStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot change subscription for inactive tenant", nil)
	}

	// Validate payment method for paid plans
	if request.NewPlan != tenant.PlanFree && request.PaymentMethodID == "" {
		return shared.NewDomainError(shared.ErrCodeValidation, "payment method required for paid plans", nil)
	}

	return nil
}

func (s *TenantManagementService) handleProration(
	tenantAgg *tenant.TenantAggregate,
	request SubscriptionChangeRequest,
) error {
	// In real implementation, this would handle billing proration
	// based on the proration policy
	return nil
}

func (s *TenantManagementService) deactivateTenantUsers(tenantID shared.TenantID, reason string) error {
	// In real implementation, this would query and deactivate all tenant users
	return nil
}

func (s *TenantManagementService) reactivateTenantUsers(tenantID shared.TenantID) error {
	// In real implementation, this would reactivate eligible tenant users
	return nil
}

func (s *TenantManagementService) validateReactivation(tenantAgg *tenant.TenantAggregate) error {
	// Check if subscription is valid
	subscription := tenantAgg.GetSubscription()
	if subscription.Status == tenant.SubscriptionStatusCanceled {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot reactivate tenant with canceled subscription", nil)
	}

	// Check payment method for paid plans
	if subscription.Plan != tenant.PlanFree {
		billing := tenantAgg.GetBilling()
		if len(billing.PaymentMethods) == 0 {
			return shared.NewDomainError(shared.ErrCodeBusinessRule, "valid payment method required for reactivation", nil)
		}
	}

	return nil
}

func (s *TenantManagementService) handleQuotaViolations(tenantID shared.TenantID, violations []string) error {
	// In real implementation, this would:
	// 1. Send notifications to tenant admins
	// 2. Apply usage restrictions
	// 3. Potentially suspend tenant if violations are severe
	
	// For now, just return an error indicating violations exist
	return shared.NewDomainError(
		shared.ErrCodeBusinessRule,
		"tenant has quota violations",
		nil,
	).WithContext("violations", violations)
}

func (s *TenantManagementService) scheduleAutoReactivation(tenantID shared.TenantID, reactivateAt shared.Timestamp) error {
	// In real implementation, this would schedule a job to reactivate the tenant
	// at the specified time (e.g., using a job queue or scheduler)
	return nil
}