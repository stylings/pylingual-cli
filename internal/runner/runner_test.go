package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cold/pylingual-cli/internal/api"
	"github.com/cold/pylingual-cli/internal/job"
)

type fakeClient struct {
	uploadErr error
	pollCount int
	content   string
}

func (f *fakeClient) Upload(ctx context.Context, path string) (*api.UploadResponse, error) {
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	return &api.UploadResponse{Identifier: "abc", Success: true}, nil
}

func (f *fakeClient) Poll(ctx context.Context, identifier string) (*api.ProgressResponse, error) {
	f.pollCount++
	if f.pollCount == 1 {
		return &api.ProgressResponse{Identifier: identifier, Stage: "queued", Success: true}, nil
	}
	return &api.ProgressResponse{Identifier: identifier, Stage: "done", Success: true}, nil
}

func (f *fakeClient) Fetch(ctx context.Context, identifier string) (*api.ViewResponse, error) {
	content := f.content
	if content == "" {
		content = "print(1)\n"
	}
	return &api.ViewResponse{
		Success: true,
		EditorContent: &api.EditorContent{
			FileRawPython: &api.FileRawPython{
				DecompilationSuccessful: true,
				EditorContent:           content,
			},
		},
	}, nil
}

func TestRunnerWritesSuccessfulOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "pkg", "mod.py")
	run := New(&fakeClient{}, Config{Concurrency: 1, PollInterval: time.Millisecond})
	events := run.Start(context.Background(), []job.Job{{
		ID:         0,
		InputPath:  "mod.pyc",
		OutputPath: out,
	}})

	var summary Summary
	for event := range events {
		summary = Summarize(summary, event)
	}
	if summary.Succeeded != 1 || summary.Failed != 0 {
		t.Fatalf("summary = %+v, want one success", summary)
	}
	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(content) != "print(1)\n" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestRunnerEmitsUploadFailure(t *testing.T) {
	run := New(&fakeClient{uploadErr: errors.New("boom")}, Config{Concurrency: 1, PollInterval: time.Millisecond})
	events := run.Start(context.Background(), []job.Job{{
		ID:         0,
		InputPath:  "mod.pyc",
		OutputPath: filepath.Join(t.TempDir(), "mod.py"),
	}})

	var failed bool
	for event := range events {
		if event.Status == StatusFailed {
			failed = true
		}
	}
	if !failed {
		t.Fatal("runner did not emit failure")
	}
}

func TestNormalizePythonSource(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "adds final newline", in: "# return None", want: "# return None\n"},
		{name: "keeps existing final newline", in: "print(1)\n", want: "print(1)\n"},
		{name: "normalizes crlf", in: "a\r\nb\r\n", want: "a\nb\n"},
		{name: "normalizes cr", in: "a\rb", want: "a\nb\n"},
		{name: "keeps empty empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePythonSource(tt.in); got != tt.want {
				t.Fatalf("normalizePythonSource(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRunnerNormalizesOutputLineEndings(t *testing.T) {
	out := filepath.Join(t.TempDir(), "mod.py")
	run := New(&fakeClient{content: "a\r\nb\r# return None"}, Config{Concurrency: 1, PollInterval: time.Millisecond})
	events := run.Start(context.Background(), []job.Job{{
		ID:         0,
		InputPath:  "mod.pyc",
		OutputPath: out,
	}})

	for range events {
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(content) != "a\nb\n# return None\n" {
		t.Fatalf("content = %q", string(content))
	}
}
