# azpipe

A terminal tool for DevOps engineers who need fast, focused visibility into Azure DevOps
pipeline health — run history, failure trends, stage breakdowns, and repo-to-pipeline
mapping — without leaving the shell.

## Install

### From source (Go 1.22+)

```bash
go install github.com/ineslino/azpipe@latest
```

### From a release binary

Download the appropriate archive for your platform from the
[Releases](https://github.com/ineslino/azpipe/releases) page, extract it, and place
`azpipe` somewhere on your `PATH`.

## Quickstart

```bash
# 1. Store your PAT and default org/project (written to ~/.config/azpipe/config.yaml)
azpipe auth set --pat <your-pat> --org myorg --project myproject

# 2. Or use env vars — no config file needed
export AZDO_PAT=<your-pat>
export AZDO_ORG=myorg

# 3. List projects
azpipe projects list

# 4. List pipelines
azpipe pipelines list --project myproject

# 5. See the last 10 runs of pipeline 42
azpipe pipelines runs 42 --project myproject -n 10

# 6. Analyse pipeline health
azpipe pipelines analyze 42 --project myproject

# 7. Live-watch the active run
azpipe pipelines watch 42 --project myproject
```

## Auth

| Source | Priority |
|--------|----------|
| `AZDO_PAT` env var | highest |
| `~/.config/azpipe/config.yaml` | fallback |

`AZDO_ORG` / `--org` work the same way. The value can be just the org name (`myorg`) or
the full URL (`https://dev.azure.com/myorg`).

Run `azpipe auth set` to write the config file interactively instead of managing env vars.

## Commands

### `auth`

```
azpipe auth set --pat <token> [--org <org>] [--project <project>]
```

Stores credentials in `~/.config/azpipe/config.yaml`.

### `projects`

```
azpipe projects list [--org <org>]
```

Lists all projects in the org.

### `repos`

```
azpipe repos list           --org <org> --project <project>
azpipe repos pipelines <repo-name>  --org <org> --project <project>
```

`repos list` lists all Git repositories. `repos pipelines` shows every pipeline
that builds from the given repository.

### `pipelines`

```
azpipe pipelines list                   --org <org> --project <project>
azpipe pipelines runs    <pipeline-id>  --org <org> --project <project> [-n 20]
azpipe pipelines analyze <pipeline-id>  --org <org> --project <project> [-n 25]
azpipe pipelines watch   <pipeline-id>  --org <org> --project <project> [--interval 5]
```

| Sub-command | What it shows |
|-------------|---------------|
| `list`      | All pipelines: ID, name, folder, linked repo |
| `runs`      | Last N runs: build#, state, result (coloured), duration, branch, start time |
| `analyze`   | Total runs, avg duration, failure %, top failing stage, flaky stages |
| `watch`     | Live-updating TUI showing each stage's current status; exits on completion |

#### `analyze` sample output

```
Total runs:        25 (last 25)
Avg duration:      4m 32s
Failure rate:      24.0% (6 of 25 non-canceled)
Top failing stage: Test

Flaky stages:
STAGE                             FAILURES  EXECUTIONS  FAILURE RATE
────────────────────────────────────────────────────────────────────
Integration                              3           8        37.5%
E2E Tests                                2           8        25.0%
```

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--org` | `$AZDO_ORG` | Azure DevOps org name or URL |
| `--project` | config value | Azure DevOps project name |
| `-o`, `--output` | `table` | Output format: `table`, `json`, `plain` |
| `--debug` | false | Enable debug output |

All listing commands support `--output json` and `--output plain` for scripting.

## Configuration file

`~/.config/azpipe/config.yaml`:

```yaml
pat: <your-personal-access-token>
org: myorg
project: myproject
```

The PAT requires at minimum the **Build (Read)** and **Project and Team (Read)** scopes.

## Building from source

```bash
git clone https://github.com/ineslino/azpipe
cd azpipe
make build    # → ./azpipe  (CGO_ENABLED=0, no CGO deps)
make test     # → go test -race ./...
make lint     # → golangci-lint run ./...
```

## License

MIT — see [LICENSE](LICENSE).
