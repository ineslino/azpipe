# Readiness and publication

## Evidence layers

Source changes, unit/fixture checks, native execution, live Azure integration,
assistive technology, GitHub integration and installed runtime are separate results.
A passing build or cross compilation does not establish the other layers.

The [acceptance guide](qa.md) defines repeatable checks and the remaining manual gates.
CI has six native jobs: Linux, macOS and Windows, each amd64/arm64. Jobs exercise PTY
or ConPTY, build metadata, private-file protection and archive extraction/checksums.
Go 1.27.1 is the validation toolchain; the project minimum remains Go 1.26.3.
Race is required on five targets and N/A on Windows ARM64, which Go does not support.

```bash
go test -race ./... -count=1
go vet ./...
go mod verify
govulncheck ./...
git diff --check
```

`govulncheck` is pinned to v1.8.0 in CI. Its result concerns reachable calls;
non-called module/package findings are reported separately, not treated as absent.
`make lint` remains an optional separately installed golangci-lint command.

## UX score

The critique uses ten Nielsen heuristics with scores 0–4. Display the normalized
percentage: `total / applicable maximum × 100`. The original 33/40 is **82.5/100**;
changing the denominator does not improve the product. UX scores are judgments
with evidence and confidence, not an automated certification or a guarantee.

Full readiness also requires real Azure QA, WSL and VoiceOver/Narrator/Orca journeys.
Missing evidence is pending. Do not publish a maximum score while required evidence
or blocking defects remain.

## Local data

Settings and TUI journals use protected temporary files and replacement writes.
Unix uses 0600 files and 0700 dedicated directories; Windows uses a protected DACL
for the current user, SYSTEM and administrators. Request bodies are protected before
writing data. CLI journal writes preserve the caller's directory permissions.
Go guarantees atomic rename on Unix, not Windows. Native Windows tests establish
replacement and ACL behaviour, not crash-proof atomicity. No file stores secrets
except explicitly persisted legacy PAT configuration; profiles remain plaintext.

## Distribution and installation

GoReleaser v2.18.2 validates the six release targets and creates snapshot archives
and checksums in CI without publication. Native CI archives additionally verify the
extracted executable on its own target. Snapshot packaging skips the existing
`before` hook so validation does not rewrite dependency metadata.

`make release` publishes and requires explicit authorization for that separate action.
Merging code does not create a release. `go install ...@latest` fetches published code;
use a clean build of the merged revision to reinstall it locally.

```bash
azpipe --version
go version -m "$(command -v azpipe)"
git rev-parse origin/main
```

The installed `vcs.revision` must equal the intended remote revision and
`vcs.modified=false`. Repeat native acceptance checks against that exact executable.
Retain the previous binary for rollback. Release publishing, cleanup of live QA
resources and purchases are separate actions.

## Documentation decisions

The existing MIT licence, CLI contracts, PLAN safeguards and local formats remain.
Tech documentation stays in the repository. Prior design/plan files are historical;
[usage.md](usage.md) describes the operational contract. Documentation assets are
fictional model views and are not proof of live Azure executions.
