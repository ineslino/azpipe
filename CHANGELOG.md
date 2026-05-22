# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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
