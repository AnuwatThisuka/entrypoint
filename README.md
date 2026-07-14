<h1 align="center">entrypoint</h1>

<p align="center">
  <strong>Lightweight, ticket-linked context checkpoints for AI coding agents —
  built for resuming, not replaying.</strong>
</p>

<p align="center">
  <a href="https://github.com/AnuwatThisuka/entrypoint/actions/workflows/ci.yml"><img src="https://github.com/AnuwatThisuka/entrypoint/actions/workflows/ci.yml/badge.svg" alt="ci"/></a>
  <a href="https://github.com/AnuwatThisuka/entrypoint/actions/workflows/release.yml"><img src="https://github.com/AnuwatThisuka/entrypoint/actions/workflows/release.yml/badge.svg" alt="release"/></a>
</p>

`entrypoint` captures a short, structured summary of _why_ an agent made the
changes it made, links it to the ticket that motivated the work, and stores
it inside your own git repo. When an agent starts a new session, it reads the
latest packet instead of re-parsing a full transcript.

It ships as a **single static binary** — no Node.js, no Bun, no
`node_modules`. Install with one `curl | sh` or `brew install`.

## Why not just use Entire?

[Entire](https://entire.io) already does agent-session-to-git checkpointing,
and does it well — multi-agent support, real-time capture, rewind, line-level
blame. If you need that, use Entire.

`entrypoint` exists for a narrower case: teams who want the _resume_ benefit
without the storage and privacy tradeoffs of full transcript capture.

|                                         | Entire                           | entrypoint                                                    |
| --------------------------------------- | -------------------------------- | ------------------------------------------------------------- |
| What's stored                           | Full session transcript          | A short packet: goal, decisions, state, next steps            |
| Public repo behavior                    | Transcript is visible to anyone  | Detail is automatically redacted by default                   |
| Ticket context                          | Not built in                     | `goal` and `ticket` are first-class fields                    |
| Resume cost                             | Agent re-reads a long transcript | Agent reads ~20 lines                                         |
| Line attribution                        | Every line, in real time         | Diff-hunk blocks, computed at capture time (last commit only) |
| Search                                  | Semantic search over transcripts | Keyword search over goals/decisions (business-readable)       |
| Multi-agent, rewind, distributed mirror | Yes                              | Not in scope — see below                                      |

`entrypoint` deliberately does **not** try to match Entire's full feature
set. Out of scope, on purpose: multi-agent support beyond Claude Code,
real-time / concurrent session tracking, file-level rewind, distributed git
mirroring, two-way ticket sync, and embedding/semantic search. It's a resume
tool, not a session recorder.

## How it works

```
1. You work with an agent on a ticket.
2. On session end (or manually), entrypoint captures a packet:
   - goal:      why this work is happening
   - decisions: key choices and why
   - state:     what's done / in progress / next
   - ticket:    linked issue, auto-fetched title if available

3. The packet is written to `refs/notes/entrypoint` and linked to the
   commit via an `Entrypoint-Packet: <id>` trailer — so it survives
   rebase and cherry-pick, and usually squash: git's default of
   concatenating squashed messages keeps the trailer, but if you
   rewrite the squashed message from scratch and drop the trailer
   line, the link is lost.

4. Next session: `entrypoint resume` prints the latest packet.
   The agent picks up where it left off, cheaply.
```

> **⚠️ capture rewrites HEAD.** To attach the `Entrypoint-Packet` trailer,
> `entrypoint capture` amends the HEAD commit message — HEAD gets a new SHA.
> Capture **before** you push. If HEAD was already pushed, your next push
> will need `git push --force-with-lease` (or `--force`). Capture warns you
> inline with the exact command when this applies.

## Install

### Homebrew (macOS / Linux)

```bash
brew install AnuwatThisuka/tap/entrypoint
```

### curl | sh

```bash
curl -fsSL https://raw.githubusercontent.com/AnuwatThisuka/entrypoint/main/install.sh | sh
```

Detects your OS/arch, verifies the download against the release
`checksums.txt`, and installs to `/usr/local/bin` (falling back to
`~/.local/bin` if that isn't writable). Override with env vars:

```bash
curl -fsSL https://raw.githubusercontent.com/AnuwatThisuka/entrypoint/main/install.sh \
  | ENTRYPOINT_VERSION=v0.1.0 ENTRYPOINT_BINDIR="$HOME/bin" sh
```

### From source (Go 1.24+)

```bash
go install github.com/AnuwatThisuka/entrypoint/cmd/entrypoint@latest
```

### Verify

```bash
entrypoint --version
```

> **PATH note.** If `entrypoint --version` prints `command not found` right
> after install, the install directory isn't on your `$PATH`. `curl | sh`
> uses `/usr/local/bin` or `~/.local/bin`; `go install` uses
> `$(go env GOPATH)/bin`. Add the relevant one:
>
> ```bash
> echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc   # then restart the shell
> ```

## Usage

```bash
# Capture a packet manually
entrypoint capture --ticket 456 --goal "Fix timeout on bulk export"

# Resume the latest packet on the current branch
entrypoint resume

# Resume a specific historical version
entrypoint resume --at 3

# List all packets on this branch
entrypoint log

# Find out why a block of code exists
entrypoint blame src/db/batch.js:50

# Record files as agent-written for this session
# (normally done by the Claude Code hook, not by hand)
entrypoint track src/db/batch.js

# Search past decisions across tickets
entrypoint why "timeout"

# Pull packets from a teammate — fetches only refs/notes/entrypoint, never code
entrypoint sync

# Share your packets — wraps: git push origin refs/notes/entrypoint
entrypoint sync --push

# Sign packets with your existing git GPG key (opt-in)
entrypoint capture --sign

# Check signatures on this branch
entrypoint verify

# Export a date range for an auditor
entrypoint report --from 2026-01-01 --to 2026-03-31 --format pdf

# Show HEAD's packet/push state and the last auto-capture outcome
entrypoint status

# Diagnose common problems; each failing check prints the exact fix
entrypoint doctor
```

`sync` moves packets in one direction per invocation: plain `entrypoint sync`
is pull-only (it will never touch your branches or working tree), and
`entrypoint sync --push` publishes your local packets. A typical team loop is
`capture` → `sync --push` on one machine, `sync` → `resume` on the other.

## Auto-capture (Claude Code)

`entrypoint` bundles the Claude Code hooks as subcommands, so ending a
session writes a packet with no manual CLI invocation. On session end the
agent that just finished is asked to summarize the session (goal, decisions,
next steps) as a short JSON reply — the transcript itself is never stored.

Add to `.claude/settings.json`:

```json
{
  "hooks": {
    "SessionEnd": [
      { "hooks": [{ "type": "command", "command": "entrypoint hook" }] }
    ],
    "PostToolUse": [
      {
        "matcher": "Write|Edit|MultiEdit|NotebookEdit",
        "hooks": [
          { "type": "command", "command": "entrypoint hook track" }
        ]
      }
    ]
  }
}
```

The `PostToolUse` entry feeds the session file-write log that `entrypoint
blame` uses to tell agent-written hunks from human edits. It's optional —
without it, captured blocks are tagged `human`.

The hook never blocks session teardown: if `claude` can't be resumed or the
summary is unusable, it logs a one-line notice and exits cleanly. You can
also run the same path by hand: `entrypoint capture --auto --session-id <id>`.

## Packet format

```json
{
  "id": "pk_9f3c2a1b",
  "version": 4,
  "createdAt": "2026-07-14T09:30:00.000Z",
  "branch": "fix/bulk-export-timeout",
  "ticket": {
    "id": "#123",
    "title": "Fix timeout on bulk export",
    "source": "github"
  },
  "goal": "Reduce export timeout by batching DB queries",
  "state": {
    "done": ["Added batch query function", "Updated tests for batch=100"],
    "inProgress": "Tuning batch size, currently testing 500 vs 1000",
    "next": ["Benchmark with prod-like data", "Update docs"]
  },
  "decisions": [
    "Chose batch over streaming because the DB driver doesn't support cursors well"
  ],
  "filesTouched": ["src/export.js", "src/db/batch.js"],
  "blocks": [
    { "file": "src/db/batch.js", "range": "L45-L60", "type": "agent" }
  ],
  "visibility": "full",
  "signature": "-----BEGIN PGP SIGNATURE-----\n..."
}
```

Packet ids are random (`pk_` + 8 hex chars); `version` is what counts up per
branch. On a public GitHub repo, `state` detail and `openQuestions` are
automatically stripped before being written — `goal`, `decisions`, and
file-level info (`filesTouched`, `blocks`) are kept. Visibility is detected
once per capture via `gh repo view`; if it can't be determined, the packet is
redacted (fail-safe). The optional `signature` field is only present when you
pass `--sign` at capture time. Signing happens _after_ redaction, so the
signature always covers exactly the packet body that gets stored — on a
public repo that's the redacted version.

## For teams with compliance needs

`entrypoint` stays a resume tool first — nothing below changes default
behavior. But because packets already capture _why_ a change happened and
link it to a ticket, they double as a lightweight audit trail if you're a
small team starting to face SOC 2 / customer security questionnaires and
don't need enterprise-grade governance yet:

```bash
# Sign packets with your existing git GPG key (opt-in, not the default)
entrypoint capture --sign

# Check that signed packets haven't been altered since signing
entrypoint verify --branch main

# Export a date range as a report for an auditor
entrypoint report --from 2026-01-01 --to 2026-03-31 --format pdf
```

`verify` walks one branch (current, or `--branch`); `report` covers packets
reachable from any branch in the date range.

A caveat worth knowing before you lean on this in an audit: `verify` is an
integrity check on _signed_ packets — it detects a signed packet whose
content changed after signing. It cannot detect a signature (or a whole
packet) being _removed_: a stripped signature just reads back as "unsigned".
It proves what a signature covers, not that signing ever happened. If a
signer's public key isn't in your keyring, the packet is reported as "can't
verify" — import their key to check it; that's not the same as tampered.

This is intentionally light: no RBAC, no enforced retention policy, no
certification of `entrypoint` itself. If you need real enterprise governance
(access control, mandated retention, vendor certification), this isn't that
tool.

## Troubleshooting

`entrypoint status` and `entrypoint doctor` cover most of these — `status`
shows current HEAD/push state and the last hook outcome; `doctor` runs
non-interactive checks and prints the exact fix command for each failure.

| Issue                                             | Likely cause                                                   | Solution                                                                                                                                        |
| ------------------------------------------------- | -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `entrypoint: command not found` after install     | Install dir not on `$PATH`                                     | Add it — see the **PATH note** in [Install](#install).                                                                                          |
| `capture` "did nothing" / hook skipped silently   | No new commit since the last packet — HEAD already carries one | Run `entrypoint status`; it names the reason. Make a new commit, then capture (or `entrypoint capture --force` to replace the packet on HEAD).  |
| Auto-capture keeps skipping                       | Agent couldn't be resumed or its summary was unusable          | `entrypoint status` shows the specific reason from the last hook run. Capture manually: `entrypoint capture --auto --session-id <id>`.          |
| Push rejected after `capture`                     | `capture` amended a commit you'd already pushed (new SHA)      | Capture warns inline with the exact command: `git push --force-with-lease <remote> <branch>`.                                                   |
| Ticket titles don't auto-fill                     | `gh` not installed or not authenticated                        | `entrypoint doctor` flags it; run `gh auth login`.                                                                                              |
| Teammate can't see your packets                   | Notes ref not pushed                                           | `entrypoint doctor` flags a local/remote mismatch; run `entrypoint sync --push` (or `entrypoint sync` to pull).                                 |

## Contributing

Ground rules specific to this codebase: packets stay small, no raw
transcript storage, the privacy check runs before every packet write, and
new ticket sources plug into the adapter interface in `internal/ticket`. New
commands need an end-to-end test against a scratch git repo.

Common tasks:

```bash
go build ./...        # compile
go test ./...         # run the suite (unit + end-to-end against scratch repos)
go vet ./...          # vet
golangci-lint run     # lint
```

Layout: `cmd/entrypoint` is the binary entrypoint; command wiring lives in
`internal/cli`, core logic in the other `internal/*` packages, and
integration tests in `test/e2e`.

## License

MIT — see [LICENSE](./LICENSE).
