package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"sift/internal/event"
	"sift/internal/pipeline"
	"sift/internal/source"
	"sift/internal/sqlite"
)

const (
	exitSuccess            = 0
	exitOperationalFailure = 1
	exitInvalidArguments   = 2
	exitRecordNotFound     = 3
	exitPolicyBlock        = 4
)

type commandError struct {
	Code int
	Err  error
}

func (e *commandError) Error() string {
	return e.Err.Error()
}

type sourcesEnvelope struct {
	Items       []sqlite.SourceRecord `json:"items"`
	GeneratedAt string                `json:"generated_at"`
}

type syncSummary struct {
	RunID            string `json:"run_id"`
	Mode             string `json:"mode"`
	Status           string `json:"status"`
	SourcesTotal     int    `json:"sources_total"`
	SourcesLoaded    int    `json:"sources_loaded"`
	SourcesSucceeded int    `json:"sources_succeeded"`
	SourcesFailed    int    `json:"sources_failed"`
	SourcesDegraded  int    `json:"sources_degraded"`
	ArticlesFetched  int    `json:"articles_fetched"`
	ArticlesInserted int    `json:"articles_inserted"`
	ArticlesUpdated  int    `json:"articles_updated"`
	ArticlesSkipped  int    `json:"articles_skipped"`
	EventsRebuilt    int    `json:"events_rebuilt"`
	ImplementedScope string `json:"implemented_scope"`
}

type eventListEnvelope struct {
	Items       []event.Record `json:"items"`
	NextCursor  *string        `json:"next_cursor"`
	GeneratedAt string         `json:"generated_at"`
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("empty value is not allowed")
	}
	*s = append(*s, trimmed)
	return nil
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		var cmdErr *commandError
		if errors.As(err, &cmdErr) {
			fmt.Fprintln(os.Stderr, cmdErr.Err)
			os.Exit(cmdErr.Code)
		}

		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitOperationalFailure)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage()
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("missing command"),
		}
	}

	switch args[0] {
	case "sync":
		return runSync(ctx, args[1:])
	case "sources":
		return runSources(ctx, args[1:])
	case "latest":
		return runLatest(ctx, args[1:])
	case "search":
		return runSearch(ctx, args[1:])
	case "digest":
		return runDigest(ctx, args[1:])
	case "eval":
		return runEval(ctx, args[1:])
	case "event":
		if len(args) < 2 || args[1] != "get" {
			return &commandError{
				Code: exitInvalidArguments,
				Err:  fmt.Errorf("usage: sift event get <event_id>"),
			}
		}
		return runEventGet(ctx, args[2:])
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		printUsage()
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unknown command: %s", args[0]),
		}
	}
}

func runSources(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sources", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	stateDir := fs.String("state-dir", "state", "Path to local state directory")
	registryPath := fs.String("registry", "docs/contracts/source-registry.seed.json", "Path to source registry JSON")
	format := fs.String("format", "json", "Output format: json|text")

	if err := fs.Parse(args); err != nil {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("invalid sources arguments: %w", err),
		}
	}

	if fs.NArg() != 0 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unexpected positional arguments for sources"),
		}
	}

	if *format != "json" && *format != "text" {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unsupported format %q, expected json or text", *format),
		}
	}

	store, err := openStore(ctx, *stateDir)
	if err != nil {
		return err
	}
	defer store.Close()

	registry, err := source.LoadSeed(*registryPath)
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	if _, err := store.UpsertSources(ctx, registry); err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	items, err := store.ListSources(ctx)
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	if *format == "json" {
		out := sourcesEnvelope{
			Items:       items,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := writeJSON(os.Stdout, out); err != nil {
			return &commandError{
				Code: exitOperationalFailure,
				Err:  err,
			}
		}
		return nil
	}

	for _, item := range items {
		lastSuccess := "-"
		if item.LastSuccessAt != nil {
			lastSuccess = *item.LastSuccessAt
		}
		lastFailure := "-"
		if item.LastFailureAt != nil {
			lastFailure = *item.LastFailureAt
		}
		lastError := "-"
		if item.LastError != nil {
			lastError = *item.LastError
		}

		fmt.Printf(
			"%s\t%s\trights=%s\tconsecutive_failures=%d\tlast_success=%s\tlast_failure=%s\tlast_error=%s\n",
			item.SourceID,
			item.SourceName,
			item.RightsMode,
			item.ConsecutiveFailures,
			lastSuccess,
			lastFailure,
			lastError,
		)
	}

	return nil
}

