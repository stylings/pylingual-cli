package cli

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestParseHelp(t *testing.T) {
	var stderr bytes.Buffer
	_, err := Parse([]string{"-h"}, &stderr)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err = %v, want ErrHelp", err)
	}
	if stderr.Len() == 0 {
		t.Fatal("help did not write usage")
	}
}

func TestParseValidatesConcurrency(t *testing.T) {
	var stderr bytes.Buffer
	_, err := Parse([]string{"-j", "0", "file.pyc"}, &stderr)
	if err == nil {
		t.Fatal("Parse returned nil error, want validation error")
	}
}

func TestParseAllowsFlagsAfterInputs(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := Parse([]string{".", "-o", "output", "-j", "2"}, &stderr)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.OutDir != "output" {
		t.Fatalf("OutDir = %q, want output", cfg.OutDir)
	}
	if cfg.Concurrency != 2 {
		t.Fatalf("Concurrency = %d, want 2", cfg.Concurrency)
	}
	if len(cfg.Inputs) != 1 || cfg.Inputs[0] != "." {
		t.Fatalf("Inputs = %#v, want dot", cfg.Inputs)
	}
}

func TestParseAllowsInlineFlagValuesAfterInputs(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := Parse([]string{"sample.pyc", "--poll-interval=1s", "-o=out"}, &stderr)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.OutDir != "out" {
		t.Fatalf("OutDir = %q, want out", cfg.OutDir)
	}
	if cfg.PollInterval != time.Second {
		t.Fatalf("PollInterval = %s, want 1s", cfg.PollInterval)
	}
}
