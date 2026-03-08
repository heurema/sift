package zitadel

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Validator interface {
	Validate(ctx context.Context, rawToken string) error
}

type OIDCValidator struct {
	verifier *oidc.IDTokenVerifier
}

func NewOIDCValidator(ctx context.Context, issuer, audience string) (*OIDCValidator, error) {
	trimmedIssuer := strings.TrimSpace(issuer)
	if trimmedIssuer == "" {
		return nil, fmt.Errorf("zitadel issuer is required")
	}

	trimmedAudience := strings.TrimSpace(audience)
	if trimmedAudience == "" {
		return nil, fmt.Errorf("zitadel audience is required")
	}

	provider, err := oidc.NewProvider(ctx, trimmedIssuer)
	if err != nil {
		return nil, fmt.Errorf("init oidc provider: %w", err)
	}

	return &OIDCValidator{
		verifier: provider.Verifier(&oidc.Config{
			ClientID: trimmedAudience,
		}),
	}, nil
}

func (v *OIDCValidator) Validate(ctx context.Context, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" {
		return fmt.Errorf("token is required")
	}

	if _, err := v.verifier.Verify(ctx, rawToken); err != nil {
		return fmt.Errorf("verify token: %w", err)
	}
	return nil
}

func ExtractBearerToken(headerValue string) (string, error) {
	trimmed := strings.TrimSpace(headerValue)
	if trimmed == "" {
		return "", fmt.Errorf("authorization header is required")
	}

	parts := strings.SplitN(trimmed, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", fmt.Errorf("authorization header must be Bearer token")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", fmt.Errorf("bearer token is empty")
	}
	return token, nil
}