func runSync(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	stateDir := fs.String("state-dir", "state", "Path to local state directory")
	registryPath := fs.String("registry", "docs/contracts/source-registry.seed.json", "Path to source registry JSON")
	format := fs.String("format", "text", "Output format: text|json")
	fetchOnly := fs.Bool("fetch-only", false, "Run only fetch stage")
	clusterOnly := fs.Bool("cluster-only", false, "Run only cluster stage")

	if err := fs.Parse(args); err != nil {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("invalid sync arguments: %w", err),
		}
	}

	if fs.NArg() != 0 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unexpected positional arguments for sync"),
		}
	}

	if *fetchOnly && *clusterOnly {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("--fetch-only and --cluster-only are mutually exclusive"),
		}
	}

	if *format != "json" && *format != "text" {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unsupported format %q, expected json or text", *format),
		}
	}

	mode := pipeline.ModeFull
	if *fetchOnly {
		mode = pipeline.ModeFetchOnly
	}
	if *clusterOnly {
		mode = pipeline.ModeClusterOnly
	}

	store, err := openStore(ctx, *stateDir)
	if err != nil {
		return err
	}
	defer store.Close()

	runtimeSummary, err := pipeline.RunSync(ctx, store, pipeline.Options{
		Mode:         mode,
		RegistryPath: *registryPath,
		OutputDir:    "output",
	})
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	summary := syncSummary{
		RunID:            runtimeSummary.RunID,
		Mode:             runtimeSummary.Mode,
		Status:           runtimeSummary.Status,
		SourcesTotal:     runtimeSummary.SourcesTotal,
		SourcesLoaded:    runtimeSummary.SourcesLoaded,
		SourcesSucceeded: runtimeSummary.SourcesSucceeded,
		SourcesFailed:    runtimeSummary.SourcesFailed,
		SourcesDegraded:  runtimeSummary.SourcesDegraded,
		ArticlesFetched:  runtimeSummary.ArticlesFetched,
		ArticlesInserted: runtimeSummary.ArticlesInserted,
		ArticlesUpdated:  runtimeSummary.ArticlesUpdated,
		ArticlesSkipped:  runtimeSummary.ArticlesSkipped,
		EventsRebuilt:    runtimeSummary.EventsRebuilt,
		ImplementedScope: runtimeSummary.ImplementedScope,
	}

	if *format == "json" {
		if err := writeJSON(os.Stdout, summary); err != nil {
			return &commandError{
				Code: exitOperationalFailure,
				Err:  err,
			}
		}
		return nil
	}

	fmt.Printf(
		"sync completed: run_id=%s mode=%s status=%s sources_total=%d sources_loaded=%d sources_degraded=%d events_rebuilt=%d scope=%s\n",
		summary.RunID,
		summary.Mode,
		summary.Status,
		summary.SourcesTotal,
		summary.SourcesLoaded,
		summary.SourcesDegraded,
		summary.EventsRebuilt,
		summary.ImplementedScope,
	)
	return nil
}

func runLatest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("latest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	stateDir := fs.String("state-dir", "state", "Path to local state directory")
	limit := fs.Int("limit", 20, "Maximum events to return")
	format := fs.String("format", "json", "Output format: json|text|md")

	if err := fs.Parse(args); err != nil {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("invalid latest arguments: %w", err),
		}
	}

	if fs.NArg() != 0 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unexpected positional arguments for latest"),
		}
	}

	if *limit < 1 || *limit > 100 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("--limit must be between 1 and 100"),
		}
	}

	if *format != "json" && *format != "text" && *format != "md" {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unsupported format %q, expected json, text, or md", *format),
		}
	}

	store, err := openStore(ctx, *stateDir)
	if err != nil {
		return err
	}
	defer store.Close()

	records, err := store.ListLatestEvents(ctx, *limit)
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	if *format == "json" {
		out := eventListEnvelope{
			Items:       records,
			NextCursor:  nil,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := writeJSON(os.Stdout, out); err != nil {
			return &commandError{
				Code: exitOperationalFailure,
				Err:  err,
			}
		}
		return nil
	}

	if *format == "md" {
		var builder strings.Builder
		builder.WriteString("# Latest Events\n\n")
		if len(records) == 0 {
			builder.WriteString("No events available.\n")
		}
		for _, rec := range records {
			builder.WriteString(fmt.Sprintf("- `%s` %s (importance %.2f, confidence %.2f)\n", rec.EventID, rec.Title, rec.ImportanceScore, rec.ConfidenceScore))
		}
		fmt.Print(builder.String())
		return nil
	}

	for _, rec := range records {
		fmt.Printf("%s\t%.2f\t%.2f\t%s\n", rec.EventID, rec.ImportanceScore, rec.ConfidenceScore, rec.Title)
	}

	return nil
}

