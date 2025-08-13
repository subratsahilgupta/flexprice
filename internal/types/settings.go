package types

import (
	"errors"
	"fmt"
	"strings"
)

type SettingKey string

const (
	SettingKeyInvoiceConfig SettingKey = "invoice_config"
)

func (s SettingKey) String() string {
	return string(s)
}

// DefaultSettingValue represents a default setting configuration
type DefaultSettingValue struct {
	Key          SettingKey             `json:"key"`
	DefaultValue map[string]interface{} `json:"default_value"`
	Description  string                 `json:"description"`
	Required     bool                   `json:"required"`
}

// GetDefaultSettings returns the default settings configuration for all setting keys
func GetDefaultSettings() map[SettingKey]DefaultSettingValue {
	return map[SettingKey]DefaultSettingValue{
		SettingKeyInvoiceConfig: {
			Key: SettingKeyInvoiceConfig,
			DefaultValue: map[string]interface{}{
				"prefix":         "INV",
				"format":         "YYYYMM",
				"start_sequence": 1,
			},
			Description: "Default configuration for invoice generation and management",
			Required:    true,
		},
	}
}

// IsValidSettingKey checks if a setting key is valid
func IsValidSettingKey(key string) bool {
	_, exists := GetDefaultSettings()[SettingKey(key)]
	return exists
}

// ValidateSettingValue validates a setting value based on its key
func ValidateSettingValue(key string, value map[string]interface{}) error {
	if value == nil {
		return errors.New("value cannot be nil")
	}

	switch SettingKey(key) {
	case SettingKeyInvoiceConfig:
		return ValidateInvoiceConfig(value)
	default:
		return fmt.Errorf("unknown setting key: %s", key)
	}
}

// ValidateInvoiceConfig validates invoice configuration settings
func ValidateInvoiceConfig(value map[string]interface{}) error {
	if value == nil {
		return errors.New("invoice_config value cannot be nil")
	}

	// Validate prefix
	prefixRaw, exists := value["prefix"]
	if !exists {
		return errors.New("invoice_config: 'prefix' is required")
	}
	prefix, ok := prefixRaw.(string)
	if !ok {
		return fmt.Errorf("invoice_config: 'prefix' must be a string, got %T", prefixRaw)
	}
	if strings.TrimSpace(prefix) == "" {
		return errors.New("invoice_config: 'prefix' cannot be empty")
	}

	// Validate format
	formatRaw, exists := value["format"]
	if !exists {
		return errors.New("invoice_config: 'format' is required")
	}
	_, ok = formatRaw.(string)
	if !ok {
		return fmt.Errorf("invoice_config: 'format' must be a string, got %T", formatRaw)
	}

	// Validate start_sequence
	startSeqRaw, exists := value["start_sequence"]
	if !exists {
		return errors.New("invoice_config: 'start_sequence' is required")
	}

	var startSeq int
	switch v := startSeqRaw.(type) {
	case int:
		startSeq = v
	case float64:
		if v != float64(int(v)) {
			return errors.New("invoice_config: 'start_sequence' must be a whole number")
		}
		startSeq = int(v)
	default:
		return fmt.Errorf("invoice_config: 'start_sequence' must be an integer, got %T", startSeqRaw)
	}

	if startSeq < 1 {
		return errors.New("invoice_config: 'start_sequence' must be greater than 0")
	}

	return nil
}
