# Security boundaries

Never commit PATs, corporate endpoints, real identities or profile data. Inject credentials externally; legacy config persists PATs. Profiles store non-secret parameters in plaintext with private file permissions. Journals contain IDs, state and URLs.

PLAN is an owner-reviewed contract, not a sandbox. Preview validates expanded YAML, not the absence of side effects. External references remain the pipeline owner's responsibility. Leaving the TUI never cancels accepted runs. An uncertain POST must be checked in Azure DevOps before any new submission.

Do not post credentials or exploitable details in public issues. Use a private reporting route only after verifying that the repository exposes one; no private reporting channel or response SLA is currently established in this checkout. Revoke exposed credentials through your provider.

Local tests do not establish live corporate authentication, platform certification or support for older published versions.

Private files use 0600 permissions on Unix and protected DACLs on Windows, allowing
the current user, SYSTEM and administrators. Dedicated settings/profile/run directories
are protected; arbitrary parent directories used by CLI journals are not changed.
Temporary request bodies are protected before writing. Settings and journals use
replacement writes: Unix rename is atomic, while Go makes no atomicity guarantee on
Windows. These permissions do not encrypt plaintext parameters or legacy persisted PATs.
