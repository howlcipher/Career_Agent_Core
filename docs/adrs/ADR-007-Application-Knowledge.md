# ADR-007: Application Knowledge and Preflight

## Status

**Accepted — implemented 2026-08-13** in `pkg/knowledge`, `pkg/storage/knowledge.go`,
`pkg/submitter/preflight.go`, `cmd/preflight`, `cmd/dashboard/knowledge_api.go`,
`cmd/dashboard/profile_api.go` and the dashboard UI. Closes `bugs.md` #544 and
`improvements.md` #538–#542.

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
| 2 | `pattern:<id>` | every phrasing of one of the 19 curated families |
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
happen before any page is loaded — an ATS that rejects Career Agent's browser, and one with no
pre-auth form at all (bug #18's Workday case) — and a test passes a nil browser to prove that
ordering.

A sign-in wall and an empty application form both look like "nothing to answer", so a password
control or an auth-wall phrase reports `auth_required` rather than `no_form_found`. Telling the
operator to expect an easy application and handing them a login page is the specific failure that
distinction prevents.

`cmd/preflight` runs as a dashboard child so its stderr passes through
`security.SanitizeChildLogLine` — ADR-006's protection, inherited rather than reimplemented. It logs
counts and reason codes, never a label and never a value. The batch cap is about how much traffic one
operator action may send to employers' servers, not about capability, and preflight is refused
outright while a visible assisted browser is open.

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

## Consequences

* The vault fills up. `SeedFromPII` is called at dashboard start, so a fresh install has suggestions
  to approve rather than blanks to retype. Every seeded row still has reuse withheld, so seeding can
  never grant an auto-fill permission.
* Approving one answer visibly resolves the rest of the queue, and says so with a count that came
  from a re-resolution rather than an estimate.
* Preflight is the only new outbound traffic in this change, and it is bounded, operator-initiated,
  read-only, and shown as a page count before it runs.
* `bugs.md` #544 was found while writing this and fixed first: `years_experience` matched on a
  duration word plus "experience" and auto-filled the operator's whole career total into questions
  about one technology. Building demand-driven surfacing of skill questions on top of that would have
  multiplied it.
* What this does **not** do: it does not change Automatic Apply, `AttemptSubmit`, or the submit gate.
  The employer's Submit is still pressed by the human, and `cmd/assist` is untouched by this work.
* The direct-browser (Workable) path has no fill, no vault and no questions today. Knowledge features
  report it as not applicable rather than silently doing nothing there.
