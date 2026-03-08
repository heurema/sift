package main

import "testing"

func TestParseDigestOptionsScopeBeforeFlags(t *testing.T) {
	t.Parallel()

	opts, err := parseDigestOptions([]string{"crypto", "--window", "7d", "--format", "md"})
	if err != nil {
		t.Fatalf("parseDigestOptions returned error: %v", err)
	}

	if opts.Scope != "crypto" {
		t.Fatalf("unexpected scope: %s", opts.Scope)
	}
	if opts.Window != "7d" {
		t.Fatalf("unexpected window: %s", opts.Window)
	}
	if opts.Format != "md" {
		t.Fatalf("unexpected format: %s", opts.Format)
	}
}

func TestParseDigestOptionsRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	_, err := parseDigestOptions([]string{"crypto", "--unknown"})
	if err == nil {
		t.Fatal("expected parseDigestOptions to return error for unknown flag")
	}
}
