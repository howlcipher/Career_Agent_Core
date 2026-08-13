# ADR-006: Log Confidentiality Boundary

## Status

**Accepted — implemented 2026-08-13** in `pkg/security/logsafe.go`,
`cmd/dashboard/assist_log.go`, `cmd/assist/main.go` and
`pkg/submitter/assisted_fill.go`. Closes `bugs.md` #543.

## Context

Assisted Apply asks the operator to type answers into a browser Career Agent
opened for them: a salary expectation, a work-authorization answer, a free-text
response to a screening question. Those values are the most sensitive thing this
project touches, and the project's existing position — stated in ADR-002 and
enforced by the fill summary, which keeps labels and discards values — is that
page content does not belong outside the database.

Logs were outside that position, in two ways that a live run found on 2026-08-13.

**The browser's errors carry the page.** Playwright's diagnostics quote the
target element's outer HTML, attributes included. Reproduced directly against a
local synthetic form: a fill that retried against a control already holding a
value produced

```
  - locator resolved to <input readonly id="salary" ... value="<the value>"/>
```

Any code rendering such an error with `%v` writes the operator's answer to
stderr. `pkg/submitter/assisted_fill.go` did exactly that on the path that
commits the operator's answers, because `safeFillWithLabelFallback` wraps the
driver's error with `%w` rather than replacing it.

**The dashboard persisted an entire child process's stderr.** The dashboard
launches `cmd/assist` and echoed every line of its stderr into its own log,
which the operator redirects to `dashboard.log`. That stream is not Career
Agent's alone: it carried Playwright's multiline diagnostics, and
`cmd/assist` was inheriting a Chromium process's stderr into it as well. What
reached the file was therefore decided by this project's dependencies, not by
this project.

## Decision

**Assisted Apply child-process diagnostics may never cause transient operator
answers or DOM values to be persisted in `dashboard.log`.** More generally,
arbitrary child stderr is never blindly persisted.

Two mechanisms, at opposite ends of the stream, both required.

**1. Producers report a bounded reason, not the error.**
`security.BrowserFailureReason` maps any browser automation error to one code
from a closed vocabulary (`browser_timeout`, `ambiguous_target`,
`element_not_interactable`, `target_missing`, `navigation_failed`,
`browser_closed`, `browser_driver_unavailable`, `unclassified`). Matching on the
error's first line is a routing decision only: the value returned is always a
constant declared in `logsafe.go`, so the output cannot contain page text
however the input is worded. This is the same shape as
`security.NetworkRejectionReason`, which ADR-002's guard work established for
the same reason, and every assisted-path call site that could hold a Playwright
error now uses it.

**2. The dashboard persists only records written through the child's own
standard logger.** `security.SanitizeChildLogLine` admits a line only if it
carries the prefix Go's standard logger writes, then strips markup from the
remainder and bounds its length. A qualifying record is logged; everything else
is counted and dropped. Readiness is decided separately, from the raw line in
memory, so the `"Assisted application is open."` contract survives a filter that
does not know it is special. `cmd/assist` additionally discards the Chromium
process's own streams rather than inheriting them.

Note the precise scope, because it is narrower than "Career Agent's own
records": the admission test is *how* a line was written, not *who* wrote it. A
library linked into `cmd/assist` that logs through the same standard logger
produces records this filter cannot distinguish from the command's own. That is
observed, not hypothetical — `playwright-go`'s installer emits
`INFO Downloading browsers...` this way, and those lines are persisted. It is
the correct outcome rather than a gap: such records are in-process status
messages, and they are still subject to the markup strip and the length bound,
which is what actually protects the page content. What the rule reliably
excludes is the separate byte stream — a subprocess writing to the inherited
descriptor, and every unprefixed continuation line of a multiline diagnostic,
which is where the element HTML lives.

### Why not a blocklist

The obvious fix is to strip `value="..."`, `<input`, `locator resolved to` and
similar. Rejected: it is a list of one dependency's current phrasing, it fails
silently and invisibly the first time Playwright rewords a diagnostic, and
nothing in the test suite would notice. Both mechanisms above decide by
structure — a closed output vocabulary, and a record shape this project
controls — so a wording change costs diagnostic precision and never
confidentiality.

### Why timestamp filtering alone is not enough

Filtering to timestamped records is the fix `bugs.md` #543 proposed, and against
every Playwright error shape captured while reproducing it (retry timeout,
strict-mode violation, pointer interception, detached locator) it would have
been sufficient: all four put the element HTML on an unprefixed continuation
line. It is still not enough on its own, because the first line of an
application-owned record is written by Career Agent, and Career Agent was
embedding the raw error in it. A producer regression, or a driver that one day
emits a single-line diagnostic, defeats the prefix rule from inside. Hence the
producer half, and hence the markup pass on records that do qualify.

## Consequences

* `dashboard.log` no longer contains Playwright call logs, DOM fragments or
  Chromium's output. It keeps the operational narrative — launched, ready,
  filled *n* fields, questions surfaced, answers entered, confirmed, closed,
  exited — plus a bounded reason on failures, and a count of what was withheld.
  Confirmed against a real assisted run on 2026-08-13: the readiness sentinel
  and the browser-closed record both persisted, and a categorical scan found no
  markup, no `locator resolved to`, no `value="..."` attribute, no `Call log:`
  block and no unprefixed continuation line.
* Diagnosing an assisted failure from the log now yields a category rather than
  a stack of driver detail. Reproducing with the driver's own output requires
  running `cmd/assist` directly, where nothing persists its stderr.
* An unclassified reason code is the signal that
  `browserFailureSignatures` needs a new entry. Adding one is safe by
  construction: the vocabulary is the output, so an entry can only improve
  precision.
* The reader is deliberately not `bufio.Scanner`. Scanner fails the whole stream
  on a token larger than its buffer, and a stream this project stops reading is
  a child process blocked on a full pipe. `readBoundedLine` truncates and keeps
  going.
* No raw copy is written anywhere. The filter reduces the number of places the
  stream lands; it does not move it.
