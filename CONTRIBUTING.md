# Contributing to azpipe

## Prerequisites

- Go 1.22+
- [golangci-lint](https://golangci-lint.run/usage/install/)
- (optional) [goreleaser](https://goreleaser.com/install/) for cutting releases

## Development setup

```bash
git clone https://github.com/ineslino/azpipe
cd azpipe
go mod tidy
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
main.go
```

## Adding a new command

1. Create `cmd/<group>.go` or add to an existing group file.
2. Wire up with `<group>Cmd.AddCommand(newCmd)` in `init()`.
3. Add API methods to `azdo.Client` interface if needed; implement in `azdo.go`; add to `MockClient`.
4. Write a test in `cmd/integration_test.go` using `MockClient`.

## Pull requests

- One logical change per PR.
- `go test -race ./...` must pass.
- `golangci-lint run ./...` must pass.
- Update `CHANGELOG.md` under `[Unreleased]`.
