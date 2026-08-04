# Pipeline Runner TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fazer `azpipe` abrir uma TUI segura para selecionar, pré-validar, executar e acompanhar várias pipelines Azure DevOps.

**Architecture:** O cliente Azure DevOps expõe operações pequenas de preview, queue e consulta. Um pacote `runner` independente da UI aplica modos, parâmetros e gates. A Bubble Tea TUI coordena catálogo, revisão e execução, enquanto os comandos CLI atuais permanecem compatíveis.

**Tech Stack:** Go 1.26.3, Cobra, Bubble Tea, Bubbles, Lip Gloss, Azure DevOps Go API v7.1.

## Global Constraints

- `azpipe` sem subcomandos abre a TUI; comandos existentes mantêm comportamento.
- `PLAN` significa exatamente `planOnly=true` e só é aceite após preview bem-sucedida.
- Nenhuma pipeline é lançada se qualquer preview falhar ou estiver pendente.
- Confirmação exige escrever exatamente `EXECUTAR`.
- Concorrência máxima por omissão: quatro.
- O modo demo nunca cria cliente Azure DevOps nem faz rede.
- O repositório público não contém nomes, organizações, identidades ou paths da Fidelidade.
- Alterações seguem TDD e cada tarefa termina num commit local focado.

---

### Task 1: Contrato Azure DevOps para preview e execução

**Files:**
- Modify: `internal/azdo/client.go`
- Modify: `internal/azdo/azdo.go`
- Modify: `internal/azdo/mock.go`
- Create: `internal/azdo/pipelines_run_test.go`

**Interfaces:**
- Produces: `PreviewPipeline(ctx, project string, request RunRequest) error`
- Produces: `QueuePipeline(ctx, project string, request RunRequest) (PipelineRun, error)`
- Produces: `GetPipelineRun(ctx, project string, runID int) (PipelineRun, error)`
- Produces: `RunRequest{PipelineID int, Branch string, Parameters map[string]string}`
- Extends: `Pipeline.Tags []string` and `Pipeline.Type() string`

- [ ] **Step 1: Write failing contract tests**

Add tests that assert branch normalization and parameter payloads through an HTTP test server or injected SDK seam:

```go
request := azdo.RunRequest{
    PipelineID: 742,
    Branch: "main",
    Parameters: map[string]string{"planOnly": "true"},
}
err := client.PreviewPipeline(ctx, "DEVCLD", request)
if err != nil { t.Fatal(err) }
```

Also assert that `QueuePipeline` maps ID, state, result and web URL, and that `Pipeline.Type()` returns the first non-empty folder segment or `root`.

- [ ] **Step 2: Run targeted tests and confirm failure**

Run: `go test ./internal/azdo -run 'TestPreviewPipeline|TestQueuePipeline|TestPipelineType'`

Expected: compilation failure because `RunRequest` and methods do not exist.

- [ ] **Step 3: Implement the API operations**

Use `pipelines.NewClient`, `pipelines.Preview`, and `pipelines.RunPipeline` with:

```go
params := map[string]string{}
for key, value := range request.Parameters { params[key] = value }
ref := normalizeBranch(request.Branch)
runParams := pipelines.RunPipelineParameters{
    Resources: &pipelines.RunResourcesParameters{
        Repositories: &map[string]pipelines.RepositoryResourceParameters{
            "self": {RefName: &ref},
        },
    },
    TemplateParameters: &params,
}
```

Use `build.GetBuild` for status and construct the web URL from the organization, project and run ID when `_links.web` is absent.

- [ ] **Step 4: Extend the mock client**

Store `PreviewRequests`, `QueueRequests`, `QueuedRuns`, `RunByID`, and operation-specific errors. Append requests under a mutex so concurrency tests are race-safe.

- [ ] **Step 5: Run tests**

