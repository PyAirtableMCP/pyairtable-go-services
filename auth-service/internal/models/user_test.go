package models

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "testpassword123"
	
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	
	if hash == password {
		t.Error("Password hash should not equal plain text password")
	}
	
	if len(hash) == 0 {
		t.Error("Password hash should not be empty")
	}
}

func TestCheckPassword(t *testing.T) {
	password := "testpassword123"
	wrongPassword := "wrongpassword"
	
	// Hash the password
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	
	// Test correct password
	if !CheckPassword(password, hash) {
		t.Error("CheckPassword should return true for correct password")
	}
	
	// Test incorrect password
	if CheckPassword(wrongPassword, hash) {
		t.Error("CheckPassword should return false for incorrect password")
	}
	
	// Test empty password
	if CheckPassword("", hash) {
		t.Error("CheckPassword should return false for empty password")
	}
	
	// Test empty hash
	if CheckPassword(password, "") {
		t.Error("CheckPassword should return false for empty hash")
	}
}

func TestLoginRequest_GetIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		request    LoginRequest
		expected   string
	}{
		{
			name: "Email provided",
			request: LoginRequest{
				Email:    "user@example.com",
				Password: "password123",
			},
			expected: "user@example.com",
		},
		{
			name: "Username provided",
			request: LoginRequest{
				Username: "testuser",
				Password: "password123",
			},
			expected: "testuser",
		},
		{
			name: "Both email and username provided - email takes precedence",
			request: LoginRequest{
				Email:    "user@example.com",
				Username: "testuser",
				Password: "password123",
			},
			expected: "user@example.com",
		},
		{
			name: "Neither email nor username provided",
			request: LoginRequest{
				Password: "password123",
			},
			expected: "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.request.GetIdentifier()
			if result != tt.expected {
				t.Errorf("GetIdentifier() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestPasswordValidation(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "Valid password",
			password: "password123",
			wantErr:  false,
		},
		{
			name:     "Short password",
			password: "short",
			wantErr:  false, // bcrypt will hash even short passwords
		},
		{
			name:     "Empty password",
			password: "",
			wantErr:  false, // bcrypt will hash empty passwords too
		},
		{
			name:     "Long password (within bcrypt limit)",
			password: "this-is-a-long-password-within-72-byte-limit-should-work",
			wantErr:  false,
		},
		{
			name:     "Too long password (exceeds bcrypt limit)",
			password: "this-is-an-extremely-long-password-that-exceeds-the-72-byte-limit-that-bcrypt-imposes-on-passwords-and-should-fail",
			wantErr:  true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			
			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			
			if !tt.wantErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			
			if !tt.wantErr {
				// Verify the hash can be validated
				if !CheckPassword(tt.password, hash) {
					t.Error("Generated hash doesn't validate against original password")
				}
			}
		})
	}
}