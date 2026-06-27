# go-rewrite

The rebrand-on-clone engine (package `rewrite`): `Discover` current/target identity from a git remote, `BuildPlan` the token replacements, and `Apply` them across a project's files (with a `DryRun` option), plus the `Git`/`FileSystem` seams and their `OS*` implementations. Reusable by every `template.*` repo that renames itself after cloning. Lives in `gomatic`.

- Depends on `gomatic/go-module` (identity derivation) and `gomatic/go-error` (its `error.Const` sentinels).
- Gate: gofumpt, vet, staticcheck, govulncheck, gocognit ≤ 7, 100% coverage.
