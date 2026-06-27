package rewrite

import (
	"os"
	"os/exec"
	"path/filepath"
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

// Read returns the contents of path resolved against Root.
func (o OSFileSystem) Read(path FilePath) ([]byte, error) {
	data, err := os.ReadFile(o.resolve(path))
	if err != nil {
		return nil, ErrOpenFile.With(err, string(path))
	}
	return data, nil
}

// newFilePerm is the restrictive mode used only when writing a file that does
// not already exist; an existing file keeps its own mode (see Write).
const newFilePerm = 0o600

// Write replaces the contents of path resolved against Root, preserving the
// file's existing permission bits. A file that does not yet exist is created
// with the restrictive newFilePerm rather than a hardcoded permissive mode.
func (o OSFileSystem) Write(path FilePath, data []byte) error {
	resolved := o.resolve(path)
	perm := os.FileMode(newFilePerm)
	if info, err := os.Stat(resolved); err == nil {
		perm = info.Mode().Perm()
	}
	if err := os.WriteFile(resolved, data, perm); err != nil {
		return ErrWriteFile.With(err, string(path))
	}
	return nil
}

// Move renames a directory from one path to another, both resolved against Root.
func (o OSFileSystem) Move(from, to FilePath) error {
	if err := os.Rename(o.resolve(from), o.resolve(to)); err != nil {
		return ErrMoveFile.With(err, string(from))
	}
	return nil
}

// resolve joins a project-relative path onto Root.
func (o OSFileSystem) resolve(path FilePath) string {
	return filepath.Join(string(o.Root), string(path))
}
