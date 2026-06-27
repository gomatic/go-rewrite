package rewrite

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gomatic/go-module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initRepo creates a git repository in dir with an origin remote and one staged
// file, returning the repository directory.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "remote", "add", "origin", "git@github.com:acme/widget.git")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o600))
	gitRun(t, dir, "add", ".")
	return dir
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestOSGitRemote(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	remote, err := OSGit{Dir: FilePath(initRepo(t))}.Remote()

	must.NoError(err)
	want.Equal(module.Remote("git@github.com:acme/widget.git"), remote)
}

func TestOSGitFiles(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	files, err := OSGit{Dir: FilePath(initRepo(t))}.Files()

	must.NoError(err)
	want.Equal([]FilePath{"go.mod"}, files)
}

func TestOSGitErrors(t *testing.T) {
	t.Parallel()
	must := require.New(t)

	// A directory that is not a git repository fails both queries.
	git := OSGit{Dir: FilePath(t.TempDir())}

	_, remoteErr := git.Remote()
	must.ErrorIs(remoteErr, ErrGitCommand)

	_, filesErr := git.Files()
	must.ErrorIs(filesErr, ErrGitCommand)
}

func TestOSFileSystemList(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	lister := func() ([]FilePath, error) { return []FilePath{"a", "b"}, nil }
	files, err := OSFileSystem{Root: ".", Lister: lister}.List()

	must.NoError(err)
	want.Equal([]FilePath{"a", "b"}, files)
}

func TestOSFileSystemReadWrite(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	dir := t.TempDir()
	fs := OSFileSystem{Root: FilePath(dir)}

	must.NoError(fs.Write("note.txt", []byte("hello")))

	data, err := fs.Read("note.txt")
	must.NoError(err)
	want.Equal("hello", string(data))
}

// TestOSFileSystemWritePreservesMode proves Write keeps an existing file's
// permission bits instead of clobbering them — the rename rewriter edits files
// in place and must not reset their modes.
func TestOSFileSystemWritePreservesMode(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	dir := t.TempDir()
	must.NoError(os.WriteFile(filepath.Join(dir, "exec.sh"), []byte("old"), 0o750))

	must.NoError(OSFileSystem{Root: FilePath(dir)}.Write("exec.sh", []byte("new")))

	info, err := os.Stat(filepath.Join(dir, "exec.sh"))
	must.NoError(err)
	want.Equal(os.FileMode(0o750), info.Mode().Perm())
}

func TestOSFileSystemReadMissing(t *testing.T) {
	t.Parallel()
	must := require.New(t)

	_, err := OSFileSystem{Root: FilePath(t.TempDir())}.Read("absent.txt")

	must.ErrorIs(err, ErrOpenFile)
}

func TestOSFileSystemWriteToMissingDir(t *testing.T) {
	t.Parallel()
	must := require.New(t)

	err := OSFileSystem{Root: FilePath(t.TempDir())}.Write("absent-dir/file.txt", []byte("x"))

	must.ErrorIs(err, ErrWriteFile)
}

func TestOSFileSystemMove(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	dir := t.TempDir()
	must.NoError(os.Mkdir(filepath.Join(dir, "from"), 0o750))
	fs := OSFileSystem{Root: FilePath(dir)}

	must.NoError(fs.Move("from", "to"))

	_, err := os.Stat(filepath.Join(dir, "to"))
	want.NoError(err, "destination exists after move")
}

func TestOSFileSystemMoveMissing(t *testing.T) {
	t.Parallel()
	must := require.New(t)

	err := OSFileSystem{Root: FilePath(t.TempDir())}.Move("absent", "dest")

	must.ErrorIs(err, ErrMoveFile)
}

// TestOSFileSystemRootMissing proves that when Root itself cannot be opened as a
// traversal-safe handle (os.OpenRoot fails because the directory does not
// exist), Read surfaces the failure as ErrOpenFile rather than panicking.
func TestOSFileSystemRootMissing(t *testing.T) {
	t.Parallel()
	must := require.New(t)

	missingRoot := filepath.Join(t.TempDir(), "no-such-dir")

	_, err := OSFileSystem{Root: FilePath(missingRoot)}.Read("anything.txt")

	must.ErrorIs(err, ErrOpenFile)
}
