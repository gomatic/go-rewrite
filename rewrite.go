// Package rewrite rewrites a project's identity tokens across its files and moves
// its command directory, turning a clone of the template into a distinct project.
//
// It is the reusable implementation behind the rename command's domain: it
// discovers the current and target identities (the module path from go.mod, the
// command name from the tracked files, the target module from the git origin
// remote), computes a Plan of token replacements and one directory move, and
// applies it through an injected FileSystem. The Git and FileSystem seams keep
// the engine pure and testable; OSGit and OSFileSystem back them with the real
// repository. The package holds no CLI or orchestration logic.
package rewrite

import (
	"strings"

	"github.com/gomatic/go-module"
)

type (
	// FilePath is a path within the project tree, relative to its root.
	FilePath string
	// Token is a literal string substituted in file contents.
	Token string
	// DryRun reports whether to compute changes without writing them.
	DryRun bool
	// Changed is the set of files whose contents the engine rewrote.
	Changed []FilePath
)

// Replacement maps an old token to its replacement.
type Replacement struct {
	From Token
	To   Token
}

// Identity is a project's identity: its Go module path and short command name.
type Identity struct {
	Module module.Path
	Name   module.Name
}

// Plan is a computed set of content replacements and one command-directory move.
type Plan struct {
	MoveFrom     FilePath
	MoveTo       FilePath
	Replacements []Replacement
}

// Git provides the git queries discovery needs.
type Git interface {
	// Remote returns the origin remote URL.
	Remote() (module.Remote, error)
}

// FileSystem is the minimal file access the engine needs. OSFileSystem backs it
// with the repository; tests back it with an in-memory map.
type FileSystem interface {
	// List returns the project files eligible for rewriting.
	List() ([]FilePath, error)
	// Read returns the contents of path.
	Read(path FilePath) ([]byte, error)
	// Write replaces the contents of path.
	Write(path FilePath, data []byte) error
	// Move renames a directory from one path to another.
	Move(from, to FilePath) error
}

// Discover reads the current and target identities. The current module comes from
// go.mod and the current name from the cmd/<name> directory among the listed
// files; the target module comes from the origin remote and the target name from
// override when non-empty, else the remote's repository name.
func Discover(git Git, fs FileSystem, override module.Name) (Identity, Identity, error) {
	current, err := currentIdentity(fs)
	if err != nil {
		return Identity{}, Identity{}, err
	}
	target, err := targetIdentity(git, override)
	if err != nil {
		return Identity{}, Identity{}, err
	}
	return current, target, nil
}

// BuildPlan computes the replacements and directory move that turn the current
// identity into the target identity.
func BuildPlan(current, target Identity) Plan {
	return Plan{
		Replacements: replacements(current, target),
		MoveFrom:     commandDir(current.Name),
		MoveTo:       commandDir(target.Name),
	}
}

// Apply executes the plan against fs, returning the files whose contents changed.
// When dry is true it reports the would-be changes without writing or moving.
func (p Plan) Apply(fs FileSystem, dry DryRun) (Changed, error) {
	files, err := fs.List()
	if err != nil {
		return nil, err
	}
	changed, err := rewriteFiles(fs, files, p.Replacements, dry)
	if err != nil {
		return nil, err
	}
	if err := move(fs, p.MoveFrom, p.MoveTo, dry); err != nil {
		return nil, err
	}
	return changed, nil
}

// currentIdentity reads the current module path and command name from fs.
func currentIdentity(fs FileSystem) (Identity, error) {
	data, err := fs.Read(goModFile)
	if err != nil {
		return Identity{}, err
	}
	path, err := parseModule(data)
	if err != nil {
		return Identity{}, err
	}
	files, err := fs.List()
	if err != nil {
		return Identity{}, err
	}
	name, err := commandName(files)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Module: path, Name: name}, nil
}

// targetIdentity reads the target module from the remote and resolves its name.
func targetIdentity(git Git, override module.Name) (Identity, error) {
	remote, err := git.Remote()
	if err != nil {
		return Identity{}, err
	}
	path, err := module.Parse(remote)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Module: path, Name: nameOrRepo(override, path)}, nil
}

// nameOrRepo returns override when set, otherwise the module's repository name.
func nameOrRepo(override module.Name, path module.Path) module.Name {
	if override != "" {
		return override
	}
	return path.Repo()
}

