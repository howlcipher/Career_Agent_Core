# bugs.md — Archived Full Accounts for Closed (Done/Resolved/Closed) Items

Full fix narratives for closed bug rows, moved out of `bugs.md`'s ranked-table rationale cells and `### N.` Details sections during the 2026-08-01 backlog-size restructure. `bugs.md` keeps only a one-line pointer for each closed item; this file has the full account for audit purposes.

## 546. The fill path logs the operator's own address values, and nothing downstream strips them

**Found and fixed 2026-08-14**, by the adversarial pass of the audit of the first two real
applications. Not found by a test — the value only ever appears at the instant a combobox selection
commits against a live browser, so every unit test in the repository passed over it.

**The defect.** `fillComboboxFromCandidates` (`pkg/submitter/browser.go`) logged the value it had just
typed:

```go
log.Printf("[Auto-Submit] %s set to %q on the initial fill (saved a validation-retry cycle)", what, want)
```

The candidates on that path are the operator's own address values — Location and Country are required
react-select fields on every Greenhouse form — so the record wrote a home city, state and country into
the log. Confirmed against the live logs by matching every `pii.yaml` value and reporting counts only:
51 records of the shape `[Auto-Submit] (Location|Country) set to …` exist across three log files —
13 in `career_agent.log`, 33 in the rotated `2026-08-12` log, 5 in the `2026-08-03` one. Redacted
shape: `[Auto-Submit] Location set to "<CITY>, MI" on the initial fill`.

**Why it was worse than a legacy-path artifact.** `pkg/submitter` is shared with `cmd/assist`'s fill
path, whose stderr reaches `dashboard.log` through `security.SanitizeChildLogLine`. That filter strips
markup and truncates (`pkg/security/logsafe.go:171`); a well-formed prose line passes through
untouched. So the first time the operator clicks **Continue** on an assisted application their address
values land in `dashboard.log` as well. That half had not fired yet — `dashboard.log` is clean —
because no *assisted* fill has ever run here: `assisted_fill_summary` reads `0 filled, 0 reused` on
all ten prepared jobs. The autonomous path plainly has run, which is where the 51 records come from;
the two paths share this function, which is the point. This is the boundary ADR-006 draws, and logs
get zipped for sharing.

**Scope — the first sweep was wrong, and how it was wrong is the useful part.** Every `log.Printf` in
`pkg/submitter` carrying `%q` was read, which found nothing else: the rest carry selectors, control
types, or bounded reason codes, and `uploadName` at the cover-letter upload is the constant
`cover_letter.<ext>`. On that basis this was written up as the only site logging an operator value.
**Post-fix review falsified it.** Two further sites format the value into a nested `fmt.Sprintf`
first and log the result with `%s`, so a `log.Printf.*%q` grep cannot see them at all:
`browser.go:2019` (`fmt.Sprintf("%s (tried %q)", selector, value)`, emitted at `:2031`) and
`browser.go:3313` (`rejectedDespiteLanding`, emitted at `:1832`). Both take `value` from `fixesMap`,
which `SolveValidationErrors` produces from `pii.ApplicationFacts()` plus `pii.EEO.Summary()` — the
same operator fields, on the same Location and Country controls, into the same two log destinations.
A grep for a format verb finds only the sites that hold the verb; the next audit of this kind should
trace the *value* to its source instead.

Those two are filed as **#549** rather than folded in here, and the reason is not size. Each was added
deliberately — bugs.md #97 and #100 — with a written rationale about telling a broken mechanism apart
from a value the widget does not offer, and each sits directly beneath a comment at `browser.go:1969`
asserting the opposite invariant (*"Selectors only, never values: the values are drawn from the PII
profile and this log is not a place for them"*). Reversing two prior decisions and resolving a
contradiction the file states about itself is its own task with its own test, and burying it inside a
PR named for the combobox leak would hide it. Neither has ever fired: zero occurrences of either
record across all four `career_agent*.log` files, against 51 for the one fixed here.

Deliberately left alone for a different reason: `browser.go`'s label-fill fallback and custom-question
failure records log employer question text, which is a real concern of the same family (ADR-007:
*"never a label, never a value"*) but a different confidentiality category.

**The fix.** Keep the diagnostic, drop the value. The record exists to answer which phrasing a
geocoder accepted (improvements.md #28), and because the caller fixes the candidate order, the
candidate's *position* answers that completely: `Location committed on the initial fill from candidate
2 of 3 (saved a validation-retry cycle)`. Split into `comboboxCommitRecord(what, position, total)` so
the rule is testable without a browser — the same bounded-record-at-the-producer shape as
`security.BrowserFailureReason`. The sibling `commitFilledCombobox` twenty lines below already logged
the field name only; this brings the two into line.

**Verification.** Two tests in `pkg/submitter/assisted_log_privacy_test.go`:

* A gated Chromium regression (`CAREER_AGENT_PLAYWRIGHT_INTEGRATION=1`) driving the shipped function
  against a synthetic react-select Location widget, with the first candidate deliberately unmatched so
  the run commits at position 2. **This is the one that demonstrates the catch.** Reverting the fix
  and re-running reproduced the leak verbatim:
  `[Auto-Submit] Location set to "CA_TEST_CITY, CA_TEST_STATE" on the initial fill` — the same shape
  found in `career_agent.log`, from a real browser. No employer was contacted and no application was
  submitted for this.
* A unit test pinning `comboboxCommitRecord`'s exact output. Its first draft asserted that the record
  contained none of the candidate strings, which post-fix review correctly called a tautology: the
  helper is never given the candidates, so that assertion is true of every possible implementation and
  would pass forever. It now asserts the whole string, so widening the signature to thread the typed
  value back in cannot leave it standing. Note the honest limit this leaves: the gated browser test is
  the only thing guarding the *call site*, and it does not run in a default `go test ./...`.

An earlier version of this account claimed both tests were "confirmed to fail against the original
line". Only the Chromium one was — the unit test could not have failed against code where
`comboboxCommitRecord` did not exist; that would have been a compile error, which is not the same
thing as a demonstrated catch. Corrected here rather than quietly, because overstating what a test
proves is the failure mode this whole session was auditing for.

`gofmt -l ./cmd ./pkg ./internal` clean, `go build ./...`, `go vet ./...`, `go test ./...` all pass.

**What the pre-existing logs still hold.** The fix stops new records; it does not rewrite the ones
already written. Precisely, so a scrub does not miss a file: five paths match `career_agent*.log`,
and **three carry records** — `career_agent.log` (13), `career_agent-2026-08-12T21-27-10.908.log`
(33), `career_agent-2026-08-03T13-23-42.195.log` (5). `career_agent-2026-08-01T14-50-23.490.log` has
none. `career_agent_live.log` is root-owned `0600` and could not be read by this user, so it was
**not** checked in either direction. All are gitignored, so nothing left the machine, but anyone
zipping logs to share should scrub or delete them first — and needs `sudo` for the fifth.

## 545. Lever's custom question cards extract a placeholder, an option, or a raw name attribute instead of the question

**Found 2026-08-13** while evaluating whether preflight should inspect Lever postings. **Closed 2026-08-14.**

### The row's own premises, re-checked first

Two claims in the filed row were wrong, and correcting them changed both the effort and the priority.

* *"The page renders client-side, so fetching the HTML is not enough."* False. Three real Lever `/apply` pages returned HTTP 200 to an anonymous `curl` with the complete server-rendered form markup. The live-browser DOM investigation the row's Effort 3 was budgeted for was done statically in minutes.
* The row was scored **1.3 = 4x1.0÷3** and ranked *below* improvements.md #545 (2.0). Re-scored to **2.0 = 6x1.0÷3** on a live queue measurement: `AWAITING_REVIEW` is 20 Lever + 6 Greenhouse, so Lever is 77% of the actionable queue, and because Lever cannot be submitted from the assisted browser (bug #520) those 20 are precisely the applications the operator finishes by hand — the case the Copy Application Packet from PR #23 exists for. The row itself said it "becomes Major the moment either of those changes"; PR #23 was that change.

### Cause, confirmed rather than suspected

The suspected cause in the row was correct. `labelFor`'s last resort was `closest('fieldset, .field, .form-group, li, div')`, which returns the *nearest* match. Lever wraps every control in its own `div.application-field`:

```html
<li class="application-question custom-question"><div>
  <div class="application-label full-width text"><div class="text">Where do you currently reside? (City/State)?<span class="required">*</span></div></div>
  <div class="application-field full-width required-field">
    <input required class="card-field-input" type="text" placeholder="Type your response" name="cards[<uuid>][field0]">
  </div>
</div></li>
```

`querySelector('legend, label')` inside that wrapper finds nothing, so the chain fell through to `placeholder` (text inputs), to `name` (textarea and select, which have no placeholder), or — for radio/checkbox — to the option's own wrapping `<label>`. Three fallbacks, one cause. The question sat in the sibling `div.application-label`, one level up.

### Fix

Not a word list, not a placeholder blocklist, not a Lever selector. A bounded upward walk that, at each ancestor, reads **only the child subtrees containing no form control**.

That single exclusion is self-limiting and ATS-agnostic: a control's own field wrapper contributes nothing because it contains the control, and a container holding several questions contributes nothing because each of those questions' text sits inside its own control's subtree. So the first non-empty text found on the way up belongs to this control and to nothing else.

Radio and checkbox groups resolve their question separately from their option text, starting above the option's own `<label>` and climbing to the group's common ancestor before reading. `fieldset > legend` still wins over all of it. Option text is unchanged.

### Three more defects the live run found, which no unit test would have

The synthetic fixtures all passed before these existed. Running the shipped read-only inspection against a real posting is what surfaced them — the fifth time on this project that a live run has found what `go test` could not.

1. **A `<label>` wrapping a `<select>` returned the question and every option.** `textContent` includes descendants, so the EEO questions read as `GenderSelect ...MaleFemaleDecline to self-identify`. A wrapping label is now read with the control's own subtree excluded. Pre-existing, not introduced by this fix.
2. **A one-option attestation card reported its option, `I Acknowledge`,** instead of the paragraph being acknowledged, because the group resolution never climbed above the option's `<label>` when the group had a single member.
3. **A help note inside a group's container outranked the group's label outside it** — Lever's pronouns group read as `Let the employer know what pronouns you use...`. Text preceding the control is now preferred; trailing text is used only when nothing precedes the control anywhere on the way up, so a form whose label genuinely follows its input keeps working.

### The second, separate mistake: reading is not submitting

`storage.AssistedBrowserRejectionReason` records one evidence-backed fact — Lever's submission verification refuses the assisted browser (bug #520). It was also being used to decide whether preflight may **read** a form, in `PreflightCandidates` and in `cmd/preflight`. Preflight fills nothing and cannot submit, and Lever's `/apply` form is public, so those are different questions with different answers.

The cost was exact: the 20 Lever applications were the only ones never inspected, and they are the ones that most need preparing. `assistedBrowserRejection` gains `blocksPreflight` (false for Lever, verified against four real postings) and `storage.PreflightRefusalReason` asks the read question. One registry, two questions.

`TestPreflightCandidates_RefuseWhatTheAssistedBrowserItselfRefuses` asserted the conflation, so it was rewritten rather than deleted and now records why its old assertion was wrong. **The submit boundary is untouched**: Lever still never opens an assisted browser, still routes to `open_in_own_browser`, and still cannot be submitted by Career Agent.

### Tests

`pkg/submitter/questions_browser_test.go` (new) runs the **shipped** `controlInventoryJS` against a **real DOM in Chromium**, gated on `CAREER_AGENT_PLAYWRIGHT_INTEGRATION=1` like the existing log-privacy regression. A Go reimplementation of the walk would only ever prove the copy agrees with itself. Every assertion checks the label **and** the control key together, because a right label on the wrong control is worse than an ugly label.

Six tests: Lever custom cards (text, textarea, select, radio, required and optional, five distinct questions none borrowing a neighbour's); Lever standard fields plus the pronouns group; label-wrapped select, one-option attestation and in-group help note; trailing text when nothing precedes; generic sibling-label, fieldset/legend and bare-text-node shapes; and a **Greenhouse fixture that passed before the change as well as after**.

`pkg/storage`: `TestPreflightRefusalReason_IsSeparateFromTheSubmitRejection` pins both directions.

### Live verification, no submission

Read-only inspection against three real Lever postings and one real Greenhouse posting, before and after, using the shipped `submitter.InspectApplication`:

| Real Lever posting, 30 controls | before | after |
|---|---|---|
| 8 text card questions | `Type your response` x8 | 8 distinct real questions |
| 2 textarea/select card questions | `cards[<uuid>][fieldN]` | the real questions |
| 1 radio card question | `Yes` | `Are you willing to relocate if needed?` |
| pronouns group | `He/him` | `Pronouns` |
| attestation checkbox | `I Acknowledge` | the full paragraph |
| 4 EEO selects | question + every option | `Gender`, `Race`, `Veteran status`, `Disability status` |
| `Current location` | + widget status text | `Current location` |

Greenhouse (`job-boards.greenhouse.io/affirm/jobs/7833950003`, 25 controls) was **byte-identical** pre-fix and post-fix, verified by diffing the two runs. Greenhouse never reaches the changed fallback: every real control carries `aria-label` or `aria-labelledby`, and its only unlabelled inputs are `aria-hidden` and already dropped by `visible()`.

End to end: binaries rebuilt, dashboard restarted, **Prepare** run through `POST /api/knowledge/preflight` against four real Lever applications. All four came back `state: inspected` (30, 18, 18, 18 controls) where all four were previously skipped outright. The Copy Application Packet for one of them now lists 11 real questions under "This form also asks" where it previously listed none. Readiness now covers 10 applications, 227 fields.

### Six more defects found in code review, and one fixture that was wrong

A `/code-review high` pass on PR #24 found six ways the new walk could still attach a wrong label to a real question — the same failure this bug is about, reached through a different door. All six are now pinned by browser fixtures.

1. **A wrapping `<label>` whose text shares a branch with its control lost the text entirely.** `<label><span><input type="checkbox"> I agree to the terms</span></label>` is the commonest checkbox markup there is; dropping every control-bearing branch dropped the question with it, and the control then fell through to whatever prose an ancestor carried (`Legal`).
2. **Sweeping the whole tree for preceding text before considering trailing text** let a section heading three levels up beat the field's own adjacent `<label>`.
3. **Reading every preceding sibling glued each earlier question onto the next field** on a flat form: `Question A Question B`.
4. **The walk's notion of "control" included submit-shaped, hidden and `aria-hidden` inputs the enumeration itself skips**, so a hidden state input sharing a container discarded the question. Lever emits exactly that beside every card.
5. **The group climb was unbounded and keyed on a document-wide name count**, so a page carrying a second copy of a form sent it to `<body>`, where the walk stops and the group falls back to its first option — the original defect, reached by walking too far.
6. **An already-completed application was recorded as "this ATS rejects Career Agent's browser".** Both skips shared one reason code, and once no registry entry blocked preflight, that mislabel became the only thing that branch could produce. `already_applied` is now its own code with its own operator-facing sentence.

**Fixing #1 then caused a live regression the fixtures could not see.** Reading a wrapping label by keeping text that shares a branch with the control re-admitted a Race label trailing every option's legal definition, and a location widget's `No location found. Try entering a different location`. Both readings are individually right for different markup, so the wrapping-label read now does **both, in order**: the branches holding no control at all, and only if that is empty, the label's text minus what is inside the control itself. Caught by re-running against the live form rather than trusting the fixtures — the second time in this one task that the live run was the thing that found it.

**A group's question is never a section heading.** This fell out of #1: a group has somewhere better to fall back to than a heading — its own option text — which a non-group control does not, having only a placeholder or a generated name. So the exclusion applies to groups alone, and the asymmetry is deliberate.

**One fixture was wrong and was corrected rather than kept.** `TestControlInventory_TrailingTextStillResolvesWhenNothingPrecedes` asserted that loose prose after a field becomes its label. It should not: the old fallback only ever read trailing text through `querySelector('legend, label')`, so refusing loose prose costs nothing that previously resolved. It was a test written to match the implementation instead of the requirement. Replaced by `TestControlInventory_ATrailingLabelStillResolves`, which pins both halves of the asymmetry — a trailing `<label>` resolves, trailing prose does not.

### A second review round, and the two doors it found back to the same defect

A second `/code-review high` pass, which verified each finding by running the shipped JS in Chromium, found eight more. Two of them returned a group's first option as its question -- the original defect, reached through doors the first round of fixtures did not cover.

1. **`<form>` was treated as a floor rather than a bound**, so the walk never read at the form level. Any question written as a plain sibling of its control directly inside the form was lost: `group:auth` came back as `Yes`, and a text input as its raw `name`. The form level is now read before the walk stops.
2. **An option's text fell through to the outward walk**, so an unwrapped radio was named with the *group's question* -- which then appeared in the operator's choice list and pushed a real option off the end of it. `options` became `["Are you willing to relocate?", "Yes"]` and "No" vanished. This is the worst of the set: it offers the operator answers the employer never offered, and those options flow into the knowledge inbox and answer grouping. Option text now stops at the accessible name plus the adjacent text, and never walks outward.
3. **Skipping every `<label>` while resolving a group** threw away the very common shape where a group's question is a bare `<label>` with no `for`. Only a label *wrapping one of the controls* is an option's own text.
4. **`precedingText` read the whole run of preceding siblings**, gluing a section heading or a hint onto the question: `Additional Information Portfolio URL`, `Email We never share this.`, `Contact Phone number`. All three returned just the label under the old fallback, so this was a regression -- and the prompt is what the vault keys and matches on, so a polluted one silently stops matching a question the operator already answered. A `<label>`/`<legend>` among the preceding siblings is now the question outright, nearest first.
5. **`<script>`, `<style>` and `<noscript>` text was read as a question.** An inline `<script>` between a question and its input produced a JSON blob as the label -- which is not merely unreadable but can trip `SanitizeControls`' injection quarantine and be replaced wholesale, losing the question entirely.
6. **`countedControl` did not mirror `visible()` for `disabled`**, so a disabled neighbour made a container look control-bearing and discarded its text. The `disabled` check is free; only `getComputedStyle` is not, which is why the CSS-hidden half remains unmirrored and is documented as such.
7. **`PreflightSkipUnreadable` mapped to `browser_rejected`**, whose operator-facing sentence asserts a submit rejection that this check never establishes -- the same conflation this bug is about, reappearing in the reason code. Both it and `InspectApplication`'s equivalent now report `auth_required`, which is what "the form cannot be read without you signed in" actually is.
8. **An already-applied candidate still incremented the "could not inspect" count**, so the dashboard reported it as a failure beside the new sentence saying it was not one.

**The pattern worth keeping.** Across two review rounds and two live runs, every defect in this task was of one kind: text that is *near* a control being mistaken for the question it asks. The fixtures written before each round all passed. What caught them was a reviewer running the shipped JS against adversarial markup, and a live run against a real employer's form -- in that order, twice.

### A third review round, and where this stopped

Nine more, one of them a regression introduced by the *second* round's own fix. Fixed: nameless checkboxes collapsing into a single group key (the worst of the three rounds -- it dropped questions silently and OR-ed one question's `required` flag onto another); a trailing `<label>` stealing the label of the field it precedes; a container discarded whole because it happened to contain a heading; nested `<script>`/`<style>` text still reaching labels through `textContent`; `already_applied` still counted under "could not be inspected" in the dashboard, since the earlier fix reached only a log line; dead `optionLabel` code that the `break` above it made unreachable; and the now-producerless `browser_rejected`, kept for stored verdicts and documented as such.

**The tests now run in `go test ./...`.** They were behind `CAREER_AGENT_PLAYWRIGHT_INTEGRATION=1`, copied from the log-privacy regression without asking whether the gate made sense here. It did not: `AGENTS.md` documents `go test ./...` as the verification loop, this file argues that a Go reimplementation of the walk would only test itself, and between those two facts `controlInventoryJS` had no coverage at all in the loop anyone runs. They serve one local page and read it -- no employer, no network, no database -- so there was nothing to opt into. They now skip only when Chromium is genuinely missing, and cost about 24s.

**Two findings were deliberately not fixed**, and this is the honest boundary of the heuristic rather than an oversight:

* A question authored *as* a heading (`<div class="card-header"><h4>Do you require sponsorship?</h4></div>`) is still discarded for a group, because a heading beside a group is more often a section name whose group has better text of its own -- the mirror case, `<h3>Legal</h3>` above `<label><input type=checkbox> I agree to the terms</label>`, wants the option. Both are structurally identical and neither was observed on a real form.
* A farther `<label>` still beats a nearer plain-text question, for the same reason in reverse: `<label>Email</label><span class="hint">We never share this.</span>` wants the label, and `<label>Optional</label><div class="application-label">Why do you want to work here?</div>` wants the div.

Both are adversarial constructions with no live evidence on either side, and each available "fix" trades one hypothetical for its mirror image. The fallback is a heuristic; where two readings are equally defensible it picks one, and the accessible-name chain above it -- which is what Greenhouse and any WCAG-compliant ATS actually use -- is never reached in these cases anyway.

**Three review rounds found 6, 8 and 9 defects.** The rate did not fall, which is the honest measure of how hard "which text near this control is its question" is to get right. What kept it convergent was that every round ran the shipped JavaScript against real markup, and every round was followed by a live run against a real employer's form -- the live run caught two regressions that all the fixtures of the moment had passed.

**Known cosmetic residue, accepted deliberately.** Lever's optional "Custom" pronouns opt-in is a second control belonging to the same question, so it also reads `Pronouns`. It is not a duplicate in the inbox: `canonical_key` is derived from the prompt, so both controls collapse into one group, which is the correct outcome. Adding a special case to distinguish them would be unprincipled machinery for one marginal control.

The required glyph (`*`/`✱`) is left in labels rather than trimmed. Trimming would be a small improvement on Lever and a **change to Greenhouse's labels**, which this fix must not make. It costs nothing: `Required` carries the fact as its own field, and `answers.Normalize` reduces every non-alphanumeric to a space, so no canonical key, alias or vault match ever sees it.

## 522. Agent lifecycle and liveness reporting are unreliable in four distinct ways

**Found 2026-08-05/06** and confirmed live. Closed 2026-08-12. Four independent defects in `cmd/dashboard` and `pkg/config` made the dashboard's answer to "is the agent running?" unsafe when operating real child processes.

**Defects and fixes.**

1. **`/api/agent/stop` claimed `{"status":"stopped"}` while the agent was still alive.** It sent `SIGTERM` and returned immediately. `serveAgentStop` (`cmd/dashboard/main.go`) now identifies the agent through the same single-instance `flock` used everywhere else, sends `SIGTERM` only to that PID, and polls the lock until it is released. Only then does it return `{"status":"stopped"}`. If the process does not exit within `CAREER_AGENT_STOP_TIMEOUT` (default `30s`, overridable), it returns HTTP 408 with `{"status":"timeout", "pid": <pid>, "message": "..."}`. There is no automatic `SIGKILL` escalation; a truthful timeout is preferred over a false success.

2. **Dashboard-launched agents became unreaped zombies.** `serveAgentStart` called `cmd.Start()` without an owner for `cmd.Wait()`. It now keeps an `agentChild` record, spawns one goroutine that owns `cmd.Wait()`, and synchronizes on `agentProc`/`agentMu` so lifecycle races between start, stop, and status are bounded. Repeated start/stop cycles no longer accumulate `<defunct>` processes.

3. **`daemon_active` read true with no agent running.** `pkg/config.GetEffectiveSettings` compared the effective settings against `applications/active_operator_settings.json` and reported `DaemonActive` from the acknowledgement alone. It now also calls `config.IsAgentAlive(AgentLockPath)`, sets a new `DaemonRunning` field from the lock-backed liveness check, and only sets `DaemonActive = acknowledged && alive`. This separates "settings the daemon acknowledged" from "an agent process actually holds the lock".

4. **Assisted Apply launched through `go run` in a source checkout.** `assistedApplicationCommand` now resolves: `$CAREER_ASSIST_BIN` if set and executable; sibling `career_assist_bin` or `assist` next to the dashboard binary; a built `career_assist_bin` in the repository root; and only falls back to `go run ./cmd/assist` as an explicit development fallback. The launch path also keeps a single `Wait()` owner so readiness-timeout kills do not leak zombies.

**Process ownership and reaping.** One goroutine owns `cmd.Wait()` for every dashboard-started child. The agent start endpoint waits only until the child has acquired its own single-instance lock, then hands the child off to the wait goroutine. The stop endpoint signals the lock-file PID and waits for the lock to release. The assisted launch goroutine owns `Wait()`; its timeout path kills via a captured `*os.Process` and then waits on the same goroutine, never calling `Wait()` twice.

**Liveness source.** The existing `flock` on `applications/career_agent.lock` is the single source of truth. `pkg/config/IsAgentAlive` checks it with a non-blocking shared lock so status checks do not race against the agent's exclusive acquisition. The PID in the lock file is used only as a signalling target, never as the sole proof of liveness.

**Tests.** New unit/Linux integration tests in `cmd/dashboard/main_test.go`: `TestServeAgentStart_LaunchesAndReapsAfterNaturalExit`, `TestServeAgentStop_WaitsForActualExit`, `TestServeAgentStop_TimeoutDoesNotClaimSuccess`, `TestServeAgentStop_AlreadyStoppedReportsTruthfully`, `TestServeAgentStart_AlreadyRunningReportsTruthfully`, `TestRepeatedStartStop_NoZombiesNoStaleLocks`, `TestAgentCommandPrefersBuiltBinary`, `TestLaunchAssistedApplication_ReadinessTimeoutKillsAndReaps`, `TestAssistedApplicationCommandPrefersBuiltBinaryInCheckoutRoot`, `TestAssistedApplicationCommandFallsBackToGoRunWhenNoBinaryExists`. New `pkg/config` tests in `effective_settings_test.go`: `TestGetEffectiveSettings_DaemonActiveRequiresLiveness`, `TestGetEffectiveSettings_DaemonActiveFalseWhenSettingsMismatch`. A build-tagged Playwright UI test, `cmd/dashboard/ui_lifecycle_test.go` (`go test -tags=ui ./cmd/dashboard -run TestUILifecycleStartStop`), drives the real dashboard in Chromium and clicks Start/Stop while asserting the rendered state transitions.

**Real process smoke tests in the Devin Linux VM.** A synthetic `fake_agent` binary acquired the same flock, printed readiness, and responded to `SIGTERM`/`FAKE_AGENT_IGNORE_SIGTERM`. Results: 10 consecutive start/stop cycles returned `{"status":"started"}`/`{"status":"stopped"}`, status toggled `running:true`/`running:false`, 0 zombies, lock free at the end; a 200ms stop delay caused the endpoint to wait ~2s and then truthfully return `stopped`; ignoring `SIGTERM` caused the endpoint to return HTTP 408 with `{"status":"timeout"}` while the process remained alive.

**Verification.** `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal` are clean. `go test -race ./cmd/dashboard/... ./pkg/config/...` passes.

**Files changed.** `cmd/dashboard/main.go`, `pkg/config/agent_liveness.go`, `pkg/config/active_settings.go`, `pkg/config/effective_settings.go`, `cmd/dashboard/main_test.go`, `pkg/config/effective_settings_test.go`, `cmd/dashboard/ui_lifecycle_test.go`, `cmd/dashboard/testdata/fake_agent/main.go`, `cmd/dashboard/testdata/fake_assist/main.go`.

## 535. A missing operator_settings.yaml could silently reactivate Automatic mode's final employer submit-click from legacy profile.yaml booleans

**Found and completed 2026-08-11**, as a proactive safety-hardening task rather than from a live incident. Severity graded **Major** under the existing rubric rather than downgraded to protect the Usability Gate's zero-Major claim: the failure mode is an irreversible external action (a real employer form submitted with no operator having explicitly chosen Automatic mode), triggered by nothing more than a settings file being absent — exactly the state a fresh install or a botched upgrade leaves behind.

**The defect had two independent occurrences of the same unsafe pattern, one worse than the other:**

1. `config.GetEffectiveSettings` (`pkg/config/effective_settings.go`), used by the dashboard's `/api/operator-settings` status endpoint and `serveQualifiedJobsPromote`, fell back to inferring `ApplicationMode` from legacy `profile.yaml` booleans (`auto_submit`/`auto_submit_click`/`copilot_mode`) whenever `LoadOperatorSettings` returned `nil` for a missing file — including inferring `automatic` from `auto_submit=true, auto_submit_click=true, copilot_mode=false`. This affected what the dashboard *reported*.
2. `config.ApplyOperatorSettings` (`pkg/config/operator.go`), called directly by `cmd/agent/main.go`'s startup path (`ApplyOperatorSettings(prof, opSettings)` where `opSettings` is whatever `LoadOperatorSettings` returned, nil on a missing file) and again inside the daemon's per-cycle refresh, was a **no-op** when `op == nil`: `"Infer legacy mode / Preserve existing behavior, no rewrites"`. That left `prof.AutoSubmitClick` exactly as loaded from `profile.yaml`, and `prof.AutoSubmitClick` is what `cmd/agent/pipeline.go` passes straight into `submitter.AttemptSubmit`'s final click parameter. This is the real production risk: not a status display, but the literal boolean gating the final employer form submission.

**Reproduced before fixing**, per protocol: a temporary synthetic `profile.yaml` (`auto_submit: true`, `auto_submit_click: true`, `copilot_mode: false`) with no `operator_settings.yaml` created, run through the real `GetEffectiveSettings`, resolved to `application_mode=automatic` and `automatic_submit_click_active=true` — confirmed failing before any fix (`TestGetEffectiveSettings_MissingOperatorSettingsNeverInfersAutomatic` and later folded into `TestGetEffectiveSettings_MissingSettingsLegacyAutomaticFlags`/`TestApplyOperatorSettings_FreshStartAfterUpgradeFailsClosed`).

**Fix.** Added `config.DefaultOperatorSettings()` (`ApplicationMode: find_only, MinimumFitScore: 50`) as the single authoritative fallback. `ApplyOperatorSettings` — the one function every call site funnels through, and the one that actually drives `profile.AutoSubmit*` — now substitutes `DefaultOperatorSettings()` for a `nil` `op` instead of leaving the caller's legacy booleans untouched. `GetEffectiveSettings` was simplified to call the same default rather than keep its own separate (and less safe) legacy-inference logic, so there is exactly one rule for "operator settings missing" in the codebase, not two. `cmd/agent/main.go`'s startup path now also logs one bounded, contents-free warning (`"operator settings missing; automatic submission disabled and mode defaulted to find_only"`) when `LoadOperatorSettings` returns nil, without changing its control flow — `cmd/agent` already started successfully in this case before the fix (there was no fatal error), it just silently carried forward whichever legacy booleans happened to be in `profile.yaml`.

**Invariant established:** Automatic mode is opt-in and requires an explicit, successfully loaded `operator_settings.yaml` with `application_mode: automatic`. Missing or invalid operator settings never infer Automatic from legacy profile flags. A corrupt/malformed operator settings file (existing behavior, unchanged) still returns an error from `LoadOperatorSettings`/`GetEffectiveSettings` rather than silently defaulting to anything — fail-closed by refusing to proceed, not by choosing a mode. An invalid `application_mode` value in an otherwise well-formed file is caught by `OperatorSettings.Validate()` (existing behavior, unchanged) and also returns an error. A stale `applications/active_operator_settings.json` heartbeat (written by `AcknowledgeActiveSettings`) only ever feeds `DaemonActive` liveness comparison in `GetEffectiveSettings` — it was never read back into `ApplicationMode`/`AutomaticSubmitClickActive` even before this fix, confirmed by a new test rather than assumed.

**Explicit Automatic mode is preserved and unaffected**: an operator settings file with `application_mode: automatic` still resolves to `ApplicationMode: automatic` and `AutomaticSubmitClickActive: true`, verified by `TestGetEffectiveSettings_ExplicitAutomatic`. Explicit `find_only` and `assisted` are equally unaffected (`TestGetEffectiveSettings_ExplicitFindOnly`, `TestGetEffectiveSettings_ExplicitAssisted`).

**Tests added** (`pkg/config/operator_test.go`, plus one in the existing `effective_settings_test.go`): missing settings + legacy Automatic flags; missing settings + ordinary (all-false) profile; missing settings + legacy Assisted/Copilot flags; explicit find_only; explicit assisted; explicit automatic; corrupt operator settings (fails closed with an error); invalid `application_mode` value (fails closed with an error); a stale `active_operator_settings.json` heartbeat cannot grant Automatic; and a fresh-start-after-upgrade regression exercising `ApplyOperatorSettings(prof, nil)` directly, the same call shape `cmd/agent/main.go`'s startup path uses.

**No second implementation was added.** `cmd/agent/main.go` (startup and the per-cycle daemon refresh) and `cmd/dashboard/operator_api.go` (`serveOperatorSettings` GET/POST, `serveQualifiedJobsPromote`) all call `config.GetEffectiveSettings`/`config.LoadOperatorSettings`/`config.ApplyOperatorSettings` and inherit the fix automatically; none of them contain their own inference logic to fix separately.

**Explicitly out of scope, per the task that filed this**: #522's agent lifecycle/liveness cleanup (`/api/agent/stop`, zombie processes, `daemon_active`'s missing liveness check) and #524's location backfill were left untouched — this row does not touch `DaemonActive`'s existing (separately tracked) staleness, only confirms it cannot leak into `ApplicationMode`.

`go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all pass.

## 534. `StartTracker` only reads the newest ~51 messages, so any outage longer than the fetch window loses outcomes permanently

**Completed 2026-08-08.** Filed 2026-08-07 by the first real production validation of #529/#533, which measured the tracker's fetch window (`from = mbox.Messages - 50`, an IMAP *sequence-number* window anchored to the current message count) at roughly 4 hours of coverage against this inbox's traffic — against a tracker that had just been restarted after a 14-day stop. Sequence numbers are mailbox-relative and shift as mail is added or removed, so the window could never serve as a durable position even widened; anchoring to "the newest N" meant it only ever moved forward, so anything that fell outside it during downtime was gone for good, not deferred.

**Diagnosis.** `pkg/tracker/imap.go`'s `StartTracker` had no durable state at all beyond `processed_emails` (a Message-ID set, which provides idempotency but not a resume position). `go-imap` v1.2.1 (already a dependency, `github.com/emersion/go-imap`) exposes `client.UidFetch`, `client.UidSearch`, and `imap.MailboxStatus.UidValidity`/`UidNext` — UIDs are stable within one `UIDVALIDITY` generation, unlike sequence numbers, and are the correct durable identifier.

**Fix — durable UID checkpoint, two independent ranges.** A new singleton table, `tracker_cursor` (`pkg/storage/manager.go`, `pkg/storage/tracker_cursor.go`), tracks:
- `forward_uid` — the high-water mark for ordinary new mail, advanced every scan.
- `catchup_floor_uid` / `catchup_ceiling_uid` / `catchup_next_uid` / `catchup_complete` — a bounded historical catch-up range, established once at bootstrap and drained independently of the forward range across as many future scans as it takes, so a large backlog can never starve new mail.

The catch-up floor is derived from live evidence, not a hardcoded lookback: `storage.EarliestTrackableApplicationTime()` returns the earliest `discovered_at`/`applied_at` among `job_funnel` rows still in an open, trackable status (`APPLIED`, `MANUAL_REQUIRED`, `AWAITING_REVIEW`). An outcome email cannot meaningfully predate the earliest still-open application, so this genuinely covers whatever the real outage was — verified against the live database, the earliest such date was 2026-07-13, comfortably covering the actual 2026-07-24–2026-08-07 outage without ever hardcoding "14 days" as product behavior. `bootstrapTrackerCursor` then uses `c.UidSearch(&imap.SearchCriteria{Since: earliest})` to find the lowest matching UID.

Both ranges are fetched in bounded batches (`trackerRangeBatchSize`, 200 UIDs) per scan, sorted ascending, processed oldest-first. A message's classification/matching/persistence still commits atomically per #533's transaction shape (unchanged); if a message's acknowledgement fails, processing stops for that batch and the checkpoint advances only through the last durably-handled UID — a crash or transient failure mid-batch retries from exactly that point next scan, never skipping past it.

`UIDVALIDITY` changing (RFC 3501's explicit signal that a mailbox's old UIDs no longer mean anything) is handled identically to "no cursor exists yet": a full re-bootstrap, re-deriving the catch-up floor from the same live evidence. `processed_emails`' Message-ID dedup (untouched by this fix) is what prevents any resulting overlap from duplicating an outcome write — the UID cursor's own job is only ever to decide what to fetch, never to gate correctness.

**A second defect was found and fixed during this same task's live production validation, before it was ever considered done.** IMAP UIDs are not contiguous — deleted mail leaves permanent gaps — so a fetched batch can legitimately contain zero messages while catch-up is nowhere near complete. The first version of this fix advanced the checkpoint only to the highest UID a *real message* carried; against the live inbox this stalled catch-up completely and silently the first time it hit a gap wider than one 200-UID batch (confirmed live 2026-08-08: `catchup_next_uid` frozen at the same value across a full scan cycle overnight, zero errors logged, zero progress). Fixed by having `processMessages` report whether the whole fetched batch was handled without a failure (`complete bool`, alongside the highest UID actually handled); when `complete` is true, the checkpoint advances to the *end of the fetched range*, not merely to the last message's UID, so an empty or partially-empty range still counts as covered. A dedicated regression, `TestCatchUpAdvancesPastAGapWithNoSurvivingMessages`, reproduces a ~250-UID gap and proves the checkpoint crosses it in bounded batches rather than stalling.

**Tests.** 10 new tests in `pkg/tracker/uid_cursor_test.go`, run against a real (not mocked) in-process `github.com/emersion/go-imap/server` instance backed by a purpose-built test mailbox (`pkg/tracker/imap_server_test.go` — the vendored `backend/memory` test backend hardcodes `UidValidity` to 1, so a minimal custom backend was written to make it test-controllable for the `UIDVALIDITY`-change test): steady state and older-mail-left-untouched; recovery of an outcome beyond the old ~50-message window (60-message mailbox, oldest message holds the outcome); multi-batch drain across scans; new mail not starved by an incomplete backlog; `UIDVALIDITY` change forcing a safe resync without duplicating an already-processed outcome; crash/restart mid-batch retrying without skipping; Message-ID replay after a simulated cursor rollback; ambiguous and unmatched outcomes preserved through the new UID path; cursor bootstrap from the pre-#534 (no-cursor) state; and the gap-stall regression. All 22 pre-existing `pkg/tracker` tests pass unmodified. `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal` all pass.

**Production validation, 2026-08-08.** The single running tracker process (PID confirmed via `ps`, one process, built from `aaebb63`) was stopped cleanly (`kill`, verified exited via `ps`, not `pkill -f`), the fixed binary built and deployed, and restarted three times across this validation to prove restart-safety — including once specifically to catch and fix the gap-stall defect above before ever leaving the buggy version running unattended. Live evidence: bootstrap found a real ~2009-UID historical catch-up window from the live evidence floor; each scan advanced the checkpoint in bounded steps (confirmed both messages-found and zero-messages-found batches advance correctly); a deliberate restart resumed from the persisted checkpoint rather than re-bootstrapping or losing progress; `processed_emails` and `unmatched_outcomes` counts stayed exactly flat across restarts and repeated catch-up passes over already-covered UID territory (idempotency holding on live data, not just in tests); single tracker process confirmed before and after every restart; zero errors, zero auth-loop symptoms, zero database-lock-loop symptoms across all scans. No matched (REJECTED/INTERVIEW_REQUESTED via a real application match) outcomes were recovered during this session's observation window — catch-up had only progressed roughly 30% through its ~2009-UID historical span by the end of the session, all of it so far overlapping mail the pre-#534 tracker's earlier ~51-message-window scans had already acknowledged. This is expected, not a defect: the fix guarantees eventual coverage across future scans, not instant coverage in one session, and the task's own instructions explicitly do not require draining the full backlog interactively.

**Deliberately not done.** No attempt was made to force-drain the ~2009-UID backlog in one sitting — that would defeat the entire purpose of bounding batches (resource control, and not starving new mail for days). The tracker was left running to continue draining on its own 15-minute cadence.

## 533. An outcome email that matches no application is acknowledged and discarded, so the evidence is unrecoverable

**Completed 2026-08-07**, immediately after #529 closed. #529's account named this path explicitly and deferred it as "a design decision with a real cost on both sides, not a bug fix". That deferral was reconsidered the same day for one reason: it is the specific hazard that makes **starting the tracker destructive**, and starting the tracker is the top operational priority coming out of #529.

**The path.** `updateDBWithTrackerResult` acknowledges the message in the same transaction that gives up on it. `StartTracker` skips any acknowledged Message-ID forever. So an email that classifies as a genuine rejection or interview request but resolves to `unmatched` (no company match) or `no_op` (company recognised, no open row) is written to `processed_emails` and then is gone — not deferred, discarded.

**Why it matters now rather than in the abstract.** The live funnel holds **44** hand-off rows (24 `MANUAL_REQUIRED`, 20 `AWAITING_REVIEW`) against **7** confirmed applications. A hand-off row only becomes matchable once the user confirms it. Every one of those 44 is a case where a real outcome email can plausibly arrive *before* the confirmation that would let it match — and under the old behaviour that email was destroyed by the first scan that saw it. With the tracker stopped for 14 days and about to be restarted against an inbox holding two weeks of unread mail, this was hours away from being exercised in bulk.

**Why acknowledging is nonetheless correct.** The obvious alternative — leave it unacknowledged and retry — recreates exactly what bug #20 fixed: `StartTracker` refetches the same trailing 50 messages every cycle, so an unacknowledged outcome email is reclassified, relogged, and (for rejections) re-runs an LLM extraction call every 15 minutes indefinitely. The acknowledgement is not the defect. Throwing away the evidence alongside it was.

**Fix.** A new `unmatched_outcomes` table records the fact separately from the acknowledgement:

```
message_id TEXT PRIMARY KEY, outcome_status TEXT, sender_domain TEXT, detected_at DATETIME
```

`storage.RecordUnmatchedOutcomeTx` writes it **inside the tracker's existing transaction**, so the record and the acknowledgement commit together or not at all — the same atomicity property commit `fabe12c` established for the outcome write, and for the same reason: a half-applied outcome is worse than a retried one. `ON CONFLICT(message_id) DO NOTHING` keeps reprocessing idempotent.

Both losing branches are recorded, not just `unmatched`. `no_op` means the company *was* recognised and the outcome still landed nowhere, which is if anything the more recoverable case.

**Privacy.** The record holds no email content: status, sender domain, timestamp, and the opaque Message-ID already stored in `processed_emails`. No subject, body, address or display name. `reportUnmatchedOutcomes` logs **counts of domains, never the domains themselves**, so a scan summary cannot enumerate who is emailing the user. A dedicated test asserts none of the stored fields contains any of the subject or body text passed in.

**Recovery surface.** `storage.UnmatchedOutcomeCounts` reads the table back, and `StartTracker` reports a one-line bounded summary at the end of each scan — "N outcome email(s) across M sender domain(s) matched no application (X rejection-shaped, Y interview-shaped)". Without that the record would exist and nothing would ever point at it, which is most of the original defect wearing a different hat. Timestamps are parsed through `assistedTimeLayouts` per ADR-003 decision 6 rather than assuming a single stored shape.

**Deliberately not done.** Matching was not widened and no automatic re-correlation was built. When a lost outcome's application is later confirmed, nothing yet joins the two — the operator sees the domain count and can act. Automatic re-correlation needs a matching rule at least as strict as the one #20 forced, and inventing it speculatively on zero real outcome data is how #20 happened. The table is deliberately a record, not a queue.

**Tests.** Four added to `pkg/tracker/imap_test.go`, all verified failing against the pre-fix code (recording disabled, tests re-run) and passing after:

- `TestUnmatchedOutcomeIsRecoverable` — both losing branches; asserts the message is **still acknowledged** (the anti-churn property) *and* recorded with the right status and sender domain. Pre-fix: "the outcome was acknowledged but not recorded — it is unrecoverable".
- `TestMatchedOutcomeIsNotRecordedAsUnmatched` — a successful match leaves no phantom loss record.
- `TestUnmatchedOutcomeRecordCarriesNoEmailContent` — the privacy boundary.
- `TestUnmatchedOutcomeRecordIsIdempotent` — reprocessing writes one row.

The existing 14 `updateDBWithTrackerResult` call sites were extended for the new `senderDomain` parameter mechanically, with no assertion changes.

**Verification.** `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal` all pass. The new table is added via `CREATE TABLE IF NOT EXISTS` in the existing schema block, so it appears on the live database at next open with no migration step and no change to any existing table.

## 529. 49 emails processed, zero outcomes recorded — the tracker's detections may never be reaching `job_funnel`

**Completed 2026-08-07.** Filed 2026-08-06 as "a discrepancy to diagnose, not a confirmed defect". That framing was correct. The headline claim — that detections never reach `job_funnel` — is **not** a current defect: the status write works and has worked since 2026-07-22. But diagnosing it exposed a real defect on the same write path, which was fixed, plus a wider one which was filed as #532.

### What the 49 processed emails actually are

All 49 were handled in exactly **two scans**, not 49 separate events:

| When (UTC) | Messages |
|---|---|
| 2026-07-22 04:21:03–04:21:04 | 41 |
| 2026-07-24 00:17:46 | 8 |

**The tracker has not run since 2026-07-24** — 14 days before this row was closed. `processed_emails` is a count of *messages the tracker has looked at*, which is bounded by `StartTracker`'s fixed "last 50 messages" fetch window. It was never a count of outcome-bearing emails, and reading it as evidence that 49 outcomes went missing was the row's central misreading.

### Whether genuine outcomes were among them

No. The reconciliation the row asked for resolves cleanly, and the artifact that resolves it is one the repository already had.

The Usability Gate recorded that the 2026-07-22 live scan "detect[ed] a real rejection (Glimpse) and a real interview invitation". That claim traces to `applications/rejection_feedback.md`, whose mtime is **2026-07-22 00:05 local (04:05 UTC)** and which holds exactly **one** entry. Two facts about that file settle the question:

1. **It was written by the pre-#20 binary, whose database writes were guaranteed no-ops.** Bug #20's fix (`a2a37b6`) landed at **2026-07-22 04:23:20 UTC**, 18 minutes *after* that file was written. Before it, `cmd/tracker/main.go` never called `storage.InitDB()`; `storage.GetDB()` returned nil and `updateDBWithTrackerResult` opened with `if db == nil { return }`. The commit message states this outright: "cmd/tracker never calling InitDB — meaning every tracker DB update in its history was a silent no-op." `logRejectionFeedback` writes a markdown file and needs no database, which is exactly why the artifact survived while the status write beside it did not.
2. **The detection was itself a false positive, by its own record.** The entry's LLM-extracted "HR Feedback" field reads: *"The candidate was not rejected — this email is a pre-interview outreach requesting availability... There is no indication of rejection in the message."* The pre-#20 classifier was keyword-only with no negative filters and derived the company from the first label of the sender's domain. This is precisely the false-positive class bug #20 was filed for and fixed the same day.

So the row's benign explanation #2 is confirmed by primary evidence rather than inferred. The 41 messages at 04:21:03 were the *verification run of the #20 fix itself*, two minutes before it was committed — the first scan in the program's history whose acknowledgements could persist at all, which is why `processed_emails` starts there and not earlier.

The row's benign explanation #3 ("56 of the 58 confirmed applications were submitted within the last few days") was **wrong on the data** and is corrected here: 51 of 58 `applied_jobs` rows predate 2026-07-23. The conclusion it supported still holds for a different reason — see below.

No `[Tracker]` log line survives in any of the five retained log files (0 occurrences across all of them), so logs could neither confirm nor refute persistence for those scans. The `rejection_feedback.md` mtime and the commit clock did the work instead.

### Why zero outcomes is nonetheless the correct current state

Beyond the classifier history, the tracker's reachable universe is far smaller than "58 confirmed applications" suggests. Joining `applied_jobs` to `job_funnel` on URL, live 2026-08-07:

| `job_funnel` status of a confirmed application | Count |
|---|---|
| *(no funnel row at all)* | 22 |
| FAILED_SUBMIT | 19 |
| INVALID_URL | 10 |
| APPLIED | 7 |

`GetTrackedCompanies` only returns companies holding a row in `APPLIED` / `INTERVIEW_REQUESTED` / `MANUAL_REQUIRED` / `AWAITING_REVIEW`, so **only 7 of 58 confirmed applications are visible to the matcher at all**. The other 51 are the residue of bug #94's history (`RecordApplicationInDB` used to run at document-generation time, so a job that generated documents and then failed to submit was recorded as applied). **Corrected 2026-08-07, later the same day:** calling all 58 "confirmed applications" was itself wrong, and this account repeated the error before checking it. Bug #94 was resolved **2026-07-25**; before that, `RecordApplicationInDB` ran from `SaveApplication` at *document-generation* time, so a pre-#94 `applied_jobs` row means "documents were written", not "submitted". All 51 predate 2026-07-23. All 7 post-#94 rows (2026-07-29 and 2026-08-06) are APPLIED in the funnel. So `applied_jobs` and `job_funnel` do **not** contradict each other: the funnel is telling the truth and 29 of the 51 explicitly record the failure (19 FAILED_SUBMIT, 10 INVALID_URL) while 22 predate the funnel being the system of record at all. **The true confirmed-application count is 7, not 58.** There is no blind spot to reconcile and no cleanup owed: verified live, zero of the 51 stale dedup rows sits in a re-queueable status, so none is currently suppressing work the way #94's seven were. Nothing was changed on this point beyond the correction.

Combined with 20 of the 21 currently trackable companies being single-word names, and the tracker being **stopped since 2026-07-24**, zero recorded outcomes is the expected state, not a symptom.

### The real defect this diagnosis did find

The funnel stage ledger is a **database trigger** (`createFunnelStageLedgerTrigger`, `pkg/storage/manager.go:597`), not a Go call. It derives `occurred_at` from `NEW.last_updated` and `reason_code` from `NEW.status_reason`. The tracker's outcome write was `UPDATE job_funnel SET status = ? WHERE id = ?` — **status alone**. Both ledger fields were therefore inherited from the row's *previous* state.

Reproduced against a real schema before fixing. A rejection applied to a row submitted 2026-07-15 produced:

```
job_funnel.status   = REJECTED                    <- correct
job_funnel.last_updated = 2026-07-15T12:00:00Z    <- never advanced
ledger reason_code  = "submitted_ok"              <- the SUBMISSION's reason
ledger occurred_at  = 2026-07-15T12:00:00Z        <- backdated to submission
```

The funnel status was always right. The *event record* of it was wrong in both the fields that make it useful: a rejection arriving weeks after submission was written into the ledger as having occurred at submission time, labelled with a reason code asserting the submission succeeded. Any consumer computing time-to-rejection would get zero, and any consumer filtering the ledger by `reason_code` would count that rejection as a submission success. Improvement #493's expected-yield ranking and `improvements_paywall.md` #14 are both sequenced behind exactly this data.

**Fix.** `applyOutcome` (`pkg/tracker/imap.go`) replaces the two duplicated inline `UPDATE` statements and stamps all three columns: `status`, a bounded `status_reason`, and `last_updated = time.Now().UTC()`. Two new exported constants, `OutcomeReasonRejected` (`outcome_email_rejected`) and `OutcomeReasonInterview` (`outcome_email_interview`), carry the reason. They are deliberately bounded codes rather than anything derived from the message, because the trigger copies `status_reason` into `reason_code` verbatim and the ledger is documented to hold "only state metadata, never job content". No email body, address, subject, or credential reaches the database or the log on this path, which was already true and remains so.

### Deliberately not changed

- **Matching was not widened.** The row warned against it and the warning is sound. One consequence was measured and left alone: a multi-word stored company name can never match a sender domain, since domains contain no spaces. Live, this affects exactly **1 of 21** trackable companies, so the cost is negligible against the risk of recreating #20.
- **Unmatched outcome emails are still acknowledged.** A classified email that matches no tracked application is marked processed inside the same transaction and will not be reconsidered. This is a genuine permanent-loss path and it is worth stating plainly, but the alternative — leaving it unacknowledged — recreates the unbounded rescan-and-relog churn #20 fixed, including a repeated LLM call per cycle for the rejection reason. Changing it is a design decision with a real cost on both sides, not a bug fix, and it was out of scope for a row whose fix direction says "diagnose before changing anything".
- **The trigger itself.** `stage_duration_ms` is NULL for all 1,385 live ledger rows and `pipeline_stage` buckets outcomes under the generic `funnel` label. Both are trigger defects affecting every writer, not the tracker, and fixing them needs a `DROP`/`CREATE TRIGGER` migration for which this repo has no precedent. Filed as **#532** rather than smuggled into this fix.

### Tests

Three new tests in `pkg/tracker/imap_test.go`, all confirmed to **fail against the pre-fix code and pass after it** (verified by reverting `applyOutcome` to the status-only write and re-running):

- `TestTrackerOutcomeIsLedgeredAtArrivalTime` — the regression proper, for both outcome statuses. Seeds a row through a real prior transition so the ledger has history, then asserts the outcome's `reason_code` is the bounded outcome code and its `occurred_at` is after the submission time and approximately now. Pre-fix it reports `reason_code = "submitted_ok"` and `occurred_at = 2026-07-15 12:00:00`.
- `TestTrackerOutcomeReasonCarriesNoEmailContent` — pins the privacy boundary the bounded codes exist to hold, asserting both `job_funnel.status_reason` and the ledger's `reason_code` equal the constant and contain none of the subject or body text passed in.
- `TestTrackerOutcomeChainEndToEnd` — the gap that let this row be filed. Every pre-existing test called `updateDBWithTrackerResult` with the company *already resolved* and empty subject/body, so nothing covered `classifyEmail` → `matchTrackedCompany` → persistence as one chain. This walks all six scenarios the close required against a real schema: a rejection for a uniquely identifiable application (→ `updated`, status REJECTED), a recruiter response for a hand-off application (→ `updated`, status INTERVIEW_REQUESTED), an unrelated newsletter (→ no classification, still acknowledged), an ambiguous two-role match (→ `ambiguous`, **both rows left APPLIED**, nothing written), an outcome for a company absent from the database (→ `unmatched`, no rows written), and a duplicate delivery (→ `no_op`, no second ledger row, and `WasEmailProcessed` already true so `StartTracker` would not have reached the writer at all).

### Verification

`go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all pass. `go test ./pkg/tracker/ -v` runs 18 tests, all passing. No real email was sent or fetched and no application was submitted; the live database was opened read-only (`file:applications.db?mode=ro`) for every diagnostic query in this account.

### What will provide the first meaningful outcome sample

Nothing will, until `cmd/tracker` is running again — it has been stopped for 14 days and scans only on a 15-minute loop while alive. Once it is running, the reachable population is the **7** confirmed applications currently in `APPLIED` plus the **44** rows in `MANUAL_REQUIRED` (24) and `AWAITING_REVIEW` (20), and it grows as the user confirms hand-off applications via `MarkHandoffApplied`. The 5 applications submitted 2026-08-06 are the realistic source of the first true rejection, on the ordinary two-to-six-week ATS timeline — so the first genuine outcome should be expected around **late August 2026**, not sooner.

## 527. The `:memory:` test database silently fails every nested query, so the assisted queue's readiness fields cannot be covered

**Completed 2026-08-07.** Filed 2026-08-06 while writing #525's tests, when a straightforward test of `GetAssistedQueue`'s `cover_letter_ready` field failed against a fix that was demonstrably working.

**Cause.** `setupTestDB` (`pkg/storage/manager_test.go`) called `InitDBWithPath(":memory:")`. A `:memory:` SQLite database is private to the one connection that opened it, and `db` is a `*sql.DB` — a pool. Any query issued while another result set is still open takes a *second* connection from that pool, which opens its own separate, empty in-memory database and fails `no such table`. `GetAssistedQueue` does exactly that: inside `for rows.Next()` it calls `assistedDocumentExists` twice per row, each running `conn.QueryRow` for the job's identity. Under test that lookup always failed, so `ResumeReady` and `CoverLetterReady` were always false whatever the documents on disk said.

**Test-harness artifact, not a production defect** — the row verified this before the fix (a dashboard built from the tree and run against the real `applications.db` reported both fields true for all 524 queued rows, because the production database is a file every pooled connection reaches). Nothing in production behaviour changed here, and no production file was touched.

**Fix.** `setupTestDB` now opens `filepath.Join(t.TempDir(), "test.db")`, so every connection in the pool finds the same schema exactly as production does, and the database is removed when the test ends. `file::memory:?cache=shared` also shares one schema across a pool but its name is process-global, so parallel tests would collide in it; that is why the file database was chosen and the reasoning is recorded on the helper rather than left implicit. The three `storage.InitDBWithPath(":memory:")` call sites in `cmd/agent/pipeline_test.go` were converted the same way in the same commit — they carried the identical latent trap, and leaving three untouched copies of a defect the same commit documents is how one of them later goes wrong.

**Reproduced before and after, not assumed.** Both new tests were run against the old `:memory:` harness and fail there — `TestGetAssistedQueue_ReadinessFieldsFollowTheDocumentsOnDisk` reports both readiness fields false after the master documents were written, and the canary reports `SQL logic error: no such table: job_funnel (1)` — then pass against the file harness.

**Tests.** `TestGetAssistedQueue_ReadinessFieldsFollowTheDocumentsOnDisk` (`pkg/storage/assisted_test.go`) is the projection-level test that could not be written before: it drives the real `GetAssistedQueue` over a queued job and asserts both fields false with no master documents on disk and both true once they are written. It is the counterpart to #525's `TestAssistedDocumentExists_CoverLetterReadinessFollowsTheMasterLetter`, which had to drive the helper directly for exactly this reason. `TestSetupTestDB_ServesQueriesNestedInsideAnOpenIteration` is a canary asserting the harness invariant directly — a query issued from inside an open `rows.Next()` loop must succeed — so a future return to `:memory:` fails loudly in one place instead of silently weakening every test that reads from inside an iteration.

**No test had been passing against the failure path.** The full suite was green before the change and green after it, and the reason was checked rather than assumed: no test anywhere in the repo asserted `ResumeReady` or `CoverLetterReady` before this one, so there was nothing to re-baseline. The fix direction's warning about mechanically re-baselining fallout turned out not to apply, but it was the reason this row was worked in Claude Code rather than delegated headlessly.

**ADR-003 decision 7** records the harness rule and why the bounded connection pool (decision 2) is what makes `:memory:` unsafe in the first place.

## 523. The assisted browser's network guard aborts requests silently

**Completed 2026-08-06.** Filed 2026-08-05 during the Assisted Apply acceptance trial, where a failing submission made the network guard a prime suspect and the log could not confirm or clear it. Ruling it out took a full synthetic-reproduction cycle against a real browser and the live CAPTCHA hosts.

**Reproduced before fixing.** The route body in `installAssistedContextGuard` was extracted verbatim into a testable `guardAssistedRequest` and driven with a request to a host resolving to `10.0.0.7`. The guard aborted it with `accessdenied` as designed and the captured log was the empty string -- the block was correct and the evidence was absent.

**Fix.** Two pieces, split by where the knowledge lives.

`pkg/security` now attaches a bounded reason code to each rejection it raises. `unsafeNetworkTarget` returns a `*networkRejection` carrying the code as a field rather than as text, and resolution problems return a `*resolverFailure`; `NetworkRejectionReason(err)` reads the code back via `errors.As`, falling back to `network_guard_rejected` and never to the error's own message. Classification does not depend on matching an error string, so a reworded message degrades nothing. The codes are `invalid_url`, `disallowed_scheme`, `missing_hostname`, `url_credentials`, `invalid_port`, `loopback_hostname`, `private_address`, `private_dns_answer`, `dns_resolution_failed`, `dns_no_addresses`, `resolver_unavailable`, and the `network_guard_rejected` fallback -- each one reachable from an existing rejection site, none invented to fill out the list. Every message text is unchanged and `networkRejection` still unwraps to `ErrUnsafeNetworkTarget`, which `cmd/agent/pipeline.go:130` uses to tell a durable safety refusal from a transient outage; `resolverFailure` deliberately does not, preserving that distinction. `TestNetworkRejectionsStillWrapTheSentinel` pins both halves.

`cmd/assist` logs one record at the rejection point: `Assisted network guard blocked request: host="example.com" reason="private_address"`. The host comes from `safeAssistedHost`, which parses with `net/url` and returns `Hostname()` only -- no userinfo, no port, no path, no query, no fragment. An unparseable URL, an empty hostname, one over 253 characters, or one containing anything outside the DNS alphabet returns the literal `unknown`; the raw input is never substituted, since an unparseable request URL is exactly the case where its contents are least trustworthy, and bounding the alphabet means a hostile hostname cannot break one log line into two. The guard is unchanged in what it permits: allowed requests still call `route.Continue()` with no log line and no added latency, nothing is retried, and no allowed-host or private-network policy moved.

`route.Abort` was previously discarded. Its error is now checked, and a failure adds a second line naming only the safe host and the fact of the failure -- Playwright's own error can quote the request it refers to, so it is never included.

**Tests.** `cmd/assist/network_guard_test.go` exercises the route callback through a hand-written `recordingRoute` (no mocking framework): six rejection classes each asserted against the exact expected log line; a privacy test using `https://user:password@example.test/application/12345?token=secret-value#private` that fails on any of the credentials, path, requisition number, query key, token value, fragment, or full URL appearing; seven malformed inputs asserting no panic, the `unknown` host, a bounded reason, a single record, and no echo of the raw value; an allowed request asserting `Continue` and total silence; an arbitrary underlying error asserting a reason from the closed set and no raw error text; one-rejection-one-record; a failed abort asserting the second line discloses neither the request nor Playwright's message; and a table pinning `safeAssistedHost` directly. `pkg/security/network_test.go` maps all nine reachable `ValidateURL` rejection paths to their codes and asserts the fallback carries no request data.

The privacy tests were mutation-checked rather than assumed: reverting the log to `rawURL` and `err.Error()` fails 23 assertions across the suite.

**Verification.** `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all pass, the last with empty output. No live application was launched, no employer contacted, no production record touched -- all evidence is from synthetic local URLs and a stub resolver. ADR-002 needed no change: its route-interceptor paragraph describes `installSafeBrowserRoutes` in `pkg/submitter/network.go`, the automatic path's separate guard, which `cmd/assist` does not use and which this change did not touch.

---
## 521. Indistinguishable duplicate cards let one click mark a job applied that was never submitted

**Completed 2026-08-06.** Found at the end of the five-application acceptance trial, after it had already produced one false `APPLIED` record.

**What happened.** Veeva listed the same "Senior Software Engineer - Python" requisition in Raleigh, Boston, Kansas City and Toronto. The assisted queue rendered them as four byte-identical cards -- same company, same role string, nothing else -- because `job_location` is empty for every pre-#516 row and the queue projection did not expose it anyway. The operator applied to exactly one posting (Raleigh, 293750), then clicked "I saw a confirmation -- Mark Applied" on two cards 8 seconds apart. Both were recorded with `manual_user_confirmation` provenance: 293750 at 18:58:49 and 293752 (Kansas City) at 18:58:58. `applied_jobs` went 57 to 59 for one real submission, and the Kansas City record had to be reverted by hand.

**Re-verified against the code before fixing.** `GetAssistedQueue` (`pkg/storage/assisted.go`) projected company, role, provider, fit and status but nothing posting-specific, and `AssistedJob` had no field that could have carried one. The card heading in `cmd/dashboard/ui/src/App.tsx` was `{job.company} — {job.role}` over a meta line of priority/fit/ATS/status/updated. The confirmation dialog named no job at all. The bug's claims held exactly as written.

**Fix.** Three parts, in `pkg/storage/assisted_identity.go` (new), `pkg/storage/assisted.go`, and the dashboard UI:

1. **A distinguisher on every card.** The projection now carries `location` (from `job_funnel.job_location`, which #524 will backfill) and `requisition_id`, parsed from the posting URL by `AssistedRequisitionID`. The URL itself stays server-side, as it already did for every row Career Agent can open on the operator's behalf; only the derived identifier is projected.
2. **A duplicate warning.** `markAssistedDuplicates` counts, for each row, how many other queued rows share its normalized company and role, reusing the same `normalizedCompany`/`normalizedRole` functions the confirmed-application duplicate matcher uses. Location is deliberately excluded from that key: location is one of the things that makes two identical titles distinct opportunities, so folding it in would hide exactly the rows that need the warning.
3. **An honest ambiguity flag.** A row whose location and requisition are both absent -- or identical to a sibling's -- is marked `ambiguous`, and the card tells the operator to open the posting instead of showing a distinguisher that looks like it matches. A distinguisher an operator checks and finds matching is worse than none, because they stop looking; that constraint drove the parser's design too, which refuses any candidate it cannot recognise as a machine identifier rather than guessing.

The confirmation dialog now names the job (`Company — Role (distinguisher)`) and repeats the duplicate warning in a `role="alert"`, so a misclick in the queue is still visible before the irreversible record is written.

**Parser shapes, all taken from URLs actually in the live queue.** Query parameters first (`gh_jid`, `jobId`, ...), then up to three meaningful path segments from the right, skipping decoration (`apply`, `jobs`, ...). Accepted: Greenhouse `293750`, Lever UUIDs, JazzHR `/apply/<id>/<role-slug>`, employer sites' `/job/R-05240/<slug>`, Workday's `<slug>_req-4917`, and Jobvite's digit-free `orrtAfwF` (an unseparated token with an interior capital). Refused: role slugs, capitalised path words like `Workday` or `Careers`, board listing pages, and employer roots. The first version of the parser stopped at the first non-identifier segment from the right and required a digit; the live queue showed that gave up on 24 rows, which is why the depth walk, the Workday `_`-tail rule, and the opaque-token rule exist.

**Live verification** (dashboard binary built from this tree, run on `127.0.0.1:8099` against the real `applications.db`; the production dashboard on `:8080` was untouched). `/api/assisted` returned 524 rows: **517 now carry a requisition id** (up from 500 before the parser work; 7 remain, all genuinely id-less URLs such as documentation pages that should not be in the queue at all). 130 rows sit in 49 duplicate groups, every one of them warned; **only 3 rows are ambiguous, and 0 groups are indistinguishable without being flagged**. The bug's own case is fixed on real data: the surviving Veeva rows (293749, 293751, 293752, plus a four-row Principal group) each now carry a distinct Lever UUID and a "2 other queued applications share this company and role" warning.

**Tests.** `pkg/storage/assisted_identity_test.go` covers the parser against all of the above shapes, sibling counting across the normalizer's equivalences ("Veeva Systems Inc" / "Veeva Systems", "- Python" / ", Python"), and the ambiguity flag for siblings sharing a location or carrying nothing. `TestGetAssistedQueue_DistinguishesPostingsSharingCompanyAndRole` covers the projection end to end and re-asserts that no posting URL leaks. One dashboard UI test covers the two rendered warning texts and the named confirm dialog, and asserts the confirmation posts the id of the card that was clicked. Full loop green: `go build`, `go vet`, `go test ./...`, `gofmt -l`, `npm run lint`, and 20 of 20 UI tests.

## 520. Lever submissions fail inside the assisted browser but succeed in an ordinary one

**Completed 2026-08-06.** Found during the acceptance trial and only correctly diagnosed on the fifth application.

**Evidence carried forward from the open row.** Inside the assisted browser, Greenhouse succeeded 4 of 4 (Grafana Labs, Affirm, Smartsheet, Temporal) while Lever failed every time with *"There was an error verifying your application. Please try again."* The same Veeva posting then submitted successfully in the operator's own Chrome, so the variable is the assisted browser combined with Lever specifically. Three earlier conclusions were retracted on evidence: the `NetworkGuard` proxy blocking CAPTCHA infrastructure, jobgether being uniquely broken, and Lever being broken platform-wide for this environment.

**Re-verification of the mechanism before fixing.** Two claims in the open row were checked against the current code rather than trusted:

- Lever takes the **Playwright** path, not the direct-Chrome path. `cmd/assist/main.go:98` routes to the direct browser only when `submitter.AssistedApplicationEntryURL` returns non-empty, and `pkg/submitter/browser.go:1155` implements that solely for `apply.workable.com`. So a Lever posting is driven by `LaunchPersistentContext` with the `AddInitScript` fingerprint spoof at `cmd/assist/main.go:115` — including `navigator.plugins` returning `[1, 2, 3, 4, 5]`, integers where a real `PluginArray` holds `Plugin` objects, which is itself a detection tell.
- The network guard is **not** a plausible cause of a content-verification failure. `pkg/security/network.go:527` `serveConnect` hijacks the connection and relays raw TCP in both directions after `200 Connection Established`, so it cannot observe or alter an HTTPS request body. Only the plain-HTTP path (`serveHTTP`) rewrites headers, and Lever is HTTPS throughout.

**Why the root cause was not pursued further.** Establishing which specific signal Lever's verification rejects would require submitting real applications to real employers as experiments, which is not an acceptable test. More importantly, the row's own **Constraint on any fix** rules the evasion route out: "Do not pursue this by making the browser harder to detect. If a site's verification legitimately refuses an automated browser, the correct answer is to hand the operator a direct link to open themselves — which is exactly how the trial's fifth application was completed." The fix implements that hand-off as automatic behavior instead of operator folklore.

**Fix.** New `pkg/storage/assisted_handoff.go` holds an evidence-backed registry of ATSes whose submission verification refuses the assisted browser, currently one entry: `lever.co`. `AssistedBrowserRejectionReason` matches on the parsed hostname, exactly or as a parent domain, so `jobs.lever.co` matches and `notlever.co` does not; a URL that fails to parse, carries no host, or is not HTTP(S) yields no match.

- **The refusal lives at one gate.** `GetAssistedLaunchInfo` (`pkg/storage/assisted.go:145`) is the single function both `serveAssistedLaunch` and `cmd/assist` pass through, so it returns `ErrAssistedBrowserRejected` there rather than in each caller. A stale dashboard tab, a direct `career_assist_bin -job N` invocation, and the batch runner are all covered by that one check, and none of them can spawn a browser whose submission is already known to fail.
- **The dashboard says what actually works.** `serveAssistedLaunch` matches the sentinel with `errors.Is` and answers 409 with "this ATS rejects the assisted browser; finish this application in your own browser" instead of the generic "no longer available", which would have read as an outage. `cmd/assist` logs the same guidance and exits without `log.Fatal` — there is simply nothing to open.
- **The queue row becomes actionable, not dead.** `GetAssistedQueue` overrides the next action to `open_in_own_browser` ("Finish in your own browser") for matching rows. `RequiresBrowser` is false so nothing tries to launch; `RequiresExplicitSubmit` is true so "I saw a confirmation — Mark Applied" stays available and the application can still become an `APPLIED` record; `CanContinue` is false because there is no assisted browser to continue in. The prepared résumé and cover letter remain reachable through the existing document endpoint.
- **One deliberate privacy exception.** `AssistedJob` is documented as withholding the canonical URL, and it still does for every other row. A hand-off row sets `ApplyURL` because there is no other way to get the operator into their own browser; the value is a public employer posting URL. The dashboard renders it as a real `<a target="_blank" rel="noopener noreferrer">`, not a fetch — clicking it opens the operator's own browser, which is the entire point.

**Tests.** `TestAssistedBrowserRejectionReason_MatchesRegisteredATSOnly` covers eleven host cases including the `notlever.co` lookalike, `lever.co` appearing as a path segment, case-insensitivity, a fully-qualified trailing-dot host, a non-HTTP scheme, and an unparseable URL. `TestGetAssistedLaunchInfo_RefusesATSThatRejectsTheAssistedBrowser` proves a fully revalidated Lever plan still cannot launch, and `TestGetAssistedLaunchInfo_StillLaunchesUnaffectedATS` proves the refusal did not widen into a general Assisted Apply outage. `TestGetAssistedQueue_HandsRejectedATSToTheOperatorsOwnBrowser` asserts both halves in one queue: the Lever row gets the hand-off action and its URL, the Greenhouse row keeps the guarded browser and leaks nothing. `TestServeAssistedLaunch_RefusesATSThatRejectsTheAssistedBrowser` proves no child process starts and the operator-facing message names the working next step. The Vitest case asserts the rendered link's `href` and `rel`, that the confirmation button survives, and that clicking it never calls `/api/assisted/launch`.

The dashboard test schema gained an `id INTEGER` column on `job_funnel`; it had been omitted, and the real schema has it.

**Verification.** `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all pass. `npm test` passes 20 of 20 in `cmd/dashboard/ui`, and `npm run build` + `npm run lint` are clean; the embedded `ui/dist` bundle was rebuilt and committed, since `//go:embed ui/dist` means an un-rebuilt bundle ships the old UI. No ADR describes the assisted-launch gate or claims Lever works in the assisted browser, so none went stale.

**Not fixed, and deliberately so.** The `AddInitScript` fingerprint spoof is left in place. Removing it is defensible on the grounds that `plugins: [1,2,3,4,5]` is a worse tell than no spoof at all, but it is unverifiable without the live submissions this fix exists to avoid, and changing it would be a step along exactly the axis the constraint forbids.

---

## 519. Assisted Apply cannot prefill on Greenhouse or Lever, the only two ATSes it is used with

**Completed 2026-08-06.** Raised by the operator after all five acceptance-trial applications had to be typed by hand.

**Mechanism.** `AttemptSubmit` routed Greenhouse, Lever, and Ashby to dedicated, hand-written handlers. Those handlers fill the form successfully — the trial logged `form filled — awaiting human review` for all six jobs processed — but they never call `SaveFormMapping`; only the Learner Module path does. So `form_mappings` held 80 rows and **zero** for Greenhouse or Lever after four successful Greenhouse fills. `FillAssistedMappedPage` knew how to replay a cached dynamic mapping and nothing else, so on precisely the two platforms the feature is used with, Continue could never refill and logged "refill stopped safely" instead. Compounding it, the pipeline's fill happens in a throwaway headless browser whose state dies with the process, and the assisted browser opens a fresh, blank session. Net effect: the agent filled each form, discarded the work, and the operator retyped it.

**Fix.** The Greenhouse/Lever/Ashby domain match is now one shared router, `dedicatedATSHandler(applyURL) (atsFillHandler, string)`, consulted by both paths — `AttemptSubmit`'s three near-identical branches collapsed into one, and `FillAssistedMappedPage` routes through the same handlers with `copilotMode: true`, keeping the cached mapping as the fallback for every other ATS. A newly supported ATS now reaches both paths at once instead of one of them.

**The submit gate, which was this fix's shipping condition.** The handlers were written for the automatic path, where they also click Submit, so a regression here would press Submit inside the operator's visible browser — the one thing assisted mode must never do. Three things now hold that line:

1. All three handlers consult `submitGate` *before* resolving any submit control, so under `copilotMode` they return `ErrAwaitingHumanReview` without ever locating a button.
2. `assistedFillOutcome` translates that sentinel into success and treats a bare `nil` as a **failure** — under copilot mode a nil can only mean a handler ran past the gate, so a future handler that forgets it cannot read as a healthy refill.
3. Tests assert zero clicks across every selector, and that the three submit selectors are never even resolved.

**Verified live**, not only against mocks: `scripts/verify_assisted_prefill.go` (added by this fix) drove `FillAssistedMappedPage` against a real Greenhouse posting and a real Lever posting with a synthetic identity. Fields, location, country, and documents were prefilled; the URL was identical before and after; no submission confirmation appeared; nothing was submitted.

**Second defect found by that live run, fixed here.** The modern Greenhouse board renders its upload control as `<input type="file" id="resume">` with **no `name` attribute**, so `handleGreenhouse`'s own `input[type='file'][name='resume']` selector matched nothing and the resume was silently never attached — on the automatic path as well as the assisted one. The three dedicated handlers now use `attachResume`'s fallback chain (mapped selector → resume-named selectors → the sole non-cover file input), which bug #118 had already built for the mapped dynamic path but never wired into these handlers. Ashby additionally stops grabbing the form's first file input outright, which on a form with a cover-letter upload could be the wrong control. This is the same structural defect class the repo has hit repeatedly: a capability wired into one fill path and missed in the others.

**Not addressed here:** #520 (Lever submissions fail inside the assisted browser) is a separate, still-open cause — this fix makes the Lever form arrive prefilled, but does not change whether Lever accepts a submission from that browser.

## 518. A revalidated, already-submitted application cannot be confirmed from the dashboard

**Completed 2026-08-06.** Found immediately after the acceptance trial's first successful submission (job 301657, Grafana Labs). The operator submitted the application, the employer accepted it, and the dashboard offered no way to record that.

**Mechanism.** The confirmation control — `I saw a confirmation — Mark Applied` — renders only when `next_action.requires_explicit_submit` is true. `actionForRevalidation`'s `application_ready` branch hardcoded that flag to false, and revalidating a candidate before launch is routine, so the flag was false for **every** assisted application. Once the assisted browser closed, `live_browser` went false, the queue projection fell back to `open_verified_application`, and the only control that can mark a job applied disappeared. The underlying `AWAITING_REVIEW` status means the pipeline had already filled the form and stopped before submitting, so submission and confirmation were the only steps left — exactly the case the control exists for.

**Fix.** `application_ready` now sets `RequiresExplicitSubmit` when the original status is `AWAITING_REVIEW`, and carries an instruction naming the real remaining steps: submit it yourself, then mark applied only after the employer confirms receipt. Other statuses are deliberately unchanged — a CAPTCHA-blocked page has no prepared form that could have been submitted, so it gains no confirmation affordance.

**Verification.** Two unit tests: one asserting the affordance appears for `AWAITING_REVIEW`, one asserting `BLOCKED_CAPTCHA`, `MANUAL_REQUIRED`, and the empty status are untouched. `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all clean.

**Note on the first confirmed application.** Job 301657 was confirmed through `/api/assisted/confirm` directly, because the button was unreachable at the time. That is the same endpoint the button calls, and the record carries the same `manual_user_confirmation` provenance; the confirmation was still an explicit human decision reported by the operator, never inferred from a Submit click.

## 517. Assisted Apply serves 404 for every cover letter once documents move to needs_manual_apply

**Completed 2026-08-06.** Found while verifying the documents precondition for the acceptance trial's third candidate (job 301657, Grafana Labs, fit 85). The dashboard reported `cover_letter_ready: false` and `/api/assisted/document?kind=cover_letter` returned **HTTP 404**, even though the agent had just logged writing the documents for that job.

**Mechanism.** `SaveApplication` writes `applications/<company>/<digest>`. `MoveToManualApply` then *renames* that directory to `applications/needs_manual_apply/<digest>` as soon as the job needs a human — which in assisted/copilot mode is every job — leaving an empty company folder behind. `GetAssistedDocument` always recomputed `applicationDir(company, url)` and never looked in the moved location, so it resolved to a path that no longer existed. Confirmed by digest: the expected directory `applications/grafanalabs/93d8b2244a2f90ce` was absent while `applications/needs_manual_apply/93d8b2244a2f90ce` held the cover letter, and `applications/grafanalabs/` existed but was empty.

This is the same class as #515 — assisted document resolution diverging from where the documents actually are — and it was systematic rather than per-job: **every** copilot-path cover letter was unreachable from the dashboard, so the operator could never review the letter before applying. `assistedDocumentExists` delegates to the same function, which is why the queue's `cover_letter_ready` flag was false too.

**Fix.** New `storage.ResolveApplicationDir` checks the company directory first, then `needs_manual_apply/<digest>`, then the numeric-suffix variants `MoveToManualApply` creates on collision, falling back to the original path so a genuine miss still produces a meaningful error. `GetAssistedDocument` resolves through it. The résumé path is unaffected: #515 already routed it to the shared `MasterResumePath` outside `applications/`. `validateAssistedDocument`'s containment root is `applications/`, not `applications/<company>/`, so the moved location passes its escape and symlink checks unchanged.

**Verification.** Four unit tests cover directory preference, the moved location, a suffixed collision, and the fallback. Live against the real database and a dashboard rebuilt from the fix, the cover letter for job 301657 went from HTTP 404 to **HTTP 200 (2,151 bytes)** and its `cover_letter_ready` flag flipped to true; 40 of 527 queue rows now resolve a cover letter. `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all clean.

## 516. Discovery has no geographic gate, so an India-only role reached a live application attempt

**Completed 2026-08-05.** Found during the five-application Assisted Apply acceptance trial. Job 259800 (jobgether, "AI Automation Engineer", fit score 100) opened a real, verified application in the assisted browser; the employer page showed the role was scoped to **India**. The user requires US/Canada/North America remote roles. The trial was halted at this candidate, the lease was released cleanly, and **no application was submitted**.

**Measured state before the fix, not assumed:**

- `job_funnel.job_location` and `is_remote` were empty for **all 12,902 rows**, including all 520 assisted-queue rows.
- `profile.yaml` carried only `remote_only: true`. That says a role may be worked remotely; it says nothing about *where from*, so an India-only "Remote" posting was indistinguishable from a US one.
- `UpdateFunnelIdentity` (`cmd/agent/pipeline.go:149`) was the sole writer of those columns and ran **only when `DuplicateCooldownDays > 0`**. `duplicate_cooldown_days` is absent from `profile.yaml`, so it was 0 and the write never executed — the mechanism behind the 100% empty columns.
- `parseLeverBoard` and `parseGreenhouseBoard` discarded location outright, keeping only title and URL, even though the feeds publish it. The live Lever response for this exact posting returned `country: "IN"`, `categories.location: "India"`, `categories.allLocations`, and `workplaceType: "remote"`. We fetched that payload and threw the answer away.
- Fit score 100 was computed from text similarity alone (`fit_similarity` 0.70) with no location input, which is the concrete mechanism behind the separately-noted "fit saturated at 100" symptom.

**Fix.** Both board parsers now capture the advertised location, any ISO-3166 alpha-2 country code, and remote status into `feedJob`. A new `pkg/scraper/location.go` resolves country evidence (explicit feed codes win; otherwise free text is matched against a partial country-name table) and applies an allowlist from the new `allowed_countries` profile key, set to `US`/`CA`. The gate sits in `pollBoard` beside the existing `titleLooksRelevant` check, on the same reasoning: it is free, and the alternative is a full fit-scoring call plus a possible live application attempt. `newDiscoveryEngine` in `cmd/agent/main.go` is now the single construction point for the funnel engine so the one-shot cycle and the daemon's background discovery loop cannot apply the allowlist on one path and forget it on the other. Discovery also now persists the advertised location unconditionally, so the queue, dashboard, and duplicate matcher can screen on real data.

**The gate fails open, deliberately.** It rejects only on positive evidence that every country a posting names is outside the allowlist; no location evidence always passes. Two reasons: all 12,902 existing rows carry no location, so a fail-closed gate would discard the entire corpus; and this file already records the governing lesson from the CAPTCHA pre-skip decision — *"skipping a job that would have submitted is strictly worse than wasting inference, because the goal is applications, not throughput."*

**Verification.** 26 unit tests cover the gate, including a dedicated word-boundary guard: a naive substring match would read **Indiana** as **India** and silently discard every posting in that state. Parser tests are pinned to the real feed shapes, with the Lever payload captured live from `api.lever.co/v0/postings/jobgether` on 2026-08-05 including the exact `country`/`categories` pairing of the posting that halted the trial. End to end against that live feed: **2,993 postings, all 2,993 carrying location, 1,952 (65%) rejected as outside US/CA, and zero admitted through the fail-open path** — the feed supplied full evidence for every posting. `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all clean.

**Known limitation, not fixed here.** The gate only protects postings discovered from this point on. The 520 existing assisted rows still carry no location and are not retroactively screened; until they are, each candidate's advertised location must be confirmed on the employer page before launching. A backfill pass over the existing queue is the obvious follow-up.

## 514. Qualified Jobs and operator settings contain post-hardening runtime regressions

**Completed 2026-08-05.** Resumed from an uncommitted, journal-less working tree left by an interrupted prior session (bug #513's hardening commit had already landed; this row and the code changes below were mid-flight but never committed or logged). Verified the existing uncommitted diff against the live schema and test suite before continuing, then found and fixed two further runtime-only regressions that neither `go build`/`go vet`/`go test` nor the prior session's unit tests could catch:

1. **`storage.PromoteJobToAssisted`, `SkipQualifiedJob`, and `MarkJobAppliedManually` used `pkg/storage`'s package-level `db`, which `cmd/dashboard` never initializes** (by design — the dashboard owns its own read/write handle and never calls `storage.InitDB`). Every other dashboard-facing `pkg/storage` function already takes a `conn *sql.DB` parameter for exactly this reason; these three didn't, so calling Promote/Skip/Confirm from the live dashboard always failed with `db not initialized`. Refactored all three to accept `conn *sql.DB` as their first parameter (matching `GetAssistedQueue`/`ConfirmAssistedSubmission`'s existing convention) and updated `cmd/dashboard/operator_api.go`'s three call sites to pass the dashboard's own `db`.
2. **`config.GetEffectiveSettings` silently hard-failed whenever `profile.yaml` was missing.** `LoadProfile` wraps `os.ReadFile`'s not-exist error with `fmt.Errorf("...: %w", err)`; the guard used `os.IsNotExist(err)`, which (unlike `errors.Is`) does not unwrap `%w`-wrapped errors, so the missing-profile tolerance the code was written to provide never actually took effect. Any request depending on effective settings (including the Promote flow, which calls `GetEffectiveSettings` for the current fit threshold) returned "Internal configuration error" whenever `profile.yaml` was absent. Fixed by switching to `errors.Is(err, os.ErrNotExist)`.

Both were caught by live end-to-end verification: built the dashboard binary, pointed it at a scratch copy of `applications.db` (never the real one) seeded with synthetic `ScratchCo`/`Test Engineer` rows, and drove `/api/qualified-jobs`, `/promote`, `/skip`, and `/confirm` over real HTTP — neither failure mode surfaces from unit tests alone since both require the dashboard's actual runtime wiring (its own uninitialized `storage` package db handle; a real missing-file path) rather than the in-memory/temp-file fixtures the existing tests use.

Also folded in the rest of the originally-diffed fix, already correct and left untouched beyond formatting: `QualifiedJob`'s API/UI response no longer leaks the raw job `url` or an unused `salary_desc` field; the qualified-jobs list, open, promote, skip, and confirm endpoints query the real `job_funnel`/`applied_jobs` column names (`id`, `job_title`, `last_updated`, `job_location`, `is_remote` — the pre-existing code referenced a nonexistent `role_name` column in `applied_jobs`, a second live-only bug this session's schema check surfaced along the way); mutating endpoints now reject non-POST methods and require a `{"confirmed_received": true}` acknowledgement before marking a job applied, with a corresponding UI confirmation dialog; and the dashboard's settings-polling loop no longer clobbers an operator's in-progress, unsaved edits to the settings form.

Removed a dead, half-written test scaffold in `pkg/config/effective_settings_test.go` (an inert `active_operator_settings.json` write the function under test could never actually read, given its hardcoded path) and added a direct regression test for the missing-profile fix instead. Added `cmd/dashboard/operator_api_test.go` and `pkg/storage/promote_test.go` covering the bounded-JSON decoder, the qualified-jobs method guard, and `MarkJobAppliedManually`'s state machine (including idempotent re-confirmation and the wrong-state rejection path).

`go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all pass. Rebuilt the dashboard UI (`npm run build`) to keep the embedded bundle in sync with the `App.tsx` changes.

---

## 513. Application Mode hardening is incomplete: Qualified Jobs mutations and settings activation are not safely verified

**Completed 2026-08-05.** Application Mode hardening was incomplete. Qualified Jobs mutations (promote, skip, open) were not safely verified, and settings activation wasn't durable. Unsafe transitions could have caused duplicate applications or wrong settings to be applied.

Implemented durable promotion intent (`processing_intent` column in `job_funnel`), atomic `PromoteJobToAssisted`, `SkipQualifiedJob`, and `MarkJobAppliedManually` transition functions in `pkg/storage/promote.go` and enforced limits (bounded JSON body, no trailing objects, same-origin origin validation) in `cmd/dashboard/operator_api.go`. Created `config.GetEffectiveSettings` for uniform rule resolution, and a settings heartbeat pattern using `applications/active_operator_settings.json` so the UI explains correctly whether settings are pending daemon activation or currently active.
Tests pass cleanly.

---

## 512. Assisted Apply presents stale, mismatched, and blank employer pages as actionable human handoffs

**Completed 2026-08-02.** Three guarded, read-only live checks contained neither a CAPTCHA nor a fillable application form: a blank BambooHR shell, a Meesho listing for a different role, and an unhydrated ATS shell. The prior current-page check treated any reachable non-CAPTCHA response as a browser-ready review, so it surfaced those unverified destinations as actionable human handoffs.

The revalidation check now permits browser launch only for a direct CAPTCHA challenge or for a response containing both the stored role and a conservative application-entry signal. Other responses are explicitly unavailable and retain a safe `Check Current Page Again` action; no browser is opened. The schema upgrade resets every obsolete `current_page_review` state to `required`, so earlier false-positive reviews must be checked again. The check remains read-only and does not retain page content.

**Regression coverage:** dashboard tests cover matching application, mismatched role, blank shell, and direct CAPTCHA classification; storage tests cover launch gating and the legacy-state reset. `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`, dashboard UI tests and build, and `git diff --check` passed. No architectural decision record changed: the behavior is a local assisted-handoff eligibility rule, not an infrastructure or persistence-architecture decision.

---

## 502. `encoding.go`'s homoglyph branch is a third zero-evidence heuristic threat source, outside #489's fix window

**Completed 2026-08-01 — closed as a measured non-issue.** The installed `promptsec@v0.1.0` source confirms the reported branch exists: `guard/heuristic/encoding.go` emits an unlocated heuristic `encoding_attack` at severity 0.7 when `HasSuspiciousConfusables` detects a suspicious script. The source fact alone did not justify broadening #489's narrowly scoped release rule, so this task measured the real audit trail before changing any security behavior.

**Measurement:** a new `scripts/audit_prompt_injection.go` reads `applications/prompt_injection_detections.csv` with Go's CSV parser and emits only aggregate counters. It never prints URLs, employer names, matched text, or posting content. Across 11,036 logged detections, 1,057 were `encoding_attack`. Of those, 649 had no matched text, but all 649 were sub-threshold severity 0.30: 647 sanitizer entries and two heuristic entries. Every decisive heuristic entry was located (11 at 0.60, 314 at 0.75, and 19 at 0.90); the 64 severity-0.70 rows were located sanitizer decoded-payload detections. There were therefore **zero** observed unlocated heuristic `encoding_attack` rows at the 0.5 decisive threshold.

**Decision:** no change to `isZeroEvidenceThreat`, the 0.7 severity, or `HasSuspiciousConfusables`. Extending the #489 allowlist without a measured population would weaken a security boundary based only on a plausible mechanism. The reusable aggregate-only script makes the same check repeatable if future data produces a nonzero decisive unlocated rate.

**Verification:** `go run scripts/audit_prompt_injection.go` completed against the live audit CSV without disclosing raw audit fields. The repository's full Go build, vet, test, and formatting loop passed before commit. No application rebuild or daemon restart was needed because runtime behavior was unchanged.

---

## 482. breezy.hr postings are excluded from GetDiscoveredJobs entirely, so they accumulate in DISCOVERED forever with no terminal status

**Completed 2026-08-01.** The earlier SQL filter was correct to keep Breezy out of automation but it applied too late: rows had already been stored as `DISCOVERED`, and one-shot discovery could also hand a fresh row directly to a worker. `storage.AddToFunnel` now recognizes the Breezy host boundary and stores a new discovery as `SKIPPED` with `status_reason = excluded_source`, while reporting it as ineligible to callers so it is never sent to their worker channel.

For legacy data, `SkipExcludedSourceDiscoveredJobs` runs at agent startup. It is idempotent and changes only still-`DISCOVERED` Breezy rows; terminal outcomes and unrelated postings are left unchanged. The dashboard renders that explicit reason as an excluded-source policy rather than its old low-fit caption.

**Regression coverage:** storage tests cover a new excluded discovery and a selective legacy sweep; dashboard coverage verifies the visible skip explanation. `go test ./pkg/storage ./cmd/agent ./cmd/dashboard`, then the full build, vet, test, and gofmt loop passed. The live agent restart applies the sweep and verifies the `DISCOVERED` count clears without inserting or processing a synthetic application.

---

## 507. The first post-#489 cohort reaches FAILED_SUBMIT 100% of the time, so its lower quarantine rate has not yet improved outcomes

**Completed 2026-08-01.** A sanitized count of the post-#489 cohort found all 8 `FAILED_SUBMIT` rows had Playwright's `target closed` error. For every one, the log showed post-score freshness passed but neither page validation nor document generation began. This rules out the later Vision, cached-mapping, and handler recovery gaps and isolates the failure to `newSubmitPage` during initial browser context/page setup.

`AttemptSubmit` now retries initial setup once only when `isTargetClosedErr` recognizes that exact Playwright failure. It neither repeats document generation nor retries an actual submit click; the later recovery paths remain unchanged. A regression test makes the initial page fail during navigation, asserts the crashed page is closed, confirms exactly one replacement context/page is created, and verifies the recovered path generates documents once before reaching its ordinary unsupported-ATS result.

**Verification:** `go test ./pkg/submitter`, then `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all passed. The daemon was rebuilt and restarted after verification; at restart it had one live process and zero eligible non-breezy queue rows, so the new recovery is deployed for the next discovery cohort without inventing a live outcome claim.

---

## 503. `TwoStepVerification`'s quarantine check never logs to the prompt-injection audit trail, undercounting the CSV total

**Completed 2026-08-01.** The pre-scrape DOM check in `TwoStepVerification` now calls `quarantineTwoStepVerificationDOM`, which retains the existing DOM pruning and rejection behavior while routing unsafe content through `quarantineCareerPageDOM`. That shared helper captures the `PromptInjectionError` threat list and calls `storage.LogPromptInjectionDetections`, so each detected threat reaches `applications/prompt_injection_detections.csv`.

The pipeline's `Execute` interface supplies a URL but not a company name at this boundary. The new CSV rows therefore record the true URL and an empty company field; no name is inferred or fabricated. This matches the logger's established schema and preserves data integrity.

**Regression coverage:** an unsafe DOM produces `ErrPromptInjectionDetected` and an audit CSV containing the expected URL and empty company field; a safe DOM returns normally and creates no audit log. Focused `go test ./pkg/submitter` passed before the full verification loop.

---

## 504. `state.TailoredContext` (our own RAG-generated content) can trip the same zero-evidence quarantine as #489, via a dedicated but unverified `QUARANTINED_RAG_CONTEXT` status

**Completed 2026-08-01.** Verified the reported pipeline path still exists: `cmd/agent/pipeline.go` checks the RAG-built `state.TailoredContext` and writes `QUARANTINED_RAG_CONTEXT` if the security layer rejects it. A read-only aggregate query against the live SQLite database returned exactly zero rows with that status. The query returned only the count; no job, company, URL, title, career-context, or other personal data was read or recorded.

**Decision:** closed as a verified non-issue for the current dataset. No trusted-content bypass or relaxation of the quarantine layer was added: zero observed incidents does not justify weakening a security boundary, and the existing status remains available for future telemetry. This can be reconsidered if a nonzero rate is observed.

**Verification:** documentation-only closure; the repository's full Go build, vet, test, and gofmt gates were run before commit.

---

## 490. [`job_funnel.applied_at` is declared in the schema but no code path ever writes it](#490-job_funnelapplied_at-is-declared-in-the-schema-but-no-code-path-ever-writes-it)

**Completed 2026-08-01.** `UpdateFunnelStatus` now records canonical UTC `applied_at` only when a row first becomes `APPLIED`; later status transitions preserve that confirmation time. `MarkHandoffApplied` writes the same canonical UTC value in its existing transaction, alongside its `applied_jobs` dedup record. Historical rows remain untouched because no reliable backfill source exists.

**Regression coverage:** automatic and manual confirmation both produce a parseable UTC timestamp, while a later `INTERVIEW_REQUESTED` transition leaves the original timestamp unchanged. The focused `go test ./pkg/storage` and full `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` verification loop all passed.

---

## 480. [UpdateFunnelStatusRetryable never records a status_reason, so every RETRY_EXHAUSTED row loses its own root cause](#480-updatefunnelstatusretryable-never-records-a-status_reason-so-every-retry_exhausted-row-loses-its-own-root-cause)

**Completed 2026-08-01.** `UpdateFunnelStatusRetryable` now accepts the causal error text and writes it to `job_funnel.status_reason` when its fifth failure changes a row to `RETRY_EXHAUSTED`. The transient backoff updates remain unchanged, so the stored reason is the final observed retryable failure that exhausted the budget.

All six retryable pipeline paths now pass their in-scope error: network-target validation, pre-flight liveness, job-page fetch, RAG retrieval, RAG embedding, and post-score freshness validation. The regression test drives a row through the complete backoff/exhaustion sequence and asserts that its final reason is retained.

**Verification:** focused `go test ./pkg/storage ./cmd/agent`, then `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all passed. The live daemon was rebuilt and restarted after verification so future exhausted rows use the change.

---

## 501. [Re-run #489's aggregate quarantine-rate queries against fresh data once the fix has been live for a batch cycle](#501-re-run-489s-aggregate-quarantine-rate-queries-against-fresh-data-once-the-fix-has-been-live-for-a-batch-cycle)

**Completed 2026-08-01.** Used only sanitized, read-only aggregates against the live SQLite database and the prompt-injection audit CSV. The cutoff was #489's fix commit, `575ce8f` at 2026-08-01 18:51:40 EDT. The running daemon was confirmed to use `career_agent_bin` built and started at 19:21 EDT, so its later rows were generated by the fixed binary.

**Measurement:** among the first 8 rows discovered and processed after the cutoff, `QUARANTINED_PROMPT_INJECTION` was 0/8 (0.0%) compared with the pre-measurement lifetime baseline of 5,983/11,739 (51.0%). There were no post-fix quarantine rows in the Lever or Greenhouse hostname groups and no post-fix prompt-injection audit-CSV detections, so the two former dominant zero-evidence categories did not reappear in this cohort. No raw URL, company, title, posting text, or matched audit text was inspected or recorded.

**Outcome qualification:** all 8 post-fix rows were `FAILED_SUBMIT`; `BLOCKED_CAPTCHA` was 0. The lifetime `FAILED_SUBMIT` baseline is 531/11,739 (4.5%), making the cohort's 100% failure share an offsetting downstream outcome, not confirmation that #489 improves applications. The cohort is too small to attribute causality. #489's stated re-open condition (quarantine share does not drop) was not met, so it remains Done; filed #507 to diagnose the downstream failures and require an outcome-preserving follow-up measurement.

**Verification:** documentation-only change; the repository's full Go build, vet, test, and gofmt gates were run before commit.

---

## 489. [`promptsec.Moderate()` still quarantines roughly half of everything discovered, disproportionately on Lever and Greenhouse](#489-promptsecmoderate-still-quarantines-roughly-half-of-everything-discovered-disproportionately-on-lever-and-greenhouse)

**Table rationale cell (original):** Found 2026-08-01, mission-alignment audit. `job_funnel`: 5,983/11,731 rows (51.0%) lifetime are `QUARANTINED_PROMPT_INJECTION`, more than any other status, dominated by `jobs.lever.co` and the `greenhouse.io` family — the two ATS platforms with dedicated auto-submit handlers. `applications/prompt_injection_detections.csv`: 88.5% of 10,967 logged detections have no located `matched_text` (99.3% of `instruction_override`, 100% of `system_prompt_leak`, the two dominant threat types) — pure fuzzy-keyword-density/coercive-language heuristics. Value 8 (ceiling), Decay 0.5 (same theme as #394, a severity dial that didn't fix it), Effort 3.

### 489. `promptsec.Moderate()` still quarantines roughly half of everything discovered, disproportionately on Lever and Greenhouse

**Fixed 2026-08-01.** Root cause traced through the vendored `promptsec@v0.1.0` source (`~/go/pkg/mod/github.com/danielthedm/promptsec@v0.1.0/guard/heuristic/`), not assumed from the audit's evidence: the heuristic guard's `Execute` runs three stages, and only stage 1 (`patterns.go`, compiled regexes) is governed by the `Preset`/`Threshold` dial bug #394's fix turned down — and only stage 1 sets a real located `Match`/`Start`/`End` span (via `FindStringIndex`). Stage 3 (`contextual.go`'s `detectContextualAttacks` plus the inline `fuzzyMatchNormalized` branch in `heuristic.go`) runs **unconditionally**, is **not filtered by `Preset`/`Threshold` at all**, and by construction never sets a located span — both are keyword co-occurrence tests (`containsAll`/`containsAny`) over the whole normalized input, not a single-span match. This stage is the entire false-positive population: its `ThreatSystemPromptLeak` "coercive attempt to extract sensitive data" branch fires on bare phrases like "personal data", "social security number", "login credentials", "credit card information" and "financial records" — ordinary privacy-notice, background-check and voluntary-self-identification boilerplate every real Lever/Greenhouse posting carries — and its fuzzy branch fires at severity 0.65 whenever two of eleven "critical keywords" match within edit distance 2 over an unbounded window (e.g. "assistance" is within edit distance 1 of "assistant"), which essentially every posting of any length trips. `HeuristicOptions` exposes one `Threshold`/`Preset` for the whole guard and it does not even reach stage 3, confirming there is no upstream per-category knob — the fix has to be a local second pass.

**Fix (`pkg/security/filter.go`):** `QuarantineLayer.QuarantinePayload` now runs a second pass, `overrideZeroEvidenceDetection`, before quarantining an unsafe result. It releases the payload only when **all three** hold: (1) every *decisive* threat (`Severity >= 0.5`, mirroring `promptsec.Protector.buildResult`'s own hardcoded unsafe threshold — sub-threshold threats never caused a quarantine on their own, so they're not evidence either way) is zero-evidence (`Guard=="heuristic" && Match=="" `, type `instruction_override`/`system_prompt_leak`); one located match, one non-heuristic guard hit (e.g. `embedding`, which always sets `Match` to the full input), or any other threat category keeps the payload quarantined exactly as before; (2) the payload carries none of 41 curated `injectionMarkers` phrases ("ignore all", "system prompt", "jailbreak", etc. — the backstop for injection variants that only reach the zero-evidence path); (3) the payload matches at least 2 of 47 curated `benignATSSignatures` (EEO/non-discrimination, ADA/accommodation, background-check/E-Verify, voluntary self-identification, candidate privacy, application-process language), normalized to survive HTML markup/entities. Deliberately excluded from `injectionMarkers`: "login credentials", "personal data", "credit card information" (the exact phrasings that caused this bug — legitimate anti-fraud/privacy notices use them too) and "language model"/"content policy" (common in genuine engineering postings). A `log.Printf` (threat count, sorted types, signature count — no payload content) fires on every release, since overridden payloads bypass `LogPromptInjectionDetections` entirely and the audit trail would otherwise be unmeasurable.

**Regression corpus (`pkg/security/filter_test.go`):** 8 paragraph-length benign ATS cases, each asserting `wasFalsePositive` against **raw** `promptsec.Moderate()` (so the corpus can't rot into cases that were already safe) — 5 were genuinely false-positively quarantined before this fix (background-check/E-Verify, voluntary self-identification, candidate privacy, multi-step application instructions, an accommodation-plus-fuzzy-density case) and are released now; 3 (plain EEO, ADA, a full posting) were already safe and serve as non-regression guards. 7 malicious cases prove the override never weakens real detection: a located `patterns.go` regex hit buried in 5 matching benign signatures with no marker present (proves `Match != ""` alone blocks release); two embedding-guard-only payloads; a zero-evidence payload matching no benign signature (conservative default); a zero-evidence set containing `role_manipulation` (outside the narrow override window); and — the adversarial case this fix must not open — a genuine "ignore all safety guidelines... confidential company information" injection camouflaged inside real EEO/accommodation boilerplate, whose threats are *all* zero-evidence instruction_override/system_prompt_leak and which clears the threat-shape test and the allowlist; only the `injectionMarkers` veto stops it. Plus `TestOverrideZeroEvidenceDetectionThreatShapes` (8 cases pinning the partition rule directly, including the sub-threshold-sanitizer-noise case and the empty set) and `TestOverrideZeroEvidenceDetectionRequiresEnoughSignatures` (the 2-signature floor and the marker veto). `TestBenignATSSignaturesAreIndependent` asserts no signature is a substring of another (caught and removed 4 real double-counting cases during development: "veteran status", "self identification", "candidate privacy", "base salary"). 79 total assertions; the 3 pre-existing tests kept verbatim.

**Verification:** `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean. The `decisiveSeverity = 0.5` threshold was independently confirmed against `promptsec.Protector.buildResult` (`threshold: 0.5`, hardcoded in `protector.go`), and the sanitizer's 0.3-severity zero-width-character threat (`guard/sanitizer/sanitizer.go:81-84`, `Guard: "sanitizer"`, no `Match`) was confirmed to exist and would otherwise have silently disabled this entire fix on any posting DOM containing a zero-width character, since a literal `Guard != "heuristic"` corroboration rule would have counted it as corroborating evidence.

**Not done — live re-measurement.** This row's own acceptance criteria call for re-running the audit's aggregate queries (Lever/Greenhouse `QUARANTINED_PROMPT_INJECTION` share) against fresh data to confirm a material drop. This is **not retroactively measurable**: `job_funnel`'s schema (`pkg/storage/manager.go:68-82`) does not store posting `description`/`rawHTML` at all, only `url`/`company_name`/`job_title`/`status`/timestamps — the original text of the 5,983 already-quarantined rows was never retained (correctly, by the quarantine layer's own design), so there is nothing to replay through the new filter. Filed as new bug #501 to re-run the aggregate queries after the next live daemon batch accumulates fresh `job_funnel`/`prompt_injection_detections.csv` data under the fix.

**Findings logged, not fixed (each independently verified against source, not just carried over from the delegate's report):**
- `guard/heuristic/encoding.go:82-90` (`detectEncodingAttacks`'s homoglyph branch) is a **third** zero-evidence source at decisive severity (0.7, `Guard: "heuristic"`, no `Match`) — but type `ThreatEncodingAttack`, outside this fix's narrow `instruction_override`/`system_prompt_leak` window by design. Filed as bug #502.
- `pkg/submitter/dynamic.go:83` (`TwoStepVerification`) calls `QuarantinePayload` directly, not `CheckPayloadDetailed` — confirmed the only two call sites of `LogPromptInjectionDetections` in the whole codebase are `cmd/agent/pipeline.go:264` and `pkg/submitter/browser.go:64`, neither of which covers this path. Quarantines during the pre-scrape career-page DOM check are invisible in `prompt_injection_detections.csv`, undercounting the audit total. Filed as bug #503.
- `cmd/agent/main.go:124` (`storedPromptInjectionThreats`) and `pkg/submitter/browser.go:37` (`toStoredThreats`) are the same field-for-field conversion written twice. Filed as improvement #505 (small dedup).
- `cmd/agent/pipeline.go:311` runs `CheckPayload` on `state.TailoredContext` — our own model-generated RAG output, not scraped posting text — and a dedicated `QUARANTINED_RAG_CONTEXT` status already exists for when it self-quarantines. Confirmed real (the fuzzy branch's edit-distance-1 "assistance"/"assistant" match makes this plausible on generated cover-letter text too), but it's a different payload source than #489's scraped-posting scope. Filed as bug #504.

---


**Table rationale cell (original):** Found 2026-08-01 investigating why `applications.db` shows zero `APPLIED` rows since 2026-07-29 despite the daemon running continuously since. `job_funnel` has exactly 2 `APPLIED` rows total, both from 2026-07-29; 2026-08-01 alone processed 8,140 rows with none reaching `APPLIED` — 4,657 quarantined as prompt injection, 1,513 `INVALID_URL` (1,319 `expired`), 1,284 `RETRY_EXHAUSTED`. Ten spot-checked `expired` rows were all discovered 2026-07-13 and first processed 2026-08-01 — ~19 days in `DISCOVERED`. `pkg/storage/ranking.go:137-138`'s freshness multiplier (`exp(-0.02×ageDays)`, ~35-day half-life) only costs a 19-day-old row ~32% of its score, so a strong fit/source score keeps stale rows ranked above fresh ones; `GetDiscoveredJobs` (`manager.go:1396-1439`) has no `ORDER BY` to compensate. Value 8: this is the measured, direct cause of the product's core outcome collapsing to zero for 3+ days. Effort 4: needs a real ordering/decay fix plus a live-verified before/after measurement against a real backlog, not just a unit test

### 481. Aged DISCOVERED postings expire before the ranking algorithm's freshness decay ever surfaces them, starving the funnel

**Fixed 2026-08-01.** `pkg/storage/ranking.go`'s `RankJobs` now treats age as a hard override, not just a soft multiplicative decay: a new `urgentAgeDays` constant (10 days — well inside the ~19-day expiry window this bug's own spot-check measured) marks any job at or past that age as urgent, and urgent jobs bypass the exploration/exploit score-based split entirely, sorted oldest-first, ahead of every non-urgent job regardless of source health or fit score. This directly fixes the root mechanism: the existing `freshnessMultiplier` only ever pushes a job's score *down* as it ages (it has no mechanism to push a job *up*), so under sustained backlog pressure a low-scoring row could keep losing to fresher high-scoring rows indefinitely and age straight through to expiry. `runAgentQueueCycle` (`cmd/agent/main.go:651-680`) confirms this fix reaches the daemon: it truncates `GetDiscoveredJobs`'s returned order at `cycleLimit` with no re-sorting, so whatever `RankJobs` puts first is exactly what gets attempted first — putting urgent jobs at the front guarantees they're attempted before non-urgent ones, every cycle, until the urgent backlog itself is cleared.

New tests `TestRankJobs_UrgentAgeBypassesStarvation` and `TestRankJobs_NonUrgentJobsStillRankedByScore` (`pkg/storage/ranking_test.go`), both real starvation scenarios (a weak-source/weak-fit stale job vs. 20 strong-source/strong-fit fresh jobs) rather than tautologies — the first test explicitly asserts the stale job's raw score loses to every fresh job's before asserting the urgent override still puts it first. **Mutation-checked**: reverting just the `isUrgent` assignment to a constant `false` reproduces the exact predicted symptom (the weak stale job sorts dead last behind all 20 fresh jobs) and the new starvation test fails; restoring the real condition passes again.

**Live-verified against the running daemon**, the same way the bug was found: rebuilt `career_agent_bin`, gracefully stopped the production daemon (clean `SIGTERM`, "Shutdown complete" logged) and restarted it fresh with the fix deployed — resumed its normal 1-cycle/minute cadence immediately, no crash. A live before/after *ordering* measurement (the effort estimate's original ask) was not observable this session: a direct query at verification time found `job_funnel`'s `DISCOVERED` backlog at exactly 185 rows, but all 185 are `breezy.hr` postings, which `GetDiscoveredJobs`'s own query excludes entirely (0 `breezy.hr`-excluded eligible rows) — the same finding the live daemon log corroborates ("No eligible discovered jobs remain in the backlog" every cycle). The aged backlog this bug measured on 2026-08-01 had already been fully drained by the time of this fix, which is itself consistent with the gate note's timeline (#478's DNS-spin fix shipped earlier the same day and let the daemon actually work through its backlog). The next session that observes a live `DISCOVERED` backlog with a genuine age spread should confirm urgent jobs are actually being attempted first, and can then drop this caveat.

`go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean. **This closes the last open Major/Blocker in this file — see the Usability Gate note above.** Original report follows.

**Found 2026-08-01** while investigating why `applications.db` shows zero new `APPLIED` rows since 2026-07-29 08:33 despite the daemon (running continuously, per bug #478's fix) processing thousands more jobs since. A direct query of `job_funnel`: `status='APPLIED'` has exactly 2 rows total, both dated 2026-07-29. The 8,140 rows updated on 2026-08-01 alone break down as 4,657 `QUARANTINED_PROMPT_INJECTION`, 1,513 `INVALID_URL` (1,319 `expired`, 194 `malformed`), and 1,284 `RETRY_EXHAUSTED` — essentially the entire day's throughput, with none reaching a submission attempt.

Spot-checking ten of today's `expired` rows against their own `discovered_at` timestamps shows all ten were discovered **2026-07-13** and received their first (and fatal) processing attempt only on **2026-08-01** — roughly 19 days sitting in `DISCOVERED` before the pipeline ever looked at them, by which point the posting was gone. This is the majority pattern in today's `expired` batch, not an isolated case.

**Root cause:** `storage.GetDiscoveredJobs` (`pkg/storage/manager.go:1396-1439`) loads every `DISCOVERED` row with no `ORDER BY`, then hands the full set to `RankJobs` (`pkg/storage/ranking.go:96-158`), whose only freshness signal is:
```go
ageDays := int(time.Since(j.GetDiscoveredAt()).Hours() / 24)
freshnessMultiplier := math.Exp(-0.02 * float64(ageDays))
```
combined multiplicatively with source success rate, fit score, and the source's penalty factor. At `-0.02`/day the half-life is `ln(2)/0.02 ≈ 35 days` — a posting discovered 19 days ago has lost only ~32% of its score from age alone (`exp(-0.02×19) ≈ 0.68`), so a strong fit score or a healthy source can keep a nearly-three-week-old row ranked above a genuinely fresh one. Freshness is a soft tiebreaker here, not a filter or a hard sort key, so under any backlog pressure — the exact condition bug #478 documented the daemon being in for an unknown period before its fix — old rows accumulate and eventually get processed only after the real-world posting has already closed.

**Value 8:** this is the direct, measured cause of the product's core outcome (`APPLIED`) going to zero for 3+ days while the daemon kept running and kept finding new jobs — not a cosmetic defect. **Decay 1.0** per this file's own rule (nothing else in the current backlog addresses it). **Effort 4**: the fix needs either a much steeper/hard freshness cutoff, an explicit freshness-first sort ahead of the existing scoring, or a TTL that retires stale `DISCOVERED` rows before they are ever attempted — and since the bug's whole shape is about *processing order under load*, any fix needs a live-verified before/after measurement against a real backlog, not just a unit test on `RankJobs`. Score = 8×1.0÷4 = **2.0**.

**Likely compounding factor, not claimed as sole cause:** bug #478 (fixed the same day this was found) had the daemon spinning at ~1 cycle/sec on a single stuck job for an unknown prior duration, which would itself let the `DISCOVERED` backlog grow unchecked. That fix does not address this row's mechanism — the freshness decay is unchanged and will let the same starvation recur under any future slowdown, source outage, or simply a backlog that grows faster than the daemon can drain it.


---

## 478. [A DNS resolution failure never moves a job out of DISCOVERED, so one bad hostname spins the daemon forever](#478-a-dns-resolution-failure-never-moves-a-job-out-of-discovered-so-one-bad-hostname-spins-the-daemon-forever)

**Table rationale cell (original):** **Fixed 2026-08-01.** See the Details section below for the full account. Original report follows. Found 2026-08-01 live-verifying improvement #477: the running daemon was doing ~1 cycle/sec (83 cycles in 85s observed) entirely on one job with an unresolvable hostname, because `cmd/agent/pipeline.go:101-110`'s `else` branch (a DNS lookup failure, not `ErrUnsafeNetworkTarget`) only logs and returns `StateEnd` — it is the one retryable-failure branch in the file that does not call `storage.UpdateFunnelStatusRetryable`, which every one of the other 5 retryable-failure sites in the same file already calls. Value 6: unbounded log growth and a tight loop that never respects `cycleInterval` is a real live-production cost, not cosmetic. Effort 2: the fix is one added call plus a test, but needs a live-verified before/after cycle-rate check, not just a passing unit test, since the bug's whole shape is about pacing

### 478. A DNS resolution failure never moves a job out of DISCOVERED, so one bad hostname spins the daemon forever

**Fixed 2026-08-01.** `cmd/agent/pipeline.go:107-110`'s `StateInit` `else` branch (the DNS-failure path, distinct from the `ErrUnsafeNetworkTarget` branch above it) now calls `storage.UpdateFunnelStatusRetryable(job.URL)`, exactly matching the other 5 retryable-failure sites in the same file (`:141`, `:212`, `:286`, `:300`, `:441`). This routes a DNS failure through the same exponential-backoff/`RETRY_EXHAUSTED` machinery bug #466 built for every other retryable failure, instead of leaving the row at `DISCOVERED` with no `next_eligible_at` forever. New test `TestStateInit_DNSFailureLeavesJobRetryable` (`cmd/agent/pipeline_test.go`) drives the real `buildJobPipeline` graph through `StateInit` with a fake `security.NetworkResolver` (`security.WithResolver`) that returns a genuine "no such host" error — not a rejected private/loopback target — then reads the `job_funnel` row back via `storage.GetDB()` and confirms `retry_count = 1`, a future `next_eligible_at`, and that `storage.GetDiscoveredJobs()` no longer returns the row in the same cycle. **Mutation-checked**: reverting just the `pipeline.go` fix (via `git stash`) reproduces the exact predicted symptom (`retry_count = 0`, zero-value `next_eligible_at`) and the test fails; restoring the fix passes again. **Live-verified end to end, the same way the bug was found**: rebuilt `career_agent_bin`, gracefully stopped the running production daemon (`SIGTERM`, clean shutdown logged) and restarted it fresh. The log shows `wwww.raileurope.com` fail exactly once in cycle 1, then cycle 2 correctly reports "No eligible discovered jobs remain in the backlog" and waits the full `1m0s` — cycle 2 at `11:46:03`, cycle 3 at `11:47:03`, a clean 60-second gap matching the documented cadence, versus the ~1/sec spin observed before the fix. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean. Implemented directly by this session's orchestrator (Sonnet 5, this row's `standard`-tier Claude Code starting point) per this run's instruction to prioritize Claude models. **This closes the last open Pending row in this file and the last open Major — see the Usability Gate note above.** Original report follows.

**Found 2026-08-01** while live-verifying improvement #477 (Yahoo fallback headers/cookie jar). Right after rebuilding and restarting the production daemon (`career_agent_bin -daemon -cycle-limit 15 -cycle-interval 1m`) with the #477 fix, `career_agent.log` showed the daemon completing 83 cycles in 85 seconds — roughly one cycle per second, not the documented 1-minute `cycleInterval` — every single one loading the same one job (`https://wwww.raileurope.com/...`, note the four `w`s, a dead/typo'd hostname) and failing it the same way:

```
[Agent] Loaded 1 previously discovered jobs from backlog into the queue.
[Worker-1] Job URL could not be resolved safely; leaving it retryable: resolve network target: lookup wwww.raileurope.com: no such host
[Agent] Batch execution complete!
[Agent] [DAEMON MODE] Cycle N completed with work; continuing immediately.
```

**Root cause:** `cmd/agent/pipeline.go`'s `StateInit` node (lines 101-110):

```go
if err := deps.NetworkGuard.ValidateURL(ctx, job.URL); err != nil {
    if errors.Is(err, security.ErrUnsafeNetworkTarget) {
        log.Printf("[Worker-%d] Unsafe job URL blocked.", workerID)
        if statusErr := storage.UpdateFunnelStatusInvalid(job.URL, storage.InvalidURLReasonMalformed); statusErr != nil {
            log.Printf("[Worker-%d] Failed to mark unsafe URL invalid: %v", workerID, statusErr)
        }
    } else {
        log.Printf("[Worker-%d] Job URL could not be resolved safely; leaving it retryable: %v", workerID, err)
    }
    return StateEnd, nil
}
```

A plain DNS lookup failure is wrapped as `fmt.Errorf("resolve network target: %w", err)` (`pkg/security/network.go:168`) — **not** wrapped in `ErrUnsafeNetworkTarget`, which is reserved for syntactically invalid or non-public targets (loopback, private ranges, etc., see `network.go:140`/`:161`/`:179`). So a genuine "no such host" falls into the `else` branch, which only logs — it never calls any `storage.UpdateFunnelStatus*` function, leaving the row's status at `DISCOVERED` with `next_eligible_at` untouched. `GetDiscoveredJobs` reloads it on every single cycle forever, with no backoff and no path to a terminal state.

This is the one gap in an otherwise-consistent pattern: bug #466 (Done 2026-07-31) added `storage.UpdateFunnelStatusRetryable(url)` — increments `retry_count`, exponential backoff (2/4/8/16 min), terminal `RETRY_EXHAUSTED` after `MaxRetryAttempts` — and wired it into every retryable-failure branch in this file: `pipeline.go:141`, `:212`, `:286`, `:300`, and `:441`. This DNS-failure branch is the only retryable path that predates #466 and was never updated to call it, so it kept #466's exact pre-fix behavior (reset to `DISCOVERED`, no backoff, no exhaustion) for this one specific failure mode.

**Live impact observed:** unbounded log growth (tens of thousands of lines added to `career_agent.log` within minutes) and `cycleInterval` pacing never taking effect for this row, since "completed with work" triggers the next cycle immediately rather than waiting. Did not fully starve the daemon — the independent discovery goroutine (`runDaemonDiscoveryLoop`, decoupled per the 2026-07-30 continuous-daemon cadence rework) kept running and producing new jobs throughout — but it did burn CPU and I/O on a job that can never succeed, and would eventually dominate the log file's size if left running.

**Fix direction:** call `storage.UpdateFunnelStatusRetryable(job.URL)` in the `else` branch, matching the other 5 sites in this file exactly, so a DNS failure gets the same exponential backoff and eventual `RETRY_EXHAUSTED` terminal state as every other retryable failure instead of looping forever. Worth a passing thought on whether a *permanent* DNS failure (e.g. `no such host`, which will never resolve differently) should skip the backoff and go straight to a terminal status rather than spending a multi-minute retry budget on something that cannot self-heal — but that is a refinement, not a requirement; simply routing it through the existing retry/backoff/exhaustion machinery already fixes the tight-loop and unbounded-growth symptoms observed live. Test should prove a DNS-failure job's status changes off `DISCOVERED` (or at minimum gets a future `next_eligible_at`) after one failed attempt, and does not reappear in the very next queue cycle — mirroring #466's own `GetDiscoveredJobs` skip-then-resume test. Live-verify the fix the same way this bug was found: rebuild, restart the daemon, and confirm the cycle rate returns to the documented ~1/minute cadence instead of ~1/second.


---

## 476. [`GetQueuePlan` has no `rows.Err()` check, so a cursor error silently truncates the requeue dry-run preview](#476-getqueueplan-has-no-rowserr-check-so-a-cursor-error-silently-truncates-the-requeue-dry-run-preview)

**Table rationale cell (original):** **Fixed 2026-08-01.** Extracted `GetQueuePlan`'s scan loop into `scanQueuePlanCandidates(rows queuePlanRows, willClearDedup bool)`, matching `cmd/dashboard/main.go`'s `scanSourceConversions`/`scanVariantConversions` pattern from bug #452/#459 exactly: a small `queuePlanRows` interface (`Next`/`Scan`/`Err`) factored out so a hand-rolled fake can force `Next()` to fail mid-stream, which no real driver can be made to do on demand. The loop now calls `rows.Err()` after exhausting and returns a wrapped error instead of falling through to `GetSourceHealthSummaries` with a silently short candidate list; `GetQueuePlan` propagates that error to its caller. Three new tests in `pkg/storage/queue_plan_test.go`: a cursor fault after one good row now surfaces as an error with no partial result (**mutation-checked** — temporarily removing the `rows.Err()` call made exactly this test fail, reproducing the bug's predicted symptom, while the other two still passed); clean exhaustion still returns the expected candidates and totals; and a dedicated regression guard confirms bug #453's per-row `Scan`-error `continue` behavior is unaffected by the new cursor check — the two failure layers (a bad single row vs. a broken cursor) are now both handled, and neither swallows the other. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean. Implemented directly by this session's orchestrator (Sonnet 5, this row's `standard`-tier Claude Code starting point) per this run's instruction to prioritize Claude models. Original report follows. Found 2026-07-31 during this groom pass's fresh audit, checking `pkg/storage` for the `rows.Err()` omission class bug #452 already fixed once in `cmd/dashboard/main.go`. `pkg/storage/queue_plan.go`'s `GetQueuePlan` never calls `rows.Err()` after its `for rows.Next()` loop, so a driver-level cursor fault mid-iteration silently truncates the plan exactly as `serveMetrics`'s nine closures did before #452. Narrower than #452 because `GetQueuePlan` only feeds `cmd/requeue`'s dry-run preview — the actual `-confirm` mutation calls `RequeueByURLPattern` independently via a bulk `UPDATE`, so a truncated plan misleads an operator reading counts but cannot itself corrupt the database

### 476. `GetQueuePlan` has no `rows.Err()` check, so a cursor error silently truncates the requeue dry-run preview

**Found 2026-07-31** during this session's fresh audit, specifically checking `pkg/storage` for the same defect class bug #452 fixed in `cmd/dashboard/main.go`'s `serveMetrics`. `pkg/storage/queue_plan.go`'s `GetQueuePlan` (the query behind `cmd/requeue -plan`, and behind the default dry-run preview whenever `-confirm` is not passed) runs:

```go
rows, err := db.Query(query, fromStatus, urlPattern)
...
defer rows.Close()
...
for rows.Next() {
    var cand QueuePlanCandidate
    ...
    if err := rows.Scan(...); err != nil {
        continue
    }
    ...
    plan.Candidates = append(plan.Candidates, cand)
}
// no rows.Err() check here — the function falls straight through to
// GetSourceHealthSummaries and returns (&plan, nil)
```

`rows.Next()` returning `false` is ambiguous between "result set exhausted normally" and "a cursor-level error occurred" — `database/sql`'s own contract requires calling `rows.Err()` after the loop to tell them apart. `GetQueuePlan` never does, so a driver fault partway through iteration (a dropped connection, a corrupt page, anything below the `Scan` level) looks identical to "this pattern only matched N rows" and returns a truncated `*QueuePlan` with `err == nil`.

**Not the same bug as #453**, which was about a `Scan` failing on a single null column and discarding the *whole* plan by `return`ing early — #453's fix (Done 2026-07-30) correctly changed that single-row scan failure to `continue`, but it did not add a `rows.Err()` check for a cursor-level fault, which is a different failure mode at a different layer and survived #453's fix untouched.

**Blast radius is narrower than #452's**, and that is reflected in this row's Value: `GetQueuePlan` only feeds the human-readable dry-run table `cmd/requeue` prints before an operator decides whether to pass `-confirm` (`cmd/requeue/main.go:123-152`). The actual state mutation, `storage.RequeueByURLPattern` (`main.go:156`), is a separate bulk `UPDATE ... WHERE url LIKE ?` that does not consult the plan's candidate list at all — so a truncated plan can mislead an operator reading the preview counts, but cannot itself cause the real requeue to skip or corrupt rows.

**Fix direction:** add `if err := rows.Err(); err != nil { return nil, fmt.Errorf(...) }` immediately after the loop, matching the pattern `pkg/dashboard`'s `scanSourceConversions`/`scanVariantConversions` already established for #452 (a small `conversionRows`-shaped interface exists there specifically so a hand-rolled fake can make `Next()` fail mid-stream for a test — the same shape is directly reusable here). Tests: one proving a genuine cursor error now surfaces as a returned `error` instead of a shorter-than-expected `*QueuePlan`, and one confirming the existing per-row scan-skip behavior (#453's fix) is unaffected. Mutation-check by temporarily removing the new `rows.Err()` call and confirming the new test fails.


---

## 475. [Yahoo fallback still fails most discovery queries despite bug 130's retry and backoff fix](#475-yahoo-fallback-still-fails-most-discovery-queries-despite-bug-130s-retry-and-backoff-fix)

**Table rationale cell (original):** **Fixed 2026-07-31.** Added `pkg/scraper/circuit_breaker.go`'s `sourceCircuitBreaker` — the same closed/open/half-open state machine as improvement #469's `domainCircuitBreaker` (`cmd/agent/circuit_breaker.go`), adapted to track one source (Yahoo) rather than a per-domain map, and kept as a package-level singleton (`yahooBreaker`) rather than a `FunnelEngine` field because a fresh `FunnelEngine` is constructed on every daemon discovery refresh (`cmd/agent/main.go`'s `runDaemonDiscoveryLoop`) — a per-instance breaker would have reset every refresh and never accumulated the sustained streak this bug is about. `discoverWithYahooHTML` now gates on `allow()` before any HTTP work, calls `recordFailure()` only on a genuine final failure (transport error, retryable 429/5xx, or body-read error) and only when `ctx.Err() == nil` so a caller-side cancellation is never miscounted as a Yahoo-side block, and calls `recordSuccess()` once a response body is read. Same threshold/cooldown constants as #469 (5 consecutive failures, 2-minute cooldown, single half-open probe). **Independently reviewed** by a second Claude session against the diff, live-verifying `go build`/`go vet`/`go test -race` itself rather than trusting this session's account; found the wiring, state machine, and singleton reasoning all correct, and one real gap — no test proved the transport-error and body-read-error branches actually fed the breaker (only the status-code branch was exercised), so a mutation deleting either `recordFailure()` call would have stayed green. **Fixed the same session**: added `TestDiscoverWithYahooHTML_TransportErrorTripsBreaker` (connection-refused via a closed `httptest.Server`) and `TestDiscoverWithYahooHTML_BodyReadErrorTripsBreaker` (hijacked connection closed mid-`Content-Length`), both **mutation-checked** — deleting the corresponding `recordFailure()` call makes each fail with the exact "0 consecutive failures" symptom, and both pass again reverted. Also added a state-machine test file (`circuit_breaker_test.go`, mirroring #469's own test file) and a behavioral test proving a sustained failure streak actually stops spending HTTP requests once open. `yahooBreaker` is a shared package-level var, so existing and new tests in `yahoo_test.go`/`funnel_test.go` that touch the Yahoo path reset it at the top rather than relying on file-alphabetical test-run ordering for isolation. `go build ./...`, `go vet ./...`, `go test ./...` (including `-race` on `pkg/scraper`) and `gofmt -l ./cmd ./pkg ./internal` all clean. Implemented directly by this session's orchestrator (Sonnet 5, this row's `standard`-tier Claude Code starting point) with an independent Claude review pass, per this run's instruction to prioritize Claude models. **This closes the last open item holding the Usability Gate open** — see the gate note above. Original report follows. Live `career_agent.log` review, 2026-07-31: 4,269 of 5,490 Yahoo fallback queries (77.8%) still ended in final failure after bug #130's 3-attempt retry budget was exhausted, holding steady at ~45-49 final failures/minute for the full 8+ hour run rather than clustering in a burst — read as a sustained block, not the transient blips #130's per-query backoff was built to smooth over

### 475. Yahoo fallback still fails most discovery queries despite bug 130's retry and backoff fix

**Evidence:** live `career_agent.log` review, 2026-07-31 — one continuous run spanning the entire log file (05:06 to 15:20, ~10h14m). `SERPAPI_API_KEY` is configured but its quota was exhausted early (`SerpApi error: Your account has run out of searches` logged 15 times as the funnel's per-run goroutines each independently hit the error before the shared `fallback` flag took effect — `pkg/scraper/funnel.go:115-121`), after which every role/ATS query for the rest of the run fell through to `discoverWithYahooHTML`. Every job discovered this session came from that path or the always-free RemoteOK/HackerNews/ATS-feed sources: 1,008 `Yahoo Fallback Discovered Live Job` lines match the log's 1,008 total `Discovered Live Job` lines exactly, so SerpApi contributed zero.

Of 5,490 `Fallback searching Yahoo HTML for:` attempts, **4,269 (77.8%) ended in `Yahoo fallback final failure`** — 4,192 `unexpected EOF` transport failures plus 77 final `status 500` failures — only after exhausting bug #130's 3-attempt retry budget (an additional 8,990 non-final transport-error retries and 106 retryable-500 retries logged along the way, all ultimately re-failing at roughly the same rate). This is not a burst: bucketing final failures by minute shows a steady ~45-49 per minute for the entire 8+ hour stretch from ~07:37 onward — the same rate at the start of the window as at the end.

**Root cause:** bug #130 (Done 2026-07-28) added a 3-attempt retry to `discoverWithYahooHTML` with a `time.Duration(attempt) * time.Second` backoff (1s, then 2s) for transport errors, 429s, and 5xx responses. That correctly handles a genuinely transient blip, but this evidence shows the dominant failure mode is not transient at that timescale — it holds at a near-constant ~78% rate for hours, which reads as Yahoo's bot detection blocking or resetting connections from this client rather than random network flakiness. A 1-3 second in-request backoff cannot out-wait a block that persists for the whole run. Two contributing factors visible in the code: `discoverWithYahooHTML` (`funnel.go:223`) sets only a `User-Agent` header — no `Accept`, `Accept-Language`, `Referer`, or persistent cookie jar — a recognizable non-browser fingerprint; and up to 5 of these requests run concurrently (`eg.SetLimit(5)`, `funnel.go:69`) hammering the same host continuously for hours whenever `SERPAPI_API_KEY` is absent or exhausted, which is exactly the condition this run was in.

**Fix direction:** keep #130's per-query retry for genuine transient errors, but add a higher-level circuit breaker/cooldown for the Yahoo source itself so a sustained failure streak stops burning the 5-way concurrent request budget and the 3x retry cost on a source that is currently not viable, instead of repeating the same doomed request pattern on every single query for hours. Improvement #469 already built exactly this shape (`domainCircuitBreaker` in `cmd/agent/circuit_breaker.go`, a closed/open/half-open state machine with a cooldown and a bounded half-open probe) for `checkJobAlive`/`fetchJobPage` — reuse or adapt it here rather than inventing a second mechanism. Separately worth trying, but not a substitute for the breaker: a more realistic header set (`Accept`, `Accept-Language`) and a shared `http.CookieJar` across queries in one run, in case part of the block is fingerprint-based rather than purely rate/volume-based — cheap to test, but do not assume it fixes the rate without live re-verification, since Yahoo's blocking behavior is opaque from here. Add tests proving the breaker opens after a consecutive-failure threshold, defers further Yahoo attempts during cooldown without spending the per-query retry budget, and half-open-probes and recovers once Yahoo starts responding again — mirroring #469's own state-machine tests.


---

## 467. [Playwright target closure aborts an application attempt without bounded browser recovery](#467-playwright-target-closure-aborts-an-application-attempt-without-bounded-browser-recovery)

**Table rationale cell (original):** **Fixed 2026-07-31.** `AttemptSubmit` now recreates the browser context once (capped) on a target-closed error, redoing only navigation and the fill attempt — not the already-paid `generateDocs()` cost. Mutation-checked, independently reviewed. See row above for the full account

### 467. Playwright target closure aborts an application attempt without bounded browser recovery

**Resolved 2026-07-31.** `pkg/submitter/browser.go`'s `AttemptSubmit` now recreates the browser context once when a submit/fill action fails with Playwright's target-closed wording. The top-of-function context/page/navigation setup was extracted into a reusable `newSubmitPage(browser, applyURL)` helper (used both for the initial page and for recovery), a new `isTargetClosedErr` classifies "target closed" / "Target page, context or browser has been closed" / "browser has been closed" case-insensitively, and a `maxTargetRecoveryAttempts = 1` cap governs a new `targetRecoveries` counter inside the existing three-attempt retry loop. On a target-closed `execErr`, the old page/session are closed, a fresh page/session are created and re-navigated, the dead-redirect/dead-job/captcha checks are re-run against the fresh page, and the fill attempt is redone from scratch (`initialAttemptComplete = false`, `attempt--` so the recreation does not consume one of the three real validation-retry attempts) — critically without re-calling `generateDocs()`, which already ran earlier in the function and is the expensive part of an attempt (tailored resume/cover-letter generation). A second target-closed failure exhausts the cap and surfaces normally.

**Verified with two new tests** in `pkg/submitter/browser_test.go`, both routed through the LinkedIn dispatch branch (the simplest ATS path to control) via the existing `MockBrowser`/`MockContext`/`MockPage`/`MockLocator` doubles: `TestAttemptSubmit_RecoversFromTargetClosedOnce` fails the first page's click with a target-closed error and succeeds the second (recreated) page's, asserting the browser context and page were each created exactly twice, `generateDocs` was called exactly once (not repeated), the final error is the *second* page's own distinct failure (proving the recovered page's attempt genuinely ran rather than the first error leaking through), and that the crashed page/context were actually `Close()`d rather than merely abandoned. `TestAttemptSubmit_TargetClosedRecoveryIsBounded` fails both pages with target-closed errors and asserts exactly one recovery occurs (two context creations total, never more) and the final error is the target-closed error itself. Plus `TestIsTargetClosedErr` unit-tests the classifier directly. **Mutation-checked:** temporarily neutralizing the recovery condition (`if false && ...`) reproduces the exact symptom the fix exists to prevent — `TestAttemptSubmit_RecoversFromTargetClosedOnce` fails with `browser context creations = 1, want 2` and the raw target-closed error leaking through as the final result — and passes again once reverted.

**Independently reviewed** by a second Claude session against the diff: no correctness bugs, resource leaks, or infinite-loop risk found. The apparent double-`Close()` on the pre-recovery page/session (the function's original `defer`s, registered before reassignment, still fire against the *old* objects at return, since Go binds a deferred method call's receiver at defer-time) was checked and confirmed harmless — `playwright-go`'s `BrowserContext.Close()` has an `IsClosed()` guard, `Page.Close()` swallows an already-closed target into `nil`, and the network guard's proxy `Close()` is `sync.Once`-wrapped. The review's one nit (neither test originally asserted the crashed context's `Close()` was actually invoked) was addressed by adding a `closed` field to `MockPage` and a `contextCloseCalls` counter to the recovery test.

**Deliberately out of scope**, noted rather than folded in: the cached-form-mapping fast path (`storage.GetFormMapping` hit, `dynErr := handleDynamic(cachedTarget, ...)`), the emailed-security-code resubmit click, and the Vision submission paths (`pkg/submitter/vision.go`) do not go through this recovery — a target-closed error there still invalidates the cache/returns directly as before. The bug's own text only asked for the reported auto-submit-click failure mode, and every job already gets its own fresh context via a separate `AttemptSubmit` call, so a dead context never actually propagated to a *later* job even before this fix — filed as `improvements.md` follow-up work rather than expanded here. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean. Implemented directly by this session's orchestrator (Sonnet 5, this row's `standard`-tier Claude Code starting point) per this run's instruction to prioritize Claude models, with an independent Claude review pass rather than a separate implementation delegate. Original report follows.

**Found 2026-07-30** in the live service log: an auto-submit attempt failed with `playwright: target closed`, and the system journal also recorded a Chromium headless crash. The current path records the job failure but does not distinguish a closed target from a normal submission failure or recreate the browser context before continuing. A single browser crash can therefore discard work and leave later attempts dependent on a damaged target.

**Fix direction:** detect target/page closure explicitly, recreate the page or browser context once, retry only the idempotent browser step, and cap recovery attempts. Preserve the original failure reason and verify that the worker continues to the next job after a simulated target closure.


---

## 466. [Retryable queue rows are reset to DISCOVERED and immediately selected again](#466-retryable-queue-rows-are-reset-to-discovered-and-immediately-selected-again)

**Table rationale cell (original):** **Fixed 2026-07-31.** `job_funnel` gained `retry_count`/`next_eligible_at` columns (idempotent migration, `migrateJobFunnelRetry`, matching the file's existing `migrateJobFunnel*` pattern). A new `storage.UpdateFunnelStatusRetryable(url)` increments `retry_count` and, while under `MaxRetryAttempts` (5), returns the row to `DISCOVERED` with `next_eligible_at` pushed out by exponential backoff (2/4/8/16 min); at `MaxRetryAttempts` it instead moves the row to a new terminal status, `RETRY_EXHAUSTED` (wired into `mergeStatusRank`'s completeness-checked switch per its own mandatory-for-new-statuses warning comment). `GetDiscoveredJobs` now excludes rows whose `next_eligible_at` is still in the future. All 5 retryable call sites in `cmd/agent/pipeline.go` (preflight check, job-page fetch, RAG embedding/retrieval, post-score freshness re-check) now call `UpdateFunnelStatusRetryable` instead of resetting straight to `DISCOVERED`. **A same-session independent Claude review pass (mirroring #451/#452/#453's practice) found two real defects before anyone else could hit them:** the fix's own doc comment overclaimed that `RETRY_EXHAUSTED` "stays visible for manual investigation" when no dashboard tile or card actually surfaces it (corrected the comment; the dashboard gap is not new work — it is already tracked as improvements.md's pending **#468**, whose own detail section already says "add dashboard counts for ... retry-exhausted rows. This is separate from bug #466 because it improves ... operator visibility after the retry scheduler is corrected" — that scheduler correction is this fix, so #468 is now unblocked rather than newly filed); and `RequeueByURLPattern` (`pkg/storage/manager.go`, used by `cmd/requeue` for exactly this "a fix shipped, give stale rows a fresh try" case) reset `status` back to `DISCOVERED` without resetting `retry_count`/`next_eligible_at`, so manually requeuing a `RETRY_EXHAUSTED` row inherited a stale, already-maxed retry count and would re-exhaust after a single subsequent attempt instead of getting a genuine fresh budget — fixed in the same commit by having the requeue also reset both columns. **Mutation-checked** three times: reverting the `GetDiscoveredJobs` eligibility filter reproduces the exact repeated-row starvation the bug reported; reverting the `MaxRetryAttempts` cap check keeps a row retrying forever instead of exhausting; reverting the `RequeueByURLPattern` reset reproduces the review's stale-budget defect. 5 new tests across `pkg/storage/manager_test.go` (backoff growth and exhaustion, `GetDiscoveredJobs` skip-then-resume-after-elapsed-backoff, the migration's pre-existing-row backfill, and the requeue reset), plus the existing `mergeStatusRank` completeness test extended with `RETRY_EXHAUSTED`. **Deliberate scope note the review also raised:** `retry_count` is one lifetime counter shared across all four retryable failure types (preflight, fetch, RAG, freshness re-check) rather than per-failure-type, so a row that already backed off twice on one cause reaches `RETRY_EXHAUSTED` sooner if a second, unrelated cause starts firing — judged an acceptable simplification, not a defect, since the row is never permanently lost (`cmd/requeue` grants a fresh budget) and per-cause counters would meaningfully complicate the schema for a marginal benefit. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean. Implemented directly by this session's orchestrator (Sonnet 5, this row's `standard`-tier Claude Code starting point exactly) rather than a separate delegate, per this run's instruction to prioritize Claude models. Original report follows. Found 2026-07-30 by reviewing the log of the continuously running daemon: cycles 424-429 showed the same 15 queue rows repeating in the same order. The retryable preflight and weak-page paths in `cmd/agent/pipeline.go` called `UpdateFunnelStatus(job.URL, "DISCOVERED")`; `pkg/storage/manager.go` then returned every `DISCOVERED` row to the next queue cycle with no attempt count or cooldown, so a transient timeout was retried immediately and could monopolize the worker while newer rows waited indefinitely

### 466. Retryable queue rows are reset to DISCOVERED and immediately selected again

**Found 2026-07-30** by reviewing the log of the continuously running daemon. Cycles 424–429 showed the same 15 queue rows repeating in the same order. The retryable preflight and weak-page paths in `cmd/agent/pipeline.go` call `UpdateFunnelStatus(job.URL, "DISCOVERED")`; `pkg/storage/manager.go` then returns every `DISCOVERED` row to the next queue cycle. With the new immediate-continuation scheduler, a transient timeout is therefore retried immediately and can monopolize the worker while newer rows wait indefinitely.

**Fix direction:** persist attempt count and next-eligible time (or a dedicated deferred status), apply exponential backoff with a maximum retry budget, and move exhausted rows to a terminal/manual-review status. Tests must prove a retryable failure does not reappear in the immediately following cycle and that later queue rows make progress.


---

## 465. [`internal/backlog`'s Pending-cell floor was a historical snapshot, and ordinary backlog progress tripped it](#465-internalbacklogs-pending-cell-floor-was-a-historical-snapshot-and-ordinary-backlog-progress-tripped-it)

**Table rationale cell (original):** **Fixed 2026-07-30.** Found while closing improvement #460: marking that row Done (Pending → Done) dropped `improvements.md`'s Pending-row count by one, which took `TestPendingBacklogRowsNameRealModels`'s "guard the guard" floor from exactly 20 checked cells to 18 and turned `go test ./...` red — not because any model ID was wrong, but because the floor (`checked < 20`, itself inherited from an even older "44" comment) was a hardcoded historical snapshot of how many Pending rows existed the day #457 shipped, not a property of the parser. `AGENTS.md` is right that a documentation edit turning this suite red is normally "the check working" — but there was nothing wrong in the document here; the bound itself was the defect, silently assuming this backlog's Pending count would never shrink. **Fixed** by replacing the constant with an independently-derived floor: a plain `strings.Count(file, " | Pending")` substring scan per backlog file, computed without going through `itemRowRE`/`splitRow`/`backlogHeaderRE` (the exact machinery `checked` depends on), so a genuine parser regression (header rename, a broken regex) still fails loudly — `checked` collapses toward 0 while the independent count does not — but ordinary backlog closures no longer do. **Mutation-checked**: forcibly zeroing the `pending` flag inside `backlogModelCells` reproduces `checked == 0` against an unchanged independent count of 9, and the test fails with an actionable message; reverting the mutation passes again. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean afterward. **Worth a standing note for this backlog: a "guard the guard" floor is itself backlog content, and needs the same "this is a live measurement, not a permanent fact" discipline as everything else here** — the same lesson #455 already taught about model-ID columns, just one level up, in the test that was supposed to prevent exactly that class of drift

---

## 461. [The runtime binary `career_agent_bin` is tracked in git and grows the repo on every rebuild](#461-the-runtime-binary-career_agent_bin-is-tracked-in-git-and-grows-the-repo-on-every-rebuild)

**Table rationale cell (original):** **Fixed 2026-07-30.** `git rm --cached career_agent_bin` (keeps the local file, only stops tracking it going forward — deliberately did not rewrite history to shrink the already-published blob, per the row's own scope note); `.gitignore` gained `career_agent_bin` and `dashboard_bin` entries. **Verified live:** rebuilt both binaries locally with `go build -o career_agent_bin ./cmd/agent` and `go build -o dashboard_bin ./cmd/dashboard`; `git status` shows neither as untracked or modified afterward. `go build ./...`, `go vet ./...` and `go test ./...` all clean. Original report follows. Found 2026-07-30 while working improvement #459, as stray working-tree state left over from a prior session. `git log --oneline -- career_agent_bin` shows it was committed once, 2026-07-28 (`70975c7`), and never since — but it was still present as a 47MB *modified* tracked file at the start of this session (`git diff --stat` showed `Bin 47248192 -> 49795394 bytes`), meaning a rebuild had touched it and, absent this catch, would have committed a 49MB diff on top of the existing 47MB blob. `.gitignore` already has `/career-agent`, `/agent` and `/dashboard` entries for exactly this class of build artifact, but none match `career_agent_bin` — the actual name bug #445's `serveAgentStart` execs (`exec.Command("./career_agent_bin", ...)`) and bug #449's `pgrep -f`/`pkill -f` identify the agent by. `dashboard_bin` (the dashboard's own equivalent local build) is not gitignored either, though it happened not to be tracked yet this session. Value 3: repo bloat that compounds on every commit an untracked build artifact accidentally rides along on, same failure shape bug #436 hit from the opposite direction (an artifact needed but gitignored) — here it is an artifact unneeded but tracked. Effort 1: `git rm --cached career_agent_bin`, add `career_agent_bin`/`dashboard_bin` to `.gitignore`. Deliberately does not include rewriting git history to shrink the existing blob out of past commits — that is a destructive, repo-wide operation and a separate decision from stopping the bleeding

### 461. The runtime binary `career_agent_bin` is tracked in git and grows the repo on every rebuild

**Found 2026-07-30** while working improvement #459. `git status` at the start of the session showed `career_agent_bin` as a *modified* tracked file, 47MB growing to 49MB:

```
$ git diff --stat -- career_agent_bin
 career_agent_bin | Bin 47248192 -> 49795394 bytes
```

`git log --oneline -- career_agent_bin` shows exactly one commit, `70975c7` (2026-07-28, "feat: add SkipScoring and Playwright Stealth"), so it was added by accident rather than on purpose — nothing in that commit's subject is about a runtime binary. `.gitignore` has `/career-agent`, `/agent` and `/dashboard` for this exact class of artifact, but the binary bugs #445 and #449 actually name (`exec.Command("./career_agent_bin", ...)`, `pgrep -f career_agent_bin`) does not match any of them. Left alone, the next session to rebuild it locally and run a broad `git add` would have committed a fresh multi-megabyte diff on top of the existing blob. Discarded the local modification this session (`git checkout -- career_agent_bin`, safe — it is reproducible build output) so the working tree stayed clean, but the already-committed 47MB blob and the missing `.gitignore` entry are both still there for the next session. `dashboard_bin`, the dashboard's own equivalent local build, was present untracked (also not gitignored) and was deleted as harmless debris rather than fixed here, since it was never committed.

**Fix direction:** `git rm --cached career_agent_bin` (keeps the local file, only stops tracking it) plus `career_agent_bin` and `dashboard_bin` entries in `.gitignore`. Shrinking the blob back out of already-published history (`git filter-repo` or similar) is a separate, destructive decision this row deliberately does not include — flag it to the user rather than doing it autonomously.


---

## 453. [`GetQueuePlan` scans `discovered_at` into a non-nullable `time.Time` while its sibling uses `sql.NullTime`](#453-getqueueplan-scans-discovered_at-into-a-non-nullable-timetime-while-its-sibling-uses-sqlnulltime)

**Table rationale cell (original):** **Fixed 2026-07-30.** `GetQueuePlan` (`pkg/storage/queue_plan.go`) now scans `discovered_at` into a `sql.NullTime`, falls back to `time.Now()` when the value is null, and only `continue`s past a row on a genuine scan error instead of returning it for the whole query — matching `GetDiscoveredJobs`'s existing pattern exactly. Added a new subtest in `queue_plan_test.go` that inserts a `job_funnel` row with `discovered_at` explicitly `NULL` alongside a normal row and asserts the plan still returns both candidates, with the null one falling back to `time.Now()` rather than the zero value. **Mutation-checked**: reverting the fix alone (via `git stash`) makes the new subtest fail with `sql: Scan error ... storing driver.Value type <nil> into type *time.Time`, confirming the test exercises the real defect. Implemented by a delegated Sonnet 5 subagent per this row's Claude model column, diff reviewed and the full `go build`/`go vet`/`go test` loop re-run by the orchestrating session before commit. Original report: found 2026-07-30 by the review agent running alongside #446. `job_funnel.discovered_at` is declared `DATETIME` with no `NOT NULL` (`pkg/storage/manager.go:75`). `GetDiscoveredJobs` reads it into a `sql.NullTime` and falls back to `time.Now()` when it is null (`manager.go:1206-1215`), and it `continue`s past a bad row. `GetQueuePlan` read the same column into a bare `time.Time` (`pkg/storage/queue_plan.go:64`) and `return`ed the scan error (`:67`), so one null row would have discarded the entire plan and `cmd/requeue` would have printed a driver error instead of a queue. Was not reachable in practice — `AddToFunnel` always sets the column — which is why this was Minor rather than higher

### 453. `GetQueuePlan` scans `discovered_at` into a non-nullable `time.Time` while its sibling uses `sql.NullTime`

**Found 2026-07-30** by the Claude review agent running alongside #446.

`job_funnel.discovered_at` is declared without a `NOT NULL` constraint (`pkg/storage/manager.go:75`):

```sql
discovered_at DATETIME,
```

Two functions read that column and disagree about whether it can be null. `GetDiscoveredJobs` assumes it can:

```go
var discoveredAt sql.NullTime                       // manager.go:1206
...
if discoveredAt.Valid { j.DiscoveredAt = discoveredAt.Time } else { j.DiscoveredAt = time.Now() }
```

and it `continue`s past a row that fails to scan. `GetQueuePlan` assumes it cannot:

```go
var discoveredAt time.Time                          // queue_plan.go:64
if err := rows.Scan(...); err != nil { return nil, err }   // queue_plan.go:67
```

**Not reachable today**, because `AddToFunnel` always writes the column — which is the whole reason this is Minor. What makes it worth filing is the asymmetry: the function that is *less* tolerant of a null is also the one with the *larger* blast radius, since a single bad row discards the whole plan and `cmd/requeue` surfaces a raw driver error instead of a queue. `queue_plan_test.go` never inserts a null `discovered_at`, so nothing would catch a future writer path that omits it.

**Fix:** match the sibling — `sql.NullTime` with an explicit fallback. One line and a test row.


---

## 452. [`serveMetrics` swallows all nine of its query errors and answers 200 with zeros](#452-servemetrics-swallows-all-nine-of-its-query-errors-and-answers-200-with-zeros)

**Table rationale cell (original):** **Fixed 2026-07-30.** Every `g.Go` closure that used to log its error and unconditionally `return nil` now returns a wrapped error instead — except where the original code already special-cased `sql.ErrNoRows` as a legitimate empty state, which still renders as zero exactly as before. `serveMetrics` checks `g.Wait()`'s result and answers `http.StatusInternalServerError` on a genuine failure instead of encoding a 200 with whatever zero/stale values the failed scans left behind. **Chose whole-response failure over per-field degrade:** the row's own "likely shape" suggested marking individual fields unavailable so the UI could render "—" per tile; a simpler whole-response 500 was implemented instead, since it already fully separates "empty" from "failed" (the row's actual ask) without touching the frontend, and `fetchMetrics` in `App.tsx` already treats a non-2xx response as "keep the last-good data" via its existing `res.ok` check, so nothing regresses on failure. Two new tests: a genuine failure (dropped table, "no such table" from `modernc.org/sqlite`) returns 500 with a non-JSON body, and an empty-but-present table still returns 200 with legitimate zeros. **Mutation-checked:** reverting the fix alone reproduces the exact bug — a 200 response with a fully-decodable all-zero `Metrics` body. **Independently reviewed by a second Claude session**, which confirmed all nine closures are handled correctly, found no frontend regression, and flagged two pre-existing (not introduced) follow-ups now filed separately: `rows.Err()` is never checked after the by-source/by-variant scan loops, and `App.tsx` has no visible error state for a failed metrics poll. Original report: found 2026-07-30 by the review agent running alongside #446. All nine `g.Go` closures in `serveMetrics` log their error and then unconditionally `return nil` (`cmd/dashboard/main.go:419`, `:433`, `:451`, `:468`, `:491`, `:518`, `:543`, `:573`, `:607`), and the group's result is explicitly discarded at `:628` with `_ = g.Wait()`. The handler therefore always responds `200 OK` carrying whatever zero values the failed scans left behind, and the single basic-counts query drives all eight top-line tiles at once. A user watching during a failure sees a confident "nothing has happened yet", not an error. **This is the mechanism #446's report described** ("its queries swallow errors into a log line and return zeros") and it was never filed on its own; #446 turned out to be about the DSN, so it is still open. Effort 2 rather than 1 because several of these are `QueryRow` calls where `sql.ErrNoRows` is a legitimate empty state that must keep rendering as zero — distinguishing "no rows" from "query failed" is the actual work

### 452. `serveMetrics` swallows all nine of its query errors and answers 200 with zeros

**Fixed 2026-07-30.** See the Ranked Backlog row above for the fix and its verification. Original report follows.

**Found 2026-07-30** by the Claude review agent running alongside #446.

Every one of the nine `g.Go` closures in `serveMetrics` has the same shape:

```go
if err != nil {
    log.Printf("Failed to query basic counts: %v", err)
}
return nil
```

at `cmd/dashboard/main.go:419`, `:433`, `:451`, `:468`, `:491`, `:518`, `:543`, `:573` and `:607`. The group's own result is then explicitly discarded — `_ = g.Wait()` at `:628` — and the handler encodes whatever is in the struct and returns `200 OK`.

So a failed query is indistinguishable from an empty database. The first closure alone populates all eight top-line tiles, meaning a single error renders the entire funnel as zero: a confident "nothing has happened yet" rather than "metrics unavailable". The only trace is a line in the dashboard's stderr, which nobody watching a web page is reading.

**This is the mechanism #446's report described** — *"its queries swallow errors into a log line and return zeros, so contention with a writing agent shows up as a briefly wrong dashboard rather than an error"* — but it was never filed as its own row. #446 turned out to be about the DSN, and fixing the DSN does not touch this. It is worth stating plainly: **the sentence that described this defect was sitting inside another bug's report, being read as background rather than as a finding.**

**Fix direction:** distinguish "no rows" from "query failed". Several of these are `QueryRow` calls where `sql.ErrNoRows` is a legitimate empty state that must keep rendering as zero, so this is not a blanket `return err` — that would turn a fresh install's empty database into a 500. The likely shape is per-field: let each closure record that its field is unavailable, and have the response carry that so the UI can render "—" instead of "0".


---

## 451. [Two summary tiles caption a two-status count with the reason for only one of them](#451-two-summary-tiles-caption-a-two-status-count-with-the-reason-for-only-one-of-them)

**Table rationale cell (original):** **Fixed 2026-07-30.** The basic-counts query now also returns each pair's members separately (`FailedScore`/`FailedSubmit`, `ManualRequiredOnly`/`AwaitingReview`), served as four new JSON fields alongside the existing pair totals. `App.tsx` captions each tile with a new `explainPair` helper that names whichever status(es) in the pair actually have a nonzero count, joined with `; ` when both contributed, instead of a hardcoded literal. Six new table-driven Go tests cover each status alone and both together for both pairs; mutation-checked by reverting `main.go` alone, which fails the test file to even compile. **A same-day independent review pass (mirroring this backlog's own established practice — see #452/#453/#449) caught two real defects in the first cut before anyone else could hit them:** the join separator was originally a bare `·`, which screen readers at default punctuation levels do not announce, producing a run-on sentence between two full stops-free clauses; and when both counts were genuinely zero the caption rendered empty, the one tile in the grid to go bare at 0 while every sibling (Skipped, Blocked, Invalid) always shows its static reason. Both fixed in the same commit: the separator is now `; `, and the zero-zero case falls back to naming both statuses instead of neither. **Verified live** against the real `applications.db`: the Failed tile's real split is 22 `FAILED_SCORE` + 35 `FAILED_SUBMIT` = 57, so the old hardcoded caption ("reached the application form but failed to submit") was flatly wrong for 22 of those 57 jobs. The UI dist bundle was rebuilt (`npm run build`) so the embedded assets reflect the new caption logic, and `TestUIDistEmbed_RendersEveryServedMetricsField` was extended with the four new field names. Original report: found 2026-07-30 by the review agent running alongside #446, and confirmed by reading all three sites. The basic-counts query aggregates two statuses per tile (`cmd/dashboard/main.go:394` sums `FAILED_SCORE` **and** `FAILED_SUBMIT`; `:395` sums `MANUAL_REQUIRED` **and** `AWAITING_REVIEW`), but the tile's caption is hardcoded to one member of each pair — `App.tsx:211` renders `explain('FAILED_SUBMIT')` and `:216` renders `explain('MANUAL_REQUIRED')`. `statusReason` (`main.go:132-151`) gives each member a genuinely different meaning: "Failed to score the job against your profile" versus "Reached the application form but failed to submit", and "ATS requires an account" versus "Filled by Copilot — awaiting your review and submit"

### 451. Two summary tiles caption a two-status count with the reason for only one of them

**Found 2026-07-30** by the Claude review agent running alongside #446, and re-confirmed here by reading all three sites rather than trusting the report.

Two of the eight summary tiles count a pair of statuses but explain only one of them. The query aggregates:

```go
COALESCE(SUM(CASE WHEN status IN ('FAILED_SCORE', 'FAILED_SUBMIT') THEN 1 ELSE 0 END), 0),      // main.go:394
COALESCE(SUM(CASE WHEN status IN ('MANUAL_REQUIRED', 'AWAITING_REVIEW') THEN 1 ELSE 0 END), 0), // main.go:395
```

while the tiles caption:

```tsx
<span className="card-reason">{explain('FAILED_SUBMIT')}</span>    // App.tsx:211
<span className="card-reason">{explain('MANUAL_REQUIRED')}</span>  // App.tsx:216
```

`statusReason` (`main.go:132-151`) gives the members of each pair meanings that are not interchangeable:

| status | reason rendered |
| --- | --- |
| `FAILED_SCORE` | Failed to score the job against your profile |
| `FAILED_SUBMIT` | Reached the application form but failed to submit |
| `MANUAL_REQUIRED` | ATS requires an account — apply manually with the saved tailored docs |
| `AWAITING_REVIEW` | Filled by Copilot — awaiting your review and submit |

**The failure is that the number is correct and the sentence under it is wrong**, which is worse than having no sentence: a run whose failures are all `FAILED_SCORE` reports having reached a form it never loaded, and a Manual Queue holding only copilot-filled jobs tells the user to go create an ATS account when the real ask is a single click on work already done.

Note that `App.tsx` already reasons about exactly this distinction correctly, at `:264-266`, where it calls the two "completely different asks" — but only for the separate "Awaiting You" detail card, not for this tile. The knowledge was in the file; the tile just did not use it.

**Fix direction:** caption from the data rather than from a literal — either render the reason for whichever status actually dominates the bucket (the API would need to return the split), or caption both. There are no frontend tests under `cmd/dashboard/ui` at all, which is why nothing caught this; whoever takes it should consider whether adding the first one is in scope.


---

## 449. [`pgrep -f career_agent_bin` matches any process whose command line merely contains that string](#449-pgrep--f-career_agent_bin-matches-any-process-whose-command-line-merely-contains-that-string)

**Table rationale cell (original):** **Fixed 2026-07-30, thirteenth session.** Replaced `pgrep -f`/`pkill -f career_agent_bin` with `agentPIDAt` (`cmd/dashboard/main.go`), which determines whether the agent is running by attempting a non-blocking `flock` on `applications/career_agent.lock` (bug #414's existing single-instance lock) rather than matching a substring of any process's command line. `cmd/agent/main.go`'s lock acquisition (`acquireSingleInstanceLock`) now writes its own PID into that same file once it holds the lock, so the dashboard can read back a real PID; `serveAgentStop` signals that specific PID with `SIGTERM` instead of `pkill -f`, so it can no longer hit an unrelated process. **Live-verified against the real dashboard binary**, not only tests: with no agent running and a decoy process (`career_agent_bin_decoy`) alive, `/api/agent/status` correctly returned `{"running":false}` (the exact false positive the bug reported no longer fires); with a real flock held and its PID written to the lock file, status correctly returned `{"running":true}`; calling `/api/agent/stop` with a second decoy also running signaled only the real lock-holder's PID — the decoy survived untouched, confirming the "worse half" (killing unrelated processes) is closed too. 8 new tests across `cmd/dashboard/agent_lock_test.go` and `cmd/agent/single_instance_lock_test.go` cover lock-free, lock-held-with-PID, lock-held-with-unparsable-content, the status check releasing its own probe lock, the decoy-process repro, PID-write correctness, contention, and truncation of a previous longer PID; the truncation test is mutation-checked (removing the `Truncate(0)` call makes it fail with the exact glued-digit output the row predicted). `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l` on all four touched files are all clean. Original report follows. **2026-07-30 re-verified (twelfth session, groom pass after #461):** unchanged as a defect; line numbers drifted a third time to `cmd/dashboard/main.go:691`, `:721`, `:727` (from `:651`/`:681`/`:687`) because improvement #459's `rows.Err()` fix added code above them in the same file. Original report follows. Found 2026-07-30 during #445's live verification, by noticing `/api/agent/status` returned `{"running":true}` on a host with **no** agent process. All three agent handlers identify the agent with `pgrep -f`/`pkill -f career_agent_bin`, and `-f` matches the whole command line of every process, not the executable. So `go build -o career_agent_bin ./cmd/agent`, `tail -f career_agent_bin.log`, or an editor with the file open all read as "the agent is running". Two consequences: the Start button silently returns `already_running` and does nothing, and — the worse half — Stop runs `pkill -f` against the same pattern and **kills those unrelated processes**. Reproduced live with a decoy process. Value 5 and Minor rather than Major because it needs a coincidental command line, but note the escalation argument: the failure mode is killing a process the user did not ask to kill

### 449. `pgrep -f career_agent_bin` matches any process whose command line merely contains that string

**Resolved 2026-07-30.** See the row above for the shipped fix and its live verification. The original report follows.

**Found 2026-07-30** during bug #445's live verification, and found the way this backlog keeps recording as the reliable way: by looking at a value that was not the thing being tested. While confirming the new same-origin guard, the ungated read-only endpoint answered

```
$ curl http://127.0.0.1:8099/api/agent/status
{"running":true}
```

on a host with **no** agent process running at all. `ps -eo args` confirmed there was none. What `pgrep` had matched was the verification shell's own command line, which contained the literal string `career_agent_bin` because the test script mentioned it.

All three agent handlers identify the agent the same way:

```go
// cmd/dashboard/main.go:644 (serveAgentStart) and :672 (serveAgentStatus)
err := exec.Command("pgrep", "-f", "career_agent_bin").Run()
// cmd/dashboard/main.go:666 (serveAgentStop)
_ = exec.Command("pkill", "-f", "career_agent_bin").Run()
```

(Line numbers as of the 2026-07-30 sixth-session groom pass; they drift with every commit that touches earlier code in the file, most recently bug #451's.)

`-f` matches against the **full command line** of every process, not the executable name. Any process whose arguments happen to contain the substring counts as "the agent". Realistic triggers, none of them contrived: `go build -o career_agent_bin ./cmd/agent`, `tail -f career_agent_bin.log`, `grep career_agent_bin ...`, or an editor holding the file open.

**Two consequences, and the second is the serious one.**

1. `serveAgentStatus` reports `running: true` when nothing is running, and `serveAgentStart` then returns `{"status": "already_running"}` and does nothing. The Start button silently no-ops, and the dashboard's activity indicator lies.
2. `serveAgentStop` runs `pkill -f career_agent_bin` against that same over-broad pattern, so pressing Stop **kills the unrelated matching processes**. A user who happens to be tailing the log loses the tail; a user mid-`go build` loses the build.

Reproduced live 2026-07-30 with a decoy process carrying the string in its command line and no agent running: `pgrep -f career_agent_bin` matched.

**Filed Minor rather than Major** because it needs a coincidental command line rather than firing on its own. The escalation argument is worth recording anyway: the failure mode of half of it is terminating a process the user never asked to terminate, which is not self-correcting the way a wrong dashboard number is.

**Fix direction:** stop identifying the process by a substring of its command line. Write a PID file when the agent starts and check that the PID is alive and is actually the agent, which also fixes the `go run` orphan trap documented at the top of this file. A narrower interim fix is `pgrep -x`/an anchored pattern, but that still cannot distinguish the real agent from anything else invoked under the same name, and it does nothing about `pkill`'s blast radius. Note this interacts with bug #414's single-instance lock, which already has a notion of "the agent is running" that this code does not consult.


---

## 447. [The dashboard UI silently swallows failed start/stop clicks, and slow polls can render stale metrics](#447-the-dashboard-ui-silently-swallows-failed-startstop-clicks-and-slow-polls-can-render-stale-metrics)

**Table rationale cell (original):** **Fixed 2026-07-30.** `handleStart`/`handleStop` now wrap their `fetch` in `try/catch`, check `res.ok`, and set a new `actionError` state rendered as a `role="alert"` line under the controls on any failure — a rejected promise or non-2xx no longer looks identical to success. The poll loop gained a `pollSeq` ref: each `fetchMetrics`/`checkAgent` call is tagged with the sequence number current when it was sent, and only applies its result if that number is still current, so a slow response can no longer land after and overwrite a faster later one. **Verified live**, not just by type-check: ran a second dashboard instance on `127.0.0.1:8099` against the real `applications.db` and hit `/api/agent/start` directly — it returned a real `500` (the scratch binary's working directory could not resolve `career_agent_bin`), the exact failure shape the bug describes, and confirmed the built bundle's `handleStart` now checks `res.ok` and sets the "Failed to start agent — check the log" message rather than swallowing it. The live production dashboard on `:8080` was left untouched throughout. No JS test runner exists in `cmd/dashboard/ui` (only `tsc`/`vite`/`oxlint`); added none for a 20-line UI fix, consistent with the rest of the file having no coverage — `go build ./...`, `go vet ./...`, `go test ./...`, `tsc -b`, and `oxlint src` all clean. Original report follows. Found 2026-07-30 while restoring the UI for #437. Two independent defects in the same 20 lines of `App.tsx`. (1) `handleStart`/`handleStop` `await fetch(...)` with no `try/catch`, unlike `fetchMetrics`/`checkAgent` which both have one; a rejected fetch skips the follow-up `checkAgent()` and surfaces only as an unhandled rejection in the console, so a failed click is indistinguishable from a successful one. (2) The 2-second `setInterval` has no `AbortController` and no in-flight guard, and `serveMetrics` fans out eight independent SQL queries with no latency bound, so a slow poll can resolve after a faster later one and overwrite fresh state with stale numbers. Self-correcting on the next tick, hence Minor

### 447. The dashboard UI silently swallows failed start/stop clicks, and slow polls can render stale metrics

**Found 2026-07-30** while restoring the conversion tables for #437. Two independent defects within about twenty lines of `cmd/dashboard/ui/src/App.tsx`, filed together because they would be fixed in one pass.

**1. A failed start or stop click looks exactly like a successful one.** `fetchMetrics` and `checkAgent` each wrap their `fetch` in `try/catch`. `handleStart` and `handleStop` do not:

```ts
const handleStart = async () => {
  await fetch('/api/agent/start', { method: 'POST' });
  checkAgent();
};
```

If the POST rejects, the `await` throws, `checkAgent()` never runs, and the only trace is an unhandled promise rejection in the browser console. The button does not change, no error appears, and the user has no way to tell whether the agent started short of reading the log. Since the adjacent functions already establish the pattern, this reads as an omission rather than a decision.

**2. A slow poll can overwrite fresher data.** The 2-second `setInterval` has no `AbortController` and no in-flight guard, and `serveMetrics` fans eight independent SQL queries out through an `errgroup` with no latency bound. If one poll's request is slow — for instance while it is losing a lock race, which bug #446 makes more likely — a later, faster request can resolve first, and then the older response's `setMetrics(data)` lands on top of it. The UI shows stale numbers until the next tick corrects them.

Self-correcting within one poll in both halves of the race, which is why this is Minor rather than Major. Fix with a `try/catch` plus a visible error state on the two handlers, and a request sequence number or `AbortController` on the poll.


---

## 446. [The dashboard's own database connection still uses the pragma syntax bug #416 was closed for fixing](#446-the-dashboards-own-database-connection-still-uses-the-pragma-syntax-bug-416-was-closed-for-fixing)

**Table rationale cell (original):** **Fixed 2026-07-30.** `pkg/storage/dsn.go` now holds `DSN(path)` and `DefaultDatabasePath` as the single source of truth; `pkg/storage/manager.go` and `cmd/dashboard/main.go` both go through it, and the dashboard keeps the result in a package-level `dashboardDSN` that `TestDashboardDSNMatchesStorage` pins to the shared builder, so the two connections cannot fork again. 7 new tests, mutation-checked: restoring the old literal fails all three dashboard assertions with the expected messages. **The live verification disproved the report's stated mechanism, which is the more useful result.** Running a pre-fix and a post-fix binary side by side against a copy of the real `applications.db` (9,710 discovered rows) while a separate process held an open write transaction produced byte-identical metrics from both, and not one lock or busy error in either log — because the database is already in WAL mode, where readers never block on a writer at all. The claim "a query that meets a write lock fails immediately" is simply not true of the deployed configuration. What the fix *does* repair was found by running each binary in an empty directory: the pre-fix dashboard, reaching a new database first, created it with `journal_mode=delete` and no `-wal` file, while the post-fix one creates it in `wal`. So the real defect was a dashboard silently downgrading a fresh database's concurrency mode, not a lost read. Separately confirmed that a live connection built by `storage.DSN` reads back `busy_timeout=5000` where the old spelling reads back `0`; that pair is now a test and its negative control. One finding filed rather than folded in — see improvement #450. Original report: found 2026-07-30 during #437's review of the dashboard data path. #416 is filed under Resolved and its text names **both** `pkg/storage/manager.go` and `cmd/dashboard/main.go`. Only the first was fixed: `manager.go:42` builds the correct `_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&...` DSN, while `main.go:247` still opens `"./applications.db?_journal_mode=WAL"` — the `go-sqlite3` syntax `modernc.org/sqlite` silently ignores. The dashboard's connection therefore runs on driver defaults, notably with no busy timeout, so it fails immediately on `SQLITE_BUSY` instead of waiting. Its queries swallow errors into a log line and return zeros, so contention with a writing agent shows up as a briefly wrong dashboard rather than an error. Effort 1: one line, matching an existing correct one

### 446. The dashboard's own database connection still uses the pragma syntax bug #416 was closed for fixing

**Found 2026-07-30** during #437's review of the dashboard's data path.

Bug #416 ("modernc.org/sqlite uses invalid pragma format, causing DB locks") sits in this file's Resolved section, and its text names two files:

> The database connection strings in `pkg/storage/manager.go` **and `cmd/dashboard/main.go`** use the old `go-sqlite3` format for pragmas.

Only the first was fixed. `pkg/storage/manager.go:42` now builds the correct DSN:

```go
dsn += "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=cache_size(-20000)&_pragma=temp_store(MEMORY)"
```

while `cmd/dashboard/main.go:247` still reads:

```go
db, err = sql.Open("sqlite", "./applications.db?_journal_mode=WAL")
```

`modernc.org/sqlite` does not recognise that parameter and does not error on it — it ignores it. So the dashboard's connection runs on driver defaults.

**The consequence that actually bites is the missing busy timeout, not WAL.** Journal mode is a property persisted in the database file, so WAL is already on in practice because `cmd/agent` set it. `busy_timeout`, by contrast, is per-connection: the dashboard's connection has none, so a query that meets a write lock fails immediately rather than waiting the 5 seconds `pkg/storage` asks for. `serveMetrics` logs each query error and leaves that field at its zero value, so the visible symptom is a dashboard panel that reads 0 for one poll while the agent is writing — not an error, and indistinguishable from an empty table.

**The general point is worth more than the one-line fix.** #416 was closed as Resolved with half its own stated scope unshipped, and it was not caught by two subsequent groom passes because the row said "Resolved" and nobody re-read what it had claimed to cover. This is the same shape as #437 and #439: *a Done note describes what was done, not what was claimed.* When a bug names N files, closing it means checking N files.

**Fix:** use the same DSN as `pkg/storage`, ideally by extracting a shared connection-string builder so the two cannot drift again.

**Resolved 2026-07-30 — and the live verification contradicted the paragraph above.**

`pkg/storage/dsn.go` now exports `DSN(path)` and `DefaultDatabasePath`. `manager.go` and `cmd/dashboard/main.go` both call it, and `TestDashboardDSNMatchesStorage` pins the dashboard's value to the shared builder so a future edit cannot silently re-fork it.

The bolded claim above — *"a query that meets a write lock fails immediately rather than waiting the 5 seconds `pkg/storage` asks for"* — **does not reproduce.** A pre-fix binary and a post-fix binary were served side by side (`127.0.0.1:8097` and `:8098`) against copies of the real `applications.db`, with a separate process holding an open write transaction against each. Both returned byte-identical metrics, and neither logged a single lock or busy error. The reason is structural: the database is in **WAL mode**, and in WAL mode readers do not block on a writer. The symptom this bug was filed to explain could not have been caused by the defect this bug describes.

What the defect really did was only visible on a database that did not exist yet. Each binary was run in an empty directory and asked for `/api/metrics`, so it created `applications.db` itself:

| binary | resulting `journal_mode` | `-wal` / `-shm` present |
| --- | --- | --- |
| pre-fix | `delete` | no |
| post-fix | `wal` | yes |

So a dashboard that reached a new database before the agent did left it in rollback-journal mode — quietly downgrading the setting the rest of the project depends on — and that is what is now fixed.

The busy-timeout half is real but was independently confirmed rather than assumed: a live connection opened with `storage.DSN` reads back `busy_timeout=5000`, one opened with the old `?_journal_mode=WAL&_busy_timeout=5000` reads back `0`. Both assertions are now tests, the second as an explicit negative control — without it, `busy_timeout=5000` could just as well have been a driver default and would have proven nothing.

**The lesson, which is a new one for this file.** Every previous session's lesson has been that some piece of *prose* was untrustworthy — a Done note, a code comment, a row's arithmetic, a bug report's file list. This one is sharper: the prose that was wrong was **this bug report's own explanation of its own symptom**, and it was wrong in the direction that makes a fix look more urgent than it is. The row was still worth fixing; the stated reason was not the real one. Stated as a rule: **a bug report contains two separable claims — that something is wrong, and why it matters — and verifying the first does not verify the second.** The first was confirmed by grep in seconds. The second needed two binaries and an empty directory, and it fell.


---

## 445. [Any web page open in your browser can start or stop the agent](#445-any-web-page-open-in-your-browser-can-start-or-stop-the-agent)

**Table rationale cell (original):** **Fixed 2026-07-30.** `requireSameOrigin` now wraps `/api/agent/start` and `/api/agent/stop`. It trusts `Sec-Fetch-Site` first, accepting only `same-origin` and `none` — JavaScript cannot set that header, which is what makes it sound — and falls back to host-matching `Origin`, then `Referer`, against `r.Host`. It fails closed on a value that does not parse or that carries no host, which is what catches a sandboxed iframe's literal `null` Origin. A request carrying none of the three is allowed through **on purpose**, since that shape is curl or a script rather than a browser page; that is a deliberate tradeoff, not an oversight, and it means these endpoints are still unauthenticated against a non-browser caller on the same machine. `/api/metrics` and `/api/agent/status` stay ungated as read-only GETs. **Verified live** against a real dashboard on `127.0.0.1:8099`, not by tests alone: cross-site POST to start and to stop both 403, a forged matching `Origin` does not rescue a cross-site request, an `Origin`-only cross-host POST is 403, a same-origin GET still reaches the 405 method check, and every rejection announces itself in the log. 12 new table-driven tests against a sentinel handler, so no test can launch the binary; the assertions were mutation-checked by neutering the guard and confirming exactly the 8 rejection cases fail. Original report: found by a review pass over the dashboard's data path during #437. `serveAgentStart`/`serveAgentStop` (`cmd/dashboard/main.go:514-552`) check only `r.Method`; the file contains zero `Origin`, `Referer`, `Sec-Fetch-Site` or CSRF-token checks. A cross-origin `POST` with no custom headers is a CORS "simple request" — no preflight — so any page in any tab of the same browser can launch `./career_agent_bin -daemon` or kill a run mid-application. CORS blocks the attacker *reading* the response, not the side effect, which has already happened. Bug #126's loopback-only bind does not help: the request originates on the same machine. Distinct from #414 (multi-instance corruption) — that is the consequence, this is the unauthenticated trigger

### 445. Any web page open in your browser can start or stop the agent

**Resolved 2026-07-30.** `requireSameOrigin` wraps both state-changing endpoints; see the row above for the shipped behaviour and the live verification. The original report follows.

**Found 2026-07-30** by a review pass over the dashboard's data path, run in parallel with bug #437's UI restoration.

`serveAgentStart` and `serveAgentStop` (`cmd/dashboard/main.go:514-552`) validate exactly one thing about a request: that it is a `POST`. A whole-file grep finds **zero** occurrences of `Origin`, `Referer`, `Sec-Fetch-Site`, or any CSRF token. `serveAgentStart` then runs:

```go
cmd := exec.Command("./career_agent_bin", "-daemon", "-cycle-limit", "5")
```

A cross-origin `fetch('http://127.0.0.1:8080/api/agent/start', {method:'POST', mode:'no-cors'})` is a CORS **simple request**: no preflight, the browser sends it, and the server acts on it. CORS then blocks the attacking page from *reading* the response — but the process has already been launched. The same applies in reverse to `/api/agent/stop`, which can kill a run in the middle of an application.

**Why bug #126's loopback bind does not cover this.** #126 stopped a remote attacker from reaching the port over the network. This request does not come over the network: it comes from the user's own browser, on the user's own machine, and the OS treats it as ordinary loopback traffic. Binding to `127.0.0.1` is exactly as effective against it as binding to `0.0.0.0` — which is to say, not at all. Any page in any tab, including an ad iframe, can send it.

The consequence is not abstract for this project specifically: starting the agent means submitting real job applications, with the user's real PII, to real employers, without the user asking for it.

**Distinct from #414.** #414 is about two agent instances corrupting the database — that is one of the things that *happens* if this is triggered while a run is in progress. This row is about the trigger being unauthenticated in the first place.

**Fix direction:** reject any `POST` whose `Sec-Fetch-Site` is not `same-origin` (falling back to an `Origin`/`Referer` allowlist for older clients), or issue a CSRF token from the page and require it. Neither costs the dashboard anything, since its only legitimate caller is its own UI. Note that `serveAgentStart`/`serveAgentStop`/`serveAgentStatus` currently have no tests at all, so this needs its own.


---

## 444. [A single 429 shuts the whole agent down, and the log blames Gemini whatever provider is configured](#444-a-single-429-shuts-the-whole-agent-down-and-the-log-blames-gemini-whatever-provider-is-configured)

**Table rationale cell (original):** **Fixed 2026-07-30.** Added `classifyGenerationError` (`cmd/agent/pipeline.go`), which treats only `"Quota exceeded"` — Gemini's own wording for an exhausted daily allotment — as fatal; a bare `"429"` now joins the existing network-error branch and is retried with backoff like any other transient condition, on every provider including Gemini itself (its SDK returns 429 for the per-minute limit too, so the status code alone was never a safe signal). Both call sites (`ScoreJob`, `ProcessJobApplication`) route through it, and the CRITICAL/shutdown log line now names the live provider via a new `Client.ProviderName()` (`pkg/mcp/client.go`) instead of a hardcoded "Gemini". `pkg/submitter/vision.go`'s five Gemini-branded log lines/comments were replaced with a `visionModelLabel` helper that reads the mapper's `ProviderName()` when available (a soft type-assertion, so the `FormMapper` interface and its test mock did not need to change) and falls back to a generic label otherwise; the "API credits" comment was reworded since the default local vision model is free. New `cmd/agent/pipeline_test.go` covers `classifyGenerationError` with 7 cases including the exact repro (a bare 429 on Claude, and 429-with-no-provider-context) — **mutation-checked**: restoring the old blanket `429 |  | Quota exceeded` classification reproduces the bug and fails two of the new subtests by name. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l` on all four touched files all clean. Original report follows. Found 2026-07-30 during the #441 groom pass, while checking whether any DOM-parsing path calls a paid endpoint directly (it does not). Two call sites in `cmd/agent/pipeline.go` (269, 406) treat any error containing `429` or `Quota exceeded` as a fatal daily quota, calling `deps.Cancel()` and abandoning the rest of the batch — but a 429 from Anthropic is routinely a per-minute rate limit that the surrounding retry loop already knows how to back off from. Same pass found seven log lines and comments across `pipeline.go` and `pkg/submitter/vision.go` naming Gemini for calls that route through whichever provider `LLM_PROVIDER` selects, including one promising "Gemini-1.5-Pro" while running a local `qwen2.5vl:7b`

### 444. A single 429 shuts the whole agent down, and the log blames Gemini whatever provider is configured

**Resolved 2026-07-30.** See the row above for the shipped fix and its verification. The original report follows.

**Found 2026-07-30** during the post-#441 groom pass, while verifying that no DOM-parsing path calls a paid endpoint directly (it does not — `ExtractFormMapping` and `ExtractFormMappingVision` both go through `c.generate`, so `improvements_paywall.md` #424 is genuinely unbuilt).

**The defect.** `cmd/agent/pipeline.go:269` and `:406` sit inside a three-attempt retry loop that already backs off correctly on network errors. Both classify *any* error string containing `429` or `Quota exceeded` as a terminal daily quota:

```go
if strings.Contains(scoreErr.Error(), "429") || strings.Contains(scoreErr.Error(), "Quota exceeded") {
    log.Printf("[Worker-%d] CRITICAL: Gemini API Daily Quota Exceeded scoring job %s. Shutting down agent...", ...)
    deps.Cancel()
```

`deps.Cancel()` tears down the entire run, abandoning every remaining job in the batch. But **429 is not a daily-quota signal on Anthropic** — it is the ordinary per-minute rate limit, transient by design and exactly what the adjacent backoff branch exists to handle. A Claude user therefore loses a whole batch to a condition that would have cleared in seconds, and the substring test is loose enough that any error text merely containing "429" (an HTTP body, a request ID) triggers it too.

**Severity reasoning, stated because it is a judgment call:** filed **Minor**, not Major, and deliberately so. It cannot fire on the default provider — local Ollama does not rate-limit — and this repo assumes no paid keys, so no autonomous run here can reach it. It does **not** hold the Usability Gate's zero-Major box open. For anyone actually running `LLM_PROVIDER=claude` it is a Major reliability bug, and a future session with that configuration should re-rank it upward rather than inherit this line.

**The wording half, which is the cheap part of the same fix.** Seven log lines and comments name Gemini for calls that route through whichever provider `LLM_PROVIDER` selects:

- `cmd/agent/pipeline.go:269, 406` — "Gemini API Daily Quota Exceeded"
- `pkg/submitter/vision.go:48, 62, 64, 67, 70` — "uses Gemini Vision", "Transmitting screenshot to Gemini-1.5-Pro", "Pass image byte array to Gemini", `gemini vision failed to map visual layout`, "Gemini successfully mapped". `vision.go:74` also warns about spending "API credits" for a call that is free and local by default.

On this host that means the log announces "Transmitting screenshot to Gemini-1.5-Pro" while a local `qwen2.5vl:7b` does the work. That is the same trap #439's post-mortem named: a comment or log line that describes what the code *used* to do is a bug report, and this one would send anyone debugging vision failures to look at Gemini configuration that is not involved. Fix by naming the active provider and model from the client, not a literal.

Effort 2: the wording is mechanical, but distinguishing a retryable 429 from a genuine hard quota needs a real decision about what a fatal condition is, and it touches the two paths that can cancel a run.


---

## 441. [A clean setup ends up configured for models the installer never pulled](#441-a-clean-setup-ends-up-configured-for-models-the-installer-never-pulled)

**Table rationale cell (original):** Fixed on all three layers rather than at its narrowest. Both installers now read `.env` (parsing only the four `OLLAMA_*` keys, never sourcing a file that holds credentials) and pull what it names, then verify their own results against `/api/tags` and exit non-zero naming the exact `ollama pull` needed. `.env.example`'s two 32 GB recommendations are commented out so its defaults match the installer's. `cmd/agent` repeats the check at startup via `mcp.Client.PreflightModels` and refuses to start against an absent model, reporting every miss at once with the env var that set it; `SKIP_MODEL_PREFLIGHT` is the escape hatch. **Verified live**: the real binary aborts in ~1s on a missing model, the preflight passes against this host's Ollama, the installer confirms the real `.env`'s three models, and a simulated fresh clone with no `.env` reports `llama3.1`/`llava` missing instead of claiming success. 22 new Go tests, two of which fail if `.env.example` and the installer ever disagree again

### 441. A clean setup ends up configured for models the installer never pulled

**Found 2026-07-29** while fixing #439, and it is the same defect one layer out — #439 was a hardcoded model that nothing installs; this is a *documented* model that nothing installs.

The README's setup path is: run the bundled installer, then copy `.env.example` to `.env`. Those two steps disagree with each other.

- `scripts/install_ollama.sh:25-27` pulls `TEXT_MODEL="${OLLAMA_MODEL:-llama3.1}"` and `VISION_MODEL="${OLLAMA_VISION_MODEL:-llava}"`. Run plainly — which is how the README presents it — it pulls `llama3.1`, `llava` and `nomic-embed-text`.
- `.env.example:10` and `:21` ship **uncommented** `OLLAMA_MODEL="qwen3:30b-instruct"` and `OLLAMA_VISION_MODEL="qwen2.5vl:7b"`.

So a user who follows both steps in the documented order ends up with a `.env` naming two models that were never downloaded, and every text and vision call fails with a model-not-found. The installer's own header says the model names are "overridable via the same env vars the agent reads", which is true, but nothing exports `.env` into the installer's environment, and the README does not tell the user to create `.env` first.

The README's own model-recommendation paragraph documents the split honestly — "the installer's defaults (`llama3.1` + `llava`) fit modest hardware. If you have ~32 GB RAM, `qwen3:30b-instruct` and `qwen2.5vl:7b` can improve writing" — so the intent is clear and only the delivery is wrong: `.env.example` presents the 32 GB recommendation as the default.

Three ways out, in increasing order of effort: comment out the two qwen lines and note what the installer pulls; have the installer source `.env` when one exists; or have the installer pull whatever `.env` names and verify afterwards against `/api/tags`. The last is the only one that closes the class rather than this instance — **a setup that can complete successfully while leaving the agent configured for an absent model will keep producing bugs of this shape** (this is the third: #439, the Usability Gate's own stale Ollama box wording, and now this).

Worth doing alongside it: a preflight that queries `/api/tags` at agent startup and fails loudly if `OLLAMA_MODEL` is absent, which would have turned #439 and this into a startup error instead of a per-job failure discovered hours in.

**Resolved 2026-07-30.** All four layers were done, because the cheap fix alone would have left the class open:

1. **`.env.example`** — the two 32 GB recommendations are commented out, so copying the file verbatim yields the installer's own defaults. The values stay visible with a note on how to opt in.
2. **Both installers read `.env`** from the repo root (resolved from the script's own location, not `$PWD`), with precedence real env var → `.env` → installer default, so `OLLAMA_MODEL=x ./scripts/install_ollama.sh` still works. `.env` is **parsed, never sourced**: it holds `ANTHROPIC_API_KEY` and `IMAP_APP_PASSWORD`, so sourcing it would execute user-authored shell and drag every secret into the installer's environment. Only the four `OLLAMA_*` keys are read, and only those are ever logged. The bash parser's quote/comment handling was checked byte-for-byte against `godotenv` on a fixture covering `export` prefixes, both quote styles, trailing comments, and a `#` inside quotes — the installer and `cmd/agent` must resolve the same file identically or the bug just relocates.
3. **Both installers verify their own pulls** against `GET /api/tags` and exit non-zero naming each missing model and its exact `ollama pull`; with `--no-models` the same check warns instead. A reachable-but-empty library reports separately from an unreachable server, because the fixes differ — that distinction was added after a first pass conflated them.
4. **`cmd/agent` preflights at startup** (`pkg/mcp/preflight.go`, wired in `cmd/agent/main.go` right after `mcp.NewClient`). It reports every missing model at once with the variable that named it, lists what *is* installed so the user can choose instead, and never silently substitutes an installed model — which model writes the documents an employer receives is the user's decision, and quietly downgrading it is how #439 stayed invisible. Claude checks only the embedding model (Anthropic has no embeddings API; a Claude user's local text/vision names are irrelevant and must not fail their startup), Gemini is a no-op. `SKIP_MODEL_PREFLIGHT` is the escape hatch for an `OLLAMA_HOST` whose tag list is genuinely unreadable.

**Live verification, on the real binary rather than only the tests** — the log lines were observed, per the repo's standing rule that a fix nothing has watched fire is not yet a working fix:

- `OLLAMA_MODEL=llama3.1 ./career_agent_bin` aborted in ~1 second: `Startup aborted before any job was touched … missing 1 configured model(s): llama3.1 (OLLAMA_MODEL). Installed there: nomic-embed-text:latest, qwen2.5vl:7b, qwen3:30b-instruct, qwen3:4b-instruct.` No discovery, scoring or submission ran.
- `SKIP_MODEL_PREFLIGHT=true` with a dead `OLLAMA_HOST` logged `Model preflight SKIPPED … will now fail per job instead of at startup` and did not run the check.
- Against the live Ollama with the real `.env`: `Model preflight passed against http://localhost:11434: qwen3:30b-instruct, qwen2.5vl:7b, nomic-embed-text`.
- `./scripts/install_ollama.sh --no-models` read the real `.env` and confirmed all three configured models present. The same script copied into a `.env`-less tree — a simulated fresh clone, which is the exact state this bug is about — reported `missing: llama3.1` and `missing: llava` instead of claiming success.

**Two guards were added in `pkg/config/setup_consistency_test.go`, because the runtime preflight only protects someone who already installed — nothing protected the repository.** They parse `.env.example` and `install_ollama.sh` and fail when an uncommented `OLLAMA_*_MODEL` disagrees with the installer's default for the same variable, and assert the three behaviours the fix rests on plus the never-source-`.env` rule. Verified by mutation: uncommenting the qwen line reproduces #441 and fails the guard by name.

**The generalisation, which is the durable part:** two files that must agree, and only a convention saying so, is a bug waiting for its next groom pass. The README documented the split honestly the entire time — the prose was right and the shipped defaults were wrong, and prose cannot fail a test. Where two artifacts must agree, assert it; where a setup step can succeed while leaving the product unusable, make the step check its own work.


---

## 440. [`scripts/server.go` is the only script without `//go:build ignore`, so `go build ./...` compiles a dead generated file](#440-scriptsservergo-is-the-only-script-without-gobuild-ignore-so-go-build--compiles-a-dead-generated-file)

**Table rationale cell (original):** **Fixed 2026-07-30.** Added `//go:build ignore` (plus the matching blank line) ahead of `package main` in `scripts/server.go`, taking the simpler of the two options the detail section named — the file has no caller and duplicates what `cmd/dashboard` already serves, so there was no reason to promote it to `cmd/`. `go build ./...` and `go vet ./...` no longer compile it, matching all 17 siblings. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean. Note for the backlog: `scripts/server.go`'s interior (the transpiled Zero body, not the two added lines) was already not gofmt-clean before this fix and still isn't — out of scope for #440, and the documented `gofmt -l` loop deliberately excludes `scripts/`, so this doesn't fail any check; filed as a new Minor row rather than fixed here. Original report follows. Found while adding `scripts/verify_tailoring.go` and matching the directory's convention. Every other `.go` file in `scripts/` opens with `//go:build ignore`; `server.go` (added by #427, transpiled Zero output) does not, so it is the one file there that `go build ./...` and `go vet ./...` compile. It has no caller anywhere in the repo. Harmless today, but it means the scripts directory has two contradictory conventions and a stray generated file sits in the main build graph

### 440. `scripts/server.go` is the only script without `//go:build ignore`, so `go build ./...` compiles a dead generated file

**Fixed 2026-07-30.** `scripts/server.go` now opens with `//go:build ignore` (blank line, then `package main`), the same shape as all 17 siblings. `go build ./...` and `go vet ./...` no longer compile it. It has no caller and duplicates `cmd/dashboard`'s `/api/metrics`, so this was the simpler of the two options below rather than promoting it to `cmd/`. Verified via `grep -L "go:build ignore" scripts/*.go` returning empty, plus `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`, all clean.

**Found 2026-07-29** while adding `scripts/verify_tailoring.go` for #439 and copying the directory's convention.

Every other `.go` file in `scripts/` opens with `//go:build ignore`, so they are excluded from the module build and run individually with `go run scripts/<name>.go`. `scripts/server.go` — added by improvement #427, and transpiled Zero output (its comments carry `//line metrics_summary.zero:NN` markers) — does not.

**Count corrected 2026-07-30:** `scripts/` holds **18** `.go` files, **17** of which carry the tag, so `server.go` is one exception out of eighteen — not "out of 21" with "20 siblings" as the 2026-07-29 rows stated. Neither figure was ever right; the defect itself is unchanged, and re-verified this pass by `grep -L`. It is therefore the only file in that directory that `go build ./...` and `go vet ./...` compile, and it defines `package main` there on its own.

It has no caller anywhere in the repo, no Makefile target, and nothing imports it; it fetches `http://127.0.0.1:8080/api/metrics` and prints counts, duplicating what `cmd/dashboard` already serves.

Nothing is broken today — it compiles, and the build is green. The defect is that the directory now carries two contradictory conventions, so the next person adding a script has no single answer to follow, and a stray generated file sits in the main build graph where a future transpiler regression would break `go build ./...` for a file nobody runs. Either add the tag (matching all 20 siblings) or move it under `cmd/` if it is meant to be a real command.


---

## 439. [The NLP microservice call ignores your LLM configuration and hardcodes a model this host does not have](#439-the-nlp-microservice-call-ignores-your-llm-configuration-and-hardcodes-a-model-this-host-does-not-have)

**Table rationale cell (original):** Fixed, and the item turned out to be a regression rather than a miswiring: improvement #427 had replaced a working provider-agnostic in-process implementation with a hard dependency on a manually-started service. Generation is in-process again by default; the microservice is now an opt-in offload behind `NLP_SERVICE_URL`, health-checked before use, with fallback to in-process on failure. The circuit breaker, `[API Metrics]` logging, dynamic `num_ctx`, provider abstraction, 120-minute timeout and four lost prompt instructions are all restored, and the prompts now live only in Go and are sent over the wire so the copies cannot drift again. **Verified live on all three routes** (in-process 6m3s, offloaded 5m48s asking for the configured `qwen3:4b-instruct` rather than `llama3`, and service-configured-but-down falling back cleanly in 6m14s), plus 16 new Go tests and 12 Python tests

### 439. The NLP microservice call ignores your LLM configuration and hardcodes a model this host does not have

**Found 2026-07-29** while establishing what `nlp_service/` needs in order to document its setup in the README — the README had a feature bullet for it and no instructions at all, so nothing had ever exercised this path from a documentation standpoint.

`ProcessJobApplication` (`pkg/mcp/client.go:230-238`) builds the microservice request payload with two values hardcoded, each carrying a comment admitting it:

```go
"provider":        "ollama", // Hardcoded for local NLP service for now
"model":           "llama3", // Can be read from environment
```

`nlp_service/main.py` takes `provider`, `model` and `ollama_host` straight off the request (`main.py:16-19`) and passes the model to every one of its four generation calls (`main.py:71,123,126,129`). So on the tailored-document path there is no configuration in play at all: `LLM_PROVIDER` and `OLLAMA_MODEL` are both ignored.

**This is not theoretical, and it is not a small degradation.** Queried live, `localhost:11434/api/tags` returns:

```
['qwen3:4b-instruct', 'qwen3:30b-instruct', 'qwen2.5vl:7b', 'nomic-embed-text:latest']
```

`llama3` is **not installed**. So with `use_master_cover_letter: false` — the setting that turns on per-job tailored resumes and cover letters, the project's headline feature — every generation asks Ollama for a model that does not exist. The caller (`cmd/agent/pipeline.go:399-412`) retries three times with exponential backoff and then fails the job.

Three distinct problems in the same call, which is why they are one bug:

1. **The model and provider are hardcoded**, ignoring the user's configured provider and model. A user set up for Claude gets a local Ollama call; a user set up for `qwen3:30b-instruct` gets `llama3`.
2. **The default is a model nothing in this repo installs.** `.env.example` and the Usability Gate's Ollama box both name the qwen models; `scripts/install_ollama.sh` does not pull `llama3`. The one hardcoded value is the one value guaranteed wrong.
3. **The port is hardcoded too** (`http://localhost:8000/process`, `client.go:246`) with no env override and no preflight readiness check, so "service not started" and "service broken" are indistinguishable from the error and only surface after the pipeline has already spent time on the job.

**Why this is filed rather than fixed in the run that found it:** the fix itself is small — thread the already-resolved provider/model/host through the payload instead of literals, and add a preflight check — but *verifying* it is not. Confirming the tailored path works end to end means a real generation on this CPU-only host, which the repo's own notes measure in minutes to tens of minutes per call, and there are four calls per job. This repo's hardest-won lesson is that a shipped fix is not a working fix until something observes it firing (#76, #435), and a fix here that is only unit-tested would be exactly that. It needs a session with a live run, so it is the next item rather than a footnote to this one.

**Note for whoever takes it:** `use_master_cover_letter: true` in the committed `profile.yaml` is what has been masking this. That toggle routes around `ProcessJobApplication` entirely, so the whole microservice path has plausibly never run since it was introduced (improvement #427, 2026-07-29). Check whether it ever produced a document before assuming this is a regression rather than a feature that never worked.

**Resolved 2026-07-29.** The note above turned out to be the important part: it *was* a regression. Auditing `40767ca` (the commit that introduced the microservice) against its parent found that replacing the in-process Go implementation dropped six things beyond the filed configuration defect, none of which any test could see, because no test and no live run had ever exercised this path:

1. **The payload circuit breaker and `[API Metrics]` logging were gone.** The old path called `incrementAndLogAPICall` four times; the new one never called it, so the tailoring path had no size ceiling at all.
2. **Dynamic `num_ctx` was gone** — `(totalChars/3)+2000` clamped to `[8192, 64000]`, replaced by nothing, so Ollama silently fell back to its own small default and truncated long postings rather than erroring.
3. **The provider abstraction was gone.** `c.provider` was never consulted, and `nlp_service` accepted `provider` and `api_key` while reading neither. A Claude or Gemini user's tailoring silently became a local Ollama call.
4. **Bug #6 was reintroduced.** `defaultOllamaTimeoutMinutes` is 120 precisely because measured CPU generation of this shape takes 25-35+ minutes. The new path used a hardcoded 10-minute Go client timeout and a hardcoded **5-minute** `requests.post` timeout. Even with `llama3` installed, this path could not have finished a real generation on this host.
5. **Four prompt instructions were lost** when the prompts were retyped in Python: "anomaly detection", "CCNA foundation", "without extra commentary", "and talking points based on my profile", plus "or de-emphasize their necessity" from the gap-analysis injection.
6. **Error semantics got worse.** Gap analysis was best-effort before; the rewrite made it raise before the other three calls started. On the Go side the response body was never read, so FastAPI's `{"detail": ...}` — the only place the real Ollama error appeared — was discarded in favour of a bare status code.

**The fix, therefore, is not "pass the right model".** In-process generation is authoritative again, and the microservice is an opt-in executor:

- `NLP_SERVICE_URL` (unset by default) is the only thing that enables the offload. There is no hardcoded URL and no hardcoded port left.
- The offload is used only if the provider implements the new `offloadTarget` interface, which **only `ollamaProvider` does**. Claude and Gemini always generate in-process, because that service speaks Ollama's API and nothing else — silently redirecting them is the original bug.
- `GET /health` is checked before the service is used (the preflight this item asked for), and a transport failure mid-job disables the offload for the rest of that job and falls back rather than failing it. The service is an optimization, so it must never be able to lose a job the agent could have completed alone.
- **Go owns the prompts and sends them over the wire.** There is now exactly one copy of each prompt in the repo, so regression 5 cannot recur. `nlp_service` was rewritten as a generic concurrent executor (`GET /health`, `POST /generate`) that owns no prompts and no model choice, with per-call error isolation so one failed call no longer aborts the batch.
- Everything in 1-4 and 6 is restored, and the response body is now read and included in the error.

**Two defects found while fixing it, both fixed here.** An empty document is now an error instead of a saved file: Ollama can answer HTTP 200 with no content, and both paths would previously have written that out as a finished resume and applied with it. And a key the offload service acknowledges neither way is reported against that key rather than becoming an empty success.

**Verified live on this host, all three routes, with a real Ollama and real generations** (`scripts/verify_tailoring.go`, added for this and kept — the honest reason this item was deferred rather than fixed inline was that verifying it needs a real multi-call generation, so the repo now has a way to do that in minutes):

| route | result |
| --- | --- |
| in-process, `NLP_SERVICE_URL` unset | OK in **6m3s**; all four `[API Metrics]` lines present; real resume (2,888 chars), cover letter (1,564) and prep sheet (4,944) |
| offloaded, service running | OK in **5m48s**; log line `Offloading tailoring to http://localhost:8000 (model qwen3:4b-instruct via http://localhost:11434)` — the configured model, not `llama3`; the service's own log shows one `/health` then exactly two `/generate` batches (gap, then the three documents) |
| `NLP_SERVICE_URL` set, service stopped | OK in **6m14s** in-process, after `failed its health check (unreachable: ... connection refused). Generating in-process.` |

Plus 16 new Go tests (`pkg/mcp/tailoring_test.go`) and 12 new Python tests (`nlp_service/test_main.py`). The Go tests pin the properties that made this bug possible: configuration is threaded rather than literal, the offload is opt-in, an unhealthy service degrades instead of failing, the prompts still contain the exact strings that went missing, and the circuit breaker fires on this path again.

**Generalisation, and it is #437's lesson arriving a second time in one day from the opposite direction.** #437 was a rewrite that dropped a *UI* surface; this was a rewrite that dropped six *behaviours*. Both shipped with a Done note describing only what was added. The check that would have caught both is the same one: **before trusting a rewrite, diff it against what it replaced and enumerate what the old code did that the new code does not.** For #437 the API response struct was the checklist; here the deleted function body was.


---

## 438. [The React rewrite also deleted four tests, including both guards on the dashboard's loopback-only listener](#438-the-react-rewrite-also-deleted-four-tests-including-both-guards-on-the-dashboards-loopback-only-listener)

**Table rationale cell (original):** Same commit as #437, filed separately because the fix is unrelated to the UI: `TestNormalizeDashboardAddress`, `TestDashboardExposureWarning`, `TestNewDashboardServerUsesAddressHandlerAndTimeouts` and `TestServeFavicon` were deleted with the template, but all three functions they cover still exist and are the security boundary bug #126 established. Restored verbatim from `0028b2f^`

### 438. The React rewrite also deleted four tests, including both guards on the dashboard's loopback-only listener

**Found 2026-07-29** alongside #437, in the same commit's diff. Filed separately because the fix has nothing to do with the UI.

`git show 0028b2f -- cmd/dashboard/main_test.go` deletes 166 lines and four tests:

- `TestNormalizeDashboardAddress`
- `TestDashboardExposureWarning`
- `TestNewDashboardServerUsesAddressHandlerAndTimeouts`
- `TestServeFavicon`

Only the last is genuinely obsolete (the favicon moved into the Vite bundle). The other three cover `normalizeDashboardAddress`, `dashboardExposureWarning` and `newDashboardServer` — **all three functions still exist in `main.go`, unchanged**, and they are the security boundary bug #126 established: the dashboard is an unauthenticated server over real application data, so it must bind loopback-only and must warn when told not to. That guard went untested for the sake of a frontend rewrite.

**Resolved 2026-07-29:** all three restored verbatim from `0028b2f^` — they compile and pass against today's code untouched, which is itself the evidence that nothing about the covered behaviour changed and the deletion was collateral. `TestServeFavicon` was deliberately **not** restored; it asserted a `cmd/dashboard/favicon.png` that no longer exists, and the Vite bundle's `favicon.svg` is covered by #436's embed test instead.


---

## 437. [The React rewrite deleted the dashboard's conversion analytics and accessibility semantics without replacing them](#437-the-react-rewrite-deleted-the-dashboards-conversion-analytics-and-accessibility-semantics-without-replacing-them)

**Table rationale cell (original):** Found while wiring #435's reasons into the UI. Improvement #426's rewrite (`0028b2f`) deleted the 831-line template that was the *only* surface for improvement #15's conversion analytics — `by_source`, `by_variant` and `interview_rate_pct` are still computed and served, and now render nowhere — and dropped improvement #34's shipped accessibility work (two `<caption>`s, twelve `scope="col"` headers). Both were separately shipped, verified and marked Done. **Fixed 2026-07-30:** both conversion tables are back in `App.tsx` as one shared typed `ConversionTable` component carrying a `<caption>`, six `scope="col"` headers and a `scope="row"` cell per row — the row scope is new, so a screen reader announces "Greenhouse, Interviews, 3" rather than a bare number. The Interview Rate tile is back and now states its own denominator. `by_source`/`by_variant` were typed `any[]`, which is part of why the loss was invisible; they are typed rows now. **Verified live** against the real `applications.db`: 200s on both routes, one Greenhouse source row and one `unspecified` variant row rendered from real data, and the served bundle carries all six `scope="col"`, the `scope="row"` and both captions. Seven new Go tests, four of them the first coverage the dashboard's conversion queries have ever had (`pkg/storage`'s mirror had three; this side had none)

### 437. The React rewrite deleted the dashboard's conversion analytics and accessibility semantics without replacing them

**Found 2026-07-29** while wiring #435's reason strings into the UI and discovering there was nowhere to put them.

Improvement #426 ("TypeScript React/Vue Dashboard Rewrite", commit `0028b2f`) deleted `cmd/dashboard/index.html` — 831 lines — and replaced it with a 137-line React app rendering **six count tiles and nothing else**. The Go API was left almost untouched, so the deleted surface's data is all still computed and served on every poll:

- `by_source` — improvement #15's per-ATS conversion table (applied / interviews / rejections / pending / interview rate)
- `by_variant` — improvement #13's per-cover-letter-tone conversion table
- `interview_rate_pct`, `total_applied_tracked`, `interviews`, `rejections`
- the `blocked_captcha` and `invalid_url` counts

The old template rendered all of it, in two tables carrying a `<caption>` each and **twelve** `scope="col"` headers — the exact markup improvement #34 ("Make the local dashboard accessible and self-contained") was filed to add, shipped, and marked Done for. `git show 0028b2f -- cmd/dashboard/index.html | grep '^-'` shows the deletions directly.

**So two separately-filed, separately-verified, separately-Done improvements silently became unreachable in one commit**, and both still read `Done (2026-07-29)` in `improvements.md`. This is the same class as `improvements.md` #10, which had to be reopened as bug #120 when its "Done" turned out not to describe the code.

**The published project page advertises the missing surface (found 2026-07-30 while checking the docs for #441).** `docs/index.html:135` — the GitHub Pages "Live Web Dashboard & Controls" card — promises "live funnel counts, **active conversions**, and the status of current applications in real-time, complete with start/stop agent controls". The counts and the start/stop controls are real (`App.tsx` posts to `/api/agent/start` and `/api/agent/stop`), but "active conversions" is precisely what #426 deleted. No edit was made to the marketing copy, because the claim is the *intent* and #437 is the work that makes it true again; restoring the tables resolves the page too. If #437 is ever closed as won't-do, that sentence has to change in the same pass.

**Partially addressed by #435's work (2026-07-29):** the rewrite's `aria-live` loss and the four "last event" cards (applied / awaiting-you / skipped / failed, with their reason strings) were restored while fixing #435, since #435 had no other way to surface a reason. The two conversion tables, their `<caption>`/`scope="col"` semantics, and the `interview_rate_pct` summary are **still missing** — that is what remains open here.

**Lesson, and it is the third time this shape has appeared in this repo:** a rewrite that reimplements a subset is a regression, not a rewrite. #426's Done note describes what was built and says nothing about what was dropped. Any future rewrite of a surface should enumerate the fields the old surface consumed — the API struct is the checklist, and here it was right there in the same file.

**Fixed 2026-07-30.** Both tables are back, as a single generic `ConversionTable` component rather than two near-identical copies — they differ only in their caption and their first column's key, so the shared renderer is also what guarantees the two can never again drift apart in their markup. What shipped:

- **Conversion by Platform** and **Conversion by Cover-Letter Tone Variant**, each with its `<caption>` and six `scope="col"` headers, restoring improvement #34's semantics.
- **`scope="row"` on each row's first cell, which the old template did not have.** The platform or tone name is a row header, not data; without the scope a screen reader reads out a bare "3" with no way to know it belongs to Greenhouse's interviews. Restoring a deleted surface was the opportunity to fix what was wrong with it before it was deleted.
- **The Interview Rate tile**, which now also states its denominator ("1 interview and 0 rejections across 2 tracked applications") — `interview_rate_pct` alone is a percentage of an invisible base.
- **Typed rows.** `by_source` and `by_variant` were declared `any[]` in the `Metrics` interface. That is a small part of why this was invisible for so long: `any` means the compiler cannot tell the difference between a field being consumed and a field being ignored.
- Empty-array hiding is preserved from the old template — a breakdown with no data renders nothing rather than a header row over emptiness.

**Verified live**, not only by tests: the real binary served the real `applications.db` on loopback, returning 200 on `/` and `/api/metrics`, with one `Greenhouse` source row and one `unspecified` variant row rendered from actual data, and the bundle it served carrying six `scope="col"`, one `scope="row"` and both captions.

**The regression guard is the more durable half of this fix.** `TestUIDistEmbed_RendersEveryServedMetricsField` asserts that every field the API serves appears somewhere in the built bundle, and `TestUIDistEmbed_KeepsAccessibleTableSemantics` asserts the captions and scope attributes survive. Both were confirmed to fail against the pre-fix bundle before being trusted — they name all six lost fields and both lost tables, which is #437 reproduced as a test failure. They run against `ui/dist`, the artifact the binary actually embeds, rather than against `src/`, because a passing assertion on source with a stale `dist/` committed is precisely the trap that would let this recur.

One caution learned while writing them: the first version checked for the bare substrings `"col"` and `"row"`, which pass against *any* React bundle — React's own source contains both — so that assertion was worthless and silently so. It now matches `scope:` together with its value. **An accessibility assertion that cannot fail is worse than none, because it certifies the thing it never checked.**

**Four of the seven new tests are the first coverage the dashboard's conversion queries have ever had.** `pkg/storage`'s mirror implementation carries three tests for the same logic; this side had zero, which is why the whole surface could be deleted with a green suite. The test schema was also missing `tone_variant` entirely, so the `by_variant` query had been failing at runtime in every test that touched it and silently returning nothing.


---

## 436. [A fresh clone cannot build: the `go:embed`-ed UI bundle is gitignored](#436-a-fresh-clone-cannot-build-the-goembed-ed-ui-bundle-is-gitignored)

**Table rationale cell (original):** Found while opening the dashboard UI for #435. `cmd/dashboard/main.go` has `//go:embed ui/dist`, but `cmd/dashboard/ui/.gitignore` ignores `dist` — so `go build ./...` fails outright on any fresh clone. Reproduced in a clean `git clone` of this repo. The Usability Gate's first box was only ever green because of an untracked local build artifact

### 436. A fresh clone cannot build: the `go:embed`-ed UI bundle is gitignored

**Found 2026-07-29** while opening the dashboard UI to render #435's reason strings.

`cmd/dashboard/main.go:143` declares `//go:embed ui/dist`, making the built frontend bundle a **compile-time** dependency of the Go package. `cmd/dashboard/ui/.gitignore:11` was the stock Vite ignore list, which ignores `dist`. Those two facts are incompatible, and nothing in the working tree reveals it, because `dist/` exists locally as a leftover build artifact.

Reproduced, not reasoned about — a clean `git clone` of this very repository:

```
$ git clone --depth 1 file:///var/home/howlcipher/dev/Career_Agent_Core cleanclone
$ cd cleanclone && go build ./...
cmd/dashboard/main.go:143:12: pattern ui/dist: no matching files found
```

**The Usability Gate's first box — `go build ./... succeeds clean` — has therefore been passing on a false premise since #426 shipped (2026-07-29).** It was verified in this working tree, where the artifact happens to exist. Nobody cloning this repo could build any package in it, including `cmd/agent`.

**Fixed by tracking `dist/`**: `.gitignore` now un-ignores it with a comment explaining why build output is committed here, and the bundle is checked in (5 files, ~220K). The alternative — a build step before `go build` — would mean the documented three-command verification loop in `AGENTS.md` could not be run on a fresh clone without Node installed, which is a worse trade for a project whose other entrypoints need no frontend toolchain at all.

The fix introduces the opposite failure mode: a `dist/` committed stale, or hollowed out to a placeholder, still compiles and still serves HTTP 200 — just with no dashboard in the response. `TestUIDistEmbed_ContainsBuiltAssets` guards that by asserting the embedded tree actually contains `index.html` plus a `.js` and a `.css` under `assets/`. **Whenever `cmd/dashboard/ui/src` changes, run `npm run build` in `cmd/dashboard/ui` and commit `dist/` with it.**


---

## 435. [`statusReason` is dead code for every status the dashboard actually needs it for](#435-statusreason-is-dead-code-for-every-status-the-dashboard-actually-needs-it-for)

**Table rationale cell (original):** Found while adding an `AWAITING_REVIEW` case. `statusReason` is only reached from the `SKIPPED` and `FAILED_*` queries, so its `MANUAL_REQUIRED`, `BLOCKED_CAPTCHA`, `INVALID_URL` and new `AWAITING_REVIEW` arms never render. Its unit test passes because it tests the function, not the wiring. Fixed by widening the manual query and serving a `status_legend`; the reasons now actually render, which the old UI never did either

### 435. `statusReason` is dead code for every status the dashboard actually needs it for

**Found 2026-07-29** while adding an `AWAITING_REVIEW` case to it for improvement #423 — the new case works, and is never reached.

`statusReason` (`cmd/dashboard/main.go:122`) maps funnel statuses to human-readable explanations and handles `MANUAL_REQUIRED`, `BLOCKED_CAPTCHA`, `INVALID_URL`, `FAILED_SUBMIT`, `FAILED_SCORE`, `SKIPPED` and now `AWAITING_REVIEW`. But it is only called from two places: the `SKIPPED` query (`:315`) and the `FAILED_SCORE`/`FAILED_SUBMIT` query (`:338`). The manual-required query at `:353` does not even select the status column.

So four of its seven arms are unreachable in production. A copilot-filled job and an account-gated job are presented to the user identically, with no explanation of why either is waiting.

**This one is worth recording for how it hid:** `TestStatusReason_KnownAndUnknownCodes` (`cmd/dashboard/main_test.go:275`) passes, and passed before and after the `AWAITING_REVIEW` case was added. It tests the function in isolation and asserts nothing about whether anything calls it. That is the same shape as bug #76 — *a shipped fix is not a working fix until something observes it firing* — expressed as a test that observes the function rather than the feature.

**Resolved 2026-07-29.** Turned out to be worse than filed: `statusReason` was not dead for four of seven arms, it was dead for **all seven**, because #426's rewrite (bug #437) had already deleted every element that rendered `last_skipped_reason` and `last_failed_reason` too. The Go side was computing reasons for a UI that no longer had anywhere to show them.

Three parts:

1. **The reported defect.** The manual query (`main.go:349`) now selects `status` and sets a new `LastManualReason`, so `AWAITING_REVIEW` and `MANUAL_REQUIRED` — which share one Manual Queue tile and ask completely different things of the user (click an already-filled form vs. do the whole application by hand) — no longer present identically.
2. **The two counted-only statuses.** `BLOCKED_CAPTCHA` and `INVALID_URL` have count tiles but no "last job" card to hang a reason on, so nothing could ever have called `statusReason` for them. Added a `status_legend` map to the payload, built by iterating a new `explainedStatuses` list — a single code path that reaches every arm by construction.
3. **Somewhere to render it.** Added the two missing tiles plus four "last event" detail cards to `App.tsx`, and restored the `aria-live` region #426 dropped.

**The tests were written to fail the pre-fix code, and were checked doing so** rather than assumed to. Reverting each of the three changes in turn produces, respectively: `expected last-manual reason "Filled by Copilot — awaiting your review and submit" for a AWAITING_REVIEW row, got ""`; `expected the metrics payload to carry a status legend`; and `statusReason has an arm for "BLOCKED_CAPTCHA" but explainedStatuses omits it, so nothing renders it`. `TestExplainedStatuses_CoverEveryStatusReasonArm` fails in **both** directions — an unrendered arm and an unexplained status — which is the guard against this exact bug returning.

Also verified live against the real `applications.db` rather than only in tests, since the whole bug was tests passing over broken wiring: the built binary served `last_manual_reason` populated from a genuine row, all seven legend entries, and an `index.html` referencing the freshly built bundle hashes.


---

## 434. [No path moves a job out of `AWAITING_REVIEW` or `MANUAL_REQUIRED`, so hand-off statuses are permanent dead ends](#434-no-path-moves-a-job-out-of-awaiting_review-or-manual_required-so-hand-off-statuses-are-permanent-dead-ends)

**Table rationale cell (original):** Added `storage.MarkHandoffApplied` and `cmd/reconcile`, which promotes ticked checklist entries to `APPLIED` (dry run by default) and refuses any row that has already moved on. Widened the tracker's candidate query and tracked-company set to include both hand-off statuses. Verified live end to end, including the safety refusal

### 434. No path moves a job out of `AWAITING_REVIEW` or `MANUAL_REQUIRED`, so hand-off statuses are permanent dead ends

**Found 2026-07-29** in review of improvement #423's Copilot Mode work. The defect is inherited, not introduced — `MANUAL_REQUIRED` has had it since it existed — but Copilot Mode makes it matter far more, because in copilot mode *every* job takes this route.

There is no code path anywhere in the repository that transitions a row out of `AWAITING_REVIEW` or `MANUAL_REQUIRED`. The user opens the queue file, submits the application by hand, and the database never learns it happened. Two consequences follow:

1. **The funnel permanently under-reports real applications.** Every hand-off application the user actually completes stays recorded as un-submitted, forever. The dashboard's `APPLIED` count and every conversion metric built on it are wrong by exactly the number of applications the user personally sent.
2. **The email tracker cannot correlate the outcome.** `updateDBWithTrackerResult` (`pkg/tracker/imap.go:312`) only updates rows whose status is `APPLIED`. So when a rejection or interview invitation arrives for a hand-off application, it matches nothing and is logged as belonging to no tracked application — the single most valuable signal the system collects is dropped for exactly the applications the user cared enough about to send by hand. `GetTrackedCompanies` (`pkg/storage/manager.go:425`) compounds this by not including `AWAITING_REVIEW` in its match set at all.

**A third finding, not in the original write-up, surfaced on re-verification and made the diagnosis worse:** `MANUAL_REQUIRED` *was* already in `GetTrackedCompanies`' match set. So those emails were fetched, recognised as belonging to a tracked company, and then dropped anyway at the candidate query — which asked for `status = 'APPLIED'` and got nothing. Half-tracked was fully broken, and the half that looked correct was the reason nobody noticed.

**Fixed 2026-07-29.** Three parts:

1. **`storage.MarkHandoffApplied(companyName, jobTitle, applyURL) (bool, error)`** — the only writer that moves a row out of a hand-off status. It reads the current status inside a transaction and **refuses anything that is not `MANUAL_REQUIRED` or `AWAITING_REVIEW`**, so a checklist box that stays ticked can never overwrite a `REJECTED`/`INTERVIEW_REQUESTED` outcome the tracker recorded later, nor re-apply to an already-`APPLIED` row. A missing row returns `(false, nil)`: checklists outlive the rows they describe. The status update and the `applied_jobs` dedup insert commit together, because a promotion visible in one and not the other would let the agent re-apply to a job the user already sent.
2. **`cmd/reconcile`** — parses all three checklists (they share one entry format) and promotes every ticked entry. Dry run by default, `-confirm` to write, following `cmd/requeue`'s convention for anything that mutates the database. The line parser is a separate function with table-driven tests covering real-world company and role names containing spaces, commas, ampersands and embedded ` - `.
3. **Tracker eligibility** — `AWAITING_REVIEW` added to `GetTrackedCompanies`, and the candidate query widened to `IN ('APPLIED', 'MANUAL_REQUIRED', 'AWAITING_REVIEW')`. The zero/one/many branching, `filterCandidates`, and the ambiguous-match rollback from bugs #124/#125 are untouched; a test pins that two eligible hand-off rows still resolve to ambiguous with zero rows written, rather than a coin flip.

**Verified live, not just by unit test** — the lesson from #76 and from this session's own Copilot Mode review, where passing tests coexisted with broken behaviour. Built the binary into a scratch workspace with its own database and ran the real command: the dry run listed the ticked entry and ignored the unticked one; `-confirm` moved the row `AWAITING_REVIEW → APPLIED` and wrote exactly one `applied_jobs` row; re-running after forcing the row to `REJECTED` reported `skipped (no hand-off row)` and **left the status at `REJECTED`**. The safety constraint is the part most likely to matter years from now, so it was exercised against a real database rather than asserted.


---

## 433. [`mergeStatuses` ranks four real statuses at 0, so scheme dedup can revive terminal jobs](#433-mergestatuses-ranks-four-real-statuses-at-0-so-scheme-dedup-can-revive-terminal-jobs)

**Table rationale cell (original):** Found while adding `AWAITING_REVIEW` for improvement #423. `PROCESSED_MANUAL`, `INVALID_URL`, `FAILED_SCORE` and the quarantine status all fall to the `default: return 0` arm, so an HTTP/HTTPS merge resolves them in favour of *any* other status including `DISCOVERED` — re-queueing a job that was deliberately closed

### 433. `mergeStatuses` ranks four real statuses at 0, so scheme dedup can revive terminal jobs

**Found 2026-07-29** while adding `AWAITING_REVIEW` to `mergeStatuses` for improvement #423 — the act of checking whether a *new* status needed a rank exposed that four *existing* ones never got one.

`mergeStatuses` (`pkg/storage/manager.go:258`) ranks nine statuses and sends everything else to `default: return 0`. Four statuses the codebase actually writes fall into that default:

- `PROCESSED_MANUAL` — written at `cmd/agent/pipeline.go:321` whenever `auto_submit: false`
- `INVALID_URL` — the terminal status for a dead or non-posting URL (bug #123)
- `FAILED_SCORE`
- the prompt-injection quarantine status (bug #121)

Rank 0 loses to *everything*, including `DISCOVERED` at rank 1. So when the HTTP/HTTPS deduplication pass (`manager.go:371`, bug #112's migration) merges a duplicate pair where one row is `INVALID_URL` and the other is still `DISCOVERED`, the surviving row becomes `DISCOVERED` — and the job the agent had deliberately closed as a dead posting goes back into the queue for another full ~10-minute scoring cycle. The same applies to a quarantined posting, which is the one that actually matters: **a row closed for a security reason can be silently reopened by a dedup merge.**

**Scope is genuinely small, which is why this is Minor:** `mergeStatuses` has exactly one caller, the scheme-dedup migration, and it only fires on URLs present under both schemes — the last live recount in #112's notes found 20 such pairs, 15 with divergent statuses. It is not on the per-job hot path.

**Resolved 2026-07-29.** It was **five** statuses, not four: the quarantine status is actually two (`QUARANTINED_PROMPT_INJECTION` from `cmd/agent/main.go:56` and `QUARANTINED_RAG_CONTEXT` from `cmd/agent/pipeline.go:236`), confirmed by enumerating every status literal the codebase writes — fifteen in total.

The `rank` closure was extracted to a package-level `mergeStatusRank` so it is directly testable, and the five statuses were given explicit ranks. Relative order of the nine pre-existing statuses is unchanged; the new ones were inserted around them:

`DISCOVERED 1 · PROCESSING 2 · FAILED_SCORE 3 · SKIPPED 4 · BLOCKED_CAPTCHA 5 · FAILED_SUBMIT 6 · INVALID_URL 7 · MANUAL_REQUIRED / AWAITING_REVIEW 8 · PROCESSED_MANUAL 9 · QUARANTINED_* 10 · APPLIED 11 · REJECTED 12 · INTERVIEW_REQUESTED 13`

Two ranking decisions worth keeping written down, because both are the kind a later reader would "tidy":

- `INVALID_URL` sits **above** the "we tried and it failed" statuses, not below. Those describe an attempt that could be retried; `INVALID_URL` means the URL was never a posting, so there is nothing to retry.
- The quarantine statuses rank above every non-outcome status — a merge must never reopen a security closure — but **deliberately below** `APPLIED`/`REJECTED`/`INTERVIEW_REQUESTED`. Ranking them above those would mean a dedup merge could destroy a real observed outcome, and there is no reprocessing risk in ranking them below, because none of those three statuses is ever pulled back into the queue anyway.

`default: return 0` stays for genuinely unknown statuses, now with a comment saying plainly that 0 loses to everything — which is precisely how this bug happened. The `isSuccess`/`isFailure` special case and `migrateURLSchemes` were deliberately left untouched; widening the failure set would change behaviour for pairs that were never broken.

Three tests, and **the first was confirmed failing against the old ranking rather than assumed to** (`mergeStatuses(FAILED_SCORE, DISCOVERED) = DISCOVERED, want FAILED_SCORE` — the bug's exact symptom): the five formerly-unranked statuses each beat both queue statuses in both argument orders (symmetry is asserted, since `mergeStatuses` is order-independent by design); quarantine beats every non-outcome and loses to every outcome; and `TestMergeStatusRank_NoKnownStatusIsUnranked` holds the canonical fifteen-status list and fails if any resolves to the 0 arm. That last one is the guard — **a new `job_funnel` status added without a rank is this bug recurring**, so add it to that list when you add it to the codebase.

Implementation delegated to a Claude subagent against a written ranking decision; its diff, the mutation check above, and the full `build`/`vet`/`test` loop were reviewed and run in the orchestrating session.


---

## 432. [`auto_submit_click: false` records a false `APPLIED` for a form that was never submitted](#432-auto_submit_click-false-records-a-false-applied-for-a-form-that-was-never-submitted)

**Table rationale cell (original):** Found while mapping the submit path for improvement #423. The README's documented "fill and wait for review" toggle silently wrote confirmed-application rows plus permanent dedup entries. Fixed with the same sentinel-error mechanism #423 needed

### 432. `auto_submit_click: false` records a false `APPLIED` for a form that was never submitted

**Found 2026-07-29** while mapping the submission path for improvement #423 — not by a test and not by a live run. That is the point of this bug: nothing observes it.

`README.md:192` documents `auto_submit_click: false` as "fill out the form and wait for you to review it." The fill half worked. The accounting half did not:

1. Each of the four ATS handlers guarded its final `.Click()` with `if autoSubmitClick`, so with the flag off, the handler filled the form and returned `nil`.
2. `confirmOrError` (`pkg/submitter/browser.go:520`) short-circuited with `return nil` for the same reason — no click, so nothing to confirm.
3. `AttemptSubmit` therefore returned `nil`.
4. `cmd/agent/pipeline.go:490` reads `err == nil` as its success arm: `SaveCheckpoint("COMPLETED")`, `UpdateFunnelStatus(url, "APPLIED")`, and **`RecordApplicationInDB(...)`**, which writes the `applied_jobs` dedup row.

So every job processed with `auto_submit_click: false` was recorded as a confirmed application **and permanently deduped against ever being attempted again** — for a form that was filled and abandoned. The funnel metrics, the dashboard's `APPLIED` count, and `HasApplied` were all corrupted by a toggle the README tells people to use.

**Same defect class as #94/#95/#102/#107/#108** — the pipeline misreading its own outcome — and it lands squarely on the standing check earned by #94: *a benign-looking log line is not evidence of a benign event.* A run in this mode logs an ordinary success for every job, and the resulting `applied_jobs` rows are indistinguishable from real ones after the fact.

**Severity Major, not Blocker:** the live `profile.yaml` has `auto_submit_click: true`, so the currently-configured run path never enters it. The exposure is to anyone following the README's documented instruction, and to the historical record if the flag was ever flipped. **Not audited:** whether any historical `applied_jobs` row was produced this way. The rows carry no marker distinguishing them, so it cannot be determined retrospectively — noted here rather than guessed at.

**Fix (shipped 2026-07-29 with improvement #423, commit `8281adb`):** the no-click case now returns a distinct sentinel, `submitter.ErrSubmitClickDisabled`, produced by the shared `submitGate` helper every submit site consults. `cmd/agent/pipeline.go` branches it to `AWAITING_REVIEW` alongside Copilot Mode's `ErrAwaitingHumanReview`, moves the generated documents to the manual-apply folder, logs the job to `applications/needs_manual_apply/copilot_queue.md`, and **does not** call `RecordApplicationInDB`. `AWAITING_REVIEW` is ranked in `mergeStatuses` at the same needs-a-human tier as `MANUAL_REQUIRED`, so a dedup merge cannot discard it as an unknown rank-0 status, and it is recorded under its own `AttemptAwaitingReview` terminal class so source-health scoring does not read a deliberate stop as an ATS account gate. Covered by `TestSubmitGate`, `TestIsSubmitGated`, `TestSubmitGateResultsAreGated`, and `TestConfirmOrError_ReturnsSentinelWhenDisabled`.


---

## 414. [Enforce single-instance execution to prevent DB corruption and stuck jobs](#414-enforce-single-instance-execution-to-prevent-db-corruption-and-stuck-jobs)

**Table rationale cell (original):** Multiple instances corrupt DB and get jobs stuck in PROCESSING. Preventing this is a critical operational fix with minimal effort.

### 414. Enforce single-instance execution to prevent DB corruption and stuck jobs

We have a known critical bug where multiple `cmd/agent` processes can run simultaneously, fighting over `applications.db` and Ollama, leading to database corruption and stuck `PROCESSING` jobs.
We need to implement a single-instance enforcement mechanism (e.g. file lock `applications/career_agent.lock`) and also add logic on startup to auto-recover any jobs stuck in `PROCESSING` back to `DISCOVERED` in `applications.db`.


---

## 413. [Enhance Greenhouse validation error resolver for <fieldset> and radio groups](#413-enhance-greenhouse-validation-error-resolver-for-fieldset-and-radio-groups)

**Table rationale cell (original):** SolveValidationErrors struggles on Greenhouse forms with required radio groups where aria-invalid is applied to parent elements.

### 413. Enhance Greenhouse validation error resolver for <fieldset> and radio groups (Resolved 2026-07-29)

**Fix applied 2026-07-29:** Expanded `isInvalidControl` to check parent elements up to the form boundary for `aria-invalid` and `data-has-error`. Also updated `PruneDOMToForm` and `StripPresentationalAttrs` to translate common error classes ("error", "invalid", "field_with_errors") into `data-has-error="true"` before stripping presentational attributes, ensuring error signals survive DOM pruning. Added test `TestPruneDOMToInvalidFields_ChecksAncestorsForInvalidMarkers` to verify fieldset and container error classes. `go build/vet/test ./...` all pass.

`SolveValidationErrors` struggles on Greenhouse forms with required radio groups (work authorization, sponsorship) where `aria-invalid` or error class attributes are applied to the parent `<fieldset>` or container `<div>` instead of child `<input type="radio">`. Validation error detection should be expanded to inspect parent `<fieldset>` and container elements.


---

## 412. [Duplicate check in pipeline.go resets APPLIED jobs back to DISCOVERED](#412-duplicate-check-in-pipelinego-resets-applied-jobs-back-to-discovered)

**Table rationale cell (original):** Discovered jobs that were already applied were being reset to DISCOVERED status, creating an infinite processing loop.

### 412. Duplicate check in pipeline.go resets APPLIED jobs back to DISCOVERED

When `storage.HasApplied(job.URL)` was true, the funnel status was updated to `DISCOVERED` instead of `APPLIED`. This caused already-applied jobs to be re-discovered and re-processed in every subsequent batch run, creating an infinite loop. Fixed by changing the status to `APPLIED`.



---

## 396. [ATS board feed truncates large JSON feeds (30MB+)](#396-ats-board-feed-truncates-large-json-feeds)

**Table rationale cell (original):** Lever board feeds for companies like Jobgether exceed 37MB, which hits the 8MB `io.LimitReader` in `fetchATSFeed`. Increased the limit to 128MB.

---

## 395. [Validation loop times out waiting for Ollama context deadline](#395-validation-loop-times-out-waiting-for-ollama-context-deadline)

**Table rationale cell (original):** 480 validation attempts failed with 'context deadline exceeded' indicating the timeout to Ollama during form validation is too short.

---

## 394. [QUARANTINED_PROMPT_INJECTION has massive false positive rate on legitimate jobs](#394-quarantined_prompt_injection-has-massive-false-positive-rate-on-legitimate-jobs)

**Table rationale cell (original):** Over 400 legitimate jobs (Lever, Greenhouse) were quarantined. The detection heuristic is too aggressive.

---

## 393. [Playwright Host missing dependencies to run browsers](#393-playwright-host-missing-dependencies-to-run-browsers)

**Table rationale cell (original):** Cleared ms-playwright cache and reinstalled dependencies inside the ubuntu:22.04 distrobox so it downloads the correct binaries for that OS version.

---

## 131. [ATS board polling discards truncated JSON without retry](#131-ats-board-polling-discards-truncated-json-without-retry)

**Table rationale cell (original):** Added retry loop in `pollBoard` with exponential backoff for transient fetch errors and truncated JSON. Added tests utilizing httptest servers mimicking truncated responses.

### 131. ATS board polling discards truncated JSON without retry

**Evidence:** the same live log contained seven `unexpected end of JSON input` errors across four known boards: `jobgether` twice, `veeva` twice, `weloglobal` twice and `bluelightconsulting` once. Each error occurred in `pollBoard`, which means the complete current posting list for that board was discarded for that discovery pass.

**Root cause:** `pkg/scraper/atsfeeds.go::fetchATSFeed` reads one response and `pollBoard` calls the parser once. A partial or rate-limited response is logged and converted to zero discovered jobs; there is no retry, response-size/content validation, or board-level cooldown to prevent repeated noisy failures on the next continuous pass.

**Acceptance criteria:** retry truncated or otherwise retryable feed responses with a bounded, injectable backoff; validate status, content type and non-empty body before parsing; keep malformed payloads isolated to one board; add tests for transient truncation recovery, persistent malformed JSON, empty bodies and cancellation; retain the existing title and junk-URL gates.


---

## 130. [Yahoo fallback drops discovery on transient unexpected EOF responses](#130-yahoo-fallback-drops-discovery-on-transient-unexpected-eof-responses)

**Table rationale cell (original):** Done 2026-07-28: added context-aware retry policy with exponential backoff for transport and 5xx/429 errors. Covered by transient recovery, exhaustion, non-retryable and cancellation tests.

### 130. Yahoo fallback drops discovery on transient unexpected EOF responses

**Evidence:** the live `career_agent.log` review on 2026-07-27 found **148** lines of `Yahoo fallback failed: ... unexpected EOF` during one continuous run. The failures span many role/ATS queries, so the fallback source is repeatedly losing discovery opportunities rather than failing on one isolated query.

**Root cause:** `pkg/scraper/funnel.go::discoverWithYahooHTML` performs one HTTP request with a 10-second client and returns immediately when `client.Do` returns an error. It has no bounded retry/backoff, no distinction between transient transport failures and permanent responses, and no per-source health signal for the caller or dashboard.

**Acceptance criteria:** add a small context-aware retry policy for retryable transport errors and retryable status codes; preserve the existing per-query rate limit; stop after a bounded attempt budget; log the final failure with query-safe context; add tests for transient recovery, exhausted retries, non-retryable responses and cancellation; do not log API keys or personal data.


---

## 129. [The agent hard-codes one developer-specific career-profile path](#129-the-agent-hard-codes-one-developer-specific-career-profile-path)

**Table rationale cell (original):** Shared resolution now supports `-profile`, `CAREER_PROFILE_PATH`, and repository-local or sibling-library defaults. Startup validates the source before cached chunks, fails closed on missing or unverifiable context, and provides explicit `-no-rag` mode

### 129. The agent hard-codes one developer-specific career-profile path

`cmd/agent/main.go` declares a constant path under `/var/home/howlcipher/dev/ai_knowledge_library/USER_PROFILE.md`. `cmd/reingest` repeats the same machine-specific path as its flag default. There is no profile setting or environment override in the normal agent path.

On a different username, checkout layout, container mount or CI run, initial ingestion fails and the worker continues with no grounded career chunks. Because cached chunks may exist from an older profile, the failure can also leave stale personal context in use.

**Acceptance criteria:** resolve the profile through an explicit flag/config/environment setting with a portable repository-relative default where appropriate; validate readability at startup; fail closed or require an explicit no-RAG mode rather than silently using empty/stale context; share resolution code with `cmd/reingest`; test missing, configured and stale-cache cases without logging profile contents.

**Done 2026-07-27:** `pkg/config` now owns one resolver used by both maintained ingestion commands. Precedence is `-profile`, `CAREER_PROFILE_PATH`, repository-local `USER_PROFILE.md`, then the standard sibling knowledge-library checkout; configured paths never silently fall through to another source. `cmd/agent` validates the selected regular file before consulting cached chunks, refuses a failed dimension probe or zero-chunk rebuild, and returns jobs to `DISCOVERED` rather than scoring with empty context after retrieval failures. Explicit `-no-rag` mode bypasses both startup ingestion and per-job retrieval without consuming stale cached chunks.

Tests cover flag/environment/default precedence, sibling fallback, missing and non-regular paths, stale cache with a missing source, matching and mismatched cache dimensions, failed probes, empty rebuilds, and explicit no-RAG selection. Both built commands expose the portable options and exit nonzero before model/browser work when an explicitly configured profile is missing. Their smoke-test output contains neither profile contents nor the removed developer-specific path. `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./cmd/agent ./pkg/config -count=1` pass. Signed implementation commit: `653f320`.


---

## 128. [Saving a second role at the same company overwrites the first role's documents](#128-saving-a-second-role-at-the-same-company-overwrites-the-first-roles-documents)

**Table rationale cell (original):** Documents now live below a company directory and a normalized-URL hash. `SaveApplication` returns the exact directory, the agent hands that path to the manual queue, and atomic private-file replacement plus a save lock prevent interleaved artifacts. Focused collision/concurrency coverage and the full Go verification loop pass

### 128. Saving a second role at the same company overwrites the first role's documents

`pkg/storage/manager.go::SaveApplication` derives `applications/<safe company>/` and writes fixed `resume.md`, `coverletter.txt`, `interview_prep.md`, and `metadata.json` names. Job title and URL are stored inside metadata but do not participate in the directory key. Sanitization also maps distinct punctuation to underscores, creating extra collisions.

The second role replaces the first role's artifacts. With multiple workers, same-company jobs can interleave those writes; manual-apply movement can then preserve documents for the wrong role. Improvement #21 made the destination collision-safe but did not make the source role-specific.

**Acceptance criteria:** key artifacts by company plus stable job ID or normalized-URL hash; write atomically; return/store the exact artifact directory instead of reconstructing it from company name; make manual-queue links use that path; preserve or migrate existing company-only folders; test two roles, sanitization collisions and concurrent saves.

**Done 2026-07-27:** new artifacts are stored in `applications/<safe-company>/<normalized-url-hash>/`, preserving existing company-only folders without touching them. `SaveApplication` returns the precise directory and uses a process-local save lock with atomic private-file replacements. The agent passes that returned directory to `MoveToManualApply`, so manual-queue links identify the correct role. Tests cover two roles at one company, two labels that sanitize to the same company name, concurrent saves, exact cover-letter paths, and restrictive modes. `go build ./...`, `go vet ./...`, and `go test ./...` pass.


---

## 127. [Sensitive credentials application data and generated documents are world-readable](#127-sensitive-credentials-application-data-and-generated-documents-are-world-readable)

**Table rationale cell (original):** Maintained commands enforce an owner-only umask and fail closed if startup repair cannot secure known private paths. Storage creates private artifacts at `0600` under `0700`; the idempotent repair refuses symlinks. Tests and live metadata verification pass

### 127. Sensitive credentials application data and generated documents are world-readable

This audit inspected permission metadata only. `.env`, `pii.yaml`, the SQLite database/WAL files, source résumé/cover-letter files, and generated application artifacts are currently mode `0644`; generated company directories are `0755`. `SaveApplication` explicitly creates `0755` directories and writes résumé, letter, prep and metadata files as `0644`.

On a multi-user machine, another local account can read credentials, personal contact data, application history and tailored documents. `.gitignore` prevents repository disclosure but is not a filesystem access control.

**Acceptance criteria:** use `0700` for private directories and `0600` for credentials, databases, logs and application documents; start with a restrictive process policy/umask; safely repair existing paths at startup or through a documented command; do not follow symlinks during repair; add mode tests and a clear warning when permissions cannot be secured.

**Done 2026-07-27:** every maintained command now applies an owner-only umask before opening databases or logs and fails closed with a clear warning if existing private paths cannot be secured. `pkg/security` repairs the known credential, SQLite, log, source-document, and generated-application paths idempotently. Changed paths are opened with `O_NOFOLLOW` and chmodded through their descriptors; symbolic links and non-regular paths are refused. `pkg/storage` creates application directories at `0700`, writes documents and reports at `0600`, and secures SQLite database, WAL, and shared-memory files before and after initialization. `cmd/securefiles` exposes the same bounded repair as a documented maintenance command.

Tests cover restrictive process defaults, recursive and repeat repair, symbolic-link target preservation, warning propagation, private database creation, and generated artifact modes. `go build ./...`, `go vet ./...`, `go test ./...`, and the focused race suite for security, storage, tracker, agent, and dashboard all pass. The live repair required one exact-path chmod for a legacy container-owned log, then completed successfully: every named private root file is `0600`, all generated directories are `0700`, all generated files are `0600`, and the application tree contains no symlinks. Signed implementation commit: `e4e48e1`.


---

## 126. [The unauthenticated dashboard binds every network interface while announcing localhost](#126-the-unauthenticated-dashboard-binds-every-network-interface-while-announcing-localhost)

**Table rationale cell (original):** The dashboard now defaults to `127.0.0.1:8080`, validates an explicit `-addr`, warns on non-loopback exposure, and uses a dedicated server with defensive timeouts. Tests and a live container restart prove both routes work on loopback while the host's non-loopback address cannot connect

### 126. The unauthenticated dashboard binds every network interface while announcing localhost

`cmd/dashboard/main.go` logs `http://localhost:8080` but calls `http.ListenAndServe(":8080", nil)`. An empty host binds all interfaces. The root page and `/api/metrics` have no authentication and expose application funnel data, recent role/company context and posting URLs. The default server also has no read-header, read, write or idle timeouts.

**Acceptance criteria:** default to `127.0.0.1:8080`; make the address configurable and visibly warn or require an access control when a non-loopback address is selected; use an explicit `http.Server` with defensive timeouts; test default/configured bind selection and preserve dashboard behavior on loopback.

**Done 2026-07-27:** the dashboard now defaults to `127.0.0.1:8080`, accepts an explicit validated `-addr`, and prints a prominent warning whenever the selected host is not provably loopback. It uses a dedicated `http.Server` with 5-second read-header, 15-second read, 30-second write, and 60-second idle timeouts. Tests cover default and configured IPv4/IPv6 addresses, malformed ports, wildcard/LAN warnings, handler selection, and every timeout. `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./cmd/dashboard -count=1` pass. End-to-end probes returned HTTP 200 for both routes on a separate loopback port; the rebuilt existing container dashboard now listens only on `127.0.0.1:8080`, and its non-loopback probe is refused. Signed implementation commit: `b717e53`.


---

## 125. [Ambiguous outcome emails retry forever instead of entering manual review](#125-ambiguous-outcome-emails-retry-forever-instead-of-entering-manual-review)

**Table rationale cell (original):** Updated tracker to correlate using ATS IDs or role keywords; ambiguous emails now generate a manual review task and are acknowledged to prevent retry loops.

### 125. Ambiguous outcome emails retry forever instead of entering manual review

Before #124, the tracker matched an outcome email at company level and could update every `APPLIED` row for that company. Bug #124 removed that corruption path: the update and Message-ID acknowledgement now share one transaction, and a match affecting more than one row rolls back. Its two-role regression proves both rows remain `APPLIED` and the email remains retryable.

The remaining limitation is fail-closed but incomplete. There is still no role, posting URL, requisition ID, or thread correlation. An ambiguous message now fails on every 15-minute poll, writes only a transient error log, and never enters a durable manual-review queue. The outcome signal is preserved in the inbox, but automated tracking cannot progress and the repeated retries create noise.

**Acceptance criteria:** correlate to one application using stable ATS/requisition metadata and normalized role clues; if more than one candidate remains, persist one explicit ambiguous event for manual review and update none; avoid repeating the same error every poll after durable handoff; add two-role tests for rejection, interview, ambiguous subjects, and idempotent manual routing.

**Re-scored 2026-07-27 after #124:** the high-value bulk-corruption failure is fixed, so this is now Minor. Value drops from 7 to 3; Decay remains 1.0 because defect value does not diminish with unrelated tracker fixes; Effort remains 4 for stable role correlation plus a durable manual-review path. Score: **0.75**, above the ROI floor.

**Done 2026-07-28:** the tracker now resolves multiple APPLIED roles at the same company by extracting stable ATS IDs (Greenhouse/Lever) from the stored job URL or searching for normalized role title keywords in the incoming email's subject and body. If exactly one role matches, it is updated. If the match remains ambiguous (zero or multiple matches), the system explicitly logs a `MANUAL_REQUIRED` task with context and acknowledges the message in `processed_emails` so it does not retry on the next poll. `go test ./pkg/tracker` passes with new coverage for ambiguous resolution, ID matching, and title keyword matching. Signed implementation commit: `9db9322`.


---

## 124. [The email tracker acknowledges a message even when its database update fails](#124-the-email-tracker-acknowledges-a-message-even-when-its-database-update-fails)

**Table rationale cell (original):** Outcome updates and Message-ID acknowledgements now commit in one SQLite transaction. Errors roll back and leave the message retryable; unmatched, no-op and one-row updates are distinct, while multi-row matches fail closed. Lock, acknowledgement-failure, rollback and retry regressions pass

### 124. The email tracker acknowledges a message even when its database update fails

`pkg/tracker/imap.go::updateDBWithTrackerResult` returns no value and discards the result of `db.Exec`. Its caller then reaches `storage.MarkEmailProcessed` regardless of whether the update succeeded. The preceding log says “Updating database,” which can look like confirmation even when SQLite rejected the write.

The Message-ID ledger makes the loss durable: later polls skip that email forever. A momentary database lock can therefore erase the only automated rejection/interview signal.

**Acceptance criteria:** return and handle the database error; use a transaction where outcome write and processed-message acknowledgement can succeed together, or acknowledge only after a confirmed durable outcome; distinguish unmatched/no-op/updated states; verify exactly one intended row changed; add tests for locked/erroring DBs and successful retries.

**Done 2026-07-27:** `updateDBWithTrackerResult` now validates the requested outcome and returns an explicit unmatched, no-op, or updated result. For matched outcomes it updates exactly one `APPLIED` row and inserts the Message-ID ledger entry in the same SQLite transaction. Database locks, acknowledgement failures, and matches affecting more than one active application roll back without acknowledging the email, so a later poll can retry safely. Tests cover every result state, invalid status rejection, multi-row rollback, a real SQLite write lock followed by retry, and an acknowledgement-table failure followed by retry. `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./pkg/tracker -count=1` pass. Signed implementation commit: `fabe12c`.


---

## 123. [Failed and non-2xx job-page fetches still proceed to expensive fit scoring](#123-failed-and-non-2xx-job-page-fetches-still-proceed-to-expensive-fit-scoring)

**Table rationale cell (original):** Missing descriptions now require meaningful 2xx page content before model work. Closed postings become `INVALID_URL`; transient failures receive bounded retries and return to `DISCOVERED`; every response closes within its attempt and affected status writes are checked

### 123. Failed and non-2xx job-page fetches still proceed to expensive fit scoring

In the main worker, a transport error from `httpClient.Do` has no error branch: the description remains blank and processing continues. Successful responses are read regardless of `StatusCode`, so a 404, 429 or 500 error page becomes job text. `defer resp.Body.Close()` is also inside the long-lived worker loop, retaining bodies until that worker finishes the entire queue.

The next step is the pipeline's slowest operation: embedding plus fit scoring. Apart from wasted runtime, a title-only or error-page score is not a grounded decision about a real posting.

**Acceptance criteria:** accept only usable 2xx content with a conservative minimum signal; close each response in the current iteration; classify 404/410 as terminal invalid/closed postings; return transient network, 429 and 5xx failures to a retryable state with bounded backoff; check status-write errors; cover each branch with an injected HTTP client/server.

**Done 2026-07-27:** `fetchJobPage` now accepts only 2xx responses with at least 200 visible runes before the worker may embed or score a missing description. HTTP 404 and 410 responses move to `INVALID_URL`. Transport errors, response-read failures, HTTP 429, and HTTP 5xx responses receive at most three attempts with one-second and two-second context-cancellable waits; exhausted failures return to `DISCOVERED`. Other non-success responses and weak 2xx pages also remain retryable without a hot loop. Response bodies close inside each attempt before any wait or return, and every affected funnel-status write reports errors. The existing CAPTCHA distinction remains intact: explicit challenges and weak non-SPA widget pages route to `BLOCKED_CAPTCHA`, while widget presence on a real posting does not pre-skip it. Injected servers and HTTP clients cover every disposition, retries, response closure, cancellation, and CAPTCHA classification. `go test -race ./cmd/agent -count=1`, `go build ./...`, `go vet ./...`, and `go test ./...` pass.


---

## 122. [SSRF defenses block literal private IPs but not hostnames that resolve to them](#122-ssrf-defenses-block-literal-private-ips-but-not-hostnames-that-resolve-to-them)

**Table rationale cell (original):** One injectable policy now rejects any non-public answer across both address families. Go transports dial validated IPs directly; discovery, redirects and posting fetches use them, while authenticated loopback proxies bind every Playwright context to the same guarded dialer

### 122. SSRF defenses block literal private IPs but not hostnames that resolve to them

The worker and redirect checks block three literal hostnames. Hacker News and scraper fetches use the same shape. Playwright's route filter calls `net.ParseIP(reqURL.Hostname())`, which works only when the URL itself contains an IP literal. It never resolves a normal hostname before deciding.

Consequently, a hostname whose A/AAAA record points to loopback, RFC1918, link-local or unspecified space passes validation. DNS rebinding can also change the result between a validation lookup and the connection. This is broader than one call site because arbitrary posting links enter through public feeds and browser subresources.

**Acceptance criteria:** centralize HTTP/HTTPS URL validation; resolve every address family and reject if any result is non-public; enforce the check on initial URLs, redirects, feed fetches and browser requests; bind validation to dialing or otherwise close the rebinding gap; make the resolver/dialer injectable; test loopback/private/link-local IPv4 and IPv6, redirects, mixed public/private answers, and a legitimate public host.

**Done 2026-07-27:** `security.NetworkGuard` now owns HTTP and HTTPS syntax validation, complete A/AAAA answer-set validation, special-use address classification, and connection-bound dialing. It rejects the entire host when any answer is non-public, then passes only a validated IP literal to the injected final dialer. Its guarded `http.Client` disables environment-configured forwarding proxies and revalidates both initial requests and every redirect. The worker validates all job URLs before claiming them, while every RemoteOK, Hacker News, ATS-feed, SerpApi and Yahoo fetch uses the same client factory.

Playwright now creates each browser context with an ephemeral authenticated proxy listening only on `127.0.0.1`. The proxy applies the guarded dialer to ordinary HTTP and HTTPS `CONNECT` traffic, Chromium's implicit loopback bypass is subtracted, and the existing route layer independently resolves and checks every initial or subresource URL. This binds validation to the actual browser connection rather than trusting a preflight lookup.

Tests cover public and special-use IPv4 and IPv6 ranges, literal and resolved targets, mixed public/private answers, validated-IP dialing, unsafe redirects, a resolution that rebinds between preflight and dial, proxy authentication, HTTP forwarding and `CONNECT` tunneling. The real-Chromium integration test passed in the documented `career-agent` distrobox from a host-built test binary: the public-name request traversed the guarded proxy, while a loopback navigation never reached the local target. `go build ./...`, `go vet ./...`, `go test ./...`, and focused race suites for `pkg/security`, `pkg/scraper`, `pkg/submitter`, and `cmd/agent` pass.


---

## 121. [Untrusted job text reaches embedding and scoring models before quarantine](#121-untrusted-job-text-reaches-embedding-and-scoring-models-before-quarantine)

**Table rationale cell (original):** One typed deterministic boundary now protects posting embedding/scoring and every model-facing generic, Greenhouse, Lever, cached, validation, and Vision path. Detections retain the private CSV audit, receive a terminal status, and never reach an LLM judge

### 121. Untrusted job text reaches embedding and scoring models before quarantine

The worker fetches and prunes a posting, builds `jobDescText`, sends it to `client.GetEmbedding`, and then passes the scraped data to `client.ScoreJob`. Only between those calls does it invoke `filter.CheckPayload(tailoredContext)`, where `tailoredContext` is trusted résumé/RAG material—not the fetched posting.

There is a later scan inside part of the browser submission path, but that occurs after scoring and is not a centralized guard for dedicated Greenhouse/Lever handlers. `Pipeline.TwoStepVerification` contains a more appropriate boundary but is not the path used by the main worker. The README's security-quarantine claim therefore does not hold for the first model calls.

**Acceptance criteria:** route every fetched description and relevant DOM through one pre-LLM quarantine boundary before embedding, scoring, mapping, or judging; make a block a durable terminal/quarantine status rather than leaving `PROCESSING`; do not expose flagged text to a model as instructions during review; preserve the CSV audit log; add spy-based tests proving malicious input causes zero model calls on generic and dedicated ATS paths.

**Done 2026-07-27:** `security.QuarantinePayload` is now the typed deterministic boundary for untrusted posting text and DOM. The worker wraps its posting-dependent embedding and scoring stage with that boundary, preserves structured detections in `applications/prompt_injection_detections.csv`, and moves blocked rows to `QUARANTINED_PROMPT_INJECTION` before any model callback can run. Browser submission applies the same boundary to initial, cached, dynamically revealed, dedicated Greenhouse/Lever, validation-retry, and pre-Vision DOM. The earlier LLM safety-judge override was removed, so flagged attacker text is never presented to a second model; typed error text also omits the match.

Spy tests cover benign passage and malicious raw HTML, checked status-write failures, initial and dynamically revealed generic/Greenhouse/Lever pages, the private CSV audit, and legacy mapping entry points. They prove zero embedding and scoring calls for malicious posting payloads, plus zero mapper, Vision, validation-solver, or judge calls for quarantined DOM; initial browser detections also precede document generation. `go build ./...`, `go vet ./...`, `go test ./...`, and the race suite across `pkg/security`, `cmd/agent`, and `pkg/submitter` pass.


---

## 120. [`--daemon` logs a six-hour drip mode but exits after one batch](#120---daemon-logs-a-six-hour-drip-mode-but-exits-after-one-batch)

**Table rationale cell (original):** Daemon mode now refreshes discovery and database work every six hours, applies a configurable positive per-cycle cap, and cancels its wait on SIGINT or SIGTERM. Batch mode remains one unlimited cycle

### 120. `--daemon` logs a six-hour drip mode but exits after one batch

`cmd/agent/main.go` parses `--daemon` and logs that the agent will drip applications every six hours. The flag is never read again. The normal producer and worker goroutines run once, `wg.Wait()` returns, “Batch execution complete!” is logged, and `main` exits.

This also invalidates `improvements.md` #10's “Done” rationale and the README launch instruction. It matters operationally because a user can leave the documented command running believing new jobs will be discovered later when the process has already stopped.

**Acceptance criteria:** extract a testable one-cycle function; in daemon mode refresh discovery/DB work each cycle, enforce a configurable per-cycle cap, wait on a context-cancellable clock, and exit promptly on SIGINT/SIGTERM; non-daemon behavior remains one batch; add deterministic tests with an injected clock rather than real six-hour sleeps.

**Done 2026-07-27:** `runAgentCycle` now loads the current `DISCOVERED` rows and invokes every discovery source on each call, merges both streams through one limit boundary, and starts a fresh worker queue for the accepted jobs. `runAgentSchedule` preserves one unlimited cycle for ordinary batch mode; daemon mode repeats the injected cycle every six hours with a default 15-job cap configurable through `-cycle-limit`. Its timer selects on the signal-backed context, so `SIGINT` and `SIGTERM` cancel the wait without another cycle or a six-hour shutdown delay. The database, browser, and model client remain process-scoped rather than being recursively recreated.

Deterministic tests cover one-shot batch behavior, repeated daemon cycles, refreshed backlog and discovery calls, a cap spanning both job sources, invalid daemon settings, an injected cancellable clock, and the real timer helper's cancellation path. `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./cmd/agent -count=1` pass.

Signed implementation commit: `6453177`.


---

## 119. [Free discovery sources are disabled when the SerpApi key is absent](#119-free-discovery-sources-are-disabled-when-the-serpapi-key-is-absent)

**Table rationale cell (original):** Free RemoteOK, Hacker News, and public ATS-feed discovery now runs before any optional-key decision. Without SerpApi, role/ATS queries use Yahoo directly; an isolated regression proves free jobs are emitted and SerpApi receives zero requests. Full build, vet, and tests pass

### 119. Free discovery sources are disabled when the SerpApi key is absent (Resolved 2026-07-26)

`pkg/scraper/funnel.go::DiscoverJobs` checks `SERPAPI_API_KEY` and returns immediately when it is blank. Calls to `discoverWithRemoteOK`, `discoverWithHackerNews`, and `discoverWithATSFeeds` come only after that return.

Those three sources are free and already shipped. The ordering makes them depend on an unrelated keyed source anyway, contradicting the project constraint that external keys are not assumed for autonomous free work. Existing funnel tests always install a fake SerpApi key, so the supported no-key configuration has no regression test.

**Acceptance criteria as filed:** always run free sources; run SerpApi only when a real key is present; retain the free Yahoo fallback where appropriate; return a useful aggregate error only when discovery truly produced nothing or a required source failed; add a no-key test proving free sources are invoked and no SerpApi request is attempted.

**Resolution:** `DiscoverJobs` now runs RemoteOK, Hacker News, and public Greenhouse/Lever feed discovery first. It trims and checks `SERPAPI_API_KEY` only for the subsequent role/ATS search loop: a configured key uses SerpApi, while an absent key starts that loop in the existing Yahoo fallback mode. SerpApi errors still switch the remaining queries to Yahoo as before. Re-evaluation removed the proposed aggregate-error behavior: every source in this best-effort discovery fan-in is optional, and a valid run may produce no jobs, so the function has no required-source failure to aggregate. The free-source helpers continue to log their own failures, and no missing optional credential is treated as a discovery failure.

**Tests and documentation:** `TestDiscoverJobsWithoutSerpAPIKeyRunsFreeSources` uses isolated HTTP servers and a temporary database to prove a RemoteOK result reaches the channel, Yahoo receives the role/ATS query, and SerpApi receives zero requests. Existing SerpApi-success and SerpApi-to-Yahoo tests remain green. README and `.env.example` now describe the key as optional. `go build ./...`, `go vet ./...`, and `go test ./...` all pass.


---

## 118. [Resume-selector fallback work breaks every submitter path without a readable resume](#118-resume-selector-fallback-work-breaks-every-submitter-path-without-a-readable-resume)

**Table rationale cell (original):** Resume controls are now resolved before the source file is read. Optional forms with no upload control continue cleanly; mapped, named-fallback and sole-file-input controls upload the resume; a found control with an unreadable/empty source fails clearly. Six regressed scenarios plus five focused tests pass

### 118. Resume-selector fallback work breaks every submitter path without a readable resume (Resolved 2026-07-26)

**Found from the current worktree, not inferred from history.** `pkg/submitter/browser.go` contains uncommitted #118 work adding `resumeFileInputSelectors` and `attachResume` after a live Pinpoint mapping targeted a nonexistent resume selector. Preserving that fallback is important. The regression is that `attachResume` reads `resumePath` before it establishes whether a resume upload is mapped or required.

That changes the contract for every dynamic and Vision test that deliberately omits a resume path while testing another behavior. Full verification now fails in six established `pkg/submitter` tests: both custom-question tests, both end-to-end fallback tests, and both cover-letter tolerance tests. The failures occur before those scenarios reach their intended assertion.

**Acceptance criteria as filed:** keep the mapped-selector-plus-fallback behavior; do not read a file when no resume control is present or required; fail clearly when a required resume exists but its file is unreadable; preserve optional-resume behavior; add focused tests for mapped, fallback, absent and unreadable cases; restore `go test ./...` without weakening the six existing tests.

**Resolution:** split control discovery from file reading. `findResumeUpload` searches the mapped selector, resume/CV-named fallbacks, and the sole non-cover-letter file-input rule first. If an optional form has no upload control it returns cleanly without touching the path; once a real control exists, unreadable or empty content is a hard failure and upload errors retain their selector context. Nil locator guards keep sparse test/browser targets from panicking.

**Tests:** added focused coverage for optional no-control/missing-path behavior, mapped-selector miss with named fallback, unreadable required content, the sole-file-input fallback, and exclusion of cover-letter signals from that last-resort selector. Re-ran the six orchestration tests that originally regressed, then `go build ./...`, `go vet ./...`, and `go test ./...`; all pass.


---

## 117. [A single mailbox fetch misses a code that IMAP has not indexed yet](#117-a-single-mailbox-fetch-misses-a-code-that-imap-has-not-indexed-yet)

**Table rationale cell (original):** Measured on ClickHouse: Greenhouse sent the code at **08:48:11**, and #111's single fetch at **08:48:21** — ten seconds later — returned **nothing**. The agent concluded the submit was not accepted, made another attempt, and clicked a submit button that was by then **`disabled=true`** (#101's diagnostic said so), burning the 30s action timeout on an application that had actually gone through. `pendingSecurityCodeAfter` now polls on a 25s budget instead of fetching once

### 117. A single mailbox fetch misses a code that IMAP has not indexed yet (Resolved 2026-07-26)

#111 made the mailbox the acceptance signal, and deliberately used **one** fetch to keep the retry path cheap. That was too tight, and ClickHouse measured it:

| event | time |
| --- | --- |
| Attempt 2 submit | 08:48:11 |
| Greenhouse sent the code (`p5Kqsn22`) | **08:48:11** |
| #111's fetch found nothing | **08:48:21** |
| Attempt 3 clicked a `disabled=true` submit control | 08:50:10 → 30s timeout |

The email existed for ten seconds before the check ran and was still not returned — IMAP had not indexed it. So the agent concluded "not accepted", proceeded to another attempt, and clicked a button that was disabled *because the form had already been accepted*. #101's diagnostic named it exactly: `submit control: disabled=true inViewport=false, nothing covering it`.

**Fix.** `pendingSecurityCodeAfter` polls on a 25-second budget with a 5-second tick, rather than fetching once. That stays well under `waitForSecurityCode`'s 90 seconds because it runs on every retry attempt, while comfortably outlasting the lag actually observed. Tests pin both directions: a code appearing on the fourth poll is found, and a genuinely absent code still gives up inside the budget.

**Wider point worth keeping.** This is the sixth instance of the same underlying mistake in this session — reading a signal before it is available. #95 read the DOM before the submission happened, #102 read the previous attempt's `aria-invalid`, #111 read a gate that had not rendered, #113 read a field that had not rendered, #116 judged a resubmit instantly, and now #117 read a mailbox that had not indexed. **Every signal this pipeline depends on arrives late; none of them should be read once.**


---

## 116. [The post-security-code resubmit still judged the page in one instantaneous read](#116-the-post-security-code-resubmit-still-judged-the-page-in-one-instantaneous-read)

**Table rationale cell (original):** **#115 worked — `Entered the emailed security code for akuity; resubmitting` — and the verdict still failed, in the same second.** This third submit site kept the original `WaitForLoadState` + single `page.Content()` read that **#95 replaced in the other two**, so the post-code resubmit was judged before the page could answer. Fifth instance of a capability wired into one path and not the others (#65/#66→#67, #74→#75, #28→#31, #98's two prompt paths). It is also the **last step before a confirmed application**, so the most expensive place to get it wrong

### 116. The post-security-code resubmit still judged the page in one instantaneous read (Resolved 2026-07-26)

**#115 worked. The chain ran end to end for the first time, and then failed on the very last step:**

```
08:00:31 Retrieved a security code from email (subject: "Security code for your application to Akuity")
08:00:31 akuity issued a security code after the last submit — that submission was ACCEPTED
08:00:31 Security-code gate detected for akuity — waiting for the emailed code...
08:00:31 Entered the emailed security code for akuity; resubmitting
08:00:31 akuity needs manual completion ... (code entered, no confirmation)
```

**Every line in the same second**, which is the signature #95 exists to eliminate.

**Root cause.** The post-code resubmit had its own verdict, written before #95 and never updated:

```go
page.WaitForLoadState(networkidle, 10s)
if content, err := page.Content(); err == nil {
    if confirmed, reason := isSubmissionConfirmed(...); confirmed { ... }
}
```

That is exactly the code #95 replaced in `confirmOrError` and in the retry loop. There were **three** post-click verdict sites and #95 fixed two. So the resubmit was judged before the page had a chance to respond — and this is the final step between an accepted application and a confirmed one, which makes it the most expensive site of the three to have missed.

**Fifth instance of one structural pattern.** The backlog already records #65/#66→#67, #74→#75, #28→#31, and #98 needing both prompt paths. Knowing the pattern was not enough to prevent it: when I wrote #95 I checked the two sites I knew about and did not enumerate all callers.

**Fix.** The resubmit now uses `awaitSubmissionOutcome`, like the other two, and logs the reason when it does not confirm so a failure here is diagnosable rather than silent.

**Structural guard, because memory has now failed five times.** `TestNoUnpolledPostClickConfirmationChecks` pins that `isSubmissionConfirmed` appears exactly three times in `browser.go` — its declaration, its use inside `decideSubmissionOutcome`, and #89's opportunistic re-check of already-settled state at the top of a retry. A **new** call site is almost certainly a post-click verdict that should be polling, and the test says so in its failure message. This is the same species of guarantee as #84's `manualReviewErrors` list, added for the same reason: the invariant cannot depend on the next person remembering it.


---

## 115. [Greenhouse splits the one-time code across eight single-character inputs](#115-greenhouse-splits-the-one-time-code-across-eight-single-character-inputs)

**Table rationale cell (original):** **#114's diagnostic answered it in one cycle.** The real markup is `security-input-0` … `security-input-7`, **eight inputs, each `maxlength=1`, each with an EMPTY `name`**. Every prior selector looked for a single `security_code`/`security-code` field — none could match, and filling one box with an 8-character code would have failed anyway. This was the last unimplemented link between an accepted submission and a completed application

### 115. Greenhouse splits the one-time code across eight single-character inputs (Resolved 2026-07-26)

**#114 answered this within one cycle of shipping — the sixth consecutive time that "log the evidence before guessing the cause" has paid off** (#80, #96, #97, #100, #114, now this).

The diagnostic dumped every fillable input present at the moment the code could not be entered, and the answer was unmistakable:

```
id:security-input-0  label:Security code  maxlength:1  type:text  visible:true
id:security-input-1  maxlength:1  visible:true
id:security-input-2  maxlength:1  visible:true
...
id:security-input-7  maxlength:1  visible:true
```

**Eight separate inputs, one character each, every `name` attribute empty.**

Three independent reasons the old code could never have worked, and none of them were guessable:

1. The ids are `security-input-N`. Every selector looked for `security_code`, `securityCode`, `verification_code`, or `id*='security-code'` — **`security-input` contains none of those**.
2. `name` is empty on all eight, so the four name-based selectors were dead on arrival regardless.
3. Even a matching selector would have called `Fill("82taTsxA")` on a box with `maxlength=1`.

**This was the last unimplemented link** between an accepted submission and a completed application. Everything either side of it was already confirmed live: the form fills to `invalid fields: 0`, #111 recognises the acceptance from the mailbox, #93 detects the gate, and improvements #32 retrieves the real code.

**Fix.** `fillSplitSecurityCode` tries the split-box layout **first**, since it is what Greenhouse actually uses, and distributes one character per box. It requires **at least as many boxes as characters** and fills exactly `len(code)` of them: fewer boxes means this is not the widget for this code, and a partial fill would submit a **truncated** answer, which is worse than reporting failure. Three tests, including one that reassembles the code from the boxes and asserts it round-trips, and one pinning that a 4-box widget refuses an 8-character code.

The single-field path is retained and still runs when no split widget is present, so other ATS platforms are unaffected.

**Also visible in that dump, worth recording:** `g-recaptcha-response` was present on the page while this submission was **accepted** — direct confirmation of the score-based behaviour inferred in #104's correction, and of why widget presence alone must never be treated as blocking.


---

## 114. [When the emailed code cannot be entered, nothing records what IS on the page](#114-when-the-emailed-code-cannot-be-entered-nothing-records-what-is-on-the-page)

**Table rationale cell (original):** #113 proved the field is **absent, not late** — a full 20s wait found nothing (`could not find a visible security-code field to fill within 20s`). But the error can only name the selectors that *failed*, so the real field cannot be identified without reproducing an accepted submit, which means **filing a real application**. Detection fires on substrings like `security-code`, which a CSS class or notice text satisfies with no input present. Same move as #80/#96/#97/#100, each of which paid off within one cycle

### 114. When the emailed code cannot be entered, nothing records what IS on the page (Resolved 2026-07-26)

The full chain fired on Akuity at 07:00, and stopped at the last step:

```
07:00:41 Retrieved a security code from email (subject: "Security code for your application to Akuity")
07:00:41 akuity issued a security code after the last submit — that submission was ACCEPTED
07:00:41 Security-code gate detected for akuity — waiting for the emailed code...
07:01:02 Retrieved a security code for akuity but could not enter it:
         could not find a visible security-code field to fill within 20s
```

**#111 and #32 are confirmed working end to end** — the acceptance was recognised from the mailbox and the real code was retrieved. **#113 established that the field is absent rather than late**: a full 20-second poll found nothing, so the earlier instantaneous failure was not a timing artifact.

**What is still unknown, and why it cannot be guessed.** `DetectSecurityCodeChallenge` fires on a field marker **plus** matching wording, and its markers include bare substrings — `security-code`, `verification-code`. A CSS class such as `security-code-notice`, or the phrase inside an explanatory message, satisfies that with **no input on the page at all**. So the likeliest reading is that detection matched a *message about* the code rather than a field for it. Other possibilities: the input lives outside the resolved fill target's frame, or Greenhouse renders it only after a reload or a click.

Distinguishing those requires seeing the page at that moment — and reproducing it requires an **accepted submit**, which means filing a real application. That is not an acceptable way to gather evidence.

**Fix (diagnostic).** On failure to enter the code, the log now names every fillable input actually present — id, name, type, placeholder, autocomplete, maxlength, label, visibility — and says so explicitly when there are none. `no fillable inputs on the page at all` and `inputs present, none matching` are different diagnoses and the log now separates them. Best-effort: an unevaluable page yields nothing and the original error passes through unchanged.

**This is the fifth time this session the move has been "ship the diagnostic before guessing the cause"** (#80, #96, #97, #100, now #114). The previous four each identified their root cause within one cycle, including #100 catching a defect in #98 within one cycle of #100 shipping. The alternative here — guessing at selectors and re-running — costs a real application per attempt.

**Nothing else changed.** The code is still never logged (improvements #32's rule), and an unenterable code still routes to `ErrNeedsEmailVerification` → manual review with documents preserved, which is correct: the code is in the user's inbox and a human can finish in seconds.


---

## 113. [The emailed code was retrieved and then discarded, because the code field had not rendered yet](#113-the-emailed-code-was-retrieved-and-then-discarded-because-the-code-field-had-not-rendered-yet)

**Table rationale cell (original):** **One step from the first confirmed application.** Akuity's submit was ACCEPTED, the gate was detected, and #32 **successfully retrieved the code** — then `fillSecurityCode` failed instantly with `could not find a visible security-code field to fill` and the job went to manual review. Detection substring-matches the HTML; filling needs a real *visible* locator, and the input renders later than the markers. Everything happened in one second, ~11s after the submit. Same DOM-lag as #95, #102 and #111, one layer further in

### 113. The emailed code was retrieved and then discarded, because the code field had not rendered yet (Resolved 2026-07-26)

**The closest the pipeline has come to a confirmed application.** Every link in the chain fired, in order, for the first time:

```
06:30:40 Attempt 2 applied 7/7 validation fix(es)              <- form satisfied
06:30:49 Submit verdict after 8.5s: ... invalid fields: 7      <- stale flags, submit was actually ACCEPTED
06:30:51 Security-code gate detected for akuity — waiting for the emailed code...
06:30:51 Retrieved a security code for akuity but could not enter it:
         could not find a visible security-code field to fill
06:30:51 akuity needs manual completion — form is waiting on a one-time code sent by email
```

The submit was accepted. The gate was found. **improvements #32 retrieved the real code from the mailbox.** And then the code was thrown away because the input could not be filled — one step short.

**Root cause.** `DetectSecurityCodeChallenge` substring-matches the page HTML for markers like `name="security_code"` and `security-code`. `fillSecurityCode` needs something stronger: a locator that exists *and* is visible. The markers reach the HTML before the input becomes fillable, and the whole sequence above ran inside **one second**, roughly 11s after the submit. So the fill was attempted against a field that was on its way but not yet there, and it failed immediately with no retry.

**This is the same DOM-lag that produced #95** (read before the submission happened), **#102** (read the previous attempt's `aria-invalid`) **and #111** (read a gate that had not rendered) — now one layer further in, at the field rather than the gate. Fifth instance. The recurring lesson holds: **anything read from this page needs a bounded wait, not an instantaneous read.**

**Fix.** `fillSecurityCode` polls its selector list on a 20-second budget instead of failing on the first pass, and reports the budget in its error so a genuine absence is distinguishable from a slow render. The timing is a `var` so tests can compress it; one test drives a field that appears 600ms late and must still be filled, another pins that a truly absent field still errors.

**Not changed:** the retrieved code is still discarded when the field never appears, and the job still routes to `ErrNeedsEmailVerification` → manual review with documents preserved. That remains correct — the code is in the user's inbox either way, and printing it into the log is explicitly out of bounds (improvements #32 never logs the code, only the subject).


---

## 112. [The same posting exists twice, once per URL scheme, and their statuses have diverged](#112-the-same-posting-exists-twice-once-per-url-scheme-and-their-statuses-have-diverged)

**Table rationale cell (original):** Live 2026-07-27: **20 scheme-duplicate pairs** in `job_funnel`, **15 holding different statuses**. Dedup now prevents a second outward application, but discovery, queueing and reporting still operate on two independently mutable rows. Reopened by the 2026-07-26 journal/backlog sweep instead of leaving unresolved work labelled Resolved

### 112. The same posting exists twice, once per URL scheme, and their statuses have diverged (Resolved 2026-07-28)

**Found while checking a correction I had just made and got wrong.** After #111 I told the user the captcha count was overstated because Akuity was in it. Verifying that claim showed Akuity was *not* among the 9 cohort rows — because its `BLOCKED_CAPTCHA` had landed on a **different row for the same job**.

`AddToFunnel` inserts with `ON CONFLICT(url) DO NOTHING`, keyed on the raw URL. Discovery yields `https://`, while some earlier records and the 82-job verification list hold `http://`. Re-measured read-only against the live database during the 2026-07-27 post-#129 groom:

| `http://` row | `https://` row | pairs |
| --- | --- | --- |
| FAILED_SUBMIT | DISCOVERED | 5 |
| SKIPPED | DISCOVERED | 5 |
| BLOCKED_CAPTCHA | DISCOVERED | 4 |
| FAILED_SUBMIT | APPLIED | 1 |
| *(in agreement)* | | 5 |

**20 pairs, 15 diverged.** The library/backlog previously said 11; live evidence supersedes that stale count and is now the synchronization source for this row.

**Two distinct consequences, of very different severity.**

1. **Outward-facing, now fixed:** `applied_jobs` is keyed the same way, so a job recorded as applied under one scheme was **not deduped under the other** — it could be applied to twice. `HasApplied` now compares on a scheme-normalised key. Only the scheme is normalised: query strings and trailing `/apply` paths genuinely distinguish postings on Lever, and a test pins that `.../aaa-111`, `.../aaa-111/apply` and `.../bbb-222` stay separate jobs.

2. **Reporting, left open deliberately:** the cohort tally used throughout the 2026-07-25/26 session reads whichever scheme the verification file holds, so for up to 15 of 82 jobs it has been showing a **stale status** while the agent worked the other row. Every conclusion drawn from the *log* is unaffected — including the "6 of 7 completed fills were captcha-blocked" figure, which came from log lines — but the funnel status breakdown quoted during that session should be treated as approximate for those rows.

**Reopened 2026-07-26 during the application sweep.** Labelling the row Resolved while one independently mutable half remained open made “zero open bugs” false. The remaining acceptance criteria are: normalize scheme at funnel insertion and lookup boundaries; define a conservative status-merge policy that never silently converts an ambiguous outcome into `APPLIED`; migrate the 20 measured pairs transactionally with before/after reporting; preserve the strongest evidence and queue retryable conflicts for review; add tests for insertion, status updates, queue reads and migration idempotency.

**Fix 2026-07-28.** Introduced `NormalizeURL` which converts `http://` to `https://`. Applied it consistently across all database write and read boundaries in `pkg/storage/manager.go`. Added a startup migration script `migrateURLSchemes` that merges the existing `http://` records into `https://` records, combining their statuses with a conservative resolution policy that promotes `APPLIED` vs `FAILED_SUBMIT` conflicts to `MANUAL_REQUIRED`. Verified idempotency and tested resolution policy via a new test suite.


---

## 111. [#104 labelled an ACCEPTED application captcha-blocked, because the DOM lags the acceptance](#111-104-labelled-an-accepted-application-captcha-blocked-because-the-dom-lags-the-acceptance)

**Table rationale cell (original):** **A false positive in my own #104, of exactly the kind #104's guard was written to prevent.** Akuity's submit was **accepted** — Greenhouse emailed code `82taTsxA` at **05:59:19** — and the verdict at 05:59:27 still reported `BLOCKED_CAPTCHA`, because the guard tested `DetectSecurityCodeChallenge(prunedHTML)` and the code input had **not yet rendered** 8s after the click. The DOM cannot distinguish accepted from blocked on this timescale; the mailbox can, and #32's fetcher only returns codes issued after the triggering click

### 111. #104 labelled an ACCEPTED application captcha-blocked, because the DOM lags the acceptance (Resolved 2026-07-26)

**A false positive in my own #104, of precisely the kind #104's own guard was written to prevent.** I had identified this risk hours earlier, added a guard, claimed a test pinned "both directions", and it still fired.

```
05:59:19  (Greenhouse email) Security code for your application to Akuity: 82taTsxA
05:59:27  Submit verdict after 8.5s: page re-rendered with fields flagged invalid (invalid fields: 7)
05:59:27  akuity is behind a bot-protection challenge — marked BLOCKED_CAPTCHA
          (every rejected field was already set; recaptcha.net/... present)
```

The email is timestamped **eight seconds before** the verdict that called the submission blocked. It was **accepted**.

**Why the guard failed.** #104 skips its captcha verdict when `parser.DetectSecurityCodeChallenge(prunedHTML)` is true. That reads the **DOM**, and Greenhouse emails the code within ~1s while the code *input* does not appear for far longer — certainly not within the 8s settle. So the guard asked a question whose answer had not arrived yet, and the stale answer looked like "no gate".

**The mistake behind the mistake.** My test pinned the logic *given a rendered gate*; it could not pin the case where the gate has not rendered, because that case has nothing to assert against in a mock DOM. I stated the guard "pins both directions" — it pinned both directions of one branch, not both real-world situations. **A test over a mocked signal cannot establish that the signal is available when needed.**

**This is the same error class a fourth time**: #95 read the DOM before the submission happened, #102 read `aria-invalid` left over from the previous attempt, #103 read internal option ids, and now #104's guard read a DOM gate that had not rendered. Each time the fix was to stop asking the page and ask something authoritative.

**Fix.** The **mailbox** is the ground truth. `pendingSecurityCodeAfter` does **one** cheap `SecurityCodeFetcher` call per retry attempt, and that fetcher only returns codes issued *after* the click that triggered them (improvements #32's design) — so a hit means "the server accepted this submission", regardless of what the page shows. It now:

1. **blocks #104's captcha verdict** when a code is pending;
2. **triggers the #93/#32 code-entry path** on the email alone, not only on a rendered gate;
3. **reuses the code already fetched**, skipping `waitForSecurityCode`'s 90-second poll.

Fetch errors and a missing fetcher both read as "no code" rather than as acceptance, and a zero click-time never queries the mailbox — tests pin all three, plus that the query is scoped to codes issued after the click so a stale one can never be reused.

**Consequence for the run:** Akuity currently sits in `BLOCKED_CAPTCHA` while actually holding an accepted, code-pending application. It needs requeuing, and the same is likely true of any other board that reached this branch.


---

## 110. [A short option label could hijack a longer answer — "Prefer not to say" selected "No"](#110-a-short-option-label-could-hijack-a-longer-answer--prefer-not-to-say-selected-no)

**Table rationale cell (original):** **Found by a test written for #109, not by the log.** `pickComboboxOption` matched by raw bidirectional `strings.Contains`, and a short label hides inside longer prose: `"no"` sits inside `"prefer **no**t to say"`. Asking for **"Prefer not to say" selected the box labelled "No"** — on an EEO question that converts a declined answer into a substantive one on a real application. The precise failure **#79** exists to prevent, in the function that enforces it. `"male"` vs `"female"` is the same shape

### 110. A short option label could hijack a longer answer — "Prefer not to say" selected "No" (Resolved 2026-07-26)

**Found by a test written for #109, not by the log** — the live symptom was indistinguishable from an ordinary rejection, and would have stayed that way.

`pickComboboxOption` matched an option against the wanted value with raw bidirectional containment:

```go
strings.Contains(text, wantN) || strings.Contains(wantN, text)
```

Normalisation strips punctuation but not word boundaries, so a **short label hides inside longer prose**. Against Sporty Group's real option list:

| want | old rule selected | correct |
| --- | --- | --- |
| `Prefer not to say` | **`No`** — because `"no"` sits inside `"prefer **no**t to say"` | `Prefer not to say` |

`"male"` inside `"female"` is the same shape, and so is `"yes"` inside `"yesterday"`.

**This is the exact failure #79 exists to prevent, occurring inside the function that enforces it.** #79's guarantee is *never commit the wrong entry* — written after an earlier probe committed "Macomb, Illinois" for a Michigan address. Here the consequence is worse in kind than in size: on an EEO question, a declined answer silently becomes a **substantive** one, submitted under the user's name.

**Fix.** `optionTextMatches` compares **whole words in sequence**: exact equality, else one side's words appearing as a contiguous run inside the other's. Every match the old rule was written for survives, and there are tests for each — `"United States"` still matches `"United States of America"` (#79), and `"Macomb, MI, USA"` still matches `"Macomb, MI"` (improvements #33). Out-of-order and non-contiguous word sets are rejected, so `"states united"` and `"united america"` no longer match.

All six pre-existing `pickComboboxOption` tests pass unchanged, which is the evidence that the loosening this replaces was never actually needed.

**Retroactive exposure, stated because it affects work already done.** The loose matcher was live for the whole 2026-07-25/26 session and every prior one. Any single-choice question where a short option label is a substring of the intended answer could have been committed wrongly, and several of those commits happened on forms that genuinely reached Greenhouse.

**Which specific answers were affected cannot be determined.** Values were not logged until #97 shipped, and the browser state is gone. What *is* established: the four applications that reached Greenhouse (Surt AI, ClickHouse ×2, Akuity) are all **incomplete**, held at an emailed security-code challenge, so nothing has been finally filed carrying a wrong answer. That is fortunate rather than by design.

**Action for the user:** if any of those pending applications is completed by hand, check the EEO / self-identification answers rather than trusting what the agent selected.


---

## 109. [A single-choice question rendered as a checkbox group was read as one box to untick](#109-a-single-choice-question-rendered-as-a-checkbox-group-was-read-as-one-box-to-untick)

**Table rationale cell (original):** Probed live: Sporty Group renders a Yes / No / Prefer-not-to-say question as **three checkboxes sharing one `name`**. A model value of `"No"` means *tick the box labelled No*, but `applyValidationFix` read it as *untick this box* — opposite results. #107 then made the wrong reading report as **landed**, so the job degraded from `MANUAL_REQUIRED` (documents preserved, field named) to a bare `FAILED_SUBMIT`

### 109. A single-choice question rendered as a checkbox group was read as one box to untick (Resolved 2026-07-26)

Probed Sporty Group's real form after #106/#107 made its last fields settable and it *still* rejected them:

| id | label | name |
| --- | --- | --- |
| `question_8242451101[]_54236359101` | **Yes** | `question_8242451101[]` |
| `question_8242451101[]_54236360101` | **No** | `question_8242451101[]` |
| `question_8242451101[]_54236362101` | **Prefer not to say** | `question_8242451101[]` |

It is a **single-choice question rendered as a checkbox group** — three boxes sharing one `name`, each marked required. A model value of `"No"` means *tick the box labelled No*. `applyValidationFix` read it through the standalone-checkbox rule and **unticked** the box instead: the opposite outcome, and the group stayed empty so the form could never validate.

**#107 made the reporting worse while making the check more correct.** Before it, the unticked box read as not-landed and the job reached `MANUAL_REQUIRED` with documents preserved and the field named. After it, the same wrong action reported as **landed**, `lastNotLanded` was empty, and Sporty Group degraded to a bare `FAILED_SUBMIT`. Worth recording plainly: a fix that made one thing more accurate made an adjacent outcome less useful, and only the live re-run showed it.

**Fix.** When a checkbox shares its `name` with siblings, the value names *which option to tick*, resolved through the same `pickComboboxOption` path comboboxes use — so #79's never-commit-the-wrong-entry rule now covers checkbox groups too, and an unmatched value ticks **nothing** rather than guessing. `verifyFixLanded` checks the *matched* option, which is usually a different element than the selector resolved to. Standalone checkboxes keep #107's behaviour exactly, pinned by a test.


---

## 108. [A submit that went nowhere was reported as "form too large for the local model"](#108-a-submit-that-went-nowhere-was-reported-as-form-too-large-for-the-local-model)

**Table rationale cell (original):** Ethos reached **`invalid fields: 0`** — fully satisfied — exhausted the settle budget with **no bot-protection frame** to explain it, and was then written up as `form content exceeds the local model's context window`. The form was never the problem: narrowing found nothing to narrow, fell back to the whole document, and the size check caught it incidentally. Right outcome (manual review, documents preserved), wrong cause — and a wrong cause has real cost, since that is exactly how #83 misdiagnosed the case #93 later reframed

### 108. A submit that went nowhere was reported as "form too large for the local model" (Resolved 2026-07-26)

```
03:29:26 Attempt 2 applied 3/3 validation fix(es) to: #preferred_name, #question_6122095009, #question_6122097009
03:29:42 Submit verdict after 15.3s: no confirmation and no rejection evidence within the settle budget
         (url moved: false, invalid fields: 0, page 105262 chars)
03:29:42 Ethos's form is too large for the local model — queued for manual submission
```

**Every part of that last line is misleading.** The form was *fully satisfied* — `invalid fields: 0`. It was not too large; its narrowed payload had been 1,491 chars. What actually happened is that the submit produced no outcome at all, narrowing then found nothing to narrow, the code fell back to sending the whole 43,672-char document, and #105's size ceiling refused it. The size check was the last thing to touch the job, so it named the outcome.

**Distinct from #99 and #104**, which cover the same "no outcome" state *when a bot-protection frame is present*. Here the inbox showed **no Greenhouse email** and the page carried **no provider frame**, so neither explanation applies and the true cause is still unknown. That is precisely why it needs its own name rather than borrowing one that fits badly.

**Why a wrong reason is worth fixing on its own.** #83 diagnosed an oversized payload and was correct about the size while being wrong about the cause; #93 later established the payload was a *symptom* of a security-code gate. A manual-review entry is something a human acts on, and one that says "too large for the local model" invites exactly the wrong follow-up — tuning context limits — for a job whose form was already complete.

**Fix.** `ErrSubmitProducedNoOutcome` is returned when nothing is flagged invalid **and** the previous verdict was budget exhaustion, before the whole-form fallback runs. That also saves the wasted cycle: there is nothing for the model to fix, so sending the entire document could only burn inference and then be refused.

Registered in `manualReviewErrors`, which is the structural guarantee **#84** added after a sentinel shipped with no routing branch and stranded a job's documents. A test pins that it routes to manual review, that the wrapped form still does, that it is **not** conflated with `ErrFormTooLargeForModel`, and that the message states what actually happened.


---

## 107. [A checkbox the model deliberately declined was recorded as uncommittable](#107-a-checkbox-the-model-deliberately-declined-was-recorded-as-uncommittable)

**Table rationale cell (original):** Live on Sporty Group, visible only because #97 logs the value: `1 fix(es) reported success but left the control empty ... input[id='question_8242451101[]_54236360101'] **(tried "No")**`. `applyValidationFix` correctly *unchecks* on a negative, then `verifyFixLanded` reads the generic "does it hold a value" and sees `checked=false` → not landed → `ErrUncommittableField` → the whole job to manual review. **A correct answer was recorded as a failure**, on every checkbox the model declines

### 107. A checkbox the model deliberately declined was recorded as uncommittable (Resolved 2026-07-26)

**Visible only because #97 logs the attempted value.** Without it this reads as an ordinary uncommittable field:

```
03:12:53 Attempt 3: 1 fix(es) reported success but left the control empty (autocomplete/combobox suspected):
         input[id='question_8242451101[]_54236360101'] (tried "No")
03:13:01 Sporty Group needs manual completion — queued for manual submission:
         a required field could not be committed with the configured value: ... (tried "No")
```

**The two halves disagree.** `applyValidationFix` handles a checkbox correctly: a value of `false`/`no`/`0`/`unchecked` calls `Uncheck`, everything else calls `Check` — with a comment that an explicit negative "must not silently tick it". So `"No"` did exactly the right thing.

Then `verifyFixLanded` re-reads the control with the generic *does it hold a value* check, sees `checked=false`, and reports **not landed**. That routes to the combobox-commit fallback (it is not a combobox), fails, and lands in `ErrUncommittableField` → `MANUAL_REQUIRED`.

**So a correct answer is recorded as a failure, on every checkbox the model declines.** The verification never knew what was asked for; it only knew what the control now holds, and for a deliberately-unticked box those are the same thing but mean opposite outcomes. Third instance this session of a check reading state without the intent behind it (#102's stale `aria-invalid`, #103's `id|label`, now this).

**Fix.** `verifyFixLanded` takes the intended value. When that value is negative *and* the control is a checkbox or radio, unchecked **is** the requested state and counts as landed. The negative set is extracted into `isNegativeCheckboxValue`, now the single source of truth for both the action and its verification, so the two halves cannot drift apart again — which is what caused this.

**Guarded against over-matching.** A test pins that `"Nope, I have no objection"`, `"none of the above"` and `"November"` do **not** read as negative. Exact-match on the trimmed, lowercased value only: a substring rule would silently untick real answers, which is worse than the bug being fixed.

**Consequence.** Sporty Group is the clearest case — it reached **11 invalid → 3** and was sent to manual over a checkbox it had answered correctly. Documents were preserved and the field and value named, so the outcome was safe; it was simply wrong.


---

## 106. [A bare bracketed checkbox-group id got no fallbacks at all — the third shape of #73](#106-a-bare-bracketed-checkbox-group-id-got-no-fallbacks-at-all--the-third-shape-of-73)

**Table rationale cell (original):** Live: `Validation fix for "question_8242451101[]_54236360101" failed: selector matched no element (**tried 1 form(s)**)`. Greenhouse names checkbox-group controls that way; the brackets alone make `looksLikeCSSSelector` true, but there is no `tag#id` to split, so the selector was used **verbatim with no fallbacks** — and it is not valid CSS for an id either, so it matched nothing. Third shape of one defect: **#73** fixed `input#430`, **#92** fixed `#question_...[]_...`, this is the bare form with no prefix. It was the remaining blocker on Sporty Group, which reached 11 invalid → 4 with three of the four being exactly these ids

### 106. A bare bracketed checkbox-group id got no fallbacks at all — the third shape of #73 (Resolved 2026-07-26)

Caught live on Sporty Group, on the one line that names how many selector forms were attempted:

```
03:09:20 Validation fix for "question_8242451101[]_54236360101" failed:
         selector matched no element (tried 1 form(s) of "question_8242451101[]_54236360101")
```

**`tried 1 form(s)`** is the tell. A bare identifier normally gets five candidate forms; a `tag#id` selector gets three. One means the selector was used verbatim and nothing else was attempted.

**Root cause.** `resolveFieldLocator` branches on `looksLikeCSSSelector`. Greenhouse names checkbox-group controls `question_8242451101[]_54236360101`, and the `[`/`]` alone are enough to make that predicate true — so the bare-identifier fallbacks are skipped. It then tries `splitTagID`, which finds no `#` to split on, so **that** branch adds nothing either. The result is a selector that is simultaneously "too CSS-like" for the identifier path and "not CSS enough" for the tag path, and it falls through both with zero fallbacks. It is also not valid CSS for an id, so the verbatim attempt matches nothing.

**Third shape of a single defect.** #73 fixed `input#430` (an id starting with a digit, used verbatim). #92 fixed `#question_...[]_...` (bracketed, with a `#` prefix, blocked because `splitTagID` refused bracketed ids). This is the same control class again with **no prefix at all** — the one arrangement neither previous fix covered.

**Fix.** When the selector looks like CSS but has no `tag#id` to split, append the attribute forms built from the whole selector. Safe for genuine CSS selectors: the candidates are appended *after* the verbatim attempt, and an attribute form built from a real selector simply matches nothing. A test pins that `input[type='email']` still resolves on the first candidate and tries nothing else.

**Consequence.** This was the remaining blocker on Sporty Group, which reached **11 invalid → 4** with three of the four survivors being exactly these bracketed ids. Verified failing against the old code before the fix was kept.


---

## 105. [The 45-minute time budget counted bytes to read, not answers to generate](#105-the-45-minute-time-budget-counted-bytes-to-read-not-answers-to-generate)

**Table rationale cell (original):** The `Remote` job sent a **30,477-char** payload — comfortably inside #83's 40,000 ceiling — and burned the **entire 45-minute Ollama timeout** (01:46:03 → 02:31:03) before failing. #83 derived its ceiling from input size alone, but the run must *generate* a value for every rejected field, and **Remote had 34 of them**. Against ClickHouse (11,140 chars / 3 fields / ~7 min) and Reddit (18,639 / 13 / ~15 min), field count — not payload size — is what separates the runs that finish

### 105. The 45-minute time budget counted bytes to read, not answers to generate (Resolved 2026-07-26)

The single most expensive failure mode in this pipeline, recurring after #83 was supposed to have closed it:

```
01:46:01 Narrowed validation retry to the rejected fields only (79608 -> 19481 chars); still invalid: <34 fields>
01:46:03 SolveValidationErrors API Call #16 executed. Payload length: 30477 characters.
02:31:03 Auto-Submit failed for Remote: ... context deadline exceeded
```

**45 minutes exactly**, then nothing. And the payload was **30,477 chars against a 40,000 ceiling** — it passed the guard #83 added specifically to prevent this.

**Why #83's model was incomplete.** It derived the ceiling from *reading* cost: ~7 tok/s × ~2.5 chars/token ≈ 17.5 chars/s, so 45 minutes buys ~47,000 chars, and 40,000 was set as the margin. That accounts for the prompt going in. It does not account for the answers coming out — and `SolveValidationErrors` must generate a value for **every** rejected field.

Three live data points on this hardware separate cleanly on field count, not size:

| job | payload | fields | outcome |
| --- | --- | --- | --- |
| ClickHouse | 11,140 | 3 | ~7 min, completed |
| Reddit | 18,639 | 13 | ~15 min, completed |
| **Remote** | **30,477** | **34** | **45 min, timed out** |

Remote's payload is 1.6× Reddit's, but its field count is 2.6× — and it did not merely take longer, it failed to finish at all.

**Fix.** `exceedsRetryTimeBudget` adds a field-count ceiling (**20**) alongside the character ceiling, and the character ceiling drops **40,000 → 28,000**, below the payload that was observed to fail. A retry over either limit routes to `ErrFormTooLargeForModel` → `MANUAL_REQUIRED`, which preserves the tailored documents for a human instead of spending 45 minutes to preserve nothing.

Applied to the **retry** call site only. The initial `ExtractFormMapping` path keeps the size-only guard: it is not answering a list of rejected fields, so the field-count reasoning does not apply to it, and tightening it without evidence would refuse forms that currently work.

A test pins the ceiling below the observed 30,477-char failure, so any future widening has to argue with the measurement rather than silently regress past it — the same treatment #83's own corrected test case got.

**Known limitation, stated rather than hidden: the field ceiling of 20 is interpolated, not measured.** The evidence brackets it loosely — 13 fields completed in ~15 min, 34 fields did not complete in 45 — and there is **no data point between 13 and 34**. So a form with, say, 22 fields might well have succeeded and will now be routed to `MANUAL_REQUIRED` instead. That is the deliberate direction to err: a wrongly-refused form costs one manual completion with its documents intact, while a wrongly-accepted one costs 45 minutes of the machine's only inference capacity and produces nothing. The number should be revisited the first time a refused form's real field count and timing can be measured, and this note is the reason to revisit it rather than treat 20 as established.


---

## 104. [A captcha-swallowed submit hid behind stale invalid flags, so #99 never fired](#104-a-captcha-swallowed-submit-hid-behind-stale-invalid-flags-so-99-never-fired)

**Table rationale cell (original):** Predicted from #99+#102, then confirmed by #100's diagnostic on the next run. Reddit job `7956443` set all five custom questions to sensible values (`"company website"`, `"Stellantis Financial Services"`, `"Yes"`, `"No"`, `"I agree"`), committed all three comboboxes, and the **identical five** came back flagged with the page **byte-for-byte unchanged** (140544 chars twice). Nothing was left to fix — the submit was never reaching the server past the page's reCAPTCHA. #99 could not catch it because the verdict settles on flagged fields and never reaches budget exhaustion

### 104. A captcha-swallowed submit hid behind stale invalid flags, so #99 never fired (Resolved 2026-07-26)

**Predicted before it was seen, then confirmed by measurement** — which is worth noting because the previous prediction this session (#103's causal claim) was wrong and had to be retracted.

After #102, the reasoning was: a reCAPTCHA-swallowed submit leaves the page untouched, so the previous attempt's `aria-invalid` markers persist, so the verdict settles as `reasonFieldsFlagged` at the 8s floor and **never reaches budget exhaustion** — which is the only state #99's bot-protection branch tests. Same structure as #102, with a captcha in place of the code gate.

The next run produced exactly that, on Reddit job `7956443`:

```
00:51:48 Attempt 2 committed 3 autocomplete selection(s): #question_67179376, #question_67179377, #question_67179378
00:51:48 Attempt 2 applied 5/5 validation fix(es)
00:51:56 Submit verdict after 8s: page re-rendered with fields flagged invalid (url moved: false, invalid fields: 5, page 140544 chars)
00:51:56 Rejected despite being set last attempt: question_67179374 = "company website";
         question_67179375 = "Stellantis Financial Services"; question_67179376 = "Yes";
         question_67179377 = "No"; question_67179378 = "I agree"
```

Every value is sensible and correctly committed. The identical five come back flagged. And the page is **byte-for-byte unchanged** — 140544 chars on this run and on the previous one, 133352 at both initial submits. A server that had processed and re-rejected the form would not return the same bytes twice.

**This also cleanly confirms #103's fix and its retraction.** The values are now human-readable labels (`"Yes"`, not `react-select-…-option-0|Yes`), so #103 works — and the rejection is *unchanged*, so #103 was correctly retracted as the cause.

**Fix.** When every still-rejected field was successfully written by the previous attempt, there is nothing left for the model to fix; the form is rejecting values it already holds. Combined with a bot-protection widget on the page, that is a swallowed submission, and it now returns `ErrCaptchaBlocked` at the top of the retry rather than spending a third ~10-minute model call to re-answer questions that were already answered correctly.

**Deliberately requires ALL of them, not merely some.** A single genuinely-bad answer among several is an ordinary validation failure and keeps its remaining retry; a test pins that one unset field prevents the captcha verdict.

**Correction, and it strengthens the constraint rather than weakening it.** This entry originally justified that with "every Greenhouse page carries reCAPTCHA". That was an assumption, and it is **wrong**. Measured across three boards the same night:

| board | reCAPTCHA frame | submit outcome |
| --- | --- | --- |
| `greenhouse.io/reddit` | **present** | blocked |
| `greenhouse.io/clickhouse` | **absent** | accepted |
| `greenhouse.io/akuity` | **present** | **accepted** (code email 23:40:07) |

reCAPTCHA Enterprise is score-based, so **Akuity carries the widget and submits fine**. The claim was wrong; the caution it was defending is now *empirically* proven right, which is a better footing than the assumption gave it. Presence of a provider frame means nothing on its own — only the conjunction with "nothing left for the model to fix" is evidence.

**Follow-up defect found by that same measurement, before it caused harm.** Akuity is precisely the case that breaks this: an accepted submission whose post-acceptance click times out (`element is not enabled`) on a page that *does* carry reCAPTCHA. Because #104's check sits **above** #93's security-code handling in the retry loop, it would have labelled an accepted application `BLOCKED_CAPTCHA`. That is #102's rule — acceptance outranks any rejection signal — reintroduced by the very fix written after learning it. The condition now tests `DetectSecurityCodeChallenge` itself rather than relying on ordering, with a test pinning both directions.

Same discipline as #99's iframe-`src` narrowness, for the same reason: #45/#46 were captcha false positives that killed most jobs on this platform.


---

## 103. [#98 showed the model react-select's internal option ids, and it answered with them](#103-98-showed-the-model-react-selects-internal-option-ids-and-it-answered-with-them)

**Table rationale cell (original):** Yes` — an internal DOM id no widget offers. Live: `Rejected despite being set last attempt: question_67179376 = "react-select-question_67179376-option-0\ | — | — | Yes"`. So #98 has been feeding garbage to the model since it shipped, on every combobox

### 103. #98 showed the model react-select's internal option ids, and it answered with them (Resolved 2026-07-26)

**#100's diagnostic caught this within one cycle of #100 itself shipping** — the fourth time an observability fix in this session paid for itself immediately (#80, #96, #97, now #100).

The first job to run under the new binary produced:

```
00:31:55 Rejected despite being set last attempt:
  question_67179374 = "company website";
  question_67179375 = "Stellantis Financial Services";
  question_67179376 = "react-select-question_67179376-option-0|Yes";
  question_67179377 = "react-select-question_67179377-option-1|No";
  question_67179378 = "react-select-question_67179378-option-0|I agree"
```

The model answered three fields with **react-select's internal DOM option ids**.

**Root cause, and it is mine.** `readComboboxOptions` deliberately returns each entry as `"id|label"` — `pickComboboxOption` needs the id so it can click the right option, which is how #79's "never commit the wrong entry" guarantee is enforced. #98's `enumerateComboboxOptions` reused that helper and rendered its output straight into the prompt, so the block told the model that `react-select-question_67179376-option-0|Yes` was a permitted value. It copied it exactly, as #98 instructed it to ("copied exactly, character for character").

So #98 — the fix whose whole purpose was to stop the model guessing wording it had never been shown — was **showing it wording no human could choose**, on every combobox, from the moment it shipped.

**Fix.** `optionLabel` strips the `id|` prefix, so only the human-readable text reaches the prompt. Entries with no separator (Lever's shape) pass through unchanged, and a label that itself contains a pipe keeps everything after the *first* separator. Two tests: one on the splitting, and an end-to-end one asserting no `react-select-` or `option-N` string can appear in the generated block at all.

**Correction, made the same night: this is a real defect but NOT proven to be the cause of that job's rejection.** After filing, I read `pickComboboxOption` properly. It splits each entry on `|` and matches the model's value against the **label** bidirectionally — `strings.Contains(text, wantN) || strings.Contains(wantN, text)`. Normalisation strips punctuation, so `react-select-question_67179376-option-0|Yes` becomes a string *ending* in `yes`, and the reverse-containment arm still matches the `Yes` option. The same holds for the `No` and `I agree` cases. **So the correct option may well have been committed regardless**, and something else is rejecting those five fields.

What remains unambiguously true: the model was being handed internal identifiers as if they were permitted values, which is wrong on its own terms, defeats the stated purpose of #98, and only worked by accident of a containment rule that was never designed to carry it. The fix stands. The causal claim does not, and the live re-run is what will settle it.

**Why it was invisible before.** #97 names values only for fields that fail to land; these three *did* land (`committed 3 autocomplete selection(s)`), because `setComboboxValue` types the value, gets zero matches, and #91's clear-and-re-read then commits *something*. Only #100 — written for exactly the "reported as set, rejected anyway" case, and shipped before its own root cause was known — could surface the value that was actually written.


---

## 102. [#95's early exit read stale invalid flags and called four accepted submissions failures](#102-95s-early-exit-read-stale-invalid-flags-and-called-four-accepted-submissions-failures)

**Table rationale cell (original):** **A defect in my own #95 fix, and the biggest single misreading of the effort.** Greenhouse *accepts* a submission and issues an emailed security-code challenge within ~1s, then re-renders the challenge later while the previous attempt's `aria-invalid` markers are **still on the page**. #95's flagged-field early exit fired at 2s on those stale markers and called the accepted submission a validation failure. Proven by timestamps: the Akuity code email is stamped **23:40:07, between** its submit click (~23:40:06) and its verdict (23:40:08); ClickHouse's is stamped **00:05:34, the same second** as its submit. **Four applications reached Greenhouse today** (Surt AI, ClickHouse ×2, Akuity) and every one was recorded as a failure

### 102. #95's early exit read stale invalid flags and called four accepted submissions failures (Resolved 2026-07-26)

**A defect in my own #95 fix, found the same way #93 and #95 were: by reading the inbox rather than the logs.**

Four Greenhouse security-code emails exist for 2026-07-25/26. Lined up against the log, the timestamps are decisive:

| code email (EDT) | job | log |
| --- | --- | --- |
| 16:58:03 | Surt AI | the original #93 case |
| 21:15:58 | ClickHouse | `21:15:58 applied 3/3` → `Submission failed validation` |
| **23:40:07** | **Akuity** | `23:40:05 applied 7/7` → **`23:40:08 verdict: invalid fields: 7`** |
| **00:05:34** | **ClickHouse** | `00:05:34 applied 3/3` → **`00:05:36 verdict: invalid fields: 3`** |

Akuity's email is timestamped **between** its submit click (~23:40:06) and its verdict (23:40:08). ClickHouse's is stamped the **same second** as its submit. In both cases the server had already accepted the application before the agent declared it failed.

**Root cause, and it is mine.** #95 replaced an instantaneous read with a bounded poll, and treated "fields flagged `aria-invalid` past a 2s floor" as *positive evidence of rejection* — reasoning that a re-rendered form with flagged fields proves the server answered and refused. That reasoning is wrong on Greenhouse: it accepts the submission, issues the code challenge, and **leaves the previous attempt's `aria-invalid` markers in place** while the challenge renders. Both signals are true simultaneously, and #95 was reading the stale one.

**This is the same trap, third time.** #76 read `el.value` that was really the artifact of typing; #81 read `[data-value]` that was really the search text; #102 reads `aria-invalid` that is really the previous attempt's leftover. Each time a signal that looked like evidence was a residue of the prior step. That the pattern recurred *in a fix written specifically to stop misreading post-submit state* is the part worth remembering.

**Why #93's detector never rescued it.** `DetectSecurityCodeChallenge` runs at the top of the *next* attempt. ClickHouse's attempt 3 began 2s after the submit and went straight to `SolveValidationErrors`, so the challenge had not rendered even then. The detector was correct; it was asked too early, twice.

**Fix.**
1. The security-code gate is now tested **inside** the verdict, on every poll, and **before** the flagged-field branch — acceptance beats a stale rejection marker by construction, not by timing luck.
2. `submitOutcomeSettleFloor` raised **2s → 8s**. Akuity's verdict at 2.2s missed a challenge that had already been issued; the floor now leaves room for one that renders late.
3. A gate verdict is routed to the existing #93/#32 path (retrieve the emailed code, enter it, resubmit) and explicitly **not** to #99's bot-protection branch, which would otherwise mislabel an accepted submission as captcha-blocked.

The rejection signal is deliberately preserved when no gate is present — a test pins that, because removing it entirely would leave every genuine validation failure exiting on budget exhaustion, which #99 maps to `BLOCKED_CAPTCHA` on any page carrying reCAPTCHA. That interaction would have traded one misreading for another. 4 new tests.

**Consequence.** The pipeline submitted **four** applications today and recorded all four as failures. "0 confirmed `APPLIED`" was never a submission problem; it was this.


---

## 101. [A submit click that timed out reported nothing about what blocked it](#101-a-submit-click-that-timed-out-reported-nothing-about-what-blocked-it)

**Table rationale cell (original):** **Three jobs ended the day this way** — Akuity, Nova and Zimperium — each with a bare `playwright: timeout: Timeout 30000ms exceeded` from the submit click and no indication of why the control was unactionable, all written off as generic `FAILED_SUBMIT`. A timeout says the click never landed; it says nothing about what stopped it. Now reads `elementFromPoint` at the control's centre — the same check that cleared Reddit's button in #99 — and names whatever covers it

### 101. A submit click that timed out reported nothing about what blocked it (Resolved 2026-07-25)

Three separate jobs ended 2026-07-25 with the identical, uninformative line:

```
23:48:08 [Worker-1] Auto-Submit failed for Akuity: playwright: timeout: Timeout 30000ms exceeded.
```

`grep` confirms the count: **Akuity, Nova and Zimperium**, each after a full set of validation fixes had been applied, each written off as a generic `FAILED_SUBMIT`. Akuity's is the clearest waste — attempt 3 applied 7/7 fixes and then spent the whole 30s action timeout failing to click.

**A timeout says the click never landed. It says nothing about what stopped it** — a disabled control, an off-screen one, a consent banner over the top (#34's shape), or a bot-protection frame (#99's). Those need different responses and were indistinguishable.

The 07-21 journal had already guessed at Nova's: *"most plausibly an hCaptcha overlay after repeated submits."* A guess is what this replaces.

**Fix.** On a failed submit click, read the control's actionability directly via `elementFromPoint` at its centre — the same check a read-only probe used to *clear* Reddit's button in #99 — and log it:

```
[Auto-Submit] Submit click failed for Akuity (timeout ...); submit control: disabled=false inViewport=true, covered by DIV#onetrust-consent-sdk.banner
```

Naming the covering element turns three indistinguishable timeouts into three separable causes. When an iframe covers it, its `src` is included, so a challenge frame identifies itself.

**Routing stays evidence-led.** `ErrCaptchaBlocked` is returned only when a provider frame is genuinely embedded, reusing #99's narrow `src` matcher. #45/#46 were CAPTCHA false positives that killed the large majority of Greenhouse/Lever/Ashby/Workable jobs before fit-scoring, so nothing here infers a captcha from a timeout alone — the timeout and the widget must both be present, and the message states both facts rather than asserting one caused the other. The original Playwright error is preserved in the wrapped message either way.

The probe is best-effort: a page that cannot be evaluated yields no description and the original error is returned unchanged, so a diagnostic can never break the failure path it exists to explain. 5 new sub-cases plus a best-effort test.


---

## 100. [A field that lands and is rejected anyway had no diagnostic at all](#100-a-field-that-lands-and-is-rejected-anyway-had-no-diagnostic-at-all)

**Table rationale cell (original):** Akuity logged `applied 7/7`, **no** not-landed line — so `verifyFixLanded` reported every control as set — and the **identical 7 fields** came back flagged. #97 names values only for fields that *fail* to land, so this opposite case had no diagnostic whatsoever and the loop could only re-guess. Probe ruled out the obvious causes: all 7 are plain required `INPUT`/`TEXTAREA`, single match, no `pattern`, and React genuinely *does* observe `Fill()` (`reactValue` matches the DOM). Fourth instance of the same lesson as #80/#96/#97

### 100. A field that lands and is rejected anyway had no diagnostic at all (Resolved 2026-07-25)

Akuity produced a signature none of the existing diagnostics could speak to:

```
23:40:05 Attempt 2 applied 7/7 validation fix(es) to: input#question_6039579009 ... textarea#question_6051662009
23:40:08 Submit verdict after 2.2s: page re-rendered with fields flagged invalid (url moved: false, invalid fields: 7, page 126557 chars)
23:40:08 Narrowed validation retry ... still invalid: question_6039579009, ... question_6110764009   [the identical 7]
```

**No not-landed line**, so `verifyFixLanded` reported all seven controls as genuinely set — and the form rejected the same seven anyway. #97 names the attempted value only when a fix *fails to land*; the opposite case had no diagnostic whatsoever, so the log could not say what had been written or why it was refused, and the loop could only re-guess.

**Probing ruled out every convenient explanation.** All seven are plain required `INPUT`/`TEXTAREA` (`LinkedIn Profile*`, `Github*`, three free-text questions), each a **single** DOM match, no `pattern`, no `minLength`. The React-controlled-input trap does **not** apply either — reading React's own props alongside the DOM after a plain `Fill()`:

| control | DOM value | React prop value |
| --- | --- | --- |
| `question_6039579009` (input) | `https://linkedin.com/in/probe` | **same** |
| `question_6110764009` (input, real keystrokes) | `https://github.com/probe` | **same** |
| `question_6051659009` (textarea) | set correctly | uncontrolled — DOM is the source of truth |

So React observes `Fill()`, and the values genuinely land. One structural note worth keeping: these controls have **no `name` attribute**, so `FormData` serialises nothing for them — Greenhouse submits from React state, which is why "the DOM says it is set" is not by itself proof the submission carries it.

**What the diagnostic adds.** `rejectedDespiteLanding` pairs each still-rejected id with the value the previous attempt wrote into it:

```
[Auto-Submit] Rejected despite being set last attempt: question_6039579009 = "https://..."; question_6051659009 = "Ran production Kubernetes..."
```

Matching is by suffix because the two sides name controls differently — the model emits `input#question_1`, `#question_1` or `input[id='question_1']`, while `parser.InvalidFieldIdentifiers` reports the bare id (the same selector-shape problem as **#73** and **#92**). A test pins that `question_60395790091` and `question_6039579` do **not** match `question_6039579009`, so a prefix collision cannot mis-attribute a value. Free-text answers are whitespace-collapsed and truncated so one answer stays on one log line.

**Fourth instance of one lesson** (#80, #96, #97, now #100): every expensive failure in this pipeline is *"the mechanism reported success and the outcome was failure"*, and each time the fix has been to log the evidence a decision rested on. The root cause of Akuity's rejection is **still open** — this makes the next occurrence diagnosable rather than guessing now.


---

## 99. [A submit silently swallowed by reCAPTCHA was reported as an ordinary validation bounce](#99-a-submit-silently-swallowed-by-recaptcha-was-reported-as-an-ordinary-validation-bounce)

**Table rationale cell (original):** Reddit reached **`invalid fields: 0`** — the form fully satisfied for the first time — and the submit still produced no confirmation and no rejection. **No Greenhouse email arrived**, while ClickHouse's accepted submit produced one in the same second, so Reddit's request never reached the server. Read-only probe: the submit control is **clean** (one match, visible, enabled, in-form, unobstructed — ruling out #87's decoy and #34's overlay) and the page embeds **reCAPTCHA Enterprise**. Score-based invisible reCAPTCHA silently discards a headless submission. The cost was ~30 min of model calls per attempt on a form with nothing left to fix, ending in a misleading manual-review reason

### 99. A submit silently swallowed by reCAPTCHA was reported as an ordinary validation bounce (Resolved 2026-07-25)

**Found by an outcome that could not be either of the two things the code knew about.** With #98 shipped, Reddit's form was fully satisfied for the first time in the entire effort:

```
23:19:28 Attempt 2 committed 9 autocomplete selection(s): 430, 431, 432, 433, 434, 436, question_67942418, question_67942419, question_67942420
23:19:28 Attempt 2 applied 13/13 validation fix(es)
23:19:43 Submit verdict after 15.1s: no confirmation and no rejection evidence within the settle budget (url moved: false, invalid fields: 0, page 169636 chars)
```

`invalid fields: 0` with no confirmation and no rejection, after a full 15s wait. **#95's budget-exhaustion branch existed precisely to represent this state honestly instead of guessing**, and #96's line is what made it legible.

**The inbox discriminated the two cases.** ClickHouse's accepted submit produced a Greenhouse security-code email *in the same second*; Reddit's produced **nothing**. So Reddit's request never reached the server, where ClickHouse's plainly did.

**Read-only probe of the submit control** (never clicking it — that files a real application):

| check | result |
| --- | --- |
| matches for `button[type='submit'], input[type='submit']` | **1** |
| identity | `BUTTON type=submit "Submit application"`, 190×40 |
| visible / enabled / inside a `<form>` | yes / yes / yes |
| `document.elementFromPoint` at its centre | **the button itself — nothing covering it** |

That rules out **#87** (a decoy control winning precedence) and **#34** (an overlay intercepting the click). What the probe did find:

```
https://www.recaptcha.net/recaptcha/enterprise/anchor?ar=1&k=6LfmcbcpAAAAAChNTbhUShzUOAMj_wY9LQIvLFX0
```

**reCAPTCHA Enterprise, score-based and invisible** — no `.g-recaptcha` marker, just the anchor frame. A headless Chromium scores badly and the submission is discarded client-side: no error, no navigation, no request, no email. Every observation follows from that, including why ClickHouse (no such widget) went through.

**Fix, and what it deliberately is not.** Solving CAPTCHAs is `improvements_paywall.md` #17 — paid, user-gated, and out of scope. What is free is *reporting the truth*: when a submit exhausts the settle budget with no outcome **and** the page carries a live bot-protection widget, that is `ErrCaptchaBlocked` (already wired to `BLOCKED_CAPTCHA`), not a validation bounce. Previously this burned another ~15-minute model call, then fell back to the whole form and landed in manual review via #83's size ceiling — with a reason that named the wrong cause entirely.

**The detection is narrow on purpose, and the narrowness is the point.** It matches the **iframe `src`** of known providers, never page wording. Bugs **#45/#46** were CAPTCHA false positives from phrase matching, and between them they killed the large majority of Greenhouse/Lever/Ashby/Workable jobs before they ever reached fit-scoring. A false positive here costs a real application, so this only reports what it can point at, and it only runs *after* a submit has already produced no outcome — it can never pre-empt a working job. The provider pattern is a single Go constant interpolated into the browser check **and** compiled in the test, so the test exercises the real pattern rather than a copy that can drift. 9 sub-cases, including a `google.com` URL that is not reCAPTCHA.

**Consequence worth stating plainly:** Reddit is not completable by this pipeline for free. It is now correctly labelled `BLOCKED_CAPTCHA` in ~15 seconds instead of ~30 minutes.


---

## 98. [The model was never shown a dropdown's permitted values, so it guessed the wording](#98-the-model-was-never-shown-a-dropdowns-permitted-values-so-it-guessed-the-wording)

**Table rationale cell (original):** **The last-mile blocker, caught by #97's diagnostic in one cycle.** Reddit reached a single remaining invalid field and proposed `"I am not a protected veteran"` on **two consecutive attempts** for a widget offering *No military service* / *I don't wish to answer* — a phrasing that filters the option list to **zero**. Probe: none of `#434`'s option strings exist in the page HTML until the widget is opened, and Greenhouse forms carry **zero native `<select>` elements**, so no option text is ever in the served document the prompt is built from. The model was asked to supply a value for a control whose permitted values it is never shown. Residual measured since #91/#92: **14 commits succeeded, 2 fields failed** — and both failures are exactly this unusual-wording case

### 98. The model was never shown a dropdown's permitted values, so it guessed the wording (Resolved 2026-07-25)

**The last-mile blocker, and #97's diagnostic produced it in exactly one cycle** — the same way #80 paid for itself.

Reddit got to a single remaining invalid field and then stalled on it twice with the value now visible:

```
22:41:35 Attempt 2: 1 fix(es) ... left the control empty: 434 (tried "I am not a protected veteran")
22:42:39 Attempt 3: 1 fix(es) ... left the control empty: #434 (tried "I am not a protected veteran")
```

The **identical** wrong value on both attempts — the model is deterministic here and would never have converged, however many retries ran.

**Root cause, established by probe.** `#434` is *Are you a veteran/have you served in the military?* and offers:

> Active Reserve · Inactive Reserve · Other Protected Veteran · Retired · Unspecified Veteran · Vietnam Era Veteran · Vietnam Veteran and Other Protected Veteran · No military service · I don't wish to answer

None of those strings appear in the page HTML until the widget is opened:

| string | in page HTML before opening | after opening |
| --- | --- | --- |
| `Other Protected Veteran` | **false** | true |
| `No military service` | **false** | true |
| `I don't wish to answer` | **false** | true |

Opening one widget grows the document 144,506 → 146,812 chars, and the form contains **zero native `<select>` elements** — Greenhouse is react-select throughout, so `<option>` text is never in the served markup. The narrowed validation payload is built from that markup. **The model was being asked to supply a value for a control whose permitted values it is never shown**, leaving it to invent plausible wording. `"I am not a protected veteran"` is a perfectly reasonable guess; it just is not on the menu, and typing it filters the list to nothing.

**This explains the shape of the entire day.** Yes/No fields committed reliably because they are trivially guessable. Measured over the window since #91/#92 shipped: **14 autocomplete commits succeeded and 2 distinct fields failed** — `#434` and Sporty Group's `#question_7849575101` — and both failures are this same unusual-wording case. Nothing else in that window failed to commit.

**Fix.** `enumerateComboboxOptions` opens each invalid control that is genuinely a combobox, reads its real option list with **no query typed** (bugs.md #91: typing filters the list, and an unrecognised query filters it to nothing — the very state this is meant to reveal rather than reproduce), closes it again, and renders a block into the prompt naming the exact permitted values and instructing that they be copied character for character.

**Wired into both prompt paths**, the validation retry *and* `ExtractFormMapping`. That is deliberate: the standing check in this backlog records three prior instances of a capability added to one path and not the other (#65/#66→#67, #74→#75, #28→#31), and this is the same shape.

**The `isComboboxLocator` gate is a correctness requirement, not an optimisation.** The invalid-field list routinely includes checkboxes — Greenhouse's GDPR consent among them — and clicking one would *toggle* it, silently changing the answer this function exists to get right. A test pins that a non-combobox is never clicked.

Bounded at 25 controls and 40 options each, and the block is added *before* `likelyExceedsModelContext` runs, so the #83 time-budget ceiling still accounts for it. 3 new tests.


---

## 97. [An uncommittable field named the control but never the value that was tried](#97-an-uncommittable-field-named-the-control-but-never-the-value-that-was-tried)

**Table rationale cell (original):** Reddit reached **one remaining invalid field** — payload 7,212 → 497 chars, 13 fields down to 1 — and then failed twice on `#434` (veteran status) with the log saying only that the control was left empty. A probe shows `#434` is genuinely selectable (typing `I don't wish to answer` filters its 9 options to exactly that entry), so this is a **value mismatch, not a broken mechanism** — but the log could not distinguish those, and they need opposite fixes. Same class as #80/#96, one level down

### 97. An uncommittable field named the control but never the value that was tried (Resolved 2026-07-25)

**Found at the closest the pipeline has ever come to finishing.** Reddit (fit 90, the job that opened this whole investigation) went from 13 invalid fields to **one**, with the narrowed payload collapsing **7,212 → 497 chars**:

```
22:11:40 Attempt 2 committed 8 autocomplete selection(s) ... #430, #431, #432, #433, #436, #question_67942418, #question_67942419, #question_67942420
22:11:40 Attempt 2: 1 fix(es) reported success but left the control empty ... #434
22:11:43 Narrowed validation retry to the rejected fields only (54218 -> 497 chars); still invalid: 434
22:12:39 Reddit needs manual completion ... a required field could not be committed with the configured value: #434
```

Eight autocompletes committed, including both legal attestations. One field stood between this and the first confirmed application of the entire effort, and the log said only that `#434` came back empty.

**That is not enough to act on.** "Left the control empty" is consistent with two opposite situations: the commit machinery is broken for this widget, or the machinery works fine and was handed a value the widget does not offer. The first needs a code fix, the second needs different data or a different prompt. Nothing in the log separated them.

**A probe settled it — the mechanism is fine.** `#434` is *Are you a veteran/have you served in the military?*, a react-select offering:

> Active Reserve · Inactive Reserve · Other Protected Veteran · Retired · Unspecified Veteran · Vietnam Era Veteran · Vietnam Veteran and Other Protected Veteran · No military service · I don't wish to answer

Typing `I don't wish to answer` filters that list to **exactly that one entry**, so the control is genuinely selectable and #90/#91's sole-option path would commit it. Two other plausible phrasings (`Prefer not to say`, `I am not a protected veteran`) filter it to **zero** — the #91 shape, where the typed query eliminates every option. So this is a **value mismatch**, and the missing datum was always the value.

**Fix.** The not-landed entry now carries it: `#434 (tried "…")`. It flows into the `ErrUncommittableField` message too, so the manual-review entry and the log both name the exact string that failed. 1 new test.

Deliberately not done in the same change: making the model's answer converge on the offered wording. That is a real follow-up, but choosing it blind — before a single log line shows what the model actually proposes — is how #90 shipped a rule that #91 proved could never fire. Measure first.

**Third instance of the same lesson in one session** (#80, #96, #97): this pipeline's expensive failures are all *"the mechanism reported success and the outcome was failure"*, and each time the fix has been to log the evidence a decision rested on rather than the decision itself.


---

## 96. [Nothing recorded what a submit verdict was actually decided on](#96-nothing-recorded-what-a-submit-verdict-was-actually-decided-on)

**Table rationale cell (original):** Observability, filed the moment its absence cost a day. #95 was findable only by cross-referencing wall-clock seconds between unrelated log lines, because the sole record of a judged submit was the word "failed" — nothing said how long it waited, whether the URL moved, how many fields came back flagged, or how large the returned page was. Same class as #80, and #80 paid for itself within one cycle

### 96. Nothing recorded what a submit verdict was actually decided on (Resolved 2026-07-25)

**Filed the moment its absence had cost a day.** The only trace of a judged submit was `Submission failed validation. Retrying...`. That line says a verdict was reached and nothing about the evidence behind it — not how long the code waited, not whether the URL moved, not how many fields came back flagged, not how large the returned page was. #95 was consequently findable only by noticing that four *unrelated* log lines shared a wall-clock second, then reasoning backwards from that coincidence.

`awaitSubmissionOutcome` now emits one line per submit click at the moment it settles:

```
[Auto-Submit] Submit verdict after 2s: page re-rendered with fields flagged invalid (url moved: false, invalid fields: 13, page 54537 chars)
```

Every field in it distinguishes cases that were previously indistinguishable: elapsed time separates a premature read from a settled one, `url moved` separates an in-place re-render from a navigation, the flagged count separates "the same fields again" from "different fields now", and page size catches the #83/#93 oversized-payload shape.

**Confirmed working within minutes of shipping**, on Reddit: `Submission failed validation (page re-rendered with fields flagged invalid)` at 21:56:22 against a fill completing at 21:56:20 — two seconds, not the same second, and settled on positive rejection evidence rather than an empty read. That is #95's settle floor behaving exactly as designed, and it is visible in the log rather than inferred.

Same class as **#80**, which was also filed the moment the diagnostics ran out and paid for itself within one cycle. The standing lesson is that this pipeline's expensive failures are all "the mechanism reported success and the outcome was failure", and the only defence is logging the evidence a decision was made on, not just the decision.


---

## 95. [The submit verdict was read from the DOM the instant the click returned, racing the submission itself](#95-the-submit-verdict-was-read-from-the-dom-the-instant-the-click-returned-racing-the-submission-itself)

**Table rationale cell (original):** Three independent jobs (ClickHouse, Stack AV, Sporty Group) logged **every field committed, every fix applied, and `Submission failed validation` in the same second**. A probe proved those forms are fully satisfiable — after the agent's exact commit sequence ClickHouse's form reports `invalidCount: 0`, natively valid. So the verdict, not the fill, was wrong. `WaitForLoadState(networkidle)` can return immediately because Playwright's `Click` returns on event dispatch, before the app issues its request, so the page is read before the submission has happened. **#93 is direct evidence this misfires:** a Greenhouse security-code email timestamped the exact second of a submit the agent had written off. Cost of a premature verdict is a ~12-min model call plus a re-click on a form that may already have gone through (#89's duplicate-application risk)

### 95. The submit verdict was read from the DOM the instant the click returned, racing the submission itself (Resolved 2026-07-25)

**Three independent jobs produced the identical impossible signature.** ClickHouse:

```
21:15:58 Attempt 2 committed 2 autocomplete selection(s) ... #question_15561491004, #question_15653623004
21:15:58 Attempt 2 applied 3/3 validation fix(es) to: ...
21:15:58 Submission failed validation. Retrying...
21:15:58 Narrowed validation retry ... still invalid: <the same three fields>
```

Stack AV reproduced it exactly at 21:36:22 (`committed 4`, `applied 4/4`, `Submission failed validation`, all in one second), and Sporty Group had done the same earlier in the day.

**The fill was ruled out by probe, not by argument.** Against ClickHouse's real form, driving the agent's own commit sequence (click, clear, type, pick the option scoped via `aria-controls`):

| field | kind | result |
| --- | --- | --- |
| `question_15561491004` (sponsorship) | react-select | committed `"No"` |
| `question_15653623004` (AI-evaluation consent) | react-select | committed `"Yes"` |
| `question_15561492004` (current location) | plain text | holds `"Macomb, MI"` |

and then, critically, native constraint validation over the form containing `#first_name`: **`formValid: true`, `invalidCount: 0`**. Before the commits it was `false` with 2 invalid controls — react-select's hidden `input.requiredInput` proxies, which carry **no id and no name**, and which react-select *removes from the DOM* once a value is selected. So the agent's commits fully satisfy these forms client-side. The fill machinery is not the problem; the verdict is.

**Root cause.** Both the initial path (`confirmOrError`) and the retry loop judged the click from a single page read taken immediately after:

```go
page.WaitForLoadState(networkidle, 10s)
currentURL := page.URL()
pageContent, _ := page.Content()
isSubmissionConfirmed(...)
```

Playwright's `Click` returns when the event is dispatched, not when the application reacts. At the moment `WaitForLoadState` is called there is frequently no request in flight yet, so the page already counts as idle and the wait returns at once. The DOM is then read **before the submission has happened**, shows the form still present and unchanged, and is scored as a validation failure. That is exactly why all four log lines share one second: there is no network round-trip in between because none had started.

**#93 is direct live evidence this misfires.** It found a Greenhouse security-code email timestamped *the exact second of a submit the agent had written off as failed* — the submission reached Greenhouse's servers while the agent concluded it had not.

**Fix.** `awaitSubmissionOutcome` replaces the single read with a bounded poll, and the per-poll rules live in a pure `decideSubmissionOutcome` so they are unit-testable — branching inside the driver loop is what let #76 ship inert:

1. Confirmation wins at any elapsed time; a thank-you view rendering in 200ms is as real as one taking 8s.
2. Past a 2s settle floor, **either** fields flagged `aria-invalid` **or** explicit validation-error wording settles it as rejection — both are positive evidence the server answered. Both forms are needed: the theme in #83 sets no `aria-invalid` but still renders error text, and a form flagged only via `aria-invalid` may carry no wording. Before the floor, neither counts: that is just the pre-submit DOM.
3. Otherwise keep waiting, to a 15s budget.

The change is deliberately one-directional: it can turn a premature "failed" into a correct "confirmed", never the reverse. 6 new tests. The timings are `var`s so the genuine no-evidence-either-way case — which now correctly waits out the full budget — does not make the suite sit through 15s.

**Stated plainly: the race is inferred, not directly observed.** It is consistent with every symptom (the same-second timing, forms proven satisfiable, payloads unchanged between attempts, #93's email, #89's late-render finding), but confirming it directly would require clicking submit on a live posting, which files a real application. Treat the mechanism as strongly supported rather than proven, and watch for the first `Submission confirmed` to settle it.


---

## 94. [The dedup row was written at document generation, so a job that never submitted was skipped forever](#94-the-dedup-row-was-written-at-document-generation-so-a-job-that-never-submitted-was-skipped-forever)

**Table rationale cell (original):** Live: the 21:16 restart logged `Duplicate check: Already applied` for **Reddit, Akuity, ClickHouse and Staff SRE** — four jobs the same day's log shows failing, and which have **never** reached `APPLIED`. `SaveApplication` wrote the `applied_jobs` row at document-generation time, so any job that generated documents and then bounced, needed manual review, or was killed mid-submit was permanently marked applied. Combined with the funnel row returning to `DISCOVERED` (startup reaper / #85's reset / requeue without `-clear-dedup`), the job was re-queued every run and skipped instantly, forever — **silently unreachable rather than visibly failed**. 7 of the 82-job cohort and 66 rows DB-wide were in this state

### 94. The dedup row was written at document generation, so a job that never submitted was skipped forever (Resolved 2026-07-25)

**Found in the restart log, in four lines that looked like routine housekeeping.** The 21:16 relaunch reported:

```
21:16:42 [Worker-1] Duplicate check: Already applied to Reddit. Skipping.
21:16:43 [Worker-1] Duplicate check: Already applied to Akuity. Skipping.
21:16:43 [Worker-1] Duplicate check: Already applied to ClickHouse. Skipping.
21:16:43 [Worker-1] Duplicate check: Already applied to Staff Site Reliability Engineer. Skipping.
```

Every one of those four is a job this same day's log shows *failing*. ClickHouse is the cleanest case: its dedup row is timestamped `21:08:15.789`, which is the exact second `SaveApplication` ran for it, and the process was killed at 21:16 while it was still mid-attempt-3. It had never submitted anything.

**Root cause.** `SaveApplication` ended with `return RecordApplicationInDB(...)`, so the `applied_jobs` row — the sole thing `HasApplied` gates on — was written at **document-generation** time, minutes before the first submit click and regardless of its outcome. Generating documents is not applying.

**Why it was permanent rather than merely wrong.** On its own, a spurious dedup row would only matter if the job were re-queued. Three separate mechanisms re-queue it, all added or exercised the same day:

- the startup reaper resetting orphaned `PROCESSING` rows to `DISCOVERED` (#55),
- #85's duplicate-path reset, which deliberately returns the row to `DISCOVERED`,
- `cmd/requeue` run without `-clear-dedup`.

So the job returns to `DISCOVERED`, is loaded into the next run's queue, hits `HasApplied` → true, is skipped in milliseconds, and is reset to `DISCOVERED` again. It is queued every single run, forever, and never progresses. The funnel status reads `DISCOVERED` — indistinguishable from a job genuinely waiting its turn — so nothing in the dashboard or the status breakdown shows it as stuck. **The failure mode is silent unreachability, not visible failure.**

Measured live at the time of the fix: **7 of the 82-job cohort** and **66 rows DB-wide** sat in `DISCOVERED` carrying a dedup row.

**This is the mechanism behind #53's falsehood, seen from the write side.** #53 recorded that `applied_jobs` overstates what was applied to, and `cmd/dashboard` was already patched to count `job_funnel.status = 'APPLIED'` instead. That fixed the *reporting*. The write itself was never corrected, and `HasApplied` still trusted it — so the same bad data kept silently suppressing work. `job_funnel` has **never** contained a single `APPLIED` row across all 3,884 rows, while `applied_jobs` holds 261.

**Fix.** `SaveApplication` no longer writes the dedup row; it remains responsible for the documents folder (`MoveToManualApply` archives it) and the record the dashboard reads. `cmd/agent` writes the row on the confirmed-submission branch, next to `UpdateFunnelStatus(job.URL, "APPLIED")`, and nowhere else. `RecordApplicationInDB` became `ON CONFLICT(url) DO NOTHING`: the row is no longer written on a path guaranteed to run exactly once, and #89's confirmation re-check can legitimately observe success twice for one URL, where the `UNIQUE` constraint would have reported an error against an application that actually worked.

**Two existing tests asserted the old behaviour and were deliberately inverted**, each with the reasoning written into the test body so it reads as a correction rather than a regression: `TestSaveApplication` required `HasApplied` to be true after saving documents, and `TestApplicationsAndDuplicates` required a duplicate insert to error. Three tests total now pin the new contract, including `TestSaveApplicationLeavesJobRetryableUntilConfirmed`, which drives the full failed-then-confirmed sequence. All three were verified failing against the old code before the fix was kept.

**Operational cleanup.** The 7 stuck cohort rows had their dedup rows cleared. That was safe to assert rather than assume: all 7 dedup timestamps fall inside the current log window, and that window contains **zero** `Submission confirmed` lines, so none of them was ever submitted. The remaining DB-wide rows were **not** cleared — their timestamps predate the log, so there is no positive evidence either way, and re-applying to an employer who already has an application is an outward-facing action that is the user's call, not the agent's. Left as an open question for the user rather than silently resolved in either direction.

**Method note, and it is the recurring one.** This was found by reading a log line that announced itself as normal operation. `Duplicate check: Already applied to X. Skipping.` is what a correctly-working dedup looks like; the defect was only visible in the conjunction of that line with a company name the same log had shown failing an hour earlier. Related to the standing warning that *an absent signal is not evidence of an absent event* (#77, #84, #81) — here the inverse: **a present, benign-looking signal is not evidence of a benign event.**


---

## 93. [Greenhouse's emailed security-code gate read as a validation error, burning the full 45-minute timeout](#93-greenhouses-emailed-security-code-gate-read-as-a-validation-error-burning-the-full-45-minute-timeout)

**Table rationale cell (original):** **Found from the user's inbox, not the logs.** A Greenhouse submit to Surt AI produced an email at **20:58:03 UTC — the exact second of the click**: *"Copy and paste this code into the security code field on your application ... After you enter the code, resubmit your application."* So the submit **succeeded** and Greenhouse issued an out-of-band verification challenge. The resulting security-code input read as just another unsatisfied required field, so the whole 50,501-char form went to the model and burned the full 45-minute timeout. **This reframes #83:** the oversized payload was a *symptom* of the code gate, not the underlying event

### 93. Greenhouse's emailed security-code gate read as a validation error, burning the full 45-minute timeout (Resolved 2026-07-25)

**Found from the user's inbox, not from the logs** — which is the whole point of this entry. The agent's own telemetry could not see it.

The user forwarded a Greenhouse email:

> **Security code for your application to Surt AI**
> Hi William, Copy and paste this code into the security code field on your application: `uOSBQvRu`
> After you enter the code, resubmit your application.

Timestamp: **2026-07-25 20:58:03 UTC**. The surtai submit clicked at **16:58:03 local — the same second.**

**What actually happened**, versus what the logs said:

| logged | actually |
| --- | --- |
| `Submission failed validation. Retrying...` | the submit reached Greenhouse and **succeeded** |
| retry payload 50,501 chars, no `aria-invalid` | the page now showed a **security-code field** |
| `context deadline exceeded` after 45 min | the model was asked to "fix" a field only an emailed code can satisfy |

**This reframes #83.** That entry blamed a theme with no `aria-invalid` markers forcing a full-form payload. The size ceiling it added is still correct and still worth having — but the *reason* this particular form had nothing flagged invalid was that it was no longer a validation failure at all. It was a verification gate.

**Why no number of retries could ever work:** the code exists only in the applicant's mailbox. The agent has no access to it, so each attempt asked a local model to invent a value that cannot be invented — at ~12 minutes a go, then 45 for the timeout.

**Fix:** `parser.DetectSecurityCodeChallenge` checks for a code input **and** the matching page wording, and the retry loop returns `ErrNeedsEmailVerification` **before any model call** — ahead of even the attestation guard, since a code the agent cannot obtain makes everything downstream pointless. It joins `manualReviewErrors`, so #84's routing preserves the job with its documents.

**Both conditions are required deliberately.** Wording alone would strand real applications — job descriptions mention "security" and "verification" routinely — and a bare field without the wording is not evidence of a gate. Pinned by two negative tests.

**Open question for the user, not decided here:** making the agent retrieve these codes itself would need Gmail API credentials wired into `cmd/agent`. That is free but a genuine new capability with real access implications, and it is the user's call. Filed as improvements #32.

**Tests:** `TestDetectSecurityCodeChallenge_FindsTheGreenhouseCodeGate`, `TestDetectSecurityCodeChallenge_IgnoresWordingWithoutAField`, `TestDetectSecurityCodeChallenge_IgnoresAFieldWithoutTheWording`.


---

## 92. [Checkbox-group ids contain brackets, which are CSS attribute syntax, so they resolved to nothing](#92-checkbox-group-ids-contain-brackets-which-are-css-attribute-syntax-so-they-resolved-to-nothing)

**Table rationale cell (original):** Live: `Validation fix for "input#question_8242451101[]_54236360101" failed: selector matched no element (tried 1 form(s))`. Greenhouse names checkbox-group controls with a literal `[]` in the id; `#question_...[]_...` is not a valid CSS id selector because the brackets read as attribute syntax. **The same class as #73** (leading digits), and #73's own attribute-form fallback was blocked here because `splitTagID` explicitly refused any id containing brackets — note `tried 1 form(s)`, versus the 3 an eligible selector gets

### 92. Checkbox-group ids contain brackets, which are CSS attribute syntax, so they resolved to nothing (Resolved 2026-07-25)

Observed live on Sporty Group:

```
Validation fix for "input#question_8242451101[]_54236360101" failed:
  selector matched no element (tried 1 form(s) of "input#question_8242451101[]_54236360101")
```

Greenhouse names checkbox-group controls with a literal `[]` in the id — `question_8242451101[]_54236360101`. As a CSS selector, `#question_8242451101[]_54236360101` is not an id: the brackets read as attribute syntax, so it matches nothing. The `[id="question_8242451101[]_54236360101"]` attribute form resolves it perfectly.

**This is the same class as #73** (a leading digit making `#430` invalid), and #73's own fix should have caught it — the attribute-form retry exists for exactly this. It did not, because `splitTagID` explicitly refused any id containing `[` or `]`. The `tried 1 form(s)` in the log is the tell: an eligible selector gets three.

**Fix:** brackets no longer disqualify. Combinators and separators still do (`#`, `.`, `>`, `:`, `,`, whitespace) — those indicate a compound selector where rewriting the tail as a single id would change the meaning.

**Tests:** `TestSplitTagID_AllowsBracketsSoCheckboxGroupIDsResolve`, `TestSplitTagID_StillRefusesCompoundSelectors`.


---

## 91. [#90's single-option rule could never fire, because typing filters the sole option out](#91-90s-single-option-rule-could-never-fire-because-typing-filters-the-sole-option-out)

**Table rationale cell (original):** **A defect in #90's own fix, caught on the very next run.** Sporty Group's `GDPR Acknowledgement*` still reported `left the control empty` with #90 shipped. #90 takes the sole option when `len(options) == 1` — but `setComboboxValue` types the model's proposed value *first*, and typing "Yes" into a widget whose only entry is "Acknowledge/Confirm" filters the list to **zero**. So the count was 0, never 1, and the rule could not fire for precisely the case it was written for

### 91. #90's single-option rule could never fire, because typing filters the sole option out (Resolved 2026-07-25)

**A defect in #90's own fix, caught on the very next run** — the same shape as #76 (a defect in #74) and #81 (a defect in #76's fallback).

Sporty Group re-run with #90 shipped:

```
20:30:50 Attempt 2: 1 fix(es) reported success but left the control empty
         (autocomplete/combobox suspected): input#question_7849575101
```

Unchanged. #90 selects the sole option when `len(options) == 1`, and probing had confirmed `GDPR Acknowledgement*` offers exactly one: `Acknowledge/Confirm`. But `setComboboxValue` **types the model's proposed value before reading the options**, and typing "Yes" into a widget whose only entry is "Acknowledge/Confirm" filters the list to **zero**. So the observed count was 0, never 1 — and the rule could not fire for precisely the case it was written for.

I had verified the option list by clicking the control open with **no query typed**; the agent always types first. The probe and the code were doing different things, and the difference was the whole bug.

**Fix:** when typing yields no options at all, clear the query and re-read. An empty query restores the unfiltered list, which is where the lone option lives.

**Method note:** this is the third time a fix of mine has been inert for a reason only a live run exposed (#76, #81, #91). The pattern is consistent — the probe reproduces *my* mental model of the sequence, not the code's actual sequence. Probing is still what found each one; the lesson is to replicate the code path exactly, including the order of operations, rather than the outcome I expect it to reach.


---

## 90. [A required control with exactly one option was refused, sending a job to manual review one click from completion](#90-a-required-control-with-exactly-one-option-was-refused-sending-a-job-to-manual-review-one-click-from-completion)

**Table rationale cell (original):** Sporty Group (Greenhouse, score **90**) got **10 of its 11 invalid fields satisfied** — invalid payload collapsed 6389 → 610 chars, including 3 committed autocompletes, a GDPR checkbox and a checkbox-group entry. The sole holdout was `GDPR Acknowledgement*`, a combobox offering **exactly one option: "Acknowledge/Confirm"**. The model proposed a differently-worded affirmative, so #79's containment check matched nothing and selected nothing — correct caution where several options exist, over-conservative where there is only one and therefore no wrong choice to make

### 90. A required control with exactly one option was refused, sending a job to manual review one click from completion (Resolved 2026-07-25)

**The closest the pipeline has come to finishing.** Sporty Group (Greenhouse, fit **90**) on the full fix stack:

```
19:54:03 still invalid: gdpr_processing_consent_given_1, question_7849567101, ... (11 fields)
20:07:46 Attempt 2 committed 3 autocomplete selection(s) that Fill() alone had left empty
20:07:46 Attempt 2 applied 9/9 validation fix(es)
20:07:46 Narrowed ... (50136 -> 610 chars); still invalid: question_7849575101
```

**Eleven invalid fields down to one.** The payload collapsed from 6,389 to 610 characters. Three autocompletes committed, a GDPR consent checkbox ticked, a checkbox-group entry set — the whole machinery from #70 through #89 working together on a genuinely hard form.

Probing the survivor:

```
label:   "GDPR Acknowledgement*"
options: [Acknowledge/Confirm]      <- exactly one
```

**Root cause:** the model proposed a reasonable affirmative ("Yes", "I acknowledge", or similar) and `pickComboboxOption` requires the option text and the wanted value to contain one another. "acknowledge confirm" neither contains nor is contained by "yes", so **nothing was selected**. #88 then correctly routed the job to `MANUAL_REQUIRED`.

That caution is right when there are several options — it is exactly what stops "Detroit, ME" being filed instead of "Detroit, MI" (#79). But with **one** option there is no wrong choice available. Refusing it costs a real application on a 90-fit job that was otherwise complete.

**Fix:** when `mustContain` is empty and the control offers exactly one option, take it. **Deliberately not applied when `mustContain` is set** — those tokens exist precisely because the *identity* of the option matters, so a lone option that fails them is a wrong answer rather than an obvious one. A lone `Detroit, ME` must still be refused.

**Tests:** `TestPickComboboxOption_TakesTheSoleOptionWhenThereIsOnlyOne`, `TestPickComboboxOption_StillRefusesALoneOptionThatFailsMustContain` (the #79 guarantee survives), `TestPickComboboxOption_StillRefusesWhenSeveralOptionsAndNoneMatch`.

**Worth noting:** #88 did its job here — the outcome was a preserved manual-review job naming the exact field, not a silent `FAILED_SUBMIT`. That is what made this diagnosable in one probe.


---

## 89. [A late-rendering confirmation page is missed, so a successful submit is retried — filing duplicates](#89-a-late-rendering-confirmation-page-is-missed-so-a-successful-submit-is-retried--filing-duplicates)

**Table rationale cell (original):** Surfaced by Orkes routing to `MANUAL_REQUIRED` via #83 with a **43,411-char** payload — which only happens when narrowing finds **nothing flagged invalid** and falls back to the whole document. Combined with attempt 2 having applied both fixes, the likeliest reading is that the submit **succeeded** and the page became a confirmation page with no form. Greenhouse replaces the form in place, so `currentURL == applyURL` and only a confirmation *phrase* can prove success — and if that page renders after the 10s networkidle wait, the check right after the click sees the old DOM and reports failure. **The loop then re-submits an application that already went through**

### 89. A late-rendering confirmation page is missed, so a successful submit is retried — filing duplicates (Resolved 2026-07-25)

**Surfaced by an outcome that did not fit.** Orkes routed to `MANUAL_REQUIRED` through #83's time-budget ceiling, with a payload of **43,411 characters**. That only happens when `PruneDOMToInvalidFields` finds **nothing flagged invalid** and falls back to the whole document. But attempt 2 had just applied both outstanding fixes, including committing the Yes/No combobox. A form with nothing invalid, right after both its blocking fields were satisfied, is far more consistent with **the submit having succeeded and the form being gone** than with a form still sitting there.

**Why the success would be missed:** Greenhouse replaces the form in place rather than navigating. So `currentURL == applyURL`, and `isSubmissionConfirmed` can only return true via a confirmation *phrase*:

```go
if currentURL == applyURL { return false, reasonURLUnchanged }
```

The check runs immediately after the click, behind a 10-second `networkidle` wait. If the thank-you view renders after that — entirely plausible on a CPU-bound host running a 30B model alongside the browser — the check reads the **old DOM**, reports failure, and the loop retries.

**Retrying a submit that already succeeded means filing a duplicate application with a real employer.** That is a materially worse failure than not applying at all, and it is invisible in the logs: it looks exactly like a validation bounce.

**Fix:** re-check `isSubmissionConfirmed` at the **top of every retry attempt**, before any DOM work. It is nearly free, and it is the only thing standing between a slow-rendering confirmation and a duplicate submission. On a hit it returns success with `on re-check before attempt N — the previous click had succeeded`.

Also added a diagnostic for the ambiguity that made this unreadable: when narrowing finds nothing invalid, the log now states **whether a `<form>` is still on the page** and the payload size. "Nothing is flagged invalid" means two opposite things — *the form is fine now* or *the form is gone because it submitted* — and the logs could not tell them apart.

**Honest status:** whether Orkes actually submitted is **not established**. The evidence is circumstantial (payload size, nothing invalid, both fields satisfied) and the browser state is gone. This fix ensures the *next* occurrence is detected rather than retried; it does not retroactively prove what happened here. Orkes sits in `MANUAL_REQUIRED`, which is a safe place for it either way — a human can check whether an application already exists.

**Correction to an earlier claim in #87:** I treated "`applied N/N` and the failure in the same second" as proof that nothing was submitted. That inference was too strong — a client-side validation rejection also produces no navigation and no delay. #87 was still a genuine defect (clicking the click-to-reveal Apply button is unambiguously wrong), but that timing signature alone did not prove non-submission.

**Tests:** `TestIsSubmissionConfirmed_ConfirmsOnPhraseWithAnUnchangedURL`, `TestIsSubmissionConfirmed_StillRefusesWithNoEvidence` (bug #51's guarantee must survive).


---

## 88. [A required widget that cannot accept the configured value was written off as a submit failure](#88-a-required-widget-that-cannot-accept-the-configured-value-was-written-off-as-a-submit-failure)

**Table rationale cell (original):** Confirmed live on Nova (Lever): `Attempt 3: 1 fix(es) reported success but left the control empty ... input[data-qa='location-input']`. The detection and commit machinery worked exactly as designed — it correctly saw the field as unset and tried to commit — but **Lever's geocoder returns zero results** for `Macomb`, `Macomb Township` and `Macomb, MI`, while Greenhouse's resolves the same address. With no option to select, the required hidden `selectedLocation` can never be populated. Not an automation failure: the job is perfectly applicable by hand, yet it burned 3 attempts and landed in `FAILED_SUBMIT`

### 88. A required widget that cannot accept the configured value was written off as a submit failure (Resolved 2026-07-25)

**This one is not a broken mechanism — it is the mechanism working and reaching an honest dead end.** Nova (Lever), re-run with #86 and #87:

```
19:22:15 Attempt 3: 1 fix(es) reported success but left the control empty
         (autocomplete/combobox suspected): input[data-qa='location-input']
```

Exactly the diagnostic #81 was built to produce: the field was correctly identified as unset, and the commit was attempted and failed. The cause is data, not code — measured directly against the live form, **Lever's geocoder returns zero results** for `Macomb`, `Macomb Township` and `Macomb, MI`, while Greenhouse's resolves `Township of Macomb, Michigan, United States` without trouble. No option exists to select, so the required hidden `selectedLocation` can never be populated, so the form can never validate.

**Why that needed a fix anyway:** the outcome was `FAILED_SUBMIT` after three full attempts. That is wrong twice over — it wastes the retries reaching a wall that was already known after the first, and it writes off a job that a human could complete in seconds, discarding its tailored documents.

**The option not taken:** substituting a nearby city the geocoder *does* know (Detroit is 25 miles away and resolves fine) would make the form submit. It would also state a false location on a real job application. Not done, and deliberately so — the same reasoning as #82's attestations.

**Fix:** `ErrUncommittableField`, added to `manualReviewErrors` so #84's catch-all routes it to `MANUAL_REQUIRED` with documents preserved and the offending selectors named in the log. The retry loop remembers the final attempt's uncommitted fields and returns this instead of the generic "failed after 3 validation error attempts" when any remain.

**Note for the user:** this is the concrete cost of a location that some ATS geocoders do not index. Greenhouse handles it; Lever does not. Since 39 of the original 82 jobs are Lever, a `pii.yaml` location the geocoders agree on would unblock a large share — but that is the user's call to make about their own address, not the agent's.

**Tests:** `TestErrUncommittableField_IsAManualReviewOutcome`.


---

## 87. [The submit locator clicked the click-to-reveal "Apply" button, so no retry ever actually submitted](#87-the-submit-locator-clicked-the-click-to-reveal-apply-button-so-no-retry-ever-actually-submitted)

**Table rationale cell (original):** **The one that was silently blocking everything.** Orkes (Greenhouse, 85) applied `2/2` fixes — both verifiably settable by probe — and still failed all 3 attempts, with `applied` and `Submission failed validation` in the **same second**, too fast for any navigation. The submit locator put `button:has-text('Apply')` in the same CSS alternation as the real controls, and alternations have **no precedence** — matches return in DOM order. Measured live: `[0] Apply (type=button)`, `[1] Quick Apply with MyGreenhouse`, `[2] Submit application (type=submit)`. Every retry clicked index 0, the click-to-reveal button, which does nothing once the form is open. The page never changed, the same fields stayed flagged, and all three attempts failed identically

### 87. The submit locator clicked the click-to-reveal "Apply" button, so no retry ever actually submitted (Resolved 2026-07-25)

**This is the defect that was silently invalidating the whole retry path**, and it took the previous eight fixes to expose it — until fields could actually be filled, nothing downstream of them was reachable.

Orkes (Greenhouse, fit 85) applied **`2/2` fixes** on every attempt and failed all three. The two fields were `LinkedIn Profile` (plain required text) and `Are you located in Australia or Europe?` (a Yes/No react-select). Probing proved both were satisfiable, in either order, with no interference:

```
order A: LinkedIn="https://linkedin.com/in/wylelias"  combobox="No"
order B: LinkedIn="https://linkedin.com/in/wylelias"  combobox="No"
```

The tell was in the timestamps: `applied 2/2` and `Submission failed validation` landed in the **same second**. A real submit plus a `networkidle` wait cannot complete that fast — so nothing was being submitted at all.

**Root cause:** the locator was a single CSS alternation —

```
input[type='submit'], button[type='submit'], button:has-text('Submit'), button:has-text('Apply')
```

CSS alternations carry **no precedence**; Playwright returns matches in **DOM order**. Measured on the live form:

```
[0] visible BUTTON type=button  "Apply"                      <- firstVisibleSubmit picked this
[1] visible BUTTON type=button  "Quick Apply with MyGreenhouse"
[2] visible BUTTON type=submit  "Submit application"          <- the real one
```

Every retry "submitted" by clicking the click-to-reveal **Apply** button — a `type=button` that does nothing once the form is already open. The page never changed, so the same fields stayed flagged and each attempt was a byte-for-byte repeat.

**This also re-frames earlier entries.** #70-#81 were all real and all necessary, but their fixes could never have produced a completed application on such a form: the fields were being filled correctly and then never submitted. The recurring "applied N/N and still rejected" signature that drove #72, #80 and #81 had *two* causes, and this was the second.

**Fix:** replaced the flat alternation with `submitControlSelectors`, tried in precedence order — real `type='submit'` controls first, then "Submit application", then "Submit" — with `findSubmitControl` returning the first group that has a visible match. **"Apply" is deliberately absent entirely**: it reveals a form, it never submits one, and keeping it even as a last resort would restore this bug on any form where the reveal button stays in the DOM.

**Tests:** `TestSubmitControlSelectors_PreferRealSubmitControlsAndNeverApply` (asserts the precedence and that "Apply" never appears), `TestFindSubmitControl_SkipsAGroupWithNoVisibleMatch`.


---

## 86. [Lever's location typeahead was invisible to combobox detection, so every Lever application failed](#86-levers-location-typeahead-was-invisible-to-combobox-detection-so-every-lever-application-failed)

**Table rationale cell (original):** Nova (Lever, score 65) failed all 3 attempts applying **7/7 fixes** each time. Probed the real form: only **3 fields are required** — name, email, `location-input` — and the resume upload is *optional*. Lever's location widget has **none of react-select's markers**: no `role`, no `aria-*`, no `select__` classes. It is a plain `<input name="location">` beside a hidden `<input name="selectedLocation">` that holds the committed value. Detection returned `false`, so it was filled with text while `selectedLocation` stayed empty — and that hidden field is what the form validates. **Clicking the option does not work either**: it loses a blur race, leaving both the visible input and the hidden field empty. Keyboard selection is blur-safe

### 86. Lever's location typeahead was invisible to combobox detection, so every Lever application failed (Resolved 2026-07-25, verified against the live form)

**Nova (Lever, `ioconnectservices`) failed all three attempts while reporting `7/7 validation fix(es) applied` each time** — the model even varied its selector syntax between attempts (`input[name='email']` then `input[data-qa='email-input']`), which looked like flailing but was irrelevant: both resolved fine.

Probing the real form settled it immediately. Lever asks for far less than Greenhouse — **only three required fields**, and the resume upload is *optional*:

```
EMPTY  text   name             EMPTY  email  email             EMPTY  text  location-input
file inputs: [optional resume files=0]
```

So the failure was entirely down to `location-input`. Its markup:

```html
<input class="location-input" data-qa="location-input" id="location-input" name="location" required>
<input id="selected-location" type="hidden" name="selectedLocation">
<div class="dropdown-container"><div class="dropdown-results">
```

**Root cause:** none of react-select's markers are present — no `role="combobox"`, no `aria-autocomplete`, no `aria-controls`, no `select__` classes. `isComboboxInputJS` therefore returned **false**, so the field was treated as a plain text input: filled with text, never committed, while the hidden `selectedLocation` — *the value the form actually validates* — stayed empty. Measured directly: `detectedAsCombobox: false`, `selectedLocation=""` after typing.

**A second, independent obstacle:** clicking the chosen option does not commit it. The click blurs the input, the dropdown closes, and the handler never fires — measured, leaving **both** the visible input and `selectedLocation` empty. Keyboard selection is blur-safe and does work.

**Fix:**
- Detection extended to a sibling `.dropdown-results`/`.dropdown-container` or a hidden `input[name^="selected"]`.
- `readHiddenCommitValueJS` reads that hidden field as the committed value — never `el.value`, for #81's reason.
- Option enumeration extended to Lever's `.dropdown-location` results.
- `setComboboxValue` keeps the click (confirmed working for react-select and left undisturbed) and falls back to **index-driven keyboard selection**: arrow to the option `pickComboboxOption` chose, then Enter. Index-driven, not "first option", because #79's guarantee has to hold here too — **"Detroit, ME" sits directly beneath "Detroit, MI" in the same list**.

**Verified against the live form, all parts in one run:**

```
detectedAsCombobox (new logic): true
options seen (4): location-0|Detroit, MI, USA  location-1|Detroit, ME, USA  location-2|Detroit, TX, USA ...
picked id="location-0" index=0 ok=true
AFTER keyboard commit: visible=Detroit, MI, USA
  hidden(selectedLocation)={"name":"Detroit, MI, USA","id":"cf06481e9473fd2cbab9d1db5ddb043a7c4170df"}
```

**Open caveat, deliberately not worked around:** Lever's geocoder returns **zero results** for `Macomb`, `Macomb Township` and `Macomb, MI`, while Greenhouse's resolves `Township of Macomb, Michigan, United States` happily. So this fix makes Lever's location *committable*, but the configured location may still not be findable there. Substituting a nearby city the geocoder does know would be misrepresenting the applicant's location on a real application, so it is not done. If this proves common, the honest outcome is to route such jobs to `MANUAL_REQUIRED` rather than to invent a location.

**Also worth noting:** Lever's geocoder rate-limits repeated queries — the same term returned 4 options, then 0, then 4 again across probe runs. Any live testing here needs a fresh page and a single query.

**Tests:** `TestComboboxJS_DetectsLeverStyleTypeaheads`, plus `TestPickComboboxOption_RejectsTheWrongStateEvenWhenItIsFirst` extended to assert the returned index, which is what drives keyboard selection.


---

## 85. [Four early-exit paths left rows stranded in PROCESSING, invisible to every future queue](#85-four-early-exit-paths-left-rows-stranded-in-processing-invisible-to-every-future-queue)

**Table rationale cell (original):** Spotted from the cohort monitor: `PROCESSING=4` on a **single-worker** run. The stranded rows clustered in pairs at exactly the moments a job was skipped — each was `Duplicate check: Already applied ... Skipping.` The worker sets `PROCESSING` at the top of the loop, and four `continue` paths exit without ever clearing it. `GetDiscoveredJobs` selects only `DISCOVERED`, so a stranded row never returns to any queue; #55's startup reaper masked it by resetting them, whereupon they were re-picked, skipped, and stranded again — a silent loop that corrupts cohort accounting and the dashboard's in-flight metrics

### 85. Four early-exit paths left rows stranded in PROCESSING, invisible to every future queue (Resolved 2026-07-25)

**Found by noticing an impossible number.** The cohort monitor reported `PROCESSING=4` on a run with `Using 1 worker(s)`. One worker cannot have four jobs in flight.

The timestamps clustered in pairs, at startup and again exactly when the previous job resolved:

```
Reddit                            22:01:28
Akuity                            22:01:28
Staff Site Reliability Engineer   22:10:45
Stack AV                          22:10:45
```

Cross-referencing the log gave the cause immediately:

```
18:01:28 Fetching job description for Reddit...
18:01:28 Duplicate check: Already applied to Reddit. Skipping.
```

**Root cause:** the worker sets `UpdateFunnelStatus(job.URL, "PROCESSING")` at the top of the loop, and **four** `continue` paths exit without ever clearing it:

| Path | Was | Now |
| --- | --- | --- |
| Invalid/unsafe URL blocked | stranded | `INVALID_URL` (the status already existed for exactly this) |
| Failed to create HTTP request | stranded | `DISCOVERED` (transient) |
| Failed to read response body | stranded | `DISCOVERED` (transient) |
| Duplicate check skip | stranded | `DISCOVERED` |

`GetDiscoveredJobs` selects only `DISCOVERED`, so a stranded row never reappears in any future queue. **#55's startup reaper masked this**: every restart reset the orphans, they were re-picked, skipped, and stranded again — a silent loop that consumed a queue slot each run, corrupted the cohort accounting, and inflated the dashboard's in-flight figures.

**On the duplicate-check case specifically — `DISCOVERED`, deliberately not `APPLIED`.** The `applied_jobs` record is written at *document generation*, not at confirmed submission. That is precisely the falsehood this entire 82-job re-verification exists to audit (see #53: most historical `APPLIED` rows had no confirmation evidence at all). Marking the row `APPLIED` here would manufacture the very claim under investigation. Resetting to `DISCOVERED` restores the pre-`PROCESSING` state, matches exactly what the startup reaper already does, and asserts nothing new about the job.

**The deeper issue is left open and unchanged:** dedup rows are written before submission is confirmed, so `HasApplied` can skip a job that was never actually submitted. That is pre-existing behaviour, not something this fix should silently redefine.


---

## 84. [#82's manual-routing branch was never applied, so refused jobs were written off as FAILED_SUBMIT](#84-82s-manual-routing-branch-was-never-applied-so-refused-jobs-were-written-off-as-failed_submit)

**Table rationale cell (original):** **My own error, caught live.** #82's guard worked perfectly — ClickHouse was refused in **0 seconds** with `work authorization, visa sponsorship` — but the job landed in `FAILED_SUBMIT`, not `MANUAL_REQUIRED`, and the routing log line never appeared. The `cmd/agent` edit adding that branch **silently failed to apply**; `go build` still passed because the submitter half compiled fine, and I verified the build instead of verifying the edit. Consequence: a job that is perfectly applicable-by-hand was written off as a failure, its tailored documents never moved to the manual-apply folder and no manual-queue entry logged

### 84. #82's manual-routing branch was never applied, so refused jobs were written off as FAILED_SUBMIT (Resolved 2026-07-25)

**My own error, and worth recording as such.** #82's guard behaved exactly as designed — watched live on ClickHouse:

```
17:52:52 Narrowed validation retry ... (53969 -> 1877 chars); still invalid: question_15561491004, ...
17:52:52 Auto-Submit failed for ClickHouse: form requires a legal attestation the applicant has not provided: work authorization, visa sponsorship
```

Same timestamp — refused in **zero seconds**, before the ~12-minute model call, exactly as intended. But then:

```
sqlite> SELECT status FROM job_funnel WHERE company_name='ClickHouse';
FAILED_SUBMIT
$ grep -c 'needs a legal attestation not set in pii.yaml' career_agent.log
0
```

`FAILED_SUBMIT`, not `MANUAL_REQUIRED`, and the routing log line never fired. The `cmd/agent` branch that was supposed to catch `ErrNeedsUnprovidedAttestation` **did not exist in the source at all** — the scripted edit silently failed to match, `go build ./...` passed anyway because the `pkg/submitter` half compiled fine, and I confirmed the build rather than confirming the edit.

**Consequence:** a job that is entirely applicable by hand — high fit score, form reachable, only a personal declaration missing — was recorded as a failure. Its tailored documents were never moved to the manual-apply folder and no manual-queue entry was written, so it would simply have been lost.

**The irony is the point.** This is the same failure mode as #76, #77 and #81: *trusting an absence*. There I reasoned from a log line that never appeared; here I reasoned from a compiler that never complained. A green build proves the code that exists compiles, not that the code you intended exists.

**Fix, in two parts:**
1. The branch, applied properly and verified present with `grep` rather than inferred from a passing build.
2. **A structural guarantee instead of a promise.** `submitter.manualReviewErrors` now lists every sentinel meaning "queue this for a human", with `IsManualReviewError` over it, and `cmd/agent` consults it as a **catch-all immediately before the generic failure log**. A sentinel added in future without its own branch still reaches manual review rather than silently becoming `FAILED_SUBMIT`.

**Confirmed live 2026-07-25 18:10**, as a clean A/B on the same job:

```
17:52:52  Auto-Submit failed for ClickHouse: form requires a legal attestation...   -> FAILED_SUBMIT
18:10:44  ClickHouse needs a legal attestation not set in pii.yaml -- queued for
          manual submission: work authorization, visa sponsorship                  -> MANUAL_REQUIRED
```

**Tests:** `TestIsManualReviewError_CoversEveryManualReviewSentinel` (including wrapped, as call sites actually return them), `TestIsManualReviewError_IgnoresOrdinaryFailures` — a real automation failure must *not* be diverted to manual review, since that would hide genuine bugs behind a queue.


---

## 83. [The payload breaker guarded the context window but not the time budget, burning the full 45-minute timeout](#83-the-payload-breaker-guarded-the-context-window-but-not-the-time-budget-burning-the-full-45-minute-timeout)

**Table rationale cell (original):** Watched live end to end: a Greenhouse theme (`surtai`) that sets **no `aria-invalid` attributes** defeated #64's narrowing, so the retry fell back to the whole form — **50,501 chars**. That fits the 80,000-char context ceiling, passed `likelyExceedsModelContext`, and then ran **16:58:03 → 17:43:03 — exactly the 45-minute Ollama timeout** before dying. Three-quarters of an hour of the single serialised LLM resource, spent on a request that was mathematically incapable of finishing. Context capacity and inference time are different limits, and on this hardware the time one binds far earlier

### 83. The payload breaker guarded the context window but not the time budget, burning the full 45-minute timeout (Resolved 2026-07-25)

**Predicted, then watched happen.** A Greenhouse posting on a different tenant (`surtai`) produced no `still invalid:` line at all — its theme sets no `aria-invalid` attributes, so `PruneDOMToInvalidFields` found nothing to narrow to and correctly fell back to the whole form, per #64's deliberate design ("an unreadable theme is a reason to send more, never less").

That fallback payload was **50,501 characters**:

```
16:58:03 Attempt 2: Solving validation errors...
16:58:03 SolveValidationErrors API Call #4 executed. Payload length: 50501 characters.
17:43:03 Auto-Submit failed: ... context deadline exceeded
```

**Exactly 45 minutes.** The full `defaultOllamaTimeoutMinutes`, on the one resource that serialises across the entire pipeline, spent on a request that could never have completed.

**Root cause: two different ceilings, only one of them enforced.** `likelyExceedsModelContext` tested against `maxPromptCharsForModelContext` (80,000) — a limit derived from the llama-server context window. 50,501 fits it comfortably, so the check passed. But fitting the context says nothing about finishing in time.

The arithmetic was already recorded in this file and simply never applied here: prompt processing measured at **~7 tok/s** on this host's 30B model (#64, improvements #25), at ~2.5 chars/token ≈ **17.5 chars/s**. Against a 45-minute timeout that is **~47,000 characters** before a request is doomed. The observed 50,501 sits just past it — the prediction and the measurement agree to within a few percent.

**Fix:** added `maxPromptCharsForTimeBudget` (40,000, leaving headroom for token generation and for CPU contention with the browser) and made `likelyExceedsModelContext` trip on either ceiling. Oversized forms now hit the existing `ErrFormTooLargeForModel` path and route to `MANUAL_REQUIRED` **immediately**, with documents saved, instead of costing 45 minutes first.

**A prior test had to be corrected, deliberately.** `TestLikelyExceedsModelContext` carried a case from #60 asserting that 54,917 chars *should pass*. #60 was right about context capacity and silent about time; today's live evidence settles it. The case now expects `true`, with the derivation written into the test so the change reads as a correction rather than a regression.

**Tests:** `TestLikelyExceedsModelContext_RejectsATimeDoomedPayloadThatFitsTheContext` (uses the exact 50,501 size observed), `TestLikelyExceedsModelContext_StillAllowsNormalPayloads`, plus the corrected `#60` case.


---

## 82. [Once the commit worked, an unanswerable legal attestation would have been guessed and really submitted](#82-once-the-commit-worked-an-unanswerable-legal-attestation-would-have-been-guessed-and-really-submitted)

**Table rationale cell (original):** **A risk created by fixing #81.** While the combobox commit was broken, nothing the model proposed was ever really set. Probed on the live form immediately after #81: `#question_67942418 -> COMMITTED "Yes"`. Reddit's "Are you currently authorized to work in the U.S.?" and its sponsorship question are **required and offer only Yes/No — no decline option**, so a model with no configured answer does not abstain, it picks one. That answer is a **legal declaration submitted under the applicant's name**. The form is now refused *before* the model is asked, and the job routed to `MANUAL_REQUIRED`

### 82. Once the commit worked, an unanswerable legal attestation would have been guessed and really submitted (Resolved 2026-07-25)

**This is a risk that #81 created rather than one it revealed**, and it is worth being precise about that. For the whole of 2026-07-25 the combobox commit was broken, so whatever the model proposed for a screening question was never actually set. #81 fixed that. Probing the live form immediately afterwards:

```
#question_67942418   want="Yes"
  after Fill(): isCombobox=true readCombobox=""  -> not landed -> commit runs
  commit: prefix="Yes" opts=1 -> COMMITTED "Yes"
```

It commits. Which means from #81 onward, whatever the model answers to *"Are you currently authorized to work in the U.S.?"* is really submitted.

**The earlier safety assumption does not hold for these fields.** `ApplicationFacts` tells the model that anything not listed "was not provided" and to "choose the form's decline option" — and for the EEO questions that works, because every one of them offers *"I don't wish to answer"* (verified: `430`, `433`, `434`). But the option-level audit found:

```
question_67942418 (authorized to work in US):  Yes | No
question_67942419 (requires sponsorship):      Yes | No
```

**Required, binary, no decline option.** There is nothing for the model to decline *to*. Instructed not to fabricate, and given no abstention, it will still pick one — and that answer is a legal declaration made under the user's name to a real employer.

**Fix — refuse before asking, not after.** `parser.DetectAttestationQuestions` scans the form's visible text for the phrasings ATS forms actually use across four categories (work authorization, visa sponsorship, security clearance, criminal history). `PII.MissingAttestations` filters those to the ones with no configured answer. If any remain, the retry loop returns `ErrNeedsUnprovidedAttestation` **before** `SolveValidationErrors` is called, and `cmd/agent` routes the job to `MANUAL_REQUIRED` with its tailored documents saved — the same path already used for auth walls and oversized forms.

Refusing *before* the model call matters twice over: it cannot produce a guess to submit, and it saves the ~12-minute inference that would have produced one.

**False positives cost a real application**, so detection is deliberately phrase-based rather than keyword-based — `TestDetectAttestationQuestions_IgnoresOrdinaryForms` pins that "desired salary" and "why do you want to work here" do not trip it. `visa_status` is accepted as a stand-in for the sponsorship question, since "U.S. Citizen" answers it unambiguously.

**Unblocking is one line of config per category** (`work.authorized_to_work_us`, `work.requires_sponsorship`, `work.security_clearance`, `work.criminal_history`). The log names exactly which category is missing.

**Tests:** `TestDetectAttestationQuestions_FindsTheRealGreenhousePhrasings`, `TestDetectAttestationQuestions_IgnoresOrdinaryForms`, `TestDetectAttestationQuestions_FindsClearanceAndCriminalHistory`, `TestMissingAttestations`.


---

## 81. [data-value mirrors the typed search text, so every react-select falsely reported "landed"](#81-data-value-mirrors-the-typed-search-text-so-every-react-select-falsely-reported-landed)

**Table rationale cell (original):** Caught by #80's new diagnostic: `Attempt 2 applied 13/13 validation fix(es)` and the still-invalid list came back **byte-identical** — none of the 13 landed, including the declinable EEO fields that have nothing to do with the missing attestations. Probed directly: after a bare `Fill()` with **nothing selected**, the value read returned `"I don't wish to answer"`. react-select puts `data-value` on `.select__input-container` to mirror the *typed search text* for input sizing, so the `[data-value]` fallback was reading the artifact of typing — the same mistake as #76, one layer deeper. The false "landed" suppressed the commit step for every custom question on every Greenhouse form

### 81. data-value mirrors the typed search text, so every react-select falsely reported "landed" (Resolved 2026-07-25)

**Caught immediately by #80's new diagnostic**, which is exactly why #80 was worth filing:

```
16:27:01 Narrowed ... still invalid: 430, 431, 432, 433, 434, 436, gdpr_..., question_67942415 ... 67942420
16:40:11 Attempt 2 applied 13/13 validation fix(es) to: #430 ... #question_67942420
16:40:13 Narrowed ... still invalid: 430, 431, 432, 433, 434, 436, gdpr_..., question_67942415 ... 67942420
```

The list is **byte-identical before and after applying all 13 fixes**. Not a near miss — nothing landed at all, including the EEO fields, which are freely declinable and have nothing to do with the two missing attestations. Before #80 this was invisible: the only observable was a payload size drifting 7212 → 7281.

**Root cause, established by probe rather than inference.** Replicating what `applyValidationFix` does — resolve, `Fill()`, then run the agent's own checks:

```
[id="430"]   Fill()=nil, nothing selected
  isCombobox   = true
  readCombobox = "I don't wish to answer"    <-- reports LANDED
```

react-select sets `data-value` on `.select__input-container` to mirror the **typed search text** (it drives the input's auto-sizing). It is therefore non-empty the instant anything is typed, committed or not. The `[data-value]` fallback in `readComboboxValueJS` was reading it and calling it a committed selection.

So: `applyValidationFix` types the text, the read-back sees `data-value` and reports success, the commit step is skipped, and the field is never actually set. Every custom question on every Greenhouse form, every attempt.

**This is the same mistake as #76 one layer deeper** — reading the artifact of typing and treating it as a committed value. #76 fixed `el.value`; the fallback added alongside it had the identical flaw and went unnoticed because it only fires when `el.value` is empty.

**Why Location/Country still worked:** `fillGreenhouseCombobox` calls `setComboboxValue` directly and clicks a real option, so `.select__single-value` genuinely exists and is checked first. Only the *retry* path, which starts from a bare `Fill()`, was fooled.

**Fix:** `readComboboxValueJS` now reads **only** `.select__single-value` / `.select__multi-value__label` — the widget's rendered selection. Pinned by `TestReadComboboxValue_IgnoresDataValueWhichMirrorsTypedText`, which asserts the JS does not mention `data-value` at all, so the fallback cannot be reintroduced.

**Second defect fixed here:** a verification *error* fell through the retry loop's condition entirely (`vErr == nil && !landed`), recording the field as neither landed nor failed — it vanished from the logs and no commit was attempted. An unverifiable field is now treated as not set and logged.


---

## 80. [The retry loop logged the payload size but never which fields were still invalid](#80-the-retry-loop-logged-the-payload-size-but-never-which-fields-were-still-invalid)

**Table rationale cell (original):** Hit the wall directly: `Attempt 2 applied 13/13 validation fix(es)` with **no** not-landed line at all, and the form still bounced — `7212 -> 7281 chars`. Every field reported as filled, nothing reported as failed, and the submission was still rejected. The byte count cannot distinguish "the same fields are still failing" from "different ones now are", so the next step would have been another blind ~25-minute cycle. `InvalidFieldIdentifiers` now names them

### 80. The retry loop logged the payload size but never which fields were still invalid (Resolved 2026-07-25)

**Filed the moment the existing diagnostics ran out.** With every earlier fix in place:

```
16:15:33 Attempt 2 applied 13/13 validation fix(es) to: #430 ... #question_67942420
16:15:34 Submission failed validation. Retrying...
16:15:34 Narrowed validation retry to the rejected fields only (54606 -> 7281 chars)
```

Read that carefully: **13 of 13 applied, no "left the control empty" line, no "Validation fix failed" line — and the form still rejected the submission.** Every signal the system had said success, and the outcome was failure. The only remaining number, the payload size, went 7212 → 7281, which is uninterpretable: it cannot distinguish "the same fields are still failing" from "a different set is now failing".

The next step from here would have been another blind ~25-minute cycle. That is the same trap #70, #76 and #77 each sprang in turn, and the same fix applies: **stop inferring, start naming.**

`parser.InvalidFieldIdentifiers` walks the narrowed payload and lists the controls the page flagged, by `id`, falling back to `name` and then the tag so a control can never be silently omitted. The retry loop logs them alongside the size:

```
Narrowed validation retry to the rejected fields only (54606 -> 7281 chars); still invalid: 430, 431, question_67942418, ...
```

**Not a code defect in itself** — it is the missing measurement that was preventing the *next* defect from being found. Filed as a bug rather than an improvement because the absence was actively blocking diagnosis of a Blocker-severity failure.

**Tests:** `TestInvalidFieldIdentifiers_NamesTheRejectedControls`, `TestInvalidFieldIdentifiers_FallsBackToNameThenTag`.


---

## 79. [The option wait watched an unrelated widget, and committing option-0 filed the wrong location](#79-the-option-wait-watched-an-unrelated-widget-and-committing-option-0-filed-the-wrong-location)

**Table rationale cell (original):** Found with a standalone Playwright probe against Reddit's real form, after the 12-min-per-guess loop became untenable. Two defects: **(a)** the options wait counted `[role="option"]` **document-wide**, and every Greenhouse page carries an always-open intl-tel-input phone-country widget holding ~244 options — so the count was permanently non-zero, the wait returned instantly, and every commit fired into an empty menu. **(b)** far worse, committing the *focused* option is unsafe: typing `Macomb` puts **"Macomb, Illinois, United States"** at option-0 while the configured address is Michigan, so a successful commit would file real applications with the wrong location

### 79. The option wait watched an unrelated widget, and committing option-0 filed the wrong location (Resolved 2026-07-25, verified against the live form)

**Found by abandoning the guess-and-wait loop.** Each hypothesis was costing ~12 minutes of inference to test through the agent, so I built a standalone Playwright probe against Reddit's real form. Feedback dropped to ~30 seconds and both defects fell out immediately.

**(a) The options wait was watching the wrong widget.** #77 polled `document.querySelectorAll('[role="option"], .select__option')`. Probe output:

```
#candidate-location typed="Macomb Township, MI"
  activedescendant=""   options=[Afghanistan+93, Åland Islands+358, Albania+355, ...]
```

Those are **dial codes**. Every Greenhouse page carries an always-present intl-tel-input phone-country widget whose menu holds ~244 options at all times, so a document-wide count is *permanently* non-zero. The wait returned instantly, every time, and each commit fired into a menu that had not opened. Now resolved through the input's own `aria-controls` (falling back to the listbox implied by `aria-activedescendant`), so it can only ever see the widget being driven.

**(b) Committing the focused option is unsafe.** This is the serious one. Typing `Macomb` returns:

```
option-0  Macomb, Illinois, United States      <-- wrong state
option-2  Township of Macomb, Michigan, ...    <-- the configured address
```

The configured address is in **Michigan**. An earlier probe run pressed Enter and committed *Macomb, Illinois* — meaning that had the commit "worked" at any point today, it would have filed real job applications with the wrong location. A silent wrong answer is worse than the visible failure it replaced.

`pickComboboxOption` now requires every token in `mustContain` (for location: the city's first word and the spelled-out state) and otherwise requires option and configured value to contain one another. **If nothing matches, nothing is selected** — the field is left to the validation-retry loop rather than filled with something wrong.

Also fixed here: the configured value frequently cannot be typed in full, because these widgets filter by substring against their own labels. `"United States of America"` matches nothing against a list whose entry is `"United States"`; `"Macomb Township, MI"` matches nothing at all. `searchPrefixes` shortens the query word by word until the list responds.

**Verified against the live form**, not merely unit-tested:

```
LOCATION: "Macomb" -> 7 options -> picked option-2
  => COMMITTED "Township of Macomb, Michigan, United States"
COUNTRY:  "United States" -> picked option-0
  => COMMITTED "+1"
```

**Tests:** `TestPickComboboxOption_RejectsTheWrongStateEvenWhenItIsFirst` (pins the safety property), `TestPickComboboxOption_SelectsNothingWhenNoOptionMatches`, `TestPickComboboxOption_MatchesAShorterListLabelAgainstALongerConfiguredValue`, `TestSearchPrefixes_ShortensUntilSomethingCanMatch`, `TestNormalizeOptionText_StripsDialCodeAndPunctuation`.


---

## 78. [Fill() never opens a react-select menu, and the read-back matched the input itself](#78-fill-never-opens-a-react-select-menu-and-the-read-back-matched-the-input-itself)

**Table rationale cell (original):** Probed directly: `Fill()` sets `input.value` while react-select's menu **never opens** — the widget's own option count stayed 0 and `aria-activedescendant` stayed empty for 3 full seconds, so the Enter that followed had nothing to select. Real keystrokes open and filter it in ~600ms. Separately, react-select sets `role="combobox"` **on the input**, and `Element.closest()` tests the element itself first — so the value read resolved its "shell" to the input, which has no children, and never found the committed value. Same DOM, corrected: `""` → `"Macomb, Illinois, United States"`

### 78. Fill() never opens a react-select menu, and the read-back matched the input itself (Resolved 2026-07-25, verified against the live form)

**Two independent reasons the combobox commit could never have worked**, both established by direct observation of the live DOM rather than inference.

**(a) `Fill()` does not open the menu.** Probed on Reddit's form: after `Fill()` succeeded, the widget's own option count stayed **0** and `aria-activedescendant` stayed **empty for a full 3 seconds**. react-select opens and filters its menu in response to real key events, not to a programmatic value set. The Enter that followed therefore had nothing to select. Clicking the control and typing produces options and a focused option within ~600ms. `setComboboxValue` now clicks, clears, and types.

**(b) The value read matched the input itself.** react-select sets `role="combobox"` **on the input element**, and `Element.closest()` tests the element before its ancestors. So `el.closest('.select__control, .select-shell, [role="combobox"]')` returned **the input** — which has no children — and the `.select__single-value` lookup inside it found nothing. The read-back reported `""` even when the DOM plainly contained `option Macomb, Illinois, United States, selected.` and a populated `.select__single-value`.

Proven by toggling only that one expression against identical DOM:

```
role attr on the input itself = "combobox"   <-- closest() matches self
before: COMMITTED VALUE = ""
after:  COMMITTED VALUE = "Macomb, Illinois, United States"
```

The shell lookup now prefers the container classes and only considers a `role="combobox"` ancestor **via `el.parentElement`**, so it can never resolve to the input.

**Why this mattered so much:** (b) made #74, #75 and #77 all report failure even where they had succeeded, which is why three consecutive fixes looked inert. It is the same class of error as #76 — a diagnostic lying about its own subject.


---

## 77. [Enter was pressed before react-select had loaded any option, so the commit selected nothing](#77-enter-was-pressed-before-react-select-had-loaded-any-option-so-the-commit-selected-nothing)

**Table rationale cell (original):** Caught the moment #76 made the read-back work: `Attempt 2: 11 fix(es) reported success but left the control empty ... 430, 431, 432, 433, 434, 436, candidate-location, country, question_67942418/19/20` — with **no** commit line, so `commitComboboxOnLocator` pressed Enter and still committed nothing. react-select populates its menu asynchronously (Greenhouse's Location field queries a geocoder), so an Enter fired immediately after `Fill()` arrives while the menu is empty, highlights nothing and selects nothing. Real progress in the same run though: the narrowed payload **shrank for the first time**, 8249 → 5988 chars

### 77. Enter was pressed before react-select had loaded any option, so the commit selected nothing (Resolved 2026-07-25, live confirmation pending)

**Caught the moment #76 made the read-back actually work.** The run carrying #76 produced a line that had never appeared before:

```
14:17:51 Attempt 2 applied 15/15 validation fix(es) to: 430 ... candidate-location, country ...
14:17:51 Attempt 2: 11 fix(es) reported success but left the control empty (autocomplete/combobox suspected):
         430, 431, 432, 433, 434, 436, candidate-location, country, question_67942418, question_67942419, question_67942420
14:17:53 Narrowed validation retry to the rejected fields only (54359 -> 5988 chars)
```

Two things to read off this:

1. **#76 works.** Before it, all 15 fields reported as landed; now 11 are correctly identified as still empty. The diagnostic is finally telling the truth.
2. **The commit still does nothing.** Those 11 went to `notLanded`, not to the committed list — so `commitComboboxOnLocator` ran, pressed Enter, and re-read an *empty* control. Enter is reaching the widget and selecting nothing.

**Root cause:** react-select populates its menu asynchronously. Greenhouse's Location field is geocoder-backed, so options arrive over the network. `Fill()` returns as soon as the text is typed, and the Enter that follows lands while the menu is still empty — nothing is highlighted, so nothing is selected. The custom-question fields (`430`-`436`, `question_679424xx`) are react-select too and fail identically.

**Fix:** `waitForComboboxOptions` polls for `[role="option"], .select__option` before the keypress, on a 5s budget at 250ms intervals. It searches document-wide because react-select renders its menu in a portal as often as inline. Best-effort: on timeout the Enter is still attempted, and the read-back afterwards remains the thing that actually decides whether anything committed — so a slow or genuinely optionless widget degrades to the previous behaviour rather than erroring.

**Genuine progress in the same run, worth recording:** the narrowed invalid-field payload **shrank for the first time in this entire investigation** — 8249 → 5988 chars, −28%. Every previous attempt held flat or grew (8249 → 8334). Four of the 15 fields are now being satisfied. The direction finally reversed.

**Method note, again:** this was very nearly missed. The `left the control empty` line was **not** in the log-monitor's filter, so it never surfaced as a notification — it was found only by grepping the log directly after the payload size dropped unexpectedly. Filter widened. **An absent notification is not evidence of an absent event; check the filter before concluding anything from silence.**

**Tests:** `TestCommitComboboxOnLocator_WaitsForOptionsBeforePressingEnter`.


---

## 76. [#74's own read-back checked el.value first, silently disabling the combobox commit it had just added](#76-74s-own-read-back-checked-elvalue-first-silently-disabling-the-combobox-commit-it-had-just-added)

**Table rationale cell (original):** **A defect in #74's fix, caught live by the absence of an expected log line.** Reddit logged `Attempt 2 applied 15/15 validation fix(es)` with **no** `committed N autocomplete selection(s)` line and **no** `left the control empty` line — so `verifyFixLanded` reported every field as landed and the commit step never ran once. Cause: the value read checked `el.value` before the combobox branch, and a react-select search input genuinely holds the typed text after `Fill()`. #74 was therefore inert on exactly the fields it was written for, and #75 inherited the same inertness

### 76. #74's own read-back checked el.value first, silently disabling the combobox commit it had just added (Resolved 2026-07-25, live confirmation pending)

**A defect in #74's fix, and it was caught by a log line that did not appear.** The run carrying #74 produced:

```
13:49:18 Attempt 2 applied 15/15 validation fix(es) to: 430 ... candidate-location, country ...
13:49:20 Submission failed validation. Retrying...
```

No `committed N autocomplete selection(s)` line. No `reported success but left the control empty` line. Both are absent, and that combination is only possible one way: `verifyFixLanded` returned **true for all 15 fields**, so the combobox branch was never entered. #74 was inert on precisely the fields it was written for, and #75 — built on the same read-back — inherited the inertness.

**Root cause:** the read script tested `el.value` before the combobox branch:

```js
if (el.value) return String(el.value);          // <-- wrong for a combobox
const shell = el.closest('.select__control, ...');
```

After `Fill()`, a react-select search input **does** hold the typed text. So `el.value` was `"Detroit, MI"`, non-empty, and the control was declared satisfied — while the widget's committed selection was still empty and the form still rejected it. The check that was supposed to detect "typed but not committed" was reading the very artifact of typing.

**Fix:** split into `readInputValueJS` and `readComboboxValueJS` (which never looks at `el.value`), and moved the choice between them into Go, in `locatorHasValue`. Branching inside one JS blob is what made the ordering untestable and let this ship in the first place; the decision is now a plain Go conditional with `isComboboxLocator`, and is unit-tested directly.

**Severity Blocker:** it rendered two other shipped fixes completely inert without any error, on required fields that no retry could otherwise satisfy.

**Lesson worth keeping:** the diagnostic that caught this was an *absent* log line, not a failing one. Both #74 and #75 looked correct in isolation and had passing tests. What exposed the defect was checking whether the fix actually announced itself at runtime — worth doing deliberately after any fix whose whole purpose is to fire on a specific condition.

**Tests:** `TestLocatorHasValue_ReadsAComboboxWithTheWidgetScriptNotElValue`, `TestLocatorHasValue_ReadsAPlainInputWithElValue`.


---

## 75. [#74's combobox commit was wired into the retry path but not the initial fill, guaranteeing a wasted retry cycle](#75-74s-combobox-commit-was-wired-into-the-retry-path-but-not-the-initial-fill-guaranteeing-a-wasted-retry-cycle)

**Table rationale cell (original):** The identical structural gap **#67** found, one layer up: `safeFillWithLabelFallback`'s three tiers all use plain `Fill()`, so a react-select field is typed into but never committed on the first pass. Since `Location (City)` and `Country` are required on every Greenhouse form, the first submit was **guaranteed** to bounce and force a full validation-retry cycle — ~12 minutes of inference on this machine — to commit something a single keypress could have done immediately. Confirmed live at 13:36: the run carrying #74 still bounced on attempt 1 with the narrowed payload at exactly 8249 chars, unchanged from the run before it

### 75. #74's combobox commit was wired into the retry path but not the initial fill, guaranteeing a wasted retry cycle (Resolved 2026-07-25, live confirmation pending)

**This is precisely the structural gap bug #67 found, recurring one layer up** — a capability added to the validation-*retry* path and never wired into the initial fill.

`safeFillWithLabelFallback`'s three tiers (accessible label → placeholder → CSS selector) all set values with a plain `Fill()`. Per #74 that types into a react-select's search box and commits nothing. `Location (City)` and `Country` are required on every Greenhouse form, so **the first submit was guaranteed to bounce** — not because anything went wrong, but because the first pass structurally could not satisfy two required fields.

The cost is not the bounce itself, it is what the bounce buys: a full validation-retry cycle is a `SolveValidationErrors` call, which on this machine is **~12 minutes of inference**, to commit something a single keypress could have done in the first pass. Multiplied across every Greenhouse posting in a ~3,100-job backlog where all LLM calls serialise, this is one of the largest avoidable time sinks in the system.

**Confirmed live at 13:36**, on the run already carrying #74: attempt 1 bounced with the narrowed payload at exactly **8249 chars — byte-for-byte the same size as the run before it**. #74 fixed the retry, so the job would eventually recover, but the wasted first pass was untouched.

**Fix:** extracted `locatorHasValue` and `commitComboboxOnLocator` so the label/placeholder tiers — which never have a selector string to re-resolve — can run the same check as the selector tier. `commitFilledCombobox` runs after every successful initial fill and is **best-effort**: it never fails an otherwise-good fill, since the validation-retry path remains as a backstop. It stays as narrow as #74's version, firing only inside `.select__control` / `.select-shell` / `role="combobox"`.

**Tests:** `TestSafeFillWithLabelFallback_CommitsAComboboxOnTheInitialFill`, `TestSafeFillWithLabelFallback_DoesNotPressEnterOnPlainInputs`.

**Worth noting as a pattern:** #65/#66 → #67, and now #74 → #75. Twice, a fill capability has been added to the retry path only. Any future change to how a control is set should be checked against *both* paths before it is called done.


---

## 74. [react-select comboboxes were filled but never committed, so their validated value stayed empty](#74-react-select-comboboxes-were-filled-but-never-committed-so-their-validated-value-stayed-empty)

**Table rationale cell (original):** #72's autocomplete hypothesis, confirmed structurally by fetching Reddit's real Greenhouse markup: `Location (City)` and `Country` are **react-select** widgets (`select__control`, `select__input-container[data-value]`, `react-select-candidate-location-live-region`). The `<input id="candidate-location">` is a *search* box — the chosen value lives in widget state and renders into a sibling `.select__single-value`. `Fill()` types the search text and commits nothing, so the value the form validates stays empty, and reading `el.value` back reports `""` whether or not a selection succeeded. These are required fields, so the form could never pass

### 74. react-select comboboxes were filled but never committed, so their validated value stayed empty (Resolved 2026-07-25, live confirmation pending)

**This is #72's autocomplete hypothesis, promoted to a root cause by evidence rather than inference.** Fetched Reddit's actual Greenhouse page and read the markup for the two fields that resolved fine on attempt 3 and still bounced:

```html
<label id="candidate-location-label" for="candidate-location">Location (City)<span aria-hidden="true">*</span></label>
<div class="select-shell remix-css-b62m3t-container">
  <span id="react-select-candidate-location-live-region" ...></span>
  <div class="select__control remix-css-13cymwt-control">
    <div class="select__value-container">
      <div class="select__placeholder" id="react-select-candidate-location-placeholder"></div>
      <div class="select__input-container" data-value="">
        <input class="select__input" ...>
```

It is **react-select**. `<input id="candidate-location">` is the widget's *search* box, not its value. The committed selection lives in React state and is rendered into a sibling `.select__single-value`; `.select__input-container[data-value]` mirrors it.

**Two consequences, both fatal:**
1. `Fill()` types search text and commits nothing. The value the form validates stays empty. `Location (City)` and `Country` are both **required** (note the `*`), so the form could never pass, no matter how many retries.
2. `verifyFixLanded` (added in #72) reads `el.value` — which is `""` *whether or not a selection succeeded*. So #72's own read-back would have reported a false negative on a working combobox. Fixed here as part of the same change.

**Fix:** `readControlValueJS` now looks past the input to the widget — `.select__single-value`, `.select__multi-value__label`, then `[data-value]` — before concluding a control is empty. `commitComboboxSelection` presses `Enter` to commit the focused option and re-reads. It is **deliberately narrow**: it fires only when the control is inside `.select__control` / `.select-shell` or carries `role="combobox"`, because a stray `Enter` in an ordinary text input can submit the form before the remaining fixes are applied. Pinned by `TestCommitComboboxSelection_LeavesPlainInputsAlone`.

The retry loop now logs `committed N autocomplete selection(s) that Fill() alone had left empty`.

**Status is "live confirmation pending" deliberately.** The markup evidence is direct and the mechanism is not in doubt, but no live run has yet produced a confirmed `APPLIED` through this path. Do not mark it verified until one does.

**Tests:** `TestCommitComboboxSelection_PressesEnterOnAComboboxAndConfirms`, `TestCommitComboboxSelection_LeavesPlainInputsAlone`.


---

## 73. [A CSS id selector cannot start with a digit, so Greenhouse's numeric custom-question ids were unfillable half the time](#73-a-css-id-selector-cannot-start-with-a-digit-so-greenhouses-numeric-custom-question-ids-were-unfillable-half-the-time)

**Table rationale cell (original):** Caught live on Reddit's third and final attempt: 6 of 15 fixes failed with `selector matched no element` for `input#430`, `input#431`, `input#432`, `input#433`, `input#434`, `input#436`. `#430` is a **CSS syntax error** — an id selector cannot begin with a digit — yet Greenhouse numbers its custom-question controls exactly that way. `resolveFieldLocator` only tried its attribute-form fallbacks when the selector did *not* look like CSS, and `input#430` contains `#`, so it was used verbatim and matched nothing. The model sent bare `430` on attempt 2 (resolved fine) and `input#430` on attempt 3 (dead), so the same field alternated between fillable and unfillable across attempts of the same job

### 73. A CSS id selector cannot start with a digit, so Greenhouse's numeric custom-question ids were unfillable half the time (Resolved 2026-07-25)

**Caught live on Reddit's third and final attempt**, minutes after #72 shipped:

```
13:13:54 Validation fix for "input#434" failed: selector matched no element (tried 1 form(s) of "input#434")
13:13:54 Validation fix for "input#430" failed: selector matched no element (tried 1 form(s) of "input#430")
13:13:55 Validation fix for "input#431" failed: ...   (same for #432, #433, #436)
13:13:55 Attempt 3 applied 9/15 validation fix(es) to: #gdpr_demographic_data_consent_given_1,
         input#candidate-location, input#country, input#question_67942415 ... question_67942420
13:13:57 Auto-Submit failed for Reddit: failed to submit application after 3 validation error attempts
```

**Root cause:** `#430` is not a valid CSS selector. A CSS identifier may not begin with a digit — `document.querySelector("#430")` throws a `SyntaxError`. Greenhouse nevertheless numbers its custom-question controls exactly that way (`id="430"`). The `[id="430"]` attribute form has no such restriction and matches perfectly.

`resolveFieldLocator` built its attribute-form fallbacks only under `if !looksLikeCSSSelector(selector)`. `input#430` contains `#`, so it "looks like CSS", so the fallbacks were skipped and the invalid selector was used verbatim — note `tried 1 form(s)` in the log, versus the 5 forms a bare identifier gets.

**The tell that makes this unambiguous:** the model sent bare `430` on attempt 2 and `input#430` on attempt 3, for the same field on the same form. Attempt 2 resolved it (via #66's bare-identifier fallbacks); attempt 3 could not. The same control alternated between fillable and unfillable purely on how the model happened to phrase the selector — which also explains why #72's `15/15 applied` on attempt 2 dropped to `9/15` on attempt 3 with no change to the page.

**Fix:** added `splitTagID`, which decomposes a simple `tag#id` / `#id` selector and refuses anything more complex (descendant combinators, attribute filters, class chains, selector lists) where a naive rewrite would change meaning. When the verbatim selector is CSS-shaped, `resolveFieldLocator` now also queues `tag[id="..."]`, `[id="..."]` and `[name="..."]`. The verbatim selector is still tried first, so nothing that worked before changes behaviour.

**Tests:** `TestResolveFieldLocator_FallsBackToAttributeFormForNumericIDs`, `TestSplitTagID` (table-driven, pins the refusal cases too).

**Note on what this does *not* explain:** `input#candidate-location` and `input#country` resolved fine on attempt 3 and the form still bounced. #72's autocomplete/combobox hypothesis remains live and unconfirmed for those two.


---

## 72. [The retry loop counts empty-valued and non-landing fixes as applied, reporting progress it is not making](#72-the-retry-loop-counts-empty-valued-and-non-landing-fixes-as-applied-reporting-progress-it-is-not-making)

**Table rationale cell (original):** Found immediately after #70 shipped, from the diagnostic #70 added: Reddit logged `Attempt 2 applied 15/15 validation fix(es)` and **still bounced**, with the narrowed payload essentially unchanged (8249 → 8334 chars) — i.e. the same fields were still invalid. Two accounting defects make that tally untrustworthy: `applyValidationFix` returns `nil` for an empty value (correct for the *initial* fill path, a lie in the retry path), and a `nil` return only means Playwright accepted the call, not that the control ended up set

### 72. The retry loop counts empty-valued and non-landing fixes as applied, reporting progress it is not making (Resolved 2026-07-25, accounting fixed; underlying non-convergence still under live investigation)

**Found by the diagnostic #70 added, within an hour of it shipping** — which is the point of that diagnostic. Reddit, re-run against #70's fix:

```
12:52:05 Narrowed validation retry to the rejected fields only (54877 -> 8249 chars)
13:04:18 Attempt 2 applied 15/15 validation fix(es) to: 430, 431, 432, 433, 434, 436,
         candidate-location, country, gdpr_demographic_data_consent_given_1,
         question_67942415, question_67942416, question_67942417, question_67942418,
         question_67942419, question_67942420
13:04:19 Submission failed validation. Retrying...
13:04:19 Narrowed validation retry to the rejected fields only (54748 -> 8334 chars)
```

**#70's fix did work** — the narrowed payload grew 5,363 → 8,249 chars on the identical form, which is the error text now reaching the model. But 15/15 fixes "applied" and the form still bounced, with the invalid-field payload essentially unchanged (8249 → 8334). The same fields are still invalid, so the tally is not measuring what it claims.

**Two accounting defects, both proven from code:**

1. **An empty value counts as applied.** `applyValidationFix` returns `nil` when `value == ""`. That is *correct* for the initial-fill path, where it means "the profile has no data for this field, skip it" — `safeFillWithLabelFallback` depends on it. In the retry path the same return is a lie: the field was just rejected, and proposing `""` cannot satisfy it. The contract could not simply be changed, since both paths share the function; fixed at the retry call site instead.
2. **A `nil` return does not mean the control ended up set.** It means Playwright accepted the call. The known gap is ATS autocomplete widgets — `candidate-location` and `country` in the list above are exactly Greenhouse's — where the visible text box is backed by a separate hidden field, so setting it without choosing a suggestion leaves the value the form actually validates completely unset.

**Fix:** the retry loop now skips empty values (logging them separately as `model proposed an empty value for N rejected field(s)`), and `verifyFixLanded` reads each control back after the fix, logging any that report success but left the control empty as `N fix(es) reported success but left the control empty (autocomplete/combobox suspected)`. The read-back test is deliberately "is it non-empty now" rather than strict equality — forms legitimately reformat phone numbers and dates, and a `<select>` set by visible label reports its underlying value, so equality would cry wolf on fixes that did land.

**Deliberately still open:** this fixes the *measurement*, not necessarily the underlying non-convergence. The autocomplete theory is a hypothesis consistent with the selector list, not yet a confirmed root cause. The next Reddit attempt will name the offending selectors outright, and that becomes the next bug — which is exactly the position #70 was in an hour ago, and it was right then.

**Tests:** `TestVerifyFixLanded_DetectsAControlLeftEmpty`, `TestVerifyFixLanded_AcceptsAReformattedValue`, `TestVerifyFixLanded_TreatsAnUncheckedBoxAsNotLanded`.


---

## 71. [firstVisibleLocator's .First() fallback reintroduces the very hang it was written to prevent, at the submit click](#71-firstvisiblelocators-first-fallback-reintroduces-the-very-hang-it-was-written-to-prevent-at-the-submit-click)

**Table rationale cell (original):** Found auditing the cohort's `FAILED_SUBMIT` rows during #70's restart: Zimperium (Lever, scored **85**) died on `playwright: timeout: Timeout 30000ms exceeded` waiting for `locator(...).first()`. The `.first()` in Playwright's own call log is the proof — `firstVisibleLocator` found *no* visible match, fell back to `loc.First()`, and the caller clicked a match already known to be invisible (#59's hidden `hcaptchaSubmitBtn`). Guaranteed to burn the full action timeout and then misreport "no visible submit button here" as a generic timeout, which is why it read as CPU/network flakiness

### 71. firstVisibleLocator's .First() fallback reintroduces the very hang it was written to prevent, at the submit click (Resolved 2026-07-25)

**Found while auditing the 82-cohort's `FAILED_SUBMIT` rows** before restarting the run for #70 — the count had grown 6 → 7 and the newcomer was not one of the five known-dead postings. Zimperium (`jobs.lever.co/zimperium/18699ad3...`), fit score **85**:

```
09:05:51 Detected Lever ATS. Filling out fields...
09:06:02 Submission failed validation. Retrying...
09:06:02 Attempt 2: Solving validation errors...
09:33:28 Auto-Submit failed for Zimperium: playwright: timeout: Timeout 30000ms exceeded.
Call log:
  - waiting for locator('input[type=\'submit\'], button[type=\'submit\'], button:has-text(\'Submit\'), button:has-text(\'Apply\')').first()
```

**Root cause:** the `.first()` in Playwright's own call log is the tell. `firstVisibleLocator` walks the matches looking for a visible one and, finding none, falls back to `return loc.First()`. The caller then clicks that match — which is *known to be invisible*, because the loop just checked every single one. Playwright waits for it to become actionable, it never does, and the click burns the full `fillActionTimeoutMs`.

This is precisely the hang the function's own doc comment says it exists to prevent (bug #59: Lever's `<button type="submit" class="hidden" id="hcaptchaSubmitBtn">" being clicked ahead of the real button). #59 fixed the "picks the wrong match" half and left the "picks a known-bad match anyway" half in the fallback.

Two costs: 30 wasted seconds per occurrence, and — worse — the failure surfaces as a bare `Timeout 30000ms exceeded`, which reads as CPU contention or a slow page. It is nothing of the sort. It means *there is no visible submit control on this form*, which is a completely different and actionable diagnosis.

**Fix:** added `firstVisibleSubmit`, which is `firstVisibleLocator` without the fallback — it returns `(locator, ok)`. The submit-click site now fails immediately with `found N submit control(s) but none visible` instead of clicking a hidden element. `firstVisibleLocator` is reimplemented on top of it and keeps its fallback for the two *fill* call sites, where attempting a hidden element is still worth doing; their existing tests are unchanged and still pass.

**Not yet known, deliberately left open:** *why* Lever presented no visible submit control at that moment. The old error message made that question unaskable. It should now be answerable from the logs the next time it happens — and Zimperium is requeued to find out.

**Tests:** `TestFirstVisibleSubmit_ReportsWhenNoMatchIsVisible`, `TestFirstVisibleSubmit_ReportsTheVisibleMatch`.


---

## 70. [The validation-retry loop strips the page's own error text, so the model never learns why a field bounced](#70-the-validation-retry-loop-strips-the-pages-own-error-text-so-the-model-never-learns-why-a-field-bounced)

**Table rationale cell (original):** Caught live on the highest-fit job in the 82-cohort (Reddit, scored **90**), which burned **17.5 minutes** and 3 LLM calls to fail with `failed to submit application after 3 validation error attempts`. `aria-describedby` was stripped as "presentational" *before* `PruneDOMToInvalidFields` ran, so the link from a rejected control to its error message was severed, and the message element was then dropped as neither control nor label. The model was told a field was invalid but never what would make it valid, so it re-guessed the same value each attempt. Compounded by an empty fix map falling through to a re-submit of a byte-identical form. This is the terminal step of the whole pipeline — it fails *after* discovery, scoring and fill have all succeeded

### 70. The validation-retry loop strips the page's own error text, so the model never learns why a field bounced (Resolved 2026-07-25)

**Caught live, in the act**, while monitoring the 82-job re-verification run on 2026-07-25. Reddit (`job-boards.greenhouse.io/reddit/jobs/8044767`) scored **90** — the highest-fit job in the entire cohort — and then died at the last step:

```
12:14:55 Fit Score Pipeline: Reddit scored 90! Proceeding with application.
12:15:03 Submission failed validation. Retrying...
12:15:03 Narrowed validation retry to the rejected fields only (53366 -> 5363 chars)
12:27:16 Submission failed validation. Retrying...
12:27:16 Narrowed validation retry to the rejected fields only (53228 -> 5439 chars)
12:32:28 Submission failed validation. Retrying...
12:32:28 Auto-Submit failed for Reddit: failed to submit application after 3 validation error attempts
```

**17.5 minutes and 3 LLM calls for nothing.** The tell is in the numbers: between attempts the form barely changed (53366 → 53228) and the narrowed slice *grew* (5363 → 5439). If the fixes were landing, the invalid-field set should shrink. It did not — the same fields were being rejected every time.

**Root cause — two independent defects on the same path:**

1. **`aria-describedby` was in `presentationalAttrs`** (`pkg/parser/dom.go`), stripped as bloat by `StripPresentationalAttrs`. But that runs at `pkg/submitter/browser.go` *before* `PruneDOMToInvalidFields`. `aria-describedby` is the WCAG-standard pointer from a rejected control to the element holding the page's explanation of the rejection. Stripping it severed the link; the pruner then dropped the error element itself, since it is neither an invalid control nor a `<label>`. Net effect: the model received `<input name="phone" aria-invalid="true">` with the label "Phone" and **no statement of what was wrong with it** — so it re-proposed a plausible value, which bounced identically. #64's narrowing made this strictly worse: before it, the full form at least carried the error text somewhere in the payload.

2. **An empty fix map fell through to a re-submit.** The guard read `if !appliedAny && len(fixesMap) > 0`. When the model proposed *nothing* (a legitimate outcome — `SolveValidationErrors` returns a nil map for a `null`/`{}` response with no error), `len(fixesMap) == 0`, the guard did not fire, and control fell straight through to the submit click — re-submitting a byte-identical form and burning another ~6-12 minutes. Note the comment already sitting above that block from #65 states the exact failure mode ("the next attempt re-sends an identical payload and the loop is guaranteed to exhaust itself"); the guard just did not cover the empty case.

**Fix:**
- Removed `aria-describedby` from `presentationalAttrs`, with the same style of carve-out comment `aria-invalid` already carries.
- `PruneDOMToInvalidFields` now follows `aria-describedby` **and** `aria-errormessage` (both space-separated id lists per WCAG) and emits **label → control → error text grouped per rejected field**, so the model never has to re-associate them by id across a flat list.
- Empty fix map is now a hard failure (`model proposed no fixes for the rejected fields`) instead of a futile re-submit.
- Added an `Attempt N applied X/Y validation fix(es) to: <selectors>` log line. **Selectors only, never values** — the values come from the PII profile and the log is not a place for them. Without this, a non-converging retry loop is undiagnosable from logs, which is exactly why this bug survived until someone watched it happen in real time.

**Tests:** `TestPruneDOMToInvalidFields_KeepsAriaDescribedByErrorText` and `TestPruneDOMToInvalidFields_KeepsAriaErrorMessageText` (both verified failing before the fix — the error text was provably absent from the output). `TestStripPresentationalAttrs_RemovesStylingAndStateAttrs` had its `aria-describedby` assertion removed and replaced with a pointer to the new tests, so the carve-out cannot be silently reverted.

**Severity Blocker, not Major:** this is the terminal step of the entire pipeline. It fails *after* discovery, scoring, document generation and form fill have all succeeded, on the jobs the agent rated highest — and it consumes the single most expensive resource in the system (~6 min of inference per wasted attempt, on a machine where all LLM calls serialise).


---

## 69. [Discovery stored the searched role as job_title and discarded the real headline](#69-discovery-stored-the-searched-role-as-job_title-and-discarded-the-real-headline)

**Table rationale cell (original):** Found while auditing throughput: **55 distinct titles across 3,131 waiting rows**. The SerpAPI path called `AddToFunnel(company, role, ...)` — storing the *search term* — while `result.Title` (the real headline) was logged one line earlier and thrown away. Beyond misleading logs and dashboard, this degraded improvements.md #22, which ranks the queue by embedding title+company: every job found under the same role got a near-identical embedding

### 69. Discovery stored the searched role as job_title and discarded the real headline (Resolved 2026-07-25)
**Found while auditing why throughput is the binding constraint** (3,131 jobs waiting at ~10 min each ≈ 22 days of continuous compute). Checking whether cheap title-based pre-filtering could skip irrelevant rows turned up something odd: `SELECT COUNT(DISTINCT job_title)` over the 3,131 waiting rows returned **55** — suspiciously close to the length of the configured roles list, and the sample was "Senior Backend Engineer" repeated ten times over.

**Root cause:** `pkg/scraper/funnel.go`'s SerpAPI path called `storage.AddToFunnel(company, role, result.Link, ...)`, passing the **searched role** as the job title. The real headline was available the whole time — `result.Title` is logged on the line immediately above and then discarded — and `extractCompanyFromTitle` already parses the company out of that very string.

**Why it matters beyond cosmetics:** improvements.md #22 (shipped 2026-07-24) ranks the discovery queue by embedding `title + company`. If the title is one of 55 role names rather than the posting's real title, every job discovered under the same search role produces a near-identical embedding, and the ranking degenerates toward "order by which role was searched" — quietly undermining a feature that had just shipped. It also made title-based pre-filtering impossible, and made logs and the dashboard misleading about what a row actually is.

**Fix:** new `extractJobTitleFromResult` reads the title from the same headline `extractCompanyFromTitle` reads the company from ("Senior Backend Engineer at Stripe - Lever" → title before " at ", company after), with a secondary "Title - Company - ATS" form, falling back to the searched role when the headline cannot be parsed so a row always carries something meaningful.

**Deliberately not changed — the Yahoo fallback.** That path parses raw anchor hrefs and has no result headline at all, so the searched role genuinely is the only label available. Left as-is with a comment saying why, rather than inventing a title from the URL slug.

**Limitation, stated plainly:** this corrects discovery *going forward*. The 3,131 existing rows keep their role-as-title values, and their `fit_similarity` scores remain computed from those, so queue ranking stays weak for the current backlog. Re-deriving real titles for those would mean re-fetching every posting; not worth it. Expect ranking quality to improve only as new discoveries replace the old backlog.

Tests: `TestExtractJobTitleFromResult` (both headline shapes plus three unparseable cases falling back to the role), `TestTitleAndCompanyExtractorsAgree` (pins that the two extractors keep reading the same headline consistently). `go build/vet/test ./...` all pass, 10 packages, 0 failures.


---

## 68. [SaveFormMapping cached semantically-empty mappings, burning a Learner Module call per visit](#68-saveformmapping-cached-semantically-empty-mappings-burning-a-learner-module-call-per-visit)

**Table rationale cell (original):** Bug #21 guards against non-JSON, but an all-null mapping is *valid* JSON. Found live: **7 of 60** cached mappings had every selector null, including `smartrecruiters.com`, `pinpointhq.com` and `applytojob.com`. Each visit to those domains loaded the useless mapping, failed every fill, invalidated the cache and burned a fresh Learner Module call to regenerate the same nulls

### 68. SaveFormMapping cached semantically-empty mappings, burning a Learner Module call per visit (Resolved 2026-07-25)
Bug #21 added a `json.Valid` guard after a cached mapping of prose poisoned a domain. That guard is necessary but **not sufficient**: a response shaped correctly but with every selector `null` is perfectly valid JSON.

**Found live** while auditing why platforms were underperforming: **7 of 60** cached mappings were in this state, including three actively-used platforms — `smartrecruiters.com`, `pinpointhq.com`, `applytojob.com` (plus `yahoo.com`, `breezy.hr` and two Workday hosts that are excluded or auth-gated anyway):
```json
{"fields": {"first_name": null, "last_name": null, "email": null, "phone": null, "submit_button": null}}
```
**Cost:** every visit to such a domain loaded the mapping, failed every fill, correctly invalidated the cache, then spent a fresh multi-minute `ExtractFormMapping` call regenerating the same nulls — a permanent tax with no path to improvement. Not a hard block (the self-healing invalidation works), but pure waste on repeat.

**Fix:** `hasUsableSelector` rejects any mapping with no non-empty selector before it can be cached. Deliberately **tolerant of both shapes** — the nested `{"fields": {...}}` form that `ExtractFormMapping` produces today, and a flat top-level map — because the guard's job is to reject worthless mappings, and mistaking an unfamiliar-but-usable shape for a worthless one would discard good work. The 7 poisoned rows were purged from the live database (60 → 53) so those domains re-map cleanly.

Test: `TestSaveFormMapping_RejectsSemanticallyEmptyMappings` (all-null refused, whitespace-only refused, a mapping with one real selector still cached and round-tripping).


---

## 67. [The initial fill path never received #65/#66's fixes, so required dropdowns always failed the first pass](#67-the-initial-fill-path-never-received-6566s-fixes-so-required-dropdowns-always-failed-the-first-pass)

**Table rationale cell (original):** #65 (dispatch by control type) and #66 (bare-identifier resolution) were both wired only into the validation-*retry* path. `handleDynamic`'s first pass still used `Fill()`-only `safeFill`, so a required `<select>` could never be set on the first attempt and every such form was forced into an avoidable, expensive retry cycle — and custom screening questions that are dropdowns could not be answered at all

### 67. The initial fill path never received #65/#66's fixes, so required dropdowns always failed the first pass (Resolved 2026-07-25)
**Found by asking where else the just-fixed defects applied.** #65 (dispatch by control type) and #66 (bare-identifier resolution) were both wired into `applyValidationFix`, which had exactly **one** call site: the validation-*retry* loop. The initial fill — `handleDynamic`'s standard fields and its custom-screening-question pass — still went through `safeFillWithLabelFallback` → `safeFill`, which is `Fill()`-only and does no selector resolution.

**Consequences on the first pass:**
- A required `<select>` (work authorization, sponsorship, EEO — routine on Greenhouse) **could not be set at all**, so the submission was guaranteed to bounce and enter the expensive validation-retry cycle that #64/#65/#66 exist to survive. The retry was avoidable in the first place.
- Custom screening questions rendered as dropdowns (improvements.md #16 generates real answers for these) could never be applied.
- A bare identifier returned in `mapping.Fields` only worked if the label or placeholder tier happened to match, which masked the problem rather than fixing it.

**Fix:** `safeFillWithLabelFallback`'s CSS tier now routes through `applyValidationFix`, so label → placeholder → **type-aware, resolution-capable** selector fill. The label and placeholder tiers are unchanged and still tried first.

**Behavioural note worth knowing:** `applyValidationFix` resolves the element (checking it exists) before acting, which `safeFill` never did. That is strictly better in production — a missing element now reports "selector matched no element" immediately instead of burning a 30s fill timeout — but it did surface three pre-existing test mocks that had no `countFunc` and therefore reported zero matches. Those mocks were completed rather than the assertion weakened; each test still asserts exactly what it did before.


---

## 66. [SolveValidationErrors returns bare id/name values, not CSS selectors, so every proposed fix matched nothing](#66-solvevalidationerrors-returns-bare-idname-values-not-css-selectors-so-every-proposed-fix-matched-nothing)

**Table rationale cell (original):** Exposed immediately by #65's new per-selector logging: **12 of 12** proposed fixes on one Greenhouse form failed with "selector matched no element", every one a bare identifier (`question_9558065008`, `country`, `candidate-location`). Playwright reads a bare word as a *tag name*, so it searched for `<country>` elements. The model was returning the literal `id`/`name` values it saw in the DOM — correct answers in an unusable form

### 66. SolveValidationErrors returns bare id/name values, not CSS selectors, so every proposed fix matched nothing (Resolved 2026-07-25)
**Found within an hour of #65 shipping, by the logging #65 added.** #65 made every fix failure visible instead of silent, and the very first job to hit the new code path reported `none of the 12 proposed validation fixes could be applied`, followed by twelve lines of:
```
Validation fix for "question_9558065008" failed: selector matched no element
Validation fix for "country" failed: selector matched no element
Validation fix for "candidate-location" failed: selector matched no element
```

**Root cause:** every value is a bare identifier — the literal contents of the element's `id` or `name` attribute — not a CSS selector. Passed to `Loc()`, Playwright interprets a bare word as a **tag name**, so `country` searches for a `<country>` element and `question_9558065008` for a `<question_9558065008>` element. Neither exists, so the match count is always zero. The model was not wrong about *which* fields needed fixing; it returned the right fields in a form the code could not use. This is why #65's dispatch-by-type fix, though correct on its own terms, did not by itself produce a successful submission: the elements were never being found in the first place.

Worth noting *why* the model does this: it is shown a DOM fragment where the attributes literally read `id="question_9558065008"`, and it echoes that value back. Tightening the prompt might help, but prompt wording is a weaker guarantee than accepting both forms in code, so the fix is normalisation rather than instruction.

**Fix:** new `resolveFieldLocator` tries the string exactly as given **first** — so a genuinely correct selector is used unchanged and never mangled into `##foo` — then, only if that matched nothing and the string carries no CSS syntax (`looksLikeCSSSelector`), falls back through `#id`, `[name="..."]`, `[id="..."]`, and `[data-qa="..."]`. `applyValidationFix` now routes through it, so #65's type dispatch operates on an element that was actually found.

Tests: `TestResolveFieldLocator_RecoversBareIdentifiers` (also asserts the raw string is attempted first), `TestResolveFieldLocator_RecoversViaNameAttribute`, `TestResolveFieldLocator_LeavesRealSelectorsAlone`, `TestResolveFieldLocator_ErrorsWhenNothingMatches`, `TestLooksLikeCSSSelector`. `go build/vet/test ./...` all pass, 10 packages, 0 failures.

**Sequence worth remembering:** #64 (timeout) hid #65 (wrong fill method), which hid #66 (unusable selectors). Three distinct blockers stacked on the same code path, each only observable once the one in front of it was cleared.


---

## 65. [Validation fixes were applied with Fill() only and their errors discarded, so required dropdowns could never be satisfied](#65-validation-fixes-were-applied-with-fill-only-and-their-errors-discarded-so-required-dropdowns-could-never-be-satisfied)

**Table rationale cell (original):** **The dominant failure mode of the current run**: 18 of 23 `FAILED_SUBMIT` outcomes were "failed to submit application after 3 validation error attempts". The retry loop called `safeFill` (Fill()-only) and **discarded the returned error**. Playwright rejects `Fill()` on a `<select>`, and Greenhouse-style forms make dropdowns required (work authorization, EEO), so those fields could never be satisfied and nothing was logged. Proven by the narrowed payload being byte-identical across attempts 2 and 3

### 65. Validation fixes were applied with Fill() only and their errors discarded, so required dropdowns could never be satisfied (Resolved 2026-07-25)
**Surfaced by bug #64's own fix.** Before #64, large forms died on a >30-minute timeout inside `SolveValidationErrors`, so nobody ever learned what happened *after* the fixes were applied. Once #64 cut the payload (confirmed live: **53,366 → 5,363 chars**, and elsewhere 43,033 → 1,200), those jobs began completing the LLM call in minutes — and immediately exposed the real blocker. It is now the run's dominant failure: **18 of 23 `FAILED_SUBMIT` outcomes** are "failed to submit application after 3 validation error attempts".

**The proof it was structural, not flaky:** the narrowing log reports payload sizes per attempt, and they were **byte-identical between attempt 2 and attempt 3** on multiple jobs (Ethos `43033 -> 1200` twice; Point Wild `3057` twice). Identical input means the same fields were still flagged invalid after the "fix" — nothing had changed, so the third attempt was arithmetically certain to fail like the second.

**Two compounding defects:**
1. **The outcome was thrown away.** The loop read `for selector, value := range fixesMap { safeFill(target, selector, value) }` — `safeFill` returns an `error` and the call site discarded it. Every failed fix was invisible: no log line, no error, no signal that the retry was unwinnable.
2. **`safeFill` is `Fill()`-only.** Playwright refuses `Fill()` on a `<select>` element. Greenhouse-style application forms routinely make dropdowns **required** — work authorization, visa sponsorship, EEO self-identification, "how did you hear about us". Such a field can therefore *never* be satisfied by this code path, no matter how correct the model's proposed answer is. The codebase already knew this: `resolveConsentGateIfPresent` uses `SelectOption` for bug #36's consent gate. The validation-fix path simply never adopted it.

**Fix:** new `applyValidationFix` dispatches on the control's real shape, resolved via a `tagName`/`type` probe:
- `<select>` → `SelectOption`, trying the **visible option label first** (the model answers as a human would — "Yes", "Decline to answer") and falling back to the underlying `value` attribute.
- `<input type=checkbox|radio>` → `Check`, with an explicit negative ("No"/"false"/"0") mapping to `Uncheck` so a decline is never silently converted into consent.
- everything else → `Fill`, as before.

Errors are now logged per selector, and if **no** proposed fix could be applied the attempt fails immediately with a clear message instead of burning the remaining retries on an unchanged form.

Tests: `TestApplyValidationFix_UsesSelectOptionForDropdowns` (also asserts `Fill()` is never called on a select), `TestApplyValidationFix_ChecksCheckboxes` (including that "No" unchecks rather than checks), `TestApplyValidationFix_ReportsUnmatchedSelector`, `TestApplyValidationFix_FillsPlainTextInputs`. `go build/vet/test ./...` all pass, 10 packages, 0 failures.


---

## 64. [SolveValidationErrors re-sends the entire form instead of just the fields that failed, timing out on large forms](#64-solvevalidationerrors-re-sends-the-entire-form-instead-of-just-the-fields-that-failed-timing-out-on-large-forms)

**Table rationale cell (original):** **Filed and fixed 2026-07-25.** The retry path sends the whole pruned form (~55k chars per bug #52's own measurement) when typically only a few fields failed validation. At the ~7 tok/s prompt processing measured live on the 30B, that is ~33 minutes against a 45-minute timeout — borderline by construction. Confirmed live: the same Reddit posting hit it **twice**, once timing out outright (`context deadline exceeded`) and once running 22+ minutes before the run was stopped. Now the pipeline's dominant bottleneck, since #23 removed tailoring and #24 can cut scoring

### 64. SolveValidationErrors re-sends the entire form instead of just the fields that failed, timing out on large forms (Resolved 2026-07-25)
**Found by asking why the 82-job run kept losing the same job.** The Reddit posting (`job-boards.greenhouse.io/reddit/jobs/8044767`) failed at the identical step twice: once with `failed to solve validation errors: ... context deadline exceeded` reaching Ollama, and once running **22+ minutes** inside a single `SolveValidationErrors` call before the run was stopped. Both times it had already filled the form successfully — this is a job that was *working* and lost at the last step.

**Root cause — a budget problem, not a logic problem.** The retry path sends `PruneDOMToForm(domHTML)`, i.e. **every field on the form**, even though a validation bounce typically involves a handful. Bug #52's own note measured a real 35-field ATS form at **52-55k chars even after both existing reduction passes**. Prompt processing on this machine's 30B was measured live at roughly **7 tokens/sec** (`ScoreJob`: 15,647 chars in 9m38s), so ~55k chars ≈ 13.7k tokens ≈ **over 30 minutes of inference against a 45-minute timeout**. Large forms were therefore failing on *time*, not on reasoning — and any concurrent load pushed them over.

**A second defect made the first one unfixable.** `StripPresentationalAttrs` listed **`aria-invalid`** among the attributes it strips. That is the WCAG-standard signal marking which control a form rejected — so by the time the payload reached the model, the information identifying the failing fields had already been deleted. Re-sending everything wasn't just wasteful, it was the only option left.

**Fix, in two parts:**
1. `aria-invalid` removed from `presentationalAttrs` — it is semantic, not presentational. (It is one short attribute per field; its retention costs essentially nothing against the 66% reduction that pass already achieves.)
2. New `parser.PruneDOMToInvalidFields` narrows the retry payload to only the controls marked invalid (`aria-invalid`, plus `data-invalid`/`data-has-error` for themes that roll their own), **plus any `<label for>` bound to them** — labels are collected separately because a label is usually a sibling or ancestor of its control, not a descendant, and the label text is exactly what tells the model what value the field wants.

**Deliberate fallback:** when no invalid control can be identified, the full form is sent exactly as before. An unreadable theme is a reason to send *more*, never less — narrowing to nothing would guarantee the retry fixes nothing. `narrowed` is returned explicitly so the caller cannot silently mistake "found none" for "narrowed to none", and the reduction is logged per attempt.

Tests: `TestPruneDOMToInvalidFields` (keeps both rejected fields and their label, drops the passing ones, asserts the payload actually shrinks), `TestPruneDOMToInvalidFields_NoMarkersFallsBackToFullForm`, `TestStripPresentationalAttrs_KeepsAriaInvalid`. One pre-existing test (`TestStripPresentationalAttrs_RemovesStylingAndStateAttrs`) asserted the *old* `aria-invalid` behavior and was updated with an inline note explaining the reversal rather than quietly edited. `go build/vet/test ./...` all pass, 10 packages, 0 failures.


---

## 63. [Every fit score was computed and thrown away — the only writer of fit_score had zero callers](#63-every-fit-score-was-computed-and-thrown-away--the-only-writer-of-fit_score-had-zero-callers)

**Table rationale cell (original):** Found while assessing whether improvements.md #14 (LoRA fine-tuning) was worth working: the DB has **zero** non-null `fit_score` values across ~3000 jobs. `UpdateFunnelStatusWithScore` is the only function that writes the column and had **no callers at all** — every scoring outcome went through `UpdateFunnelStatus`, which doesn't take a score. The pipeline's single most expensive step (~9m49s/job after #23) was read once for the `< 50` threshold check and discarded, so no scoring history could ever accumulate

### 63. Every fit score was computed and thrown away — the only writer of fit_score had zero callers (Resolved 2026-07-25)
**Found while assessing whether improvements.md #14 (local LoRA fine-tuning) was worth working.** Checking what training data actually existed returned a surprising result: `SELECT COUNT(*) FROM job_funnel WHERE fit_score IS NOT NULL` came back **0**, across a database with ~3000 discovered jobs and months of runs.

**Root cause:** `pkg/storage/manager.go` has exactly one function that writes the column — `UpdateFunnelStatusWithScore(url, status, fitScore)` — and a repo-wide grep found it had **zero call sites**. It was dead code. Every scoring outcome in `cmd/agent/main.go` instead called plain `UpdateFunnelStatus(url, status)`, which never touches `fit_score`. So `ScoreJob`'s result was used for the in-memory `score < 50` threshold check, written to the log line, and then discarded.

**Why this matters more than it looks:** `ScoreJob` is now the single most expensive operation in the entire pipeline — measured live at **9m49s of a ~10m job cycle** once improvements.md #23 removed the tailoring call. The project was spending essentially its whole per-job compute budget producing a number it immediately threw away. Knock-on effects: no scoring history for analytics, nothing for `cmd/dashboard` to chart, and — the reason this surfaced — **no labeled dataset could ever accumulate**, which is a hard structural blocker for improvements.md #14 independent of that item's other problems.

**Fix:** both post-scoring branches in `cmd/agent/main.go` now call `UpdateFunnelStatusWithScore` — the `SKIPPED` path (score under 50) and the proceed path (recording `PROCESSING` with the score) — so a score is persisted whichever way the decision goes. Verified that a subsequent plain `UpdateFunnelStatus` (e.g. the later `APPLIED`/`FAILED_SUBMIT` transition) preserves the stored score rather than nulling it, since those transitions are what every job hits afterward.

Test: `TestUpdateFunnelStatusWithScorePersistsScore` — writes a score, asserts it lands, then applies a later plain status change and asserts the score survives. `go build/vet/test ./...` all pass, 10 packages, 0 failures.

**Note for whoever reads this next:** scores only start accumulating from the next run onward. Every historical job remains `NULL`, and re-deriving those would mean re-scoring at ~10 minutes each — not worth it. Treat the dataset as starting 2026-07-25.


---

## 62. [The saved cover letter was deleted from the application folder, stripping the manual-apply queue](#62-the-saved-cover-letter-was-deleted-from-the-application-folder-stripping-the-manual-apply-queue)

**Table rationale cell (original):** `defer os.Remove(coverPath)` fired on every exit path including the `ErrAuthWall` early return, so `MANUAL_REQUIRED` jobs lost their letter before `MoveToManualApply` archived the folder — defeating that queue's entire "tailored docs ready" purpose. Live evidence: 5 of 6 sampled `needs_manual_apply/` folders held `resume.md` + `interview_prep.md` but no `coverletter.txt`. Compounded by a path bug (raw vs. sanitized company name) that made the delete land only on sanitize-stable names

### 62. The saved cover letter was deleted from the application folder, stripping the manual-apply queue (Resolved 2026-07-24)
**Found alongside #61**, tracing the same `coverPath` value.

**Root cause:** `pkg/submitter/browser.go` ran `defer os.Remove(coverPath)` right after `generateDocs()`, treating the file as a scratch temporary. It is not: `SaveApplication` writes it into `applications/<company>/coverletter.txt` deliberately as the saved record of what was sent, and `MoveToManualApply` archives that entire folder for jobs routed to `MANUAL_REQUIRED` — a queue whose whole value proposition is handing the user ready-made documents to apply with by hand. Because it was a `defer`, it fired on **every** exit path, including the `ErrAuthWall` early return that precedes the manual-apply routing.

**Live evidence:** `applications/needs_manual_apply/` held `Akuity`, `alteryxcareers`, `careers`, `ClickHouse`, and `DexCare` all with `resume.md` + `interview_prep.md` + `metadata.json` but **no** `coverletter.txt`. `Backend_Software_Engineer` was the lone folder that kept its letter, which confirmed the exact mechanism rather than leaving it a theory: `cmd/agent/main.go` built the path as `"applications/" + job.CompanyName + "/coverletter.txt"` from the **raw** company name, while `SaveApplication` writes under `safeCompanyDirName`'s sanitized name. The two agree only for names that are already sanitize-stable ("Reddit" → deleted), and silently diverge for anything with a space or punctuation ("Backend Software Engineer" → `os.Remove` targeted a nonexistent path, unchecked error, letter survived).

**Fix:** dropped the deletion entirely — nothing at that call site needs cleanup, since `resumePath` is the persistent `master_resume.pdf` and `coverPath` is either the persistent `master_cover_letter.txt` or an application-folder file meant to outlive the call. Separately fixed the path divergence with a new exported `storage.CoverLetterPath(companyName)`, which builds the path through the same `safeCompanyDirName` that `SaveApplication` writes with, so the two can no longer disagree. Note the same unsanitized-path class was already fixed once before for the manual-queue links (improvements.md #21's amendment) — this was a second, missed instance of it.

Tests: `TestCoverLetterPath` (including names with spaces and punctuation), `TestCoverLetterPathMatchesSaveApplication` (writes via `SaveApplication`, reads back via `CoverLetterPath` — fails if the two ever diverge again). `go build/vet/test ./...` all pass.


---

## 61. [The cover letter was never sent to any employer — no handler ever filled it](#61-the-cover-letter-was-never-sent-to-any-employer--no-handler-ever-filled-it)

**Table rationale cell (original):** Found while scoping the static master cover letter: `handleDynamic`/`handleGreenhouse`/`handleLever` fill name, email, phone, resume and custom questions, but there was zero `cover_letter` handling anywhere in `pkg/submitter/` — despite `ExtractFormMapping` being prompted to map a `cover_letter` selector and `FormMapping` carrying it. `AttemptSubmit` took `coverPath` and used it for exactly one thing: an `os.Remove` deferral. Every application in this project's history went out resume only, while still paying the full LLM cost to write a letter that was then discarded

### 61. The cover letter was never sent to any employer — no handler ever filled it (Resolved 2026-07-24)
**Found while scoping the user's request for a static master cover letter** (improvements.md #23), by tracing what `ProcessJobApplication`'s cover letter output actually does downstream.

**Root cause:** none of the three fill handlers had any cover letter step. `handleDynamic` fills `first_name`, `last_name`, `email`, `phone`, `resume`, then custom screening questions, then clicks submit. `handleGreenhouse` and `handleLever` fill their hardcoded equivalents plus the resume upload. A `grep -n "cover_letter\|coverLetter\|CoverLetter" pkg/submitter/*.go` returned **zero matches** in non-test code. Meanwhile `ExtractFormMapping`'s system directive explicitly instructs the LLM to map a `cover_letter` selector, and `FormMapping.Fields` carries it — the mapping was produced and then read by nobody. `AttemptSubmit` receives `coverPath` from `generateDocs()` and its only use was `defer os.Remove(coverPath)` (bug #62).

**Impact:** every application this project has ever auto-submitted went out **resume only**. The pipeline spent the single most expensive step in the run (`ProcessJobApplication`, measured live at 15-20+ minutes per job on this machine's CPU-only Ollama) generating a tailored cover letter that was written to disk, never uploaded, and then deleted.

**Fix:** new `fillCoverLetterIfPresent` in `pkg/submitter/browser.go`, wired into all three handlers (`handleDynamic` via the mapping's `cover_letter` selector/label, `handleGreenhouse` via `input[type='file'][name='cover_letter']`, `handleLever` via `textarea[name='comments']`). It tolerates both shapes real ATS platforms use: a file input (upload, detected via a `tagName`/`type` probe since Playwright rejects `Fill()` on a file input) or a textarea/text input (paste), and falls back through the existing label/placeholder/CSS chain. **Best effort by design** — a cover letter is optional on the large majority of real postings, so a failure to place one logs and continues rather than aborting an otherwise complete, submittable application, matching the contract already established for custom screening questions. `coverPath` is now threaded through `handleDynamic`/`handleGreenhouse`/`handleLever`/`AttemptVisionSubmit`.

Tests: `TestFillCoverLetter_PastesIntoTextarea`, `TestFillCoverLetter_UploadsToFileInput` (asserts the uploaded buffer is the real letter content), `TestFillCoverLetter_FailureDoesNotAbortSubmission`, `TestFillCoverLetter_MissingFileIsTolerated`. `go build/vet/test ./...` all pass.


---

## 60. [Ollama server pinned to an unnecessarily conservative 6,144-token context window — the dominant cause of MANUAL_REQUIRED outcomes](#60-ollama-server-pinned-to-an-unnecessarily-conservative-6144-token-context-window--the-dominant-cause-of-manual_required-outcomes)

**Table rationale cell (original):** Found live during the 82-job re-verification: every single `MANUAL_REQUIRED` outcome (13/13) was "form too large for the local model." The model itself supports up to 262,144 tokens; the systemd service was manually pinned to 6,144 on 2026-07-21. Raised to 32,768 with KV-cache quantization (`OLLAMA_KV_CACHE_TYPE=q8_0`) — confirmed live that available memory actually *improved* after the change

### 60. Ollama server pinned to an unnecessarily conservative 6,144-token context window — the dominant cause of MANUAL_REQUIRED outcomes (Resolved 2026-07-24)
**Found live while watching the 82-job re-verification**, after the user asked "any improvements or bugs we can make from this" given 0/82 had reached `APPLIED`: cross-referencing every `MANUAL_REQUIRED` outcome so far (13 total) against the live log found **all 13, no exceptions**, were "form too large for the local model" (bug #57's circuit breaker). 4 of the run's 6 `FAILED_SUBMIT` outcomes were confirmed genuinely dead/expired postings, 1 was bug #59. `MANUAL_REQUIRED` was by a wide margin the single largest blocker to a real `APPLIED` in this entire run.

**Root cause:** `ps aux` showed the live `llama-server` process launched with `-c 6144` — Ollama's actual runtime context window. Cross-referencing `ollama show`'s `model_info` for the running model (`qwen3:30b-instruct`, `qwen3moe` architecture) found `qwen3moe.context_length = 262144` — the model architecturally supports **262,144 tokens**, 42x the 6,144 it was actually being run with. Found the exact cause: `~/.config/systemd/user/ollama.service.d/override.conf` explicitly set `OLLAMA_CONTEXT_LENGTH=6144` (dated 2026-07-21, likely a conservative first value chosen before this model's actual memory profile was understood).

**Why this was assumed to be a hard constraint rather than just fixed outright:** the machine appeared genuinely memory-tight at the time (only ~496MB free, 4.8GB "available", 3.2GB already in swap, with the `llama-server` process alone at 19.3GB RSS / 63% of 29GB total RAM) — naively raising context risked an OOM on a live, valuable run. Investigated the actual KV-cache cost before assuming that risk was real: this model uses GQA with only 4 KV heads (of 32 attention heads, `qwen3moe.attention.head_count_kv = 4`), across 48 layers — working out to roughly **96KB of KV cache per token at f16**, meaning the observed 19.3GB was almost entirely the 30.5B model's fixed weight size (~17GB), not context-dependent at all. Raising context is comparatively cheap for this specific model.

**Fix:** raised `OLLAMA_CONTEXT_LENGTH` to `32768` (covers both real observed failures — Reddit needed 18,572 tokens, Akuity 16,604 — with ~2x margin) and added `OLLAMA_KV_CACHE_TYPE=q8_0` (quantizes the KV cache, roughly halving its already-small per-token cost, near-lossless in practice) to the same systemd override, then `systemctl --user daemon-reload && systemctl --user restart ollama.service`. Confirmed via the restarted `llama-server` process's own launch flags (`-c 32768 --cache-type-k q8_0 --cache-type-v q8_0`). **Confirmed live that system memory headroom actually improved after the restart** (15GB available vs. 4.8GB before) despite the ~5x larger context — the KV cache was never the dominant memory cost for this model. Correspondingly raised `pkg/submitter/browser.go`'s `maxPromptCharsForModelContext` from `14000` to `80000` (same `(context_tokens - 400) × 2.5 chars/token` conservative formula the original constant used) so the character-based circuit breaker stays in sync with the server's actual new capacity. Updated `TestLikelyExceedsModelContext` to reflect the new threshold, including a case confirming Reddit's real repro size (54,917 chars) now correctly fits under the raised window rather than being routed to manual. `go build/vet/test ./...` all pass.

**Operational cost of the fix itself:** restarting the Ollama service interrupted whatever was mid-generation in the live 82-job run at the time (one job, "Jumio," failed with a connection-reset error and needs a natural retry — not a new bug, direct fallout of the restart, and Ollama's per-request retry logic in `pkg/mcp` already handles transient connection failures on the next job).

**Not yet applied to the in-progress 82-job re-verification's own binary:** `maxPromptCharsForModelContext` is a compile-time constant — PID 3137654 (the isolated run) was built before this change and still enforces the old 14,000-char ceiling even though the Ollama server itself can now handle far more. Whether to rebuild and restart that specific run to benefit the ~55 still-`DISCOVERED` jobs, versus letting this fix apply from the next full-backlog batch onward, is a decision left to whoever is driving that run next (see the task journal) — restarting it also interrupts whatever's currently in-flight and needs the documented isolated-run restart procedure.


---

## 59. [Generic submit-button selector could click a hidden anti-spam-widget button instead of the real submit control](#59-generic-submit-button-selector-could-click-a-hidden-anti-spam-widget-button-instead-of-the-real-submit-control)

**Table rationale cell (original):** Found live during the 82-job re-verification: a real Lever posting ("Nova") failed with `playwright: timeout: Timeout 30000ms exceeded` clicking a locator that resolved to `<button type="submit" class="hidden" id="hcaptchaSubmitBtn">` — the `SolveValidationErrors` retry path's submit-button selector matched a hidden hCaptcha auxiliary button before the real, visible submit control

### 59. Generic submit-button selector could click a hidden anti-spam-widget button instead of the real submit control (Resolved 2026-07-24)
**Found live during the 82-job re-verification**, while monitoring for status changes: "Nova" (`jobs.lever.co/ioconnectservices.com/...`) reached `SolveValidationErrors`'s retry path and failed with:
```
Auto-Submit failed for Nova: playwright: timeout: Timeout 30000ms exceeded.
Call log:
  - waiting for locator('input[type=\'submit\'], button[type=\'submit\'], button:has-text(\'Submit\'), button:has-text(\'Apply\')').first()
    - locator resolved to <button type="submit" class="hidden" id="hcaptchaSubmitBtn"></button>
  - attempting click action
    - waiting for element to be visible, enabled and stable
    - element is not visible
    (56 retries, 30s total)
```

**Root cause:** `pkg/submitter/browser.go`'s retry-path submit click (line ~874, inside the `SolveValidationErrors` branch) used `submitLocator.First()` on `input[type='submit'], button[type='submit'], button:has-text('Submit'), button:has-text('Apply')`. That broad selector matched a hidden `<button type="submit" id="hcaptchaSubmitBtn">` — an internal control belonging to Lever's hCaptcha anti-spam embed (same widget class as bug #23's iframe, not the applicant-facing form) — before it ever reached the real, visible submit button later in the DOM. `.First()` doesn't consider visibility, so Playwright spent the full click timeout retrying a click on an element it will never consider clickable, surfacing as a generic timeout rather than a clear "wrong element" signal.

**Fix:** new `firstVisibleLocator(loc playwright.Locator, count int) playwright.Locator` in `pkg/submitter/browser.go` — iterates `loc.Nth(i)` for `i` in `[0, count)`, returns the first match where `IsVisible()` reports true, falling back to `loc.First()` if none report visible (better to attempt and get a clear timeout than silently give up when visibility detection itself might be unreliable). The retry-path submit click now calls this instead of `submitLocator.First()`. Only one call site in the codebase used this exact selector pattern, so the fix is self-contained.

**Tests:** `TestFirstVisibleLocator_SkipsHiddenCaptchaButton` (reproduces the exact live shape: index 0 hidden, index 1 visible, confirms index 1 is returned) and `TestFirstVisibleLocator_FallsBackToFirstWhenNoneVisible`. Extended `MockLocator` (`pkg/submitter/browser_test.go`) with `Nth`/`IsVisible` overrides (both default to matching prior behavior — `Nth` returns the receiver, `IsVisible` returns true — so no existing test's behavior changed). `go build/vet/test ./...` all pass.

**Not yet re-verified live against the specific "Nova" posting** — the fix is compiled but the isolated 82-job re-verification's already-running binary (PID 3137654) doesn't pick up code changes without a restart, and this bug was found mid-run rather than before it started. Requeuing "Nova" specifically (`cmd/requeue -pattern '%ioconnectservices%' -status FAILED_SUBMIT -confirm`) and restarting with a freshly-built binary would give a real live confirmation, but that also interrupts whatever the run is currently mid-flight on — left as a decision for whoever is driving that run next rather than done unilaterally here. See `documentation/task_journals/2026-07-25_monitor-live-run-and-fix-bugs.md` for the current consolidated run state.


---

## 58. [Stale career_chunks embedding dimension silently zeroed out all live RAG resume-context retrieval](#58-stale-career_chunks-embedding-dimension-silently-zeroed-out-all-live-rag-resume-context-retrieval)

**Table rationale cell (original):** Discovered while implementing improvements.md #22 (fit-similarity queue ranking): every stored `career_chunks` row is 3072-dimensional but the currently configured `nomic-embed-text` model produces 768-dim vectors — `CosineSimilarity`'s length-mismatch guard silently returns 0 for every comparison, so `RetrieveTopK` (used live in `cmd/agent`'s per-job resume/cover-letter tailoring) has been returning an arbitrary, non-semantic chunk order for every application this whole time, no error ever surfaced

### 58. Stale career_chunks embedding dimension silently zeroed out all live RAG resume-context retrieval (Resolved 2026-07-24)
**Found while implementing improvements.md #22** (rank the discovery queue by resume-fit similarity): a live 3-job test run of the new `cmd/rankjobs` backfill returned `fit_similarity = 0.0` for every job, which shouldn't happen across genuinely different job titles unless something structural was wrong, not just "genuinely low similarity."

**Root cause:** `pkg/parser/rag.go`'s `CosineSimilarity` has a length-mismatch guard (`if len(a) != len(b) { return 0 }`, added defensively, never meant to be load-bearing). Confirmed live: every one of the 8 rows in `career_chunks` stores a 3072-dimension embedding, but the currently configured `OLLAMA_EMBED_MODEL` (`nomic-embed-text`, confirmed via `/api/show`'s `embedding_length`) actually produces 768-dimension vectors. `cmd/agent`'s RAG ingestion only ever runs when `career_chunks` is empty (`len(existingChunks) == 0`) — once any chunks exist, from however long ago and under whatever model was configured at the time, they are never refreshed. The stored chunks predate the current embed-model configuration (exact prior model/provider not recoverable from `git log` — no code path in this repo has ever called anything but `nomic-embed-text` via the `ollama` provider's `Embed`, so the mismatch most likely predates a change to `.env`/`OLLAMA_EMBED_MODEL` outside version control, or a fresh model pull that changed what `nomic-embed-text` resolved to locally).

**Real-world impact:** `parser.RetrieveTopK`, called live in `cmd/agent`'s main worker loop (`cmd/agent/main.go`, RAG retrieval before every `ScoreJob`/tailoring call) scores every one of the 8 chunks 0 against any real job embedding, because every comparison hits the dimension mismatch. `sort.Slice`'s stable sort then returns the chunks in storage-insertion order regardless of actual relevance — so every tailored resume and cover letter generated by this pipeline, for its entire live history, has been built from an arbitrary fixed set of resume chunks rather than the ones actually most relevant to each job. No error, crash, or log line ever indicated this; `CosineSimilarity`'s guard exists purely to avoid a panic on mismatched slice lengths, not to signal an upstream configuration problem.

**Fix:** `parser.IngestResumeChunks(embed func(string) ([]float32, error), profilePath string) (int, error)` extracts the previously-inline ingestion logic (clear `career_chunks`, re-chunk `USER_PROFILE.md`, re-embed, re-save) into a reusable, independently-testable function. `parser.CareerChunksNeedReingest(existing []storage.CareerChunk, freshDim int) bool` detects a dimension mismatch. `cmd/agent`'s startup RAG-ingestion block now probes the configured embed model's actual current dimension with one cheap `GetEmbedding` call whenever chunks already exist, and re-ingests automatically if it no longer matches what's stored — so this class of drift self-heals on the next `cmd/agent` restart regardless of cause. New `cmd/reingest` CLI exposes the same ingestion for two cases the automatic startup check can't reach without a restart: fixing a live database a `cmd/agent` process is already using (a separate short-lived writer takes effect on that process's very next job — `career_chunks` is read fresh per job via `RetrieveTopK`, not cached — so this required no restart of PID 3137654, the live 82-job re-verification run in progress at the time this was found), and manually refreshing after editing `USER_PROFILE.md`. Tests: `TestCareerChunksNeedReingest`, `TestIngestResumeChunks` (confirms the pre-existing stale chunk gets cleared, not merged), `TestIngestResumeChunksSkipsFailedEmbeddings`. `go build/vet/test ./...` all pass.

**Live remediation:** ran the new `cmd/reingest` against the real `applications.db` while PID 3137654 (the 82-job re-verification) kept running unaffected — queued behind the live run's single-slot Ollama usage (~19 minutes total, several individual embed calls each themselves queued behind separate `ProcessJobApplication` calls), completed cleanly (`Re-ingested 9 career chunk(s)`). Confirmed all 9 `career_chunks` rows now store 768-dimension embeddings, matching `nomic-embed-text`. The 3 `job_funnel` rows the `cmd/rankjobs` test run had written `fit_similarity = 0.0` against the stale chunks were reset to `NULL` beforehand so they'd get correctly re-scored rather than silently kept as false data — re-ran `cmd/rankjobs -limit 3` against the same 3 real jobs and confirmed real, non-zero, non-uniform scores this time (`0.610`, `0.600`, `0.586`). Both this bug and improvements.md #22 (which surfaced it) are now verified end to end, not just unit-tested.


---

## 57. [Forms too large for Ollama's context window burned a full doc-gen cycle before failing with an ugly HTTP 400](#57-forms-too-large-for-ollamas-context-window-burned-a-full-doc-gen-cycle-before-failing-with-an-ugly-http-400)

**Table rationale cell (original):** Reddit and Akuity (both Greenhouse, both real, large screening forms) each hit `ollama returned HTTP 400: ... exceeds the available context size (6144 tokens)` on the exact same forms bug #52's payload-size fixes had already cut down — confirming a genuinely different, deeper constraint (the model's actual context window) that no amount of character-based trimming alone could fully solve

### 57. Forms too large for Ollama's context window burned a full doc-gen cycle before failing with an ugly HTTP 400 (Resolved 2026-07-24)
**Symptom:** live during the 82-job re-verification run, after bug #52's `StripPresentationalAttrs`/75k-limit fixes already shipped: Reddit failed again with `ollama returned HTTP 400: {"error":{"code":400,"message":"request (18572 tokens) exceeds the available context size (6144 tokens)..."}}`. Shortly after, Akuity — a different real Greenhouse posting — hit the identical error (16,604 tokens against the same 6,144 limit), confirming this wasn't a one-off.

**Root cause:** the character-based circuit breaker (`pkg/mcp/client.go`'s `payloadSafetyLimits`) and the local Ollama model's actual context window are two independent constraints. A payload can sit comfortably under the 75k-character safety limit and still overflow the model's real 6,144-token budget, since HTML content runs roughly 3 characters per token — well short of the 1:1 assumption a pure character limit implies. Both real failures happened at the validation-retry stage, meaning a full ~20-40 minute doc-generation cycle had already completed before the form was discovered to be unfillable at all.

**Fix applied 2026-07-24:** added `likelyExceedsModelContext` (`pkg/submitter/browser.go`) — a conservative 14,000-character budget (2.5 chars/token against the observed 6,144-token window, minus ~400 tokens reserved for the system prompt and EEO context) checked against the combined DOM-plus-profile-context length *before* calling either `ExtractFormMapping` or `SolveValidationErrors`, not just the retry path (the same 6,144-token ceiling applies to the very first mapping attempt too, which doesn't benefit from the retry path's extra DOM trimming and so is if anything more exposed). New `ErrFormTooLargeForModel` sentinel error, routed to `MANUAL_REQUIRED` in `cmd/agent/main.go` exactly like the existing `ErrAuthWall` path (bug #18) — the tailored documents are already generated and saved, so nothing is lost, just no longer force-fit into an automated submission the model structurally cannot process. 3 new tests, `go build/vet/test ./...` all pass.

**User's explicit choice (2026-07-24):** raising Ollama's `num_ctx` was considered and rejected for now — this machine has documented OOM history (bug #13) and increasing the context window directly increases per-request RAM usage (KV cache). Routing to manual submission avoids that risk entirely; revisit only if manual-queue volume from this specific reason becomes a real burden.

**Not yet verified live** — needs a fresh oversized form (Reddit or Akuity, once requeued) to confirm it now routes straight to `MANUAL_REQUIRED` instead of attempting a doomed LLM call.


---

## 56. [Dashboard has no tile for BLOCKED_CAPTCHA or INVALID_URL, silently omitting 9% of all job_funnel rows](#56-dashboard-has-no-tile-for-blocked_captcha-or-invalid_url-silently-omitting-9-of-all-job_funnel-rows)

**Table rationale cell (original):** User asked whether the dashboard's visible counts were accurate; each visible tile's own number checked out, but cross-referencing the full `job_funnel` status breakdown found 337 real rows (301 `INVALID_URL`, 36 `BLOCKED_CAPTCHA`) had no tile anywhere on the dashboard at all

### 56. Dashboard has no tile for BLOCKED_CAPTCHA or INVALID_URL, silently omitting 9% of all job_funnel rows (Resolved 2026-07-24)
**Symptom:** user reported the exact numbers shown on the dashboard UI (3140 in queue, 112 skipped, 282 failed, 12 manual queue) and asked me to verify accuracy.

**Investigation:** cross-checked each tile's exact query (`cmd/dashboard/main.go`'s `serveMetrics`) directly against the live DB — every number matched (the 282 vs 283 "Failed" discrepancy was pure live-data timing, the batch was actively running). But summing the six displayed tiles against `SELECT status, COUNT(*) FROM job_funnel GROUP BY status` left a gap: 337 rows (301 `INVALID_URL`, 36 `BLOCKED_CAPTCHA`) belonged to neither the shown tiles nor any hidden aggregate — they were simply never queried at all. Also found a smaller, related inconsistency while fixing this: the "last skipped" detail widget's own query already included `BLOCKED_CAPTCHA` (`WHERE status IN ('SKIPPED', 'BLOCKED_CAPTCHA')`), while the `Skipped` tile's count never did — the detail widget could show a CAPTCHA-blocked company while the tile total silently excluded it.

**Fix applied 2026-07-24:** added `BlockedCaptcha`/`InvalidURL` fields to `Metrics`, their own count queries, a `statusReason` case for `INVALID_URL`, and two new dashboard tiles (`cmd/dashboard/index.html`, two new neon colors — pink, teal — added to the existing palette, matching every other tile's CSS pattern exactly). Narrowed the "last skipped" query to just `SKIPPED` now that `BLOCKED_CAPTCHA` has its own dedicated tile, fixing the inconsistency found along the way. 3 new/updated tests (`go build/vet/test ./...` all pass), and visually verified via a Playwright screenshot of the real running dashboard against the live DB — both new tiles render correctly with accurate counts (36, 301).


---

## 55. [Jobs killed mid-flight get permanently stuck in PROCESSING, never retried, inflating the dashboard's live count](#55-jobs-killed-mid-flight-get-permanently-stuck-in-processing-never-retried-inflating-the-dashboards-live-count)

**Table rationale cell (original):** User asked why the dashboard UI showed 235 "processing" jobs — found every one was a permanently orphaned row from a run killed mid-job (`kill -9`, the only reliable method documented in this file's own Operational Trap notes), accumulated since 2026-07-21, none of them ever retriable since `GetDiscoveredJobs` only pulls `DISCOVERED`

### 55. Jobs killed mid-flight get permanently stuck in PROCESSING, never retried, inflating the dashboard's live count (Resolved 2026-07-24)
**Symptom:** user asked why the dashboard UI showed 235 jobs as "processing." A single-worker run can only ever have one job truly in flight at a time — 235 was never a live figure.

**Root cause:** every `kill -9` used to restart a run tonight (and every other night since 2026-07-21, per this file's own Operational Trap notes on why `kill -9` is necessary in the first place) kills whatever job the worker was mid-way through, and `AttemptSubmit`'s `PROCESSING` status write (`cmd/agent/main.go`, right before dispatch) never gets a chance to be reverted — no signal handler, no cleanup path, nothing. `GetDiscoveredJobs` only ever queries `status = 'DISCOVERED'` (`pkg/storage/manager.go`), so a row stuck at `PROCESSING` is invisible to every future run, forever, regardless of how many fixes ship afterward. Confirmed live: `MIN(last_updated)` among `PROCESSING` rows was 2026-07-21, `MAX` was the current moment — three full days of silent accumulation. Directly re-encountered twice tonight already (Enveritas, Akuity) while manually managing the 82-job re-verification run, each requiring a one-off manual `UPDATE` to recover.

**Fix applied 2026-07-24:** `storage.ReapStaleProcessingJobs()` — a single `UPDATE job_funnel SET status = 'DISCOVERED' WHERE status = 'PROCESSING'`, called once in `cmd/agent/main.go` right after `InitDB`, before any job can be marked `PROCESSING` by the run doing the reaping. Safe by construction: a freshly-started process cannot have produced any `PROCESSING` row itself yet, so anything already in that state at startup is unconditionally orphaned, regardless of worker count. 1 new test (`TestReapStaleProcessingJobs`), `go build/vet/test ./...` all pass. One-time cleanup: reset 234 of the 235 stale rows directly (excluded the one genuinely-active job at the time, confirmed via the running process's own log).

**Verified against real data:** `SELECT COUNT(*) FROM job_funnel WHERE status='PROCESSING'` dropped from 235 to 1 (the genuinely active job) immediately after the cleanup pass. The code fix itself will self-verify on the next restart of any kind — expect a `[Agent] Reset N stale PROCESSING row(s)...` log line whenever N > 0.


---

## 54. [Raw-HTML captcha pre-check misclassifies Ashby's client-rendered SPA shell as a block](#54-raw-html-captcha-pre-check-misclassifies-ashbys-client-rendered-spa-shell-as-a-block)

**Table rationale cell (original):** Investigating why 2 Ashby postings hit the raw-HTML captcha check before fit-scoring even ran (while trying to get a fresh confirmed success through the generic Learner Module path for bugs #8/#10/#14): confirmed both were real, currently-open, unblocked postings — the check's "little real text = block" corroborating signal is structurally meaningless for a client-rendered SPA fetched without executing JavaScript

### 54. Raw-HTML captcha pre-check misclassifies Ashby's client-rendered SPA shell as a block (Resolved 2026-07-24)
**Symptom:** while trying to get bugs #8/#10/#14 a fresh confirmed success through the generic Learner Module path (needed since neither has been exercised under the new post-#53 confirmation logic yet), two different, currently-`DISCOVERED` Ashby postings both hit `[Worker-%d] Security/Captcha block detected` — the raw-`net/http`-fetch pre-check in `cmd/agent/main.go` (bug #46's area) — before fit-scoring ever ran.

**Investigation:** wrote a standalone probe using the exact same plain `net/http` fetch (no browser, no JS execution) the real check uses, against one of the two flagged URLs. Found: raw HTML 41,996 bytes (a substantial, real response, not a small interstitial), but `parser.PruneDOMToText` extracted **0 characters** of visible text, and the raw HTML does contain a "recaptcha" substring. This is exactly bug #46's `widgetOnlyPhrasing && len(pruned) < 200` corroborating-signal condition — but the underlying assumption (a genuine interstitial replaces the real page, leaving little text behind) doesn't hold for Ashby: it renders all real content client-side via JavaScript, so a non-JS-executing fetch sees an empty shell on *every* posting, genuinely blocked or not. Checked `cmd/requeue -stats -source ashby`: only 9 total attempts (3 `BLOCKED_CAPTCHA`, 6 `FAILED_SUBMIT`, 0 `APPLIED`) — too small a sample to call this a 100%-deterministic block, but a real, reproducible false-positive mechanism regardless. Tried to find a genuine currently-blocked page from another platform to calibrate a general raw-HTML-size threshold instead of an Ashby-specific carve-out, but the other `BLOCKED_CAPTCHA` rows sampled were either stale (no longer reproducible) or a separate, distinct issue (`developers.smartrecruiters.com`/`developers.pinpointhq.com` API-docs pages being scraped as if they were postings).

**Addendum, same investigation:** the `developers.smartrecruiters.com`/`developers.pinpointhq.com` docs-page sighting turned out to be its own real, fixable bug, same class as the already-fixed board-index junk-URL bugs (#41/#42/#44) — confirmed live: `careers.smartrecruiters.com/<company>` tenant board-index pages are already correctly caught by the existing path-segment check, but SmartRecruiters' and Pinpoint's own API-documentation subdomain (`developers.`) was never a company tenant and had no filter at all. Fixed in `pkg/scraper/funnel.go`'s `IsKnownJunkJobURL`: exact-host-match exclusion for `developers.smartrecruiters.com` and `developers.pinpointhq.com`, alongside the existing corporate-subdomain exclusions for Workday/homerun.co/BambooHR. Confirmed a genuine Pinpoint company-tenant subdomain (`sunking.pinpointhq.com/postings/...`) still passes. 4 new test cases. One-time DB pass flipped the 8 existing `BLOCKED_CAPTCHA` rows for these two hosts to `INVALID_URL`.

**Fix applied 2026-07-24:** since this check cannot distinguish "SPA shell for a real posting" from "SPA shell leading to a block" without executing JavaScript, added `clientRenderedSPAHosts`/`isClientRenderedSPAHost` in `cmd/agent/main.go` (host-suffix matching, same convention as `authGatedATSHosts` in `pkg/submitter/browser.go`) — for these hosts, only the explicit block phrasing (`genuineBlockPhrasing`: Cloudflare + "verify you are human"/"attention required") is trusted; the widget-substring fallback is skipped entirely rather than trusting a text-length signal that's meaningless for this platform shape. Currently lists only `ashbyhq.com`. 5 new test cases, `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh Ashby posting to reach fit-scoring instead of being killed at this pre-check.


---

## 53. [isSubmissionConfirmed only ever ran for Lever/Greenhouse/LinkedIn — every other ATS platform's APPLIED had zero confirmation evidence](#53-issubmissionconfirmed-only-ever-ran-for-levergreenhouselinkedin--every-other-ats-platforms-applied-had-zero-confirmation-evidence)

**Table rationale cell (original):** User asked to verify APPLIED jobs generate email confirmations, worried about false positives. Live-probing the exact evidence-tier logic (added the same night for observability) found a live example landing on the weakest tier, which led to discovering the confirmation check was structurally unreachable for most ATS platforms — the majority of `job_funnel`'s entire APPLIED history for non-Lever/Greenhouse/LinkedIn platforms rested on a bare "handler didn't error" signal, the exact unverified-success pattern bug #51 was written to fix

### 53. isSubmissionConfirmed only ever ran for Lever/Greenhouse/LinkedIn — every other ATS platform's APPLIED had zero confirmation evidence (Resolved 2026-07-24)
**Symptom:** the user asked to verify that `APPLIED` jobs actually generate email confirmations, worried about false positives. While investigating, added logging to `isSubmissionConfirmed` (bug #51's addendum) to expose which of its three evidence tiers fired for each success. The very first live example after shipping that logging (Kobie Marketing, Lever) landed on the weakest tier — "URL changed, no confirmation or error wording found."

**Investigation:** wrote a read-only probe (no submission attempted) against a separate, untouched Lever posting to check whether that weak tier could plausibly be masking a real failure. Found 18 fields with native HTML5 `required` attributes and no `formnovalidate` override on the submit button — meaning a blank required field makes the *browser itself* block the submit client-side, with no navigation and no error text ever rendered into the page's HTML. Tracing why the "URL changed" fallback would fire true in that exact scenario found the real defect: `isSubmissionConfirmed`'s baseline for "did the URL change" was `applyURL`, the *original job-posting URL* captured once at the top of `AttemptSubmit` — but bug #47's click-to-reveal step navigates the page away from that URL (to a `.../apply` sub-page) *before any fill or submit ever happens*, for every Lever/Greenhouse job. So "the URL changed" was trivially true by the time confirmation ran, regardless of whether the submit click itself did anything at all.

**Second, larger gap found while mapping the fix:** tracing every code path that can lead to `AttemptSubmit` returning `nil` (success) found `isSubmissionConfirmed` is only ever reached by the Lever, Greenhouse, and LinkedIn dispatch branches, which are the only ones that fall through to the loop's shared bottom code. Every other path — the cached-mapping fast path (for any domain the Learner Module previously mapped), and both `AttemptVisionSubmit` call sites (used whenever the generic Learner Module handles an unrecognized ATS, i.e. SmartRecruiters, Ashby, Homerun, Pinpoint, Jobvite, BambooHR, applytojob.com, recruitee.com, and any platform without a dedicated handler) — returned success straight from `handleDynamic`'s bare error value, with no confirmation evidence of any kind. This is the exact unverified-success pattern bug #51 fixed; that fix was simply never extended past three of the many ATS paths.

**Fix applied 2026-07-24:** extracted `confirmOrError(page, companyName, urlBeforeClick, autoSubmitClick) error` as a shared helper (wraps the existing wait/URL/content/`isSubmissionConfirmed`/logging sequence). Threaded a new `urlBeforeSubmitClick` variable through every dispatch branch in `AttemptSubmit`'s loop (LinkedIn, Greenhouse, Lever, the generic Learner Module's primary `handleDynamic` call, and the validation-retry branch), captured immediately before each branch's own submit click rather than reusing the stale `applyURL` parameter. Wired `confirmOrError` into the cached-mapping fast path. Made `AttemptVisionSubmit` (`pkg/submitter/vision.go`) self-contained: it now captures its own pre-click URL and calls `confirmOrError` before returning success, so both loop-internal Vision-fallback call sites were changed from `execErr = AttemptVisionSubmit(...)` (which let it re-enter the loop's own confirmation check against a now-stale baseline) to a direct `return AttemptVisionSubmit(...)`, since its result is now already fully verified. 4 new tests (`TestConfirmOrError_*`), including `TestConfirmOrError_CatchesNativeValidationBlock` reproducing the exact live-repro shape. `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh submission on a non-Lever/Greenhouse/LinkedIn platform (e.g. Ashby or Homerun, both confirmed to have genuinely fillable forms per bug #50) to confirm the newly-wired confirmation check doesn't introduce false negatives on a real success.


---

## 52. [SolveValidationErrors sends the whole page's DOM, tripping the LLM-cost circuit breaker and losing otherwise-successful applications](#52-solvevalidationerrors-sends-the-whole-pages-dom-tripping-the-llm-cost-circuit-breaker-and-losing-otherwise-successful-applications)

**Table rationale cell (original):** Caught live resuming a watch session: a real Greenhouse posting (fit score 90) generated real tailored documents, filled every field, then was lost outright at the validation-retry step — `incrementAndLogAPICall`'s 50k-char safety limit aborted a 104,932-char payload that was the *entire* pruned page, not just the form

### 52. SolveValidationErrors sends the whole page's DOM, tripping the LLM-cost circuit breaker and losing otherwise-successful applications (Resolved 2026-07-23)
**Symptom:** caught while resuming a watch session on the live batch (PID 2542429, running bug #51's fix): `career_agent.log` showed a real Greenhouse posting ("Senior GTM Systems & Automation Engineer" at Cobalt.io, fit score 90) go all the way through real document generation and a full field fill, then get lost outright — `Submission failed validation. Retrying...` followed immediately by `CIRCUIT BREAKER TRIGGERED: Payload size 104932 exceeds safety limit (50k chars). Aborting to prevent runaway LLM costs.` `job_funnel.status` for this job was `FAILED_SUBMIT`.

**Root cause:** `pkg/mcp/client.go`'s `incrementAndLogAPICall` aborts any LLM call over 50,000 characters as a blanket safety net against runaway cost. `SolveValidationErrors`'s prompt is `Applicant Profile + Failed Form DOM`, where the DOM comes from `AttemptSubmit`'s validation-retry branch (`pkg/submitter/browser.go`): `target.HTML()` (the *entire* page or frame content, not scoped to the form) run through `parser.PruneDOM`, which only strips `<script>/<style>/<svg>/<path>/<iframe>/<noscript>/<meta>/<link>` — everything else (nav, footer, marketing copy, every class/data/aria attribute on every element) survives. On a modern React-rendered Greenhouse board page, that's well over 100k characters even after pruning; the circuit breaker isn't malfunctioning, it's correctly catching a payload that should never have been that large, since only the form's own fields matter for solving a validation error.

**Fix applied 2026-07-23:** added `parser.PruneDOMToForm` (`pkg/parser/dom.go`) — runs the existing `PruneDOM` pass, then narrows to the first `<form>` element found and renders only that subtree; falls back to the full pruned document when no `<form>` tag exists (covers ATS forms assembled without a real `<form>` element, so this can't regress a page that previously worked). Wired into the validation-retry call site only (`pkg/submitter/browser.go` line ~761); the initial `ExtractFormMapping` call site is untouched since it may need full-page context to find where the form even is. 2 new tests (`TestPruneDOMToForm_ScopesDownToFormWhenPresent`, `TestPruneDOMToForm_FallsBackToFullDocumentWhenNoFormTag`), both passing. `go build/vet/test ./...` all pass.

**Verified against real data:** confirmed via direct `applications.db` query that the diagnosed job (`job-boards.greenhouse.io/cobaltio/jobs/8603198002`) was genuinely `FAILED_SUBMIT` from this exact failure. Requeued it (`cmd/requeue -pattern '%cobaltio/jobs/8603198002%' -status FAILED_SUBMIT -confirm -clear-dedup`) back to `DISCOVERED`. Rebuilt and restarted the batch (PID 2542429 → 2579802) to pick up the fix; confirmed sole instance via `ps aux` and log growth. Live confirmation that this specific job now reaches a genuine `APPLIED` is still pending — the requeue happened after this restart's queue snapshot was already loaded, so it won't be retried until either a future restart or a `TARGET_JOB_URL` targeted run.

**Recurred 2026-07-24, root-caused and fixed for real this time:** the 82-job re-verification run's first outcome (Reddit, `job-boards.greenhouse.io`, fit 90) hit the exact same circuit breaker (102,963 chars) even with `PruneDOMToForm` live. A probe confirmed this posting genuinely has a `<form>` element and `PruneDOMToForm` correctly scoped to it — but that form element alone is 98,255 characters, because modern Greenhouse themes wrap every field in several layers of styling `<div>`s and accessibility attributes (`class`, `aria-describedby`, `aria-hidden`, `role`, `tabindex`, etc.). The fix wasn't wrong, the form is just genuinely that large. Added `parser.StripPresentationalAttrs` (`pkg/parser/dom.go`) — strips attributes that carry no selector-relevant information (styling/state/most `aria-*`), deliberately keeping `aria-label`/`aria-labelledby` since `ExtractFormMapping`/`SolveValidationErrors` both rely on them as a fallback label source. On the real Reddit form: 98,255 → 33,629 characters (66% reduction), comfortably under the 50k limit. Wired into the validation-retry call site alongside `PruneDOMToForm`. 3 new tests, `go build/vet/test ./...` all pass.

**Recurred a third time on the very same Reddit posting**, requeued into the re-verification run with both fixes live: this time 54,917 chars — the two-round trim clearly worked (down from 102,963), but still ~10% over 50k. Probed directly: this specific form genuinely has 35 real input/textarea fields and 24 labels (Reddit's actual screening questionnaire), no `<select>` dropdown bloat, no obviously-strippable fat left — the remaining size is proportional to real field count, not inefficiency. Raised the limit specifically for `SolveValidationErrors` to 75,000 chars (`payloadSafetyLimits` in `pkg/mcp/client.go`, `incrementAndLogAPICall` now looks up a per-call-type override instead of one hardcoded constant) — still far below the ~103-145k this call site saw before any of these three fixes existed, so a real regression back toward sending whole pages would still trip the breaker; every other call type keeps the original 50k. 3 new tests, `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh requeue of this exact Reddit posting into an active run to confirm the raised limit actually lets it through to a genuine `APPLIED`.


---

## 51. [Post-submit success check trusted any URL change, not proof of an actual successful submission](#51-post-submit-success-check-trusted-any-url-change-not-proof-of-an-actual-successful-submission)

**Table rationale cell (original):** User asked why no confirmation emails were arriving for today's real `APPLIED` jobs. Live Gmail search confirmed real ATS confirmation/rejection emails do exist for today's applications (a real Lever rejection, a real Workable confirmation+rejection) — auto-submit is genuinely reaching employers — but while investigating found `AttemptSubmit`'s only success signal was `currentURL != applyURL`, true for a validation-error redirect just as much as a real success

### 51. Post-submit success check trusted any URL change, not proof of an actual successful submission (Resolved 2026-07-23)
**Symptom:** the user asked why they weren't getting email confirmation receipts for today's real `APPLIED` jobs, the way they normally would applying manually.

**Investigation:** `cmd/tracker`'s classifier only recognizes REJECTED/INTERVIEW_REQUESTED-shaped emails — it has no "application received" category at all, so a quick tracker scan finding nothing proved nothing either way. Searched the connected Gmail account directly instead and found real, concrete evidence: a genuine Lever rejection email (`no-reply@hire.lever.co`, "Avive Solutions," dated today) and a genuine Workable confirmation-then-rejection pair (ZeroFox) — proof that `AttemptSubmit` is producing real submissions that reach real employer systems, not pure false positives. Nearly every one of these emails (Lever, Workable, and even unrelated LinkedIn Easy Apply confirmations) was sitting in the account's Trash rather than the inbox; `pkg/tracker`'s code has no delete/trash logic anywhere, so that routing is a Gmail-side filter/rule outside this codebase's control — flagged to the user to check their own Gmail filters, not something fixable in this repo. Not every ATS is configured to send an immediate "received" email either, so an absent receipt isn't always evidence of failure on its own.

**Real code issue found along the way:** while reading the post-submit verification logic (`pkg/submitter/browser.go`, the validation-error retry loop), the only success signal was `currentURL != applyURL || urlContains(thank/success/confirmation)`. A validation-error page reached via a redirect, or a bounce back to the company's careers listing, would satisfy `currentURL != applyURL` just as easily as a genuine success — the check never looked at page *content* to distinguish the two, and never handled AJAX-style ATS themes that show a success message without changing the URL at all (which would have been wrongly retried as a failure).

**Fix applied 2026-07-23:** added `isSubmissionConfirmed(applyURL, currentURL, pageContent)` in `pkg/submitter/browser.go`: prefers explicit confirmation wording anywhere on the page content (`submissionConfirmationPhrases` — works even when the URL never changed), falls back to the URL itself looking like a confirmation page, and only falls back further to "URL changed" when the resulting page doesn't show validation-error wording (`submissionErrorPhrases`). 5 new test cases in `TestIsSubmissionConfirmed`, all passing, including the exact false-positive shape this fix targets. `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh submission to confirm the tightened check doesn't introduce false negatives (rejecting a real success it used to accept).

**Addendum 2026-07-24:** user asked whether `APPLIED` jobs actually generate a confirmation email, worried about false positives — Gmail search for the last several real Lever `APPLIED` jobs tonight found zero traceable emails (user believes they may have deleted at least one relevant email, so inconclusive on its own). While investigating, found `isSubmissionConfirmed` had no way to tell, after the fact, which of its three evidence tiers (explicit confirmation phrase / confirmation-looking URL / weak "URL changed, no error text" last resort) actually fired for a given success — the function returned a bare bool. Added a `submissionConfirmationReason` return value and logged it at the call site (`[Auto-Submit] Submission confirmed for %s (%s)`), so future log reads (or a future automated cross-check) can distinguish strong evidence from the weak fallback without depending on email at all. No behavior change — same three tiers, same outcomes, just now observable. Tests updated to assert the reason per case. `go build/vet/test ./...` all pass.


---

## 50. [Workable requires account sign-in on every posting — same structural class as Workday](#50-workable-requires-account-sign-in-on-every-posting-same-structural-class-as-workday)

**Table rationale cell (original):** Investigating why Ashby/Workable/Homerun sat at 0 `APPLIED` despite #45/#46 clearing their CAPTCHA false positives: probed several live `jobs.workable.com` postings and found a "log in"/"sign in" gate before any form field on every one — 0 `APPLIED` across 12 real attempts this session, not a fill-selector problem. Ashby and Homerun, by contrast, turned out to have real fillable forms once probed properly (see #50's Details section)

### 50. Workable requires account sign-in on every posting — same structural class as Workday (Resolved 2026-07-23)
**Symptom:** the user asked why Ashby, Workable, and Homerun all sat at 0 `APPLIED` despite #45/#46 clearing the CAPTCHA false positives that were killing them. A per-source `job_funnel` breakdown (`cmd/requeue -stats`) confirmed `BLOCKED_CAPTCHA` had dropped to 0 for all three, but `FAILED_SUBMIT` remained high (12/12 Workable attempts, 6/7 Ashby, 6/6 Homerun) — so something downstream of the CAPTCHA fix was still killing every attempt.

**Investigation:** wrote a standalone probe and checked several real, currently-`FAILED_SUBMIT` postings from each source directly:
- **Workable** (`jobs.workable.com/view/.../at-telestream`, `.../at-callminer`): zero real form fields on load, no "Apply" button found even after a proper wait, and the page's own text contains "log in" — a genuine account-gate, same structural shape as Workday (bug #18). Confirmed real across 2 independent postings.
- **Ashby** (`jobs.ashbyhq.com/xbowcareers/...`): a first, too-fast probe (2s wait, `DOMContentLoaded`) falsely read this as broken (0 inputs, no button found). A proper probe (`NetworkIdle` wait + 3s, then click) found the real "Apply for this Job" button and a genuinely fillable 12-field form behind it. **Not a structural blocker** — Ashby's failures are happening in the Learner Module's mapping/fill quality, the same class as bugs #8/#10/#14 (still open, still gating the Usability Gate).
- **Homerun**: two `FAILED_SUBMIT` rows turned out to be `homerun.co`'s own marketing pages (`/hiring-kits/...`, `/job-description-templates/...`), not real postings — but `IsKnownJunkJobURL` in `pkg/scraper/funnel.go` already filters these; the rows in the DB are just stale, pre-fix data, not a live gap (no code change needed). A real posting (`root-sustainability.homerun.co/senior-software-engineer`) had a genuinely fillable 11-field form once probed properly, same as Ashby — its failures are also Learner Module quality, not structural.

**Fix applied 2026-07-23:** added `workable.com` to `authGatedATSHosts` in `pkg/submitter/browser.go` (the exact list Workday/bug #18 already uses) — `AttemptSubmit` now routes Workable jobs straight to manual submission with tailored documents already generated, instead of burning a full Learner Module + Vision cycle on a form that will never be reachable pre-auth. Also moved `workable.com` from priority tier 1 to tier 3 (alongside Workday) in `sourcePriorityCASE` (`pkg/storage/manager.go`) so it no longer competes for worker cycles ahead of platforms that can actually reach `APPLIED`. Added 4 new cases to `TestIsKnownAuthGatedHost` in `pkg/submitter/browser_test.go`, all passing. `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh Workable posting to confirm it now routes to `MANUAL_REQUIRED` immediately instead of a wasted fill cycle.


---

## 49. [handleGreenhouse's hardcoded submit selector doesn't exist on modern-board postings](#49-handlegreenhouses-hardcoded-submit-selector-doesnt-exist-on-modern-board-postings)

**Table rationale cell (original):** Live 2026-07-23, right after the priority-queue change put Greenhouse first: a real posting (`job-boards.greenhouse.io/alphasense`) filled every field successfully then failed with `failed to click submit: Timeout 30000ms exceeded` — a probe confirmed `input#submit_app` (the only selector `handleGreenhouse` ever tried) has zero matches on this posting's modern board template; the real control is an unidentified `<button type='submit'>Submit application</button>`

### 49. handleGreenhouse's hardcoded submit selector doesn't exist on modern-board postings (Resolved 2026-07-23)
**Symptom:** live 2026-07-23, right after the priority-queue change (this session) put Greenhouse jobs first in the queue and #45/#46/#47's fixes let one actually reach `handleGreenhouse`: `job-boards.greenhouse.io/alphasense/jobs/8420858002` (Staff Site Reliability Engineer) filled every field successfully — no errors on `first_name`/`last_name`/`email`/`phone` — then failed at the very last step: `failed to click submit: playwright: timeout: Timeout 30000ms exceeded`.

**Root cause:** `handleGreenhouse`'s submit step only ever tried one hardcoded selector, `input#submit_app`. A standalone probe against the live posting confirmed that selector has **zero matches** on this page — it's from Greenhouse's legacy embed theme, but this posting uses Greenhouse's modern board template, whose actual submit control is an unidentified `<button type='submit'>Submit application</button>` (also present on the page: an "Apply" button and a "Quick Apply with MyGreenhouse" button, both wrong targets — confirms a naive `:has-text('Submit')` search wasn't safe without narrowing by `type='submit'` too).

**Fix applied 2026-07-23:** in `pkg/submitter/browser.go`'s `handleGreenhouse`, check `input#submit_app`'s count first (preserves legacy-theme postings unchanged) and fall back to `button[type='submit']` only when the legacy selector has zero matches. Added `TestHandleGreenhouse_SubmitFallsBackWhenLegacySelectorMissing` and `TestHandleGreenhouse_SubmitUsesLegacySelectorWhenPresent` in `pkg/submitter/browser_test.go`, both passing. `go build/vet/test ./...` all pass.

**Verified live 2026-07-23 14:47:** requeued the exact same diagnosed job (`job-boards.greenhouse.io/alphasense`) after the fix shipped — it reached `handleGreenhouse` again, and `job_funnel.status` is now `APPLIED`. Third fresh `APPLIED` this session (after two via Lever, `smarsh` and `DexCare`), first via Greenhouse.


---

## 48. [Lever click-to-reveal (bug #47's fix) doesn't fire on a second real posting — possible page staleness after the long doc-gen wait](#48-lever-click-to-reveal-bug-47s-fix-doesnt-fire-on-a-second-real-posting-possible-page-staleness-after-the-long-doc-gen-wait)

**Table rationale cell (original):** Live 2026-07-23, right after #47 shipped: a second real Lever posting (`jobs.eu.lever.co/pnlfin/...`) failed the same way #47 was supposed to fix, but an isolated probe using the exact same selector against a *fresh* page load found and clicked it instantly with no error — pointing at page staleness during the ~14-16 min doc-gen wait between navigation and the click attempt, not a selector problem. Single occurrence so far, not yet confirmed

### 48. Lever click-to-reveal (bug #47's fix) doesn't fire on a second real posting — possible page staleness after the long doc-gen wait (Resolved 2026-07-24, not reproduced)
**Symptom:** live 2026-07-23, shortly after #47 shipped and was confirmed working on `smarsh`: a second real Lever posting (`jobs.eu.lever.co/pnlfin/024459c9-ba1b-4e36-b173-72b4f46a72d4`, Finom) went through the same code path and failed with the identical `form failed to render in time: playwright: timeout: Timeout 30000ms exceeded` #47 was supposed to have fixed — but with no `Clicked an Apply-labeled element` log line beforehand, meaning `clickApplyIfPresent`'s locator found zero matches this time (not a click failure, a *no-match* result).

**Investigation so far:** wrote a standalone probe against the same URL and confirmed the page genuinely has an "APPLY FOR THIS JOB" button (all-caps, still `:has-text('Apply')`-matchable — Playwright's `has-text` is case-insensitive). Re-ran `clickApplyIfPresent`'s exact selector against a *fresh* page load of the same URL: found 1 match, clicked with no error. So the selector itself is not the problem, and the button genuinely exists.

**Working theory, not yet confirmed:** the real difference between the probe and the live run is time — `AttemptSubmit` navigates to the page, then spends the full doc-generation window (~14-16 minutes on this machine's CPU-only Ollama) before ever attempting the click, while a fresh probe interacts within seconds of navigation. A page left open that long could plausibly go stale in a way a quick probe can't reproduce: a session/idle-timeout reload, lazy-unmounted content, or an anti-bot heuristic reacting to a suspiciously long dwell time with no interaction. Not yet confirmed — would require watching a live ~15 minute cycle end-to-end (page state at click time vs. at nav time) to prove, which wasn't done this session given the cost. Only one occurrence so far; needs a second live repro before treating the staleness theory as confirmed root cause.

**No fix attempted yet** — filed for a future session to either reproduce with direct evidence (e.g. a screenshot taken immediately before the click attempt in the real flow) or downgrade/close if it doesn't recur.

**Closed 2026-07-24, `/groom_backlogs` pass — not reproduced despite ample opportunity under the exact theorized precondition:** the 82-job re-verification run (now consolidated in `documentation/task_journals/2026-07-25_monitor-live-run-and-fix-bugs.md`) processed at least 11 further Lever postings on 2026-07-24 alone (Gateway Automation Engineer, AHEAD, Agentic Engineer, Dijital Team, Celara, Senior Infrastructure Software Engineer, Eneba, Grant Street Group, Instrumentl, Aircall, Kobie Marketing), each one going through the identical ~10-20 minute `generateDocsFunc` doc-gen wait between navigation and the Apply click that this bug's staleness theory blamed. `career_agent.log` shows a clean `Clicked an Apply-labeled element to reveal the application form` immediately followed by `Detected Lever ATS` in every one of them — zero occurrences of `form failed to render in time` for Lever anywhere in the log since the original 2026-07-23 13:17:38 sighting (confirmed via `grep -n 'form failed to render in time' career_agent.log`, last Lever-relevant hit predates this bug's own filing). This is a direct test of the staleness theory under matching conditions, not just absence of a fresh repro attempt — treating the original sighting as a one-off (an unlucky timing/network blip) rather than a systemic defect. Reopen if a genuine second occurrence is ever observed with a fresh timestamp.


---

## 47. [Dedicated Greenhouse/Lever handlers never click "Apply" to reveal the form, only the generic Learner Module path does](#47-dedicated-greenhouselever-handlers-never-click-apply-to-reveal-the-form-only-the-generic-learner-module-path-does)

**Table rationale cell (original):** Discovered live 2026-07-23 while re-verifying #45: a real Lever posting reached `handleLever` for the first time (previously always killed earlier by #45/#46) and failed with `form failed to render in time` — `handleLever`/`handleGreenhouse` were never wired to bug #8's click-to-reveal step, unlike the Learner Module path, because they "weren't implicated" when #8 was fixed. Invisible until #45/#46 stopped killing these jobs before they ever reached this code

### 47. Dedicated Greenhouse/Lever handlers never click "Apply" to reveal the form, only the generic Learner Module path does (Resolved 2026-07-23)
**Symptom:** live 2026-07-23, right after fixing #45/#46, a real Lever posting (`jobs.lever.co/smarsh/...`, one of the exact postings used to diagnose #45) reached `handleLever` for the first time — the earlier CAPTCHA false-positives had been killing every Lever/Greenhouse job before they ever got this far, so this code path had effectively never run against real traffic. It failed immediately: `Auto-Submit failed for smarsh: form failed to render in time: playwright: timeout: Timeout 30000ms exceeded` — `handleLever` waits on `input[name='name']` right away with no earlier step.

**Root cause:** confirmed via the same live-probe technique used for #45 — this exact Lever posting has **zero** real form fields on page load; the fields only appear after clicking "Apply for this job." Bug #8 already solved this exact click-to-reveal problem, but its fix (`clickApplyIfPresent`) was deliberately scoped only to the generic Learner Module branch of `AttemptSubmit` — the comment on that fix explicitly says the dedicated Greenhouse/Lever/LinkedIn handlers "weren't implicated" at the time, because no live case had ever reached them with this failure shape. #45/#46's CAPTCHA-detection fix is what finally let real traffic reach `handleLever`/`handleGreenhouse` and exposed the gap.

**Fix applied 2026-07-23:** added `clickApplyIfPresent(page)` plus the same post-click `isCaptchaBlocked` re-check bug #35 added to the Learner Module branch, to both the Greenhouse and Lever dispatch branches in `AttemptSubmit` (`pkg/submitter/browser.go`). `clickApplyIfPresent` no-ops when no "Apply"-labeled element exists, so postings whose form is already present on load are unaffected. `go build/vet/test ./...` all pass.

**Verified live 2026-07-23 12:00:** re-ran the same Lever posting after the fix — log showed the click firing (`Clicked an Apply-labeled element to reveal the application form`), then `Detected Lever ATS. Filling out fields...` with no `form failed to render in time`, and the job reached a genuine `APPLIED` in `job_funnel` — the first fresh `APPLIED` produced this entire verification effort (since 2026-07-21). See #45's Details section for the full fix chain.


---

## 46. [Raw-HTML job-description fetch also misdetects reCAPTCHA/Turnstile widgets as a block, before fit-scoring even runs](#46-raw-html-job-description-fetch-also-misdetects-recaptchaturnstile-widgets-as-a-block-before-fit-scoring-even-runs)

**Table rationale cell (original):** Found immediately after fixing #45: a second, earlier, independent CAPTCHA check in `cmd/agent/main.go` (on the raw HTML from a plain `net/http` fetch, before fit-scoring) had the exact same bare-substring false-positive bug — confirmed live, it killed a real Lever posting before #45's fix could even matter

### 46. Raw-HTML job-description fetch also misdetects reCAPTCHA/Turnstile widgets as a block, before fit-scoring even runs (Resolved 2026-07-23)
**Symptom:** discovered immediately after fixing #45. Re-ran the exact same Lever `smarsh` posting standalone (`TARGET_JOB_URL` single-job test harness, temporarily added to `cmd/agent/main.go`) and it was still killed instantly: `[Worker-1] Security/Captcha block detected for smarsh. Skipping job to save API tokens.` — before fit-scoring, before #45's fix could even be reached.

**Root cause:** a second, entirely separate CAPTCHA check lives in `cmd/agent/main.go`, run on the raw HTML from a plain `net/http` fetch of the job-description page (no browser, no rendered DOM, no frames) — completely independent of `isCaptchaBlocked` in `pkg/submitter/browser.go`. Its condition included two bare, uncorroborated substring checks: `strings.Contains(lowerHTML, "recaptcha") || strings.Contains(lowerHTML, "cf-turnstile")`, with no check for whether real page content was also present. Since virtually every Greenhouse/Lever/Ashby/Workable job page's raw HTML references a `recaptcha`-hosted script tag as a standard anti-spam measure, this check alone likely accounts for most of #45's DB-observed false-positive rate — it runs *before* #45's check ever gets a chance to matter, on every single job, not just ones that reach the apply stage.

**Fix applied 2026-07-23:** same principle as #45, adapted for raw HTML (no DOM/frame API available here): compute the pruned plain-text content first, then only treat the bare `recaptcha`/`cf-turnstile` substring match as a block if that pruned text is also unusually short (<200 chars) — a genuine interstitial replaces the real page content and prunes down to almost nothing; a real job posting with a standard widget script tag prunes down to hundreds/thousands of characters of real description text regardless. The explicit two-phrase Cloudflare check (`"cloudflare" && ("verify you are human" || "attention required")`) is untouched and still applies unconditionally. `go build/vet/test ./...` all pass.

**Verified live 2026-07-23:** re-ran the same Lever posting after this fix — it passed the description-fetch stage cleanly for the first time (no more instant `Security/Captcha block detected`), reached fit-scoring, then #45's fix, then #47's fix, then a genuine `APPLIED`.


---

## 45. [isCaptchaBlocked misdetects standard reCAPTCHA/hCaptcha anti-spam widgets on real forms as a full block](#45-iscaptchablocked-misdetects-standard-recaptchahcaptcha-anti-spam-widgets-on-real-forms-as-a-full-block)

**Table rationale cell (original):** DB analysis 2026-07-23: `BLOCKED_CAPTCHA` accounted for 89% of Greenhouse outcomes, 91% of Lever, 96% of Ashby, 82% of Workable — platforms with dedicated handlers, previously assumed reliable. Live probes of several of these postings found large, genuinely fillable forms (21-40 real fields) being killed purely because a standard invisible reCAPTCHA/hCaptcha anti-spam widget iframe was present. Almost certainly the single largest suppressor of `APPLIED` outcomes in the project's history, dwarfing #4/#8/#10/#14 combined

### 45. isCaptchaBlocked misdetects standard reCAPTCHA/hCaptcha anti-spam widgets on real forms as a full block (Resolved 2026-07-23)
**Discovered while investigating the user's question "is there anything else we can do to increase chances of APPLIED working?"** Ran a per-source breakdown of `job_funnel` outcomes for the first time (this had never been done before — prior source exclusions like breezy.hr and Workday, #38, came from diagnosing individual live failures, not a full statistical pass):

| source | total | applied | captcha | failed | manual |
| --- | --- | --- | --- | --- | --- |
| greenhouse | 392 | 29 | 351 (89%) | 12 | 0 |
| lever | 291 | 6 | 264 (91%) | 21 | 0 |
| ashby | 168 | 0 | 162 (96%) | 6 | 0 |
| workable | 66 | 0 | 54 (82%) | 12 | 0 |
| smartrecruiters | 31 | 2 | 11 (35%) | 18 | 0 |

Greenhouse and Lever have dedicated handlers (`handleGreenhouse`, `handleLever`) and were assumed to be among the most reliable platforms — instead they were being killed by `BLOCKED_CAPTCHA` at an 89-91% rate, similar to or worse than SmartRecruiters (the platform CAPTCHA detection, bug #23, was originally built for).

**Root cause:** wrote a standalone Playwright probe and loaded several live, currently-`DISCOVERED` postings directly. `jobs.lever.co/smarsh/...` and `jobs.lever.co/teramind/...`: 35 and 40 real, fillable `<input>/<textarea>/<select>` fields on the main page — genuinely fillable forms — that also embed a standard `hcaptcha.com` invisible anti-spam widget iframe (2 elements). `job-boards.greenhouse.io/rushdownstudios/...`: 21 real fields plus a `recaptcha.net` invisible enterprise-reCAPTCHA anchor iframe. `isCaptchaBlocked` (`pkg/submitter/browser.go`, bug #23) treated the mere presence of any frame whose URL contains `hcaptcha.com`/`recaptcha`/etc. as proof of a full block, with no regard for whether a real form also exists — so every one of these forms was killed by the very first `isCaptchaBlocked` check (`AttemptSubmit`, before doc generation, before any ATS-specific dispatch), before `handleGreenhouse`/`handleLever` ever ran. Genuine DataDome-style interstitials (the original #23 repro, and the still-current SmartRecruiters case) instead replace the real page content, leaving essentially zero real form fields — confirmed live 2026-07-23: SmartRecruiters postings, post-click, show 0 main-page fields and only the captcha frame's own internal fields.

**Fix applied 2026-07-23:** added a `captchaWidgetFieldThreshold` (5) check to `isCaptchaBlocked` — if the main page already has more than 5 real `input`/`textarea`/`select` fields, the frame-host heuristic is not trusted (only the explicit block-wording text check, `isCaptchaContent`, still applies). Genuine interstitials still have ~0 real fields and are unaffected; forms with a benign anti-spam widget no longer get killed. Added `TestIsCaptchaBlocked_GenuineInterstitialWithFewMainFields`, `TestIsCaptchaBlocked_RealFormWithBenignCaptchaWidget`, and `TestIsCaptchaBlocked_ExplicitBlockWordingStillWins` in `pkg/submitter/browser_test.go`, all passing. `go build/vet/test ./...` all pass.

**Verified live 2026-07-23 12:00:** re-ran the exact Lever posting (`jobs.lever.co/smarsh/...`) used to diagnose this bug, after also fixing #46 (a second, independent CAPTCHA false-positive, found immediately while re-verifying this one) and #47 (the dedicated Lever handler's own missing click-to-reveal step, exposed only once #45/#46 stopped killing the job earlier). Result: `job_funnel.status` went from `BLOCKED_CAPTCHA` to a genuine `APPLIED` — the first fresh, real `APPLIED` produced since this whole verification effort began (2026-07-21). This is also the first live confirmation of the Usability Gate's "one full batch run reaches `APPLIED` end to end" checkbox.


---

## 44. [BambooHR corporate subdomains kept slipping past a growing denylist](#44-bamboohr-corporate-subdomains-kept-slipping-past-a-growing-denylist-resolved-2026-07-23)

**Table rationale cell (original):** Confirmed live overnight 2026-07-22→23: after #42 excluded `www.`/`app.bamboohr.com` specifically, two more corporate subdomains (`learn.`, `trust.bamboohr.com`) were discovered and processed as postings during unattended running, and a log grep found two more still unexcluded (`developers.`, `documentation.bamboohr.com`). Replaced the denylist with a positive check: every real posting seen across dozens of tenants uses `/jobs/questions...` or `/careers/<id>`; anything else on any `*.bamboohr.com` subdomain is now junk, catching all current and future corporate subdomains at once instead of one at a time

### 44. BambooHR corporate subdomains kept slipping past a growing denylist (Resolved 2026-07-23)
**Symptom:** confirmed live over 2026-07-22 into 2026-07-23: after #42 excluded `www.bamboohr.com` and `app.bamboohr.com` specifically, two *more* BambooHR corporate subdomains were discovered and processed as postings a few hours later during unattended running — `learn.bamboohr.com/introduction-to-bamboohrs-open-api` and `trust.bamboohr.com/controls`, each burning a full doc-gen cycle. A `find`-style grep of the log for every unique `*.bamboohr.com` URL seen turned up two more still unexcluded (`developers.bamboohr.com`, `documentation.bamboohr.com`).

**Root cause:** #42's fix was a denylist of specific known-bad subdomains, which only ever catches subdomains already observed failing — BambooHR evidently has an open-ended family of shared corporate subdomains (login, product docs, compliance, API docs, ...) that a one-at-a-time denylist can never keep ahead of.

**Fix:** replaced the denylist with a positive check instead. A grep across every real BambooHR posting URL seen this session (dozens, across many different tenant subdomains) showed they *all* use exactly one of two path shapes: `/jobs/questions...` or `/careers/<id>`. `IsKnownJunkJobURL` now treats any `*.bamboohr.com` URL whose path doesn't match one of those two shapes as junk, regardless of which subdomain it's on — this catches every subdomain above plus any future one without needing to be told about it individually. `go build/vet/test ./...` all pass.

---

## 43. [getByLabel/getByPlaceholder threw a Playwright strict-mode violation when a label matched more than one element](#43-getbylabelgetbyplaceholder-threw-a-playwright-strict-mode-violation-when-a-label-matched-more-than-one-element)

**Table rationale cell (original):** Confirmed live 2026-07-22 (Workable/Dispel posting): after every other fix landed, a real fill attempt got past "First Name" cleanly for the first time all session, then failed on "Phone" with `strict mode violation: getByLabel('Phone') resolved to 2 elements` — a second element (likely a hidden duplicate or a country-code sub-field) shared the same accessible label. `GetByLabelLoc`/`GetByPlaceholderLoc` in `pkg/submitter/browser.go` now call `.First()` — filling isn't order-sensitive, so narrowing to one match beats failing outright

### 43. getByLabel/getByPlaceholder threw a Playwright strict-mode violation when a label matched more than one element
**Symptom:** confirmed live 2026-07-22 on a Workable/Dispel posting — after every other fix this session had landed (cookie banner, Apply-button matching, consent gates, junk-page filters), a real fill attempt finally got past "First Name" cleanly, the first time all session a fill reached a second field at all. It then failed immediately on "Phone": `playwright: Error: strict mode violation: getByLabel('Phone') resolved to 2 elements`.

**Root cause:** Playwright's `getByLabel`/`getByPlaceholder` throw rather than silently pick one when more than one element on the page shares the same accessible label or placeholder text (e.g. a visible phone field plus a hidden duplicate, or a country-code sub-control also labeled "Phone"). `GetByLabelLoc`/`GetByPlaceholderLoc` in `pkg/submitter/browser.go` called these directly with no disambiguation.

**Fix:** both methods, on both the `pageTarget` and `frameTarget` implementations, now chain `.First()`. Filling a field isn't order-sensitive — any element genuinely carrying that label is an acceptable fill target, so narrowing to the first match is strictly better than failing the whole field (and cascading to a Vision fallback) over an ambiguity that doesn't actually matter for correctness. `go build/vet/test ./...` all pass. **Not yet verified end-to-end** — confirmed the error class and the fix's mechanism, but hasn't yet been observed live producing a full successful submission past this point.


---

## 42. [www.bamboohr.com and app.bamboohr.com pages (marketing site, shared login portal) scored as postings](#42-wwwbamboohrcom-and-appbamboohrcom-pages-marketing-site-shared-login-portal-scored-as-postings)

**Table rationale cell (original):** Same bare-domain pattern as the homerun.co fix (#29-class): `www.bamboohr.com/integrations/listings/remote` (BambooHR's own product marketing page) burned a 16-minute doc-gen cycle, and `app.bamboohr.com/login/` (the shared employee login portal every tenant uses) scored 80 and reached `AttemptSubmit`. Real postings are always on a company subdomain, e.g. `cxm.bamboohr.com/jobs/questions?id=169`. Both bare hosts now filtered in `IsKnownJunkJobURL`

### 42. www.bamboohr.com and app.bamboohr.com pages (marketing site, shared login portal) scored as postings
**Symptom:** confirmed live 2026-07-22: `www.bamboohr.com/integrations/listings/remote` (BambooHR's own product-integrations marketing page) scored 80 and burned a 16-minute tailoring cycle before the Learner Module found nothing fillable. Minutes later, `app.bamboohr.com/login/` — BambooHR's shared employee login portal, used by every tenant, not a job posting at all — also scored 80 and reached `AttemptSubmit`.

**Root cause:** identical shape to the already-fixed homerun.co bug: BambooHR is a subdomain-tenant platform where real postings always live on a company subdomain (`cxm.bamboohr.com/jobs/questions?id=169`), but the bare `www.bamboohr.com` (marketing site) and `app.bamboohr.com` (shared app/login shell) hosts were never excluded, so anything discovered on them got treated like a tenant posting.

**Fix:** two more bare-host checks added to `IsKnownJunkJobURL` alongside the homerun.co one: `host == "bamboohr.com" || host == "www.bamboohr.com"` and `host == "app.bamboohr.com"`. Verified live: the very next `www.bamboohr.com/careers/` URL pulled from the backlog was correctly caught at worker intake (`Skipping known-junk URL (never a posting)`), rather than reaching doc generation. `go build/vet/test ./...` all pass.


---

## 41. [applytojob.com and recruitee.com board-index/landing pages scored and processed as real postings](#41-applytojobcom-and-recruiteecom-board-indexlanding-pages-scored-and-processed-as-real-postings)

**Table rationale cell (original):** Confirmed live 2026-07-22: `holafly.applytojob.com/apply` (bare path, no job ID) is the company's full 20-role "Current Openings" list, not a posting — a standalone script showed zero form fields and a screenshot showed a generic job board. `greatminds.recruitee.com/homepage` is the same shape, the tenant's landing page. Same root cause and fix pattern as the already-fixed board-index bugs for smartrecruiters/lever/greenhouse/ashbyhq/workable/jobvite, just never extended to these two subdomain-tenant platforms. `IsKnownJunkJobURL` now treats any `*.applytojob.com` or `*.recruitee.com` URL with ≤1 path segment as junk (real postings need `/apply/<id>/<slug>` or `/o/<slug>`). applytojob.com had 0 `APPLIED` across 176 historical attempts — this was very likely the dominant reason

### 41. applytojob.com and recruitee.com board-index/landing pages scored and processed as real postings
**Symptom:** confirmed live 2026-07-22 on two different platforms in the same session: `holafly.applytojob.com/apply` scored 80, generated a full tailored application, and then failed the fill stage — a standalone script inspecting the page found zero form fields and a screenshot showed a plain list of 20 unrelated open roles ("Current Openings"), not the specific job that had been scored. Separately, `greatminds.recruitee.com/homepage` showed the identical shape: scored, tailored, then no form to fill.

**Root cause:** both platforms are subdomain-tenant ATS hosts (`company.applytojob.com`, `company.recruitee.com`) whose bare board-index/landing path is indistinguishable from a real posting by URL alone unless the path depth is checked — exactly the same class of bug already fixed months earlier for the six *path*-tenant platforms (`pathTenantATS` in `IsKnownJunkJobURL`), just never extended to these two subdomain-tenant ones. Confirmed the real-posting shape by contrast: `brightvisiontechnologies.applytojob.com/apply/z4xS0fd5C5/Senior-Backend-Engineer` (3 path segments, a real job that reached full doc generation earlier the same night) versus `holafly.applytojob.com/apply` (1 segment, junk). Recruitee's convention is `/o/<slug>` for real postings; even a bare `/o` (seen several times in earlier logs this session, e.g. `sensysgatsogroup.recruitee.com/o`) is suspected to be that tenant's job-board index under the `/o` prefix, not a posting.

**Fix:** added two new blocks to `IsKnownJunkJobURL` in `pkg/scraper/funnel.go`, one per platform, each counting path segments and rejecting ≤1 (mirroring the existing `pathTenantATS` loop's logic but applied per-host since these are subdomain- not path-tenant). Caught live immediately after deploying: the very next `bamboohr.com` board-index URL discovered was correctly skipped at worker intake (see #42). `go build/vet/test ./...` all pass. **Why this one matters most:** `applytojob.com` had 0 `APPLIED` across 176 historical attempts in `applications.db` — the worst attempted-vs-success ratio of any platform still actively targeted after #38 excluded `breezy.hr`. This bug, not a fill-strategy problem, is the most likely explanation for that entire 0% record.


---

## 40. [~200+ files/dirs under applications/ are still owned by a stale UID from an earlier containerized run](#40-200-filesdirs-under-applications-are-still-owned-by-a-stale-uid-from-an-earlier-containerized-run)

**Table rationale cell (original):** Confirmed live 2026-07-22: `manual_queue.md`, `manual_submissions.md`, and two per-job directories (`applications/en/`, then `applications/jobs/`) were owned by UID `524288` and silently failing every write with `permission denied` — the second collision (`applications/jobs/`) cost two fully-generated, otherwise-successful applications in one 20-minute window, which is what justified the full fix instead of continuing to patch one collision at a time. User ran `sudo chown -R $(whoami):$(whoami) applications/`; `find applications -not -user howlcipher` now returns zero paths

### 40. ~200+ files/dirs under applications/ are still owned by a stale UID from an earlier containerized run
**Symptom:** confirmed live 2026-07-22: `applications/needs_manual_apply/manual_queue.md`, `applications/manual_submissions.md`, and `applications/en/` (a per-job output directory) were all owned by UID `524288` — not `howlcipher` — and every write to them failed with `permission denied`, silently. For `manual_queue.md`/`manual_submissions.md` this meant an unknown number of jobs that should have been recorded as `MANUAL_REQUIRED` were dropped with zero record anywhere. For `applications/en/`, it outright killed an otherwise-successful job (`failed to write resume: ... permission denied`) because the company slug happened to collide with a pre-existing stale directory of the same name.

**Root cause:** UID `524288` is a classic rootless-podman/distrobox subuid mapping — these paths were written by an earlier session running inside (or with root inside) a container, and are simply inaccessible to the `howlcipher` (UID 1000) user these paths are normally accessed as, from either the host or the `career-agent` container (see the regressed Bazzite entry above — same container, unrelated cause).

**Fix, partial:** `manual_queue.md` and `manual_submissions.md` were fixable without root — their *parent* directory was owned correctly, so a delete-and-recreate (preserving content via a copy first) worked fine, since Unix permission checks for unlinking a file depend on the containing directory's write bit, not the file's own ownership. `applications/en/` needed the user to run `sudo rm -rf applications/en` directly, since that directory's *own* mode (755, no group/other write) blocked emptying it even though its parent was owned correctly.

**Still open:** `find applications -not -user howlcipher` shows roughly 200+ other paths in the same state. None of them currently block anything (they're historical per-job output directories the app doesn't write back into), but any future job whose company slug collides with one of these names will silently fail exactly like `applications/en/` did. Needs either a one-time `sudo chown -R $(whoami):$(whoami) applications/` sweep, or a code-level fix: the manual-queue path already has collision-avoidance (suffixes like `en_US-5`, `en_US-6` are visible in `manual_queue.md`), but the main `applications/<company>/` doc-writing path used by `AttemptSubmit` does not.

**Resolved 2026-07-22 (later same session):** the predicted collision happened for real, twice, within 20 minutes — two different jobs (both with the generic company slug "jobs", from `opn.bamboohr.com` and `it8.bamboohr.com`) each completed a full 6-17 minute tailoring cycle and then failed at the very last step, `applications/jobs/resume.md: permission denied`, because `applications/jobs/` was one of the stale-owned directories. That real, repeated cost was enough to justify the full sweep instead of continuing to patch collisions one at a time: user ran `sudo chown -R $(whoami):$(whoami) applications/`. Verified: `find applications -not -user howlcipher` now returns nothing. The code-level collision-avoidance gap (the main doc-writing path still has none, unlike the manual-queue path) remains but is now low-priority since the underlying ownership problem is gone.


---

## 39. [Vision-fallback fill fails with "empty selector provided for form filling"](#39-vision-fallback-fill-fails-with-empty-selector-provided-for-form-filling)

**Table rationale cell (original):** Observed live 2026-07-22 (`brightvisiontechnologies.applytojob.com`): a stale pre-fix cached mapping timed out, correctly invalidated itself, and fell back to `AttemptVisionSubmit`, which then failed with `ErrEmptySelector` rather than a genuine fill attempt — the Vision LLM's response apparently didn't parse into a usable selector for at least one field. Not yet root-caused; needs the same standalone-script approach used for #34-#37, reproduced against a *fresh* (non-cached) Learner Module attempt so cache staleness doesn't confound the diagnosis

### 39. Vision-fallback fill fails with "empty selector provided for form filling" (Resolved 2026-07-23)
**Symptom:** observed live 2026-07-22 on `brightvisiontechnologies.applytojob.com`: a cached form-mapping (learned before today's #34-#37 fixes, so potentially stale) timed out on the primary fill attempt, correctly invalidated itself, and fell back to `AttemptVisionSubmit` — which then failed immediately with `ErrEmptySelector` ("empty selector provided for form filling") rather than attempting a real fill.

**Root cause, confirmed via a standalone script (same methodology as #34-#37):** wrote a small program using the app's own `mcp.Client`/Playwright launch config to navigate a *fresh* (non-cached, non-stale) `brightvisiontechnologies.applytojob.com` posting discovered live that same morning, screenshot it, and call `ExtractFormMappingVision` directly. The page had **zero inputs, zero forms, zero iframes, and zero "Apply"-labeled elements** — a screenshot confirmed why: the posting had expired, rendering JazzHR/ApplyToJob's own banner, **"This position is no longer available. Click here to view more opportunities..."**. This exact wording doesn't match any existing entry in `deadJobPhrases` (`"job is no longer available"`, `"no longer exists"`, etc. — all close but not this one, which says "position" instead of "job"), so the dead-job guard at the top of `AttemptSubmit` (`isDeadJobPage`, checked before doc generation) let it straight through. By the time `AttemptVisionSubmit` ran against a screenshot with no real form in it at all, the vision model (`qwen2.5vl:7b`) had nothing grounded to map — in this reproduction it confidently **hallucinated** a fully plausible-but-fake selector set (`#first-name-input`, `#last-name-input`, etc., none of which exist on the page); the original 2026-07-22 report saw the same underlying "no real form" condition instead produce empty fields/labels for at least one field. Both are the same root cause — a dead page reaching Vision with nothing to see — manifesting as two different failure shapes depending on the model's response that run. This is the same bug class as #9/#15 (a dead-job phrasing variant the guard didn't know about yet), not a Vision-module defect at all.

**Fix:** added `"position is no longer available"` to `deadJobPhrases` in `pkg/submitter/browser.go`. A dead posting with this wording now bails in seconds at the pre-existing `isDeadJobPage` check, before document generation, the Learner Module, or Vision ever run. Added a regression case to `TestIsDeadJobPage`. `go build/vet/test ./...` all pass. Diagnostic script was written to a temporary, untracked directory and deleted before commit, per this project's established practice for these live-repro scripts.


---

## 38. [FunnelEngine kept sending Learner+doc-gen cycles at a 0%-success source and let Workday monopolize the worker queue](#38-funnelengine-kept-sending-learnerdoc-gen-cycles-at-a-0-success-source-and-let-workday-monopolize-the-worker-queue)

**Table rationale cell (original):** DB analysis 2026-07-22: `breezy.hr` had 0 `APPLIED` across 212 discovered jobs (48 `FAILED_SUBMIT`, the worst ratio of any actively-attempted platform) — excluded from `TargetATS` and `isValidATSUrl` entirely. Separately, `GetDiscoveredJobs` had no `ORDER BY`, so 228 already-queued Workday rows (account-gated, can only ever reach `MANUAL_REQUIRED` per #18) came back clustered together and monopolized every worker cycle — confirmed live as 6 Workday jobs in a row post-cleanup. Query now excludes `breezy.hr` and sorts Workday rows last

### 38. FunnelEngine kept sending Learner+doc-gen cycles at a 0%-success source and let Workday monopolize the worker queue
**Symptom:** two related findings from a DB analysis of `applications.db` on 2026-07-22 (cross-referencing `job_funnel.url` against `job_funnel.status` by ATS platform): `breezy.hr` had **0 `APPLIED` across 212 discovered jobs**, with 48 `FAILED_SUBMIT` — the worst attempted-vs-success ratio of any platform with meaningful volume. Separately, live observation showed **6 Workday jobs discovered and processed in a row** immediately after a clean single-instance restart, crowding out every other platform.

**Root cause, breezy.hr:** no single root cause found (unlike #34-#36) — just a sustained, evidence-backed pattern of failure worth cutting rather than continuing to chase platform-by-platform.

**Root cause, Workday:** `storage.GetDiscoveredJobs()` (`pkg/storage/manager.go`) had no `ORDER BY`, so its `SELECT ... WHERE status = 'DISCOVERED'` came back in raw SQLite rowid order. 228 Workday rows already sitting in the backlog from an earlier discovery run were inserted consecutively, so they came back clustered together — and since Workday postings are account-gated (bug #18, already fixed), every one of those 6 could only ever reach `MANUAL_REQUIRED`, never `APPLIED`, while doing so ahead of platforms that actually can.

**Fix:** removed `"breezy.hr"` from `TargetATS` (`pkg/scraper/funnel.go`, stops new discovery) and from `isValidATSUrl`'s `atsDomains` allowlist (stops any stray breezy.hr URL from validating even via other discovery paths). Changed `GetDiscoveredJobs`'s query to `WHERE status = 'DISCOVERED' AND url NOT LIKE '%breezy.hr%' ORDER BY CASE WHEN url LIKE '%myworkdayjobs.com%' THEN 1 ELSE 0 END, id` — excludes breezy.hr entirely (no future value expected) and deprioritizes (not excludes) Workday, since it still produces useful pre-tailored manual-apply documents. Verified live: backlog dropped from 2021→1917 on restart (matching the ~104 excluded breezy.hr rows almost exactly), and the very first job pulled after restart was a non-Workday platform for the first time all session. `go build/vet/test ./...` all pass.


---

## 37. [fillActionTimeoutMs (15000ms) too tight for genuine CPU contention from the co-located Ollama model](#37-fillactiontimeoutms-15000ms-too-tight-for-genuine-cpu-contention-from-the-co-located-ollama-model)

**Table rationale cell (original):** Confirmed live 2026-07-22: even after cleaning up duplicate processes down to one clean instance, two different real (non-junk) jobs still hit the fill timeout on all three tiers (label/placeholder/CSS) in the same ~45s window immediately after Ollama finished a heavy generation burst (200%+ CPU). Same failure shape as #6's already-fixed Ollama client timeout — a fixed value too short for genuinely slow-but-honest contention, not a selector bug. Doubled to 30000ms across all 6 call sites via one named constant

### 37. fillActionTimeoutMs (15000ms) too tight for genuine CPU contention from the co-located Ollama model
**Symptom:** confirmed live 2026-07-22, *after* cleaning up the duplicate-process contention (bugs.md's Operational Trap) down to a single clean instance: two different real, non-junk jobs on two different platforms (`utilus.homerun.co`, `jobs.workable.com/view`) still hit `Timeout 15000ms exceeded` on all three fill tiers (label, placeholder, CSS selector) in the same ~45-second window, each time immediately after the local Ollama model finished a heavy generation burst (200%+ CPU on an 8-core/29GB machine, per `ps`/`uptime` at the time).

**Root cause:** the exact same failure shape as bug #6's already-fixed Ollama client timeout — a hardcoded value too short for genuinely slow-but-honest work under real, bursty local-LLM contention, not a selector-strategy bug (#4/#14's territory, already fixed). The three consecutive fill-tier failures in the same short window, immediately following an LLM burst, is the signature of CPU starvation, not a missing element.

**Fix:** introduced a named `fillActionTimeoutMs = 30000` constant in `pkg/submitter/browser.go` and replaced all 6 raw `playwright.Float(15000)` call sites (fill, label-fill, placeholder-fill, click, upload, submit-click) with it. Doubling gave real headroom against the observed contention window without being reckless. **Caveat:** confirmed *not* sufficient on its own — the next job tested after this fix still hit `Timeout 30000ms exceeded` on Jobvite, but that specific case was root-caused separately as #36 (no form existed yet, not a timing issue at all), so this fix's actual hit rate against pure CPU-contention timeouts specifically is still unconfirmed by a clean counterfactual. `go build/vet/test ./...` all pass.


---

## 36. [Jobvite's "Data Consent" step means the application form doesn't exist until a location/language <select> is chosen](#36-jobvites-data-consent-step-means-the-application-form-doesnt-exist-until-a-locationlanguage-select-is-chosen)

**Table rationale cell (original):** Confirmed live 2026-07-22 (CMG Financial, Jobvite): after the Apply click, the page has zero `<input>` elements anywhere (main page or any frame) — only a single `<select>` labeled "Location of Residence and Language". Confirmed via standalone script that choosing an option alone (no extra click needed) instantly reveals the real form (24 fields). `resolveConsentGateIfPresent` now detects a zero-input page with a `<select>` present and chooses an option matching the candidate's actual state from `pii.yaml` (so the CA-privacy-specific disclosure some tenants show stays honest), falling back to the first non-placeholder option

### 36. Jobvite's "Data Consent" step means the application form doesn't exist until a location/language <select> is chosen
**Symptom:** confirmed live 2026-07-22 (CMG Financial, `jobs.jobvite.com`): Apply click succeeded, Learner Module mapping succeeded, and the fill still failed on "First Name" at the full 30000ms timeout. A standalone script inspecting the post-click page found **zero `<input>` elements anywhere** — main page, all frames, nothing — only a single `<select>` labeled "Location of Residence and Language" with options "Non-California" and "California" (a CA-privacy-disclosure gate, not an actual country picker despite the label wording).

**Root cause:** the real application form (confirmed 24 real fields once revealed) simply does not exist in the DOM until the `<select>` has a value chosen — not a click-to-reveal pattern like #8, a genuinely separate "form doesn't render until this prerequisite step" pattern. Confirmed via the standalone script that selecting an option alone (no extra button click needed) is sufficient to reveal it immediately.

**Fix:** added `resolveConsentGateIfPresent(page, pii)` in `pkg/submitter/browser.go`, called right before `resolveFillTarget` in the "Unknown ATS" branch. Only activates when the page has zero `<input>` elements *and* a `<select>` is present, so it can't false-positive on normal single-page forms (the vast majority of postings). Prefers an option whose text matches the candidate's actual state — "Non-California" here, correctly, since `pii.yaml`'s address is in Michigan — so the CA-specific disclosure some tenants show stays honest, falling back to the first non-placeholder option (skipping the "Select..." placeholder at index 0) when no match is found. `go build/vet/test ./...` all pass.


---

## 35. [SmartRecruiters' "I'm interested" button and post-click CAPTCHA reveals both went undetected](#35-smartrecruiters-im-interested-button-and-post-click-captcha-reveals-both-went-undetected)

**Table rationale cell (original):** Confirmed live 2026-07-22 (Oteemo, SmartRecruiters): `clickApplyIfPresent`'s selector only matched "Apply" text, so SmartRecruiters' actual button ("I'm interested") was never clicked and the fill logic always targeted the public job-description page, which has no form. Separately, clicking it navigates to a *new* `oneclick-ui` URL that can be gated by a fresh DataDome challenge (`geo.captcha-delivery.com`) the earlier #23 captcha check never saw, since that check only ran once, before this click. Both fixed together: broadened the click-target selector, and re-run `isCaptchaBlocked` immediately after the reveal click

### 35. SmartRecruiters' "I'm interested" button and post-click CAPTCHA reveals both went undetected
**Symptom:** confirmed live 2026-07-22 (Oteemo, `jobs.smartrecruiters.com`): a real, non-expired job reached the Learner Module, mapped, and still failed to fill "First Name" — a standalone script showed 0 real inputs anywhere on the page, only one unlabeled input and an empty `about:blank` iframe. A screenshot showed the page was a normal job-description page with a button reading **"I'm interested"**, not "Apply".

**Root cause, part one:** `clickApplyIfPresent`'s locator (`button:has-text('Apply'), a:has-text('Apply')`) never matches SmartRecruiters' own wording, so the click-to-reveal step silently no-ops and every downstream fill attempt targets the public job-description page, which has no form on it at all.

**Root cause, part two:** confirmed via the same standalone script that clicking "I'm interested" navigates to a *new* URL (`jobs.smartrecruiters.com/oneclick-ui/company/.../publication/...`) that can itself be gated by a fresh DataDome challenge (`geo.captcha-delivery.com`) — this exact case reproduced live. Bug #23's captcha check runs once, right after the initial navigation, before this reveal click ever happens, so it never sees a challenge that only appears after the click.

**Fix:** broadened `clickApplyIfPresent`'s selector to also match `"I'm interested"` text. Added a second `isCaptchaBlocked` check immediately after the reveal click in `AttemptSubmit`'s "Unknown ATS" branch, before the Learner Module runs — bails with `BLOCKED_CAPTCHA` instead of burning a full mapping + fill + Vision cycle on an unfillable challenge page. Verified live same session: the very next SmartRecruiters job (SynergyMachines) logged `Clicked an Apply-labeled element to reveal the application form` immediately followed by the new check catching a DataDome challenge and marking `BLOCKED_CAPTCHA` in under 2 seconds. `go build/vet/test ./...` all pass.


---

## 34. [A cookie-consent banner's backdrop silently intercepts every click, defeating clickApplyIfPresent](#34-a-cookie-consent-banners-backdrop-silently-intercepts-every-click-defeating-clickapplyifpresent)

**Table rationale cell (original):** Confirmed live 2026-07-22 (Workable/European Dynamics): the real "Apply now" button reported visible/enabled/stable yet every click retried and timed out — Playwright's own error log showed a `data-ui="backdrop"` div intercepting pointer events across the click target. No amount of increasing `fillActionTimeoutMs` could have fixed this; it's a genuine interaction blocker. `dismissCookieBanner` now runs right after page load, before any interaction

### 34. A cookie-consent banner's backdrop silently intercepts every click, defeating clickApplyIfPresent
**Symptom:** confirmed live 2026-07-22 on a Workable posting (European Dynamics, "Junior DevSecOps Engineer"): `clickApplyIfPresent` found the real "Apply now" button, and Playwright's own diagnostic log showed it as visible, enabled, and stable — yet every click attempt retried and eventually timed out at both 5000ms (the production code) and 10000ms (a standalone diagnostic run). No amount of increasing the timeout could have helped, since the actionability check itself never passed.

**Root cause:** a screenshot of the page showed a cookie-consent banner ("Workable uses cookies... Accept all / Decline all") sitting at the bottom of the viewport. Playwright's click-retry log named the actual blocker: a `<div data-ui="backdrop" class="styles__backdrop--1TOnJ">` intercepting pointer events across the click target area. The existing consent-detection logic (used elsewhere for a different check) only matched literal `cookie`/`consent` substrings in element `id`/`class`, but this banner's markup uses obfuscated CSS-module class names, so nothing before this ever saw it as a consent banner at all.

**Fix:** added `dismissCookieBanner(page)` in `pkg/submitter/browser.go`, called immediately after the initial page-load wait and before any dead-job/captcha/interaction checks. Prefers a Decline/Reject button when offered, falling back to Accept — the choice only matters for unblocking the click (this is a one-shot headless session with no persistent identity to protect), and Decline is preferred only because it isn't always offered and the project is otherwise privacy-conscious (`pii.yaml`, `scripts/sanitize_jobs.go`). Verified with a standalone script: the same "Apply now" click that timed out for 10+ seconds before the fix succeeded instantly (`err=<nil>`) after adding the decline-click step. `go build/vet/test ./...` all pass.


---

## 25. [Fit scoring ignores geographic eligibility restrictions](#25-fit-scoring-ignores-geographic-eligibility-restrictions)

**Table rationale cell (original):** Observed live 2026-07-22 02:05: "Site Reliability Engineer — Remote from Romania or Hungary" scored 80 for a US-based candidate. `remote_only` is enforced but *where the remote worker must live* is not — cycles get burned (and applications potentially sent) for roles the candidate is ineligible for. Likely fix is a ScoreJob prompt instruction to hard-fail location-restricted roles outside the candidate's country; needs scoring-quality verification before shipping, not a blind prompt edit

### 25. Fit scoring ignores geographic eligibility restrictions
**Symptom:** live 2026-07-22 02:05, `jobs.smartrecruiters.com/AristaNetworks/744000001578165-site-reliability-engineer-remote-from-romania-or-hungary` scored **80** and proceeded to a full application attempt. The role is explicitly restricted to workers located in Romania or Hungary; the candidate is US-based (Michigan). `profile.yaml`'s `remote_only` gate passed (it *is* remote) — nothing checks whether the candidate is in the required location. Every such job burns a full cycle, and a successful auto-submit would send an application the employer must discard.

**Fix applied 2026-07-22 (shipped as part of the #34-#38 batch, commit `b1709fd`):** added a 7th rule to the `ScoreJob` prompt in `pkg/mcp/client.go`: "If the job explicitly restricts remote candidates to a specific country or region, and my location does not match, deduct 80 points" — same-magnitude penalty as the existing `remote_only` mismatch rule, hard-failing the fit score for a role like the Arista Networks one above. No dedicated test exists (consistent with the rest of `ScoreJob`, which has no test coverage anywhere — it's a prompt-template function with no deterministic output to assert against); "verified locally" means the prompt text was read back and confirmed present, not a live re-score of the original flagged posting. **Groom-pass note (2026-07-23):** this Details section had drifted from the table's Resolved status — confirmed via `grep` that rule 7 is live in the current prompt; updating this section to match.


---

## 24. [Prompt-injection quarantine may false-positive on ordinary job-page copy ("you are a...")](#24-prompt-injection-quarantine-may-false-positive-on-ordinary-job-page-copy-you-are-a)

**Table rationale cell (original):** Observed live 2026-07-22 02:03 (Versant3, SmartRecruiters): the quarantine layer blocked the application on `role_manipulation 0.4` via a "you are a" heuristic plus a 0.65 fuzzy keyword match — phrasing like "you are a passionate engineer" is standard job-ad copy, so this heuristic class may silently block legitimate applications. Needs a false-positive-rate check against BLOCKED/quarantine logs before any tuning: loosening an injection defense requires evidence, not one anecdote

### 24. Prompt-injection quarantine may false-positive on ordinary job-page copy ("you are a...")
**Symptom:** live 2026-07-22 02:03:59, the Versant3 SmartRecruiters application was hard-blocked: `malicious prompt injection detected on career page: [{role_manipulation 0.4 potential role assignment via 'you are a' heuristic ...} {instruction_override 0.65 fuzzy match detected multiple injection-related keywords (possible typo evasion) ...}]`. "You are a ..." is near-universal job-ad copy ("you are a self-starter who..."), so the role-manipulation heuristic plausibly fires on a large fraction of legitimate postings, and combined-score blocking turns that into silently lost applications (status FAILED_SUBMIT, indistinguishable from real failures in the funnel).

**Fix applied 2026-07-22:** Implemented a secondary LLM check. If the quarantine layer flags a payload, we perform a lightweight inference call to verify if the context is legitimately describing a role (e.g. Prompt Engineer, AI Engineer) or if it is an actual malicious attack trying to hijack the agent. If the LLM classifies it as SAFE, we proceed instead of blocking the job application.


---

## 23. [Bot-protection interstitials (DataDome) aren't detected, burning full cycles and feeding the Learner captcha pages](#23-bot-protection-interstitials-datadome-arent-detected-burning-full-cycles-and-feeding-the-learner-captcha-pages)

**Table rationale cell (original):** Confirmed live 2026-07-22 01:43 (AbbVie on jobs.smartrecruiters.com): DataDome served "Access is temporarily restricted" (12-element page, challenge iframe from geo.captcha-delivery.com), yet the pipeline generated docs, "mapped" the captcha page, failed all fill tiers, and went to Vision. The only captcha detection was Cloudflare/reCAPTCHA phrases in the scraper path. `AttemptSubmit` now checks content phrases + challenge-iframe hosts right after navigation and returns `ErrCaptchaBlocked` → `BLOCKED_CAPTCHA`, before any doc generation

### 23. Bot-protection interstitials (DataDome) aren't detected, burning full cycles and feeding the Learner captcha pages (Resolved 2026-07-22)
**Symptom:** live 2026-07-22 01:37-01:43, a genuine AbbVie posting (`jobs.smartrecruiters.com/oneclick-ui/company/AbbVie/publication/...`) went through the full pipeline — docs generated, Learner Module "successfully mapped" the page, all three fill tiers timed out on `first_name`, Vision fallback launched — exactly the shape of the fill bugs (#8/#14/#16), which is what made it valuable to diagnose properly instead of assuming.

**Root cause, confirmed by standalone diagnostic + screenshot:** the page served to the agent is a DataDome bot-protection interstitial — "Access is temporarily restricted ... Automated (bot) activity on your network" — a 12-element DOM with zero forms and one challenge iframe from `geo.captcha-delivery.com`. The Learner mapped a captcha page; no fill strategy could ever work. The only captcha detection in the codebase was Cloudflare/reCAPTCHA phrase matching in the scraper's description-fetch path (`cmd/agent/main.go`) — nothing guarded `AttemptSubmit`, and DataDome wasn't recognized anywhere.

**Fix:** `AttemptSubmit` now runs `isCaptchaBlocked` right after navigation, before document generation: content phrases ("access is temporarily restricted", "verify you are human", ...) plus a frame scan for challenge hosts (`captcha-delivery.com`, `hcaptcha.com`, `recaptcha`, `challenges.cloudflare.com`, `arkoselabs.com`). Detection returns `ErrCaptchaBlocked`; the worker maps it to the pre-existing `BLOCKED_CAPTCHA` status (already rendered by the dashboard's `statusReason`). Cost per bot-walled job drops from 10-40 minutes to seconds. `TestIsCaptchaContent` covers the live DataDome copy plus Cloudflare/human-check variants.

**Operational note, not fixable in code:** the block message cites the machine's network IP — SmartRecruiters has rate-flagged this network for automated activity, so SmartRecruiters jobs will keep hitting `BLOCKED_CAPTCHA` until the flag ages out (typically hours to days) regardless of this fix. Actually *solving* challenges is `improvements_paywall.md` #17 (needs a paid 2captcha/capsolver key, explicitly user-gated).


---

## 22. [Stale pre-filter backlog rows and error-redirect URLs bypass every discovery filter](#22-stale-pre-filter-backlog-rows-and-error-redirect-urls-bypass-every-discovery-filter)

**Table rationale cell (original):** Confirmed live 2026-07-22: `www.workday.com/.../hiring-programs.html` (a marketing page queued before #5's filters existed) burned doc-gen + a 34-minute Vision call, and the funnel "discovered" `remotecom?error=true` as a live job. `isValidATSUrl` now rejects any URL with an `error` query param; 62 known-invalid DISCOVERED rows flipped to `INVALID_URL` by one-time DB pass

### 22. Stale pre-filter backlog rows and error-redirect URLs bypass every discovery filter (Resolved 2026-07-22, mitigated)
Two related leak paths observed live 2026-07-22: (1) the 2,001-row DISCOVERED backlog predates every FunnelEngine filter added since #5, so long-dead junk (`www.workday.com/en-us/company/careers/hiring-programs.html` — a marketing page, not a posting) still reaches workers and burns full cycles — filters only gate *new* discoveries; (2) Yahoo discovery indexed Greenhouse's own expired-posting redirect (`job-boards.greenhouse.io/remotecom?error=true`) as a "live job". Fixes: `isValidATSUrl` rejects any URL carrying an `error` query parameter (regression cases added to `TestIsValidATSUrl`), and a one-time DB pass flipped 62 known-invalid DISCOVERED rows (32 `www.workday.com`/`developer.workday.com`, 14 `error=` URLs, 16 `/search`/`?workplaceType=` listing pages) to a dedicated `INVALID_URL` status — reversible, distinct from fit-based SKIPPED; agent restarted to shed the already-queued copies (queue: 2001 → 1939). *Mitigated, not eliminated:* other stale-junk shapes may still lurk in the remaining backlog; the submit-time dead-page/redirect checks (#15) remain the backstop.

**Hardened same night (01:17):** ten minutes after the first purge, `digital.workday.com/en-us` — a third Workday corporate subdomain the LIKE patterns didn't cover — reached the Learner Module from the stale backlog. Purge-by-pattern is whack-a-mole, so the junk rules are now code, applied at two layers: exported `scraper.IsKnownJunkJobURL` (any non-`myworkdayjobs` `workday.com` host, `error` query params, `/search` listing paths, `?workplaceType=` board filters — deliberately a blacklist so legitimate non-ATS company-site URLs from RemoteOK still pass) backs both `isValidATSUrl` at discovery and a new worker-intake guard in `cmd/agent` that flips matching queue rows to `INVALID_URL` before any scoring or tailoring. A second DB pass flipped 36 more `workday.com` corporate rows. `TestIsKnownJunkJobURL` covers all live-seen shapes plus must-pass regressions.

**Hardened again (01:33): company board-index pages.** `careers.smartrecruiters.com/aristanetworks` — "Careers at Arista Networks", the tenant's whole job board, confirmed by direct fetch — scored 80 and burned docs + Learner + a Vision fallback. Added the general rule to `IsKnownJunkJobURL`: on path-tenant ATS hosts (SmartRecruiters, Lever, Greenhouse, Ashby, Workable, Jobvite), a path with ≤1 segment is the company's board index, never a posting (real postings always carry `/company/<id-or-slug>` or deeper). The matching DB pass was the biggest cleanup of the night: **181 queued board-index rows** flipped to `INVALID_URL` (~9% of the backlog, each worth a 10-40 minute wasted cycle). Queue after restart: 1826.


---

## 21. [SaveFormMapping caches non-JSON LLM output, poisoning every future visit to the domain](#21-saveformmapping-caches-non-json-llm-output-poisoning-every-future-visit-to-the-domain)

**Table rationale cell (original):** Confirmed live 2026-07-22 00:31: cached mapping for `www.workday.com/en-us` began with prose ("invalid character 'T'"), guaranteeing a parse failure and a multi-minute Vision fallback on every reuse until invalidated. `SaveFormMapping` now rejects anything failing `json.Valid`

### 21. SaveFormMapping caches non-JSON LLM output, poisoning every future visit to the domain (Resolved 2026-07-22)
Observed live 2026-07-22 00:31: `AttemptSubmit` loaded the cached mapping for `www.workday.com/en-us` and immediately failed `failed to parse mapping json: invalid character 'T' looking for beginning of value` — an earlier Learner Module response was prose, got cached verbatim, and every subsequent visit paid a guaranteed parse failure plus the Vision fallback (34 minutes on CPU Ollama this time). Fix: `SaveFormMapping` rejects input failing `json.Valid` with an explicit error, so a bad LLM response costs one failed save instead of a poisoned cache. `TestSaveFormMappingRejectsNonJSON` covers accept/reject/not-cached.


---

## 20. [Email tracker classifies unrelated emails as INTERVIEW_REQUESTED and writes them to the DB](#20-email-tracker-classifies-unrelated-emails-as-interview_requested-and-writes-them-to-the-db)

**Table rationale cell (original):** Observed live 2026-07-22 00:05 (first real logged-in scan): Google payment receipts ("we've received your payment") and a LinkedIn application-sent confirmation were classified INTERVIEW_REQUESTED, each logging "Updating database" — junk statuses written against fuzzy company matches ("google") can corrupt real application history

### 20. Email tracker classifies unrelated emails as INTERVIEW_REQUESTED and writes them to the DB
**Symptom:** during the first fully-logged-in `cmd/tracker` scan (2026-07-22 00:05, one cycle, ~2000s wall time — email analysis queues behind the live batch on the single Ollama slot), the tracker correctly detected a Glimpse rejection and a genuine Glimpse interview invitation, but also logged `Detected INTERVIEW_REQUESTED` + "Updating database" for clearly unrelated emails: four copies of a Google payment receipt ("google: we've received your payment for ..."), a LinkedIn "your application was sent to ClearlyAgile" confirmation (an application-*sent* notice, not an interview), and duplicate detections of the same recruiter thread (three identical updates).

**Why it matters:** whatever matching writes these statuses (company-name fuzzy match on "google"/"linkedin"?) is writing junk state transitions into real application history — the same DB the dashboard and conversion analytics (improvements #15) read. Needs: inspection of `pkg/tracker`'s classification prompt/heuristics and its DB-update matching logic; likely fixes are a stricter prompt with an explicit NOT_JOB_RELATED class, sender-domain allowlisting against known applied-to companies, and de-duplication by message ID. Not yet root-caused — filed from live log evidence only.

**Root cause, confirmed by reading `pkg/tracker/imap.go` — four distinct defects, one of them lucky:** (1) classification was pure keyword matching (any body containing "interview"/"next steps"/"availability" ⇒ INTERVIEW_REQUESTED) — the "AnalyzeEmail" metric in the logs is actually `ExtractRejectionReason`, only ever called on the rejection path, so no LLM was involved in classifying; (2) the DB update was `UPDATE ... WHERE company_name LIKE '%<label>%'` with the label being the sender domain's first token ("google", "linkedin"); (3) no de-duplication — the last 50 messages are refetched every 15-minute cycle, re-detecting the same threads; (4) **`cmd/tracker` never called `storage.InitDB`**, so `GetDB()` returned nil and every "Updating database" line in the tracker's entire history was a silent no-op — which is why defect (2) never actually corrupted anything, and also why the tracker has never done its job.

**Fix, verified live 2026-07-22 00:21:** `cmd/tracker` now initializes the DB; classification (`classifyEmail`, extracted pure for tests) short-circuits on not-job phrases (payment receipts, "your application was sent", "automated message"); a detected status only writes if the email matches a company we actually track — `storage.GetTrackedCompanies()` (APPLIED/INTERVIEW_REQUESTED/MANUAL_REQUIRED rows) matched by exact stored value against sender domain or subject, with sub-4-char and pre-#19 garbage labels ("en", "en-US") excluded and updates restricted to `company_name = ? AND status = 'APPLIED'`; processed Message-IDs persist in a new `processed_emails` table so each email is handled once ever. Live rerun of the same inbox: the Google receipts and LinkedIn confirmation produced nothing, the Glimpse emails were correctly held back as "matches no tracked application" (no funnel row exists for Glimpse — nothing to update). The rerun exposed one further hole, fixed in the same pass: tracked company "Remote" (remote.com) matched a recruiter subject merely containing the word "remote" — common-word company names (`commonWordCompanies`) now only match via the sender's domain, and the one bogus `Remote → INTERVIEW_REQUESTED` row the test run wrote was reverted to APPLIED by direct DB correction. Known accepted limitation: several emails in one thread arriving in the same first scan each get detected once (idempotent same-status updates); dedup silences them from the next cycle on. Tests: `TestClassifyEmail` (all live false positives as regression cases), `TestMatchTrackedCompany`, `TestEmailProcessedDedup`, `TestGetTrackedCompanies`.


---

## 19. [Workday URL parsing takes the locale/site segment as the company name](#19-workday-url-parsing-takes-the-localesite-segment-as-the-company-name)

**Table rationale cell (original):** Long-observed cosmetic defect, never filed on its own (referenced in passing in #12 and #17): Workday jobs get company names like `en-US`, `External_Career_Site`, `apply`, `en` from URL path segments instead of the real employer (GDIT, U-Haul, etc.), polluting `job_funnel`/dashboard rows and making log lines ambiguous

### 19. Workday URL parsing takes the locale/site segment as the company name
**Symptom:** Workday-hosted jobs enter the funnel with company names parsed from a URL path segment rather than the employer: `en-US` (U-Haul job in #12, and again live 2026-07-21 23:05-23:07 — `[Worker-1] Fetching job description for en-US...` / `Fit Score Pipeline: en-US scored 80!`), `External_Career_Site` (GDIT, this session), `ABCFinancialServices`, `apply`/`en` (the applytojob/pinpointhq cases noted in #17). Dashboard rows, log lines, and `job_funnel`/`applied_jobs` records all inherit the garbage name, which makes live debugging ambiguous (two different "en-US" jobs are indistinguishable at a glance) and the dashboard's company column meaningless for these rows.

**Likely shape:** wherever the scraper derives `CompanyName` from the URL, Workday URLs need the tenant subdomain (`gdit` from `gdit.wd5.myworkdayjobs.com`) rather than a path segment, and locale segments (`en-US`, `en`) should never be accepted as a name. Cosmetic, but cheap to fix and improves every future debugging session.

**Fix 2026-07-21:** added `companyFromURL` to `pkg/scraper/funnel.go`, used by the Yahoo-fallback discovery path (the only URL-based extraction — the SerpAPI path derives from the result title, RemoteOK from its API): subdomain-tenant platforms (Workday, ApplyToJob, Breezy, Recruitee, Pinpoint, BambooHR, Homerun) take the tenant host label; path-tenant platforms take the first path segment that isn't a locale (`en-US` regex) or generic section (`jobs`, `careers`, `apply`, ...); empty result falls back to "Unknown Company" instead of the old first-path-segment grab. Known trade-off documented in code: genuine two-letter company slugs get skipped as locale-like. `TestCompanyFromURL` covers every garbage name observed live (`en-US`, `External_Career_Site`, `apply`, `en`). Note: only affects newly discovered jobs — existing `job_funnel`/`applied_jobs` rows keep their old garbled names (a data backfill wasn't attempted; the URL remains the reliable key).


---

## 18. [Workday postings burn full Learner+Vision cycles against an auth-gated application flow with no fillable form](#18-workday-postings-burn-full-learnervision-cycles-against-an-auth-gated-application-flow-with-no-fillable-form)

**Table rationale cell (original):** Observed live 2026-07-21 ~22:51-23:01 (GDIT SRE `RQ219922` on `gdit.wd5.myworkdayjobs.com`): full tailoring (~6 min) + Apply click + Learner mapping (~3 min) + all three fill tiers + Vision fallback, all doomed from the start — Workday's real application form sits behind account-creation/sign-in, so no First Name field exists on any pre-auth page. Workday URLs are a large share of discovered jobs; each one wastes a 10-30 min cycle that should short-circuit to `applications/manual_submissions.md` on login-wall detection

### 18. Workday postings burn full Learner+Vision cycles against an auth-gated application flow with no fillable form
**Symptom:** live 2026-07-21, the GDIT Site Reliability Engineer posting (`gdit.wd5.myworkdayjobs.com/External_Career_Site/job/Any-Location--Remote/Site-Reliability-Engineer_RQ219922-1`) went through the entire pipeline: scored 80 (22:51:16), tailored documents generated and saved (~6 min, 22:57:22), Learner Module triggered, `clickApplyIfPresent` clicked an Apply-labeled element (22:57:22 — bug #8's fix firing live), `ExtractFormMapping` returned a mapping (23:00:27), then every fill tier failed — `failed to fill first_name: label fill for "First Name" failed: playwright: timeout: Timeout 15000ms exceeded` (23:01:12) — and the Vision fallback fired (bug #10's fix). Total: ~10 minutes of LLM + browser work on a form that was never fillable.

**Why it was never fillable:** Workday's application flow requires creating an account or signing in before the actual application form (with name/email/phone fields) is reachable. The pre-auth job page has a job description and an Apply button that leads to a sign-in/account-creation step — no First Name field exists in any frame, so the label, placeholder, and CSS-selector tiers (bugs #14/#16) all correctly time out, and Vision can only screenshot a login wall. Note this batch's Apply click landed on the fixed binary (built 22:46, includes #8/#10/#14/#16), so this is not any of those bugs recurring — it is a missing capability: nothing detects "this ATS requires an account" and short-circuits.

**Why it matters:** `*.myworkdayjobs.com` is one of the most common domains in the discovery funnel (GDIT, Cisco, U-Haul, Carrier, ABC Financial all seen this session), so this class silently dominates wasted cycle time the same way #5's `developer.workday.com` docs pages did before they were filtered.

**Second confirmed case 2026-07-21 23:15-23:16 (post-restart batch, all current fixes compiled in):** `redhat.wd5.myworkdayjobs.com/en-US` replayed the identical sequence — Learner Module "successfully mapped" (23:15:56), `failed to fill first_name: label fill for "First Name" failed: Timeout 15000ms` (23:16:41), Vision fallback triggered. Two independent Workday tenants (GDIT, Red Hat) with the same shape in one evening confirms this is the platform's auth-gate, not a per-tenant quirk. With Workday's share of the discovery funnel, this is now the leading candidate for the next fix.

**Suggested direction (not attempted):** detect the auth wall early — before document generation, or at latest before the Learner Module — via cheap signals (Workday domain + presence of "Sign In"/"Create Account" markers, or absence of any text input after the Apply click), then log the job to `applications/manual_submissions.md` with the tailored docs path and mark it `MANUAL_REQUIRED` in `job_funnel` instead of `FAILED_SUBMIT`. A full auto-account-creation flow is a much bigger (and riskier) feature and should be its own decision.

**Fix applied 2026-07-21 (unverified live):** two detection tiers in `pkg/submitter/browser.go`, both returning a new `ErrAuthWall` sentinel. (1) Known-host tier: `isKnownAuthGatedHost` suffix-matches `myworkdayjobs.com` (list: `authGatedATSHosts`), checked in `AttemptSubmit` right after document generation — docs are deliberately still generated, since they're the payload the manual application needs — and before the cached-mapping/Learner/Vision chain, cutting the wasted portion (~4+ min of mapping/fill/Vision) per Workday job. (2) Generic tier: in the Learner branch after `clickApplyIfPresent`, a password input present *plus* account-gate phrasing (`looksLikeAuthWallContent`: "sign in to apply", "create account", "returning candidate", etc.) short-circuits the same way, covering future auth-gated platforms beyond the known list. `cmd/agent/main.go` maps `errors.Is(err, submitter.ErrAuthWall)` to a new `MANUAL_REQUIRED` funnel status plus a `manual_submissions.md` entry (via existing `LogFailedSubmission`), distinct from `FAILED_SUBMIT`; the dashboard's `/api/metrics` now reports a `manual_required` count and `statusReason` explains the status (a dashboard UI tile is left for `improvements.md`). Tests: `TestIsKnownAuthGatedHost` (incl. `developer.workday.com` and a host-suffix-spoof URL staying false) and `TestLooksLikeAuthWallContent`. `go build/vet/test ./...` all pass in the `career-agent` container. Live batch restarted 23:26 with the fix compiled in — verification is the next Workday job in the queue short-circuiting to `MANUAL_REQUIRED` in seconds instead of ~10 minutes.

**Verified live 2026-07-21 23:34:36:** the next Workday job in the queue (`healthcatalyst.wd5.myworkdayjobs.com`) logged `is a known account-gated ATS — no pre-auth application form exists. Routing to manual submissions with tailored docs ready.` immediately after doc generation, and the worker recorded it as `queued for manual submission` (`MANUAL_REQUIRED`) — zero Learner/fill/Vision time spent, exactly as designed.


---

## 17. [ORDER BY last_updated DESC picked a stale row over a genuinely newer one](#17-order-by-last_updated-desc-picked-a-stale-row-over-a-genuinely-newer-one)

**Table rationale cell (original):** User caught this live from the dashboard: "Working on X since 1:48:26 AM" while it was actually ~9:59 PM, looking like an 8+ hour stall on a job really only ~10 minutes old. Initially misdiagnosed as a cosmetic container-timezone display issue; actually a real sort-order bug — mixing UTC and local-offset timestamp formats across two relaunches broke `ORDER BY last_updated DESC`'s plain TEXT comparison

### 17. ORDER BY last_updated DESC picked a stale row over a genuinely newer one (Resolved 2026-07-21)
**Symptom:** the user caught this directly from the dashboard screenshot: "Working on `apply` — Site Reliability Engineer since 1:48:26 AM" while the real time was ~9:59 PM — looked like an 8+ hour stall on a job that was actually only ~10 minutes old.

**First (wrong) diagnosis:** assumed the `career-agent` podman container had no timezone configured and `job_funnel.last_updated` was simply being displayed in UTC without conversion — filed as a "cosmetic" Minor issue. Checking `date` inside the container showed correct EDT already, which didn't fit that theory, so it was checked further instead of left as filed.

**Real root cause, confirmed via direct DB inspection:** two different `applytojob.com` postings (both company-labeled `"apply"` due to the same URL-parsing mislabeling as the pinpointhq `"en"` case) were both `status='PROCESSING'`: one genuinely current (`last_updated="2026-07-21T21:50:47.247518127-04:00"`, written by this session's `UpdateFunnelStatus` fix using a local-offset `time.Now()`), one stuck from ~20 minutes earlier during a brief window when an intermediate build wrote the same column via SQLite's `CURRENT_TIMESTAMP` (always UTC) — `last_updated="2026-07-22T01:48:26Z"`, already rolled over to the next calendar day in UTC terms. `ORDER BY last_updated DESC` in SQLite is a plain **TEXT** comparison, not a real chronological one: comparing `"2026-07-22..."` against `"2026-07-21..."` as strings, `'2' > '1'` makes the OLD stuck row sort first, even though it happened chronologically earlier once both are correctly interpreted with their offsets.

**Fix:** `UpdateFunnelStatus`/`UpdateFunnelStatusWithScore` now bind `time.Now().UTC()` explicitly (canonical, always-comparable `...Z` format) instead of a local-offset `time.Time`, guaranteeing every future write to this column is directly text-comparable regardless of local wall-clock rollovers. The dashboard now calls `.Local()` before formatting any of the three affected timestamp fields (plus `last_applied_at`, for consistency) for display, so storage stays sortable while the viewer still sees their own local time. Added a regression test confirming the canonical-UTC write format, and a dashboard test confirming UTC-stored values are correctly converted to local time in the API response. `go build/vet/test ./...` all pass.


---

## 16. [#14's label fallback needs a GetByPlaceholder tier too](#16-14s-label-fallback-needs-a-getbyplaceholder-tier-too)

**Table rationale cell (original):** Observed live 2026-07-21 three times (Jobvite x2, ApplyToJob): `GetByLabel("First Name")` found nothing even with the *correct* label text identified, most likely because those forms use `placeholder="First Name"` with no real `<label>`/`aria-label` association — a form that looks labeled to a human but isn't semantically one. Added a `GetByPlaceholder` tier to the fallback chain

### 16. #14's label fallback needs a GetByPlaceholder tier too (Resolved 2026-07-21)
**Symptom:** live 2026-07-21, three separate cases (`techinsights.applytojob.com/apply`, `jobs.jobvite.com/ninjaone`, `brightvisiontechnologies.applytojob.com/apply`) all logged `label fill for "First Name" failed (playwright: timeout: Timeout 15000ms exceeded.` — the label text identified was correct ("First Name", not a garbage value like the earlier phone-number mixup), yet `GetByLabel` still found nothing.

**Explanation:** modern minimalist ATS widgets commonly style an input's `placeholder` attribute to look exactly like a label visually (e.g. "First Name" greyed-out text inside the empty box) without an actual `<label for="...">` or `aria-label` association — visually indistinguishable from a real label to a human or a screenshot-reading vision model, but semantically invisible to Playwright's accessibility-tree-based `GetByLabel`.

**Fix:** added `GetByPlaceholderLoc` to the `fillTarget` interface (page + iframe targets) and a third tier to `safeFillWithLabelFallback`: label → placeholder → CSS-selector-guess, only advancing to the next tier if the previous one failed or wasn't available. Added tests for the new tier and for all three tiers failing together. `go build/vet/test ./...` all pass.

**First post-fix live case 2026-07-21 23:01 — not counter-evidence:** the GDIT Workday job failed `first_name` on a binary that includes this fix (built 22:46, fix committed 22:43), but that page is an auth-gated Workday flow with no name field present pre-login in any form (bug #18) — all three tiers failing there is correct behavior, not a placeholder-tier miss. A genuine verification still needs a placeholder-styled form (applytojob.com/Jobvite pattern) to recur post-fix.


---

## 15. [Dedicated Greenhouse/Lever handlers timing out waiting for the form to render](#15-dedicated-greenhouselever-handlers-timing-out-waiting-for-the-form-to-render)

**Table rationale cell (original):** Observed live 2026-07-21: two consecutive jobs on the *dedicated* (non-Learner-Module) handler path — `handleLever` (`mistral`) and `handleGreenhouse` (`nebius`) — both failed with `form failed to render in time: playwright: timeout: Timeout 30000ms exceeded.`. These handlers were assumed reliable all session (the confirmed-real `APPLIED` rows are all Greenhouse/Lever); this is the first evidence they can fail too. Not yet root-caused

### 15. Dedicated Greenhouse/Lever handlers timing out waiting for the form to render
**Symptom:** live 2026-07-21, two consecutive jobs — `jobs.lever.co/mistral/...` (`handleLever`) and `boards.eu.greenhouse.io/nebius/jobs/...` (`handleGreenhouse`) — both failed at the exact same error, `form failed to render in time: playwright: timeout: Timeout 30000ms exceeded.` (the shared error string at `pkg/submitter/browser.go:375` and `:431`, one per handler, each behind its own `WaitForSel(..., 30000)` call). Notable because these are the *dedicated*, hand-written handlers, not the Learner Module path this session has spent most of its time on — every confirmed-real `APPLIED` row checked earlier this session was Greenhouse/Lever, so this is the first live evidence that path can fail too, not just the generic fallback.

**Not yet root-caused (superseded below).** Both handlers already call `resolveFillTarget(page)` (bug #4's iframe-fallback resolver) before their `WaitForSel`, and that call happens several minutes after page load (after `generateDocsFunc`'s LLM call), so a simple "hasn't rendered yet" race seems unlikely per the same timing argument that ruled it out for #8/#9 — but this hasn't been directly verified for these two specific postings the way #4/#8/#9 were, via a standalone diagnostic script inspecting the actual live DOM. Needs that same treatment before assuming a fix.

**Root cause, confirmed via standalone diagnostic 2026-07-21 (~23:35): not a rendering/timeout problem at all — every case was a dead or moved posting the code had no way to detect.** A playwright-go script (same launch args/UA/stealth as the app) loaded each failing URL directly:
- `jobs.lever.co/mistral/f76907fd-...` (and its `/apply` variant): renders Lever's expired-posting shell — title `Not found – 404 error`, **zero** inputs/forms — served with HTTP 200, in phrasing `isDeadJobPage` didn't match.
- `boards.eu.greenhouse.io/nebius/jobs/4558243101`: **silently redirects to `careers.nebius.com/`** — the company migrated its board off Greenhouse; the handler then waited 30s for `input#first_name` on a generic careers landing page.
- `job-boards.greenhouse.io/remotecom/jobs/7778860003` (bug #7's URL): **redirects to `job-boards.greenhouse.io/remotecom?error=true`** — Greenhouse's expired-posting redirect, the same class bug #9 caught on Jobvite (`?error=404`).

So "form failed to render in time" was always the same story as #9 — jobs dying between discovery and the worker reaching them (queue latency is hours) — surfacing through a different, misleading error because nothing checked *where navigation actually landed*. The dedicated handlers themselves are likely fine; no genuinely-live posting failure has been observed on them.

**Fix applied 2026-07-21 (unverified live):** two additions in `pkg/submitter/browser.go`, both running before the costly doc generation: (1) `deadRedirectReason(applyURL, finalURL)` — flags an `error` query parameter on the post-navigation URL (Greenhouse `?error=true`, Jobvite `?error=404`) or a redirect to a different registrable domain (`registrableDomain` last-two-labels approximation; within-ATS board migrations like `boards.greenhouse.io` → `job-boards.greenhouse.io` are deliberately allowed through). (2) `"404 error"` added to `deadJobPhrases` for Lever's 404 shell. Tests: `TestDeadRedirectReason` (all three live-confirmed cases plus benign-redirect regressions), `TestRegistrableDomain`, new `TestIsDeadJobPage` case. `go build/vet/test ./...` pass. Live verification: the next expired posting hit by any handler path should log `job posting is dead or expired: redirected...` in seconds instead of a 30s form timeout after minutes of doc generation.

**Verified live 2026-07-22 00:11:** a jobgether job (Lever — the exact company whose postings produced the original 2026-07-20 timeouts) scored 80 at 00:11:22 and was rejected `job posting is dead or expired` at 00:11:25 — three seconds, before document generation, versus the old ~5-minute doc generation plus 30-second timeout path.


---

## 14. [No accessible-label fallback for form-field filling, only CSS selector guessing](#14-no-accessible-label-fallback-for-form-field-filling-only-css-selector-guessing)

**Table rationale cell (original):** Structural gap behind every fill failure in #4/#8/#9/#10: the Learner Module only ever produces a guessed CSS selector, with no fallback to the field's accessible label (`<label for>`/`aria-label`) that WCAG-compliant enterprise ATS forms reliably expose even when raw name/id attributes are obfuscated. Confirmed against current Playwright best-practice guidance via web search: `GetByLabel`-style "user-first locators" are the recommended primary strategy over raw CSS selectors for exactly this kind of unknown-markup automation

### 14. No accessible-label fallback for form-field filling, only CSS selector guessing (Resolved 2026-07-24)
**Symptom:** every single fill failure diagnosed this session (bugs #4, #8, #9, #10) had the same shape — the Learner Module's `ExtractFormMapping`/`ExtractFormMappingVision` confidently returned a plausible-looking CSS selector (e.g. `input[name='first_name']`) that simply didn't match the real page, and there was no second strategy to fall back on beyond re-guessing via Vision.

**Root cause / gap:** `handleDynamic` only ever calls `target.Loc(selector).Fill(...)` with the LLM's raw CSS-selector guess. Enterprise ATS platforms are generally WCAG-compliant (legal/accessibility requirements), meaning their form fields reliably carry a stable, human-readable accessible label — a `<label for="...">` association or an `aria-label`/`aria-labelledby` attribute — even when the underlying `name`/`id`/`class` attributes are auto-generated, obfuscated, or vary wildly by ATS vendor theme (exactly the pattern behind every failure this session). Checked current Playwright best-practice guidance via web search: "user-first locators" like `GetByLabel`/`GetByRole` are the explicitly recommended primary strategy for resilient automation against unknown markup, precisely because they're tied to what a human sees rather than implementation details — ahead of raw CSS selectors, which can also silently match the wrong element without ever raising an error.

**Fix applied 2026-07-21:** added `GetByLabelLoc(text string) playwright.Locator` to the `fillTarget` interface (implemented for both the page and iframe targets), added a `Labels map[string]string` field to `FormMapping` alongside the existing `Fields`, and updated both Learner Module prompts (`pkg/mcp/client.go`) to also return each field's visible accessible label text. Added `safeFillWithLabelFallback` in `pkg/submitter/browser.go`: tries the label first when one was identified, falls back to the CSS selector guess if the label attempt fails or no label was available. Wired into `handleDynamic`'s four PII fields (first_name, last_name, email, phone). Added 4 new tests covering label-first success, selector fallback on label failure, both failing, and no-label-available. `go build/vet/test ./...` all pass.

**Not yet verified live:** needs a fresh batch run to confirm the label fallback actually improves real-world fill success rate.

**Resolved 2026-07-24, via a direct end-to-end test:** live traffic through 2026-07-24 kept confirming the label tier engaging (e.g. bug #18's Workday cases) but never on a page where the underlying form was actually fillable — the auth wall meant no label, placeholder, or selector tier could ever have succeeded there regardless of this fix. Same "mechanism firing, outcome unreachable live" shape as bug #4. Closed via `TestAttemptSubmit_ClickToRevealPlusLabelFallback_EndToEndSuccess` (`pkg/submitter/browser_test.go`): drives real `AttemptSubmit` against a mock page where the Learner Module's mapping has deliberately wrong CSS selectors for every field but correct labels, and confirms the submission still reaches `isSubmissionConfirmed` — proof the label fallback is what carried the fill, not the (broken) selector. `go build/vet/test ./...` all pass.


---

## 13. [Ollama gets kernel OOM-killed under this machine's real RAM ceiling](#13-ollama-gets-kernel-oom-killed-under-this-machines-real-ram-ceiling)

**Table rationale cell (original):** Confirmed via `journalctl -k`: two live `llama-server` OOM-kills this session (14:02, 15:57), the second immediately after #10's Vision fallback started loading a second model concurrently with the 30B text model on a 29GB-RAM machine. Not a soft/config limit — the kernel OOM-killer genuinely ran out of memory. Mitigated (not eliminated) by setting `OLLAMA_MAX_LOADED_MODELS=1`

### 13. Ollama gets kernel OOM-killed under this machine's real RAM ceiling (Resolved 2026-07-21, mitigated not eliminated)
**Symptom:** two `context deadline exceeded`/`EOF`/`connection refused` incidents this session (~14:02, ~15:57), each briefly breaking every in-flight LLM call across the batch until Ollama's systemd unit auto-restarted.

**Root cause, confirmed via `journalctl -k`:** genuine kernel OOM-killer events both times — `Out of memory: Killed process ... (llama-server) ... anon-rss:13-15GB` — not a soft config limit or app-level bug. `free -h` showed this machine has 29GB total RAM with only ~5.7GB "available" at rest. The second kill landed immediately after bug #10's Vision fallback first triggered live, which loads a second model (`qwen2.5vl:7b`) concurrently with the already-loaded 30B text model (`qwen3:30b-instruct`, ~19GB alone) — tonight's own fix increased peak memory pressure.

**Mitigation applied:** added `Environment=OLLAMA_MAX_LOADED_MODELS=1` to `~/.config/systemd/user/ollama.service.d/override.conf` (alongside the existing `OLLAMA_CONTEXT_LENGTH=6144` from bug #3's fix) and restarted the service. This makes Ollama evict one model before loading the other instead of holding both simultaneously, trading a ~1-2 second model-swap delay (per `journalctl` timings, negligible next to the 5-30 minute generation calls) for not exceeding available RAM when text and vision calls interleave.

**Not eliminated:** this reduces peak memory (one model instead of two) but doesn't guarantee headroom against everything else competing for the same 29GB (desktop environment, browser automation, other apps) — a third OOM kill under sufficient combined pressure is still possible. A real fix would need either more RAM, a smaller model, or a dedicated headless environment for long batch runs.


---

## 12. [Same job URL reprocessed repeatedly, hitting a UNIQUE constraint on applied_jobs.url](#12-same-job-url-reprocessed-repeatedly-hitting-a-unique-constraint-on-applied_jobsurl)

**Table rationale cell (original):** Root-caused as `AddToFunnel`'s `ON CONFLICT ... DO UPDATE SET status=excluded.status` silently resetting an in-progress/finished job's status back to `DISCOVERED` on every re-discovery, combined with FunnelEngine unconditionally re-queuing on any successful `AddToFunnel` call. Observed live re-running individual jobs 3-5+ times, 20-30+ min each — very likely the dominant reason `applied` never moved during tonight's ~7 hour session

### 12. Same job URL reprocessed repeatedly, hitting a UNIQUE constraint on applied_jobs.url (Resolved 2026-07-21)
**Symptom:** live 2026-07-21, multiple different jobs (`developer.workday.com/rest-api-explorer`, `carrier.wd5.myworkdayjobs.com` "jobs", `abcfinancial.wd5.myworkdayjobs.com` "ABCFinancialServices", `uhaul.wd1.myworkdayjobs.com` "en-US", and more) each got reprocessed 3-5+ times over the course of the session, each full attempt taking 20-30+ minutes (score → tailor → Learner Module/Vision → fill), before eventually failing with `failed to generate application documents: UNIQUE constraint failed: applied_jobs.url`. Confirmed via `ProcessJobApplication API Call #N` counters: three near-simultaneous log lines for the same company showed three *different* call numbers, and independent LLM scoring calls for the same URL produced different scores (80, 85, 80) moments apart — proof these were genuinely separate, independent pipeline runs for the same job, not one run's sequential log lines. This was very likely the dominant reason `applied` never moved during the session's ~7 hour live batch despite continuous activity — misdiagnosed for a while as a multi-worker concurrency bug (ruled out: only `Worker-1` was ever active, confirmed via a properly date-anchored log check after an earlier unanchored `awk` filter gave a false positive).

**Root cause, confirmed by reading `pkg/storage/manager.go` and `pkg/scraper/funnel.go`:** `AddToFunnel` (called by all three FunnelEngine discovery paths, always with `status="DISCOVERED"`) used `INSERT ... ON CONFLICT(url) DO UPDATE SET status=excluded.status`. Since the same URL routinely gets rediscovered across separate search passes (observed literally all session — the same URLs "discovered" repeatedly hours apart), every rediscovery silently reset that job's `job_funnel.status` back to `DISCOVERED`, even while a worker was actively processing it or had already finished it — making it eligible to be picked up again by `GetDiscoveredJobs()`/pushed again to `jobChan`. Compounding this, all three FunnelEngine call sites pushed the job onto `jobChan` unconditionally on any successful `AddToFunnel` call (insert *or* update), so a rediscovery queued a fresh duplicate work item regardless of the status field at all. The existing "Duplicate check" (`storage.HasApplied`) only ever caught the case where a *prior* attempt had already fully succeeded — it did nothing for a job that was currently in-flight or had merely failed previously, so a failing job could be retried indefinitely, each time from scratch, for as long as it kept getting rediscovered.

**Fix:** `AddToFunnel` now uses `ON CONFLICT(url) DO NOTHING` and returns whether a row was genuinely newly inserted (via `RowsAffected()`); all three FunnelEngine call sites (`DiscoverJobs`, `discoverWithYahooHTML`, `discoverWithRemoteOK`) only push to `jobChan` when it reports a new insert. Added a regression test in `pkg/storage/manager_test.go` covering that re-adding an already-known URL neither reports as new nor resets its already-advanced status. `go build/vet/test ./...` all pass.

**Not yet verified live:** needs a fresh batch run to confirm `applied` actually starts moving now that jobs aren't burning repeated 20-30 minute cycles on the same handful of URLs.

**One-time data correction 2026-07-21 (post-relaunch):** confirmed a handful of post-fix `UNIQUE constraint` echoes were harmless one-time artifacts of pre-existing corruption, not a repeat of the bug: exactly 23 `job_funnel` rows had `status='DISCOVERED'` while *also* already having a matching `applied_jobs` row — a combination only possible via this exact bug (a job that had already fully succeeded, whose status was later reset to `DISCOVERED` by an earlier rediscovery, before the fix landed). Corrected those 23 rows' status to `APPLIED` directly via a one-off script (dashboard `applied` count: 35 → 58). Deliberately did **not** touch the much larger set of `job_funnel` rows that have a matching `applied_jobs` entry but a different status (`PROCESSING`: 135, `FAILED_SUBMIT`: 90, `BLOCKED_CAPTCHA`: 52, `FAILED_SCORE`: 3) — `applied_jobs` only records that a tailored resume/cover letter was generated and saved (`SaveApplication`, called early in `AttemptSubmit`), not that the actual browser form submission succeeded, so those other statuses may already correctly reflect a real submission failure (matching bugs #4/#8/#9/#10's exact failure shape: doc generation succeeds, the fill/submit step after it doesn't) and reclassifying them without individual verification would overstate real progress. The honest total is 58 fully-successful applications recorded across this project's history, not the 430 raw `applied_jobs` rows.

**Correction to the correction, 2026-07-21 (~21:30):** the assumption above — "`status='DISCOVERED'` + a matching `applied_jobs` row can only mean a job that had already fully succeeded" — was wrong. It missed a second, equally possible path to that same state: docs generated (→ `applied_jobs` row written) → the actual browser submission then failed → status should have gone to `FAILED_SUBMIT`, but instead got reset to `DISCOVERED` by a later rediscovery (this bug, active until the earlier fix landed). Caught while building the dashboard's "last applied" feature: it surfaced `jobs.jobvite.com/cloudone-digital/search` — the exact FunnelEngine `/search`-listing false positive identified hours earlier (bug #11) — as a "successful application," which is structurally impossible (a search page has no form to submit). Cross-checked all 58 `status='APPLIED'` rows against `execution_logs` (an append-only audit trail populated by `pipeline.SaveCheckpoint`/`storage.LogExecution`, distinct from the unused `execution_state` table) for each URL's most recent logged status: **12 of the 23 corrected rows had a last-known status of `FAILED`, not `COMPLETED`.** Reverted those 12 to `FAILED_SUBMIT` (their true last-known state) via a second one-off script. Corrected honest total: **46**, not 58. This entire episode is itself worth internalizing: a "safe-looking" data correction based on a single necessary-but-not-sufficient condition still produced real errors — worth a second independent signal (here, `execution_logs`) before trusting a bulk correction, not just after.

**Post-fix verification was contaminated by an orphaned-process bug, not this bug:** continued apparent duplicate processing after this fix landed (e.g. the same job "skinspirit" logging 4 near-simultaneous `Initiating submission sequence` lines) was traced to five separate orphaned `go run` child processes left running from the session's own earlier `kill -9`-on-wrapper-only relaunches, not a gap in this fix — see the operational note above the Usability Gate. Once all orphans were killed and the agent relaunched as a single directly-run binary, this needs one more clean observation window to fully confirm.


---

## 11. [FunnelEngine lets Jobvite `/search` listing pages into the pipeline](#11-funnelengine-lets-jobvite-search-listing-pages-into-the-pipeline)

**Table rationale cell (original):** Observed live 2026-07-21: `jobs.jobvite.com/cloudone-digital/search` (a listing page, not a posting) was scored and reached `AttemptSubmit`, same false-positive class as #5/#7 but for Jobvite. Not yet root-caused — `isValidATSUrl` only gates the Yahoo-fallback path; worth checking whether Jobvite needs the same kind of path-based tightening Workable got in #5

### 11. FunnelEngine lets Jobvite `/search` listing pages into the pipeline
**Symptom:** live 2026-07-21, `jobs.jobvite.com/cloudone-digital/search` scored 80 and reached `AttemptSubmit`/the Learner Module, same as #5's Workday/Workable false positives — a listing/search page, not an individual posting, so the eventual `failed to fill first_name` was never fixable. Not yet root-caused with the same rigor as #5 (no direct DOM inspection done this session, just the URL shape and the eventual failure); needs its own pass to confirm the fix shape (likely a `/search` path rejection for `jobvite.com` similar to Workable's, added to `isValidATSUrl` in `pkg/scraper/funnel.go`).

**Fix 2026-07-21:** exactly the anticipated shape — `isValidATSUrl` now rejects `jobvite.com`/`*.jobvite.com` URLs whose path ends in `/search` or contains `/search/`, mirroring #5's Workable rule. Real Jobvite postings (`/job/<id>` paths) covered by regression cases in `TestIsValidATSUrl`. `go build/vet/test ./...` pass.



---

## 10. [DOM-mapped fill failures never fell back to the Vision module, only outright mapping failures did](#10-dom-mapped-fill-failures-never-fell-back-to-the-vision-module-only-outright-mapping-failures-did)

**Table rationale cell (original):** Structural observation after diagnosing #4, #8, and #9 in one session: every live failure was the Learner Module confidently returning a plausible-but-wrong selector mapping (a fill failure), never an outright mapping-generation failure — yet `AttemptVisionSubmit` (screenshot + visual LLM reasoning, already implemented and wired to a locally-pulled vision model) only triggered on the latter. A fill failure just deleted the cache and gave up

### 10. DOM-mapped fill failures never fell back to the Vision module, only outright mapping failures did (Resolved 2026-07-24)
**Observation:** across this session's diagnosis of bugs #4, #8, and #9, every single live failure had the same shape: the Learner Module's `ExtractFormMapping` call *succeeded* — it confidently returned a plausible-looking JSON mapping (e.g. `input[name='first_name']`) — but that mapping was simply wrong against the real page (iframe-embedded, click-gated, or a dead/redirected listing). `AttemptSubmit`'s existing fallback chain in `pkg/submitter/browser.go` only invoked `AttemptVisionSubmit` (screenshot -> vision-LLM reasoning -> selectors, `pkg/submitter/vision.go`) when `ExtractFormMapping` returned an outright error or empty string; a fill failure on an otherwise "successful" mapping just deleted the cache and returned an error, never trying the more robust visual path.

**Why this matters:** confirmed the Vision path is fully wired and usable in this environment — `OLLAMA_VISION_MODEL=qwen2.5vl:7b` is set in `.env` and the model is present in `ollama list` — so this wasn't a missing-capability gap, just an under-triggered one. A single structural change here has a chance to subsume an open-ended class of future DOM surprises (new ATS themes, new click-reveal patterns, new field-naming conventions) instead of requiring a dedicated bugfix per pattern as they're discovered one at a time, which is how #4, #8, and #9 all happened this session.

**Fix applied 2026-07-21:** in both places `AttemptSubmit` calls `handleDynamic` (the cached-mapping path and the fresh Learner-Module-mapping path), a fill failure now invalidates the cache and calls `AttemptVisionSubmit` before giving up, instead of returning the error immediately. `go build/vet/test ./...` all pass.

**Not yet verified live:** needs a fresh Learner Module fill failure in a live run to confirm the Vision fallback actually fires and improves the outcome, and to check it doesn't meaningfully slow down the common case (Vision calls are a second LLM round-trip on top of the mapping call, only paid when the mapping-based fill already failed, so worst case is not much worse than today's dead end — but not yet measured).

**Partial live verification 2026-07-21 23:01:** the trigger half is confirmed — on the GDIT Workday job (bug #18), the Learner-mapped fill failed and the log immediately shows `Invalidating cache. Falling back to Vision module` → `Taking a full-page screenshot` → `Transmitting screenshot`, i.e. the fill-failure→Vision path this fix added fired exactly as designed on a live job for the first time. The outcome half remains unverified: the batch was deliberately restarted (~23:05, to load an expanded roles list) while that Vision attempt was still in flight, and the underlying page was an auth-gated Workday form no fill strategy could have succeeded on anyway.

**Resolved 2026-07-24, via a direct end-to-end test:** three more days of live batch traffic (including the dedicated 82-job re-verification run) never produced a case where a genuine Learner Module fill failure landed on a page that could still be filled successfully — every live trigger observed happened to hit a structurally unfillable page (Workday's auth wall), the same "mechanism confirmed firing, outcome unreachable via live traffic" shape bug #4 hit. Closed the same way: `TestAttemptSubmit_VisionFallback_EndToEndSuccess` (`pkg/submitter/browser_test.go`) drives the real `AttemptSubmit` → `AttemptVisionSubmit` orchestration (not just its helpers in isolation) against a mock page where the Learner Module's mapping is deliberately wrong (fill genuinely fails, not just an outright mapping error), confirms `ExtractFormMappingVision` is actually invoked with a real screenshot payload, and confirms the resulting vision-remapped fill carries the submission all the way to `isSubmissionConfirmed` returning true. `go build/vet/test ./...` all pass.


---

## 9. [Dead-job-posting detection missed common phrasings, wasting cycles on expired listings](#9-dead-job-posting-detection-missed-common-phrasings-wasting-cycles-on-expired-listings)

**Table rationale cell (original):** Confirmed live 2026-07-21: a Jobvite posting that had expired between discovery and `AttemptSubmit` looked exactly like a #8 click-to-reveal failure until a standalone diagnostic script showed it redirected to a `?error=404` page whose text ("job listing no longer...") didn't match the single hardcoded dead-job phrase the code checked for

### 9. Dead-job-posting detection missed common phrasings, wasting cycles on expired listings (Resolved 2026-07-21)
**Symptom:** live 2026-07-21 while re-verifying #8's fix, a Jobvite posting (`jobs.jobvite.com/dwt/job/o79Qzfwp/apply`) failed with the same `failed to fill first_name` symptom as bugs #4/#8, but `clickApplyIfPresent` (bug #8's just-applied fix) never logged a click attempt.

**Root cause, confirmed via a standalone diagnostic script:** wrote a small headless-Playwright program (`mxschmitt/playwright-go`, same version and browser launch args as the app) to load the URL directly, wait past network-idle plus an extra settle period, and dump input/form/iframe counts, any "Apply"-text elements, and a screenshot. Found the page had actually redirected to `jobs.jobvite.com/careers/dwt/jobs?error=404` — the job had simply expired between discovery and `AttemptSubmit` (jobs can sit in the funnel queue for hours before a worker reaches them) — with page text reading "the job listing no longer [exists]". `AttemptSubmit`'s existing dead-job guard in `pkg/submitter/browser.go` only checked for the literal substring `"job is no longer available"` (plus two other exact phrases), which this ATS's wording didn't match, so the dead page sailed through the check and wasted a full generation + Learner Module cycle before failing with a misleading fill-timeout error that looked exactly like bugs #4/#8.

**Fix:** extracted the inline check into `isDeadJobPage(content string) bool` and widened the phrase list to also cover `"no longer exists"`, `"no longer accepting applications"`, `"job listing no longer"`, `"posting is no longer active"`, and `"job has been filled"`. Added `TestIsDeadJobPage` (pure string-matching test, no browser needed). `go build/vet/test ./...` all pass.


---

## 8. [Dynamic/Learner Module fill path never clicks an "Apply" button to reveal click-to-reveal application forms](#8-dynamiclearner-module-fill-path-never-clicks-an-apply-button-to-reveal-click-to-reveal-application-forms)

**Table rationale cell (original):** Confirmed live 2026-07-21 on a Breezy.hr posting: `resolveFillTarget` correctly found no real form on the page or in any iframe (the only main-page input was an unrelated readonly referral-link box), yet `handleDynamic` filled against it anyway instead of first clicking the page's "Apply" button to reveal the actual form (a fancybox/lightbox modal on this legacy Breezy portal theme). Distinct from #4 — not an iframe problem, a click-to-reveal problem the Learner Module's fill path has no step for

### 8. Dynamic/Learner Module fill path never clicks an "Apply" button to reveal click-to-reveal application forms (Resolved 2026-07-24)
**Symptom:** live 2026-07-21, `AttemptSubmit` for a Breezy.hr posting (`jway-group.breezy.hr/p/419b44576d64-backend-developer-laravel-aws`) reached the Learner Module, successfully "mapped" the page, then failed with `failed to fill first_name: playwright: timeout: Timeout 15000ms exceeded` on the re-attempt.

**Root cause, confirmed by direct inspection:** fetched the raw page (`curl`, same URL) and found **zero `<form>` tags anywhere**, exactly **one `<input>`** on the page (a `readonly` referral-link text box, unrelated to any application form), and **one `<iframe>`** (a Google Maps embed for the office location, also unrelated). The page loads `jquery.fancybox` and other circa-2017 Breezy portal assets and has visible "Apply" button text — this ATS's real application form is a fancybox/lightbox modal that only renders into the DOM after a user clicks "Apply," not something present on page load or embedded in an already-present iframe. `resolveFillTarget` (bug #4's fix) correctly detected this: it found no real form on the main page or in any frame and fell back to the page target, exactly as designed for this case — so this is not a #4 regression. The actual bug is one level up: `handleDynamic` in `pkg/submitter/browser.go:396` goes straight from the Learner Module's field mapping to `safeFill`/`WaitForSel` calls with no step that clicks an "Apply"/"Apply for this Job" trigger first, so it's waiting for fields that will never appear without that click. The Learner Module's `ExtractFormMapping` DOM capture likely has the same blind spot — it should be inspecting the DOM *after* the apply-click, not before.

**Suggested fix:** before invoking the Learner Module / `handleDynamic` fill path, look for a common "Apply"-labeled clickable element (button/link with text matching `apply` case-insensitively, or common ATS-specific selectors) and click it first if present, then re-resolve the fill target and re-capture the DOM for `ExtractFormMapping` if using the Learner Module. Needs design thought on how to detect "form already present" (skip the click) vs. "form behind a click" (do the click) without false-triggering on unrelated "Apply Now" marketing links elsewhere on the page. Not attempted this session — found while investigating #4's live verification, filed for its own dedicated session.

**Second confirmed case 2026-07-21 (same session, SmartRecruiters this time):** `jobs.smartrecruiters.com/sosi1/3743990013881284-cloud-web-developer` — the exact ATS platform bug #4 was originally diagnosed and fixed against — hit the identical failure shape: Learner Module mapped successfully, then `failed to fill first_name` after the full 15s timeout. Direct inspection (`curl`) again found zero `<form>` tags, zero `<iframe>` tags, and one unrelated stray `<input>` (a "copy link to share via WeChat" text box), plus script tags hinting at a client-rendered framework. Critically, `resolveFillTarget`'s check runs several *minutes* after page navigation (after the 5-10 minute `generateDocs()` LLM call completes, see call order in `pkg/submitter/browser.go` around line 140-178) — long enough that a merely slow-rendering SPA should have finished by then. That timing rules out "raced a slow render" as the explanation and makes "form is genuinely gated behind a click, and nothing in the code ever clicks it" the best-supported explanation for both cases. Net effect: in every live case observed this session (2 ATS platforms, 4 total attempts: Breezy x3, SmartRecruiters x1), the failure was this bug, not bug #4's iframe scenario — bug #4's fix has still not been positively exercised (no case has hit the `using embedded iframe` log line), so it remains unverified, though nothing here contradicts it either. This bug (#8) is now the best next candidate for unblocking the Usability Gate's live-batch checkbox, likely more impactful than further #4 investigation.

**Fix applied 2026-07-21:** added `clickApplyIfPresent(page)` in `pkg/submitter/browser.go`, called once in `AttemptSubmit`'s Learner Module branch (`else if mapper != nil`) right before the DOM is captured for `ExtractFormMapping` and before `resolveFillTarget` — so both the Learner Module's mapping and the later fill see the post-click DOM. Looks for `button:has-text('Apply'), a:has-text('Apply')`, clicks the first match if any exist, no-ops otherwise. Scoped only to the Learner Module path (not the dedicated Greenhouse/Lever/LinkedIn handlers, which weren't implicated, and not the validation-error retry branch). Added `TestClickApplyIfPresent_NoApplyButton` and `TestClickApplyIfPresent_ClicksWhenFound` in `pkg/submitter/browser_test.go`. `go build/vet/test ./...` all pass.

**Delegation note:** the first attempt (agy/Gemini 3.1 Pro) hit an account-wide quota error before writing anything (git status stayed clean, correctly caught before assuming success). The retry (agy/GPT-OSS 120B) returned exit 0 and a plausible-sounding diff, but the actual diff (verified via `git diff`, per this repo's own standing rule to never trust a delegate's self-report) had duplicated `resolveFillTarget`'s body with a stray extra `}` — would not have compiled. Reverted and applied the fix directly instead, including working around a genuine Go gotcha along the way: `playwright.Locator`'s own `Locator(...)` chaining method collides with the field name Go gives an anonymously-embedded `playwright.Locator` in a mock struct; fixed by embedding via a local type alias (`type pwLocator = playwright.Locator`) instead, which lets the embedded field take a different name while remaining the identical type.

**Not yet verified live:** the currently-running live batch (started before this fix was written) is a separately compiled `go run` process and does not have this fix — a fresh run is needed to confirm it actually resolves the Breezy/SmartRecruiters cases without breaking ATS platforms that don't need a click.

**Relaunched with the fix 2026-07-21, still unresolved by first look:** killed and relaunched the batch with this fix compiled in. The next Learner-Module-routed job (Jobvite, `jobs.jobvite.com/dwt/job/o79Qzfwp/apply`) still failed with the identical `failed to fill first_name` symptom, `clickApplyIfPresent` did not log a click (confirmed via `grep` — no "Clicked an Apply-labeled element" line). Wrote a standalone headless-Playwright diagnostic script (same browser launch args and `mxschmitt/playwright-go` version as the app) to inspect this specific URL directly. **Turned out to be a different bug entirely, not this one:** the job had expired and redirected to `jobs.jobvite.com/careers/dwt/jobs?error=404`, whose rendered text ("the job listing no longer...") didn't match the app's existing (too narrow) dead-job detection, so it wasn't caught before reaching the Learner Module. Filed and fixed separately as bug #9. This means #8's actual click-to-reveal fix still has zero live evidence either confirming or refuting it beyond the two original cases that motivated it (both observed *before* the fix existed) — genuinely re-verifying #8 needs a fresh Breezy or SmartRecruiters case to recur post-fix, which hasn't happened yet this session.

**Later confirmed firing live 2026-07-22** (GDIT Workday job, bug #18's Details): `clickApplyIfPresent clicked an Apply-labeled element` logged correctly post-fix — the click mechanism itself works. **Resolved 2026-07-24, via a direct end-to-end test:** no live case since has recurred where a click-to-reveal page's form was actually fillable afterward (Workday's cases were auth-gated regardless of the click). Closed via `TestAttemptSubmit_ClickToRevealPlusLabelFallback_EndToEndSuccess` (`pkg/submitter/browser_test.go`), the same test that closes #14: a mock page with zero form fields until an "Apply" element is clicked (the exact original Breezy/SmartRecruiters repro shape), confirming `AttemptSubmit` clicks it, the form fields become visible to `resolveFillTarget`, and the fill+submit that follows reaches a confirmed success. `go build/vet/test ./...` all pass.


---

## 7. [FunnelEngine still lets Greenhouse job-search/listing pages into the pipeline](#7-funnelengine-still-lets-greenhouse-job-searchlisting-pages-into-the-pipeline)

**Table rationale cell (original):** Remainder of #5 after the Workday/Workable fix (Resolved 2026-07-21): the one Greenhouse false positive seen (`job-boards.greenhouse.io/remotecom/jobs/7778860003`) wasn't re-reproduced live this session, and a bare `/jobs/<id>` path is normally a real posting pattern, so a safe tightening rule wasn't obvious enough to fix opportunistically — needs its own live repro before writing a fix

### 7. FunnelEngine still lets Greenhouse job-search/listing pages into the pipeline
**Remainder of #5** (Workday/Workable portion resolved 2026-07-21, see Resolved section below). The one Greenhouse false positive seen, `https://job-boards.greenhouse.io/remotecom/jobs/7778860003`, wasn't re-reproduced live during this session's ~1-hour batch run, and unlike Workday/Workable its path shape (`/jobs/<id>`) is normally exactly what a *real* Greenhouse posting URL looks like, so there's no obvious safe tightening rule (path-based or domain-based) without risking false negatives on legitimate postings. Needs a fresh live repro and DOM inspection of the specific false-positive case before a fix is attempted, same approach that diagnosed #4 and the rest of #5.

**Resolved 2026-07-21, via #15's diagnostic:** loading the exact URL directly showed it was never a listing-page false positive — it's a real posting URL whose job expired, and Greenhouse's expired-posting redirect (`→ job-boards.greenhouse.io/remotecom?error=true`) landed it on the tenant's board index, which *looked* like a listing page in the original diagnosis. That's exactly why its `/jobs/<id>` shape resisted a URL-filter fix: the URL was legitimate. #15's `deadRedirectReason` now catches the `?error=` redirect at submit time and bails in seconds. No evidence of a genuine Greenhouse listing-page discovery false positive remains; if one ever appears, file it fresh.


---

## 6. [Ollama generation throughput collapses mid-request, likely context-shift thrashing](#6-ollama-generation-throughput-collapses-mid-request-likely-context-shift-thrashing)

**Table rationale cell (original):** Root cause turned out to be different from the leading hypothesis (see Resolved section): the hardcoded 10-minute client timeout, not context-shift, was killing honest-but-slow generations

### 6. Ollama generation throughput collapses mid-request, likely context-shift thrashing (Resolved 2026-07-21)
**Symptom:** across two consecutive 40-minute live runs late on 2026-07-20 (after #3 and #4's fixes, and after a clean `systemctl --user restart ollama`), almost every real `AttemptSubmit` attempt died at the document-generation stage with the same `context deadline exceeded` error #3 was supposed to have fixed — but this time each attempt ran a genuinely long time (33 minutes to fail) rather than hanging outright. A direct `journalctl` check mid-request showed `tg` (generation speed) at **1.58 tokens/sec**, `n_decoded = 944` — down from **8.8 tokens/sec** on a tiny throwaway prompt with no context pressure.

**Leading hypothesis going in (disproven):** that `--context-shift` (confirmed present in the live `llama-server` args: `-c 6144 ... --context-shift --keep 4`) was triggering expensive KV-cache recompute/eviction events as generation approached the 6144-token window.

**Actual root cause, confirmed by direct reproduction 2026-07-21:** sent a real ~4042-token prompt to the live `qwen3:30b-instruct` server directly (bypassing the app's client timeout) and watched `journalctl --user -u ollama -f` for the full run. Generation speed was **already** down at ~1.77 tok/s by `n_decoded = 100` (not "fast then collapsing" — just uniformly slow once a ~4000-token prompt is loaded) and declined smoothly to ~1.58-1.62 tok/s by `n_decoded ≈ 1200`, with **zero** `context-shift`/cache-eviction log lines anywhere in the run — the request never got close enough to the 6144 ceiling (peaked at ~5266 total tokens) to trigger a shift at all, yet was already this slow. This is simply attention-decode cost scaling with total context length on CPU-only inference — expected llama.cpp/CPU behavior, not a defect or a discrete "thrashing" event. The request completed cleanly with `HTTP 200` after **15m58s** for 1224 output tokens. Extrapolating that rate to a full `ProcessJobApplication` response (resume + cover letter + interview prep, likely 1500-2500 combined tokens) lands at **~25-35 minutes** — which matches the original incident's "33 minutes to fail" almost exactly. The real bug was `pkg/mcp/provider_ollama.go`'s `ollamaProvider.Timeout()` hardcoded to **10 minutes**: Go's `context.WithTimeout` was cancelling honest, still-progressing generations long before they could finish, and that cancellation surfaces as the same `context deadline exceeded` string bug #3 also produced (from a genuinely different cause — server-side hang on context overflow, already fixed), which is what made this look like a recurrence of #3 rather than a new, distinct problem.

**Ruled out:** thermal throttling (CPU never exceeded 69°C) and a Playwright/Chromium process leak (confirmed to be one browser's normal multi-process architecture).

**Fix:** made the Ollama client timeout configurable (`pkg/mcp/provider_ollama.go`): added `ollamaTimeoutFromEnv()` reading `OLLAMA_TIMEOUT_MINUTES` (falls back to a default on unset/non-numeric/non-positive values), defaulting to **45 minutes** (`defaultOllamaTimeoutMinutes`) — comfortable headroom over the ~25-35 minute measured/extrapolated real generation time. Documented the new var in `.env.example`. Added `pkg/mcp/provider_ollama_test.go` covering the default, a valid override, and invalid-value fallback (non-numeric, zero, negative). `go build/vet/test ./...` all pass.

**Not done (separate, larger scope):** splitting `ProcessJobApplication`'s single call into three smaller generations (resume/cover letter/interview prep) would reduce per-call latency and risk further, but re-sends the full prompt context three times, increasing total wall-clock on this CPU-bound hardware — a real trade-off, not a strict improvement, and out of scope for closing this specific bug. Also not done: a full live `cmd/agent` batch run to a genuine `APPLIED` status — that remains the Usability Gate's own broader unchecked item (blocked on bug #4's fill-step fix still being unverified in practice, and #5's FunnelEngine false positives), not something this bug's fix alone can close.


---

## 4. [AttemptSubmit form-fill logic never looked inside iframes](#4-attemptsubmit-form-fill-logic-never-looked-inside-iframes)

**Table rationale cell (original):** Real root cause of the "timeout" symptom: several ATS platforms (confirmed on SmartRecruiters) embed the application form in an `<iframe>`; every fill selector was scoped to the top-level page only. A first attempt just widened the timeouts (wrong diagnosis, verified live not to help — same failures recurred at the new, longer timeout). Re-diagnosed via a standalone script that loaded a failing page directly and found 0 `<input>` tags on the main page plus 1 iframe

### 4. AttemptSubmit form-fill logic never looked inside iframes (Resolved 2026-07-23)
**Symptom:** across live `cmd/agent` runs on 2026-07-20 (after the Playwright/container fixes below, and again post-#3 fix), `AttemptSubmit` reached a live job page and began tailoring successfully several times, but failed at the form-fill stage for different reasons each time: `form failed to render in time: playwright: timeout: Timeout 15000ms exceeded` (Lever, reproduced twice on two different `jobgether` postings in one 40-min window), `failed to fill first_name: playwright: timeout: Timeout 5000ms exceeded` (seen on both a Learner-Module-mapped SmartRecruiters job and an `applytojob.com` job), and one case correctly blocked by the security layer (`malicious prompt injection detected on career page`, indirect prompt injection score 0.85 — this one is a guardrail working as intended, not a bug).

**First (wrong) diagnosis:** the "ran out of time" shape of every failure looked like a pure timing issue (CPU-bound Chromium sharing a heavily-loaded host with a 30B local LLM), so timeouts were doubled/tripled as a first fix. **Verified live not to help** — a second 40-minute run with the longer timeouts produced the exact same failures, at the exact same field, now correctly waiting the *full* new timeout before failing. Waiting longer changing nothing is itself the signal that it was never a timing problem.

**Real root cause:** wrote a standalone script (`playwright-go`, same version as the app) to load a failing job page (`TechnologyNavigators` on SmartRecruiters) directly and inspect it. Found **zero `<input>` tags on the main page and one `<iframe>`** — the real application form is embedded in an iframe. `handleGreenhouse`, `handleLever`, `handleDynamic` (the Learner Module path), and `safeFill` all searched only `page.Locator(...)`/`page.WaitForSelector(...)` — the top-level document — which will never find a field that lives one frame down, no matter the timeout. Also found, separately, that some URLs entering the pipeline (`remotecom` on Greenhouse, `search` on Workable) are generic job-search/listing pages, not individual postings — those will never have an application form either; that's a distinct FunnelEngine URL-quality issue, not this bug.

**Fix applied 2026-07-20 (unverified):** added a `fillTarget` abstraction (`pageTarget`/`frameTarget`) in `pkg/submitter/browser.go` with a `resolveFillTarget(page)` resolver: checks the main page for form inputs first, falls back to scanning child frames for the first one that has any, and uses whichever is found for every fill/wait/DOM-extraction call downstream (`handleGreenhouse`, `handleLever`, `handleDynamic`, `safeFill`, the Learner Module's DOM capture, and the validation-error-retry path). Not yet confirmed to actually produce an `APPLIED` result — re-run and check `job_funnel` status before closing this out.

**Progress 2026-07-21 (live batch, still in progress):** one `failed to fill first_name` case observed so far (Breezy.hr, `jway-group`) turned out **not** to be this bug — `resolveFillTarget` correctly found no real form on the page or in any iframe and fell back to the page target as designed; the actual cause was a click-to-reveal "Apply" form, filed separately as bug #8. No SmartRecruiters-pattern (form genuinely embedded in an iframe already present in the DOM) case has been reached yet this run to positively verify the fix works end to end; also no counter-evidence that it doesn't. Still Pending verification.

**Structural blocker found 2026-07-23:** wrote a standalone probe (same `resolveFillTarget` logic reimplemented against a real headless browser) and ran it against 6 current `DISCOVERED` SmartRecruiters postings — the *only* platform this bug was ever reproduced on. Every single one now serves a DataDome CAPTCHA iframe (`geo.captcha-delivery.com`, 7 `<input>` fields) immediately after the "I'm interested"/"Apply" click, which bug #35's post-click `isCaptchaBlocked` check (confirmed correctly placed *before* `resolveFillTarget` runs, `pkg/submitter/browser.go:591`) now intercepts every time — so SmartRecruiters can no longer reach the fill stage at all, let alone this bug's iframe-fallback path. Also probed a broader live sample (Ashby, Pinpoint, Homerun, Jobvite) and found every one of those renders its real form directly on the main page — no iframe needed. **Conclusion: no platform in the current live traffic mix can organically exercise this bug's fix anymore**; waiting on a live batch to do it is likely to run indefinitely. Given that, added a direct unit test instead — `TestResolveFillTarget_FallsBackToIframeWithInputs` in `pkg/submitter/browser_test.go` reproduces the exact original repro shape (zero main-page inputs, one child frame with inputs) via mocks and confirms `resolveFillTarget` correctly returns a `frameTarget` pointed at that frame; two sibling tests confirm it prefers the main page when that already has inputs, and falls back to the page (not a frame) when nothing has inputs anywhere — covering the DataDome-iframe-with-inputs-but-not-a-form edge case specifically. `go build/vet/test ./...` all pass.

**Resolved 2026-07-23:** closed on the strength of the above rather than a live-traffic confirmation, per user go-ahead — live verification of this specific path is structurally unreachable (not merely unlucky), since the fix's own reasoning (frame-scan fallback) is directly, deterministically exercised by the unit test above using the exact original repro shape, and no code path between here and a real `APPLIED` remains unverified as a result of closing this one. If a genuine non-SmartRecruiters iframe-embedded-form case ever surfaces live, treat it as a bonus confirmation, not a requirement to reopen.


---
## 508. Discovery has no independent current-listings fallback when SerpApi quota is exhausted and Yahoo search fails

**Completed 2026-08-01.** Live daemon evidence showed the configured SerpApi account was out of searches and the Yahoo fallback was repeatedly failing with unexpected EOF. RemoteOK, Hacker News, and 120 known ATS boards supplied no new posting, leaving the eligible queue empty.

Added a Jobicy structured-feed source before the slower RemoteOK sweep. It uses the existing role-title relevance gate, rejects malformed/non-HTTP/junk records, persists `discovery_source = "jobicy"`, retains URL deduplication, and has a process-wide one-hour poll limit matching the provider's documented fair-use guidance. Tests cover successful ingestion, relevance filtering, duplicates, non-200 and malformed/unsuccessful responses, and the poll limit. The prior discovery tests now mock the added feed to stay offline and deterministic.

`go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` passed. Rebuilt the daemon and launched it through the persistent dashboard control path; its first live refresh admitted 18 relevant Jobicy postings and the next queue cycle loaded 15 jobs. The dashboard reports the daemon running.

---
## 515. Assisted Apply uploaded the saved reference note in place of the résumé

**Completed 2026-08-05.** Found during the five-application Assisted Apply acceptance trial, before any application was launched, while verifying the "documents are correct" precondition for the selected candidate.

`cmd/agent` uploads `master_resume.pdf` to every ATS. Both of its document branches return that path: the untailored branch (`cmd/agent/pipeline.go:542`) and the tailored branch (`:585`, `:587`). The per-job `resume.md` written under `applications/<company>/<url-digest>/` is a saved reference document, never the upload payload — a fact the constant's own comment at `cmd/agent/main.go:35` already stated.

Assisted Apply did not follow that invariant. `storage.GetAssistedDocument(conn, jobID, "resume")` resolved the résumé to `applications/<company>/<url-digest>/resume.md`, and `cmd/assist/main.go:526` passed that path into `submitter.FillAssistedMappedPage` → `handleDynamic` → `attachResume`, which reads the file and uploads it as `Name: "resume.pdf"` with no content or type validation.

When `profile.yaml` sets `use_master_cover_letter: true`, no per-job tailoring is generated and `cmd/agent/pipeline.go:530` writes a fixed 116-byte note into `resume.md`: "Master documents used for this application (use_master_cover_letter is enabled); no per-job tailoring was generated." On this machine every non-legacy queued application carried exactly that file. An assisted application would therefore have uploaded a 116-byte placeholder note, renamed `resume.pdf`, to a real employer in place of the user's 66 KB résumé.

The cover letter was never affected: `cmd/agent` hands the submitter `storage.CoverLetterPath(...)`, the same per-job `coverletter.txt` the assisted path resolved, so that document was already correct.

Two secondary effects resolved with the same change. The dashboard's document-preview endpoint shares `GetAssistedDocument`, so the operator reviewed one artifact while a different file would have been attached; preview and attachment are now the same file. And `assistedDocumentExists` drives the queue's `resume_ready` flag, which previously reported ready because a placeholder existed rather than because a résumé did.

Reproduced against a scratch database in a temporary working directory, independent of the live `applications.db`, before any code changed: the pre-fix resolution returned the 116-byte note as the upload payload while a genuine `master_resume.pdf` sat unused beside it.

**Fix.** Added the exported `storage.MasterResumePath`, and `GetAssistedDocument` now resolves `kind == "resume"` to it rather than to the application folder. `cmd/agent`'s `masterResumePath` was repointed at the same constant so the automatic and assisted paths cannot drift apart again. Cover-letter resolution is unchanged. `validateAssistedDocument`'s containment check does not apply to the master résumé, which deliberately lives at the repository root, so `validateMasterResume` was added to keep the symlink, regular-file, and non-empty guarantees for it.

Tests: `TestGetAssistedDocument_ResumeIsMasterResumeNotSavedArtifact` reconstructs the untailored save and fails if the resolved payload contains the placeholder text; `TestGetAssistedDocument_CoverLetterStaysPerJobArtifact` pins the cover letter to the per-job artifact so the fix is not over-applied.

`go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` pass. No frontend sources exist for this change to affect. The trial was halted at this defect and no application was submitted.

---
## 531. Every queue card whose timestamp was written by Go reports `last_updated` as year 0001

**Completed 2026-08-07.** Filed the same day, out of #530's scan-alignment test, which had to skip asserting `LastUpdated` because the field was reliably zero.

**Measured before and after, same database, same binary shape.** A dashboard built from `HEAD` served **406 of 506** queue rows as `0001-01-01T00:00:00Z`; one built from the fixed tree served **0 of 506**, all in 2026. (The row's original 424-of-524 figure was taken before #530 removed the 18 dead postings.)

**The cause is not the one the row assumed, and the difference matters.** The row said `parseAssistedTime` had no layout for `time.Time`'s default `String()` form and that this broke every Go-written timestamp. The first half is true; the second is not, and re-verification found why.

`modernc.org/sqlite` stores a bound `time.Time` as `t.String()` — `conn.formatTime` falls back to it when the DSN sets no `_time_format`, which this project's DSN does not. That is the literal text in 12046 of `job_funnel`'s 12980 rows. But the same driver reads it back correctly: `rows.go` parses any column whose *declared* type is `DATE`/`DATETIME`/`TIMESTAMP` through `parseTime`, whose first branch is exactly the `t.String()` shape, and hands `database/sql` a `time.Time` that a string scan renders as RFC3339. So a bare `SELECT last_updated` was never broken.

What broke was **this one projection**. `GetAssistedQueue` reads `COALESCE(jf.last_updated, jf.discovered_at, CURRENT_TIMESTAMP)`, and SQLite reports an empty decltype for an expression — the driver's own source names `COALESCE` in the comment on that branch. The conversion never fires, the raw `String()` text reaches Go, and with no matching layout `parseAssistedTime` returned the zero time.

Confirmed by execution rather than by reading: writing through a real writer (`UpdateFunnelStatusWithReasonAndScore`) and reading the column back bare yields RFC3339, while `CAST(last_updated AS TEXT)` — which erases the decltype the same way `COALESCE` does — yields `2026-08-07 15:35:07.977083232 +0000 UTC`. The `sqlite3` CLI confirms the stored bytes are the `String()` form in both cases.

**Two claims in the row were checked and are wrong.** The lease heartbeat was *not* affected: `aa.updated_at` is selected as a bare `DATETIME` column, so the driver converts it, and `TestAssistedQueueExposesContinueAndReviewAfterRefill` had been asserting `LiveBrowser == true` and passing all along. Nor is the defect "most of `job_funnel`" — the 12046 String()-form rows read fine everywhere except through an expression, and a tree-wide grep found `assisted.go:726` is the only expression-wrapped time column any Go code parses. (`scripts/check_latest.go` selects `MAX(applied_at)` but only prints it.)

**Fix.** The layout list moved to a package-level `assistedTimeLayouts` with `2006-01-02 15:04:05.999999999 -0700 MST` appended, and `parseAssistedTime` now returns `(time.Time, bool)`. Handling the shape here was preferred over the driver's `_texttotime` DSN option: the defect is in a function that claims to read stored timestamps, and one of the shapes it can be handed is this one.

**A parse miss is no longer silent.** This is why the bug survived nine days. `GetAssistedQueue` counts unreadable non-empty values across the scan and logs one line after it — counted, not per-row, because this backs a polled endpoint over hundreds of rows. An empty value stays silent: that is an absent timestamp, not a malformed one, and 152 rows legitimately have one.

**Tests.** `TestParseAssistedTime_ReadsEveryStoredTimestampShape` is a table over every shape observed in the live database on 2026-08-07 plus both failure paths, written as literal stored strings rather than formatted times — deriving the input from the layouts under test would assume away the drift being tested. `TestGetAssistedQueue_ServesRealTimestampsForRowsWrittenByGo` drives the whole path: a real writer, a premise check on the raw stored bytes, then the projection. #530's alignment test now asserts `LastUpdated` instead of documenting why it could not.

**ADR-003 gained decision 6**, recording the driver's encoding, the expression/decltype trap, and its corollary that SQLite's own `date()`/`datetime()` return NULL over these columns — the trap that produced a wrong "0 rows updated" reading during #524's verification.

---

## 530. A posting that has died still occupies a queue card, because the assisted queue never reads the funnel status it selects

**Completed 2026-08-07.** Filed 2026-08-06 during #524's backfill work, which asked each board directly about every queued posting and found 18 had been taken down. All 18 kept their card and stayed clickable.

**Cause.** `GetAssistedQueue` filtered on `aa.assisted_state != 'completed'` and nothing else. It *selected* `COALESCE(jf.status, '')` and scanned it into a local `currentStatus` that was never read again, and `serveAssistedQueue` — its only caller — added no filter. So `assisted_state`, which only records whether the *operator* finished with a row, was standing in for a question it cannot answer: whether the posting still exists.

**What re-evaluation added.** The confirm path already enforced the missing rule: `ConfirmAssistedSubmission` refuses when `status != original || !isAssistedEligibleStatus(status)`, so clicking Confirm on a dead card already failed safely with "refusing to overwrite newer job status". There was never a corruption risk — only wasted operator effort. That settled the design question the row had left open: **reuse `eligibleAssistedStatuses` rather than invent a list of terminal statuses.** An allowlist of "still workable" cannot fall behind as new failure statuses appear, whereas a denylist silently would.

Checked before relying on it: the only `job_funnel.status` write anywhere on the assisted path is `ConfirmAssistedSubmission`'s `APPLIED` update, made in the same transaction that sets `assisted_state = 'completed'`. So an in-flight row still holds its original eligible status, and the filter cannot hide work genuinely in progress.

**Fix.** `eligibleAssistedStatusList()` renders the existing set as a sorted SQL `IN` list, and `GetAssistedQueue` gained `AND jf.status IN (...)` bound through parameters. The queue and the confirm path now share one definition of "still workable". The dead `jf.status` select/scan was removed — reading it is now the `WHERE` condition, which is what reading it should always have meant — and with it a second dead read found in the same scan: `jf.status_reason` was selected into `job.Interruption` and then immediately overwritten by `aa.interruption_reason`. Removing it changed no value, only wasted work.

**Deliberately not done: marking the rows `completed`.** In this schema `assisted_state = 'completed'` is written in exactly one place, together with `confirmation_provenance = 'manual_user_confirmation'`, and means *an application was submitted and the operator confirmed it*. Using it for a dead posting would fabricate an application record — the same class of defect as #518 and #521. The rows keep `waiting_human` and simply stop being offered as work; the funnel already records them as `INVALID_URL`/`expired`, so nothing is lost, and the dashboard's funnel metrics still count them.

**Mutation-checked.** Reverting the filter makes the exclusion test fail with exactly the defect (`queue length after the posting died = 2, want 1`). Swapping two adjacent `SELECT` columns makes the alignment test fail (`OriginalStatus = "solve_captcha"`), confirming it really guards the scan realignment. One honest negative: re-introducing the duplicate `jf.status_reason` scan does **not** fail any test, and cannot — the later scan always overwrote it, so the removal changed no observable value. The test comment was corrected to stop claiming otherwise.

**Verified live against the real `applications.db`.** A dashboard built from the fixed tree on `127.0.0.1:8099` served **506** rows where the pre-fix production build on `:8080` served **524** — the difference being exactly the 18 dead postings, with zero leaking through, matching the database's own count of 506 eligible rows. The 18 records survive untouched: still `waiting_human`, still no `confirmation_provenance`. Production was then restarted on the fixed build and confirmed serving 506 rows with 67 locations.

**Found along the way:** bug #531 — `parseAssistedTime` has no layout for `time.Time`'s default string form, so 424 of 524 live queue rows report `last_updated` as `0001-01-01T00:00:00Z`.

---

## 525. Assisted Apply attached a .txt extraction where the automatic path uploads the master cover letter

**Completed 2026-08-06.** Filed 2026-08-05 during the Assisted Apply acceptance trial, alongside #515 and #517, as the least severe of the three document-resolution divergences.

`profile.yaml` sets `use_master_cover_letter: true`, so `cmd/agent` uploads the master cover letter file itself: `generateDocsFunc` resolved `Profile.MasterCoverLetterPath` (falling back to `master_cover_letter.txt`) and returned that path to the submitter. `storage.GetAssistedDocument` resolved `kind == "cover_letter"` unconditionally to the per-job `applications/<company>/<url-digest>/coverletter.txt`, which `SaveApplication` writes as the *extracted text* of that same file.

Both paths hand their resolved path to the same fill handlers — `cmd/assist` passes it to `submitter.FillAssistedMappedPage`, which calls the same `dedicatedATSHandler`/`handleDynamic` functions `cmd/agent` uses — so the content was right and only the file differed. On a cover-letter textarea that is invisible. On a file-upload field the employer received an unformatted `.txt` in place of the designed PDF.

**Fix.** Added `config.DefaultMasterCoverLetterPath` and `(*config.Profile).ResolvedMasterCoverLetterPath()`, which answers "which static letter does this profile actually send" once: `""` when `use_master_cover_letter` is off or `send_cover_letter` is false, the configured path when set, the default otherwise. `cmd/agent/pipeline.go`'s open-coded copy of that branch was deleted and repointed at the method, and `cmd/agent`'s own `defaultMasterCoverLetterPath` constant was removed so only one literal remains — the same drift-proofing #515 applied to the résumé.

`GetAssistedDocument` now resolves the cover letter through the same method and validates the result. `validateMasterResume` was generalised to `validateMasterDocument` (symlink, regular-file and non-empty guarantees, without `validateAssistedDocument`'s `applications/` containment check, since master documents live at the repository root) and now serves both documents. A configured master letter that fails validation returns an error rather than falling back to the `.txt` — that fallback is precisely the defect, so it would convert a visible failure into a silent wrong-format upload; the error degrades safely, since the queue then reports the document as not ready and `cmd/assist` preserves the application for manual completion.

`GetAssistedDocument` was split over an unexported `assistedDocument` taking the resolved path, so `GetAssistedQueue` resolves the profile **once** per queue rather than per row. That queue holds 524 rows on the live database and probes documents twice per row; a per-row resolution would have parsed `profile.yaml` over a thousand times per dashboard poll.

**Mutation-checked before being trusted:** with the new cover-letter branch deleted, both new tests fail, and they fail *with the defect itself* — `cover letter path = "applications/Master_CL_Co/…/coverletter.txt", want master cover letter path "Omni_CoverLetter.pdf"`.

**Verified live, before and after, against the same job on the real `applications.db`.** A dashboard built from the current tree ran on `127.0.0.1:8099`; the production instance on `:8080`, still running the pre-fix binary, provided the baseline and was never touched beyond a read. For job `297438`, the pre-fix build served `Content-Disposition: coverletter.txt`, `Content-Type: text/plain`, 2151 bytes; the fixed build served `Omni_CoverLetter.pdf`, `application/pdf`, 11055 bytes, with a SHA-256 identical to the master file on disk. All 524 queued rows reported `resume_ready` and `cover_letter_ready` true.

Tests: `TestGetAssistedDocument_MasterCoverLetterServedWhenEnabled` writes distinguishable content into the per-job artifact and asserts the served bytes are the master letter's; `TestGetAssistedDocument_InvalidMasterCoverLetterReturnsError` pins the fail-closed behaviour; `TestAssistedDocumentExists_CoverLetterReadinessFollowsTheMasterLetter` covers the queue's readiness signal; `TestResolvedMasterCoverLetterPath` covers all five cases of the new method. `TestGetAssistedDocument_CoverLetterStaysPerJobArtifact`, added by #515 to stop that fix being over-applied, still passes unchanged and now pins the no-master-letter case specifically.

**Two findings filed from this work.** #527: `setupTestDB` uses a `:memory:` database, which is private per connection, so any query issued from inside an open `rows` iteration takes a second pooled connection and fails with `no such table` — `GetAssistedQueue`'s `resume_ready`/`cover_letter_ready` are therefore always false under test and cannot be covered end to end. Confirmed to be a harness artifact only, by the live 524-row check above. #528: with `send_cover_letter: false` the assisted path would attach the per-job `coverletter.txt`, whose contents in that configuration are the sentence "Cover letters are disabled…" — the last member of this divergence family, left out of scope here because fixing it requires representing "no cover letter" distinctly from "the cover letter failed to load".

`go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all pass. Implementation delegated to Antigravity CLI / `gemini-3.1-pro-high`; reviewed, corrected and verified in this session.

---

## 399 Newly discovered jobs bypass Bayesian smoothing due to queue architecture flaw

**Legacy entry, moved out of the live backlog by the 2026-08-06 groom pass.** It carried no table row — only this inline narrative — so it was invisible to the backlog's own structure while still counting against the file's size.

**Symptom:** In daemon mode, `candidates` channel interleaves the ranked backlog with newly discovered jobs pushed directly by `discoverJobs`. 
**Impact:** Newly discovered jobs bypass `RankJobs` and get processed immediately at the tail of the channel, completely circumventing the Bayesian source-health smoothing and `FitSimilarity` ranking logic. Additionally, `cycleLimit` caused the `runAgentCycle` producer to aggressively discard thousands of ranked jobs from the channel, wasting CPU and forcing a full DB reload on every cycle.
**Fix:** Decoupled discovery from processing. Passed a `nil` channel to `discoverJobs` so it only populates the database as `DISCOVERED` without injecting into the active queue. Limited the backlog producer loop to `cycleLimit` to prevent channel thrashing and dropping backlog items.
**Status:** Done.

---

## 400 Bayesian smoothing aggregated AvgInferenceMs but never penalized slow sources

**Legacy entry, moved out of the live backlog by the 2026-08-06 groom pass.** It carried no table row — only this inline narrative — so it was invisible to the backlog's own structure while still counting against the file's size.

**Symptom:** Source health tracking accurately recorded the average inference MS per source, but the actual queue ranking algorithm (`ComputeSourceScores`) completely ignored this metric when calculating the raw score.
**Impact:** The app could not get "faster" over time because it never learned to penalize excessively slow ATS endpoints (e.g., endpoints requiring huge DOM parsing times).
**Fix:** Added an explicit `speedPenalty` to the Bayesian `PenaltyFactor` in `pkg/storage/ranking.go`. If a source consistently takes over 20,000ms (20s) to process, its rank score is penalized up to 40%, naturally surfacing faster applications to the front of the queue.
**Status:** Done.

---

## 401 Learner Module destroys form-mapping cache on transient or optional field errors

**Legacy entry, moved out of the live backlog by the 2026-08-06 groom pass.** It carried no table row — only this inline narrative — so it was invisible to the backlog's own structure while still counting against the file's size.

**Symptom:** Forms that do not require an optional standard field (e.g., `phone`) caused `ErrEmptySelector` in `safeFillWithLabelFallback`, which bubbled up to `AttemptSubmit`. 
**Impact:** `AttemptSubmit` incorrectly treated this as a stale mapping and instantly wiped the cache via `DeleteFormMapping(domain)`. This caused the agent to repeatedly trigger expensive LLM mapping generations on every visit to the same ATS, completely defeating the "learning" and caching mechanism. Furthermore, transient network timeouts also triggered instant cache wipes.
**Fix:** Modified `handleDynamic` to gracefully tolerate `ErrEmptySelector` for standard fields `first_name`, `last_name`, `email`, and `phone`. Logged Bug B regarding transient timeouts for future improvement.
**Status:** Done (Bug A). Bug B (transient timeouts) remains open as a known limitation of Playwright's timeout overlapping with stale selector errors.

---

## 536. Tracker email classifier treats promotional emails as `INTERVIEW_REQUESTED`

**Found 2026-08-12 from live daemon logs.** The classifier in `pkg/tracker/imap.go` emitted `INTERVIEW_REQUESTED` whenever the combined lowercased subject/body contained "interview", "next steps", or "availability". Live logs showed obvious marketing/entertainment emails being logged as interviews: `email.dunhamssports.com` weekly ads ("deals are here", "super sale"), `google.com` Pixel pre-order announcements ("pre-order", "availability"), `mail.beehiiv.com` Pixel promo copy, `gofobo.com` movie first-look notices ("first look:", "tickets"), and generic retail "availability" language. These inflated `unmatched_outcomes` and made it harder to spot genuine interviews among unmatched outcomes.

**Fix.** Hardened `classifyEmail` in `pkg/tracker/imap.go` to require recruiting context before emitting `INTERVIEW_REQUESTED`: an email must contain at least one interview signal word/phrase ("interview", "phone screen", "next steps", "availability", "available") AND at least one recruiting context word/phrase ("interview", "phone screen", "job", "role", "position", "candidate", "hiring", "recruiter", "recruiting", "application", "opportunity", "schedule", plus common job-function stems such as "engineer", "developer", "manager", "analyst", and "architect"). Expanded `notJobPhrases` with retail/entertainment markers observed live ("weekly ad", "pre-order", "shop now", "order now", "free shipping", "coupon code", "sale ends", "super sale", "deals are here", "limited time offer", "your order", "track your package", "first look:", "now streaming", "in theaters", "watch now", "tickets") so obvious commerce/entertainment copy short-circuits to "no classification" even if it accidentally hits a context word. Rejection classification stayed keyword-based on negative outcome language, which had not shown this particular false-positive pattern.

**Tests.** Extended `TestClassifyEmail` in `pkg/tracker/imap_test.go` with regression cases for the live promotional subjects and a few genuine recruiting emails that must still classify (interview availability with an engineering role, phone-screen availability, real rejection). All pre-existing `pkg/tracker` tests continued to pass.

**Verification.** `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all passed clean.

**Files changed.** `pkg/tracker/imap.go`, `pkg/tracker/imap_test.go`.

---

## 543. The assisted browser's stderr is echoed verbatim into the dashboard log, and Playwright's retry diagnostics include element HTML

**Found 2026-08-13** by improvements.md #537's live run, during the PII scan that run performs on its own logs. **Closed 2026-08-13.**

**The invariant this closes.** *Assisted Apply child-process diagnostics may never cause transient operator answers or DOM values to be persisted in `dashboard.log`.* More generally: arbitrary child stderr is never blindly persisted. Recorded as ADR-006.

**The filed description was accurate but incomplete.** It named one leak — the dashboard's verbatim echo — and proposed filtering to Career Agent's own timestamped lines. Reading the code first found three more sites, two of which defeat that filter from inside:

* `pkg/submitter/assisted_fill.go:193` rendered the raw error from committing **the operator's answer** with `%v`. `safeFillWithLabelFallback` wraps Playwright's error with `%w`, so the driver's full diagnostic reached the log on a line that *does* carry a Career Agent timestamp. (`:133`, the vault-answer path, is the same shape.)
* `cmd/assist/main.go:681`/`:729` did the same for the refill and answer-application failures.
* `cmd/assist/main.go:358` set the **direct Chromium process's** `Stderr = os.Stderr`, so an entire third-party browser's output was already inside the stream the dashboard persisted.

**Reproduced, not assumed.** A local synthetic form (`http://127.0.0.1:<port>`, one text input, no employer contacted, no `pii.yaml`, no production database) was filled with the synthetic canary `CA_TEST_SECRET_543_d7f912a4` and then locked, so the next commit retried and timed out. Real Playwright produced:

```
  - locator resolved to <input readonly id="salary" ... value="<the canary>"/>
```

Four error shapes were captured — retry timeout, strict-mode violation, pointer interception, detached locator. Three carried the canary; **all four put it on an unprefixed continuation line, never on line 0.**

**Was timestamp filtering alone sufficient? Against the driver, yes; against this project, no.** Every captured shape would have been caught by the prefix rule. It is still insufficient, because the first line of an application-owned record is written by Career Agent, and Career Agent was embedding the raw error in it. The producer half is therefore not optional.

**Proof of the leak, end to end.** `TestAssistedBrowserLogging_RealPlaywrightDiagnosticNeverReachesTheLogFile` (`cmd/dashboard`) re-executes the test binary as a stand-in for `cmd/assist`, runs a real Chromium against the localhost form, provokes the real retry failure, and pipes that child's stderr through the shipped reader into a real log file. With the filter disabled, **the canary was written to the persisted log file.** With it in place, it is not.

**Fix — an application-owned boundary at both ends** (`pkg/security/logsafe.go`):

* **Producer.** `security.BrowserFailureReason` maps any automation error to one code from a closed vocabulary (`browser_timeout`, `ambiguous_target`, `element_not_interactable`, `target_missing`, `navigation_failed`, `browser_closed`, `browser_driver_unavailable`, `unclassified`). It matches on the error's first line, but *returns only constants declared in that file*, so its output cannot contain page text however the input is worded. Same shape as `NetworkRejectionReason`, which #523 established. Applied to every assisted-path call site that could hold a Playwright error.
* **Consumer.** `security.SanitizeChildLogLine` admits a line only if it carries the standard logger's record prefix, then strips markup from the remainder and bounds it to 1000 bytes. `cmd/dashboard/assist_log.go` logs what qualifies and counts what does not. `cmd/assist` now discards Chromium's own streams instead of inheriting them.

**Blocklisting Playwright phrasing was rejected** — it encodes one dependency's current wording, fails silently on the first reword, and no test would notice. Both halves decide by structure instead.

**Readiness preserved by separating the concerns.** `"Assisted application is open."` is matched against the **raw in-memory line**, before filtering; only persistence goes through the filter. No second file holds the raw stream — the change reduces persistence boundaries rather than moving them.

**Also fixed while here:** the reader was a `bufio.Scanner`, which fails the whole stream on a token over 64 KB. Since an employer's page controls how long a diagnostic is, that was a path to a stalled read and a child blocked on a full pipe. `readBoundedLine` truncates and keeps reading.

**Tests.** 7 in `pkg/security` (closed-vocabulary property, record survival, sentinel survival, continuation-line rejection, markup redaction, length bounding, CRLF/near-miss prefixes); 9 in `cmd/dashboard` (the same boundary through the real reader, plus a 4 MB line asserting no stall and a missing-final-newline case); 1 real-Playwright producer regression in `pkg/submitter` and 1 real-Playwright cross-process regression in `cmd/dashboard`, both gated on `CAREER_AGENT_PLAYWRIGHT_INTEGRATION=1` per the existing convention. The canary appears only as an assertion target; no real answer was used anywhere.

**Adjacent persistence paths checked** (bounded, per the task's scope): `pending_answers` is written only by the answer API and consumed by `TakePendingAnswers`, which reads and deletes in one transaction and returns an empty map if the commit fails; `DiscardPendingAnswers` clears it when a browser closes; `ConfirmAssistedSubmission` deletes it in the confirmation transaction. `answer_text` appears in exactly two tables — that transient one and the vault, which holds only explicitly approved answers. No log statement anywhere prints an answer value; the assisted log lines report counts. `Store.Save`'s refusal errors are fixed strings and do not quote the answer. The assisted browser was the **only** child stream the dashboard persisted (the agent daemon's streams go to `/dev/null`). No other plaintext path was found; nothing new was filed.

**A note for whoever reads the pre-fix history.** A `role="combobox"` control — which is what #537's live run happened to hit — routes through `commitComboboxSelection`, which replaces the driver's error with a sentinel and never leaked. The default control path is the one that did. The producer regression test deliberately uses a plain text input for that reason.

**Verification.** `gofmt -l ./cmd ./pkg ./internal` empty; `go build ./...`, `go vet ./...`, `go test ./...` all pass; both Playwright integration tests pass and both were confirmed to fail against the pre-fix tree.

**Files changed.** `pkg/security/logsafe.go` (new), `pkg/security/logsafe_test.go` (new), `cmd/dashboard/assist_log.go` (new), `cmd/dashboard/assist_log_test.go` (new), `pkg/submitter/assisted_log_privacy_test.go` (new), `cmd/dashboard/main.go`, `cmd/assist/main.go`, `pkg/submitter/assisted_fill.go`, `pkg/submitter/browser.go`, `docs/adrs/ADR-006-Log-Confidentiality-Boundary.md` (new).

---
