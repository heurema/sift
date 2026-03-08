package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClusteringEvalDataset(t *testing.T) {
	t.Parallel()

	path := writeTestEvalDataset(t, clusteringEvalDataset{
		Version:   1,
		UpdatedAt: "2026-03-07",
		Metric:    "sorensen_dice_trigram_on_normalized_titles",
		Pairs: []clusteringEvalPair{
			{
				ID:     "p1",
				TitleA: "Bitcoin ETF inflows hit $500 million",
				TitleB: "Bitcoin ETF inflows reach $500 million",
				Label:  "same_event",
			},
			{
				ID:     "p2",
				TitleA: "Solana outage halts block production",
				TitleB: "Stablecoin reserve report confirms backing",
				Label:  "different_event",
			},
		},
	})

	dataset, pairs, err := loadClusteringEvalDataset(path)
	if err != nil {
		t.Fatalf("loadClusteringEvalDataset returned error: %v", err)
	}

	if dataset.Version != 1 {
		t.Fatalf("unexpected dataset version: %d", dataset.Version)
	}
	if len(pairs) != 2 {
		t.Fatalf("unexpected pair count: %d", len(pairs))
	}
}

func TestRunEvalClusteringPasses(t *testing.T) {
	t.Parallel()

	path := writeTestEvalDataset(t, clusteringEvalDataset{
		Version:   1,
		UpdatedAt: "2026-03-07",
		Metric:    "sorensen_dice_trigram_on_normalized_titles",
		Pairs: []clusteringEvalPair{
			{
				ID:     "p1",
				TitleA: "Bitcoin ETF inflows hit $500 million",
				TitleB: "Bitcoin ETF inflows reach $500 million",
				Label:  "same_event",
			},
			{
				ID:     "p2",
				TitleA: "Solana outage halts block production",
				TitleB: "Stablecoin reserve report confirms backing",
				Label:  "different_event",
			},
		},
	})

	err := run(context.Background(), []string{
		"eval", "clustering",
		"--dataset", path,
		"--min-pairs", "2",
		"--format", "json",
	})
	if err != nil {
		t.Fatalf("run eval clustering returned error: %v", err)
	}
}

func TestRunEvalClusteringFailsOnLowPrecision(t *testing.T) {
	t.Parallel()

	path := writeTestEvalDataset(t, clusteringEvalDataset{
		Version:   1,
		UpdatedAt: "2026-03-07",
		Metric:    "sorensen_dice_trigram_on_normalized_titles",
		Pairs: []clusteringEvalPair{
			{
				ID:     "p1",
				TitleA: "Bitcoin ETF inflows hit $500 million",
				TitleB: "Bitcoin ETF inflows reach $500 million",
				Label:  "same_event",
			},
			{
				ID:     "p2",
				TitleA: "Bitcoin ETF inflows hit $500 million",
				TitleB: "Bitcoin ETF inflows reach $500 million",
				Label:  "different_event",
			},
		},
	})

	err := run(context.Background(), []string{
		"eval", "clustering",
		"--dataset", path,
		"--min-pairs", "2",
		"--format", "json",
	})
	if err == nil {
		t.Fatal("expected precision gate failure")
	}

	var cmdErr *commandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected commandError, got %T", err)
	}
	if cmdErr.Code != exitOperationalFailure {
		t.Fatalf("unexpected command error code: %d", cmdErr.Code)
	}
}

func writeTestEvalDataset(t *testing.T, dataset clusteringEvalDataset) string {
	t.Helper()

	raw, err := json.Marshal(dataset)
	if err != nil {
		t.Fatalf("marshal dataset failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "eval.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write dataset file failed: %v", err)
	}

	return path
}
