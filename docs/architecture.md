# Architecture

This document is the running architectural record for `entrypoint` /
`entrypointd`. The authoritative, forward-looking specification is
[`IMPLEMENT_PLAN.md`](../IMPLEMENT_PLAN.md); this file summarizes the target
shape and appends a short note per phase describing **what changed and why**
(required by the plan's Definition of Done).

## Target shape

See [`IMPLEMENT_PLAN.md` §1 — Target architecture](../IMPLEMENT_PLAN.md#1-target-architecture)
for the full diagram. In brief:

- **Agent/dev machine (peer):** the `entrypoint` CLI captures agent work into
  **git refs (source of truth)** and enqueues events into a local SQLite outbox
  that drains over the Meshwire (WireGuard) tunnel.
- **Server (`entrypointd`, peer):** verifies peer identity (mTLS client cert +
  short-lived signed token — **not** IP range), writes to **git first**, then
  updates a **derived, rebuildable** SQL index. A read-only dashboard, API, and
  signed audit reports read only from that index.
- **Meshwire is transport, not trust.** The encrypted tunnel provides
  connectivity; identity and authorization live above it.

The non-negotiable invariants (I1–I8) that constrain every phase are in
[`IMPLEMENT_PLAN.md` §0](../IMPLEMENT_PLAN.md#0-non-negotiable-invariants-guardrails).

## Change log

### Phase 0 — Repository preparation

- Added `NOTICE` and `THIRD_PARTY.md`. Provenance audit found **no code derived
  from Entire (`github.com/entireio/cli`)**; entrypoint is a clean-room
  implementation, recorded explicitly per Invariant I7. Third-party module
  licenses (all permissive: MIT / BSD-3-Clause / Apache-2.0) are catalogued.
- Extended CI (`.github/workflows/ci.yml`) with a `goreleaser release --snapshot`
  cross-compile dry run so linux/darwin × amd64/arm64 stays green every phase
  (Invariant I5).
- `go.mod` already targets a Go version at or above the plan's `go 1.24` floor;
  left unchanged (no downgrade, per plan task 1).
- Added this document.

### Phase A — Normalized core + importers

- Added `internal/checkpoint`, the normalized core domain:
  - `model.go` — `Session`, `Summary`, `Source`, `Commit`, `Ticket`,
    `Visibility`, `DeriveID`, `Normalize`, `ErrIncomplete`. `DeriveID` is a
    content hash over `(importer, native id)` giving a stable `Session.ID` for
    idempotent upsert / at-least-once ingest dedup. `Normalize` fails
    visibility *safe* (unknown → redacted, I3) and normalizes timestamps to UTC.
  - `importer.go` — `Importer`, `RawSession` (thin: identity + lazy blob
    reader, so transcripts stay by-reference, I3), `Registry` (dispatch by
    importer name).
  - `index.go` — `Index`, `Query`, `ErrNotFound` (interface contract only;
    the SQLite implementation lands in Phase C).
- Added importers under `internal/importer` (the core imports none of them —
  I4, verified via `go list -deps`):
  - `entire` — maps Entire's `metadata.json` and is the *only* package that
    names `metadata.json`/`prompt.txt`/`full.jsonl`; raw prompt/transcript
    blobs are never read (I3), only exposed as `ByReferenceFiles`. Unknown
    metadata keys are preserved into `Session.Extra`.
  - `entrypoint` — maps the native `internal/packet.Packet` (reconciling the
    two domains into one normalized type); `FromPacket` is the seam for the
    capture path, native-only fields (version, inProgress, blocks) preserved
    into `Extra`, GPG signature intentionally not indexed.
  - `registry.go` (`package importer`) — `Default()` wires both importers;
    the dependency arrow is `checkpoint <- entire/entrypoint <- importer <- cmd`.

### Phase B — Git as source of truth (ref reader/writer)

- Added `internal/gitstore` (`RefWalker`), pure go-git, never shelling out and
  never touching the working tree or code branches:
  - `Fetch` — single-ref fetch via an anonymous remote (one refspec only).
  - `Walk` — `iter.Seq2[RawSession, error]`, one session per top-level subtree;
    `RawSession.File` lazily reads sibling blobs so transcripts are never read
    eagerly (I3).
  - `CommitLinks` — read-only log scan mapping native record id → code commit
    sha, recognizing `Entrypoint-Packet` and `Entire-Checkpoint`/`Entire-Metadata`
    trailers.
  - `Rebuild` — walk → map via `Registry` → overlay `Commit.SHA` from links
    when empty (fills the gap `FromPacket` left). Shared primitive for Phase C's
    `rebuild-index` and Phase E's ingest.
  - `WritePacket` — writes entrypoint's own records to `refs/entrypoint/packets/v1`
    as `<id>/packet.json`, storing the **full body including the GPG signature**
    so records stay verifiable from git ("not indexed" ≠ "discarded"). Builds
    blob/tree/commit objects and moves the ref via the object store; no checkout.
- Extended `internal/trailer` with `Entire-*` keys and `ParseLinkedIDs`.
- **go-git version deviation:** the plan names go-git **v6**, but v6 has no
  stable release (alpha only). We depend on the stable **v5** line — equally
  pure-Go / no-CGO, so I5 holds. CGO-off cross-compile for linux/darwin ×
  amd64/arm64 verified.
- The existing notes-based capture and `entrypoint verify` paths are left
  untouched; Phase B only adds a ref reader/writer alongside them.
