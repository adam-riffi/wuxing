# Contributing

## Prerequisites

- **Go 1.23+**
- **Docker** (for integration tests and running service containers; not needed for unit tests or building)
- **golangci-lint** (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`)

## Branch model — trunk-based

`main` is protected and always releasable. Work happens on short-lived branches
merged via squash PR.

- Branch names: `feat/<slug>`, `fix/<slug>`, `chore/<slug>`, `docs/<slug>`.
- Open a PR into `main`. CI must be green before merge.
- Squash-merge; the PR title becomes the commit, so it must be a conventional commit.

### Required status checks

The following must pass before a PR can merge (enforced by branch protection):

- `lint`
- `test-unit (ubuntu-latest)`, `test-unit (macos-latest)`, `test-unit (windows-latest)`
- `test-integration`
- `build (ubuntu-latest)`, `build (macos-latest)`, `build (windows-latest)`
- `commit-lint`

## Conventional commits

PR titles (and therefore squashed commits) follow
[Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<optional scope>): <description>
```

Types: `feat`, `fix`, `docs`, `test`, `chore`, `refactor`, `perf`, `ci`, `build`.
Examples: `feat(scheduler): dual-resource admission`, `docs(vocabulary): pin ai.infer signature`.

The `commit-lint` CI job validates this on every PR.

## Local workflow

```sh
make build            # compile daemon + CLI into ./bin
make test             # unit tests: go test ./... -short -race -cover
make test-integration # integration tests (needs Docker)
make lint             # golangci-lint
make fmt              # gofumpt -w .
```

A pre-commit hook is recommended (runs lint + short tests + format check). Install
[pre-commit](https://pre-commit.com/) and add a hook that runs `golangci-lint run`,
`go test ./... -short`, and `gofumpt -l .`.

## One-time repository setup (maintainer)

Branch protection cannot be set from code; configure it once in the GitHub UI
(Settings → Branches → add rule for `main`):

- Require a pull request before merging.
- Require status checks to pass — select every check listed above.
- Require branches to be up to date before merging.
- Do not allow direct pushes to `main`.

Verify by attempting a direct push to `main` — it must be rejected.

## Testing conventions

- Unit tests live next to source as `*_test.go`; table-driven where it fits.
- Integration tests live in `test/integration/` and call `t.Skip` under `testing.Short()`.
- End-to-end tests live in `test/e2e/`; the only one at M1 is the MTG flow.
- Every test run uses `-race`. Coverage floor is **70%** per the unit job.
- No mocks at the SQL boundary — use real SQLite files in `t.TempDir()`.

## Design docs

Architecture lives in [`docs/architecture/`](architecture/) (committed copies of
the authoring drafts in the untracked top-level `context/`). The tool function
vocabulary — the gating work — lives in [`docs/vocabulary/`](vocabulary/). Keep
both in sync with the code in the same PR that changes behaviour.