// replacements lists the token substitutions, most specific first so that the
// full module path is replaced before the bare name, the uppercase environment
// prefix before the lowercase identifier, and the identifier before the raw name.
//
// Replacement contract: every token is matched and replaced as a RAW substring,
// with no word boundaries, casing rules, or token grammar (see applyAll). Order
// is therefore load-bearing — the more specific token must precede any token
// that is a substring of it, so the broad form never consumes the narrow one
// first. Because matching is unanchored, a short or common identity token will
// over-rewrite incidental occurrences elsewhere in the files; callers must
// supply distinctive identity tokens (a full module path, a namespaced command
// name) so that a match is always the project's identity and never coincidence.
func replacements(current, target Identity) []Replacement {
	cn, tn := current.Name, target.Name
	return []Replacement{
		{From: Token(current.Module), To: Token(target.Module)},
		{From: Token(cn.EnvPrefix()), To: Token(tn.EnvPrefix())},
		{From: Token(cn.Identifier()), To: Token(tn.Identifier())},
		{From: Token(cn), To: Token(tn)},
	}
}

// rewriteFiles applies the replacements to each file, collecting those changed.
func rewriteFiles(fs FileSystem, files []FilePath, repls []Replacement, dry DryRun) (Changed, error) {
	var changed Changed
	for _, file := range files {
		ok, err := rewriteFile(fs, file, repls, dry)
		if err != nil {
			return nil, err
		}
		if ok {
			changed = append(changed, file)
		}
	}
	return changed, nil
}

// rewriteFile rewrites a single file, reporting whether its contents changed. It
// writes only when not a dry run and only when the contents actually differ.
func rewriteFile(fs FileSystem, path FilePath, repls []Replacement, dry DryRun) (bool, error) {
	data, err := fs.Read(path)
	if err != nil {
		return false, err
	}
	updated := applyAll(string(data), repls)
	if updated == string(data) {
		return false, nil
	}
	if dry {
		return true, nil
	}
	return true, fs.Write(path, []byte(updated))
}

// applyAll applies every non-identity replacement to content in order. Each
// token is replaced as a raw substring via strings.ReplaceAll — no word
// boundaries, no escaping — so the caller-supplied ordering and the
// distinctiveness of the tokens (see replacements) are what keep the rewrite
// correct. Replacements with an empty or unchanged From are skipped.
func applyAll(content string, repls []Replacement) string {
	for _, r := range repls {
		if r.From == "" || r.From == r.To {
			continue
		}
		content = strings.ReplaceAll(content, string(r.From), string(r.To))
	}
	return content
}

// move renames the command directory, skipping a no-op or a dry run.
func move(fs FileSystem, from, to FilePath, dry DryRun) error {
	if from == to || dry {
		return nil
	}
	return fs.Move(from, to)
}

// commandDir is the cmd/<name> directory for a command name.
func commandDir(name module.Name) FilePath {
	return FilePath(cmdPrefix + string(name))
}

// parseModule extracts the module path from the "module" directive of go.mod.
func parseModule(data []byte) (module.Path, error) {
	for line := range strings.SplitSeq(string(data), "\n") {
		if path, ok := moduleDirective(line); ok {
			return path, nil
		}
	}
	return "", ErrNotFound.With(nil, "module directive")
}

// moduleDirective returns the module path declared by a "module <path>" line.
func moduleDirective(line string) (module.Path, bool) {
	const directive = "module "
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, directive) {
		return "", false
	}
	return module.Path(strings.TrimSpace(strings.TrimPrefix(trimmed, directive))), true
}

// commandName returns the single segment under cmd/ among the project files.
func commandName(files []FilePath) (module.Name, error) {
	for _, file := range files {
		if name, ok := commandSegment(string(file)); ok {
			return name, nil
		}
	}
	return "", ErrNotFound.With(nil, "cmd directory")
}

// commandSegment returns the <name> in a "cmd/<name>/..." path.
func commandSegment(path string) (module.Name, bool) {
	if !strings.HasPrefix(path, cmdPrefix) {
		return "", false
	}
	rest := path[len(cmdPrefix):]
	if before, _, found := strings.Cut(rest, "/"); found {
		return module.Name(before), true
	}
	return "", false
}

const goModFile FilePath = "go.mod"

const cmdPrefix = "cmd/"
