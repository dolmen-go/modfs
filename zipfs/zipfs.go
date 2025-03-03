package zipfs

import (
	"archive/zip"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// ZipFS implements [io/fs.ReadFileFS], [io/fs.SubFS], and [io/fs.ReadDirFS] interfaces
// for a zip archive. It provides a read-only filesystem interface to access files and
// directories within the zip archive.
type ZipFS struct {
	reader *zip.Reader
	files  map[string]*zip.File // direct file lookup
	dirs   map[string]*dirInfo  // emulated directory entries
}

// NewZipFS creates a new ZipFS instance from an [archive/zip.Reader].
func NewZipFS(r *zip.Reader) *ZipFS {
	z := &ZipFS{
		reader: r,
		files:  make(map[string]*zip.File, len(r.File)),
		dirs: map[string]*dirInfo{
			// Initialize root directory
			".": &dirInfo{
				name:    ".",
				entries: []fs.DirEntry{},
			},
		},
	}
	z.buildIndex()
	return z
}

// buildIndex creates the internal directory structure and file mappings.
func (z *ZipFS) buildIndex() {
	dirModTime := time.Now()
	z.dirs["."].modTime = dirModTime

	for _, f := range z.reader.File {
		isDir := len(f.Name) == 0 || f.Name[len(f.Name)-1] == '/'
		name := path.Clean(f.Name)
		if name == "." {
			continue
		}

		if name[0] == '/' { // Ignore absolute paths
			continue
		}

		var entry fs.DirEntry

		if isDir {
			// Unexpected file content
			// See https://cs.opensource.google/go/go/+/refs/tags/go1.24.0:src/archive/zip/reader.go;l=222
			if f.FileHeader.UncompressedSize64 != 0 {
				continue
			}
			dir := &dirInfo{
				name:    path.Base(name),
				modTime: f.FileInfo().ModTime(),
			}
			z.dirs[name] = dir
			entry = dir
		} else {
			// Add file to direct lookup
			z.files[name] = f

			entry = &fileEntry{file: f}
		}

		// Create entries for all parent directories up to root
		dir := path.Dir(name)
		for {
			parent, exists := z.dirs[dir]
			if exists {
				parent.entries = append(parent.entries, entry)
				break // All parent directories have already been populated
			}

			parent = &dirInfo{
				name:    path.Base(dir),
				modTime: dirModTime,
				entries: []fs.DirEntry{entry},
			}
			z.dirs[dir] = parent

			entry = parent
			dir = path.Dir(dir)
		}
	}
}

// Open implements fs.FS
func (z *ZipFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	name = path.Clean(name)

	// Check if it's a directory
	if dir, ok := z.dirs[name]; ok {
		return &dirReader{info: dir, path: name}, nil
	}

	// Check if it's a file
	if file, ok := z.files[name]; ok {
		return &fileReader{file: file}, nil
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// ReadDir implements [fs.ReadDirFS].
func (z *ZipFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	cleanName := path.Clean(name)
	dir, ok := z.dirs[cleanName]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	entries := append([]fs.DirEntry(nil), dir.entries...)
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries, nil
}

// ReadFile implements fs.ReadFileFS
func (z *ZipFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}

	cleanName := path.Clean(name)
	file, ok := z.files[cleanName]
	if !ok {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}

	rc, err := file.Open()
	if err != nil {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: err}
	}
	defer rc.Close()

	return io.ReadAll(rc)
}

// Sub implements fs.SubFS
func (z *ZipFS) Sub(dir string) (fs.FS, error) {
	if !fs.ValidPath(dir) {
		return nil, &fs.PathError{Op: "sub", Path: dir, Err: fs.ErrInvalid}
	}

	dir = path.Clean(dir)
	if dir == "." {
		return z, nil
	}

	// Check if directory exists
	if _, ok := z.dirs[dir]; !ok {
		return nil, &fs.PathError{Op: "sub", Path: dir, Err: fs.ErrNotExist}
	}

	return &subFS{parent: z, prefix: dir}, nil
}

// subFS implements a sub-filesystem view of a ZipFS
type subFS struct {
	parent *ZipFS
	prefix string
}

func (s *subFS) rebaseAny(v any) {
	if v, ok := v.(interface{ rebase(string) }); ok {
		v.rebase(s.prefix)
	}
}

func rebaseSlice[T any](s *subFS, sl []T) {
	if len(sl) == 0 {
		return
	}
	for _, v := range sl {
		s.rebaseAny(v)
	}
}

func (s *subFS) rebaseError(err error) {
	if e, ok := err.(*fs.PathError); ok {
		e.Path = strings.TrimPrefix(e.Path, s.prefix+"/")
	}
}

func (s *subFS) rebaseReader(f fs.File) {
	if f, ok := f.(interface{ rebase(string) }); ok {
		f.rebase(s.prefix)
	}
}

func (s *subFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	f, err := s.parent.Open(path.Join(s.prefix, name))
	s.rebaseAny(f)     // fix path
	s.rebaseError(err) // fix path
	return f, err
}

func (s *subFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	entries, err := s.parent.ReadDir(path.Join(s.prefix, name))
	rebaseSlice(s, entries) // fix path
	s.rebaseError(err)      // fix path
	return entries, err
}

func (s *subFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	b, err := s.parent.ReadFile(path.Join(s.prefix, name))
	s.rebaseError(err)
	return b, err
}

func (s *subFS) Sub(dir string) (fs.FS, error) {
	if !fs.ValidPath(dir) {
		return nil, &fs.PathError{Op: "sub", Path: dir, Err: fs.ErrInvalid}
	}
	ss, err := s.parent.Sub(path.Join(s.prefix, dir))
	s.rebaseError(err)
	return ss, err
}

type (
	fsFileInfo = fs.FileInfo

	roFileInfo struct {
		fsFileInfo
	}
)

func (rfi roFileInfo) Mode() fs.FileMode {
	// Remove W permissions
	return rfi.fsFileInfo.Mode() &^ 0222
}

func (rfi roFileInfo) Sys() any {
	return nil
}

func (rfi roFileInfo) String() string {
	return fs.FormatFileInfo(rfi)
}

// fileReader implements [fs.File] for zip archive entries.
type fileReader struct {
	file *zip.File
	rc   io.ReadCloser
}

func (f *fileReader) Stat() (fs.FileInfo, error) {
	return roFileInfo{f.file.FileInfo()}, nil
}

func (f *fileReader) Read(b []byte) (int, error) {
	if f.rc == nil {
		var err error
		f.rc, err = f.file.Open()
		if err != nil {
			return 0, err
		}
	}
	return f.rc.Read(b)
}

func (f *fileReader) Close() error {
	if f.rc != nil {
		err := f.rc.Close()
		f.rc = nil
		return err
	}
	return nil
}

// fileEntry implements [fs.DirEntry] for real zip entries.
type fileEntry struct {
	file *zip.File
}

func (i fileEntry) Name() string               { return path.Base(i.file.Name) }
func (i fileEntry) IsDir() bool                { return false }
func (i fileEntry) Type() fs.FileMode          { return i.file.FileInfo().Mode().Type() }
func (i fileEntry) Info() (fs.FileInfo, error) { return roFileInfo{i.file.FileInfo()}, nil }

// dirInfo implements [fs.DirEntry] and [fs.FileInfo] for directories.
type dirInfo struct {
	name    string
	modTime time.Time
	entries []fs.DirEntry
}

func (i *dirInfo) Name() string       { return i.name }
func (i *dirInfo) Size() int64        { return 0 }
func (i *dirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0555 }
func (i *dirInfo) ModTime() time.Time { return i.modTime }
func (i *dirInfo) IsDir() bool        { return true }
func (i *dirInfo) Sys() any           { return nil }

func (i *dirInfo) Type() fs.FileMode          { return fs.ModeDir }
func (i *dirInfo) Info() (fs.FileInfo, error) { return i, nil }

func (i *dirInfo) String() string {
	return fs.FormatFileInfo(i)
}

// dirReader implements [fs.File] and [fs.ReadDirFile] for synthesized directories.
type dirReader struct {
	info *dirInfo
	path string
	pos  int
}

func (d *dirReader) rebase(prefix string) {
	d.path = strings.TrimPrefix(d.path, prefix+"/")
}

func (d *dirReader) Stat() (fs.FileInfo, error) {
	return d.info, nil
}

func (d *dirReader) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.path, Err: fs.ErrInvalid}
}

func (d *dirReader) Close() error {
	d.pos = 0
	return nil
}

// ReadDir reads the contents of the directory and returns a slice of entries.
// If n > 0, ReadDir returns at most n entries. In this case, if ReadDir returns an empty slice,
// it will return an error explaining why. At the end of a directory, the error is io.EOF.
func (d *dirReader) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		// Return all remaining entries
		remaining := d.info.entries[d.pos:]
		d.pos = len(d.info.entries)
		return remaining, nil
	}

	// Return up to n entries
	if d.pos >= len(d.info.entries) {
		return nil, io.EOF
	}

	end := d.pos + n
	if end > len(d.info.entries) {
		end = len(d.info.entries)
	}

	entries := d.info.entries[d.pos:end]
	d.pos = end

	if len(entries) == 0 {
		return nil, io.EOF
	}

	return entries, nil
}
