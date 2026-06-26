# Contributing

## Prerequisites

- **Go 1.23+**
- **Docker** (for integration tests and running service containers; not needed for unit tests or building)
- **golangci-lint** (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`)

## Branch model — `dev` integrates, `main` is stable

Two long-lived branches:

- **`main`** — always-stable / releasable. Only `dev` promotes into it. Never commit to it directly.
- **`dev`** — the integration branch where CI runs and changes accumulate between releases.

Flow:

```
feat/<slug> ──PR──▶ dev ──(CI green)──▶  ... ──PR──▶ main ──tag v*──▶ release
   fix/…              (integration)                  (stable)
```

1. Branch off `dev`: `feat/<slug>`, `fix/<slug>`, `chore/<slug>`, `docs/<slug>`.
2. Open a PR **into `dev`**. CI must be green; squash-merge (PR title becomes the commit, so it must be a conventional commit).
3. When `dev` is ready to ship, open a **promotion PR `dev` → `main`**. CI runs again on that PR; use a **merge commit** (not squash) so `main`'s history references the real `dev` commits.
4. Tag `main` with `v*.*.*` to cut a release (triggers `release.yml`).

### Sanity vs. required checks

Two tiers of CI:

- **Sanity** (`sanity.yml`) — runs on every push to a feature branch (lint + unit + build, one OS). A fast pre-PR signal; *not* a merge gate.
- **Full** (`ci.yml`) — runs on every PR and on pushes to `dev`/`main` (lint, 3-OS unit + build, integration, commit-lint, coverage floor). These are the merge gate.

The full checks gate **both** the `feat → dev` PRs and the `dev → main` promotion PR
(enforced by branch protection on each branch):

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
(Settings → Branches). Add **one rule each** for `main` and `dev` — both get the
same settings:

- Require a pull request before merging.
- Require status checks to pass — select every check listed above.
- Require branches to be up to date before merging.
- Do not allow direct pushes.

Set the repository's **default branch to `dev`** (Settings → General → Default
branch) so new PRs target the integration branch automatically; `main` then only
ever receives the promotion PR.

Verify by attempting a direct push to either branch — it must be rejected.

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
