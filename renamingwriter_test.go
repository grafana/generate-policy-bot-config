package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	errFakeWrite  = errors.New("fake write error")
	errFakeClose  = errors.New("fake close error")
	errFakeRemove = errors.New("fake remove error")
)

type errFakeRename struct {
	dest string
}

func (e errFakeRename) Error() string {
	return "fake rename error"
}

// fakeWriteCloserRenamerRemover is a fake implementation of
// writeCloserRenamerRemover which can return errors on-demand and counts the
// number of calls to each of the operations.
type fakeWriteCloserRenamerRemover struct {
	writeCount        int
	closeCount        int
	renameCount       int
	removeCount       int
	writeShouldError  bool
	closeShouldError  bool
	renameShouldError bool
	removeShouldError bool
	content           []byte
}

func (f *fakeWriteCloserRenamerRemover) Write(p []byte) (n int, err error) {
	f.writeCount++
	if f.writeShouldError {
		return 0, errFakeWrite
	}
	f.content = append(f.content, p...)
	return len(p), nil
}

func (f *fakeWriteCloserRenamerRemover) Close() error {
	f.closeCount++
	if f.closeShouldError {
		return errFakeClose
	}
	return nil
}

func (f *fakeWriteCloserRenamerRemover) RenameTo(dest string) error {
	f.renameCount++
	if f.renameShouldError {
		return errFakeRename{dest: dest}
	}
	return nil
}

func (f *fakeWriteCloserRenamerRemover) Remove() error {
	f.removeCount++
	if f.removeShouldError {
		return errFakeRemove
	}
	return nil
}

func TestRenamingWriterStdout(t *testing.T) {
	var rw renamingWriter
	err := rw.UnmarshalFlag("-")
	require.NoError(t, err)
	require.IsType(t, nopRenamerRemover{}, rw.writeCloserRenamerRemover)
	require.Equal(t, os.Stdout, rw.writeCloserRenamerRemover.(nopRenamerRemover).WriteCloser)
}

func TestRenamingWriterFile(t *testing.T) {
	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "output.txt")

	var rw renamingWriter
	err := rw.UnmarshalFlag(destPath)
	require.NoError(t, err)

	_, err = rw.Write([]byte("test content"))
	require.NoError(t, err)

	_, err = os.Stat(destPath)
	require.True(t, os.IsNotExist(err), "destination file should not exist before Close()")

	err = rw.Close()
	require.NoError(t, err)

	content, err := os.ReadFile(destPath)
	require.NoError(t, err)
	require.Equal(t, "test content", string(content))
}

func TestRenamingWriterNonWritableDestination(t *testing.T) {
	tempDir := t.TempDir()
	nonWritableDir := filepath.Join(tempDir, "non-writable")
	err := os.Mkdir(nonWritableDir, 0555) // Read and execute, but not write
	require.NoError(t, err)

	destPath := filepath.Join(nonWritableDir, "output.txt")

	var rw renamingWriter
	err = rw.UnmarshalFlag(destPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create temporary file")
}

func TestRenamingWriterErrorOnWrite(t *testing.T) {
	fake := &fakeWriteCloserRenamerRemover{writeShouldError: true}
	rw := renamingWriter{writeCloserRenamerRemover: fake}

	_, err := rw.Write([]byte("test"))
	require.ErrorIs(t, err, errFakeWrite)
	require.Equal(t, 1, fake.writeCount)
}

func TestRenamingWriterErrorOnClose(t *testing.T) {
	fake := &fakeWriteCloserRenamerRemover{closeShouldError: true}
	rw := renamingWriter{writeCloserRenamerRemover: fake}

	err := rw.Close()
	require.ErrorIs(t, err, errFakeClose)
	require.Equal(t, 1, fake.closeCount)
	require.Equal(t, 0, fake.renameCount)
	require.Equal(t, 0, fake.removeCount)
}

func TestRenamingWriterErrorOnRename(t *testing.T) {
	fake := &fakeWriteCloserRenamerRemover{renameShouldError: true}
	rw := renamingWriter{dest: "foo", writeCloserRenamerRemover: fake}

	err := rw.Close()
	var fakeRenameErr errFakeRename
	require.ErrorAs(t, err, &fakeRenameErr)
	require.Equal(t, "foo", fakeRenameErr.dest)
	require.Equal(t, 1, fake.closeCount)
	require.Equal(t, 1, fake.renameCount)
	// we still tried to remove the temporary file
	require.Equal(t, 1, fake.removeCount)
}

func TestRenamingWriterErrorOnRenameAndRemove(t *testing.T) {
	fake := &fakeWriteCloserRenamerRemover{renameShouldError: true, removeShouldError: true}
	rw := renamingWriter{dest: "dest", writeCloserRenamerRemover: fake}

	err := rw.Close()
	var fakeRenameErr errFakeRename
	require.ErrorAs(t, err, &fakeRenameErr)
	require.Equal(t, "dest", fakeRenameErr.dest)
	require.ErrorIs(t, err, errFakeRemove)
	require.Equal(t, 1, fake.closeCount)
	require.Equal(t, 1, fake.renameCount)
	require.Equal(t, 1, fake.removeCount)
}

func TestRenamingWriterAbort(t *testing.T) {
	fake := &fakeWriteCloserRenamerRemover{}
	rw := renamingWriter{writeCloserRenamerRemover: fake}

	_, err := rw.Write([]byte("test"))
	require.NoError(t, err)

	err = rw.Abort()
	require.NoError(t, err)
	require.Equal(t, 1, fake.writeCount)
	require.Equal(t, 1, fake.closeCount)
	require.Equal(t, 0, fake.renameCount)
	require.Equal(t, 1, fake.removeCount)
}

func TestRenamingWriterAbortIgnoresCloseError(t *testing.T) {
	fake := &fakeWriteCloserRenamerRemover{closeShouldError: true}
	rw := renamingWriter{writeCloserRenamerRemover: fake}

	err := rw.Abort()
	require.NoError(t, err)
	require.Equal(t, 1, fake.closeCount)
	require.Equal(t, 1, fake.removeCount)
}

func TestRenamingWriterAbortErrorOnRemove(t *testing.T) {
	fake := &fakeWriteCloserRenamerRemover{removeShouldError: true}
	rw := renamingWriter{writeCloserRenamerRemover: fake}

	err := rw.Abort()
	require.ErrorIs(t, err, errFakeRemove)
	require.Equal(t, 1, fake.closeCount)
	require.Equal(t, 1, fake.removeCount)
}

func TestRenamingWriterMultipleCalls(t *testing.T) {
	fake := &fakeWriteCloserRenamerRemover{}
	rw := renamingWriter{writeCloserRenamerRemover: fake}

	_, err := rw.Write([]byte("test"))
	require.NoError(t, err)

	err = rw.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = rw.Close()
	require.NoError(t, err)

	// Abort after close should be no-op
	err = rw.Abort()
	require.NoError(t, err)

	require.Equal(t, 1, fake.writeCount)
	require.Equal(t, 1, fake.closeCount)
	require.Equal(t, 1, fake.renameCount)
	require.Equal(t, 0, fake.removeCount)
}
