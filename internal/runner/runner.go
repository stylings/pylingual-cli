package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cold/pylingual-cli/internal/api"
	"github.com/cold/pylingual-cli/internal/job"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusUploading Status = "uploading"
	StatusPolling   Status = "polling"
	StatusFetching  Status = "fetching"
	StatusWriting   Status = "writing"
	StatusSucceeded Status = "succeeded"
	StatusWarning   Status = "warning"
	StatusFailed    Status = "failed"
)

type Event struct {
	JobID      int
	InputPath  string
	OutputPath string
	Status     Status
	Stage      string
	Err        error
	At         time.Time
}

type Summary struct {
	Total     int
	Succeeded int
	Warnings  int
	Failed    int
}

type Config struct {
	Concurrency  int
	PollInterval time.Duration
}

type Client interface {
	Upload(ctx context.Context, path string) (*api.UploadResponse, error)
	Poll(ctx context.Context, identifier string) (*api.ProgressResponse, error)
	Fetch(ctx context.Context, identifier string) (*api.ViewResponse, error)
}

type Runner struct {
	client Client
	cfg    Config
}

func New(client Client, cfg Config) *Runner {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 300 * time.Millisecond
	}
	return &Runner{client: client, cfg: cfg}
}

func (r *Runner) Start(ctx context.Context, jobs []job.Job) <-chan Event {
	events := make(chan Event, len(jobs)+r.cfg.Concurrency)

	go func() {
		defer close(events)

		work := make(chan job.Job)
		var wg sync.WaitGroup
		for i := 0; i < r.cfg.Concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for planned := range work {
					r.process(ctx, planned, events)
				}
			}()
		}

		for _, planned := range jobs {
			select {
			case <-ctx.Done():
				close(work)
				wg.Wait()
				return
			case work <- planned:
			}
		}
		close(work)
		wg.Wait()
	}()

	return events
}

func (r *Runner) process(ctx context.Context, planned job.Job, events chan<- Event) {
	emit := func(status Status, stage string, err error) bool {
		event := Event{
			JobID:      planned.ID,
			InputPath:  planned.InputPath,
			OutputPath: planned.OutputPath,
			Status:     status,
			Stage:      stage,
			Err:        err,
			At:         time.Now(),
		}
		select {
		case events <- event:
			return true
		case <-ctx.Done():
			return false
		}
	}

	if !emit(StatusUploading, "upload", nil) {
		return
	}
	upload, err := r.client.Upload(ctx, planned.InputPath)
	if err != nil {
		emit(StatusFailed, "upload", fmt.Errorf("upload: %w", err))
		return
	}

	lastStage := ""
	for {
		if !emit(StatusPolling, displayStage(lastStage), nil) {
			return
		}
		progress, err := r.client.Poll(ctx, upload.Identifier)
		if err != nil {
			emit(StatusFailed, displayStage(lastStage), fmt.Errorf("poll: %w", err))
			return
		}
		if progress.Stage != "" && progress.Stage != lastStage {
			lastStage = progress.Stage
			if !emit(StatusPolling, displayStage(lastStage), nil) {
				return
			}
		}
		if progress.Stage == "done" {
			break
		}
		select {
		case <-time.After(r.cfg.PollInterval):
		case <-ctx.Done():
			emit(StatusFailed, displayStage(lastStage), ctx.Err())
			return
		}
	}

	if !emit(StatusFetching, "fetch", nil) {
		return
	}
	view, err := r.client.Fetch(ctx, upload.Identifier)
	if err != nil {
		emit(StatusFailed, "fetch", fmt.Errorf("fetch: %w", err))
		return
	}
	if view.EditorContent == nil || view.EditorContent.FileRawPython == nil {
		emit(StatusFailed, "fetch", fmt.Errorf("unexpected response structure"))
		return
	}

	if !emit(StatusWriting, "write", nil) {
		return
	}
	if err := os.MkdirAll(filepath.Dir(planned.OutputPath), 0755); err != nil {
		emit(StatusFailed, "write", fmt.Errorf("create output directory: %w", err))
		return
	}
	python := view.EditorContent.FileRawPython
	content := normalizePythonSource(python.EditorContent)
	if err := os.WriteFile(planned.OutputPath, []byte(content), 0644); err != nil {
		emit(StatusFailed, "write", fmt.Errorf("write: %w", err))
		return
	}
	if python.DecompilationSuccessful {
		emit(StatusSucceeded, "done", nil)
		return
	}
	emit(StatusWarning, "done with issues", nil)
}

func displayStage(stage string) string {
	if stage == "" {
		return "queued"
	}
	return stage
}

func normalizePythonSource(source string) string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	if source != "" && !strings.HasSuffix(source, "\n") {
		source += "\n"
	}
	return source
}

func Summarize(summary Summary, event Event) Summary {
	switch event.Status {
	case StatusSucceeded:
		summary.Succeeded++
	case StatusWarning:
		summary.Warnings++
	case StatusFailed:
		summary.Failed++
	}
	return summary
}
