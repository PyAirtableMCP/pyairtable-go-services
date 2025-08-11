package shared

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Money represents a monetary value with currency
type Money struct {
	Amount   int64  // Amount in smallest currency unit (e.g., cents)
	Currency string // ISO 4217 currency code
}

// NewMoney creates a new Money value object
func NewMoney(amount int64, currency string) (Money, error) {
	if !IsValidCurrency(currency) {
		return Money{}, NewDomainError(ErrCodeValidation, "invalid currency code", nil)
	}
	
	return Money{
		Amount:   amount,
		Currency: strings.ToUpper(currency),
	}, nil
}

// Add adds two Money values (must be same currency)
func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, NewDomainError(ErrCodeValidation, "cannot add different currencies", nil)
	}
	
	return Money{
		Amount:   m.Amount + other.Amount,
		Currency: m.Currency,
	}, nil
}

// Subtract subtracts two Money values (must be same currency)
func (m Money) Subtract(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, NewDomainError(ErrCodeValidation, "cannot subtract different currencies", nil)
	}
	
	return Money{
		Amount:   m.Amount - other.Amount,
		Currency: m.Currency,
	}, nil
}

// IsPositive checks if the amount is positive
func (m Money) IsPositive() bool {
	return m.Amount > 0
}

// IsZero checks if the amount is zero
func (m Money) IsZero() bool {
	return m.Amount == 0
}

// ToFloat converts amount to float (for display purposes)
func (m Money) ToFloat() float64 {
	return float64(m.Amount) / 100.0 // Assuming cents
}

// Equals checks if two Money values are equal
func (m Money) Equals(other ValueObject) bool {
	if otherMoney, ok := other.(Money); ok {
		return m.Amount == otherMoney.Amount && m.Currency == otherMoney.Currency
	}
	return false
}

// IsValid validates the Money value object
func (m Money) IsValid() error {
	if !IsValidCurrency(m.Currency) {
		return NewDomainError(ErrCodeValidation, "invalid currency", nil)
	}
	return nil
}

// String returns string representation
func (m Money) String() string {
	return fmt.Sprintf("%.2f %s", m.ToFloat(), m.Currency)
}

// PhoneNumber represents a validated phone number
type PhoneNumber struct {
	Number      string
	CountryCode string
}

// NewPhoneNumber creates a new PhoneNumber value object
func NewPhoneNumber(number, countryCode string) (PhoneNumber, error) {
	// Clean the number
	cleanNumber := CleanPhoneNumber(number)
	
	if !IsValidPhoneNumber(cleanNumber) {
		return PhoneNumber{}, NewDomainError(ErrCodeValidation, "invalid phone number format", nil)
	}
	
	if !IsValidCountryCode(countryCode) {
		return PhoneNumber{}, NewDomainError(ErrCodeValidation, "invalid country code", nil)
	}
	
	return PhoneNumber{
		Number:      cleanNumber,
		CountryCode: strings.ToUpper(countryCode),
	}, nil
}

// GetFormattedNumber returns a formatted phone number
func (p PhoneNumber) GetFormattedNumber() string {
	return fmt.Sprintf("+%s %s", p.CountryCode, p.Number)
}

// Equals checks if two PhoneNumber values are equal
func (p PhoneNumber) Equals(other ValueObject) bool {
	if otherPhone, ok := other.(PhoneNumber); ok {
		return p.Number == otherPhone.Number && p.CountryCode == otherPhone.CountryCode
	}
	return false
}

// IsValid validates the PhoneNumber value object
func (p PhoneNumber) IsValid() error {
	if !IsValidPhoneNumber(p.Number) {
		return NewDomainError(ErrCodeValidation, "invalid phone number", nil)
	}
	if !IsValidCountryCode(p.CountryCode) {
		return NewDomainError(ErrCodeValidation, "invalid country code", nil)
	}
	return nil
}

// URL represents a validated URL
type URL struct {
	Value string
}

// NewURL creates a new URL value object
func NewURL(urlString string) (URL, error) {
	if urlString == "" {
		return URL{}, NewDomainError(ErrCodeValidation, "URL cannot be empty", nil)
	}
	
	parsedURL, err := url.Parse(urlString)
	if err != nil {
		return URL{}, NewDomainError(ErrCodeValidation, "invalid URL format", err)
	}
	
	if parsedURL.Scheme == "" {
		return URL{}, NewDomainError(ErrCodeValidation, "URL must include scheme", nil)
	}
	
	if parsedURL.Host == "" {
		return URL{}, NewDomainError(ErrCodeValidation, "URL must include host", nil)
	}
	
	return URL{Value: urlString}, nil
}

// GetDomain returns the domain part of the URL
func (u URL) GetDomain() string {
	parsedURL, _ := url.Parse(u.Value)
	return parsedURL.Host
}

// GetScheme returns the scheme part of the URL
func (u URL) GetScheme() string {
	parsedURL, _ := url.Parse(u.Value)
	return parsedURL.Scheme
}

// IsHTTPS checks if the URL uses HTTPS
func (u URL) IsHTTPS() bool {
	return u.GetScheme() == "https"
}

// Equals checks if two URL values are equal
func (u URL) Equals(other ValueObject) bool {
	if otherURL, ok := other.(URL); ok {
		return u.Value == otherURL.Value
	}
	return false
}

