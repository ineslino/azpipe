# Operational guide

A terminal tool for DevOps engineers who need fast, focused visibility into Azure DevOps
pipeline health — run history, failure trends, stage breakdowns, and repo-to-pipeline
mapping — without leaving the shell.

## Install

For installation of the current checkout into `~/.local/bin`, persistent shell PATH
configuration and verification from any directory, see the [installation guide](../README.en.md#install-and-launch).
The installed binary is a snapshot of the build, not a link to the source or an automatic updater.

### From source (Go 1.26.3+)

```bash
go install github.com/ineslino/azpipe@latest
```

### Publication boundary

`@latest` fetches published code, not this working tree. Build this checkout to use
its current features. Release packaging is configured, but availability of published
binary assets has not been verified.

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

Start with `a` or `?`: arrows choose an action and Enter opens it. Disabled actions
explain why they are unavailable and cannot be activated. Esc closes the menu
without changing the selection. Existing shortcuts remain available in the catalog;
the contextual footer only shows the primary actions. On review errors, select the
affected row and press Enter to return to that pipeline for correction and a fresh
preview. Exact execution confirmation is unchanged.

The catalog and offline demo show the AZPIPE block banner at 60+ columns and 32+
rows. Shorter terminals keep the compact identity and description to preserve rows.

| Key | Action |
|-----|--------|
| `j`/`k` or arrows | Move through the catalog |
| `/` | Filter by name, ID, folder, type, repository, or tag |
| `Space` | Select or remove the active pipeline |
| `m` (`p` alias) | Toggle `RUN`/`PLAN` on an already selected pipeline with an explicit PLAN contract |
| `P` / `R` | Apply PLAN / RUN to the whole selection; PLAN requires contracts for all selected pipelines |
| `e` | Read the root YAML and open typed fields; Tab moves fields, arrows choose options, Ctrl+R restores the default, Ctrl+S saves, Esc discards |
| `J` | Advanced JSON parameter editor |
| `s` / `l` | Save the current selection as a profile / load a saved profile |
| `h` | Browse previous batches in this context and resume monitoring without submitting runs |
| `Enter` in a field | Finish editing and retain the filter or branch |
| `PgUp` / `PgDn` | Page through review and execution rows; arrows select a review item and left/right scroll its vertical detail |
| `b` | Edit the global branch, initially `main` |
| `Enter` | Review the selection |
| `Esc` | Leave the current input or return to the catalog without losing the selection |
| `q`, `Ctrl+C`, `Ctrl+D` | Exit safely without creating a run before confirmation |

`PLAN` requires an explicit contract loaded from `AZPIPE_CONTRACTS` (JSON file).
The contract identifies the organization URL, project, pipeline ID, definition
version, source SHA, parameter name/type, RUN/PLAN values, and review evidence.
Only boolean and string mode parameters are supported. The pipeline owner must
validate the meaning of that parameter. Azure DevOps preview is YAML expansion,
not proof that a pipeline has no side effects.

Example contract (replace the illustrative values with reviewed values):

```json
[
  {
    "organization": "https://dev.azure.com/example-org",
    "project": "sample-project",
    "pipelineId": 202,
    "definitionVersion": 7,
    "commit": "0123456789012345678901234567890123456789",
    "parameter": "planOnly",
    "type": "boolean",
    "planValue": "true",
    "runValue": "false",
    "evidence": "Owner review reference confirming PLAN and RUN behaviour"
  }
]
```

With a contract, RUN explicitly sends the RUN value. Without a contract, RUN
uses the pipeline's defaults for omitted parameters. Review labels parameters as
**sent**, not effective defaults. Do not put secrets in template parameters.

### Pipeline parameter forms

The form reads the pipeline definition, resolves the selected branch to a commit,
and reads its root YAML from Azure Repos Git at that commit. It uses the declared
`name`, `displayName`, `type`, `default`, and `values`, not a guessed schema from
preview output. Strings, numbers, booleans and scalar option lists are editable.
Defaults remain omitted unless explicitly overridden. Required values, unknown
names, invalid types and disallowed options are checked again before preview,
including when a saved profile is loaded.

Complex types (`object`, `stringList`, `stepList`, jobs and stages) are displayed
as read-only and can use their YAML defaults. Required complex inputs without a
default are blocked. The tool does not recursively expose parameters of nested
templates, and does not support classic pipelines or non-Azure-Repos YAML sources.
The advanced JSON editor does not bypass these checks. The mode parameter remains
owned by the RUN/PLAN control rather than being edited twice.

The underlying contract is documented in Microsoft's
[runtime parameters](https://learn.microsoft.com/en-us/azure/devops/pipelines/process/runtime-parameters?view=azure-devops)
and [Git Items API](https://learn.microsoft.com/en-us/rest/api/azure/devops/git/items/get?view=azure-devops-rest-7.1).

### Profiles and batch history

Press `s` to save pipeline IDs, modes, branches and non-secret parameter overrides.
The save screen explains what is persisted and Enter confirms the write. Existing
profile names are never overwritten. Press `l` to load a profile for the current
organization/project. Loading replaces the selection, but does not reuse a preview
or trigger any run. Removed pipelines and unavailable PLAN contracts block loading;
changed parameters are checked during the new preview.

Profiles and batch journals use separate `profiles/` and `runs/` directories under
`os.UserConfigDir()/azpipe` (on macOS, `~/Library/Application Support/azpipe`).
Set `AZPIPE_DATA_DIR` to an absolute directory to relocate both. Files are created
with mode 0600. Profiles contain parameter values in plaintext: never store secrets.
Profile branches are retained per pipeline; editing `b` applies one global branch.

Press `h` to browse saved batches and resume one. The dashboard counts queued,
running, successful, failed and unknown-ID runs; accepted runs retain their URLs.
Refreshes persist updated states. Esc returns to the catalog without cancelling
remote runs; reopen the batch with `h`. Monitoring is active only for the open batch.
An unknown-ID submission stays uncertain and is never resubmitted by resume.

The demo has typed fixture parameters and an example batch with mixed states.
Demo profiles are kept in memory for the session only. It never writes these stores
or calls Azure DevOps, and the example batch does not simulate live progress.

### Preview and execution safety

Every selected pipeline uses the preview API, with at most four HTTP requests in
parallel. For Azure Repos Git, the source SHA and definition revision are resolved
before preview and reused for queue. Review shows context, branch, SHA, definition
revision, mode, sent parameters and preview result. Other repository types fail
preparation. The expanded YAML hash is checked again before queue. This does not
freeze external services, variable groups, mutable images or external template
repositories; these still require owner-controlled immutable references.
No pipeline can be queued while any preview is pending or failed.

After every preview succeeds, type `EXECUTAR` exactly to queue the selection. Queue
requests also use a maximum concurrency of four. If one queue request fails, already
accepted runs remain active and are shown alongside the failure; the command returns
a non-zero exit status after monitoring the result. Leaving the final screen stops
local monitoring only and never cancels a remote run.

Concurrency limits HTTP requests, not active runs in Azure DevOps. A completed
failed/canceled run or stopping monitoring before completion produces a non-zero
exit status. The TUI records accepted run IDs in a private journal and prints its
path. A POST timeout without an ID remains uncertain: check Azure DevOps before
submitting again. Resume only reads known run IDs and never retries submissions.

### Non-interactive batches and resume

Create a JSON selection file, for example:

```json
[{"id":202,"mode":"PLAN","branch":"main","parameters":{"environment":"dev"}}]
```

```bash
# Preview only; no runs created
azpipe batch --file selection.json --project sample-project
# A profile saved by the TUI can also be reviewed from the CLI
azpipe batch --profile dev-stack --project sample-project
# Explicit execution confirmation, with a new journal path
azpipe batch --file selection.json --project sample-project --execute --journal batch.json
# Resume monitoring with the same organization/project
azpipe resume batch.json --project sample-project
```

An existing journal path is refused during execution to prevent accidental reuse.
Journal files contain run IDs, state and URLs, not credentials or template inputs.

### Validation boundaries

The offline demo exercises selection, filtering, parameter editing, modes and
review. HTTP integration tests use local servers. Native Windows and WSL runtime
are not established by cross-compilation; the CI matrix runs Go checks on Linux
and macOS and cross-compiles Windows when published. The POSIX installer targets
macOS/Linux/WSL.
The optional `azdo-as` adapter keeps credentials inside the helper process. Set
`AZPIPE_AZDO_AS` to its executable, `AZPIPE_AUTH_PROFILE` to the approved profile and
`AZPIPE_EXPECTED_IDENTITY` to the expected account. Identity is checked before each
operation, and requests use `devops invoke`. The helper resolves credentials afresh
on each call. Expired credentials require your approved renewal process; azpipe
does not renew PATs or launch an interactive login. Real corporate/VDI validation
is still required. Without the adapter, authentication uses `AZDO_PAT` or the
legacy config.

## Existing CLI commands

The non-interactive commands remain available for automation:

```bash
azpipe projects list [--org <org>]
azpipe repos list --org <org> --project PROJECT
azpipe repos pipelines <repo-name> --org <org> --project PROJECT
azpipe pipelines list --org <org> --project PROJECT
azpipe pipelines runs <pipeline-id> --project PROJECT [-n 20]
azpipe pipelines analyze <pipeline-id> --project PROJECT [-n 25]
azpipe pipelines watch <pipeline-id> --project PROJECT [--interval 5]
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
azpipe auth set --pat <token> [--org <org>] [--project PROJECT]
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
azpipe repos list           --org <org> --project PROJECT
azpipe repos pipelines <repo-name>  --org <org> --project PROJECT
```

`repos list` lists all Git repositories. `repos pipelines` shows every pipeline
that builds from the given repository.

### `pipelines`

```
azpipe pipelines list                   --org <org> --project PROJECT
azpipe pipelines runs    <pipeline-id>  --org <org> --project PROJECT [-n 20]
azpipe pipelines analyze <pipeline-id>  --org <org> --project PROJECT [-n 25]
azpipe pipelines watch   <pipeline-id>  --org <org> --project PROJECT [--interval 5]
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
| `azpipe` interactive preview and execution | **Build (Read & execute)** and **Code (Read)** for Azure Repos source resolution |
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

MIT — see [LICENSE](../LICENSE).
