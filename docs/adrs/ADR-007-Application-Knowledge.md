# ADR-007: Application Knowledge and Preflight

## Status

**Accepted — implemented 2026-08-13** in `pkg/knowledge`, `pkg/storage/knowledge.go`,
`pkg/submitter/preflight.go`, `cmd/preflight`, `cmd/dashboard/knowledge_api.go`,
`cmd/dashboard/profile_api.go` and the dashboard UI. Closes `bugs.md` #544 and
`improvements.md` #538–#542.

**Amended 2026-08-14** with decision 7's form-inventory model (`bugs.md` #547) and
**2026-08-15** with decision 8, which separates knowing a form from filling one
(`bugs.md` #548) in `pkg/storage/questions.go`, `cmd/preflight`, `cmd/assist` and
`CompletedSummary.tsx`.

**Amended 2026-08-17** with decision 9, which extends decision 8's fill provenance to
Automatic Apply and adds `fill_source` (`bugs.md` #551) in `pkg/storage/questions.go`,
`pkg/submitter/browser.go` and `CompletedSummary.tsx`.

**Amended 2026-08-19** with decision 10, which adds the `education` curated pattern and
`config.PII.EducationSummary()` (`pkg/answers/patterns.go`, `pkg/config/pii.go`,
`pkg/answers/resolve.go`).

**Amended 2026-08-19** with decision 11, which adds first-class intentional absence
(`improvements.md` #545) in `pkg/answers/model.go`, `pkg/answers/store.go`,
`pkg/answers/resolve.go`, `pkg/knowledge/knowledge.go`, and
`cmd/dashboard/knowledge_api.go`.

## Context

The Approved Answer Vault (ADR-adjacent; `pkg/answers`, `improvements.md` #497) already solved the
hard problem. It resolves an employer's question to an approved answer deterministically — operator
alias, then approved answer, then a curated pattern over `pii.yaml` facts, then unresolved — with no
model call in the path, a `Resolved`/`AutoFill` split that lets a sensitive question be *suggested*
without being *typed*, and a refusal enforced inside `Store.Save` so no caller can route around it.

But it only ever ran **inside one live browser session, for one job, one time**. On the live
database on 2026-08-13, after 372 assisted applications had been processed:

| Table | Rows |
|---|---|
| `approved_answers` | 0 |
| `answer_aliases` | 0 |
| `application_questions` | 0 |

`SeedFromPII` existed, was tested, and was never called from production code. Questions were
recorded only while `cmd/assist` held a browser open, so the inventory was empty. No query anywhere
grouped questions by anything but `job_id`, and there was no index on any question key. Approving an
answer re-resolved nothing — the other queued applications found out when they were next opened, one
at a time.

The operator therefore answered the same question on every application, and Career Agent's knowledge
never compounded. The success metric this project cares about is **human seconds per high-quality
submitted application**, and that number was flat no matter how many applications had been done.

## Decisions

### 1. The question inventory is advisory; the live browser is authoritative

`application_questions` gains `auto_fillable`, recording what the vault concluded the last time the
row was re-evaluated. Nothing in this feature changes a question's `status`, touches the assisted
state machine, or writes to an application an assisted browser currently holds — `QueuedQuestions`
excludes leased jobs in SQL, so the rule is enforced rather than remembered.

When `cmd/assist` opens a page it re-resolves every control from the vault, and its answer wins. What
this feature stores is a cached opinion, so the dashboard can say "nine of these are now resolved"
without opening nine browsers to find out.

This is why the whole thing integrates cheaply: `cmd/assist` is unchanged. An answer approved in the
inbox reaches the next application because the fill path already looks it up, not because anything
was pushed to it.

### 2. Deduplication keys on the question's text, not its DOM key — and stops before semantics

`application_questions.question_key` is the DOM control's key (`name`, then `id`, then a hash of the
label). It is unique to one rendering of one form, which is exactly right for deciding *which control
an answer is typed into* and useless for deciding *whether two employers asked the same question*.
A separate `canonical_key`, derived from the prompt via `answers.QuestionKey`, answers the second.
It is computed inside `ReplaceApplicationQuestions` rather than taken from a caller, for the reason
`Store.Save` enforces its own rule: a caller that forgot would not produce an error, it would produce
a question that silently belongs to no group.

Grouping itself is layered, most specific first:

| Layer | Group key | Collapses |
|---|---|---|
| 1 | (inside the vault) | an operator-approved alias for that exact phrasing |
| 2 | `pattern:<id>` | every phrasing of one of the 20 curated families |
| 3a | `experience:<skill>` | every phrasing of one skill's duration question |
| 3b | `q:<QuestionKey>` | presentational differences only |
| 4 | **not built** | semantic similarity |

Layer 2 uses `answers.MatchedPatternID` — the resolver's own table, exported rather than
reimplemented. If grouping had its own idea of which questions are the same family, the operator
could answer a group and find the resolver disagreed about half of it; and the `Deny` lists that keep
sponsorship apart from work authorization would apply to only one of them.

**There is no layer 4, deliberately.** No embedding, no similarity threshold, no model call enters
this path. The cost asymmetry is the same one `normalize.go` already reasons about, only sharper
here: a missed collision costs one operator approval that then becomes an alias forever, while a
false collision puts a wrong answer — potentially a wrong attestation — on a real application. The
`Group` type leaves room for a candidate list if that is ever revisited; the storage and the UI do
not assume its absence.

### 3. Aliasing a declaration is a separate decision from approving it

When the operator answers a group, the other phrasings it collapsed are bound as aliases, so the
next occurrence is a layer-1 lookup rather than another interruption.

For a **sensitive** group this requires an explicit third acknowledgement
(`ErrSensitiveAliasNeedsConfirmation`). Approving an attestation answers one question the operator
read; aliasing it asserts that several other wordings ask that same question. The vocabulary these
questions are drawn from is small and treacherous — "authorized to work" and "require sponsorship"
share most of their tokens and mean opposite things, which is why the pattern table needed a `Deny`
list in the first place. Career Agent may propose the grouping and shows every wording; only the
operator may accept it. Without the confirmation the answer is still stored, and the unconfirmed
wording simply stays unresolved, which is the safe direction.

`AddAliases` also refuses outright on an answer whose reuse is withheld. Such an answer is never
typed, so recognising more ways of asking for it would change nothing — and reporting success would
leave the operator believing the vault knows something it does not.

### 4. Reuse policy is derived, never stored

The five policies (`safe_auto_fill`, `approved_reusable`, `suggest_ask`, `human_review`,
`generate_per_job`) are already facts about the `(Sensitivity, ReuseAllowed, Source)` triple the vault
records. `knowledge.Policy` names them; no column, no enum, no migration. A stored policy would be a
second place for the same truth to live and a second place for it to go stale — and the dashboard's
management view and its inbox both call the same function, so they cannot come to describe one
answer differently.

Note in particular that `suggest_ask` is not a new capability. It is what withholding reuse has
always meant, finally given a name on a screen.

### 5. Preflight is discovery-only, and cannot reach a submit control

`InspectApplication` reuses `AttemptSubmit`'s prologue verbatim — same guarded session, same
dead-posting checks, same bot-protection check, same ADR-002 quarantine — then reads the controls and
closes the page. It follows at most the employer's own Apply affordance, which is how their form is
reached at all.

The boundary is pinned **at the source level**, not behaviourally: `preflight_test.go` parses
`preflight.go` and asserts it contains no call to `findSubmitControl`, to any fill path, or to the
submit selector table. A behavioural test could not prove this — it would have to enumerate every
real employer form to show none was ever submitted. What can be proved is that no call path exists.
This is the same technique, and the same reasoning, as
`TestControlInventory_NeverEnumeratesAControlThatCouldSubmit`.

Unavailability is a closed vocabulary in the shape ADR-006 established: `captcha_blocked`,
`auth_required`, `posting_dead`, `quarantined`, `navigation_failed`, `no_form_found`,
`browser_rejected`, `unclassified`. Never an error's text, which can quote the page. Two refusals
happen before any page is loaded — an ATS whose form cannot be read at all, and one with no
pre-auth form at all (bug #18's Workday case) — and a test passes a nil browser to prove that
ordering.

**Amended 2026-08-14 (bugs.md #545).** The first of those refusals originally asked
`storage.AssistedBrowserRejectionReason`: *does this ATS accept a submission from the assisted
browser?* That is the wrong question for a read. Preflight fills nothing and cannot submit — the
paragraph above is the proof — so an employer refusing an automated submission has said nothing
about whether their form can be inspected, and Lever answers no to the first question and yes to the
second: its `/apply` form is served to an anonymous request.

The conflation inverted the feature's value. An ATS Career Agent may not submit to is precisely the
one the operator finishes by hand, and therefore precisely the one whose questions are worth having
in advance — yet those were the only applications never inspected. On the live queue that was 20 of
26. The registry now carries `blocksPreflight` alongside the submit rejection, and preflight asks
`storage.PreflightRefusalReason`: one evidence-backed registry, two questions, each needing its own
observation. `browser_rejected` keeps its meaning for an ATS genuinely unreadable without the
operator signed in.

Nothing about the submit boundary moved. Lever still never opens an assisted browser and still
routes through `actionForOperatorBrowser`.

A sign-in wall and an empty application form both look like "nothing to answer", so a password
control or an auth-wall phrase reports `auth_required` rather than `no_form_found`. Telling the
operator to expect an easy application and handing them a login page is the specific failure that
distinction prevents.

`cmd/preflight` runs as a dashboard child so its stderr passes through
`security.SanitizeChildLogLine` — ADR-006's protection, inherited rather than reimplemented. It logs
counts and reason codes, never a label and never a value. The batch cap is about how much traffic one
operator action may send to employers' servers, not about capability, and preflight is refused
outright while a visible assisted browser is open.

**Amended 2026-08-14 (bugs.md #547): preparation state is carried explicitly, and preparation has a
second entry point.**

Everything above was about *recording* an inspection. Nothing consumed the recording on the path
where it mattered most. The Copy Application Packet sourced the employer's questions from
`GetPendingQuestions` alone and rendered that section only when the list was non-empty, so a form
nobody had ever opened and a form read and found quiet produced byte-identical output. On the
operator's own Lever application that silence meant the first while looking like the second.

`storage.DeriveFormInventory` now answers the question the packet was implicitly guessing at, from
the table built for it — `application_preflight`, whose own schema comment already said *"we looked
and found nothing" and "we could not look" are different facts, and only this table can tell them
apart*. Four states travel on the wire (`not_prepared`, `preparing`, `ready`, `failed`) alongside the
counts, and the UI is forbidden from deriving any of them from `len(questions)`.

Three properties of the derivation are load-bearing:

* **A verdict is the strongest evidence, but not the only evidence.** A live assisted session records
  questions without writing a verdict (`assisted.go`'s `ReplaceApplicationQuestions` call), so "no
  preflight row" cannot mean "never inspected" — that reading would print *nobody has read this form*
  directly above questions read off that very form. Question rows are accepted as proof of a reading,
  labelled with their source.
* **`assisted_fill_summary` is not consulted at all.** Its `recorded_at` is stamped by preparation as
  well as by filling (bugs.md #548), so it already carries two meanings; making it carry a third
  would deepen that defect rather than route around it. *(#548 has since been fixed — see decision 8
  — and the exclusion still stands, now for a stronger reason: what a fill did to a form is a
  different fact from what Career Agent knows about it.)*
* **`preparing` is process state, not durable state.** Only the dashboard that spawned a run knows it
  is still going, and it tracks which job identifiers are in flight rather than only how many — a
  run-wide busy flag would have claimed every open packet was being inspected. A dashboard restarted
  mid-run falls back to what the database holds, which is the honest answer.

The packet offers `Prepare this application` in the incomplete and failed states. It posts to
`/api/knowledge/preflight` — the same endpoint, the same child process, the same one-run-at-a-time
guard and the same refusal while an assisted browser is open — rather than adding a second way to
read a form. Preparation stays **explicit** rather than firing on packet open: the batch cap exists
to bound how much traffic one operator action sends to employers' servers, and a panel that launches
Chromium because it was expanded would spend that budget without being asked. Everything in decision
5 above still holds; this amendment adds a caller, not a capability.

### 6. The profile edits `pii.yaml` rather than copying it

`pii.yaml` is the source of truth for the facts the curated patterns read. A dashboard-side copy
would be a second answer to "what is your phone number?" that could silently disagree with the one
the fill path uses, so `/api/knowledge/profile` reads and writes that file directly.

This adds the only HTTP write path to `pii.yaml` in the repository, which is a real expansion of what
a local process can do, and it was taken deliberately rather than by omission. The alternative was a
Knowledge Center that can say "eleven queued applications want a phone number you have not
configured" and then offer nothing but "go and edit a YAML file". Constraints:

* Only the fields `config.PII` declares are accepted (`DisallowUnknownFields`), so a request cannot
  introduce arbitrary YAML keys.
* Read-modify-write, so a client that knows about four fields cannot erase the twenty it does not.
* A timestamped `0600` backup before every overwrite.
* Temporary file in the same directory at `security.PrivateFileMode`, `Sync`, then `Rename`, so a
  failure mid-write cannot leave a truncated `pii.yaml`. Same directory because rename is only atomic
  within a filesystem.
* `RepairPrivatePaths` re-asserted afterwards.
* No value logged or echoed, on any path. A test proves it.
* **Backups are bounded and gitignored.** Each one holds the same personal data
  as `pii.yaml` itself, so five are kept and older ones deleted; and `.gitignore`
  matches `pii.yaml.*` as well as `pii.yaml`, because the bare name does not
  cover a timestamped backup and an untracked file full of somebody's address
  sitting in the repository root is one `git add -A` from being published. Found
  while reviewing this branch's own diff, not by a test.

Two limits are documented rather than papered over: **YAML comments do not survive a save** (hence
the backup), and `requireSameOrigin` is not authentication — it admits requests carrying none of
`Sec-Fetch-Site`, `Origin` or `Referer`, exactly as it does for every other mutating dashboard route.
What is new is that the set of things a local process can already do now includes rewriting
`pii.yaml`.

### 7. Readiness is demand-driven

Every figure is measured against the applications actually in the queue. There is deliberately no
"profile 93% complete": a percentage of an imagined universe of questions tells the operator nothing
about whether they can apply to the jobs they have, and it goes *up* when they answer things nobody
asked. `Readiness.KnownPercent` returns 0 rather than 100 for an empty queue, because "nothing has
been looked at" and "everything is known" must not render the same way to someone about to start a
session.

`AnswersNeeded` and `FieldsUnlockable` count only groups that can honestly be answered once for
everyone. A declaration the operator must read on every application unlocks nothing in bulk, and
counting it would overstate what the number buys them.

### 8. Knowing a form and filling a form are separate records, and preparation cannot speak about filling

*Added 2026-08-15, closing bugs.md #548.*

The invariant, in two lines, because everything below is a consequence of it:

```
FormInventory = what Career Agent knows about the form.
FillSummary   = what Career Agent actually did to the form.
```

Decision 7's derivation already refused to read the fill summary. The defect was on the other side of
the boundary: **preparation was writing it.** `cmd/preflight` passed a zero-value summary through the
same storage function a real fill used, and that function stamps `recorded_at` unconditionally. The
dashboard read the row's existence as proof a fill had run, so a prepared application was described
as one Career Agent had tried and failed on — *"Nothing could be filled automatically on this
application"* — while the vault held answers for 8 of that form's 10 questions and the operator
filled all 21 controls by hand.

Measured on the live database at fix time: **all 11 summary rows paired 1:1 with an `inspected`
preflight verdict**, in batch clusters seconds apart. Every row was a preparation record. No fill had
ever run on this installation, and nothing in the schema could say so.

Three decisions follow.

**The evidence is added, not inferred.** `assisted_fill_summary` gains a nullable `fill_attempted_at`.
Every candidate inference was rejected on evidence: `recorded_at` is the defect itself; `filled_count
> 0` is false for a fill that types nothing and for one whose closing snapshot fails
(`browser.go:4250`), both of which write a row byte-identical to preparation's; and
`form_inventory.ready` means the form was *read*, which is exactly the conflation being removed.

**NULL means unknown — but evidence that exists is not discarded.** A row written before the column
existed usually says nothing about whether a fill ran, and those rows are left NULL: a cleaner-looking
dashboard is not worth a fabricated fact, and the UI has a state for ignorance. The exception is a row
carrying a non-zero `filled_count`, `reused_answers` or `documents`, which is not ambiguous at all —
those columns are written only from a fill report, and this decision's own writer split means
preparation cannot write them. The migration recovers exactly those.

Getting this wrong in the first cut is worth recording, because two of the three independent reviewers
found it and both found it the same way: the original migration refused *all* backfill, while the
paragraph above in `form_inventory.go` declared `filled_count > 0` structural proof that a fill ran.
Both could not stand. Refusing the inference did not protect anything — it converted known work into
unknown work, and the card renders unknown by saying no fill is recorded, which about a row holding
eight filled fields is this defect with its sign flipped. A rule against inventing history is not a
rule against reading evidence.

**Preparation cannot claim or erase a fill outcome, by signature.** `RecordPreparedQuestions` takes no
summary parameter, so there is no argument through which a preparation run could say anything about
filling. This closes a second defect found by the same audit and not in the original report: the old
upsert wrote *every* column from the zero value it was handed, so re-preparing an already-filled
application reset its `filled_count`, `documents` and `filled_labels` to nothing — silently weakening
decision 7's own `filled_count > 0` evidence. Removing the parameter is the difference between an
invariant and a convention that holds until someone edits a call site; the source-level assertion
over `cmd/preflight` (decision 5) was extended to enforce it.

**An attempt is marked when a fill reaches the form, never when it succeeds.** This is what makes the
record true on the paths that report nothing at all — a Playwright error, a browser closed mid-fill,
a handler that dies before any summary is written. Every one of those previously left the database
looking exactly like an application nobody had opened. A fill that runs and completes zero fields is
still a fill, and only that state entitles the product to say the attempt completed nothing.

*Reaches the form*, not *begins*, and the distinction cost a round to learn. The first cut marked the
attempt immediately before calling the fill, and the independent review pointed out that
`FillAssistedMappedPage` returns on four guards — expired posting, bot check, quarantined DOM,
unreadable content — before it reads a single control. On a posting taken down since it was queued,
the card would have said *"Career Agent attempted this form"* about a page with no form on it, and
then sent the operator to hand-fill it. The marker therefore travels into the fill as
`AssistedFillPlan.OnFormReached` and fires from the one place that knows: past every guard, with a
real form surface resolved, about to touch the employer's controls. The answers path marks before its
call instead, correctly — `ApplyApprovedAnswers` has no guards to clear, and the operator is by then
answering questions read off the form in front of them.

Because a fill can also error *after* typing several fields — the handler returns and its report is
discarded — the card does not claim the form is empty. It reports what was *recorded*, which is the
most it can honestly know from this table.

Automatic Apply is unchanged and deliberately so: it never wrote this table, sharing the ATS handlers
but not the reporting layer, and recording its own evidence in `application_attempts`. Defining the
attempt once at the storage boundary is what keeps Assisted and Automatic from drifting into two
definitions.

The review clock (`assistedReviewStartedAt`) still reads `recorded_at`, and this change is careful not
to feed it. `MarkFillAttempted` deliberately does not write `recorded_at` — not on conflict, and not
on insert either, where it stores an unparseable empty value rather than a timestamp. The first cut
did write one there, and the review caught what that meant: on a never-prepared job whose fill failed,
the operator's own forty minutes of hand-filling would have been booked as forty minutes of *review*,
inflating the one metric that exists to tell them whether their time is being saved.

That the clock starts at preparation at all is a separate, real defect — on job 310026 it would
measure 17h23m against a 30-minute credibility cap and be silently discarded — and it is filed with
its own evidence rather than folded in here.

**What this decision deliberately does not cover.** Automatic Apply fills real employer forms and
writes no fill summary at all, before or after this change, so an application that reached
`AWAITING_REVIEW` through `cmd/agent` carries no marker. That is not a regression — the card said
something equally uninformative about those jobs before — but it does mean the invariant at the top
of this decision describes the assisted path only. Whether a copilot-mode fill whose browser has since
closed, and whose typed values are therefore gone, should be reported to the operator as work Career
Agent did is a genuine product question rather than an oversight, and it is filed separately.

*The paragraph above is now historical. Decision 9 answers the question it deliberately left open.*

### 9. `FillSummary` gains a source, because two processes can now write it and only one's browser is ever still open

*Added 2026-08-17, closing bugs.md #551.*

Decision 8's invariant held for the assisted path and, by its own admission, described nothing about
Automatic Apply: `cmd/agent`'s `AttemptSubmit` reaches `AWAITING_REVIEW` through the same submit gate
`FillAssistedMappedPage` does, fills the same employer form through the same ATS handlers, and wrote
nothing to `assisted_fill_summary` either before or after decision 8. Two of decision 8's own
independent reviewers filed this as a defect in its own right rather than folding it in: it is not a
mechanical extension of the marker, because an automatic fill's browser closes before the operator
ever opens the Assisted queue, so reporting "8 fields filled" about it would describe values that are
no longer anywhere for the operator to check — a different, and in one direction worse, way of the
card lying about its own work than decision 8 fixed.

The fix is deliberately not a parallel table. `RecordAutomaticFillAttempt` (`pkg/storage`) is a second
caller of the same `markFillAttempted` body `MarkFillAttempted` already used, differing only in the
one fact that was missing: which machinery ran the attempt. `assisted_fill_summary` gains one column,
`fill_source` (`FillSourceAutomatic` | `FillSourceAssisted`), rather than a second row shape for the
same job — the alternative decision 7 already rejected for a different column, for the same reason:
two places for the same application's fill history to live is two places for them to disagree.

Three properties, each closing a way this could still lie:

* **The marker fires only past every guard `AttemptSubmit` has that can end it before a control is
  touched** — dead posting, bot protection, DOM quarantine, the account-gated-ATS early return — at
  each of the handful of points in `AttemptSubmit` that actually dispatch to an ATS handler (the
  cached-mapping fast path, the dedicated-handler path, the LinkedIn path, and the Learner Module
  path). This is decision 8's own hard-won lesson (`FillAssistedMappedPage`'s `OnFormReached`,
  documented above) applied to a function with several dispatch points instead of one: a dead posting
  or a bot check must still record nothing, and a fill that reaches a handler and then fails is still
  a fill.
* **`fill_source` moves with `fill_attempted_at`, not against it.** Both describe the most recent fill
  attempt, whichever process ran it; a later attempt's marker call overwrites both unconditionally,
  because a second, later attempt by different machinery genuinely means the earlier one's history no
  longer describes what a browser might currently hold. The one COALESCE in this design
  (`ReplaceApplicationQuestions`' and `RecordAssistedAnswersApplied`'s fill-completion writers keeping
  an existing `fill_attempted_at` rather than advancing it to their own completion timestamp) protects
  a single attempt's begin-time from its own completion writer, not one attempt's history from a later
  one — the completion writers always set `fill_source` to `FillSourceAssisted` unconditionally,
  because reaching either of them means a live assisted browser just did real work.
* **No field values are added anywhere.** `RecordAutomaticFillAttempt` records a timestamp and a
  three-letter source tag against a job id resolved from the posting URL, the same lookup
  `EnsureAssistedPlanForURL` already does for the same reason (`AttemptSubmit` never sees `job_funnel`'s
  primary key). It does not attempt to count automatic fields filled: unlike the assisted path, which
  already resolves questions through the vault and can diff a before/after control snapshot, the
  automatic ATS handlers fill by direct field-by-field `Playwright` calls, and instrumenting six of them
  to count successes was judged a larger and riskier change than the truthfulness fix itself needed —
  `fill_source` alone is enough to keep the card honest, and a future task can add counts without
  touching this decision.

The UI consequence is the point of the whole fix: `CompletedSummary` (`cmd/dashboard/ui`) treats
`fill_source === 'automatic'` as its own state, distinct from "a fill ran and completed nothing" —
*"Career Agent previously attempted this form during Automatic Apply. That browser session has ended,
so those values are not present here — run Assisted Fill to populate the current form."* — rather than
implying the operator should "check the form" for values that cannot be there. Once `cmd/assist`'s
refill actually reaches the form in a fresh browser, `fill_source` flips to `FillSourceAssisted` with
real counts, and ordinary present-tense reporting resumes, because at that point the values may
genuinely still be on screen.

## Consequences

* The vault fills up. `SeedFromPII` is called at dashboard start, so a fresh install has suggestions
  to approve rather than blanks to retype. Every seeded row still has reuse withheld, so seeding can
  never grant an auto-fill permission.
* Approving one answer visibly resolves the rest of the queue, and says so with a count that came
  from a re-resolution rather than an estimate.
* Preflight is the only new outbound traffic in this change, and it is bounded, operator-initiated,
  read-only, and shown as a page count before it runs.
* **`dashboard.log` gains one new class of line: the posting URL.** `newSubmitPage` logs
  `Navigating to <url>` and preflight runs through it, so that record now reaches the dashboard's log
  where previously only `cmd/assist` — which logs hostnames only — did. This was checked rather than
  assumed: a real preflight run's stderr was scanned against all 80 values in `pii.yaml` and against
  markup, `value="`, `Call log:` and `locator resolved to`, and contained none of them, nor any
  employer question text. A public job-posting URL from this project's own database is not operator
  data and not page content, so it is within ADR-006's boundary; it is recorded here because it is a
  change in what that file contains and a future reader should not have to rediscover why. The same
  shared code also stamps preflight's records `[Auto-Submit]`, which is misleading for a path that
  cannot submit; renaming it touches the automatic path and was left alone deliberately.
* `bugs.md` #544 was found while writing this and fixed first: `years_experience` matched on a
  duration word plus "experience" and auto-filled the operator's whole career total into questions
  about one technology. Building demand-driven surfacing of skill questions on top of that would have
  multiplied it.
* What this does **not** do: it does not change Automatic Apply, `AttemptSubmit`, or the submit gate.
  The employer's Submit is still pressed by the human, and `cmd/assist` is untouched by this work.
* The direct-browser (Workable) path has no fill, no vault and no questions today. Knowledge features
  report it as not applicable rather than silently doing nothing there.

### 10. Education summaries are a curated pattern over the configured profile, not a per-job generation

*Added 2026-08-19.*

Factual education-summary questions ("Education background", "Highest level of education",
"post-secondary education") appeared repeatedly on the live queue but had no deterministic family,
so they fell into `generate_per_job` and were retyped. They are now the 20th curated pattern in
`pkg/answers/patterns.go`.

The answer is derived deterministically from the existing `pii.yaml` `education` list via
`config.PII.EducationSummary()`; no second education store was added, and nothing is invented when
the list is empty. The pattern is classified `Sensitive`, so it suggests the configured summary but
never auto-fills it until the operator explicitly approves it and allows reuse — the same two-
decision path used for work authorization and sponsorship. Free-text essay prompts about education
("Describe how your education prepared you...", "Why did you choose your field of study?") and
skill-scoped or possession questions ("Do you have a degree in computer science?") are rejected or
miss the pattern's `RequireAll` groups, so they do not inherit the generic summary.

### 11. Intentional absence is a first-class answer state, distinct from both "value" and "unanswered"

*Added 2026-08-19. Closes `improvements.md` #545.*

The vault previously had two states: a question has an approved answer (value), or it does not
(unresolved). Production evidence showed a third: the operator has decided, and the decision is
that an optional field gets nothing. Five out of six inspected applications asked for a Twitter/X
URL the operator does not have. The vault rejected empty answers by design (`answer_text TEXT NOT
NULL` and `Store.Save`'s empty-answer refusal), so there was no way to record this truthful,
reusable fact.

**Representation.** `KindAbsence` (`answer_kind = 'absence'`) is added to the existing `Kind`
enum. The `answer_text` column stores a human-readable reason (e.g. "No Twitter/X account")
rather than an empty string, preserving the non-NULL invariant and giving the vault management
view something meaningful to display. `Resolution` gains an `IntentionalAbsence bool` field that
the fill path uses to distinguish "leave the control untouched" from "type this value".

**Optional-field safety invariant.** The central rule: absence may only auto-fill (resolve
without operator intervention) when the live field is optional. If `Question.Required == true`,
`applyAbsenceSafety` in `resolve.go` forces `AutoFill = false`, so the field surfaces as needing
operator attention. The safety is enforced at the resolution boundary, not at storage time,
because the same absence answer must correctly resolve an optional Twitter field on one
application while surfacing as a conflict on another that marks the same field required.

**Canonicalization boundaries.** Absence reuse follows the same deterministic alias/key/scope
model as value answers. A Twitter absence does not spread to LinkedIn, GitHub, or portfolio
because those are separate question keys and separate patterns. No semantic similarity or broad
account-type inference is added.

**Sensitive-question safety.** Sensitive questions (work authorization, sponsorship, privacy
consent, legal attestations, EEO/demographic) require the same explicit two-decision approval
path for absence as for value answers. `SaveAbsence` refuses `GeneratePerJob` outright and
demands `ReuseDecisionMade && ReuseAllowed` for `Sensitive`.

**Metrics.** `Readiness.AbsenceResolved` counts fields handled by operator absence decisions,
as a subset of `Resolved`. The UI can accurately report "18/20 fields resolved (2 intentionally
left blank)" without inflating the "fields filled" count. The fill path skips the control
entirely rather than typing an empty string; `AssistedFillSummary.FilledCount` is not incremented.

**Approval path.** `knowledge.ApproveAbsence` stores the absence, binds aliases for equivalent
phrasings, and immediately re-evaluates the queue, following the same machinery `Approve` uses.
`POST /api/knowledge/absence` exposes this to the dashboard. Absence always grants reuse (an
absence without reuse resolves nothing and serves no purpose).

**Revocation.** The existing `Store.Revoke` applies unchanged: revoking an absence row
restores the question to unresolved, and the next re-evaluation surfaces it in the inbox.