// IsValid validates the URL value object
func (u URL) IsValid() error {
	_, err := url.Parse(u.Value)
	if err != nil {
		return NewDomainError(ErrCodeValidation, "invalid URL", err)
	}
	return nil
}

// String returns the URL string
func (u URL) String() string {
	return u.Value
}

// DateRange represents a range between two dates
type DateRange struct {
	StartDate Timestamp
	EndDate   Timestamp
}

// NewDateRange creates a new DateRange value object
func NewDateRange(startDate, endDate Timestamp) (DateRange, error) {
	if startDate.After(endDate) {
		return DateRange{}, NewDomainError(ErrCodeValidation, "start date cannot be after end date", nil)
	}
	
	return DateRange{
		StartDate: startDate,
		EndDate:   endDate,
	}, nil
}

// Duration returns the duration of the date range
func (dr DateRange) Duration() time.Duration {
	return dr.EndDate.Time().Sub(dr.StartDate.Time())
}

// Contains checks if a timestamp falls within the date range
func (dr DateRange) Contains(timestamp Timestamp) bool {
	return !timestamp.Before(dr.StartDate) && !timestamp.After(dr.EndDate)
}

// Overlaps checks if two date ranges overlap
func (dr DateRange) Overlaps(other DateRange) bool {
	return dr.StartDate.Before(other.EndDate) && other.StartDate.Before(dr.EndDate)
}

// Equals checks if two DateRange values are equal
func (dr DateRange) Equals(other ValueObject) bool {
	if otherRange, ok := other.(DateRange); ok {
		return dr.StartDate.Time().Equal(otherRange.StartDate.Time()) &&
			   dr.EndDate.Time().Equal(otherRange.EndDate.Time())
	}
	return false
}

// IsValid validates the DateRange value object
func (dr DateRange) IsValid() error {
	if dr.StartDate.After(dr.EndDate) {
		return NewDomainError(ErrCodeValidation, "invalid date range", nil)
	}
	return nil
}

// IPAddress represents a validated IP address
type IPAddress struct {
	Value string
	Type  IPType
}

// IPType represents the type of IP address
type IPType string

const (
	IPv4 IPType = "ipv4"
	IPv6 IPType = "ipv6"
)

// NewIPAddress creates a new IPAddress value object
func NewIPAddress(ip string) (IPAddress, error) {
	ipType, err := ValidateIPAddress(ip)
	if err != nil {
		return IPAddress{}, err
	}
	
	return IPAddress{
		Value: ip,
		Type:  ipType,
	}, nil
}

// IsIPv4 checks if the IP address is IPv4
func (ip IPAddress) IsIPv4() bool {
	return ip.Type == IPv4
}

// IsIPv6 checks if the IP address is IPv6
func (ip IPAddress) IsIPv6() bool {
	return ip.Type == IPv6
}

// Equals checks if two IPAddress values are equal
func (ip IPAddress) Equals(other ValueObject) bool {
	if otherIP, ok := other.(IPAddress); ok {
		return ip.Value == otherIP.Value
	}
	return false
}

// IsValid validates the IPAddress value object
func (ip IPAddress) IsValid() error {
	_, err := ValidateIPAddress(ip.Value)
	return err
}

// String returns the IP address string
func (ip IPAddress) String() string {
	return ip.Value
}

// Helper functions for validation

// IsValidCurrency checks if a currency code is valid (simplified)
func IsValidCurrency(currency string) bool {
	validCurrencies := map[string]bool{
		"USD": true, "EUR": true, "GBP": true, "JPY": true,
		"CAD": true, "AUD": true, "CHF": true, "CNY": true,
	}
	return validCurrencies[strings.ToUpper(currency)]
}

// CleanPhoneNumber removes formatting from phone number
func CleanPhoneNumber(number string) string {
	reg := regexp.MustCompile(`[^\d]`)
	return reg.ReplaceAllString(number, "")
}

// IsValidPhoneNumber checks if a phone number is valid (simplified)
func IsValidPhoneNumber(number string) bool {
	// Simple validation: 7-15 digits
	matched, _ := regexp.MatchString(`^\d{7,15}$`, number)
	return matched
}

// IsValidCountryCode checks if a country code is valid (simplified)
func IsValidCountryCode(code string) bool {
	// Simple validation: 1-3 digits
	matched, _ := regexp.MatchString(`^\d{1,3}$`, code)
	return matched
}

// ValidateIPAddress validates an IP address and returns its type
func ValidateIPAddress(ip string) (IPType, error) {
	// IPv4 pattern
	ipv4Pattern := `^(\d{1,3}\.){3}\d{1,3}$`
	if matched, _ := regexp.MatchString(ipv4Pattern, ip); matched {
		// Validate octets are in range 0-255
		parts := strings.Split(ip, ".")
		for _, part := range parts {
			if len(part) > 1 && part[0] == '0' {
				return "", NewDomainError(ErrCodeValidation, "invalid IPv4 address", nil)
			}
			var octet int
			fmt.Sscanf(part, "%d", &octet)
			if octet < 0 || octet > 255 {
				return "", NewDomainError(ErrCodeValidation, "invalid IPv4 address", nil)
			}
		}
		return IPv4, nil
	}
	
	// IPv6 pattern (simplified)
	ipv6Pattern := `^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`
	if matched, _ := regexp.MatchString(ipv6Pattern, ip); matched {
		return IPv6, nil
	}
	
	return "", NewDomainError(ErrCodeValidation, "invalid IP address format", nil)
}