package httpapi

import (
	"context"
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Roles used by KubeNova. Keep in sync with docs and enforcement.
const (
	RoleAdmin       = "admin"
	RoleOps         = "ops"
	RoleTenantOwner = "tenantOwner"
	RoleProjectDev  = "projectDev"
	RoleReadOnly    = "readOnly"
)

type contextKey int

const claimsKey contextKey = 1

type Claims struct {
	Subject string   `json:"sub"`
	Roles   []string `json:"roles"`
	jwt.RegisteredClaims
}

type AuthConfig struct{ Key []byte }

func (a AuthConfig) ParseFromHeader(authz string) (*Claims, error) {
	if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		return nil, errors.New("KN-401: missing bearer")
	}
	tok := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer"))
	var c Claims
	if _, err := jwt.ParseWithClaims(tok, &c, func(t *jwt.Token) (interface{}, error) { return a.Key, nil }); err != nil {
		return nil, errors.New("KN-401: invalid token")
	}
	return &c, nil
}

func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}
func ClaimsFrom(ctx context.Context) *Claims {
	if v := ctx.Value(claimsKey); v != nil {
		if c, ok := v.(*Claims); ok {
			return c
		}
	}
	return &Claims{Subject: "anonymous", Roles: []string{RoleReadOnly}}
}

func HasRole(c *Claims, want string) bool {
	for _, r := range c.Roles {
		if r == want {
			return true
		}
	}
	return false
}
