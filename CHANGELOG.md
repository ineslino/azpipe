# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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
- Every persisted `~/.config/azpipe/config.yaml` is written with `0600` permissions
- `azpipe auth set --pat` is documented and labelled as legacy; `AZDO_PAT` or external
  credential injection is recommended
