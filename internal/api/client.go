package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAttempts = 4
	defaultBackoff  = 300 * time.Millisecond
)

type Config struct {
	BaseURL string
	Timeout time.Duration
}

type Client struct {
	baseURL string
	headers map[string]string
	http    *http.Client
}

type UploadResponse struct {
	Identifier string `json:"identifier"`
	Message    string `json:"message"`
	Success    bool   `json:"success"`
}

type ProgressResponse struct {
	Identifier string `json:"identifier"`
	Stage      string `json:"stage"`
	Success    bool   `json:"success"`
}

type ViewResponse struct {
	Success       bool           `json:"success"`
	EditorContent *EditorContent `json:"editor_content"`
}

type EditorContent struct {
	FileRawPython *FileRawPython `json:"file_raw_python"`
}

type FileRawPython struct {
	DecompilationSuccessful bool   `json:"decompilation_successful"`
	EditorContent           string `json:"editor_content"`
}

type statusError struct {
	code int
	body string
}

func (e statusError) Error() string {
	if e.body == "" {
		return fmt.Sprintf("HTTP %d", e.code)
	}
	return fmt.Sprintf("HTTP %d: %s", e.code, e.body)
}

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		headers: map[string]string{
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:151.0) Gecko/20100101 Firefox/151.0",
			"Accept":          "*/*",
			"Accept-Language": "en-US",
			"Referer":         "https://www.pylingual.io/",
			"Origin":          "https://www.pylingual.io",
			"DNT":             "1",
			"Sec-GPC":         "1",
			"Connection":      "keep-alive",
			"Sec-Fetch-Dest":  "empty",
			"Sec-Fetch-Mode":  "cors",
			"Sec-Fetch-Site":  "same-site",
			"Priority":        "u=4",
			"TE":              "trailers",
		},
		http: &http.Client{Timeout: timeout},
	}
}

func (c *Client) Upload(ctx context.Context, path string) (*UploadResponse, error) {
	var out *UploadResponse
	err := retry(ctx, func() error {
		resp, err := c.uploadOnce(ctx, path)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	return out, err
}

func (c *Client) Poll(ctx context.Context, identifier string) (*ProgressResponse, error) {
	var out *ProgressResponse
	err := retry(ctx, func() error {
		reqURL := fmt.Sprintf("%s/get_progress?identifier=%s", c.baseURL, url.QueryEscape(identifier))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return err
		}
		c.applyHeaders(req)
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if err := checkStatus(resp); err != nil {
			return err
		}
		var decoded ProgressResponse
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			return err
		}
		out = &decoded
		return nil
	})
	return out, err
}

func (c *Client) Fetch(ctx context.Context, identifier string) (*ViewResponse, error) {
	var out *ViewResponse
	err := retry(ctx, func() error {
		reqURL := fmt.Sprintf("%s/view_chimera?identifier=%s", c.baseURL, url.QueryEscape(identifier))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return err
		}
		c.applyHeaders(req)
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if err := checkStatus(resp); err != nil {
			return err
		}
		var decoded ViewResponse
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			return err
		}
		out = &decoded
		return nil
	})
	return out, err
}

func (c *Client) uploadOnce(ctx context.Context, path string) (*UploadResponse, error) {
	filename := filepath.Base(path)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := io.Copy(fileWriter, file); err != nil {
		return nil, err
	}
	if err := writer.WriteField("fileName", filename); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/upload", &buf)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var decoded UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if !decoded.Success {
		if decoded.Message != "" {
			message := strings.TrimSuffix(decoded.Message, ".")
			if message != "" {
				return nil, errors.New(message)
			}
		}
		return nil, fmt.Errorf("upload failed")
	}
	if decoded.Identifier == "" {
		return nil, fmt.Errorf("upload response missing identifier")
	}
	return &decoded, nil
}

func (c *Client) applyHeaders(req *http.Request) {
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return statusError{code: resp.StatusCode, body: strings.TrimSpace(string(body))}
}

func retry(ctx context.Context, fn func() error) error {
	var last error
	for attempt := 1; attempt <= defaultAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		last = err
		if !isTransient(err) || attempt == defaultAttempts {
			return err
		}

		delay := defaultBackoff * time.Duration(1<<(attempt-1))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		}
	}
	return last
}

func isTransient(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var status statusError
	if errors.As(err, &status) {
		return status.code == http.StatusTooManyRequests || status.code >= 500
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
