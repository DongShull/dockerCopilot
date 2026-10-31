package main

import "testing"

func TestValidateSecretKey(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{name: "empty", secret: "   ", wantErr: true},
		{name: "unexpanded env", secret: "${secretKey}", wantErr: true},
		{name: "too short", secret: "abc12345", wantErr: true},
		{name: "pure digits", secret: "123456789", wantErr: true},
		{name: "valid", secret: "change-me-please", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSecretKey(tt.secret)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSecretKey(%q) error = %v, wantErr %v", tt.secret, err, tt.wantErr)
			}
		})
	}
}
