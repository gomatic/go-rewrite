package rewrite

// Imported bare (the package is named error); this file declares only sentinels
// and uses no builtin error type, so each declaration reads error.Const.
import "github.com/gomatic/go-error"

const (
	// ErrGitCommand indicates a git invocation failed.
	ErrGitCommand error.Const = "git command failed"
	// ErrNotFound indicates a required token (module directive, cmd dir) was absent.
	ErrNotFound error.Const = "not found"
	// ErrOpenFile indicates a file could not be read.
	ErrOpenFile error.Const = "failed to open file"
	// ErrWriteFile indicates a file could not be written.
	ErrWriteFile error.Const = "failed to write file"
	// ErrMoveFile indicates a path could not be moved.
	ErrMoveFile error.Const = "failed to move file"
)
