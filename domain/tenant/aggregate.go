// Package tenant contains the tenant management domain model with subscription support
package tenant

import (
	"strings"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
)

// TenantAggregate represents the tenant aggregate root
type TenantAggregate struct {
	*shared.BaseAggregateRoot
	name         string
	slug         string
	domain       string
	status       TenantStatus
	subscription *Subscription
	settings     TenantSettings
	quotas       TenantQuotas
	usage        TenantUsage
	billing      BillingInfo
	createdAt    shared.Timestamp
	updatedAt    shared.Timestamp
}

// TenantStatus represents the status of a tenant
type TenantStatus string

const (
	TenantStatusActive     TenantStatus = "active"
	TenantStatusSuspended  TenantStatus = "suspended"
	TenantStatusDelinquent TenantStatus = "delinquent"
	TenantStatusCanceled   TenantStatus = "canceled"
	TenantStatusDeleted    TenantStatus = "deleted"
)

// IsValid checks if the tenant status is valid
func (s TenantStatus) IsValid() bool {
	switch s {
	case TenantStatusActive, TenantStatusSuspended, TenantStatusDelinquent, TenantStatusCanceled, TenantStatusDeleted:
		return true
	default:
		return false
	}
}

// Subscription represents a tenant's subscription
type Subscription struct {
	ID              shared.ID
	PlanID          string
	Plan            SubscriptionPlan
	Status          SubscriptionStatus
	CurrentPeriod   BillingPeriod
	TrialEndsAt     *shared.Timestamp
	CanceledAt      *shared.Timestamp
	CancellationReason string
	PaymentMethod   PaymentMethod
	Addons          []Addon
	CreatedAt       shared.Timestamp
	UpdatedAt       shared.Timestamp
}

// SubscriptionPlan represents available subscription plans
type SubscriptionPlan string

const (
	PlanFree        SubscriptionPlan = "free"
	PlanStarter     SubscriptionPlan = "starter"
	PlanProfessional SubscriptionPlan = "professional"
	PlanEnterprise  SubscriptionPlan = "enterprise"
	PlanCustom      SubscriptionPlan = "custom"
)

// GetPlanLimits returns the limits for a subscription plan
func (p SubscriptionPlan) GetPlanLimits() TenantQuotas {
	switch p {
	case PlanFree:
		return TenantQuotas{
			MaxUsers:      5,
			MaxWorkspaces: 3,
			MaxProjects:   10,
			MaxStorageGB:  1,
			MaxAPICallsPerMonth: 1000,
			MaxIntegrations: 2,
		}
	case PlanStarter:
		return TenantQuotas{
			MaxUsers:      25,
			MaxWorkspaces: 10,
			MaxProjects:   50,
			MaxStorageGB:  10,
			MaxAPICallsPerMonth: 10000,
			MaxIntegrations: 5,
		}
	case PlanProfessional:
		return TenantQuotas{
			MaxUsers:      100,
			MaxWorkspaces: 50,
			MaxProjects:   500,
			MaxStorageGB:  100,
			MaxAPICallsPerMonth: 100000,
			MaxIntegrations: 20,
		}
	case PlanEnterprise:
		return TenantQuotas{
			MaxUsers:      1000,
			MaxWorkspaces: 500,
			MaxProjects:   5000,
			MaxStorageGB:  1000,
			MaxAPICallsPerMonth: 1000000,
			MaxIntegrations: 100,
		}
	default:
		return TenantQuotas{} // Custom plans have configurable limits
	}
}

// GetMonthlyPrice returns the monthly price for a plan in cents
func (p SubscriptionPlan) GetMonthlyPrice() int {
	switch p {
	case PlanFree:
		return 0
	case PlanStarter:
		return 1900 // $19.00
	case PlanProfessional:
		return 4900 // $49.00
	case PlanEnterprise:
		return 9900 // $99.00
	default:
		return 0 // Custom pricing
	}
}

// SubscriptionStatus represents the status of a subscription
type SubscriptionStatus string

const (
	SubscriptionStatusTrial     SubscriptionStatus = "trial"
	SubscriptionStatusActive    SubscriptionStatus = "active"
	SubscriptionStatusPastDue   SubscriptionStatus = "past_due"
	SubscriptionStatusCanceled  SubscriptionStatus = "canceled"
	SubscriptionStatusExpired   SubscriptionStatus = "expired"
)

