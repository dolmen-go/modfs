// Package modfs is a client for the GOPROXY protocol.
//
// Package [github.com/dolmen-go/modfs/httpfs] allows to access the resources on an HTTP server.
//
// The GOPROXY protocol: https://go.dev/ref/mod#goproxy-protocol
package modfs

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/dolmen-go/modfs/zipfs"
)

type ModFS struct {
	fs fs.FS
}

func New(f fs.FS) *ModFS {
	return &ModFS{fs: f}
}

type (
	decoder = *json.Decoder

	jsonFile struct {
		decoder
		close func() error
	}
)

func (jf *jsonFile) Close() error {
	if jf.decoder.More() {
		defer jf.close()
		return fmt.Errorf("more data than expected")
	}
	return jf.close()
}

func (m *ModFS) openJSON(path string) (*jsonFile, error) {
	f, err := m.fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	dec := json.NewDecoder(f)
	if !dec.More() {
		f.Close()
		return nil, fmt.Errorf("%s: JSON expected", path)
	}

	return &jsonFile{
		decoder: dec,
		close:   f.Close,
	}, nil
}

func (m *ModFS) decodeJSON(path string, v any) error {
	f, err := m.openJSON(path)
	if err != nil {
		return err
	}
	defer f.Close()

	err = f.Decode(v)
	if err != nil {
		err = fmt.Errorf("%s: %w", path, err)
	}

	return err
}

func (m *ModFS) OpenModule(path string) (*Module, error) {
	if !fs.ValidPath(path) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	mod := Module{fs: m, Path: path}
	err := mod.decodeJSON("@latest", &mod.Latest)
	if err != nil {
		return nil, err
	}
	return &mod, nil
}

type Module struct {
	fs     *ModFS
	Path   string
	Latest VersionInfo
}

func (m *Module) openJSON(path string) (*jsonFile, error) {
	return m.fs.openJSON(m.Path + "/" + path)
}

func (m *Module) decodeJSON(path string, v any) error {
	return m.fs.decodeJSON(m.Path+"/"+path, v)
}

func (m *Module) ListVersions() ([]*VersionInfo, error) {
	dec, err := m.openJSON("@v/list")
	if err != nil {
		return nil, err
	}
	defer dec.Close()

	var versions []*VersionInfo
	for dec.More() {
		var v VersionInfo
		if err = dec.Decode(&v); err != nil {
			return nil, err // TODO: give more context
		}
	}
	return versions, nil
}

func (m *Module) Version(v string) (*Version, error) {
	if strings.ContainsAny(v, "/\\ \t\r\n\000") {
		return nil, fmt.Errorf("%s: invalid version %q", m.Path, v)
	}

	if v == m.Latest.Version {
		return &Version{
			module:      m,
			VersionInfo: m.Latest,
		}, nil
	}

	ver := Version{
		module: m,
	}
	if err := m.decodeJSON("@v/"+v+".info", &ver.VersionInfo); err != nil {
		return nil, err
	}
	return &ver, nil
}

func (m *Module) VersionLatest() (*Version, error) {
	return m.Version(m.Latest.Version)
}

type VersionInfo struct {
	Version string
	Time    time.Time
}

type Version struct {
	module *Module
	VersionInfo
}

// GoMod returns the content of go.mod.
func (ver *Version) GoMod() ([]byte, error) {
	return fs.ReadFile(ver.module.fs.fs, ver.module.Path+"/@v/"+ver.Version+".mod")
}

type ZipFS interface {
	fs.FS
	fs.ReadFileFS
	// The FS must be closed to free resources.
	Close() error
}

// GoMod returns an [fs.FS] with the content of the module.
//
// The FS must be closed ([io.Closer]) when done.
func (ver *Version) OpenFS() (ZipFS, error) {
	zipPath := ver.module.Path + "/@v/" + ver.Version + ".zip"

	f, err := ver.module.fs.fs.Open(ver.module.Path + "/@v/" + ver.Version + ".zip")
	if err != nil {
		return nil, err
	}

	r, ok := f.(interface {
		io.ReaderAt
		io.Closer
	})
	if !ok { // not Seekable, so download the file and open the local copy
		fi, err := os.CreateTemp("", "modfs_*.zip")
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("%v: %w", zipPath, err)
		}
		_, err = io.Copy(fi, f)
		f.Close()
		if err != nil {
			fi.Close()
			return nil, fmt.Errorf("%v: %w", zipPath, err)
		}
		if _, err = fi.Seek(0, 0); err != nil {
			fi.Close()
			return nil, fmt.Errorf("%v: %w", zipPath, err)
		}
		// Remove the temp file on Close
		r = &struct {
			io.ReaderAt
			closerFunc
		}{
			ReaderAt: fi,
			closerFunc: func() error {
				fi.Close()
				return os.Remove(fi.Name())
			},
		}
	}

	fi, err := f.Stat()
	if err != nil {
		r.Close()
		return nil, &fs.PathError{Op: "stat", Path: zipPath, Err: err}
	}
	if fi.IsDir() {
		r.Close()
		return nil, &fs.PathError{Op: "open", Path: zipPath, Err: fs.ErrInvalid}
	}

	zr, err := zip.NewReader(r, fi.Size())
	if err != nil {
		r.Close()
		return nil, err
	}

	zfs := zipfs.NewZipFS(zr)

	// Hide the "module@version/" prefix of all paths in the zip
	subfs, err := zfs.Sub(ver.module.Path + "@" + ver.Version)
	if err != nil {
		r.Close()
		return nil, &fs.PathError{Op: "zipread", Path: zipPath, Err: err}
	}

	type ffs = interface {
		fs.FS
		fs.ReadFileFS
	}
	return &struct {
		ffs
		io.Closer
	}{subfs.(ffs), r}, nil
}

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}
