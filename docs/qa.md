# Acceptance checks

These checks distinguish native product behaviour from fixtures, remote integration
and assistive technology. A green mock test is not a live Azure PASS.

## Native executable

Build and inspect the exact executable under test:

```bash
go build -o /tmp/azpipe-qa .
/tmp/azpipe-qa --version
go version -m /tmp/azpipe-qa
AZPIPE_QA_CAPTURE_DIR=/tmp/azpipe-qa-captures python3 scripts/qa/native_pty.py /tmp/azpipe-qa
```

`native_pty.py` uses a real Unix PTY and a synthetic, read-only credential adapter.
It isolates local configuration, stores text/ANSI captures, checks 60×24, 80×24 and
120×40, `NO_COLOR`, resize, filtering, fields/options, history, review, monitoring,
branch review, auth errors and cancellation. A light-background run sets
`AZPIPE_QA_LIGHT=1`. No real Azure runs or deletes are made.

Windows tests use ConPTY through the existing `x/sys/windows` dependency. Set
`AZPIPE_TEST_BINARY` to the native `.exe`, then run `go test ./scripts/qa -v`.
The six CI jobs execute binaries natively, check private files, package/extract the
binary and verify SHA-256. Windows ARM64 uses ordinary tests because the Go race
detector does not support that target. It is an explicit N/A for race only.

## Confined live Azure QA

Use a new **private `AZPIPE-QA` project**, repository **`azpipe-qa`**, the two
[committed fixtures](../scripts/qa/fixtures/) at `qa/parameters.yml` and `qa/plan.yml`,
and a disposable `azpipe-qa/flow` branch. Disable initial/automatic runs when creating
the two definitions. Check project-create/code/build permissions and hosted parallel
job availability before creating resources or queueing; do not purchase capacity,
add service connections or change existing projects/policies to pass QA.

Record the returned project/repository IDs, branch commit and definition revisions
in a private copy of [manifest.example.json](../scripts/qa/fixtures/manifest.example.json)
outside the checkout. Do not commit identities, corporate URLs, real IDs or run logs.
Record created resource IDs even if provisioning stops halfway. Cleanup requires
an explicit decision; the helper never deletes projects, repositories or branches.

The helper verifies identity, private project, repository ownership, disposable
branch SHA, both definition revisions/paths and exact remote fixture YAML before
calling the product. It refuses production targets. Use the approved adapter/session:

```bash
python3 scripts/qa/live_qa.py \
  --manifest /private/path/qa-manifest.json \
  --adapter /path/to/credential-adapter --profile approved-profile \
  --expected-identity user@example.test --binary /path/to/azpipe
```

This performs the real preview without queueing. Add `--execute` only to perform the
authorised QA runs. Each run retains a new evidence directory and journal; uncertain
submissions are inspected remotely and never automatically retried. PLAN uses the
reviewed echo-only fixture; RUN and PLAN do not touch application resources.

On a local helper timeout, its process group is stopped on Unix and its process tree
is stopped with Windows `taskkill`. Accepted remote runs remain active; inspect the
retained journal and Azure before any new submission. A synthetic process test checks
that a local child does not survive the deadline.

Complete live coverage additionally requires:

| Scenario | Evidence required |
| --- | --- |
| Parameters, RUN/PLAN and selection | TUI form, preview, exact confirmation, accepted IDs, result URLs |
| Failed/waiting run and resume | `outcome: failure` / `wait`, journal reopened, same IDs, no resubmission |
| Context and credentials | One/all-project reads, session renewal, denied reads, unchanged selection |
| Disposable branch management | Create `azpipe-qa/` refs, inspect creator, reject protected/current/locked refs, delete only reviewed SHA |
| Cancel/timeout | Pending reads/preview cancelled, late results discarded, uncertain POST retained |

Branches and additional failure runs are controlled separately from the two happy
path batches. The helper does not grant permissions, simulate a permission denial
as real evidence or certify these manual scenarios.

## WSL and assistive technology

Use the installed executable and record OS, architecture, terminal, font, tool revision,
settings and observed result. Headless CI does not prove screen reader usability.

| Environment | Required user journey |
| --- | --- |
| WSL2 Ubuntu 24.04, Windows x64/ARM64 | Install the corresponding Linux binary, welcome, form, review, resume, `:q`, resize and clipboard paste |
| macOS Terminal/iTerm + VoiceOver | Discover current step, focused/selected row, parameter name/default/error, live state and recovery |
| Windows Terminal + Narrator | Same journey with ConPTY, plain mode and reduced visual emphasis |
| Linux terminal + Orca | Same journey with PTY and `NO_COLOR` |
| Low vision / keyboard / cognitive load | Three sizes, light/dark, no colour-only status, visible shortcuts, preserved work after Esc |

If a required host, session or assistive technology is unavailable, record **pending**
with the reason. Do not award complete readiness or 100/100 from source, fixtures or
cross compilation.