// BillingPeriod represents a billing period
type BillingPeriod struct {
	StartDate shared.Timestamp
	EndDate   shared.Timestamp
	Amount    int // Amount in cents
	Currency  string
}

// IsActive checks if the billing period is currently active
func (bp *BillingPeriod) IsActive() bool {
	now := shared.NewTimestamp()
	return !now.Before(bp.StartDate) && now.Before(bp.EndDate)
}

// PaymentMethod represents payment method information
type PaymentMethod struct {
	ID          string
	Type        PaymentMethodType
	LastFour    string
	ExpiryMonth int
	ExpiryYear  int
	Brand       string
	IsDefault   bool
	UpdatedAt   shared.Timestamp
}

// PaymentMethodType represents types of payment methods
type PaymentMethodType string

const (
	PaymentMethodCard       PaymentMethodType = "card"
	PaymentMethodBankAccount PaymentMethodType = "bank_account"
	PaymentMethodPayPal     PaymentMethodType = "paypal"
)

// Addon represents additional features or capacity
type Addon struct {
	ID          string
	Name        string
	Description string
	Type        AddonType
	Price       int // Price in cents
	Quantity    int
	AddedAt     shared.Timestamp
}

// AddonType represents types of addons
type AddonType string

const (
	AddonTypeStorage      AddonType = "storage"
	AddonTypeUsers        AddonType = "users"
	AddonTypeAPIRequests  AddonType = "api_requests"
	AddonTypeIntegrations AddonType = "integrations"
	AddonTypeSupport      AddonType = "support"
)

// TenantSettings represents tenant configuration
type TenantSettings struct {
	Branding         BrandingSettings
	Security         SecuritySettings
	Features         FeatureSettings
	Notifications    NotificationSettings
	UpdatedAt        shared.Timestamp
}

// BrandingSettings represents branding configuration
type BrandingSettings struct {
	LogoURL      string
	PrimaryColor string
	SecondaryColor string
	CustomCSS    string
	FaviconURL   string
}

// SecuritySettings represents security configuration
type SecuritySettings struct {
	RequireSSO          bool
	RequireMFA          bool
	PasswordPolicy      PasswordPolicy
	SessionTimeout      time.Duration
	IPWhitelist         []string
	AllowedDomains      []string
}

// PasswordPolicy represents password requirements
type PasswordPolicy struct {
	MinLength        int
	RequireUppercase bool
	RequireLowercase bool
	RequireNumbers   bool
	RequireSpecial   bool
	MaxAge           time.Duration
}

// FeatureSettings represents feature toggles
type FeatureSettings struct {
	EnabledFeatures  []string
	DisabledFeatures []string
	BetaFeatures     []string
}

// NotificationSettings represents notification preferences
type NotificationSettings struct {
	EmailNotifications    bool
	SlackIntegration      bool
	WebhookURL           string
	NotificationChannels []string
}

// TenantQuotas represents resource limits
type TenantQuotas struct {
	MaxUsers            int
	MaxWorkspaces       int
	MaxProjects         int
	MaxStorageGB        int
	MaxAPICallsPerMonth int
	MaxIntegrations     int
	CustomQuotas        map[string]int
}

// TenantUsage represents current resource usage
type TenantUsage struct {
	CurrentUsers           int
	CurrentWorkspaces      int
	CurrentProjects        int
	CurrentStorageGB       float64
	CurrentAPICallsThisMonth int
	CurrentIntegrations    int
	LastUpdated            shared.Timestamp
}

// BillingInfo represents billing and payment information
type BillingInfo struct {
	BillingEmail      shared.Email
	BillingAddress    BillingAddress
	TaxID             string
	Currency          string
	PaymentMethods    []PaymentMethod
	BillingHistory    []Invoice
	UpdatedAt         shared.Timestamp
}

// BillingAddress represents a billing address
type BillingAddress struct {
	Line1      string
	Line2      string
	City       string
	State      string
	PostalCode string
	Country    string
}

// Invoice represents a billing invoice
type Invoice struct {
	ID          string
	Number      string
	Amount      int
	Currency    string
	Status      InvoiceStatus
	DueDate     shared.Timestamp
	PaidAt      *shared.Timestamp
	CreatedAt   shared.Timestamp
}

