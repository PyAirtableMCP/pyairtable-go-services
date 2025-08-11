package cqrs

import (
	"encoding/json"
	"fmt"
)

// parseJSONArray parses a JSON string into a string slice
func parseJSONArray(jsonStr string, target *[]string) error {
	if jsonStr == "" || jsonStr == "null" {
		*target = []string{}
		return nil
	}

	var temp []interface{}
	if err := json.Unmarshal([]byte(jsonStr), &temp); err != nil {
		return fmt.Errorf("failed to unmarshal JSON array: %w", err)
	}

	result := make([]string, 0, len(temp))
	for _, item := range temp {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}

	*target = result
	return nil
}

// parseJSONObject parses a JSON string into a map[string]interface{}
func parseJSONObject(jsonStr string, target *map[string]interface{}) error {
	if jsonStr == "" || jsonStr == "null" {
		*target = make(map[string]interface{})
		return nil
	}

	if err := json.Unmarshal([]byte(jsonStr), target); err != nil {
		return fmt.Errorf("failed to unmarshal JSON object: %w", err)
	}

	return nil
}