package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"sift/internal/event"
)

const defaultClusteringEvalDatasetPath = "docs/contracts/clustering-eval.v0.json"

type clusteringEvalDataset struct {
	Version            int                  `json:"version"`
	UpdatedAt          string               `json:"updated_at"`
	Metric             string               `json:"metric"`
	SameEventThreshold float64              `json:"same_event_threshold"`
	TargetPrecision    float64              `json:"target_precision"`
	Pairs              []clusteringEvalPair `json:"pairs"`
}

type clusteringEvalPair struct {
	ID     string `json:"id"`
	TitleA string `json:"title_a"`
	TitleB string `json:"title_b"`
	Label  string `json:"label"`
}

type clusteringEvalReport struct {
	DatasetPath      string  `json:"dataset_path"`
	DatasetVersion   int     `json:"dataset_version"`
	DatasetUpdatedAt string  `json:"dataset_updated_at"`
	Metric           string  `json:"metric"`
	PairsTotal       int     `json:"pairs_total"`
	SameEventPairs   int     `json:"same_event_pairs"`
	DifferentPairs   int     `json:"different_event_pairs"`
	Threshold        float64 `json:"threshold"`
	TargetPrecision  float64 `json:"target_precision"`
	TruePositives    int     `json:"tp"`
	FalsePositives   int     `json:"fp"`
	TrueNegatives    int     `json:"tn"`
	FalseNegatives   int     `json:"fn"`
	Precision        float64 `json:"precision"`
	Passed           bool    `json:"passed"`
	GeneratedAt      string  `json:"generated_at"`
}

func runEval(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("usage: sift eval clustering [--dataset path] [--format json|text]"),
		}
	}

	switch args[0] {
	case "clustering":
		return runEvalClustering(ctx, args[1:])
	default:
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unknown eval command: %s", args[0]),
		}
	}
}

func runEvalClustering(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("eval clustering", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	datasetPath := fs.String("dataset", defaultClusteringEvalDatasetPath, "Path to clustering eval dataset JSON")
	format := fs.String("format", "json", "Output format: json|text")
	threshold := fs.Float64("threshold", event.MergeSimilarityThreshold, "Similarity threshold for same-event prediction")
	targetPrecision := fs.Float64("target-precision", 0.90, "Minimum required precision")
	minPairs := fs.Int("min-pairs", 100, "Minimum required labeled pairs in dataset")

	if err := fs.Parse(args); err != nil {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("invalid eval clustering arguments: %w", err),
		}
	}

	if fs.NArg() != 0 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unexpected positional arguments for eval clustering"),
		}
	}

	if *format != "json" && *format != "text" {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unsupported format %q, expected json or text", *format),
		}
	}

	if *threshold <= 0 || *threshold > 1 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("--threshold must be in (0, 1]"),
		}
	}

	if *targetPrecision <= 0 || *targetPrecision > 1 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("--target-precision must be in (0, 1]"),
		}
	}

	if *minPairs <= 0 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("--min-pairs must be positive"),
		}
	}

	dataset, evalPairs, err := loadClusteringEvalDataset(*datasetPath)
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	metrics, err := event.EvaluatePairs(evalPairs, *threshold)
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	report := clusteringEvalReport{
		DatasetPath:      *datasetPath,
		DatasetVersion:   dataset.Version,
		DatasetUpdatedAt: dataset.UpdatedAt,
		Metric:           dataset.Metric,
		PairsTotal:       metrics.PairsTotal,
		SameEventPairs:   metrics.SameEventPairs,
		DifferentPairs:   metrics.DifferentPairs,
		Threshold:        *threshold,
		TargetPrecision:  *targetPrecision,
		TruePositives:    metrics.TruePositives,
		FalsePositives:   metrics.FalsePositives,
		TrueNegatives:    metrics.TrueNegatives,
		FalseNegatives:   metrics.FalseNegatives,
		Precision:        metrics.Precision,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	report.Passed = report.PairsTotal >= *minPairs && report.Precision >= report.TargetPrecision

	if *format == "json" {
		if err := writeJSON(os.Stdout, report); err != nil {
			return &commandError{
				Code: exitOperationalFailure,
				Err:  err,
			}
		}
	} else {
		status := "PASS"
		if !report.Passed {
			status = "FAIL"
		}
		fmt.Printf("dataset=%s version=%d pairs=%d same=%d different=%d\n", report.DatasetPath, report.DatasetVersion, report.PairsTotal, report.SameEventPairs, report.DifferentPairs)
		fmt.Printf("threshold=%.2f target_precision=%.2f precision=%.4f tp=%d fp=%d tn=%d fn=%d\n", report.Threshold, report.TargetPrecision, report.Precision, report.TruePositives, report.FalsePositives, report.TrueNegatives, report.FalseNegatives)
		fmt.Printf("status=%s\n", status)
	}

	if report.PairsTotal < *minPairs {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  fmt.Errorf("clustering eval gate failed: dataset has %d pairs, minimum required is %d", report.PairsTotal, *minPairs),
		}
	}
	if report.Precision < report.TargetPrecision {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  fmt.Errorf("clustering eval gate failed: precision %.4f is below %.2f", report.Precision, report.TargetPrecision),
		}
	}

	return nil
}

func loadClusteringEvalDataset(path string) (clusteringEvalDataset, []event.EvalPair, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return clusteringEvalDataset{}, nil, fmt.Errorf("read clustering eval dataset %s: %w", path, err)
	}

	var dataset clusteringEvalDataset
	if err := json.Unmarshal(raw, &dataset); err != nil {
		return clusteringEvalDataset{}, nil, fmt.Errorf("decode clustering eval dataset %s: %w", path, err)
	}

	if strings.TrimSpace(dataset.Metric) == "" {
		return clusteringEvalDataset{}, nil, fmt.Errorf("clustering eval dataset %s: metric is required", path)
	}

	pairs := make([]event.EvalPair, 0, len(dataset.Pairs))
	for i, pair := range dataset.Pairs {
		titleA := strings.TrimSpace(pair.TitleA)
		titleB := strings.TrimSpace(pair.TitleB)
		label := event.EvalLabel(strings.ToLower(strings.TrimSpace(pair.Label)))
		if titleA == "" || titleB == "" {
			return clusteringEvalDataset{}, nil, fmt.Errorf("clustering eval dataset %s: pair %d has empty title", path, i)
		}
		if label != event.EvalLabelSameEvent && label != event.EvalLabelDifferentEvent {
			return clusteringEvalDataset{}, nil, fmt.Errorf("clustering eval dataset %s: pair %d has unsupported label %q", path, i, pair.Label)
		}

		id := strings.TrimSpace(pair.ID)
		if id == "" {
			id = fmt.Sprintf("pair_%03d", i+1)
		}

		pairs = append(pairs, event.EvalPair{
			ID:     id,
			TitleA: titleA,
			TitleB: titleB,
			Label:  label,
		})
	}

	return dataset, pairs, nil
}
