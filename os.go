package rewrite

import (
	"os"
	"os/exec"
	"strings"

	"github.com/gomatic/go-module"
)

// OSGit implements Git by shelling out to git in a working directory.
type OSGit struct {
	Dir FilePath
}

// Remote returns the origin remote URL via "git remote get-url origin".
func (g OSGit) Remote() (module.Remote, error) {
	out, err := g.run("remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return module.Remote(strings.TrimSpace(string(out))), nil
}

// Files returns the repository's tracked files via "git ls-files".
func (g OSGit) Files() ([]FilePath, error) {
	out, err := g.run("ls-files")
	if err != nil {
		return nil, err
	}
	return lines(string(out)), nil
}

// run executes a git subcommand in g.Dir and returns its standard output.
func (g OSGit) run(args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = string(g.Dir)
	out, err := command.Output()
	if err != nil {
		return nil, ErrGitCommand.With(err, strings.Join(args, " "))
	}
	return out, nil
}

// lines splits trimmed output into non-empty file paths.
func lines(out string) []FilePath {
	var paths []FilePath
	for line := range strings.SplitSeq(out, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			paths = append(paths, FilePath(trimmed))
		}
	}
	return paths
}

// OSFileSystem implements FileSystem over the repository rooted at Root,
// enumerating its files through Lister (typically OSGit.Files).
type OSFileSystem struct {
	Lister func() ([]FilePath, error)
	Root   FilePath
}

// List enumerates the project's files via Lister.
func (o OSFileSystem) List() ([]FilePath, error) {
	return o.Lister()
}

// Read returns the contents of path resolved beneath Root.
func (o OSFileSystem) Read(path FilePath) ([]byte, error) {
	var data []byte
	err := o.withRoot(func(root *os.Root) error {
		var err error
		data, err = root.ReadFile(string(path))
		return err
	})
	if err != nil {
		return nil, ErrOpenFile.With(err, string(path))
	}
	return data, nil
}

// newFilePerm is the restrictive mode used only when writing a file that does
// not already exist; an existing file keeps its own mode (see Write).
const newFilePerm = 0o600

// Write replaces the contents of path resolved beneath Root, preserving the
// file's existing permission bits. A file that does not yet exist is created
// with the restrictive newFilePerm rather than a hardcoded permissive mode.
func (o OSFileSystem) Write(path FilePath, data []byte) error {
	err := o.withRoot(func(root *os.Root) error {
		return writeRoot(root, string(path), data)
	})
	if err != nil {
		return ErrWriteFile.With(err, string(path))
	}
	return nil
}

// writeRoot writes data to name beneath root, keeping an existing file's
// permission bits and creating a new file with the restrictive newFilePerm.
func writeRoot(root *os.Root, name string, data []byte) error {
	perm := os.FileMode(newFilePerm)
	if info, err := root.Stat(name); err == nil {
		perm = info.Mode().Perm()
	}
	return root.WriteFile(name, data, perm)
}

// Move renames a directory from one path to another, both resolved beneath Root.
func (o OSFileSystem) Move(from, to FilePath) error {
	err := o.withRoot(func(root *os.Root) error {
		return root.Rename(string(from), string(to))
	})
	if err != nil {
		return ErrMoveFile.With(err, string(from))
	}
	return nil
}

// withRoot opens Root as a traversal-safe handle and runs fn against it. The
// handle (os.OpenRoot, Go 1.24+) confines every path operation beneath Root: a
// path that would escape the root fails rather than resolving outside it, so the
// adapter cannot be tricked into reading or writing a sibling of the project.
func (o OSFileSystem) withRoot(fn func(*os.Root) error) error {
	root, err := os.OpenRoot(string(o.Root))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return fn(root)
}
