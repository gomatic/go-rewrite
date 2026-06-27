package rewrite

// errs holds this package's intrinsic sentinels on the shared go-error mechanism.
import errs "github.com/gomatic/go-error"

const (
	// ErrGitCommand indicates a git invocation failed.
	ErrGitCommand errs.Const = "git command failed"
	// ErrNotFound indicates a required token (module directive, cmd dir) was absent.
	ErrNotFound errs.Const = "not found"
	// ErrOpenFile indicates a file could not be read.
	ErrOpenFile errs.Const = "failed to open file"
	// ErrWriteFile indicates a file could not be written.
	ErrWriteFile errs.Const = "failed to write file"
	// ErrMoveFile indicates a path could not be moved.
	ErrMoveFile errs.Const = "failed to move file"
)
