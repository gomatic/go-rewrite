package rewrite

import (
	"errors"
	"testing"

	"github.com/gomatic/go-module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReadFailed is an arbitrary error injected by the "list fails" cases to prove
// the engine propagates a FileSystem.List failure unchanged.
var errReadFailed = errors.New("read failed")

// The fixtures use a neutral before.cli -> after.cli identity that shares no
// substring with this project's own identity, so the rename command never
// rewrites these tests when it renames the project that ships them.

const targetRemote = module.Remote("git@example.com:org/after.cli.git")

// fakeFS is an in-memory FileSystem for engine tests.
type fakeFS struct {
	files    []FilePath
	data     map[FilePath][]byte
	listErr  error
	readErr  map[FilePath]error
	writeErr error
	moveErr  error
	moved    [][2]FilePath
}

func (f *fakeFS) List() ([]FilePath, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.files, nil
}

func (f *fakeFS) Read(path FilePath) ([]byte, error) {
	if err := f.readErr[path]; err != nil {
		return nil, err
	}
	return f.data[path], nil
}

func (f *fakeFS) Write(path FilePath, data []byte) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.data[path] = data
	return nil
}

func (f *fakeFS) Move(from, to FilePath) error {
	if f.moveErr != nil {
		return f.moveErr
	}
	f.moved = append(f.moved, [2]FilePath{from, to})
	return nil
}

// fakeGit is an in-memory Git for engine tests.
type fakeGit struct {
	err    error
	remote module.Remote
}

func (g fakeGit) Remote() (module.Remote, error) { return g.remote, g.err }

func sourceFS() *fakeFS {
	return &fakeFS{
		files: []FilePath{"go.mod", "project.go", "README.md", "cmd/before.cli/main.go"},
		data: map[FilePath][]byte{
			"go.mod":                 []byte("module example.com/org/before.cli\n"),
			"project.go":             []byte("package beforecli\n"),
			"README.md":              []byte("no identity tokens here\n"),
			"cmd/before.cli/main.go": []byte("const env = \"BEFORE_CLI\"\nname = \"before.cli\"\n"),
		},
		readErr: map[FilePath]error{},
	}
}

func sourceIdentity() Identity {
	return Identity{Module: "example.com/org/before.cli", Name: "before.cli"}
}

func targetIdentityValue() Identity {
	return Identity{Module: "example.com/org/after.cli", Name: "after.cli"}
}

func TestDiscover(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	current, target, err := Discover(fakeGit{remote: targetRemote}, sourceFS(), "")

	must.NoError(err)
	want.Equal(module.Path("example.com/org/before.cli"), current.Module)
	want.Equal(module.Name("before.cli"), current.Name)
	want.Equal(module.Path("example.com/org/after.cli"), target.Module)
	want.Equal(module.Name("after.cli"), target.Name)
}

func TestDiscoverOverrideName(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	_, target, err := Discover(fakeGit{remote: targetRemote}, sourceFS(), "mytool")

	must.NoError(err)
	want.Equal(module.Path("example.com/org/after.cli"), target.Module)
	want.Equal(module.Name("mytool"), target.Name)
}

