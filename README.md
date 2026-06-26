# wuxing

Modular, declarative control plane that runs services, AI agents, and ETL flows as one governed, fully-observable system.

A personal control plane: a single place to **set up**, **observe**, **control**, and **orchestrate** all your services. Mechanically, a **declarative cfg-interpreter** — the config is the program, the runtime is its interpretation, routed to tools and services over an in-process bus. Lineage: Terraform, Kubernetes, Airflow. AI is a first-class capability, but governed as infrastructure: an agent is a bounded content producer in a sealed room with supervised doors.

> **Status: walking-skeleton build (Phase 0).** The kernel boots, loads its manifest, and shuts down cleanly. Tools and the interpreter are stubs being filled in phase by phase — see [the development plan](#roadmap).

## Three tiers

- **wuxing — the kernel.** The cfg-interpreter and router. A long-running, compiled daemon (Go). Its seven faces: interpreter, bus, scheduler, sessions/registry, triggers, launcher, lineage.
- **tools — the platform verbs.** wuxing's own compiled code, named in the boot manifest: `library`, `graph`, `processors`, `connectors`, `ai`. Tools compose; they talk only over the bus.
- **services — sealed user boxes.** Runtime content, run as Docker containers, listed in the library index. Mutually ignorant — the only thing crossing a service boundary is an event on the bus.

## Repository layout

```
cmd/wuxing      kernel daemon       internal/tools       the five platform tools
cmd/wxg         operator CLI        internal/contracts   cfg / bus / sdk contracts
internal/kernel the seven faces     internal/storage     fact/dim spine + artifacts
services/       first-party + examples                   manifest/  boot manifest
docs/           architecture + vocabulary                test/      integration + e2e
```

## Quickstart

Requires **Go 1.23+** and (for service containers, later phases) **Docker**.

```sh
# build the daemon and the CLI into ./bin
make build            # or: go build ./cmd/...

# run unit tests (race detector + coverage, no Docker)
make test             # or: go test ./... -short -race -cover

# lint
make lint             # or: golangci-lint run

# boot the kernel (idles at Phase 0, Ctrl-C to stop)
make run              # or: go run ./cmd/wuxing --manifest manifest/boot.yml
```

On Windows, run `make` from Git Bash/WSL, or invoke the underlying `go` commands directly from PowerShell.

Secrets follow 12-factor: copy `.env.example` to `.env` and fill in values. The cfg holds the reference (`${SCRYFALL_KEY}`); `.env` holds the value, never committed.

## Roadmap

The build follows the critical path **vocabulary → grammar → contracts → interpreter → scaffold → build**:

| Phase | Deliverable |
|---|---|
| **0** | Repo scaffold: layout, CI/CD, lint, tests, the daemon boot path *(current)* |
| **1** | Tool function vocabulary (`ai`, `connectors` signature catalogs) |
| **2** | Lock the three contracts: cfg schema, bus envelope, SDK |
| **3** | Kernel faces: bus, lineage, sessions, triggers, scheduler, launcher |
| **4** | Storage spine (append-only fact/dim tables, SQLite) |
| **5** | First tools + finalize the interpreter |
| **6** | First-party services (messenger, state) + graph |
| **7** | MTG new-set notifier end-to-end — **M1 acceptance** |

See [`docs/architecture/`](docs/architecture/) for the design documents.

## License

[MIT](LICENSE) © 2026 Adam Riffi