func runSearch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	stateDir := fs.String("state-dir", "state", "Path to local state directory")
	limit := fs.Int("limit", 20, "Maximum events to return")
	format := fs.String("format", "json", "Output format: json|text|md")
	sinceRaw := fs.String("since", "", "Filter from relative window (24h, 72h, 7d) or RFC3339 timestamp")
	untilRaw := fs.String("until", "", "Filter up to RFC3339 timestamp")

	var assets stringSliceFlag
	var topics stringSliceFlag
	var eventTypes stringSliceFlag
	var statuses stringSliceFlag
	var sources stringSliceFlag
	fs.Var(&assets, "asset", "Asset filter (repeatable)")
	fs.Var(&topics, "topic", "Topic filter (repeatable)")
	fs.Var(&eventTypes, "event-type", "Event type filter (repeatable)")
	fs.Var(&statuses, "status", "Status filter (repeatable)")
	fs.Var(&sources, "source", "Source filter (repeatable)")

	if err := fs.Parse(args); err != nil {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("invalid search arguments: %w", err),
		}
	}

	if fs.NArg() != 0 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unexpected positional arguments for search"),
		}
	}

	if *limit < 1 || *limit > 100 {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("--limit must be between 1 and 100"),
		}
	}

	if *format != "json" && *format != "text" && *format != "md" {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unsupported format %q, expected json, text, or md", *format),
		}
	}

	now := time.Now().UTC()
	since, hasSince, err := parseSinceValue(*sinceRaw, now)
	if err != nil {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  err,
		}
	}

	until, hasUntil, err := parseUntilValue(*untilRaw)
	if err != nil {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  err,
		}
	}

	if hasSince && hasUntil && since.After(until) {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("--since must be earlier than or equal to --until"),
		}
	}

	store, err := openStore(ctx, *stateDir)
	if err != nil {
		return err
	}
	defer store.Close()

	records, err := store.ListEvents(ctx)
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	filters := searchFilters{
		Assets:     normalizedSet([]string(assets), strings.ToUpper),
		Topics:     normalizedSet([]string(topics), strings.ToLower),
		EventTypes: normalizedSet([]string(eventTypes), strings.ToLower),
		Statuses:   normalizedSet([]string(statuses), strings.ToLower),
		Sources:    sourceFilterSet([]string(sources)),
	}
	if hasSince {
		filters.Since = &since
	}
	if hasUntil {
		filters.Until = &until
	}

	filtered, err := filterSearchEvents(records, filters)
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	if len(filtered) > *limit {
		filtered = filtered[:*limit]
	}

	if *format == "json" {
		out := eventListEnvelope{
			Items:       filtered,
			NextCursor:  nil,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := writeJSON(os.Stdout, out); err != nil {
			return &commandError{
				Code: exitOperationalFailure,
				Err:  err,
			}
		}
		return nil
	}

	if *format == "md" {
		var builder strings.Builder
		builder.WriteString("# Search Results\n\n")
		if len(filtered) == 0 {
			builder.WriteString("No matching events.\n")
		}
		for _, rec := range filtered {
			builder.WriteString(fmt.Sprintf("- `%s` %s (importance %.2f, confidence %.2f)\n", rec.EventID, rec.Title, rec.ImportanceScore, rec.ConfidenceScore))
		}
		fmt.Print(builder.String())
		return nil
	}

	for _, rec := range filtered {
		fmt.Printf("%s\t%.2f\t%.2f\t%s\n", rec.EventID, rec.ImportanceScore, rec.ConfidenceScore, rec.Title)
	}

	return nil
}

func runEventGet(ctx context.Context, args []string) error {
	opts, err := parseEventGetOptions(args)
	if err != nil {
		return &commandError{
			Code: exitInvalidArguments,
			Err:  err,
		}
	}

	store, err := openStore(ctx, opts.StateDir)
	if err != nil {
		return err
	}
	defer store.Close()

	record, found, err := store.GetEvent(ctx, opts.EventID)
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}
	if !found {
		return &commandError{
			Code: exitRecordNotFound,
			Err:  fmt.Errorf("event not found: %s", opts.EventID),
		}
	}

	if opts.Format == "json" {
		if err := writeJSON(os.Stdout, record); err != nil {
			return &commandError{
				Code: exitOperationalFailure,
				Err:  err,
			}
		}
		return nil
	}

	if opts.Format == "md" {
		fmt.Print(renderEventMarkdown(record))
		return nil
	}

	fmt.Printf(
		"%s\t%s\tstatus=%s\timportance=%.2f\tconfidence=%.2f\tsources=%d\n",
		record.EventID,
		record.Title,
		record.Status,
		record.ImportanceScore,
		record.ConfidenceScore,
		record.SourceClusterSize,
	)
	return nil
}

