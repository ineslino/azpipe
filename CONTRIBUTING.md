# Contributing to azpipe

## Prerequisites

- Go 1.26.3+ (see go.mod)
- [golangci-lint](https://golangci-lint.run/usage/install/)
- (optional) [goreleaser](https://goreleaser.com/install/) for cutting releases

## Development setup

```bash
git clone https://github.com/ineslino/azpipe
cd azpipe
go mod download
make build        # produces ./azpipe binary
make test         # run all tests with -race
make lint         # run golangci-lint
```

## Running against a real org

```bash
export AZDO_PAT=<your-pat>
export AZDO_ORG=<your-org>
./azpipe projects list
./azpipe pipelines list --project myproject
```

## Project layout

```
cmd/              cobra commands (one file per subcommand group)
internal/
  azdo/           Azure DevOps API client interface + implementation + MockClient
  config/         viper config loading (~/.config/azpipe/config.yaml)
  analysis/       pipeline stats computation (pure functions, no API calls)
  ui/             lipgloss table renderer + bubbletea watch model
  runner/         preview/queue gates, profiles and run journals
  tui/runner/     context, catalog, review and batch monitoring models
main.go
```

## Adding a new command

1. Create `cmd/<group>.go` or add to an existing group file.
2. Wire up with `<group>Cmd.AddCommand(newCmd)` in `init()`.
3. Add API methods to `azdo.Client` only when needed; update both native and command adapters and `MockClient`.
4. Write a test in `cmd/integration_test.go` using `MockClient`.

## Pull requests

- One logical change per PR.
- `go test -race ./...` must pass.
- Run `go vet ./...` and `go build ./...`; optional `make lint` requires a separately installed golangci-lint (not version-pinned).
- Update `CHANGELOG.md` under `[Unreleased]`.
- Keep both READMEs aligned and update the [operational guide](docs/usage.md).
- Follow the [offline demo](docs/demo.md); regenerate model images when views change.
- Do not run real pipelines, publish or change remote settings as part of a local validation.
