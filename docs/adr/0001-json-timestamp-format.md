# ADR 0001: JSON timestamps are UTC RFC 3339 with whole seconds

- **Status:** Accepted
- **Date:** 2026-09-22
- **Introduced in:** #6

## Purpose

`prog`'s `--json` output exists so scripts and agents can process tasks, logs,
and learnings with standard tools, jq in particular. Timestamps are the field
those tools most often compute with: "what hasn't been touched in 30 days" is
the canonical query. That only works if every timestamp, from every command,
parses with the same tool call.

Before this decision, logs came out as UTC (`2026-08-26T02:38:39Z`),
learnings kept their stored local offset (`2026-01-09T15:12:45-06:00`), and
items had no timestamps at all. jq's `fromdateiso8601` rejected the learning
form, so a timestamp-based pipeline broke depending on which command fed it.

## Decision

Every timestamp in JSON output is formatted as RFC 3339 in UTC with whole
seconds, e.g. `2026-01-09T21:12:45Z`.

The rule lives on a type, `Timestamp` in `cmd/prog`, whose `MarshalJSON`
applies it. JSON output structs declare time fields as `Timestamp`.

## Scope

Covers every `*_at` field emitted by `--json` on any command. Today that is
`list`, `show`, and `context`.

Non-goals:
- **Database storage format.** How timestamps are stored in `~/.prog/prog.db`
  is a separate decision. Changing storage MUST NOT change this JSON format.
- **Human-readable text output.** Non-JSON output may use any format,
  including local time.
- **Which commands expose timestamps.** `ready --json` omits them by design;
  adding timestamps to a command is allowed and MUST follow this format.

## Behavior Contract

- JSON timestamps MUST be RFC 3339 strings ending in `Z` (UTC).
- JSON timestamps MUST have whole-second precision; they MUST NOT contain a
  fractional-seconds component.
- JSON timestamps MUST NOT contain a numeric offset such as `-06:00`, even
  when the stored value carries one.
- Every JSON timestamp MUST be accepted by jq's `fromdateiso8601`.
- Formatting MUST preserve the instant: converting to UTC and truncating to
  the second is the only transformation allowed.
- New timestamp fields in JSON output structs MUST use `Timestamp`, not
  `time.Time` or `string`.

## Why this form

jq 1.6's `fromdateiso8601` only accepts the pattern `%Y-%m-%dT%H:%M:%SZ`.
That pattern rules out numeric offsets and fractional seconds, which is why
both are forbidden rather than merely discouraged. Most other consumers (Go,
JavaScript `Date`, Python 3.11+ `datetime.fromisoformat`, `date -d`) also accept
this form, so it is the most widely parseable choice rather than a
jq-specific one.

Alternatives considered:

- **Local time with offset.** Preserves the author's wall clock, but jq
  rejects it, and consumers comparing timestamps would have to normalize
  anyway.
- **Plain `time.Time` fields.** Go's built-in `MarshalJSON` emits RFC 3339
  with nanoseconds (`…45.123456789Z`), which jq rejects. It is the most
  tempting shortcut and the one this decision most needs to prevent.
- **Unix epoch integers.** jq handles these without parsing, but they are
  unreadable in raw output and lose self-description. Rejected for human
  and agent readability.
- **A shared formatting helper.** Fixes existing call sites, but a new field
  is correct only if its author knows the helper exists. A type puts the
  rule on the field declaration, where copying a neighboring field gets it
  right.
- **A lint/AST test that rejects `time.Time` in JSON structs.** Would close
  the remaining gap mechanically. Rejected as more machinery than the risk
  warrants; the type plus review is the accepted barrier.

## Consequences

- Sub-second precision is dropped. Events within the same second get equal
  timestamps; consumers that need ordering within a second should rely on
  array order, not timestamps.
- The author's local time is not recoverable from JSON. Consumers that
  display local time must convert.
- A field declared as `time.Time` in a JSON struct compiles and looks
  correct but violates this contract. Review is the only guard.

## Verification

- `go test ./cmd/prog -run TestTimestamp` asserts that a `-06:00` time with
  nanoseconds marshals to exactly `"2026-01-09T21:12:45Z"` and round-trips
  to the same instant.
- Against a real database, this must produce no errors and no output:

  ```bash
  prog list --json | jq -r '.[] | .created_at, .updated_at | fromdateiso8601' >/dev/null
  prog context -p <project> -c <concept> --json |
    jq -r '.[].created_at | select(endswith("Z") | not)'
  ```
