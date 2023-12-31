package envs

import (
	"os"
	"testing"
)

func TestLookupEnv(t *testing.T) {
	// Test when the environment variable is not set
	os.Clearenv()
	result := LookupEnv("NON_EXISTING_VAR")
	if result != nil {
		t.Errorf("Expected nil, got %s", *result)
	}

	// Test when the environment variable is set
	expectedValue := "test_value"
	os.Setenv("EXISTING_VAR", expectedValue)
	result = LookupEnv("EXISTING_VAR")
	if result == nil || *result != expectedValue {
		t.Errorf("Expected %s, got %s", expectedValue, *result)
	}
}
