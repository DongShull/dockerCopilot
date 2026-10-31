package auth

import (
	"context"
	"testing"

	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
)

func TestLoginRejectsEmptyConfiguredSecret(t *testing.T) {
	logic := NewLoginLogic(context.Background(), &svc.ServiceContext{Config: config.Config{}})
	resp, err := logic.Login(&types.LoginReq{SecretKey: ""})
	if err == nil {
		t.Fatal("expected error when server secret is empty")
	}
	if resp.Code != 500 {
		t.Fatalf("code = %d, want 500", resp.Code)
	}
}

func TestLoginRejectsWrongSecret(t *testing.T) {
	cfg := config.Config{}
	cfg.Auth.AccessSecret = "correct-secret"
	logic := NewLoginLogic(context.Background(), &svc.ServiceContext{Config: cfg})
	resp, err := logic.Login(&types.LoginReq{SecretKey: "wrong-secret"})
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
	if resp.Code != 401 {
		t.Fatalf("code = %d, want 401", resp.Code)
	}
}

func TestLoginAcceptsCorrectSecret(t *testing.T) {
	cfg := config.Config{}
	cfg.Auth.AccessSecret = "correct-secret"
	cfg.Auth.AccessExpire = 3600
	logic := NewLoginLogic(context.Background(), &svc.ServiceContext{Config: cfg})
	resp, err := logic.Login(&types.LoginReq{SecretKey: "correct-secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("code = %d, want 200", resp.Code)
	}
}
