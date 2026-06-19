package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRecursiveDirectoryMirrorsOutput(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "pkg")
	nested := filepath.Join(input, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "mod.pyc"), []byte{1}, 0644); err != nil {
		t.Fatalf("write pyc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "ignore.txt"), []byte("no"), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	out := filepath.Join(root, "out")
	got, err := Discover([]string{input}, out)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got.Jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(got.Jobs))
	}

	wantInput := filepath.Join(nested, "mod.pyc")
	wantOutput := filepath.Join(out, "pkg", "a", "b", "mod.py")
	if got.Jobs[0].InputPath != wantInput {
		t.Fatalf("input = %q, want %q", got.Jobs[0].InputPath, wantInput)
	}
	if got.Jobs[0].OutputPath != wantOutput {
		t.Fatalf("output = %q, want %q", got.Jobs[0].OutputPath, wantOutput)
	}
}

func TestDiscoverWarnsAndDeduplicatesFlatFileOutputs(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	if err := os.MkdirAll(left, 0755); err != nil {
		t.Fatalf("mkdir left: %v", err)
	}
	if err := os.MkdirAll(right, 0755); err != nil {
		t.Fatalf("mkdir right: %v", err)
	}
	leftFile := filepath.Join(left, "same.pyc")
	rightFile := filepath.Join(right, "same.pyc")
	if err := os.WriteFile(leftFile, []byte{1}, 0644); err != nil {
		t.Fatalf("write left: %v", err)
	}
	if err := os.WriteFile(rightFile, []byte{1}, 0644); err != nil {
		t.Fatalf("write right: %v", err)
	}

	out := filepath.Join(root, "out")
	got, err := Discover([]string{leftFile, rightFile, filepath.Join(root, "nope.txt")}, out)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got.Jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(got.Jobs))
	}
	if got.Jobs[0].OutputPath == got.Jobs[1].OutputPath {
		t.Fatalf("outputs should be unique, both were %q", got.Jobs[0].OutputPath)
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(got.Warnings))
	}
}
