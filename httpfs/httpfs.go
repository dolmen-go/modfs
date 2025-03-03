// Package httpfs exposes the resources of an HTTP server as an [io/fs.FS].
package httpfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"time"
)

// HTTPFS implements an [io/fs.FS] that accesses remote resources via HTTP.
type HTTPFS struct {
	client *http.Client
	base   *url.URL
}

// NewHTTPFS creates a new filesystem that accesses resources via HTTP.
// The baseURL parameter specifies the root of the remote filesystem.
func NewHTTPFS(client *http.Client, baseURL string) (*HTTPFS, error) {
	if client == nil {
		panic(errors.New("client cannot be nil"))
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	if base.Fragment != "" {
		return nil, errors.New("invalid base URL: no fragment allowed")
	}

	return &HTTPFS{
		client: client,
		base:   base,
	}, nil
}

// Open implements [fs.FS].
func (h *HTTPFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	name = path.Clean(name)
	if name == "." {
		// return unreadableDir("."), nil
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}

	// Construct the full URL
	fullURL := *h.base
	fullURL.Path = path.Join(fullURL.Path, name)

	// Make the request
	resp, err := h.client.Get(fullURL.String())
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &fs.PathError{Op: "open", Path: name, Err: fmt.Errorf("HTTP status %d", resp.StatusCode)}
	}

	return &httpFile{
		reader: resp.Body,
		size:   resp.ContentLength,
		name:   path.Base(name),
	}, nil
}

// unreadableDir implements [fs.File] and [fs.ReadDirFile] but denies reading entries.
type unreadableDir string

func (ud unreadableDir) Stat() (fs.FileInfo, error) {
	return nil, &fs.PathError{Op: "stat", Path: string(ud), Err: fs.ErrPermission}
}

func (ud unreadableDir) Read(b []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: string(ud), Err: fs.ErrInvalid}
}

func (unreadableDir) Close() error { return nil }

func (ud unreadableDir) ReadDir(n int) ([]fs.DirEntry, error) {
	return nil, &fs.PathError{Op: "readdir", Path: string(ud), Err: fs.ErrPermission}
}

type httpFile struct {
	reader io.ReadCloser
	size   int64
	name   string
	offset int64
}

func (f *httpFile) Read(b []byte) (int, error) {
	return f.reader.Read(b)
}

func (f *httpFile) Close() error {
	return f.reader.Close()
}

func (f *httpFile) Stat() (fs.FileInfo, error) {
	return &httpFileInfo{
		name: f.name,
		size: f.size,
	}, nil
}

type httpFileInfo struct {
	name string
	size int64
}

func (fi *httpFileInfo) Name() string       { return fi.name }
func (fi *httpFileInfo) Size() int64        { return fi.size }
func (fi *httpFileInfo) Mode() fs.FileMode  { return 0444 } // read-only
func (fi *httpFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *httpFileInfo) IsDir() bool        { return false }
func (fi *httpFileInfo) Sys() interface{}   { return nil }
