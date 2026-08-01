package rewrite

import "github.com/gomatic/go-module"

// The rewrite plan: what to replace, and the rules for which replacements are
// meaningful. A plan is DATA — it arrives from a caller — so the skips here are
// what keep a malformed entry from becoming a no-op edit or an infinite one.

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