func TestDiscoverErrors(t *testing.T) {
	t.Parallel()

	noModule := sourceFS()
	noModule.data["go.mod"] = []byte("// no directive\n")

	noCmd := sourceFS()
	noCmd.files = []FilePath{"go.mod", "project.go"}

	readErr := sourceFS()
	readErr.readErr["go.mod"] = ErrOpenFile

	listErr := sourceFS()
	listErr.listErr = errReadFailed

	tests := []struct {
		git     fakeGit
		wantErr error
		fs      *fakeFS
		name    string
	}{
		{name: "go.mod read fails", git: fakeGit{remote: targetRemote}, fs: readErr, wantErr: ErrOpenFile},
		{name: "go.mod has no module", git: fakeGit{remote: targetRemote}, fs: noModule, wantErr: ErrNotFound},
		{name: "list fails", git: fakeGit{remote: targetRemote}, fs: listErr, wantErr: errReadFailed},
		{name: "no cmd directory", git: fakeGit{remote: targetRemote}, fs: noCmd, wantErr: ErrNotFound},
		{name: "remote fails", git: fakeGit{err: ErrGitCommand}, fs: sourceFS(), wantErr: ErrGitCommand},
		{name: "remote invalid", git: fakeGit{remote: "not-a-remote"}, fs: sourceFS(), wantErr: module.ErrInvalidRemote},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := Discover(tt.git, tt.fs, "")
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestBuildPlan(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	plan := BuildPlan(sourceIdentity(), targetIdentityValue())

	want.Equal(FilePath("cmd/before.cli"), plan.MoveFrom)
	want.Equal(FilePath("cmd/after.cli"), plan.MoveTo)
	want.Equal([]Replacement{
		{From: "example.com/org/before.cli", To: "example.com/org/after.cli"},
		{From: "BEFORE_CLI", To: "AFTER_CLI"},
		{From: "beforecli", To: "aftercli"},
		{From: "before.cli", To: "after.cli"},
	}, plan.Replacements)
}

func TestApply(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	fs := sourceFS()

	changed, err := BuildPlan(sourceIdentity(), targetIdentityValue()).Apply(fs, false)

	must.NoError(err)
	want.ElementsMatch([]FilePath{"go.mod", "project.go", "cmd/before.cli/main.go"}, changed)
	want.Equal("module example.com/org/after.cli\n", string(fs.data["go.mod"]))
	want.Equal("package aftercli\n", string(fs.data["project.go"]))
	want.Equal("const env = \"AFTER_CLI\"\nname = \"after.cli\"\n", string(fs.data["cmd/before.cli/main.go"]))
	want.Equal("no identity tokens here\n", string(fs.data["README.md"]))
	want.Equal([][2]FilePath{{"cmd/before.cli", "cmd/after.cli"}}, fs.moved)
}

func TestApplyDryRun(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	fs := sourceFS()

	changed, err := BuildPlan(sourceIdentity(), targetIdentityValue()).Apply(fs, true)

	must.NoError(err)
	want.ElementsMatch([]FilePath{"go.mod", "project.go", "cmd/before.cli/main.go"}, changed)
	want.Equal("module example.com/org/before.cli\n", string(fs.data["go.mod"]), "dry run must not write")
	want.Empty(fs.moved, "dry run must not move")
}

func TestApplyNoMoveWhenNamesMatch(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	fs := sourceFS()
	identity := sourceIdentity()

	_, err := BuildPlan(identity, identity).Apply(fs, false)

	must.NoError(err)
	want.Empty(fs.moved, "identical identity must not move")
}

func TestApplyErrors(t *testing.T) {
	t.Parallel()

	plan := BuildPlan(sourceIdentity(), targetIdentityValue())

	listErr := sourceFS()
	listErr.listErr = errReadFailed

	readErr := sourceFS()
	readErr.readErr["go.mod"] = ErrOpenFile

	writeErr := sourceFS()
	writeErr.writeErr = ErrWriteFile

	moveErr := sourceFS()
	moveErr.moveErr = ErrMoveFile

	tests := []struct {
		wantErr error
		fs      *fakeFS
		name    string
	}{
		{name: "list fails", fs: listErr, wantErr: errReadFailed},
		{name: "read fails", fs: readErr, wantErr: ErrOpenFile},
		{name: "write fails", fs: writeErr, wantErr: ErrWriteFile},
		{name: "move fails", fs: moveErr, wantErr: ErrMoveFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := plan.Apply(tt.fs, false)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestApplySkipsEmptyAndIdenticalReplacements(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	fs := &fakeFS{
		files:   []FilePath{"f"},
		data:    map[FilePath][]byte{"f": []byte("keep abc xyz")},
		readErr: map[FilePath]error{},
	}
	plan := Plan{
		Replacements: []Replacement{
			{From: "", To: "ignored"},  // empty From is skipped
			{From: "abc", To: "abc"},   // identical is skipped
			{From: "xyz", To: "final"}, // applied
		},
	}

	changed, err := plan.Apply(fs, false)

	must.NoError(err)
	want.Equal(Changed{"f"}, changed)
	want.Equal("keep abc final", string(fs.data["f"]))
}

func TestParseModule(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	path, err := parseModule([]byte("// comment\n\nmodule example.com/org/after.cli\n\ngo 1.26\n"))
	must.NoError(err)
	want.Equal(module.Path("example.com/org/after.cli"), path)

	_, err = parseModule([]byte("go 1.26\n"))
	must.ErrorIs(err, ErrNotFound)
}

func TestCommandName(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	name, err := commandName([]FilePath{"README.md", "cmd/tool/main.go"})
	must.NoError(err)
	want.Equal(module.Name("tool"), name)

	_, err = commandName([]FilePath{"README.md", "cmd"})
	must.ErrorIs(err, ErrNotFound)
}

func TestCommandSegment(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	name, ok := commandSegment("cmd/tool/main.go")
	want.True(ok)
	want.Equal(module.Name("tool"), name)

	_, ok = commandSegment("internal/app/x.go")
	want.False(ok, "non-cmd path is not a command")

	_, ok = commandSegment("cmd/tool")
	want.False(ok, "cmd entry without a trailing segment is not a command")
}