// InvoiceStatus represents the status of an invoice
type InvoiceStatus string

const (
	InvoiceStatusDraft     InvoiceStatus = "draft"
	InvoiceStatusOpen      InvoiceStatus = "open"
	InvoiceStatusPaid      InvoiceStatus = "paid"
	InvoiceStatusVoid      InvoiceStatus = "void"
	InvoiceStatusUncollectible InvoiceStatus = "uncollectible"
)

// NewTenantAggregate creates a new tenant aggregate
func NewTenantAggregate(
	id shared.TenantID,
	name string,
	slug string,
	domain string,
	ownerEmail shared.Email,
) (*TenantAggregate, error) {
	
	// Validate inputs
	if name == "" {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "tenant name is required", shared.ErrEmptyValue)
	}
	
	if slug == "" {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "tenant slug is required", shared.ErrEmptyValue)
	}
	
	if !isValidSlug(slug) {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "invalid tenant slug format", nil)
	}
	
	if ownerEmail.IsEmpty() {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "owner email is required", shared.ErrEmptyValue)
	}

	now := shared.NewTimestamp()
	
	// Start with free plan trial
	freePlan := PlanFree
	trialEnd := now.AddDuration(30 * 24 * time.Hour) // 30-day trial
	
	subscription := &Subscription{
		ID:     shared.NewID(),
		PlanID: string(freePlan),
		Plan:   freePlan,
		Status: SubscriptionStatusTrial,
		CurrentPeriod: BillingPeriod{
			StartDate: now,
			EndDate:   trialEnd,
			Amount:    0,
			Currency:  "USD",
		},
		TrialEndsAt: &trialEnd,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	aggregate := &TenantAggregate{
		BaseAggregateRoot: shared.NewBaseAggregateRoot(id.ID, id),
		name:              strings.TrimSpace(name),
		slug:              strings.ToLower(strings.TrimSpace(slug)),
		domain:            strings.ToLower(strings.TrimSpace(domain)),
		status:            TenantStatusActive,
		subscription:      subscription,
		settings: TenantSettings{
			Security: SecuritySettings{
				RequireSSO:     false,
				RequireMFA:     false,
				SessionTimeout: 24 * time.Hour,
				PasswordPolicy: PasswordPolicy{
					MinLength:        8,
					RequireUppercase: true,
					RequireLowercase: true,
					RequireNumbers:   true,
					RequireSpecial:   false,
					MaxAge:           90 * 24 * time.Hour,
				},
			},
			Features: FeatureSettings{
				EnabledFeatures: []string{"basic_features"},
			},
			Notifications: NotificationSettings{
				EmailNotifications: true,
			},
			UpdatedAt: now,
		},
		quotas:  freePlan.GetPlanLimits(),
		usage:   TenantUsage{LastUpdated: now},
		billing: BillingInfo{
			BillingEmail: ownerEmail,
			Currency:     "USD",
			UpdatedAt:    now,
		},
		createdAt: now,
		updatedAt: now,
	}

	// Raise tenant created event
	aggregate.RaiseEvent(
		shared.EventTypeTenantCreated,
		"TenantAggregate",
		map[string]interface{}{
			"tenant_id":    id.String(),
			"name":         name,
			"slug":         slug,
			"domain":       domain,
			"owner_email":  ownerEmail.String(),
			"plan":         string(freePlan),
		},
	)

	return aggregate, nil
}

// GetName returns the tenant name
func (t *TenantAggregate) GetName() string {
	return t.name
}

// GetSlug returns the tenant slug
func (t *TenantAggregate) GetSlug() string {
	return t.slug
}

// GetDomain returns the tenant domain
func (t *TenantAggregate) GetDomain() string {
	return t.domain
}

// GetStatus returns the tenant status
func (t *TenantAggregate) GetStatus() TenantStatus {
	return t.status
}

// GetSubscription returns the current subscription
func (t *TenantAggregate) GetSubscription() *Subscription {
	return t.subscription
}

// GetQuotas returns the current quotas
func (t *TenantAggregate) GetQuotas() TenantQuotas {
	return t.quotas
}

