# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Full focused-value detail in F2 parameter choices, with PgUp/PgDn inspection before selection and a regression for long identical prefixes.
- `--version` with commit and working-tree metadata; six native CI targets, PTY/ConPTY acceptance checks, vulnerability scans and verified snapshot archives.
- Confined live Azure QA fixtures and a preflight helper for a private disposable project; real integration and assistive technology remain separate evidence gates.
- Branch browser (`azpipe branches`, catalog `B`) with creator/name filters, repository selection, offline demo and guarded multi-delete. CLI `branches list` and preview-first `branches delete` share default-branch, policy, lock, active-PR and SHA checks.
- Documented user-local installation, persistent PATH setup and offline verification.
- Contextual action menu (`a` / `?`), focused footer, next-step guidance and review error recovery.
- Responsive AZPIPE block banner in the catalog and offline demo.
- Typed pipeline parameter forms, context-isolated profiles and resumable batch journals.
- Explicit owner-reviewed PLAN contracts, source pinning and expanded-YAML revalidation.
- CLI batch preview/execution, optional external authentication adapter and offline demo fixtures.
- Framed TUI tables, AZPIPE welcome banner, bilingual READMEs and reproducible demo images.
- Interactive `azpipe` pipeline runner with catalog filtering, multi-selection,
  branch and `PLAN` selection, preview review, confirmation, and run monitoring
- `azpipe demo` offline runner with local fixtures and no Azure DevOps client or runs
- `azpipe projects list` — list all projects in an org
- `azpipe repos list` — list repositories in a project
- `azpipe repos pipelines <repo>` — show pipelines linked to a repository
- `azpipe pipelines list` — list all pipelines in a project
- `azpipe pipelines runs <id>` — show last N runs with status and duration
- `azpipe pipelines analyze <id>` — avg duration, failure rate, top failing stage, flaky stages
- `azpipe pipelines watch <id>` — live-poll active run with bubbletea TUI
- `azpipe auth set` — store PAT and default org/project in config file
- `--output table|json|plain` global flag for all listing commands
- `--org`, `--project` global flags; `AZDO_PAT`/`AZDO_ORG` env var support

### Changed
- Profile errors keep recovery actions visible, catalog search uses adaptive text styles, and simple parameter types use Portuguese labels without changing their schemas.
- Review details use available terminal height while preserving confirmation and help; resizing and command bars retain the space reserved for the surrounding frame.
- Parameter edits now follow changes in the visible value, including Ctrl+W/Ctrl+H; schema reads cancel with Esc or a superseding action and discard late forms.
- Reopening the same context retains preparation; reauthentication uses the current client. Switching context explains the reset, and an invalid profile no longer blocks another valid choice.
- Search accepts typed/pasted command bursts; review states use Portuguese labels, and batch/persistence diagnostics remain fully inspectable with the existing detail controls.
- Preview now reports per-pipeline progress, supports cancellation and enforces the shared 500-pipeline limit. Execution remains blocked until every preview succeeds.
- Parameter errors focus the invalid field; F2 inspects complete choices without altering defaults. Search Esc retains the filter; saved batches show pipeline names, saved states and last-updated time.
- Settings, profiles, journals and request bodies use private Unix permissions or Windows DACLs. Replacement writes preserve formats; atomic rename is guaranteed only on Unix.
- Refined the TUI palette for light/dark terminals, with full-row focus, consistent tables and semantic RUN/PLAN/status colours. Active pipeline details now include folder and tags, while existing shortcuts and confirmation gates remain unchanged.
- Demo GIF/MP4 and PNGs now render the model's ANSI colours instead of inferring row colours from text.
- Every persisted `~/.config/azpipe/config.yaml` is written with `0600` permissions
- `azpipe auth set --pat` is documented and labelled as legacy; `AZDO_PAT` or external
  credential injection is recommended
