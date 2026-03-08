package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	digestproj "sift/internal/digest"
)

type digestOptions struct {
	StateDir  string
	OutputDir string
	Scope     string
	Window    string
	Format    string
}

func runDigest(ctx context.Context, args []string) error {
	opts, err := parseDigestOptions(args)
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

	records, err := store.ListEvents(ctx)
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	projection, err := digestproj.BuildProjection(records, opts.OutputDir, opts.Scope, opts.Window, time.Now().UTC())
	if err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	if err := digestproj.PublishProjection(projection); err != nil {
		return &commandError{
			Code: exitOperationalFailure,
			Err:  err,
		}
	}

	switch opts.Format {
	case "json":
		if err := writeJSON(os.Stdout, projection.Envelope); err != nil {
			return &commandError{
				Code: exitOperationalFailure,
				Err:  err,
			}
		}
	case "md":
		fmt.Print(projection.Markdown)
	case "text":
		fmt.Printf(
			"scope=%s window=%s generated_at=%s event_count=%d markdown_path=%s\n",
			projection.Envelope.Scope,
			projection.Envelope.Window,
			projection.Envelope.GeneratedAt,
			len(projection.Envelope.EventIDs),
			projection.Envelope.MarkdownPath,
		)
	default:
		return &commandError{
			Code: exitInvalidArguments,
			Err:  fmt.Errorf("unsupported format %q, expected json, text, or md", opts.Format),
		}
	}

	return nil
}

func parseDigestOptions(args []string) (digestOptions, error) {
	opts := digestOptions{
		StateDir:  "state",
		OutputDir: "output",
		Window:    "24h",
		Format:    "json",
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--state-dir":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return digestOptions{}, fmt.Errorf("missing value for --state-dir")
			}
			opts.StateDir = args[i]
		case strings.HasPrefix(arg, "--state-dir="):
			value := strings.TrimPrefix(arg, "--state-dir=")
			if strings.TrimSpace(value) == "" {
				return digestOptions{}, fmt.Errorf("missing value for --state-dir")
			}
			opts.StateDir = value
		case arg == "--output-dir":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return digestOptions{}, fmt.Errorf("missing value for --output-dir")
			}
			opts.OutputDir = args[i]
		case strings.HasPrefix(arg, "--output-dir="):
			value := strings.TrimPrefix(arg, "--output-dir=")
			if strings.TrimSpace(value) == "" {
				return digestOptions{}, fmt.Errorf("missing value for --output-dir")
			}
			opts.OutputDir = value
		case arg == "--window":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return digestOptions{}, fmt.Errorf("missing value for --window")
			}
			opts.Window = args[i]
		case strings.HasPrefix(arg, "--window="):
			value := strings.TrimPrefix(arg, "--window=")
			if strings.TrimSpace(value) == "" {
				return digestOptions{}, fmt.Errorf("missing value for --window")
			}
			opts.Window = value
		case arg == "--format":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return digestOptions{}, fmt.Errorf("missing value for --format")
			}
			opts.Format = args[i]
		case strings.HasPrefix(arg, "--format="):
			value := strings.TrimPrefix(arg, "--format=")
			if strings.TrimSpace(value) == "" {
				return digestOptions{}, fmt.Errorf("missing value for --format")
			}
			opts.Format = value
		case strings.HasPrefix(arg, "-"):
			return digestOptions{}, fmt.Errorf("unknown flag: %s", arg)
		default:
			if opts.Scope != "" {
				return digestOptions{}, fmt.Errorf("usage: sift digest <scope> [--window 24h] [--format json|text|md]")
			}
			opts.Scope = strings.ToLower(strings.TrimSpace(arg))
		}
	}

	if opts.Scope == "" {
		return digestOptions{}, fmt.Errorf("usage: sift digest <scope> [--window 24h] [--format json|text|md]")
	}

	if opts.Format != "json" && opts.Format != "text" && opts.Format != "md" {
		return digestOptions{}, fmt.Errorf("unsupported format %q, expected json, text, or md", opts.Format)
	}

	return opts, nil
}