// GetUsage returns the current usage
func (t *TenantAggregate) GetUsage() TenantUsage {
	return t.usage
}

// UpdateTenant updates tenant information
func (t *TenantAggregate) UpdateTenant(name, domain string) error {
	if t.status != TenantStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot update inactive tenant", nil)
	}

	hasChanges := false

	if name != "" && name != t.name {
		t.name = strings.TrimSpace(name)
		hasChanges = true
	}

	if domain != t.domain {
		t.domain = strings.ToLower(strings.TrimSpace(domain))
		hasChanges = true
	}

	if hasChanges {
		t.updatedAt = shared.NewTimestamp()

		t.RaiseEvent(
			shared.EventTypeTenantUpdated,
			"TenantAggregate",
			map[string]interface{}{
				"tenant_id": t.GetID().String(),
				"name":      t.name,
				"domain":    t.domain,
			},
		)
	}

	return nil
}

// ChangePlan changes the subscription plan
func (t *TenantAggregate) ChangePlan(newPlan SubscriptionPlan, paymentMethodID string) error {
	if t.status != TenantStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot change plan for inactive tenant", nil)
	}

	if t.subscription.Plan == newPlan {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "tenant already on this plan", nil)
	}

	// Validate plan change is allowed
	if newPlan != PlanFree && paymentMethodID == "" {
		return shared.NewDomainError(shared.ErrCodeValidation, "payment method required for paid plans", nil)
	}

	oldPlan := t.subscription.Plan
	now := shared.NewTimestamp()

	// Update subscription
	t.subscription.Plan = newPlan
	t.subscription.PlanID = string(newPlan)
	t.subscription.Status = SubscriptionStatusActive
	t.subscription.UpdatedAt = now

	// Update quotas based on new plan
	t.quotas = newPlan.GetPlanLimits()

	// Calculate new billing period
	if newPlan != PlanFree {
		nextMonth := now.AddDuration(30 * 24 * time.Hour)
		t.subscription.CurrentPeriod = BillingPeriod{
			StartDate: now,
			EndDate:   nextMonth,
			Amount:    newPlan.GetMonthlyPrice(),
			Currency:  "USD",
		}

		// Clear trial if moving to paid plan
		t.subscription.TrialEndsAt = nil
	}

	t.updatedAt = shared.NewTimestamp()

	t.RaiseEvent(
		shared.EventTypeTenantPlanChanged,
		"TenantAggregate",
		map[string]interface{}{
			"tenant_id": t.GetID().String(),
			"old_plan":  string(oldPlan),
			"new_plan":  string(newPlan),
			"amount":    newPlan.GetMonthlyPrice(),
		},
	)

	return nil
}

// UpdateUsage updates the current usage statistics
func (t *TenantAggregate) UpdateUsage(usage TenantUsage) error {
	// Validate usage doesn't exceed quotas
	if usage.CurrentUsers > t.quotas.MaxUsers {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "user quota exceeded", nil)
	}
	
	if usage.CurrentWorkspaces > t.quotas.MaxWorkspaces {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "workspace quota exceeded", nil)
	}
	
	if usage.CurrentProjects > t.quotas.MaxProjects {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "project quota exceeded", nil)
	}
	
	if usage.CurrentStorageGB > float64(t.quotas.MaxStorageGB) {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "storage quota exceeded", nil)
	}

	t.usage = usage
	t.usage.LastUpdated = shared.NewTimestamp()
	t.updatedAt = shared.NewTimestamp()

	return nil
}

// AddPaymentMethod adds a new payment method
func (t *TenantAggregate) AddPaymentMethod(paymentMethod PaymentMethod) error {
	if t.status != TenantStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot add payment method to inactive tenant", nil)
	}

	// If this is the first payment method or marked as default, make it default
	if len(t.billing.PaymentMethods) == 0 || paymentMethod.IsDefault {
		// Set all existing methods to non-default
		for i := range t.billing.PaymentMethods {
			t.billing.PaymentMethods[i].IsDefault = false
		}
		paymentMethod.IsDefault = true
	}

	t.billing.PaymentMethods = append(t.billing.PaymentMethods, paymentMethod)
	t.billing.UpdatedAt = shared.NewTimestamp()
	t.updatedAt = shared.NewTimestamp()

	return nil
}

