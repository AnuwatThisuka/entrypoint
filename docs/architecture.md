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
