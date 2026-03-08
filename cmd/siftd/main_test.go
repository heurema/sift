package main

import "testing"

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("SIFTD_POSTGRES_DSN", "postgres://user:pass@localhost:5432/sift?sslmode=disable")
	t.Setenv("SIFTD_ZITADEL_ISSUER", "https://auth.example.com")
	t.Setenv("SIFTD_ZITADEL_AUDIENCE", "audience")
	t.Setenv("SIFTD_SYNC_INTERVAL", "1m")
	t.Setenv("SIFTD_SYNC_TIMEOUT", "30s")
	t.Setenv("SIFTD_SYNC_ON_START", "false")
	t.Setenv("SIFTD_WS_ALLOWED_ORIGINS", "https://sift.local, https://console.sift.local")

	cfg, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv returned error: %v", err)
	}

	if cfg.SyncOnStart {
		t.Fatal("expected SyncOnStart=false")
	}
	if cfg.SyncInterval.String() != "1m0s" {
		t.Fatalf("unexpected sync interval: %s", cfg.SyncInterval)
	}
	if cfg.SyncTimeout.String() != "30s" {
		t.Fatalf("unexpected sync timeout: %s", cfg.SyncTimeout)
	}
	if len(cfg.WSAllowedOrigins) != 2 {
		t.Fatalf("unexpected ws allowed origins count: %d", len(cfg.WSAllowedOrigins))
	}
	if cfg.WSAllowedOrigins[0] != "https://sift.local" {
		t.Fatalf("unexpected first ws allowed origin: %s", cfg.WSAllowedOrigins[0])
	}
}

func TestLoadConfigFromEnvRequiresMandatoryValues(t *testing.T) {
	t.Setenv("SIFTD_POSTGRES_DSN", "")
	t.Setenv("SIFTD_ZITADEL_ISSUER", "")
	t.Setenv("SIFTD_ZITADEL_AUDIENCE", "")

	if _, err := loadConfigFromEnv(); err == nil {
		t.Fatal("expected error for missing required env values")
	}
}
