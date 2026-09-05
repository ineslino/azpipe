# Readiness and publication

This documentation describes the local checkout, including uncommitted work. It is not evidence that GitHub, an installed binary or a corporate wrapper contains these features.

## Quality gates

Local verification on 2026-09-05: `go test -race ./...`, `go vet ./...`, `go build ./...` and `git diff --check` passed. The repository-readiness structural checker passed after documentation updates. Root, batch and resume help were checked against documented flags. Hero and model SVGs were decoded and visually inspected in the browser. No real Azure DevOps runs were submitted.

```bash
go test -race ./...
go vet ./...
go build ./...
git diff --check
```

Run the offline walkthrough in [demo.md](demo.md). Tests cover models, local HTTP fixtures and execution gates. Native Windows/WSL, live Azure DevOps permissions, credential renewal and corporate integration remain manual checks.

`make lint` requires separately installed golangci-lint; no version is pinned. It is not part of the current CI workflow and is not a substitute for the checks above.

## Release boundary

[GoReleaser configuration](../.goreleaser.yaml) targets Linux, macOS and Windows, amd64/arm64. Packaging configuration is not evidence of an available release. `make release` invokes a publishing command and must not be used as a local check. No release or remote mutation is performed by these documentation changes.

Before publishing, verify the intended account, remote, branch, licence, diff and metadata; obtain authorization for the exact publication. Installation with `@latest` fetches published code, not local work.

## Documentation decisions

- Existing MIT licence preserved without modification.
- CONTRIBUTING and SECURITY are useful for operational safeguards.
- CHANGELOG retains an Unreleased section.
- No CODE_OF_CONDUCT added: no additional community moderation process is established.
- Historical design/plan retained and marked superseded; [usage.md](usage.md) describes the current operational contract.
- Hero is original SVG, small and scalable; demo SVGs are generated from real model views using fictional fixtures.
- Metadata remains an [unapplied proposal](repository-metadata.md).
