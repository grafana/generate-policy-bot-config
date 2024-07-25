package main

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

// renamingWriter writes to a temporary file, and then renames it to the final
// destination on close. This is to avoid writing a partial file in case of an
// error. Any type can be used here, as long as it is an io.WriteCloser which
// implements renaming and removing the temporary file, corresponding to our
// `Close()` and `Abort()` methods. This means a fake implementation can be used
// in tests.
type renamingWriter struct {
	dest string
	writeCloserRenamerRemover
}

// Close closes the writer and renames the temporary file to the destination. It
// is used when exiting successfully.
func (rw *renamingWriter) Close() error {
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
func (rw *renamingWriter) Abort() error {
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

// nopRenamerRemover is a writeCloserRenamerRemover that does nothing. It is
// used when the destination is standard output, which does not need to be
// renamed or removed.
type nopRenamerRemover struct {
	io.WriteCloser
}

func (nopRenamerRemover) RenameTo(dest string) error { return nil }
func (nopRenamerRemover) Remove() error              { return nil }

// fileRenamerRemover is a writeCloserRenamerRemover that renames and removes a
// temporary file on the real filesystem. It is used when the destination is a
// file.
type fileRenamerRemover struct {
	*os.File
}

func (f fileRenamerRemover) RenameTo(dest string) error {
	slog.Debug("moving temporary file to final destination", "from", f.Name(), "to", dest)
	return os.Rename(f.Name(), dest)
}

func (f fileRenamerRemover) Remove() error {
	slog.Debug("removing temporary file", "path", f.Name())
	return os.Remove(f.Name())
}

// UnmarshalFlag implements the flag.Value interface for renamingWriter, used
// when parsing the destination from a commandline flag. If the value is "-",
// the writer writes to standard output. Otherwise, it writes to a temporary
// file, which is renamed to the final destination on success and removed on
// error.
func (rw *renamingWriter) UnmarshalFlag(value string) error {
	if value == "-" {
		*rw = renamingWriter{
			dest:                      value,
			writeCloserRenamerRemover: nopRenamerRemover{os.Stdout},
		}
		return nil
	}

	dir := filepath.Dir(value)

	tempFile, err := os.CreateTemp(dir, ".policy-bot.*.yml")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	*rw = renamingWriter{
		dest: value,
		writeCloserRenamerRemover: fileRenamerRemover{
			File: tempFile,
		},
	}

	return nil
}
