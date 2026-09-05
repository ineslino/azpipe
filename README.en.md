[Português](README.md) | [English](README.en.md)

# azpipe

![AZPIPE: select, review and monitor pipelines in one terminal](docs/assets/hero.svg)

Select and run multiple Azure DevOps pipelines in a TUI, review before submitting, and monitor each run. A CLI remains available for automation and analysis.

## Try it first

Requires Go **1.26.3+** and an interactive terminal. From this checkout:

```bash
go run . demo
```

The demo uses fictional data, without credentials, Azure DevOps calls or profile writes to disk. Follow the [visual walkthrough](docs/demo.md).

![Actual TUI catalog rendered with fictional data](docs/assets/catalog.svg)

## Capabilities

- One list with name, type, folder, repository, ID and tag filters.
- Multi-selection, per-pipeline RUN/PLAN, typed forms and reusable profiles.
- Branch, SHA, sent parameters and preview review before exact `EXECUTAR` confirmation.
- Up to four concurrent requests, per-run status and URL, resumable history without resubmission.
- CLI batch operations, history, analysis and pipeline queries.

**PLAN requires an explicit contract reviewed by the pipeline owner.** Preview expands YAML; it does not prove the absence of side effects. Preparation pins the source SHA and definition revision, but does not freeze external services or references.

This does not replace Azure DevOps, cancel remote runs or run Terraform locally. Forms support root YAML in Azure Repos Git, not classic pipelines or external Git sources.

## Install and launch

To use exactly this checkout:

```bash
go build -o azpipe .
./azpipe
```

The TUI asks for organization and project. To install the **published** version, which may not yet include these changes:

```bash
go install github.com/ineslino/azpipe@latest
```

Use `AZDO_PAT` injected by your credential mechanism and `AZDO_ORG` for context. Never put tokens in examples, profiles or parameters. Legacy `auth set` persists the PAT and is not recommended. The optional `azdo-as` adapter, permissions and PLAN contracts are covered in the [operational guide](docs/usage.md).

## Usage

| Key | Action |
| --- | --- |
| Arrows / `j` / `k` | Navigate |
| `/` / Space | Filter / select |
| `m` / `P` / `R` | Toggle mode / PLAN for selection / RUN for selection |
| `e` / `b` | Typed parameters / branch |
| `s` / `l` / `h` | Save profile / load profile / history |
| Enter / Esc | Review / return without submitting |

CLI example, preview only, using a selection file prepared as described in the guide:

```bash
./azpipe batch --file selection.json --org example-org --project sample-project
./azpipe --help
```

Profiles store parameters in plaintext. Never include secrets. Leaving monitoring does not cancel accepted runs; submissions without an ID remain uncertain and are never retried automatically.

## Architecture and validation

`cmd` connects the CLI to services in `internal/runner`; `internal/azdo` handles the API and contracts. The TUI lives in `internal/tui/runner`, separate from execution logic. `internal/analysis` and `internal/ui` support analysis and display.

```bash
go test -race ./...
go vet ./...
go build ./...
```

HTTP tests use local servers. CI is configured for Linux/macOS and Windows compilation. This does not establish native Windows/WSL runtime or live corporate integration. See [validation and publication](docs/readiness.md) and [contributing](CONTRIBUTING.md).

## Documentation

- [Complete operational guide (EN)](docs/usage.md): authentication, contracts, parameters, profiles, CLI and limits.
- [Demo and images](docs/demo.md).
- [Validation, distribution and outstanding checks](docs/readiness.md).
- [Proposed metadata, not applied](docs/repository-metadata.md).
- [Contributing](CONTRIBUTING.md), [security](SECURITY.md) and [changelog](CHANGELOG.md).
- History, not the current contract: [initial design](docs/superpowers/specs/2026-08-04-pipeline-runner-tui-design.md) and [initial plan](docs/superpowers/plans/2026-08-04-pipeline-runner-tui.md).

## License

[MIT](LICENSE), preserved from the existing repository.
