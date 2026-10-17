# Contributing

Thanks for your interest in LayerHop!

## Where the code lives

This repository is **exported automatically** from the LayerHop monorepo
(`sdks/go`), which is the single source of truth. Every sync commit is titled
`Sync from layerhop monorepo <sha>` and references the monorepo commit it was built from.

**Pull requests are welcome here.** A maintainer reviews your PR, applies the change upstream in the
monorepo, and the next sync brings it back into this repository with you credited in the PR. Because
of that, your PR may be closed rather than merged directly; that is expected and does not mean it was
rejected.

Issues are welcome too. For security problems, follow [SECURITY.md](SECURITY.md) instead.

## Development

See [README.md](README.md) for setup. CI (`.github/workflows/ci.yml`) runs the same checks you
should run locally before opening a PR.

## Releases

There is no release workflow: Go modules are released by tags. Pushing a semver tag such as `v0.2.0` on `main` publishes that version of `github.com/layerhop/layerhop-go` to the Go module proxy (once the repository is public). Local dev: `go vet ./... && go test ./...`.
