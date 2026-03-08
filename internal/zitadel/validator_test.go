package zitadel

import "testing"

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		headerValue string
		wantToken   string
		wantErr     bool
	}{
		{
			name:        "valid token",
			headerValue: "Bearer token123",
			wantToken:   "token123",
		},
		{
			name:        "missing header",
			headerValue: "",
			wantErr:     true,
		},
		{
			name:        "wrong scheme",
			headerValue: "Basic token123",
			wantErr:     true,
		},
		{
			name:        "empty token",
			headerValue: "Bearer ",
			wantErr:     true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ExtractBearerToken(tc.headerValue)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantToken {
				t.Fatalf("unexpected token: got %q want %q", got, tc.wantToken)
			}
		})
	}
}
