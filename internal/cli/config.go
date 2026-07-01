package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.pylingual.io"

var ErrUsage = errors.New("usage error")

var ErrHelp = errors.New("help requested")

type Config struct {
	BaseURL      string
	Concurrency  int
	Inputs       []string
	OutDir       string
	Plain        bool
	PollInterval time.Duration
	Timeout      time.Duration
}

func Parse(args []string, stderr io.Writer) (Config, error) {
	var cfg Config

	fs := flag.NewFlagSet("pylingual", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&cfg.Concurrency, "j", 4, "max concurrent decompilations")
	fs.StringVar(&cfg.OutDir, "o", ".", "output directory")
	fs.BoolVar(&cfg.Plain, "plain", false, "force line-based output")
	fs.DurationVar(&cfg.PollInterval, "poll-interval", 300*time.Millisecond, "polling interval")
	fs.DurationVar(&cfg.Timeout, "timeout", 60*time.Second, "HTTP request timeout")
	fs.StringVar(&cfg.BaseURL, "base-url", defaultBaseURL, "Pylingual API base URL")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: pylingual [flags] <file.pyc ... | directory ...>\n\nFlags:\n")
		fs.PrintDefaults()
	}

	normalized, err := normalizeArgs(fs, args)
	if err != nil {
		return Config{}, err
	}

	if err := fs.Parse(normalized); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Config{}, ErrHelp
		}
		return Config{}, ErrUsage
	}

	cfg.Inputs = fs.Args()
	if len(cfg.Inputs) == 0 {
		fs.Usage()
		return Config{}, ErrUsage
	}
	if cfg.Concurrency < 1 {
		return Config{}, fmt.Errorf("-j must be at least 1")
	}
	if cfg.PollInterval <= 0 {
		return Config{}, fmt.Errorf("--poll-interval must be positive")
	}
	if cfg.Timeout <= 0 {
		return Config{}, fmt.Errorf("--timeout must be positive")
	}
	if cfg.BaseURL == "" {
		return Config{}, fmt.Errorf("--base-url must not be empty")
	}

	return cfg, nil
}

// boolFlag matches the unexported interface flag.Value implementations use
// to mark themselves as not needing a value (see flag package docs).
type boolFlag interface {
	IsBoolFlag() bool
}

func normalizeArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}

		flags = append(flags, arg)
		name, hasInlineValue := splitFlag(arg)
		if hasInlineValue {
			continue
		}
		f := fs.Lookup(name)
		if f == nil {
			continue // unknown flag; fs.Parse will report the error
		}
		if bf, ok := f.Value.(boolFlag); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("%s requires a value", arg)
		}
		i++
		flags = append(flags, args[i])
	}

	return append(flags, positional...), nil
}

func splitFlag(arg string) (string, bool) {
	trimmed := strings.TrimLeft(arg, "-")
	name, _, ok := strings.Cut(trimmed, "=")
	return name, ok
}
