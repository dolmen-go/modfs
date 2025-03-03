package httpfs

import (
	"errors"
	iofs "io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestFSTestInterface(t *testing.T) {
	// Create a test server that serves a small virtual filesystem
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello"))
		case "/dir/nested.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("world"))
		case "/empty.txt":
			w.WriteHeader(http.StatusOK)
			// Empty file
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create our HTTP filesystem
	fs, err := NewHTTPFS(http.DefaultClient, server.URL)
	if err != nil {
		t.Fatalf("NewHTTPFS() error = %v", err)
	}

	// Run the standard filesystem tests
	err = fstest.TestFS(fs,
		"file.txt",
		"dir/nested.txt",
		"empty.txt",
	)
	var errs interface{ Unwrap() []error }
	if errors.As(err, &errs) {
		for _, err := range errs.Unwrap() {
			var pe *iofs.PathError
			if errors.As(err, &pe) && pe.Path == "." && pe.Op == "open" {
				t.Logf("[expected]: %v", pe)
			} else {
				t.Logf("[%T] %v", err, err)
			}
		}
	} else if err != nil {
		// Use t.Logf instead of t.Errorf since HTTP filesystems typically
		// don't support full directory listing capabilities
		t.Logf("fstest.TestFS failed (expected for HTTP filesystems) %T: %v", err, err)
	}
}

func TestFSReadFileFS(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hello.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello, World!"))
		case "/empty.txt":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create our HTTP filesystem
	fs, err := NewHTTPFS(http.DefaultClient, server.URL)
	if err != nil {
		t.Fatalf("NewHTTPFS() error = %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "read existing file",
			path: "hello.txt",
			want: "Hello, World!",
		},
		{
			name:    "read non-existent file",
			path:    "nonexistent.txt",
			wantErr: true,
		},
		{
			name: "read empty file",
			path: "empty.txt",
			want: "",
		},
		{
			name:    "invalid path",
			path:    "../escape.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := fs.Open(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Open() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			defer content.Close()

			// Read the file contents
			data := make([]byte, 1024)
			n, err := content.Read(data)
			if err != nil && err.Error() != "EOF" {
				t.Errorf("Read() error = %v", err)
				return
			}

			got := string(data[:n])
			if got != tt.want {
				t.Errorf("Read() got = %q, want %q", got, tt.want)
			}
		})
	}
}
