# Third-Party Notices

`entrypoint` / `entrypointd` is distributed as a single static binary. That
binary links the Go modules listed below. Each is redistributed under its own
license; all are permissive (MIT / BSD-3-Clause / Apache-2.0) and none are
copyleft.

## Provenance (Invariant I7)

- **No code in this repository is derived from Entire
  (`github.com/entireio/cli`) or any other agent-checkpointing product.**
  entrypoint is a clean-room implementation; see `NOTICE`.
- The dependencies below are ordinary upstream libraries, unmodified, pulled via
  Go modules. They are not vendored source and are not derived work.
- If a future phase introduces code derived from a third party, add its per-file
  license header and record it in this file **before merge**.

## Direct dependencies

| Module | Version | License |
|--------|---------|---------|
| github.com/spf13/cobra | v1.10.2 | Apache-2.0 |
| modernc.org/sqlite | v1.53.0 | BSD-3-Clause |

## Indirect dependencies

| Module | Version | License |
|--------|---------|---------|
| github.com/dustin/go-humanize | v1.0.1 | MIT |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/inconshreveable/mousetrap | v1.1.0 | Apache-2.0 |
| github.com/mattn/go-isatty | v0.0.20 | MIT |
| github.com/ncruces/go-strftime | v1.0.0 | MIT |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| github.com/spf13/pflag | v1.0.9 | BSD-3-Clause |
| golang.org/x/sys | v0.44.0 | BSD-3-Clause |
| modernc.org/libc | v1.73.4 | BSD-3-Clause |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause |
| modernc.org/memory | v1.11.0 | BSD-3-Clause |

Full license texts ship with each module in the Go module cache and are
reproduced by the upstream projects. Regenerate this table from `go.mod` after
any dependency change.
