package files

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cold/pylingual-cli/internal/runner"
)

type Discovery struct {
	Jobs     []runner.Job
	Warnings []string
}

func Discover(inputs []string, outDir string) (Discovery, error) {
	var result Discovery
	usedOutputs := make(map[string]int)

	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skipping %s: %v", input, err))
			continue
		}

		if info.IsDir() {
			jobs, warnings, err := discoverDir(input, outDir)
			if err != nil {
				return Discovery{}, err
			}
			result.Warnings = append(result.Warnings, warnings...)
			for _, planned := range jobs {
				planned.OutputPath = uniqueOutputPath(planned.OutputPath, usedOutputs)
				result.Jobs = append(result.Jobs, planned)
			}
			continue
		}

		if !isPYC(input) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skipping %s: not a .pyc file", input))
			continue
		}
		output := filepath.Join(outDir, pyOutputName(filepath.Base(input)))
		output = uniqueOutputPath(output, usedOutputs)
		result.Jobs = append(result.Jobs, runner.Job{InputPath: input, OutputPath: output})
	}

	sort.Slice(result.Jobs, func(i, j int) bool {
		return result.Jobs[i].InputPath < result.Jobs[j].InputPath
	})
	for i := range result.Jobs {
		result.Jobs[i].ID = i
	}

	return result, nil
}

func discoverDir(input string, outDir string) ([]runner.Job, []string, error) {
	root := filepath.Clean(input)
	rootLabel := filepath.Base(root)
	if rootLabel == "." || rootLabel == string(filepath.Separator) {
		rootLabel = ""
	}

	var jobs []runner.Job
	var warnings []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping %s: %v", path, err))
			return nil
		}
		if entry.IsDir() || !isPYC(entry.Name()) {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("calculate relative path for %s: %w", path, err)
		}
		outputRel := strings.TrimSuffix(rel, filepath.Ext(rel)) + ".py"
		output := filepath.Join(outDir, rootLabel, outputRel)
		jobs = append(jobs, runner.Job{InputPath: path, OutputPath: output})
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk %s: %w", input, err)
	}
	return jobs, warnings, nil
}

func isPYC(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".pyc")
}

func pyOutputName(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name)) + ".py"
}

func uniqueOutputPath(path string, used map[string]int) string {
	if count := used[path]; count > 0 {
		used[path] = count + 1
		ext := filepath.Ext(path)
		stem := strings.TrimSuffix(path, ext)
		return fmt.Sprintf("%s__%d%s", stem, count+1, ext)
	}
	used[path] = 1
	return path
}
