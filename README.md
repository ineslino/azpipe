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
# 1. Recommended: inject credentials without writing a PAT to disk
export AZDO_PAT=<your-pat>
export AZDO_ORG=myorg

# 2. Start the interactive pipeline runner
azpipe

# 3. Or explore the offline demo with no credentials or Azure DevOps calls
azpipe demo

# 4. Keep using the scriptable commands
azpipe projects list

# 5. List pipelines
azpipe pipelines list --project myproject

# 6. See the last 10 runs of pipeline 42
azpipe pipelines runs 42 --project myproject -n 10

# 7. Analyse pipeline health
azpipe pipelines analyze 42 --project myproject

# 8. Live-watch the active run
azpipe pipelines watch 42 --project myproject
```

`azpipe auth set --pat <your-pat>` remains available for backwards compatibility,
but persisted PATs are legacy. It writes `~/.config/azpipe/config.yaml`; use
`AZDO_PAT` or an external credential-injection mechanism for normal use.

## Interactive pipeline runner

Running `azpipe` with no subcommand opens the pipeline runner. It collects the
organization and project, using configured values as defaults, then lets you select
multiple pipelines before any Azure DevOps run is created.

`azpipe demo` opens the same catalog with local fixture data. It does not request
credentials, create an Azure DevOps client, make network calls, or expose an action
that can queue a pipeline.

### Shortcuts

| Key | Action |
|-----|--------|
| `j`/`k` or arrows | Move through the catalog |
| `/` | Filter by name, ID, folder, type, repository, or tag |
| `Space` | Select or remove the active pipeline |
| `p` | Toggle the active pipeline between `RUN` and `PLAN` (and select it if needed) |
| `b` | Edit the global branch, initially `main` |
| `Enter` | Review the selection |
| `Esc` | Leave the current input or return to the catalog without losing the selection |
| `q`, `Ctrl+C`, `Ctrl+D` | Exit safely without creating a run before confirmation |

`PLAN` sends `planOnly=true`. This is accepted only if Azure DevOps preview confirms
that the pipeline supports it.

### Preview and execution safety

Every selected pipeline is previewed with `previewRun=true`, with at most four
requests in parallel. The review shows the effective branch, mode, parameters and
preview result. No pipeline can be queued while any preview is pending or failed.

After every preview succeeds, type `EXECUTAR` exactly to queue the selection. Queue
requests also use a maximum concurrency of four. If one queue request fails, already
accepted runs remain active and are shown alongside the failure; the command returns
a non-zero exit status after monitoring the result. Leaving the final screen stops
local monitoring only and never cancels a remote run.

## Existing CLI commands

The non-interactive commands remain available for automation:

```bash
azpipe projects list [--org <org>]
azpipe repos list --org <org> --project <project>
azpipe repos pipelines <repo-name> --org <org> --project <project>
azpipe pipelines list --org <org> --project <project>
azpipe pipelines runs <pipeline-id> --project <project> [-n 20]
azpipe pipelines analyze <pipeline-id> --project <project> [-n 25]
azpipe pipelines watch <pipeline-id> --project <project> [--interval 5]
```

## Auth

| Source | Priority |
|--------|----------|
| `AZDO_PAT` env var | highest |
| `~/.config/azpipe/config.yaml` | fallback |

`AZDO_ORG` / `--org` work the same way. The value can be just the org name (`myorg`) or
the full URL (`https://dev.azure.com/myorg`).

Use `AZDO_PAT` or an external credential-injection mechanism. `azpipe auth set` is a
legacy compatibility command for writing the config file and should not be the normal
way to persist a PAT.

## Commands

### `auth`

```
azpipe auth set --pat <token> [--org <org>] [--project <project>]
```

Stores a legacy PAT and defaults in `~/.config/azpipe/config.yaml`. The file is written
with permissions `0600`; prefer `AZDO_PAT` or external credential injection.

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

Use the least-privilege PAT scope for the commands you run:

| Command | PAT scope |
|---------|-----------|
| `azpipe` interactive preview and execution | **Build (Read & execute)** |
| `pipelines list`, `runs`, `analyze`, `watch` | **Build (Read)** |
| `projects list` | **Project and Team (Read)** |
| `repos list` | **Code (Read)** |
| `repos pipelines` | **Code (Read)** and **Build (Read)** |

Azure DevOps documents the execution API under the `vso.build_execute` scope, which
includes the ability to queue a build: [Runs - Run Pipeline](https://learn.microsoft.com/en-us/rest/api/azure/devops/pipelines/runs/run-pipeline?view=azure-devops-rest-7.1).

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