type eventGetOptions struct {
	StateDir string
	Format   string
	EventID  string
}

type searchFilters struct {
	Assets     map[string]struct{}
	Topics     map[string]struct{}
	EventTypes map[string]struct{}
	Statuses   map[string]struct{}
	Sources    map[string]struct{}
	Since      *time.Time
	Until      *time.Time
}

func parseEventGetOptions(args []string) (eventGetOptions, error) {
	opts := eventGetOptions{
		StateDir: "state",
		Format:   "json",
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--state-dir":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return eventGetOptions{}, fmt.Errorf("missing value for --state-dir")
			}
			opts.StateDir = args[i]
		case strings.HasPrefix(arg, "--state-dir="):
			value := strings.TrimPrefix(arg, "--state-dir=")
			if strings.TrimSpace(value) == "" {
				return eventGetOptions{}, fmt.Errorf("missing value for --state-dir")
			}
			opts.StateDir = value
		case arg == "--format":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return eventGetOptions{}, fmt.Errorf("missing value for --format")
			}
			opts.Format = args[i]
		case strings.HasPrefix(arg, "--format="):
			value := strings.TrimPrefix(arg, "--format=")
			if strings.TrimSpace(value) == "" {
				return eventGetOptions{}, fmt.Errorf("missing value for --format")
			}
			opts.Format = value
		case strings.HasPrefix(arg, "-"):
			return eventGetOptions{}, fmt.Errorf("unknown flag: %s", arg)
		default:
			if opts.EventID != "" {
				return eventGetOptions{}, fmt.Errorf("usage: sift event get <event_id> [--format json|text|md]")
			}
			opts.EventID = arg
		}
	}

	if opts.EventID == "" {
		return eventGetOptions{}, fmt.Errorf("usage: sift event get <event_id> [--format json|text|md]")
	}

	if opts.Format != "json" && opts.Format != "text" && opts.Format != "md" {
		return eventGetOptions{}, fmt.Errorf("unsupported format %q, expected json, text, or md", opts.Format)
	}

	return opts, nil
}

func parseSinceValue(raw string, now time.Time) (time.Time, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false, nil
	}

	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC(), true, nil
	}

	if strings.HasSuffix(trimmed, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(trimmed, "d"))
		if err != nil || days <= 0 {
			return time.Time{}, false, fmt.Errorf("invalid --since value %q, expected RFC3339 or relative window like 24h/72h/7d", trimmed)
		}
		return now.Add(-time.Duration(days) * 24 * time.Hour), true, nil
	}

	if strings.HasSuffix(trimmed, "h") {
		window, err := time.ParseDuration(trimmed)
		if err != nil || window <= 0 {
			return time.Time{}, false, fmt.Errorf("invalid --since value %q, expected RFC3339 or relative window like 24h/72h/7d", trimmed)
		}
		return now.Add(-window), true, nil
	}

	return time.Time{}, false, fmt.Errorf("invalid --since value %q, expected RFC3339 or relative window like 24h/72h/7d", trimmed)
}

func parseUntilValue(raw string) (time.Time, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false, nil
	}

	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid --until value %q, expected RFC3339 timestamp", trimmed)
	}

	return parsed.UTC(), true, nil
}

func normalizedSet(values []string, normalize func(string) string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		norm := normalize(strings.TrimSpace(value))
		if norm == "" {
			continue
		}
		set[norm] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func sourceFilterSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizeSourceValue(value)
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}

		trimmed := strings.TrimSuffix(normalized, "_rss")
		trimmed = strings.TrimSuffix(trimmed, "_feed")
		trimmed = strings.TrimSuffix(trimmed, "_api")
		trimmed = strings.TrimSuffix(trimmed, "_blog")
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}

	if len(set) == 0 {
		return nil
	}
	return set
}

func normalizeSourceValue(value string) string {
	lowered := strings.ToLower(strings.TrimSpace(value))
	if lowered == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "_", "-", "_")
	return replacer.Replace(lowered)
}

