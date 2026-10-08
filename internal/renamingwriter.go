package internal

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type writeCloserRenamerRemover interface {
	io.WriteCloser
	RenameTo(dest string) error
	Remove() error
}

// TempFile is a temporary file created by a WritableFS.
type TempFile interface {
	io.WriteCloser
	Name() string
}

// WritableFS contains the filesystem operations that RenamingWriter uses to
// write a file. OSFS implements them on the real filesystem, and MemFS
// implements them in memory for tests.
type WritableFS interface {
	CreateTemp(dir, pattern string) (TempFile, error)
	Rename(oldpath, newpath string) error
	Remove(name string) error
}

// OSFS is a WritableFS which uses the real filesystem.
type OSFS struct{}

func (OSFS) CreateTemp(dir, pattern string) (TempFile, error) { return os.CreateTemp(dir, pattern) }
func (OSFS) Rename(oldpath, newpath string) error             { return os.Rename(oldpath, newpath) }
func (OSFS) Remove(name string) error                         { return os.Remove(name) }

// RenamingWriter writes to a temporary file, and then renames it to the final
// destination on close. This is to avoid writing a partial file in case of an
// error.
type RenamingWriter struct {
	dest string
	writeCloserRenamerRemover
}

// Close closes the writer and renames the temporary file to the destination. It
// is used when exiting successfully.
func (rw *RenamingWriter) Close() error {
	if rw.writeCloserRenamerRemover == nil {
		return nil
	}
	defer func() { rw.writeCloserRenamerRemover = nil }()

	slog.Debug("closing writer", "path", rw.dest)

	if err := rw.writeCloserRenamerRemover.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	if errRename := rw.writeCloserRenamerRemover.RenameTo(rw.dest); errRename != nil {
		if errRemove := rw.writeCloserRenamerRemover.Remove(); errRemove != nil {
			return fmt.Errorf("failed to rename temporary file: %w; failed to remove temporary file: %w", errRename, errRemove)
		}

		return fmt.Errorf("failed to rename temporary file: %w", errRename)
	}

	return nil
}

// Abort closes the writer and removes the temporary file. It is used when
// exiting with an error.
func (rw *RenamingWriter) Abort() error {
	if rw.writeCloserRenamerRemover == nil {
		return nil
	}
	defer func() { rw.writeCloserRenamerRemover = nil }()

	slog.Debug("aborting writer", "path", rw.dest)

	if err := rw.writeCloserRenamerRemover.Close(); err != nil {
		slog.Warn("failed to close output file", "error", err)
	}

	if err := rw.writeCloserRenamerRemover.Remove(); err != nil {
		return fmt.Errorf("failed to remove temporary file: %w", err)
	}

	return nil
}

// fileRenamerRemover is a writeCloserRenamerRemover that renames and removes a
// temporary file in a WritableFS.
type fileRenamerRemover struct {
	fsys WritableFS
	TempFile
}

func (f fileRenamerRemover) RenameTo(dest string) error {
	slog.Debug("moving temporary file to final destination", "from", f.Name(), "to", dest)
	return f.fsys.Rename(f.Name(), dest)
}

func (f fileRenamerRemover) Remove() error {
	slog.Debug("removing temporary file", "path", f.Name())
	return f.fsys.Remove(f.Name())
}

// NewRenamingWriter returns a RenamingWriter which writes to a temporary file in
// fsys. The file is renamed to dest on Close and removed on Abort.
func NewRenamingWriter(fsys WritableFS, dest string) (*RenamingWriter, error) {
	dir := filepath.Dir(dest)

	tempFile, err := fsys.CreateTemp(dir, ".policy-bot.*.yml")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	return &RenamingWriter{
		dest: dest,
		writeCloserRenamerRemover: fileRenamerRemover{
			fsys:     fsys,
			TempFile: tempFile,
		},
	}, nil
}
