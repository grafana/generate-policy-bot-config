package internal

import (
	"path/filepath"
	"strconv"
	"strings"
)

// MemFS is an in-memory WritableFS for tests. Files maps the name of each file
// to its contents. If CreateTempErr or RenameErr is set, the corresponding
// operation fails with that error.
type MemFS struct {
	Files         map[string]string
	CreateTempErr error
	RenameErr     error

	tempFiles int
}

// CreateTemp creates an empty file in dir. Its name is pattern with the "*"
// replaced by a counter, so that tests can predict it.
func (m *MemFS) CreateTemp(dir, pattern string) (TempFile, error) {
	if m.CreateTempErr != nil {
		return nil, m.CreateTempErr
	}

	if m.Files == nil {
		m.Files = map[string]string{}
	}

	m.tempFiles++
	name := filepath.Join(dir, strings.Replace(pattern, "*", strconv.Itoa(m.tempFiles), 1))
	m.Files[name] = ""

	return memFile{fsys: m, name: name}, nil
}

func (m *MemFS) Rename(oldpath, newpath string) error {
	if m.RenameErr != nil {
		return m.RenameErr
	}

	m.Files[newpath] = m.Files[oldpath]
	delete(m.Files, oldpath)

	return nil
}

func (m *MemFS) Remove(name string) error {
	delete(m.Files, name)
	return nil
}

type memFile struct {
	fsys *MemFS
	name string
}

func (f memFile) Write(p []byte) (int, error) {
	f.fsys.Files[f.name] += string(p)
	return len(p), nil
}

func (f memFile) Close() error { return nil }
func (f memFile) Name() string { return f.name }