func filterSearchEvents(records []event.Record, filters searchFilters) ([]event.Record, error) {
	filtered := make([]event.Record, 0, len(records))
	for _, rec := range records {
		matches, err := matchesSearchFilters(rec, filters)
		if err != nil {
			return nil, err
		}
		if matches {
			filtered = append(filtered, rec)
		}
	}
	return filtered, nil
}

func matchesSearchFilters(rec event.Record, filters searchFilters) (bool, error) {
	publishedAt, err := time.Parse(time.RFC3339, rec.PublishedAt)
	if err != nil {
		return false, fmt.Errorf("parse event %s published_at %q: %w", rec.EventID, rec.PublishedAt, err)
	}

	if filters.Since != nil && publishedAt.Before(*filters.Since) {
		return false, nil
	}
	if filters.Until != nil && publishedAt.After(*filters.Until) {
		return false, nil
	}

	if len(filters.EventTypes) > 0 {
		if _, ok := filters.EventTypes[strings.ToLower(rec.EventType)]; !ok {
			return false, nil
		}
	}

	if len(filters.Statuses) > 0 {
		if _, ok := filters.Statuses[strings.ToLower(rec.Status)]; !ok {
			return false, nil
		}
	}

	if len(filters.Assets) > 0 && !matchesAnySet(rec.Assets, filters.Assets, strings.ToUpper) {
		return false, nil
	}

	if len(filters.Topics) > 0 && !matchesAnySet(rec.Topics, filters.Topics, strings.ToLower) {
		return false, nil
	}

	if len(filters.Sources) > 0 {
		sourceMatch := false
		for _, article := range rec.SupportingArticles {
			normalizedSource := normalizeSourceValue(article.Source)
			if _, ok := filters.Sources[normalizedSource]; ok {
				sourceMatch = true
				break
			}
		}
		if !sourceMatch {
			return false, nil
		}
	}

	return true, nil
}

func matchesAnySet(values []string, set map[string]struct{}, normalize func(string) string) bool {
	for _, value := range values {
		if _, ok := set[normalize(value)]; ok {
			return true
		}
	}
	return false
}

func renderEventMarkdown(rec event.Record) string {
	var builder strings.Builder
	builder.WriteString("# " + rec.Title + "\n\n")
	builder.WriteString(fmt.Sprintf("- Event ID: `%s`\n", rec.EventID))
	builder.WriteString(fmt.Sprintf("- Status: `%s`\n", rec.Status))
	builder.WriteString(fmt.Sprintf("- Event type: `%s`\n", rec.EventType))
	builder.WriteString(fmt.Sprintf("- Importance: `%.2f`\n", rec.ImportanceScore))
	builder.WriteString(fmt.Sprintf("- Confidence: `%.2f`\n", rec.ConfidenceScore))
	builder.WriteString(fmt.Sprintf("- Published at: `%s`\n", rec.PublishedAt))

	if len(rec.Summary1L) > 0 {
		builder.WriteString("\n## Summary\n\n")
		builder.WriteString(rec.Summary1L + "\n")
	}

	if len(rec.SupportingArticles) > 0 {
		builder.WriteString("\n## Supporting Articles\n\n")
		for _, item := range rec.SupportingArticles {
			builder.WriteString(fmt.Sprintf("- %s (%s) %s\n", item.Source, item.PublishedAt, item.URL))
		}
	}

	return builder.String()
}

func notImplemented(command string) error {
	return &commandError{
		Code: exitOperationalFailure,
		Err:  fmt.Errorf("%s is not implemented yet", command),
	}
}

func openStore(ctx context.Context, stateDir string) (*sqlite.Store, error) {
	store, err := sqlite.OpenStateStore(ctx, stateDir)
	if err != nil {
		return nil, &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	return store, nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}
	return nil
}

func printUsage() {
	lines := []string{
		"Sift v0 CLI (Slice C event retrieval)",
		"",
		"Usage:",
		"  sift <command> [options]",
		"",
		"Commands:",
		"  sync              Load registry, fetch feeds, normalize articles, and cluster events",
		"  sources           Show approved source registry with health fields",
		"  latest            Show latest clustered events",
		"  search            Search events by filters",
		"  event get <id>    Show one event",
		"  digest <scope>    Build a digest for scope and window",
		"  eval clustering   Run clustering precision gate on labeled title pairs",
		"",
		"Examples:",
		"  sift sources --format json",
		"  sift sync --fetch-only",
		"  sift latest --limit 20 --format json",
		"  sift event get <event_id> --format md",
		"  sift digest crypto --window 24h --format md",
		"  sift eval clustering --format json",
	}

	fmt.Println(strings.Join(lines, "\n"))
}
