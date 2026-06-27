# go-rewrite

A rebrand-on-clone engine: it `Discover`s a project's current and target identity (the module path from `go.mod`, the command name from the tracked files, the target module from the git origin remote), `BuildPlan`s the token replacements and the one command-directory move that turn the current identity into the target, and applies them with `Plan.Apply` through an injected `FileSystem`. The `Git` and `FileSystem` seams keep the engine pure and testable; `OSGit` and `OSFileSystem` back them with the real repository, the latter confining all file access beneath the project root via [`os.OpenRoot`](https://pkg.go.dev/os#OpenRoot).

## Install

```sh
go get github.com/gomatic/go-rewrite
```

## Usage

```go
package main

import (
	"log"

	rewrite "github.com/gomatic/go-rewrite"
)

func main() {
	git := rewrite.OSGit{Dir: "."}
	fs := rewrite.OSFileSystem{Root: ".", Lister: git.Files}

	// Discover the current identity (from go.mod and cmd/<name>) and the target
	// identity (from the origin remote); pass an override name instead of "" to
	// force a specific command name.
	current, target, err := rewrite.Discover(git, fs, "")
	if err != nil {
		log.Fatal(err)
	}

	// Apply the plan; pass true for a dry run that reports changes without writing.
	changed, err := rewrite.BuildPlan(current, target).Apply(fs, false)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("rewrote %d files", len(changed))
}
```
