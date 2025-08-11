package shared

import (
	"regexp"
	"strings"
	"unicode"
)

// ToSlug converts a string to a URL-friendly slug
func ToSlug(input string) string {
	// Convert to lowercase
	result := strings.ToLower(input)
	
	// Replace spaces and special characters with hyphens
	reg := regexp.MustCompile(`[^\p{L}\p{N}]+`)
	result = reg.ReplaceAllString(result, "-")
	
	// Remove leading and trailing hyphens
	result = strings.Trim(result, "-")
	
	// Limit length
	if len(result) > 50 {
		result = result[:50]
		result = strings.TrimSuffix(result, "-")
	}
	
	// Ensure it's not empty
	if result == "" {
		result = "unnamed"
	}
	
	return result
}

// IsValidSlug checks if a string is a valid slug format
func IsValidSlug(slug string) bool {
	if len(slug) < 1 || len(slug) > 50 {
		return false
	}
	
	// Check pattern: alphanumeric and hyphens only, no leading/trailing hyphens
	matched, _ := regexp.MatchString(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, slug)
	return matched
}

// SanitizeString removes dangerous characters from input strings
func SanitizeString(input string) string {
	// Remove control characters
	result := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, input)
	
	// Trim whitespace
	result = strings.TrimSpace(result)
	
	return result
}

// TruncateString truncates a string to a maximum length
func TruncateString(input string, maxLength int) string {
	if len(input) <= maxLength {
		return input
	}
	
	// Truncate at word boundary if possible
	if maxLength > 3 {
		truncated := input[:maxLength-3]
		if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxLength/2 {
			return truncated[:lastSpace] + "..."
		}
		return truncated + "..."
	}
	
	return input[:maxLength]
}

// ValidateStringLength validates string length constraints
func ValidateStringLength(value string, minLength, maxLength int, fieldName string) error {
	length := len(value)
	
	if length < minLength {
		return NewDomainError(
			ErrCodeValidation,
			fieldName+" is too short",
			nil,
		).WithContext("min_length", minLength).WithContext("actual_length", length)
	}
	
	if length > maxLength {
		return NewDomainError(
			ErrCodeValidation,
			fieldName+" is too long",
			nil,
		).WithContext("max_length", maxLength).WithContext("actual_length", length)
	}
	
	return nil
}

// ContainsOnlyAlphanumeric checks if string contains only alphanumeric characters
func ContainsOnlyAlphanumeric(input string) bool {
	for _, char := range input {
		if !unicode.IsLetter(char) && !unicode.IsNumber(char) {
			return false
		}
	}
	return true
}

// IsEmptyOrWhitespace checks if string is empty or contains only whitespace
func IsEmptyOrWhitespace(input string) bool {
	return strings.TrimSpace(input) == ""
}