Run: `go test -race ./internal/azdo`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/azdo
git commit -m "feat: suportar preview e execução de pipelines"
```

### Task 2: Domínio e gates do runner

**Files:**
- Create: `internal/runner/model.go`
- Create: `internal/runner/service.go`
- Create: `internal/runner/service_test.go`

**Interfaces:**
- Consumes: `azdo.Client.PreviewPipeline`, `QueuePipeline`, `GetPipelineRun`
- Produces: `ModeRun`, `ModePlan`, `Selection`, `Review`, `RunResult`
- Produces: `Service.PreviewAll(ctx, selections, parallel) []Review`
- Produces: `Service.QueueAll(ctx, reviews, parallel) ([]RunResult, error)`
- Produces: `Service.Refresh(ctx, runs, parallel) []RunResult`

- [ ] **Step 1: Write failing model tests**

Test these invariants:

```go
selection := runner.Selection{Pipeline: p, Mode: runner.ModePlan, Branch: "main"}
if got := selection.Parameters()["planOnly"]; got != "true" { t.Fatalf("got %q", got) }
```

Normal mode must not send `planOnly=false`; selection identity is pipeline ID; branch defaults to `main`.

- [ ] **Step 2: Write failing gate and concurrency tests**

Use `azdo.MockClient` to prove:

- every selected pipeline is previewed;
- one preview error prevents all queue calls;
- successful queue calls run with maximum observed concurrency `<= parallel`;
- partial queue failure is returned with successful remote run IDs preserved.

- [ ] **Step 3: Run tests and confirm failure**

Run: `go test ./internal/runner`

Expected: failure because package does not exist.

- [ ] **Step 4: Implement the minimal domain**

Use a bounded worker pool, stable result ordering by original selection index, and per-operation `context.WithTimeout(ctx, 30*time.Second)`. `QueueAll` must start with:

```go
for _, review := range reviews {
    if review.State != ReviewReady {
        return nil, ErrPreviewIncomplete
    }
}
```

- [ ] **Step 5: Run race tests**

Run: `go test -race ./internal/runner`

Expected: PASS with no data races.

- [ ] **Step 6: Commit**

```bash
git add internal/runner
git commit -m "feat: adicionar gates de execução múltipla"
```

### Task 3: Catálogo TUI com detalhe progressivo

**Files:**
- Create: `internal/tui/runner/catalog.go`
- Create: `internal/tui/runner/catalog_test.go`
- Create: `internal/tui/runner/styles.go`

**Interfaces:**
- Consumes: `[]azdo.Pipeline`
- Produces: `CatalogModel.Selected() []runner.Selection`
- Produces: `CatalogReviewMsg{Selections []runner.Selection}`

- [ ] **Step 1: Write failing navigation tests**

Construct a model with three pipelines and send `tea.KeyMsg` values. Assert:

- arrows and `j/k` move the cursor within bounds;
- `space` toggles only the active pipeline;
- `p` sets plan and also selects the active pipeline;
- `/` focuses search and filters name, ID, folder, type, repository and tags;
- `esc` clears search before quitting;
- `enter` with zero selections stays in the catalog with a visible warning.

- [ ] **Step 2: Write failing render test for progressive detail**

Assert the active row renders repository, folder and tags exactly once, while inactive rows do not render their secondary details.

- [ ] **Step 3: Run tests and confirm failure**

Run: `go test ./internal/tui/runner -run Catalog`

Expected: failure because the model does not exist.

- [ ] **Step 4: Implement catalog state and filter**

Use `bubbles/textinput` for `/` and branch editing. Keep selection in `map[int]runner.Mode`; derive visible rows from the query on each update; clamp cursor after filtering.

- [ ] **Step 5: Implement responsive rendering**

Render columns `SEL`, `MODE`, `ID`, `TYPE`, `PIPELINE`. Truncate names using visual width and reserve one line below the active item for progressive detail. Footer shortcuts remain visible at terminal heights `>= 12`.

- [ ] **Step 6: Run tests and commit**

Run: `go test -race ./internal/tui/runner -run Catalog`

```bash
git add internal/tui/runner
git commit -m "feat: criar catálogo interativo de pipelines"
```

### Task 4: Revisão, confirmação, execução e demo

**Files:**
- Create: `internal/tui/runner/app.go`
- Create: `internal/tui/runner/context.go`
- Create: `internal/tui/runner/review.go`
- Create: `internal/tui/runner/execution.go`
- Create: `internal/tui/runner/app_test.go`
- Create: `cmd/tui.go`
- Modify: `cmd/root.go`

**Interfaces:**
- Consumes: `runner.Service`, `CatalogReviewMsg`
- Produces: `NewApp(client azdo.Client, project string, pipelines []azdo.Pipeline) AppModel`
- Produces: `NewBootstrapApp(factory ClientFactory, defaults ContextDefaults) AppModel`
- Produces: `NewDemoApp() AppModel`
- Produces: Cobra command `azpipe demo`

- [ ] **Step 1: Write failing workflow tests**

Test full model transitions with `azdo.MockClient`:

```go
model := NewApp(mock, "DEVCLD", fixtures)
model = press(model, "space", "enter")
// wait for preview message
if model.Screen() != ScreenReview { t.Fatal("expected review") }
```

Assert the context screen accepts organization and project without flags, failed preview hides confirmation, `esc` preserves selection, wrong confirmation stays on review, and `q` before queue leaves `len(mock.QueueRequests) == 0`.

- [ ] **Step 2: Write failing demo isolation test**

Inject a client factory that panics if called. Execute `azpipe demo` and confirm the factory is never invoked. The demo review must label all entries `DEMO` and omit execution confirmation.

- [ ] **Step 3: Run tests and confirm failure**

Run: `go test ./internal/tui/runner ./cmd -run 'App|Demo|Root'`

Expected: failure because workflow and command do not exist.

- [ ] **Step 4: Implement context bootstrap**

Use two `textinput.Model` values for organization and project, prefilled from `ContextDefaults`. `Enter` with both values creates the client through `ClientFactory`, lists pipelines and enters the catalog. Missing values or client/list errors stay on the context screen with an actionable error. Secrets never appear in this model.

- [ ] **Step 5: Implement review and confirmation**

Start previews on entry, display `CHECK`, `READY`, or `ERROR`, and accept confirmation only when all reviews are ready and the text input value equals `EXECUTAR`.

- [ ] **Step 6: Implement execution monitoring**

Queue through `runner.Service`, poll non-terminal runs every five seconds, show partial queue failures, and expose run URLs. `q` exits monitoring without calling any cancellation endpoint.

- [ ] **Step 7: Wire root and demo commands**

Set `rootCmd.RunE` to launch the bootstrap TUI only when Cobra resolved no subcommand. Pass configured organization/project as defaults; defer client creation and pipeline listing to the context screen. `azpipe demo` uses fixtures only and works without config.

- [ ] **Step 8: Run tests and commit**

Run: `go test -race ./internal/tui/runner ./cmd`

```bash
git add internal/tui/runner cmd
git commit -m "feat: integrar revisão e execução na TUI"
```

### Task 5: Configuração segura, documentação e verificação final

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Modify: `cmd/auth.go`
- Modify: `README.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Preserves: `AZDO_PAT`, `AZDO_ORG`, `--org`, `--project`
- Changes: `config.yaml` permissions always `0600`

