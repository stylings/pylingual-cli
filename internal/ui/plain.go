package ui

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/cold/pylingual-cli/internal/job"
	"github.com/cold/pylingual-cli/internal/runner"
)

// RunPlain renders line-based progress for non-interactive output.
func RunPlain(out io.Writer, jobs []job.Job, events <-chan runner.Event) runner.Summary {
	summary := runner.Summary{Total: len(jobs)}
	lastStage := make(map[int]string, len(jobs))

	for event := range events {
		switch event.Status {
		case runner.StatusUploading:
			fmt.Fprintf(out, "[%d/%d] upload %s\n", event.JobID+1, len(jobs), event.InputPath)
		case runner.StatusPolling:
			if event.Stage != lastStage[event.JobID] {
				lastStage[event.JobID] = event.Stage
				fmt.Fprintf(out, "[%d/%d] %s %s\n", event.JobID+1, len(jobs), event.Stage, event.InputPath)
			}
		case runner.StatusFetching, runner.StatusWriting:
			fmt.Fprintf(out, "[%d/%d] %s %s\n", event.JobID+1, len(jobs), event.Stage, event.InputPath)
		case runner.StatusSucceeded:
			summary = runner.Summarize(summary, event)
			fmt.Fprintf(out, "[%d/%d] ok %s -> %s\n", event.JobID+1, len(jobs), event.InputPath, filepath.Clean(event.OutputPath))
		case runner.StatusWarning:
			summary = runner.Summarize(summary, event)
			fmt.Fprintf(out, "[%d/%d] warn %s -> %s (decompilation reported issues)\n", event.JobID+1, len(jobs), event.InputPath, filepath.Clean(event.OutputPath))
		case runner.StatusFailed:
			summary = runner.Summarize(summary, event)
			fmt.Fprintf(out, "[%d/%d] fail %s: %v\n", event.JobID+1, len(jobs), event.InputPath, event.Err)
		}
	}

	fmt.Fprintf(out, "done: %d ok, %d warnings, %d failed\n", summary.Succeeded, summary.Warnings, summary.Failed)
	return summary
}
