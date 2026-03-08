package main

import (
	"strings"
	"testing"
	"time"

	"sift/internal/event"
)

func TestParseEventGetOptionsSupportsEventIDBeforeFlags(t *testing.T) {
	t.Parallel()

	opts, err := parseEventGetOptions([]string{
		"evt_123",
		"--format", "md",
		"--state-dir", "tmp-state",
	})
	if err != nil {
		t.Fatalf("parseEventGetOptions returned error: %v", err)
	}

	if opts.EventID != "evt_123" {
		t.Fatalf("unexpected event id: %s", opts.EventID)
	}
	if opts.Format != "md" {
		t.Fatalf("unexpected format: %s", opts.Format)
	}
	if opts.StateDir != "tmp-state" {
		t.Fatalf("unexpected state dir: %s", opts.StateDir)
	}
}

func TestParseEventGetOptionsRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	_, err := parseEventGetOptions([]string{"evt_123", "--unknown"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestParseSinceValueSupportsRelativeWindows(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC)

	since24h, ok, err := parseSinceValue("24h", now)
	if err != nil {
		t.Fatalf("parseSinceValue(24h) returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected parseSinceValue(24h) to return ok=true")
	}
	if !since24h.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("unexpected since24h value: %s", since24h)
	}

	since7d, ok, err := parseSinceValue("7d", now)
	if err != nil {
		t.Fatalf("parseSinceValue(7d) returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected parseSinceValue(7d) to return ok=true")
	}
	if !since7d.Equal(now.Add(-7 * 24 * time.Hour)) {
		t.Fatalf("unexpected since7d value: %s", since7d)
	}
}

func TestMatchesSearchFilters(t *testing.T) {
	t.Parallel()

	rec := event.Record{
		EventID:     "evt_1",
		PublishedAt: "2026-03-06T12:00:00Z",
		EventType:   "policy",
		Status:      "multi_source_verified",
		Assets:      []string{"BTC"},
		Topics:      []string{"policy"},
		SupportingArticles: []event.SupportingArticle{
			{Source: "CoinDesk"},
		},
	}

	since := time.Date(2026, 3, 6, 10, 0, 0, 0, time.UTC)
	until := time.Date(2026, 3, 6, 14, 0, 0, 0, time.UTC)
	filters := searchFilters{
		Assets:     normalizedSet([]string{"btc"}, strings.ToUpper),
		Topics:     normalizedSet([]string{"POLICY"}, strings.ToLower),
		EventTypes: normalizedSet([]string{"Policy"}, strings.ToLower),
		Statuses:   normalizedSet([]string{"MULTI_SOURCE_VERIFIED"}, strings.ToLower),
		Sources:    sourceFilterSet([]string{"coindesk_rss"}),
		Since:      &since,
		Until:      &until,
	}

	matches, err := matchesSearchFilters(rec, filters)
	if err != nil {
		t.Fatalf("matchesSearchFilters returned error: %v", err)
	}
	if !matches {
		t.Fatal("expected event to match filters")
	}
}

func TestFilterSearchEventsReturnsParseErrorForInvalidPublishedAt(t *testing.T) {
	t.Parallel()

	records := []event.Record{
		{
			EventID:     "evt_bad",
			PublishedAt: "not-a-time",
		},
	}

	_, err := filterSearchEvents(records, searchFilters{})
	if err == nil {
		t.Fatal("expected filterSearchEvents to return error on invalid published_at")
	}
}