- [ ] **Step 1: Write failing permission test**

Set a temporary home/config directory, call `SetPAT`, then assert:

```go
info, err := os.Stat(configPath)
if err != nil { t.Fatal(err) }
if got := info.Mode().Perm(); got != 0o600 { t.Fatalf("mode=%o", got) }
```

- [ ] **Step 2: Implement restricted permissions and legacy warning**

After `viper.WriteConfigAs`, call `os.Chmod(path, 0o600)`. Change `auth set --pat` help/output to say the persisted PAT is legacy and recommend `AZDO_PAT`.

- [ ] **Step 3: Update documentation**

Document `azpipe`, `azpipe demo`, shortcuts, preview gate, partial queue semantics, authentication recommendation and legacy CLI commands. Add an unreleased changelog entry.

- [ ] **Step 4: Run complete verification**

Run:

```bash
go test -race ./...
go vet ./...
go build ./...
GOOS=linux GOARCH=amd64 go build ./...
GOOS=windows GOARCH=amd64 go build ./...
git diff --check
```

Expected: all commands exit zero.

- [ ] **Step 5: Run safe manual demo**

Run: `go run . demo`

Verify visually: filtering, multi-selection, plan toggle, progressive detail, review, back navigation and safe exit. Confirm the demo never requests credentials and creates no Azure DevOps run.

- [ ] **Step 6: Commit**

```bash
git add internal/config cmd/auth.go README.md CHANGELOG.md
git commit -m "docs: fechar utilização segura da TUI"
```
