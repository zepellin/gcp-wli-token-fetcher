package tools

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestGetEnv(t *testing.T) {
	// Test case 1: Key exists in environment
	os.Setenv("EXISTING_KEY", "existing_value")
	defer os.Unsetenv("EXISTING_KEY")

	result := GetEnv("EXISTING_KEY", "fallback_value")
	if result != "existing_value" {
		t.Errorf("GetEnv(EXISTING_KEY, fallback_value) = %s; expected existing_value", result)
	}

	// Test case 2: Key does not exist in environment
	result = GetEnv("NON_EXISTING_KEY", "fallback_value")
	if result != "fallback_value" {
		t.Errorf("GetEnv(NON_EXISTING_KEY, fallback_value) = %s; expected fallback_value", result)
	}

	// Test case 3: Fallback value used when key is empty in environment
	os.Unsetenv("EMPTY_KEY")

	result = GetEnv("EMPTY_KEY", "fallback_value")
	if result != "fallback_value" {
		t.Errorf("GetEnv(EMPTY_KEY, fallback_value) = %s; expected fallback_value", result)
	}
}

func TestFileExists(t *testing.T) {
	// Test case 1: File exists
	existingFilePath := "existing_file.txt"
	createTestFile(existingFilePath)
	defer removeTestFile(existingFilePath)

	result, err := FileExists(existingFilePath)
	if err != nil {
		t.Errorf("FileExists(%s) returned an error: %v", existingFilePath, err)
	}
	if !result {
		t.Errorf("FileExists(%s) = false; expected true", existingFilePath)
	}

	// Test case 2: File does not exist
	nonExistingFilePath := "non_existing_file.txt"
	result, err = FileExists(nonExistingFilePath)
	if err != nil {
		t.Errorf("FileExists(%s) returned an error: %v", nonExistingFilePath, err)
	}
	if result {
		t.Errorf("FileExists(%s) = true; expected false", nonExistingFilePath)
	}
}

func TestGetJWTExp(t *testing.T) {
	// Test case 1: Valid token with correct structure
	validToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2NDU2MjQ0NDAsImp0aSI6IjE1MzM4ZmY3LWZhZDgtNDJjNC05NjJlLTNkOTgyYmEwZGNmZiIsImlhdCI6MTY0NTYyMzA0MCwiaXNzIjoic3ViIiwibmJmIjoxNjQ1NjIzMDQwLCJzdWIiOiJhZG1pbiJ9.4eAqjsbwbgGLTMVTz_F-jCGlQ1Hg48uRjwPFRsOH4DY"
	expTime := time.Date(2022, time.February, 23, 13, 54, 0, 0, time.UTC)

	exp, err := getJWTExp(validToken)
	if err != nil {
		t.Errorf("TestGetJWTExp failed for valid token: %v", err)
	}
	if exp.UTC() != expTime {
		t.Errorf("TestGetJWTExp failed for valid token. Expected expiration time: %s, Got: %s", expTime, exp)
	}

	// Test case 2: Invalid token (missing parts)
	invalidToken := "invalid_token"
	exp, err = getJWTExp(invalidToken)
	if err == nil {
		t.Errorf("TestGetJWTExp did not return an error for an invalid token")
	}
	if !errors.Is(err, jwtParseFail) {
		t.Errorf("TestGetJWTExp returned unexpected error for an invalid token: %v", err)
	}
	if !exp.IsZero() {
		t.Errorf("TestGetJWTExp failed for an invalid token. Expected zero expiration time, Got: %s", exp)
	}

	// Test case 3: Invalid token (error decoding payload)
	invalidToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature"
	exp, err = getJWTExp(invalidToken)
	if err == nil {
		t.Errorf("TestGetJWTExp did not return an error for an invalid token")
	}
	if !errors.Is(err, jwtDecodeFail) {
		t.Errorf("TestGetJWTExp returned unexpected error for an invalid token: %v", err)
	}
	if !exp.IsZero() {
		t.Errorf("TestGetJWTExp failed for an invalid token. Expected zero expiration time, Got: %s", exp)
	}
}

// Helper functions for creating and removing test files
func createTestFile(path string) {
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	file.Close()
}

func removeTestFile(path string) {
	err := os.Remove(path)
	if err != nil {
		panic(err)
	}
}
