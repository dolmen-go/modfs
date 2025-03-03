package httpfs

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPFS(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/test.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello world"))
		case "/notfound":
			w.WriteHeader(http.StatusNotFound)
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tests := []struct {
		name    string
		path    string
		wantErr bool
		want    string
	}{
		{
			name:    "existing file",
			path:    "test.txt",
			wantErr: false,
			want:    "hello world",
		},
		{
			name:    "not found",
			path:    "notfound",
			wantErr: true,
		},
		{
			name:    "server error",
			path:    "error",
			wantErr: true,
		},
		{
			name:    "invalid path",
			path:    "../test.txt",
			wantErr: true,
		},
	}

	fs, err := NewHTTPFS(http.DefaultClient, server.URL)
	if err != nil {
		t.Fatalf("NewHTTPFS() error = %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := fs.Open(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Open() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			defer f.Close()

			// Read the content
			content, err := io.ReadAll(f)
			if err != nil {
				t.Errorf("ReadAll() error = %v", err)
				return
			}

			if string(content) != tt.want {
				t.Errorf("content = %v, want %v", string(content), tt.want)
			}

			// Test Stat()
			info, err := f.Stat()
			if err != nil {
				t.Errorf("Stat() error = %v", err)
				return
			}

			if info.Name() != tt.path {
				t.Errorf("Name() = %v, want %v", info.Name(), tt.path)
			}
			if info.IsDir() {
				t.Error("IsDir() = true, want false")
			}
		})
	}
}

func TestNewHTTPFS_Validation(t *testing.T) {
	tests := []struct {
		name    string
		client  *http.Client
		baseURL string
		wantErr bool
	}{
		{
			name:    "valid input",
			client:  http.DefaultClient,
			baseURL: "http://example.com",
			wantErr: false,
		},
		{
			name:    "invalid URL",
			client:  http.DefaultClient,
			baseURL: "://invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewHTTPFS(tt.client, tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHTTPFS() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
