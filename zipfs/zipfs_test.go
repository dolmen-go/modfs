package zipfs

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
)

var (
	_ = []interface {
		fs.FS
		fs.ReadDirFS
		fs.ReadFileFS
		fs.SubFS
	}{
		(*ZipFS)(nil),
		(*subFS)(nil),
	}
)

func createTestZip() (*zip.Reader, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// Add some test files
	files := map[string]string{
		"hello.txt":        "Hello, World!",
		"dir/file.txt":     "File in directory",
		"dir/subdir/a.txt": "Nested file A",
		"dir/subdir/b.txt": "Nested file B",
		"empty/":           "",
		"other/file2.txt":  "Another file",
	}

	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			return nil, err
		}
		if content != "" {
			if _, err := f.Write([]byte(content)); err != nil {
				return nil, err
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
}

func TestZipFS(t *testing.T) {
	zr, err := createTestZip()
	if err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	zipFS := NewZipFS(zr)

	// Test basic file operations
	t.Run("ReadFile", func(t *testing.T) {
		content, err := zipFS.ReadFile("hello.txt")
		if err != nil {
			t.Errorf("Failed to read file: %v", err)
		}
		if string(content) != "Hello, World!" {
			t.Errorf("Unexpected content: got %q, want %q", string(content), "Hello, World!")
		}
	})

	// Test directory operations using ReadDir interface
	t.Run("ReadDir", func(t *testing.T) {
		entries, err := zipFS.ReadDir("dir")
		if err != nil {
			t.Fatalf("Failed to read directory: %v", err)
		}

		// Check if we have the expected number of entries
		if len(entries) != 2 { // file.txt and subdir
			t.Errorf("Unexpected number of entries: got %d, want 2", len(entries))
		}

		// Verify root directory
		rootEntries, err := zipFS.ReadDir(".")
		if err != nil {
			t.Fatalf("Failed to read root directory: %v", err)
		}
		if len(rootEntries) != 4 { // hello.txt, dir, empty, other
			t.Errorf("Unexpected number of root entries: got %d, want 4", len(rootEntries))
		}
	})

	// Test Sub interface and its additional interfaces
	t.Run("SubFS", func(t *testing.T) {
		subFS, err := zipFS.Sub("dir/subdir")
		if err != nil {
			t.Fatalf("Failed to create sub filesystem: %v", err)
		}

		// Test fs.FS interface
		t.Run("Open", func(t *testing.T) {
			f, err := subFS.Open("a.txt")
			if err != nil {
				t.Fatalf("Failed to open file in sub filesystem: %v", err)
			}
			defer f.Close()

			content, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("Failed to read file in sub filesystem: %v", err)
			}
			if string(content) != "Nested file A" {
				t.Errorf("Unexpected content: got %q, want %q", string(content), "Nested file A")
			}
		})

		// Test fs.ReadFileFS interface
		t.Run("ReadFile", func(t *testing.T) {
			readFS, ok := subFS.(fs.ReadFileFS)
			if !ok {
				t.Fatal("SubFS does not implement fs.ReadFileFS")
			}

			content, err := readFS.ReadFile("b.txt")
			if err != nil {
				t.Fatalf("Failed to read file using ReadFile: %v", err)
			}
			if string(content) != "Nested file B" {
				t.Errorf("Unexpected content: got %q, want %q", string(content), "Nested file B")
			}
		})

		// Test fs.ReadDirFS interface
		t.Run("ReadDir", func(t *testing.T) {
			readDirFS, ok := subFS.(fs.ReadDirFS)
			if !ok {
				t.Fatal("SubFS does not implement fs.ReadDirFS")
			}

			entries, err := readDirFS.ReadDir(".")
			if err != nil {
				t.Fatalf("Failed to read directory: %v", err)
			}
			if len(entries) != 2 { // a.txt and b.txt
				t.Errorf("Unexpected number of entries: got %d, want 2", len(entries))
			}
		})

		// Test fs.SubFS interface
		t.Run("Sub", func(t *testing.T) {
			subFS2, ok := subFS.(fs.SubFS)
			if !ok {
				t.Fatal("SubFS does not implement fs.SubFS")
			}

			// Try to create a sub-filesystem of the sub-filesystem
			_, err := subFS2.Sub(".")
			if err != nil {
				t.Errorf("Failed to create sub-sub filesystem: %v", err)
			}
		})
	})

	// Test error cases
	t.Run("Errors", func(t *testing.T) {
		// Test non-existent file
		_, err := zipFS.Open("nonexistent.txt")
		if err == nil {
			t.Error("Expected error opening non-existent file")
		}

		// Test invalid path
		_, err = zipFS.Open("../invalid")
		if err == nil {
			t.Error("Expected error opening invalid path")
		}

		// Test ReadFile with invalid path
		_, err = zipFS.ReadFile("../invalid")
		if err == nil {
			t.Error("Expected error reading invalid path")
		}

		// Test Sub with invalid path
		_, err = zipFS.Sub("../invalid")
		if err == nil {
			t.Error("Expected error creating sub filesystem with invalid path")
		}

		// Test ReadDir with invalid path
		_, err = zipFS.ReadDir("../invalid")
		if err == nil {
			t.Error("Expected error reading directory with invalid path")
		}

		// Test ReadDir with non-existent directory
		_, err = zipFS.ReadDir("nonexistent")
		if err == nil {
			t.Error("Expected error reading non-existent directory")
		}
	})
}

// TestFSTestSuite runs the standard fs test suite from testing/fstest
func TestFSTestSuite(t *testing.T) {
	zr, err := createTestZip()
	if err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	zipFS := NewZipFS(zr)

	// Run the standard fs test suite
	err = fstest.TestFS(zipFS,
		"hello.txt",
		"dir/file.txt",
		"dir/subdir/a.txt",
		"dir/subdir/b.txt",
		"other/file2.txt",
	)
	if err != nil {
		t.Errorf("fstest.TestFS failed: %v", err)
	}
}

// TestSubFSTestSuite runs the standard fs test suite on a sub-filesystem
func TestSubFSTestSuite(t *testing.T) {
	zr, err := createTestZip()
	if err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	zipFS := NewZipFS(zr)

	// Get sub-filesystem for dir/subdir
	subFS, err := zipFS.Sub("dir/subdir")
	if err != nil {
		t.Fatalf("Failed to create sub filesystem: %v", err)
	}

	// Run the standard fs test suite on the sub-filesystem
	err = fstest.TestFS(subFS,
		"a.txt",
		"b.txt",
	)
	if err != nil {
		t.Errorf("fstest.TestFS failed on sub-filesystem: %v", err)
	}
}
