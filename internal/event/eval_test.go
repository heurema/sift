package event

import "testing"

func TestEvaluatePairs(t *testing.T) {
	t.Parallel()

	pairs := []EvalPair{
		{
			ID:     "same-1",
			TitleA: "Bitcoin ETF inflows reach $500 million",
			TitleB: "Bitcoin ETF inflows hit $500 million",
			Label:  EvalLabelSameEvent,
		},
		{
			ID:     "diff-1",
			TitleA: "Ethereum staking deposits rise ahead of upgrade",
			TitleB: "SEC fines exchange for AML violations",
			Label:  EvalLabelDifferentEvent,
		},
	}

	metrics, err := EvaluatePairs(pairs, MergeSimilarityThreshold)
	if err != nil {
		t.Fatalf("EvaluatePairs returned error: %v", err)
	}

	if metrics.PairsTotal != 2 {
		t.Fatalf("unexpected pair count: %d", metrics.PairsTotal)
	}
	if metrics.TruePositives != 1 {
		t.Fatalf("unexpected true positives: %d", metrics.TruePositives)
	}
	if metrics.FalsePositives != 0 {
		t.Fatalf("unexpected false positives: %d", metrics.FalsePositives)
	}
	if metrics.Precision != 1 {
		t.Fatalf("unexpected precision: %f", metrics.Precision)
	}
}

func TestEvaluatePairsRejectsUnsupportedLabel(t *testing.T) {
	t.Parallel()

	_, err := EvaluatePairs([]EvalPair{
		{
			ID:     "bad",
			TitleA: "a",
			TitleB: "b",
			Label:  "unknown",
		},
	}, MergeSimilarityThreshold)
	if err == nil {
		t.Fatal("expected error for unsupported label")
	}
}

func TestSimilarityForTitles(t *testing.T) {
	t.Parallel()

	score := SimilarityForTitles(
		"Bitcoin ETF inflows hit $500 million.",
		"Bitcoin ETF inflows hit $500 million",
	)

	if score < MergeSimilarityThreshold {
		t.Fatalf("expected similarity >= %.2f, got %.4f", MergeSimilarityThreshold, score)
	}
}