// Suspend suspends the tenant
func (t *TenantAggregate) Suspend(reason string) error {
	if t.status == TenantStatusSuspended {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "tenant is already suspended", nil)
	}

	t.status = TenantStatusSuspended
	t.updatedAt = shared.NewTimestamp()

	t.RaiseEvent(
		shared.EventTypeTenantDeactivated,
		"TenantAggregate",
		map[string]interface{}{
			"tenant_id": t.GetID().String(),
			"status":    string(t.status),
			"reason":    reason,
		},
	)

	return nil
}

// Reactivate reactivates a suspended tenant
func (t *TenantAggregate) Reactivate() error {
	if t.status == TenantStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "tenant is already active", nil)
	}

	if t.status == TenantStatusDeleted {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot reactivate deleted tenant", nil)
	}

	t.status = TenantStatusActive
	t.updatedAt = shared.NewTimestamp()

	return nil
}

// CheckQuotas checks if current usage is within quotas
func (t *TenantAggregate) CheckQuotas() []string {
	violations := make([]string, 0)

	if t.usage.CurrentUsers > t.quotas.MaxUsers {
		violations = append(violations, "user quota exceeded")
	}
	if t.usage.CurrentWorkspaces > t.quotas.MaxWorkspaces {
		violations = append(violations, "workspace quota exceeded")
	}
	if t.usage.CurrentProjects > t.quotas.MaxProjects {
		violations = append(violations, "project quota exceeded")
	}
	if t.usage.CurrentStorageGB > float64(t.quotas.MaxStorageGB) {
		violations = append(violations, "storage quota exceeded")
	}
	if t.usage.CurrentAPICallsThisMonth > t.quotas.MaxAPICallsPerMonth {
		violations = append(violations, "API quota exceeded")
	}

	return violations
}

// IsTrialExpired checks if the trial period has expired
func (t *TenantAggregate) IsTrialExpired() bool {
	if t.subscription.TrialEndsAt == nil {
		return false
	}
	return shared.NewTimestamp().After(*t.subscription.TrialEndsAt)
}

// isValidSlug validates a tenant slug
func isValidSlug(slug string) bool {
	if len(slug) < 3 || len(slug) > 50 {
		return false
	}
	
	// Check if slug contains only alphanumeric characters and hyphens
	for _, char := range slug {
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-') {
			return false
		}
	}
	
	// Cannot start or end with hyphen
	return !strings.HasPrefix(slug, "-") && !strings.HasSuffix(slug, "-")
}

// LoadFromHistory rebuilds the aggregate from events
func (t *TenantAggregate) LoadFromHistory(events []shared.DomainEvent) error {
	if err := t.BaseAggregateRoot.LoadFromHistory(events); err != nil {
		return err
	}

	for _, event := range events {
		switch event.EventType() {
		case shared.EventTypeTenantCreated:
			t.applyTenantCreatedEvent(event)
		case shared.EventTypeTenantUpdated:
			t.applyTenantUpdatedEvent(event)
		case shared.EventTypeTenantPlanChanged:
			t.applyTenantPlanChangedEvent(event)
		case shared.EventTypeTenantDeactivated:
			t.applyTenantDeactivatedEvent(event)
		}
	}

	return nil
}

func (t *TenantAggregate) applyTenantCreatedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	
	t.name = payload["name"].(string)
	t.slug = payload["slug"].(string)
	t.domain = payload["domain"].(string)
	t.status = TenantStatusActive
	
	if plan, ok := payload["plan"].(string); ok {
		t.subscription.Plan = SubscriptionPlan(plan)
		t.quotas = t.subscription.Plan.GetPlanLimits()
	}
}

func (t *TenantAggregate) applyTenantUpdatedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	
	if name, ok := payload["name"].(string); ok {
		t.name = name
	}
	if domain, ok := payload["domain"].(string); ok {
		t.domain = domain
	}
}

func (t *TenantAggregate) applyTenantPlanChangedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	
	if newPlan, ok := payload["new_plan"].(string); ok {
		plan := SubscriptionPlan(newPlan)
		t.subscription.Plan = plan
		t.quotas = plan.GetPlanLimits()
	}
}

func (t *TenantAggregate) applyTenantDeactivatedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	t.status = TenantStatus(payload["status"].(string))
}