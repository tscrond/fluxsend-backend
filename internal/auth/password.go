package auth

import (
	"context"
	"net/http"
)

func NewPasswordAuthProvider() AuthProvider {
	return &PasswordAuthProvider{}
}

type PasswordAuthProvider struct{}

func (p *PasswordAuthProvider) Name() string {
	return "password"
}

func (p *PasswordAuthProvider) GetAuthURL(state string) string {
	return ""
}

func (p *PasswordAuthProvider) HandleCallback(ctx context.Context, r *http.Request) (*AuthResult, error) {
	return nil, nil
}

func (p *PasswordAuthProvider) Logout(ctx context.Context, accessToken string) error {
	return nil
}
