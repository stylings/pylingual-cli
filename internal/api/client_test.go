package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUploadRetriesTransientFailure(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload" {
			t.Fatalf("path = %s, want /upload", r.URL.Path)
		}
		attempts++
		if attempts == 1 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		fmt.Fprint(w, `{"identifier":"abc","success":true}`)
	}))
	defer server.Close()

	file := filepath.Join(t.TempDir(), "sample.pyc")
	if err := os.WriteFile(file, []byte{1, 2, 3}, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := NewClient(Config{BaseURL: server.URL, Timeout: 2 * time.Second})
	got, err := client.Upload(context.Background(), file)
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if got.Identifier != "abc" {
		t.Fatalf("identifier = %q, want abc", got.Identifier)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestUploadErrorHandling(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  string
	}{
		{
			name:     "returns failure message without trailing period",
			response: `{"message":"server rejected upload.","success":false}`,
			wantErr:  "server rejected upload",
		},
		{
			name:     "falls back without message",
			response: `{"success":false}`,
			wantErr:  "upload failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				fmt.Fprint(w, tt.response)
			}))
			defer server.Close()

			file := filepath.Join(t.TempDir(), "sample.pyc")
			if err := os.WriteFile(file, []byte{1, 2, 3}, 0644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			client := NewClient(Config{BaseURL: server.URL, Timeout: 2 * time.Second})
			_, err := client.Upload(context.Background(), file)
			if err == nil {
				t.Fatal("Upload returned nil error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("error = %q, want %q", err, tt.wantErr)
			}
			if attempts != 1 {
				t.Fatalf("attempts = %d, want 1", attempts)
			}
		})
	}
}

func TestPollRejectsNonTransientStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Timeout: 2 * time.Second})
	if _, err := client.Poll(context.Background(), "abc"); err == nil {
		t.Fatal("Poll returned nil error, want status error")
	}
}

func TestFetchDecodesViewResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"success":true,"editor_content":{"file_raw_python":{"decompilation_successful":true,"editor_content":"print(1)\n"}}}`)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Timeout: 2 * time.Second})
	got, err := client.Fetch(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if got.EditorContent.FileRawPython.EditorContent != "print(1)\n" {
		t.Fatalf("content = %q", got.EditorContent.FileRawPython.EditorContent)
	}
}
