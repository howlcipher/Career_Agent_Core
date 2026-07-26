# Task Journal: Monitor the 82-job live run, fix bugs as they surface

## Summary

- **Task:** User asked me to sit and monitor the live 82-job re-verification run, fix any bugs that arise, log them in `bugs.md`, groom the backlog when doing so, and keep this journal against a session limit/outage. Explicit standing authority: *"If a choice arises, do what you recommend, don't feel you need to ask, do not do anything that adds a monetary cost."*
- **Status:** In progress — user set a `/goal` at ~14:00 making the monitor-and-fix loop a standing directive until the condition holds
- **Started:** 2026-07-25 ~12:24
- **Agent and model:** Claude Code / Opus 5

## Standing constraints for this session

- **No monetary cost.** Local Ollama only. Do not build anything needing a paid key/signup — that rules out `improvements_paywall.md` entirely (#17 CAPTCHA solving) and improvements #14 (LoRA, needs paid cloud compute — no discrete GPU here).
- **Do not kill/restart PID `3755906`** casually. Restarting loses in-flight progress and re-snapshots the queue (the queue is a *startup snapshot* — see the standing note in `bugs.md`'s Operational Trap section). Only restart when there is a fix worth picking up, and rebuild from current HEAD when doing so.

## Live state at session start (2026-07-25 12:24)

- Agent PID `3755906` (`/tmp/career_agent_bin_verify82j`, built at HEAD `81792b0`) — alive, started 12:04.
  **Note it predates `2972797` (#69) and `4134b3d` (#26)** — it does not carry the real-title fix or ATS feed discovery.
- 82-cohort breakdown: `DISCOVERED=69 FAILED_SUBMIT=6 PROCESSING=1 SKIPPED=6` → moved to `FAILED_SUBMIT=7 DISCOVERED=68` at ~12:32.
- **Still 0 of 82 confirmed `APPLIED`.** That remains the open question of the parent journal (`2026-07-21_verify-bug4-iframe-fill-live-batch.md`).

## Monitors armed in THIS session (previous session's were orphaned and killed)

- `bcivm59k9` — polls the 82-cohort status breakdown every 120s, emits on change, and on PID `3755906` death. Script: `scratchpad/watch_82.sh`.
- `bpsfomclo` — `tail -F career_agent.log` filtered to outcome/failure signatures.
- Killed PID `3756779`, the previous session's monitor `b03wmuerf` — it was alive but its notifications could never reach this session.

## Progress Log

- **12:24** — Resumed. Verified journal claims against the tree: working tree clean, HEAD `c9e270c`, agent process genuinely alive. The most-recently-modified journal (`2026-07-25_throughput-and-discovery-quality.md`) is **Complete**; the genuinely in-flight task is the parent 82-job re-verification.
- **12:32** — First bug surfaced live: **Reddit (`job-boards.greenhouse.io/reddit/jobs/8044767`), fit score 90 — the best-scoring job in the cohort — failed with `failed to submit application after 3 validation error attempts`.** Burned ~17.5 min of LLM time (12:14:55 → 12:32:28) for nothing. Under investigation; see below.

## ✅ FIRST CONFIRMED APPLICATION — Akuity, 2026-07-26 08:31:02

The question this whole 82-job effort was created to answer is now answered, with a real application to show for it.

```
08:30:45 Attempt 2 applied 7/7 validation fix(es)
08:31:01 Retrieved a security code from email (subject: "Security code for your application to Akuity")
08:31:01 akuity issued a security code after the last submit — that submission was ACCEPTED
08:31:01 Security-code gate detected for akuity — waiting for the emailed code...
08:31:01 Entered the emailed security code for akuity; resubmitting
08:31:02 Submit verdict after 500ms: confirmation phrase found on page (url moved: true, invalid fields: 0, page 14250 chars)
08:31:02 Submission confirmed for akuity ... at .../jobs/4240492009/confirmation
```

**Evidence, not inference:** the URL moved to a real `/confirmation` page, the document collapsed **126,557 → 14,250 chars** (the form is gone), a confirmation phrase is present, and `invalid fields: 0`.

**Verified in the database:**

| check | result |
| --- | --- |
| `job_funnel.status` | **`APPLIED`** (fit score 85) |
| `applied_jobs` dedup row | written `08:31:02.600` — matching the confirmation to the millisecond |
| `APPLIED` rows in the entire DB | **1** (was **0** across 3,884 rows) |
| `Submission confirmed` lines in the log | **1** (was **0**) |

That millisecond match is **#94** working as designed: before it, the dedup row was written at document generation ~13 minutes earlier, whether or not anything was ever submitted.

**Every link, each confirmed live:**

1. Form filled to `invalid fields: 0` — #98, #106, #107, #109, #110
2. Submit accepted by Greenhouse
3. Acceptance recognised **from the mailbox, not the DOM** — #111
4. Code challenge detected — #93
5. Real code retrieved over IMAP — improvements #32
6. Code distributed across **eight single-character boxes** — #115
7. Resubmit judged with a settle budget — #116
8. `APPLIED` and the dedup row written **only** on confirmation — #94

**The answer to the audit.** The historical `APPLIED` rows were not genuine, and the reason was never an inability to fill forms. It was two things: the pipeline submitted without being able to tell that it had (#95, #102, #111), and it could not complete Greenhouse's out-of-band code challenge (#113, #115, #116). Both are fixed and demonstrated.

**Note on the funnel rows:** `Akuity` still shows `FAILED_SUBMIT` alongside `akuity` → `APPLIED`. That is **#112**'s scheme-duplicate issue, documented rather than guessed at. The application is real; the stale twin row is bookkeeping.

## ⚠️ DUPLICATE APPLICATION FILED TO AKUITY — disclosure (2026-07-26)

**Akuity received TWO applications for the same role.** Found by checking the inbox after the confirmed run, not from the logs:

| "Thank you for applying to Akuity" | corresponds to |
| --- | --- |
| **08:01:01** | the run the agent reported as `(code entered, no confirmation)` |
| **08:32:03** | the run reported here as the first confirmed application |

**So #115 completed the application at 08:01 as well.** The code was entered, the resubmit went through, and Greenhouse acknowledged receipt — the agent simply could not *detect* it, which was **#116**'s bug. I then requeued Akuity and it applied a second time at 08:31.

**This is #89's duplicate-application risk materialising**, through the combination of an undetected success and my own restart-and-requeue loop. #116 prevents the recurrence — the first success is now detected and the loop stops — but the duplicate already exists and cannot be undone.

**Honest attribution:** the requeue decisions were mine. Each was individually justified by a specific fix, but the cumulative effect on a job that had already succeeded invisibly was a second real application to a real employer. The lesson is narrower than "requeue less": **before requeuing a job that reached the code-entry stage, check the inbox for a completion email**, because that is the only place the success is visible when the in-page verdict fails.

**Also established in the same check:** ClickHouse has a **pending accepted application** (code `p5Kqsn22`, emailed 08:48:11) that was never completed — see #117 for why the agent missed it. Four further Akuity code emails (05:59, 06:30, 07:00, 07:30) are from accepted attempts that did not complete.

## Shipped this session

- **bugs.md #70 (Blocker) — `d68ce61`.** The validation-retry loop stripped the page's own error text. `aria-describedby` was in `presentationalAttrs`, so `StripPresentationalAttrs` severed the WCAG link from a rejected control to its error message *before* `PruneDOMToInvalidFields` ran; the pruner then dropped the message element as neither control nor label. The model was told a field was invalid but never what would make it valid. Plus an empty fix map fell through to re-submitting a byte-identical form. Fixed all three; 2 new regression tests, verified failing first.
- **bugs.md #71 (Major) — `4234fba`.** `firstVisibleLocator` falls back to `loc.First()` when nothing is visible, and the submit-click site then clicked an element it had just proven invisible — hanging the full 30s action timeout and misreporting "no visible submit button" as a generic Playwright timeout. Added `firstVisibleSubmit` returning `(locator, ok)`; fill call sites keep the old fallback. 2 new tests.
- **bugs.md #72 (Major) — `ff00d80`.** Found by #70's own diagnostic within an hour of it shipping. Reddit logged `Attempt 2 applied 15/15 validation fix(es)` and still bounced with the invalid-field payload essentially unchanged (8249 → 8334), so the tally was not measuring what it claimed. Two accounting defects: `applyValidationFix` returns nil for an empty value (right for the initial-fill path, a lie in the retry path — shared contract, so fixed at the call site), and a nil return only means Playwright accepted the call, not that the control ended up set. Added `verifyFixLanded` read-back. 3 new tests.
  - **#70's fix is confirmed working:** the narrowed payload grew **5,363 → 8,249 chars on the identical form**. That delta is the error text now reaching the model.
  - **Still open:** this fixes the measurement, not necessarily the non-convergence. The autocomplete/combobox theory (`candidate-location`, `country` are Greenhouse autocompletes) is a hypothesis, not a confirmed root cause.
- **bugs.md #73 (Major) — `5425359`.** Caught on Reddit's third and final attempt: 6 of 15 fixes failed with `selector matched no element` for `input#430` … `input#436`. **`#430` is a CSS syntax error** — an id selector cannot begin with a digit — and Greenhouse numbers its custom-question controls exactly that way. `resolveFieldLocator` built its attribute fallbacks only when the selector did *not* look like CSS, and `input#430` contains `#`, so it was used verbatim (`tried 1 form(s)`, versus 5 for a bare identifier). The model sent bare `430` on attempt 2 and `input#430` on attempt 3 for the *same field on the same form* — resolved, then dead. That also explains `15/15` dropping to `9/15` with no page change. Added `splitTagID`; 2 new tests.
- **bugs.md #74 (Major) — `78f1dd0`.** #72's autocomplete hypothesis, promoted to root cause by fetching Reddit's real Greenhouse markup rather than by inference. `Location (City)` and `Country` are **react-select**: the `<input id="candidate-location">` is a *search* box, the committed value lives in React state and renders into a sibling `.select__single-value`. `Fill()` types and commits nothing, so the validated value stays empty — and both fields are **required**, so the form could never pass however many retries ran. It also caught a latent flaw in #72's own read-back (`el.value` is `""` either way, a false negative on a *working* combobox). `commitComboboxSelection` presses Enter and re-reads, firing only inside `.select__control`/`.select-shell`/`role="combobox"` since a stray Enter in a plain input can submit the form early. 2 new tests. **Marked live-confirmation-pending** — mechanism is not in doubt, but no `APPLIED` has come through it yet.
- **bugs.md #75 (Major) — `37aca0e`.** Precisely the gap #67 found, one layer up. `safeFillWithLabelFallback`'s three tiers all use plain `Fill()`, so a react-select field is typed into but never committed on the *initial* fill. `Location`/`Country` are required on every Greenhouse form, so the first submit was **guaranteed** to bounce and buy a full ~12-min `SolveValidationErrors` cycle to commit what a keypress could have. Confirmed live at 13:36 on the run already carrying #74: attempt 1 bounced with the narrowed payload at **exactly 8249 chars, byte-for-byte the same as the run before it**. 2 new tests. **Pattern: #65/#66 → #67, and now #74 → #75** — twice a fill capability has been added to the retry path only. Check both paths on any future change to how a control is set.
  - **Deliberately did NOT restart for #75.** PID `3797112` already carries #74 and was mid-attempt-2 inference — the decisive test of whether the combobox commit yields the first `APPLIED`. #75 only saves time; restarting would have discarded ~12 min about to answer the real question. Restart for #75 once Reddit resolves.
- **bugs.md #76 (Blocker) — `b352566`.** **A defect in #74's own fix, caught by a log line that did not appear.** Reddit logged `Attempt 2 applied 15/15 validation fix(es)` with *no* `committed N autocomplete selection(s)` line and *no* `left the control empty` line — a combination only possible if `verifyFixLanded` returned true for all 15, i.e. the commit step never ran once. Cause: the read script tested `el.value` before the combobox branch, and after `Fill()` a react-select search input **does** hold the typed text. So the check meant to detect "typed but not committed" was reading the artifact of typing. #74 was inert on exactly the fields it was written for; #75 inherited it. Split into `readInputValueJS`/`readComboboxValueJS` and moved the choice into Go (`locatorHasValue`) so the ordering is unit-testable — branching inside one JS blob is what let this ship. 2 new tests.
  - **Method note worth keeping:** the signal was an *absent* log line. Both #74 and #75 looked correct in isolation and had passing tests. After any fix whose purpose is to fire on a specific condition, deliberately check that it actually announces itself at runtime.
- **improvements.md #28 filed (score 2.33)** — `handleGreenhouse` fills only first/last/email/phone and **never attempts `Location (City)` or `Country`**, both required on every Greenhouse form. So the first submit always bounces and the *only* path that can satisfy them is a `SolveValidationErrors` call, ~12 min of inference, on **every** Greenhouse posting. Blocked on a `pii.yaml` schema change (`config.PII` has `Address` but no discrete city/country) that only the user can populate with real data — parsing the free-text address was considered and rejected, since a wrong value there is worse than none. **Not autonomous work.**
- **bugs.md #77 (Major) — `6ae6c08`.** Caught the moment #76 made the read-back work: `Attempt 2: 11 fix(es) reported success but left the control empty ... candidate-location, country ...` with **no** commit line — so `commitComboboxOnLocator` pressed Enter and still committed nothing. react-select populates its menu asynchronously (Location is geocoder-backed), so an Enter fired right after `Fill()` lands on an empty menu and selects nothing. `waitForComboboxOptions` polls for `[role="option"], .select__option` on a 5s budget, document-wide (react-select often renders into a portal). 1 new test.
  - **First real progress signal of the whole investigation:** the narrowed invalid-field payload **shrank**, 8249 → 5988 chars (−28%). Every previous attempt held flat or grew (8249 → 8334). Four fields are now being satisfied.
  - **#76 is confirmed working** — before it all 15 reported as landed; now 11 are correctly flagged as still empty.
  - **Method note:** the `left the control empty` line was **not in the log monitor's filter**, so it never fired a notification. Found only by grepping the log after the payload size dropped unexpectedly. Filter widened (`bbk8sss42` → `bdwa415i5`). **An absent notification is not evidence of an absent event — check the filter before concluding anything from silence.**
- **improvements.md #28 — Done (`96b4896`).** User supplied the address fields on request, so this became actionable and shipped the same day. `handleGreenhouse` now sets Location and Country on the initial fill via the combobox-commit path. Two findings: (a) **yaml.v3 matches keys case-sensitively** — verified empirically, `City:` binds to nothing on a `yaml:"city"` field and returns *no error*, so the user's `City`/`State`/`Full_state` would have silently loaded blank; `PII.UnmarshalYAML` now lower-cases keys, pinned by a test using their exact casing. (b) The geocoder's accepted phrasing is unknowable in advance, so `LocationSearchCandidates` offers `"City, ST"` → `"City, Full State"` → `"City"` and stops at whichever commits. **`country` was missing and I added `country: "United States"`** to `pii.yaml` (gitignored, never committed) — derivable with certainty from a Michigan ZIP+state. Free-text `Address` deliberately still not parsed.
- **bugs.md #78 + #79 (both Blocker) — `00e28fc`.** Found by building a standalone Playwright probe against Reddit's real form once each hypothesis started costing ~12 min of inference to test through the agent; feedback dropped to ~30s. Four defects: (a) `Fill()` never opens a react-select menu — own option count 0 and `aria-activedescendant` empty for 3s; real keystrokes open it in ~600ms. (b) react-select sets `role="combobox"` **on the input**, and `closest()` tests self first, so the value read resolved its shell to the input and never saw the committed value — this is why #74/#75/#77 all looked inert. (c) the options wait counted `[role="option"]` document-wide, and every Greenhouse page carries an always-open intl-tel-input phone widget with ~244 options, so the wait always returned instantly. (d) **committing option-0 is unsafe** — "Macomb" puts *Macomb, Illinois* first while the address is Michigan; an earlier probe run committed exactly that. Now scoped via `aria-controls`, and `pickComboboxOption` requires city+state tokens and selects **nothing** when no option genuinely matches. **Verified live:** `COMMITTED "Township of Macomb, Michigan, United States"`.
- **improvements.md #29 — Done (`0f0a4a9`).** User asked for hard-coded repeatable facts. `pii.yaml` gains `links`/`work`/`education`/`experience`; `PII.ApplicationFacts()` renders them into the prompt context for both `ExtractFormMapping` and `SolveValidationErrors`. Filled from `USER_PROFILE.md`. **Legal attestations left blank deliberately** (work authorization, sponsorship, visa, clearance, criminal history, over-18) — declarations on a real application, not the agent's to guess. Caught a bug of my own reading the rendered output: `strings.Trim(s, "to")` is a character *set*, so ranges rendered as `Feb 2023 to Presen`.
- **bugs.md #80 (Major) — observability, filed the moment the diagnostics ran out.** `Attempt 2 applied 13/13 validation fix(es)` with **no** not-landed line, **no** failure line, and the form still bounced (`7212 -> 7281 chars`). Every signal said success; the outcome was failure. Payload size alone cannot distinguish "same fields still failing" from "different fields now failing", so the next move would have been another blind ~25-min cycle — the same trap #70/#76/#77 each sprang. `parser.InvalidFieldIdentifiers` now names the flagged controls (id → name → tag) in the log. 2 new tests.
- **bugs.md #81 (Blocker) — `3978ec6`.** #80 paid for itself within one cycle. `Attempt 2 applied 13/13` and the still-invalid list came back **byte-identical** — nothing landed, including the freely-declinable EEO fields. Probed: after a bare `Fill()` with **nothing selected**, the value read returned `"I don't wish to answer"`, because react-select puts `data-value` on `.select__input-container` to mirror the *typed search text* for input auto-sizing. The `[data-value]` fallback was reading the artifact of typing — **the same mistake as #76, one layer deeper**, and it hid because it only fires when `el.value` is empty. False "landed" → commit skipped → field never set, on every custom question of every Greenhouse form. Location/Country were unaffected because `fillGreenhouseCombobox` clicks a real option. Also fixed: a verification *error* fell through `vErr == nil && !landed`, recording a field as neither landed nor failed so it vanished from the logs entirely.
- **bugs.md #82 (Blocker) — `2cd9f3e`. A risk *created* by fixing #81, not revealed by it.** While the commit was broken, nothing the model proposed was ever really set. Probed straight after #81: `#question_67942418 -> COMMITTED "Yes"`. So from #81 onward the model's answer to *"Are you currently authorized to work in the U.S.?"* is genuinely submitted. The decline-option safety net does **not** cover these: EEO all offer "I don't wish to answer" (verified on 430/433/434), but work authorization and sponsorship are **Yes|No with no decline** — nothing to decline *to*, so the model picks one, and that is a legal declaration under the user's name. Now refused **before** `SolveValidationErrors` is called (so no guess can exist, and ~12 min of inference is saved), routed to `MANUAL_REQUIRED`. Detection is phrase-based, with a test pinning that "desired salary"/"why do you want to work here" do not trip it — a false positive costs a real application. 4 new tests.
  - **Killed PID `3873741` immediately on realising it carried #81 without #82.** Verified nothing was submitted: **zero `Submission confirmed` lines in the entire log**, and the run died at the fill stage before any retry could commit.
- **bugs.md #83 (Major) — `c4b28c4`.** Predicted, then watched: a Greenhouse theme (`surtai`) setting no `aria-invalid` defeated #64's narrowing, so the retry sent the whole form — **50,501 chars**. That fits the 80,000-char *context* ceiling and passed the check, then ran **16:58:03 → 17:43:03, exactly the 45-minute Ollama timeout**. Context capacity and inference time are different limits and the time one binds first here: ~7 tok/s × ~2.5 chars/token ≈ 17.5 chars/s, so 45 min allows ~47,000 chars — the observed 50,501 sits just past it. Added `maxPromptCharsForTimeBudget` (40,000). **Corrected a #60 test case** that asserted 54,917 chars should pass, with the derivation written into the test so it reads as a correction, not a regression.
- **bugs.md #84 (Major) — `3ce3095`. My own error.** #82's guard refused ClickHouse in **0 seconds** (`work authorization, visa sponsorship`) exactly as designed — but the job landed in `FAILED_SUBMIT`, not `MANUAL_REQUIRED`, and the routing line never fired. **The `cmd/agent` branch did not exist in the source at all**: the scripted edit silently failed to match, `go build` passed because the submitter half compiled, and I verified the build instead of the edit. Same failure mode as #76/#77/#81 — *trusting an absence*; there a log line that never appeared, here a compiler that never complained. Fixed with the branch (verified by `grep`, not inference) **plus a structural guarantee**: `manualReviewErrors`/`IsManualReviewError` consulted by `cmd/agent` as a catch-all before the generic failure log, so a future sentinel without its own branch still reaches manual review. 2 new tests.
- **bugs.md #85 (Major) — `b98962a`.** Found by noticing an impossible number: cohort monitor reported `PROCESSING=4` on a **1-worker** run. Four `continue` paths exit the worker loop without clearing the `PROCESSING` set at the top — invalid URL, request-creation failure, response-read failure, and the duplicate check. `GetDiscoveredJobs` selects only `DISCOVERED`, so a stranded row never returns to any queue. **#55's startup reaper was masking it** — each restart reset the orphans, they were re-picked, skipped and stranded again, consuming a queue slot per run and inflating the dashboard's in-flight numbers. Invalid URL now takes the `INVALID_URL` status that already existed for it; the rest reset to `DISCOVERED`. **On the duplicate case, `DISCOVERED` deliberately and not `APPLIED`:** the `applied_jobs` record is written at document generation, not confirmed submission — exactly the falsehood this 82-job effort exists to audit (#53) — so asserting `APPLIED` would manufacture the claim under investigation. Deeper dedup-semantics issue left open, not silently redefined.
  - **Deliberately did not restart for this one.** Ten restarts today; each re-scores the same top-ranked jobs (~10 min apiece) instead of advancing through the 82, so restart frequency is now itself a throughput cost. #85's harm is bounded and self-healing via the reaper — unlike #82, which was safety-critical and justified an immediate kill. Batch it into the next natural restart.
- **bugs.md #86 (Blocker) — `e39f6c7`.** Nova (Lever) failed 3 attempts applying **7/7** each time. Probed the real form: only **3 required fields** (name, email, location) and the resume upload is *optional* — so it was entirely the location widget. Lever's typeahead has **none** of react-select's markers; it is a plain `<input name="location">` beside a hidden `<input name="selectedLocation">` holding the committed value. Detection returned false → filled with text, never committed, hidden field empty. Second obstacle: **clicking the option loses a blur race** (measured: both visible and hidden ended empty); keyboard is blur-safe. Verified live in one run: detection true, 4 options, correct one picked, `selectedLocation` populated. **Caveat left open:** Lever's geocoder finds nothing for `Macomb`/`Macomb Township`/`Macomb, MI` while Greenhouse resolves it — substituting a nearby city would misrepresent the applicant's location, so it is not done.
- **bugs.md #87 (Blocker) — `c00d8c7`. The one silently invalidating the whole retry path.** Orkes applied `2/2` fixes and failed 3 attempts; probing proved both fields settable in either order with no interference. The tell: `applied 2/2` and `Submission failed validation` in the **same second** — too fast for navigation, so nothing was being submitted. The submit locator was one CSS alternation including `button:has-text('Apply')`, and alternations have **no precedence** — matches return in DOM order: `[0] Apply (type=button)`, `[1] Quick Apply`, `[2] Submit application (type=submit)`. Every retry clicked the click-to-reveal button. **This re-frames #70-#81:** all real and necessary, but none could have completed such a form, because fields were filled correctly and then never submitted. The "applied N/N and still rejected" signature had two causes; this was the second. `submitControlSelectors` now tried in precedence order, with "Apply" absent entirely.
- **bugs.md #88 (Major) — `004213b`.** Not a broken mechanism — the mechanism working and hitting an honest dead end. Nova (Lever) produced exactly the diagnostic #81 was built for: `Attempt 3: 1 fix(es) reported success but left the control empty ... input[data-qa='location-input']`. Cause is **data**: Lever's geocoder returns **zero** results for `Macomb`/`Macomb Township`/`Macomb, MI` while Greenhouse resolves the same address. No option to select → required hidden `selectedLocation` never populated → form can never validate. The *outcome* was the bug: 3 wasted attempts then `FAILED_SUBMIT`, discarding a job a human could finish in seconds. `ErrUncommittableField` now routes it to `MANUAL_REQUIRED` with documents preserved and the field named. **Option not taken:** Detroit is 25 miles away and resolves fine on Lever — using it would make the form submit and would state a false location on a real application. Same reasoning as #82.
  - **User-facing consequence:** 39 of the original 82 jobs are Lever. A `pii.yaml` location both geocoders index would unblock a large share — but that is the user's call about their own address, not the agent's.
- **Open, single observation, not chased:** Nova's attempt-3 submit click hit a 30s Playwright timeout. Probed `#btn-submit` on a fresh page: visible, enabled, uncovered, trial-click actionable in 0.0s. So the timeout came from *state*, most plausibly an hCaptcha overlay after repeated submits (Lever's known behaviour, and the origin of #71's hidden `hcaptchaSubmitBtn`). Recorded rather than built on.
- **bugs.md #89 (Blocker) — `a010cd9`.** Surfaced by an outcome that did not fit: Orkes routed to `MANUAL_REQUIRED` via #83 with a **43,411-char** payload, which only happens when narrowing finds **nothing invalid** and falls back to the whole document — right after attempt 2 applied both outstanding fixes. Greenhouse replaces the form **in place**, so `currentURL == applyURL` and only a confirmation *phrase* can prove success; if the thank-you view renders after the 10s networkidle wait, the check reads the old DOM and reports failure. **The loop then re-submits an application that already succeeded — a duplicate filed with a real employer, invisible in the logs because it looks like a validation bounce.** Now re-checks confirmation at the top of every retry. Also logs whether a `<form>` is still present when narrowing finds nothing, since that meant two opposite things.
  - **Not established:** whether Orkes actually submitted. Evidence is circumstantial, browser state gone. The fix catches the *next* occurrence, not this one. Orkes sits in `MANUAL_REQUIRED` — safe either way, but worth checking the inbox before applying by hand.
  - **Correction to #87:** I treated "applied N/N and failure in the same second" as proof nothing was submitted. Too strong — a client-side rejection looks identical. #87 was still a genuine defect; the timing signature just did not prove that claim.
- **improvements.md #31 — Done (`e787c65`).** `handleLever` filled only name/email/phone while Lever marks **location required**, so its first submit always bounced — the same gap #28 closed for Greenhouse. Renamed `fillGreenhouseCombobox` → `fillComboboxFromCandidates`; the logic was never ATS-specific. **Third instance of one structural pattern** (#65/#66→#67, #74→#75, #28→#31): a fill capability wired into one path and not the others. Standing check now recorded in the backlog.
- **bugs.md #90 (Major) — `3f1b10d`. The closest the pipeline has come to finishing.** Sporty Group (Greenhouse, fit **90**) went from **11 invalid fields to 1** — payload 6389 → 610 chars, with 3 autocompletes committed, a GDPR consent checkbox ticked and a checkbox-group entry set. The survivor was `GDPR Acknowledgement*`, a combobox offering **exactly one option: "Acknowledge/Confirm"**; the model proposed a differently-worded affirmative, #79's containment check matched nothing, and nothing was selected. Right with several options (it is what stops "Detroit, ME" being filed for "Detroit, MI"); over-conservative with one, where no wrong choice exists. Now takes the sole option when `mustContain` is empty — **deliberately not** when it is set, since a lone option failing those tokens is a wrong answer, not an obvious one. 3 new tests.
  - **#88 earned its keep here:** the outcome was a preserved manual-review entry naming the exact field, not a silent `FAILED_SUBMIT` — which is what made this diagnosable in a single probe.
- **bugs.md #91 + #92 (both Major) — `8ca1d8f`.** **#91 is a defect in #90's own fix, caught on the very next run** — the same shape as #76 (defect in #74) and #81 (defect in #76's fallback). #90 takes the sole option when `len(options)==1`, and probing confirmed `GDPR Acknowledgement*` offers exactly one. But `setComboboxValue` **types the model's value before reading the options**, and typing "Yes" into a widget whose only entry is "Acknowledge/Confirm" filters the list to **zero** — so the count was never 1 and the rule could not fire for the case it was written for. **My probe opened the control with no query typed; the agent always types first.** The probe reproduced my mental model of the sequence, not the code's actual sequence, and that gap was the whole bug. Now clears the query and re-reads when typing yields nothing. **#92:** Greenhouse names checkbox-group controls `question_8242451101[]_54236360101`; `#question_...[]_...` is not a valid CSS id (brackets read as attribute syntax). Same class as #73, whose attribute-form fallback was blocked because `splitTagID` refused bracketed ids — `tried 1 form(s)` versus the 3 an eligible selector gets. 2 new tests.
  - **Method note, third occurrence (#76, #81, #91):** when a fix proves inert, the cause has each time been that my probe replicated the *outcome* I expected rather than the code's exact order of operations. Probing still found each one; the discipline is to mirror the code path step for step.
- **bugs.md #93 (Major) — `8c7afa7`. Found from the user's inbox, not the logs.** The user forwarded a Greenhouse email timestamped **20:58:03 UTC — the exact second of the surtai submit**: *"Copy and paste this code into the security code field on your application ... After you enter the code, resubmit your application."* So that submit **succeeded** and Greenhouse issued an out-of-band verification challenge; the resulting security-code input read as just another unsatisfied required field, so the whole 50,501-char form went to the model and burned the full 45-minute timeout. **This reframes #83** — its size ceiling remains correct, but the reason that form had nothing flagged invalid was that it was no longer a validation failure at all. `DetectSecurityCodeChallenge` now requires a code input **and** matching wording (either alone would strand real applications), and the retry loop returns `ErrNeedsEmailVerification` **before any model call**. 3 new tests.
- **improvements.md #32 filed, deliberately not built.** The user asked whether the agent can retrieve these codes. In-session it can (Gmail tooling retrieved `uOSBQvRu` on request, scoped to Greenhouse senders + "security code" subject). `cmd/agent` cannot — that needs Gmail OAuth wired in. Free, buildable, and narrowable to ATS senders only, **but it grants the agent read access to the user's mailbox** — a materially different level of trust from filling forms, and not an autonomous decision. Awaiting the user's call.
- **ALL ATTESTATIONS NOW SET (2026-07-25 ~20:55).** User populated `authorized_to_work_us`, `requires_sponsorship`, `visa_status`, `security_clearance`, `criminal_history`, `over_18`, plus `how_did_you_hear`, `notice_period`, `languages`. `MissingAttestations` now returns **empty** — #82's guard can no longer refuse any job. The largest systemic blocker of the day is gone. ClickHouse (90), DexCare (90) and Stack AV (80) requeued to take advantage.
  - **Note:** the user's first attempt to set the two keys did not reach the file (still `""` on disk). Verified before restarting rather than trusting the report — worth repeating, since a silently-blank attestation is invisible until a form asks.
- **improvements.md #32 — Done (`c1f46c1`), user-approved.** Emailed one-time codes are now retrieved automatically. **It needed far less than expected:** `pkg/tracker/imap.go` already spoke IMAP (`emersion/go-imap`) for the email tracker and `IMAP_USER`/`IMAP_APP_PASSWORD`/`IMAP_SERVER` were **already in `.env`** — no OAuth, no Google Cloud project, no new dependency. Access is bounded at three independent levels (known ATS sender domains; subject announcing a code; **arrival after the submit click that triggered it**, so a stale code can never be reused), mailbox opened **read-only**, and **the code is never logged** — only the subject. Extraction anchors on the introducing sentence rather than scanning the message, and `isPlausibleCode` rejects prose. Every failure path falls back to `ErrNeedsEmailVerification` and manual review. 8 new tests. Confirmed live at startup: `Security-code retrieval enabled (IMAP)`.
- **improvements.md #33 — Done (`e6a1f0a`-ish).** User direction: *"Macomb works, sometimes township doesn't show up ... try to get this right."* Re-tested Lever's geocoder on a fresh page: `Macomb` → 0, **`Macomb, MI` → 2 results**. **This corrects bugs.md #88** — the earlier zero for `Macomb, MI` was the geocoder rate-limiting after repeated probes, not an absence. Candidates now include the bare place name via `stripCivilDivisionSuffix`. Also caught a self-inflicted trap in the same test: `LocationMustContain` demanded the spelled-out `Michigan`, which would have **rejected Lever's own correct `Macomb, MI, USA`** — tokens now accept `|`-separated alternatives, while `Macomb, IL` still fails. Plus `earliest_start_date` computed as **today + 14 days** at render time per the user's standing rule.
- **User answers received:** attestations all set; Orkes confirmed **not** submitted (no confirmation email) so requeued rather than left in manual; `willing_to_relocate` set to yes (low impact — target is remote).
- **Backlog groomed:** dated groom-pass note added to `bugs.md`. No re-ranking was warranted — all bug rows Resolved, both remaining `improvements.md` Pending rows below the ROI floor.
- **Cleaned up 5 stale leftover monitor shells** (PIDs 2166044/2295635/2368407/2476472/2543238) from long-disconnected sessions, all targeting confirmed-dead PIDs 2165142-2542429. The 07-21 journal predicted they would self-terminate; they had not, and each held a `tail -F` on the log. Verified every target PID dead before killing.

## Session 2 (new Claude Code context, resumed 2026-07-25 21:21)

Context was cleared; re-oriented from this journal. Verified against the live tree rather than trusting it: working tree clean at `a30ffc3`, PID `3978328` genuinely alive, cohort `DISCOVERED=66 FAILED_SUBMIT=9 PROCESSING=1 SKIPPED=6`.

**Cleaned up 10 orphaned monitor processes** from disconnected sessions before doing anything else — the previous session's `watch_82.sh` pair, its log tail, and five `tail -n0 -F` shells that the last session's cleanup had missed (it killed the wrapper shells; the `tail` children survived). Re-armed two fresh monitors in this session: `bmrzy93bu` (cohort + PID death, script at this session's `scratchpad/watch_82.sh`) and `b1h3bcrgz` (log signals, filter widened to include `Duplicate check`).

- **bugs.md #94 (Blocker) — `9f6325a`.** Found in the 21:16 restart log, in four lines that look like routine housekeeping: `Duplicate check: Already applied to Reddit / Akuity / ClickHouse / Staff SRE. Skipping.` — all four of which the same day's log shows *failing*. `SaveApplication` ended with `RecordApplicationInDB`, so the `applied_jobs` row was written at **document generation**, minutes before the first submit click and regardless of outcome. ClickHouse is the clean case: dedup row timestamped `21:08:15.789`, the exact second its docs were saved, killed at 21:16 mid-attempt-3, never submitted. Because three separate mechanisms return the funnel row to `DISCOVERED` (the #55 reaper, #85's duplicate-path reset, requeue without `-clear-dedup`), such a job is loaded into **every** subsequent run, skipped in milliseconds, reset, and re-loaded — queued forever, never progressing, reading as `DISCOVERED` the whole time. **Silently unreachable rather than visibly failed.** Measured live: **7 of the 82 cohort, 66 rows DB-wide.** Fix: `SaveApplication` keeps the documents folder but writes no dedup row; `cmd/agent` writes it on the confirmed-submission branch only; `RecordApplicationInDB` is `ON CONFLICT DO NOTHING` because #89's re-check can legitimately confirm one URL twice.
  - **This is the write-side of #53.** #53 corrected the *dashboard* to count `job_funnel.status='APPLIED'`; the bad write was never fixed and `HasApplied` still trusted it. `job_funnel` has **never** held a single `APPLIED` row across 3,884 rows, while `applied_jobs` held 261.
  - **Two existing tests asserted the old behaviour and were deliberately inverted**, reasoning written into each test body so they read as corrections (the #83 precedent). All three new/changed tests verified failing against the old code before the fix was kept — done by temporarily restoring the old line, not by inspection.
  - **Method note, the inverse of the standing one.** The recurring warning here is *an absent signal is not evidence of an absent event* (#77, #84, #81). This was the mirror image: **a present, benign-looking signal is not evidence of a benign event.** `Duplicate check: Already applied` is exactly what correct dedup looks like; the defect was visible only in the conjunction with a company name the log had shown failing an hour earlier.
  - **Operational cleanup, scoped deliberately.** Cleared the dedup rows for the 7 stuck cohort jobs — safe to *assert*, not assume: all 7 timestamps fall inside the current log window, which contains **zero** `Submission confirmed` lines. **Did not clear the remaining DB-wide rows** — their timestamps predate the log, so there is no positive evidence either way, and re-applying to an employer who may already hold an application is outward-facing and the user's call. Left open for them rather than silently resolved in either direction.

- **bugs.md #95 (Blocker) — `e0c88f7`.** Three independent jobs produced the same impossible signature: **every field committed, every fix applied, and `Submission failed validation` in the same second** (ClickHouse 21:15:58, Stack AV 21:36:22, Sporty Group earlier). **Ruled out the fill by probe rather than by argument:** driving the agent's own commit sequence against ClickHouse's real form committed `"No"`/`"Yes"` into both react-selects and held `"Macomb, MI"` in the plain-text location question, after which native constraint validation over the form containing `#first_name` returned **`formValid: true`, `invalidCount: 0`** (it was `false` with 2 invalid before). So those forms are fully satisfiable and the verdict, not the fill, was wrong. Root cause: both `confirmOrError` and the retry loop judged the click from **one page read taken immediately after `WaitForLoadState(networkidle)`**. Playwright's `Click` returns on event dispatch, not when the app reacts, so at that moment there is frequently no request in flight, the page already counts as idle, the wait returns at once, and the DOM is read *before the submission has happened* — which is exactly why all four log lines share one second. `awaitSubmissionOutcome` now polls, with the rules in a pure `decideSubmissionOutcome` (unit-testable; branching inside the driver loop is what let #76 ship inert). 6 new tests.
  - **#93 is direct live evidence this misfires** — a Greenhouse security-code email timestamped the exact second of a submit the agent had written off as failed.
  - **Stated plainly: the race is inferred, not directly observed.** Confirming it would require clicking submit on a live posting, which files a real application. Strongly supported by every symptom, not proven. The first `Submission confirmed` settles it.
  - **The fix is one-directional by construction** — it can turn a premature "failed" into a correct "confirmed", never the reverse.
  - **Two measurement errors of my own, caught and corrected mid-probe**, both worth recording: (a) `el.parentElement.closest('.select-shell, .select__control')` resolves to the **inner** `.select__control`, which does not contain react-select's hidden `requiredInput` proxy — the same wrong-ancestor `closest()` trap as **#78(b)**, reproduced in my own probe; (b) `document.querySelector('form')` is not the application form, so a `formValid: true` reading was meaningless until it was re-anchored on `#first_name`'s form. **The first run of that probe appeared to refute the hypothesis and did not — it was measuring the wrong nodes.** Re-measured before drawing any conclusion.
  - **A hypothesis I did hold and had to drop:** that react-select's `requiredInput` proxies (required, `value:""`, and carrying **no id and no name**, so unaddressable by the agent) were the unsatisfiable blocker. The corrected probe refuted it outright — react-select *removes* those proxies from the DOM once a value is selected. Recorded because the "unaddressable required control" shape is a plausible future bug and was worth ruling out explicitly.

- **bugs.md #96 (Major, observability) — `acd1c5c`.** Filed the moment its absence had cost a day. The only trace of a judged submit was `Submission failed validation. Retrying...` — the decision, never the evidence. #95 was findable *only* by noticing four unrelated log lines shared a wall-clock second. `awaitSubmissionOutcome` now emits one line per submit click when it settles, carrying elapsed time, whether the URL moved, the flagged-field count and the returned page size — each separating cases previously indistinguishable (premature vs settled, in-place re-render vs navigation, same fields again vs different fields now, and the #83/#93 oversized-payload shape). Same class as #80, filed for the same reason.
- **#95 confirmed working live within minutes, on Reddit.** `21:56:20 Country set ... 21:56:22 Submission failed validation (page re-rendered with fields flagged invalid)` — **two seconds apart, not the same second**, and settled on positive rejection evidence rather than an empty read. The settle floor is doing exactly what it was written to do, and the new reason string is visible in the log rather than inferred. Reddit's *first* bounce is genuine: 13 custom questions are unanswered on the initial fill, which is expected. The open question is what attempt 2 does after the model answers them.

- **bugs.md #97 (Major, observability) — `39728b3`. Filed at the closest the pipeline has ever come to finishing.** Reddit went **13 invalid fields → 1**, narrowed payload **7,212 → 497 chars**, with 8 autocompletes committed *including both legal attestations*. The lone survivor was `#434` (*Are you a veteran/have you served in the military?*), which failed twice reporting only that the control was left empty. That is consistent with two opposite situations — broken commit machinery, or a value the widget does not offer — which need opposite fixes, and nothing in the log separated them. **A probe settled it: the mechanism is fine.** `#434` offers 9 options and typing `I don't wish to answer` filters it to *exactly that entry*, so #90/#91's sole-option path would commit it; `Prefer not to say` and `I am not a protected veteran` each filter it to **zero** (the #91 shape). So it is a **value mismatch**. The not-landed entry now carries `#434 (tried "…")`, which also flows into the `ErrUncommittableField` message. 1 new test.
  - **Deliberately did not also "fix" the model's wording.** Choosing a convergence rule before a single log line shows what the model actually proposes is exactly how #90 shipped a rule that #91 proved could never fire. Measure first — the next Reddit run will print the value.
  - **Third instance of one lesson in this session** (#80, #96, #97): every expensive failure here is *"the mechanism reported success and the outcome was failure"*, and each time the fix was to log the evidence a decision rested on, not the decision.
- **Restarted (21st) at 22:16 for #96/#97.** PID `4007419` (`verify84b`), 66 queued, all four of this session's fixes verified present by `strings` on the binary. Killed PID `3995985` (SIGKILL again). **Reddit requeued** `MANUAL_REQUIRED → DISCOVERED` with its dedup row cleared, so `#434` gets re-tested with the value now logged. Cost was ~3 min of Akuity scoring, cheap against having the diagnostic on every subsequent Greenhouse job.
  - **Watch out: the stored URL is `https://`, and the log prints `http://`.** A scoped `UPDATE`/`DELETE` written from the log's spelling matched **zero rows** and silently did nothing. Caught it because the verification `SELECT` in the same statement returned no row — worth keeping that pattern, since a requeue that quietly no-ops looks identical to one that worked.
  - Log monitor replaced (`b1h3bcrgz` → `bkmg640jz`) to add `Submit verdict after` and the dedup-record failure line. #77's lesson: a diagnostic missing from the filter is invisible.

### bugs.md #98 — the held finding, confirmed and shipped (2026-07-25 22:47)

**#97's diagnostic answered it in exactly one cycle**, the same way #80 paid for itself. Reddit reached a single remaining invalid field and printed:

```
22:41:35 Attempt 2 ... left the control empty: 434 (tried "I am not a protected veteran")
22:42:39 Attempt 3 ... left the control empty: #434 (tried "I am not a protected veteran")
```

The **identical** wrong value both times — deterministic, so it would never have converged however many retries ran. And it is one of the exact phrasings the earlier probe had tested, which filters `#434`'s list to **zero**.

So the held question resolved to **wrong phrasing**, not a broken mechanism. Candidate fix (a) was the right one, and the probe's prediction held.

Shipped as **`a79744d`**: `enumerateComboboxOptions` opens each invalid control that is genuinely a combobox, reads its options with **no query typed** (#91), closes it, and names the exact permitted values in the prompt. **Wired into both the retry path and `ExtractFormMapping`**, per this backlog's standing check about capabilities added to one path only (#65/#66→#67, #74→#75, #28→#31). The `isComboboxLocator` gate is a **correctness** requirement, not an optimisation — the invalid-field list routinely includes checkboxes (Greenhouse's GDPR consent), and clicking one would toggle it. 3 new tests, including one pinning that a non-combobox is never clicked.

**Caught a dispatch-order bug in my own test mock while writing it** — the readiness probe case matched before the options case, and `comboboxOptionsJS` mentions `aria-activedescendant` too, so it swallowed the option read entirely. Same class as the ordering defects this session has been fixing all day (#76, #91), which is a fair reminder that the technique does not exempt the person applying it.

**Overhead of #98 measured, not assumed.** Suspected `openAndReadComboboxOptions` would burn `waitForComboboxReady`'s full 5s budget per field, since that polls for `aria-activedescendant` and it was unclear whether react-select sets it on a *bare* open with no query typed — which would have cost ~65s per retry on Reddit's 13-field form. Probed it: `aria-activedescendant` appears at **60-130ms**, essentially alongside the options (70-150ms), on all three fields tested. So enumeration costs ~150ms per field, **~2s per attempt**, against a ~15-minute model call. **No change made** — the 30s measurement replaced a fix for a problem that did not exist.

**Restarted (22nd) at 22:47.** PID `4019401` (`verify84c`), 66 queued, all **five** of this session's fixes verified present by `strings`. Reddit requeued (status verified `DISCOVERED`, dedup 0) — this run is the decisive test. Monitor `bnr24opx1`.

### THE CENTRAL QUESTION IS ANSWERED: submissions ARE reaching Greenhouse (2026-07-25 23:25)

Checked the inbox after Reddit's run ended ambiguously. Two Greenhouse security-code emails exist:

| email (UTC) | = EDT | subject |
| --- | --- | --- |
| `2026-07-26T01:15:58Z` | **21:15:58** | Security code for your application to **ClickHouse** |
| `2026-07-25T20:58:03Z` | 16:58:03 | Security code for your application to Surt AI (this is #93's) |

**`21:15:58` is the exact second of ClickHouse's attempt-2 submit** — the one the agent logged as `Submission failed validation. Retrying...`. That submit **reached Greenhouse's servers and was accepted**; Greenhouse then issued an out-of-band verification challenge.

**This is independent, artifact-based confirmation of #95's race**, arrived at from a completely different direction than the timing argument — and it is the second time an email has proved a submit succeeded while the agent recorded failure (#93 was the first). #95 was shipped on inferred evidence and explicitly flagged as unproven; it is now proven.

**It also reframes the parent journal's open question.** "0 confirmed `APPLIED`" has been read all along as *the pipeline cannot submit*. It is not: **the pipeline submits, and cannot tell that it did.** ClickHouse ran on the pre-#95 binary, whose verdict came from a single instantaneous DOM read.

**Why #93's detector did not catch it:** ClickHouse's attempt 3 went straight to `SolveValidationErrors` with no `Security-code gate detected` line, so `DetectSecurityCodeChallenge` saw nothing. Most plausibly the challenge had not rendered yet when the DOM was read — the same race — which would mean #95's 15s settle also fixes #93's detection timing. **Not established**: the browser state is gone and this is inference, not measurement.

**Two real, incomplete applications exist.** Greenhouse's own wording is *"After you enter the code, resubmit your application"*, so neither is filed yet: **ClickHouse** (code issued 21:15) and **Surt AI** (16:58). Codes of this kind usually expire. ClickHouse is already back in the queue as `DISCOVERED` with its dedup row cleared, so the current run will reach it and improvements #32's IMAP retrieval should complete it end to end — the designed path, and the first real test of #32 in the wild.

### Reddit at 23:19 is a DIFFERENT failure — the submit never reached the server

Same run, opposite evidence. #98 worked completely:

```
23:19:28 Attempt 2 committed 9 autocomplete selection(s): 430, 431, 432, 433, 434, 436, question_67942418, question_67942419, question_67942420
23:19:28 Attempt 2 applied 13/13 validation fix(es)
23:19:43 Submit verdict after 15.1s: no confirmation and no rejection evidence within the settle budget (url moved: false, invalid fields: 0, page 169636 chars)
```

**`invalid fields: 0`** — the form was completely satisfied for the first time ever, `#434` included. And then: no confirmation, no rejection, a full 15s wait, and **no email**. So unlike ClickHouse, this submit did **not** reach Greenhouse.

`firstVisibleSubmit` raised no `none visible` error and the click returned no error, so a visible submit control was found and clicked and nothing happened. Open hypotheses, none yet tested: a decoy control winning precedence (#87's shape), an overlay intercepting the click (#34's shape), or a bot-protection gate. **Cannot be probed the usual way — clicking submit on a live posting files a real application** — so the probe must read the control's identity, obstruction and any challenge widget without clicking.

Attempt 3 then found nothing invalid, fell back to the whole 54,190-char form, and #83's ceiling routed it to `MANUAL_REQUIRED`. **That accidentally prevented a re-submit** of a form that might already have gone through — #89's exact risk, avoided by an unrelated guard rather than by design.

- **bugs.md #99 (Major) — `eecf49c`. Reddit's mystery, solved.** With #98 shipped the form reached **`invalid fields: 0`** — fully satisfied for the first time in the whole effort — and the submit still produced no confirmation and no rejection after the full 15s budget. **The inbox discriminated it**: ClickHouse's accepted submit produced a Greenhouse email in the same second; Reddit's produced nothing, so the request never reached the server. A read-only probe found the submit control **clean** — one match, `BUTTON type=submit "Submit application"`, visible, enabled, in-form, and `elementFromPoint` at its centre returning the button itself — which **rules out #87's decoy and #34's overlay**. It also found `recaptcha.net/recaptcha/enterprise/anchor` embedded. Score-based invisible reCAPTCHA discards a headless submission client-side: no error, no navigation, no request, no email. Solving captchas is paywalled (#17) and out of scope; reporting the truth is free, so budget-exhaustion **plus** a live provider frame is now `ErrCaptchaBlocked`. Reddit is labelled `BLOCKED_CAPTCHA` in ~15s instead of ~30min of model calls ending in a manual-review entry naming the wrong cause.
  - **Detection matches iframe `src`, never page wording, and only runs *after* a submit produced no outcome** — so it cannot pre-empt a working job. #45/#46 were phrase-matching captcha false positives that killed the large majority of Greenhouse/Lever/Ashby/Workable jobs before fit-scoring; a false positive here costs a real application. The provider pattern is **one Go constant** interpolated into the browser check *and* compiled in the test, so the test exercises the real pattern rather than a copy that can drift.
  - **Honest limit: Reddit is not completable by this pipeline for free.** That is a constraint, not a bug awaiting a fix.
- **bugs.md #100 (Major, observability) — `a6d444a`.** Akuity logged `applied 7/7`, **no** not-landed line — `verifyFixLanded` reported every control as set — and the **identical 7 fields** came back flagged. #97 names values only for fields that *fail* to land, so this opposite case had no diagnostic at all. Probing ruled out the convenient explanations: all 7 are plain required `INPUT`/`TEXTAREA`, single match, no `pattern`, and **React genuinely does observe `Fill()`** (its own prop value matches the DOM; the textarea is uncontrolled so the DOM is authoritative). Structural note kept: these controls have **no `name`**, so `FormData` serialises nothing and Greenhouse submits from React state — which is why "the DOM says it is set" is not proof the submission carries it. `rejectedDespiteLanding` now pairs each still-rejected id with the value written into it, matching by suffix because the model emits selectors while the parser reports bare ids (#73/#92's problem); a test pins that prefix collisions cannot mis-attribute a value. **Akuity's root cause remains open** — this makes the next occurrence diagnosable instead of guessing now.
- **bugs.md #101 (Major, observability) — `bf9f1cb`.** `grep` found **three** jobs ending the day with a bare `playwright: timeout: Timeout 30000ms exceeded` from the submit click — **Akuity, Nova, Zimperium** — each after a full set of fixes had been applied, each written off as generic `FAILED_SUBMIT`. A timeout says the click never landed and nothing about what stopped it; a disabled control, an off-screen one, a consent banner (#34) and a challenge frame (#99) were indistinguishable. Now reads `elementFromPoint` at the control's centre — **the same check that cleared Reddit's button in #99** — and names what covers it, including an iframe's `src`. The 07-21 journal had *guessed* at Nova's ("most plausibly an hCaptcha overlay"); this replaces the guess. Routing stays evidence-led (captcha only when a provider frame is genuinely present), and the probe is best-effort so it can never break the failure path it explains.

### Fourth instance of the session's dominant lesson

#80, #96, #97, #100 and #101 are all the same shape: **the mechanism reported success and the outcome was failure**, and each time the fix was to log the evidence a decision rested on rather than the decision. #80, #96 and #97 each paid for themselves within one cycle. That track record is why #100 and #101 were written *before* their root causes were known rather than after.

- **bugs.md #102 (Blocker) — `ada8f63`. A defect in my own #95 fix, and the largest misreading of the whole effort.** Found by reading the inbox, not the logs. Four Greenhouse code emails exist; lined up against the log the timestamps are decisive: **Akuity's is stamped 23:40:07, between its submit click (~23:40:06) and its verdict (23:40:08)**, and **ClickHouse's 00:05:34, the same second as its submit**. The server had accepted both before the agent called them failed. Cause: #95 treated "fields flagged `aria-invalid` past a 2s floor" as positive evidence of rejection, but Greenhouse **accepts** the submission, issues the code challenge, and leaves the *previous attempt's* `aria-invalid` markers in place while the challenge renders — so both signals are true at once and #95 read the stale one. Gate is now tested inside the verdict on every poll and **before** the flagged-field branch; floor raised **2s → 8s**; a gate routes to the existing #93/#32 path and explicitly **not** to #99's captcha branch. Rejection signal deliberately preserved when no gate is present, pinned by a test — removing it would push genuine failures into budget exhaustion, which #99 maps to `BLOCKED_CAPTCHA` on any reCAPTCHA page, trading one misreading for another. 4 new tests.
  - **The same trap, third time: #76 (`el.value` was the artifact of typing), #81 (`[data-value]` was the search text), #102 (`aria-invalid` is the prior attempt's leftover).** Each time a signal that looked like evidence was residue of the previous step. That it recurred *inside a fix written specifically to stop misreading post-submit state* is the part worth remembering.
  - **#101 paid off within the hour.** ClickHouse's final click timeout carried `element is not enabled` — Greenhouse **disables** the submit button once a submission is accepted and a code is pending. So the day's four `Timeout 30000ms` failures (Akuity, Nova, Zimperium, ClickHouse) were mostly the agent hammering a disabled button *after its application had already been accepted*.
  - **Corrected headline: the pipeline submitted FOUR applications today** (Surt AI, ClickHouse ×2, Akuity) **and recorded every one as a failure.** "0 confirmed `APPLIED`" was never a submission problem.
- **Restarted (23rd) at 00:14 for #99/#100/#101/#102**, promptly rather than at a natural break: until #102 runs, every Greenhouse job submits up to **three** times, each accepted, each generating a pending application — #89's duplicate risk actually materialising rather than theorised. ClickHouse alone accumulated three tonight. PID `4052202` (`verify84d`), 63 queued, all nine of this session's fixes verified present by `strings`. **ClickHouse and Akuity requeued** with dedup cleared so #102 + #32 can *complete* their pending applications rather than leave them stranded. Monitor `bx8k9li2z`.

- **bugs.md #103 (Blocker) — `fcce44a`. A defect in my own #98 fix, caught by #100's diagnostic within one cycle of #100 shipping.** The first job under the new binary printed:

```
00:31:55 Rejected despite being set last attempt:
  question_67179376 = "react-select-question_67179376-option-0|Yes";
  question_67179377 = "react-select-question_67179377-option-1|No";
  question_67179378 = "react-select-question_67179378-option-0|I agree"
```

  The model was answering with **react-select's internal DOM option ids**. Cause: `readComboboxOptions` deliberately returns `"id|label"` so `pickComboboxOption` can click the right entry — that is how #79's never-commit-the-wrong-entry guarantee works — and #98 rendered that raw into the prompt. So **#98, the fix whose entire purpose was to stop the model guessing wording it had never been shown, was showing it wording no human could choose**, on every combobox, from the moment it shipped. `optionLabel` now strips the id; bare entries (Lever's shape) pass through and a label containing a pipe keeps everything after the *first* separator. 2 tests, including an end-to-end assertion that no `react-select-`/`option-N` string can reach the generated block.
  - **Invisible to #97**, which names values only for fields that *fail* to land — these committed, because `setComboboxValue` types the bogus value, matches nothing, and #91's clear-and-re-read then commits *something*. Only **#100**, written for the "reported as set, rejected anyway" case and shipped **before its own root cause was known**, could surface it. Fourth consecutive observability fix to pay for itself within one cycle (#80, #96, #97, #100).
- **Restarted (24th) at 00:35 for #103.** PID `4059119` (`verify84e`), 63 queued. Restarted immediately rather than letting the run continue: #98 was actively feeding garbage to every combobox, so leaving it running was worse than losing the in-flight job. Staff SRE requeued with dedup cleared. Monitor `b5o4vor1f`.

- **bugs.md #104 (Major) — `f677830`. Predicted from #99+#102, then confirmed by measurement** — stated that way deliberately, because the previous prediction this session (#103's causal claim) was wrong and had to be retracted. Reasoning first: a reCAPTCHA-swallowed submit leaves the page untouched, so the prior attempt's `aria-invalid` markers persist, so the verdict settles as `reasonFieldsFlagged` at the 8s floor and **never reaches budget exhaustion** — the only state #99's captcha branch tests. The next run produced exactly that on Reddit `7956443`: all five custom questions set to sensible values (`"company website"`, `"Stellantis Financial Services"`, `"Yes"`, `"No"`, `"I agree"`), all three comboboxes committed, the identical five flagged again, and the page **byte-for-byte unchanged — 140544 chars across two separate runs**, 133352 at both initial submits. A server that had processed and re-rejected would not return identical bytes. Now returns `ErrCaptchaBlocked` when *every* still-rejected field was already written and a provider frame is present.
  - **Requires ALL fields, not some, and that constraint is load-bearing.** Every Greenhouse page carries reCAPTCHA, so the widget alone is far too weak a signal; it is the conjunction with "nothing left for the model to fix" that makes it evidence. One genuinely-bad answer among several keeps its remaining retry, pinned by a test. Same discipline as #99's iframe-`src` narrowness, for the same reason (#45/#46).
  - **This run also settled #103 in both directions:** the logged values are now human-readable labels, so the fix works; and the rejection is *unchanged*, so retracting its causal claim was right.

- **#101 confirmed live at 00:56** — the first accurate `BLOCKED_CAPTCHA` from the submit path: `Staff Site Reliability Engineer is behind a bot-protection challenge ... (submit click did not land; https://www.recaptcha.net/recaptcha/enterprise/anchor present): playwright: timeout: Timeout 30000ms exceeded.` That is the exact failure class that produced three bare, causeless timeouts earlier in the day.
- **#104 follow-up — `cf69812`. I corrected a false statement I had written into #104 as fact, and the correction caught a false positive before it could cause harm.** #104 justified its strictness with *"every Greenhouse page carries reCAPTCHA"*. That was an assumption. Measured across three boards:

| board | reCAPTCHA frame | submit outcome |
| --- | --- | --- |
| `greenhouse.io/reddit` | **present** | blocked |
| `greenhouse.io/clickhouse` | **absent** | accepted |
| `greenhouse.io/akuity` | **present** | **accepted** (code email 23:40:07) |

  reCAPTCHA Enterprise is score-based, so **Akuity carries the widget and submits fine**. The claim was wrong; the caution it defended is now *empirically* proven rather than assumed — a better footing than it had.
  - **The defect that measurement exposed:** Akuity is precisely the breaking case — an accepted submission whose post-acceptance click times out (`element is not enabled`) on a page that *does* carry reCAPTCHA. #104's check sits **above** #93's security-code handling, so it would have labelled an **accepted application** `BLOCKED_CAPTCHA`. That is **#102's own rule — acceptance outranks any rejection signal — reintroduced by the fix written after learning it.** The condition now tests `DetectSecurityCodeChallenge` directly instead of relying on ordering, with a test pinning both directions.
- **Cohort scope, measured:** only **2 of 82** jobs are Reddit (the captcha-blocked board). The bulk is **Lever 39**, other Greenhouse 30, other 11. Greenhouse acceptance is demonstrably working, so the reCAPTCHA limit is narrow and board-specific rather than platform-wide.

### OPEN RISK TO THE WHOLE EFFORT: bot protection is widespread, on BOTH platforms (2026-07-26 01:45)

**This supersedes my earlier "only 2 of 82 are Reddit" reassurance, which was too narrow.** Four distinct boards were confirmed blocked within 40 minutes, and both #99 and #101 labelled every one correctly:

| board | platform | provider | detected via |
| --- | --- | --- | --- |
| `greenhouse.io/reddit` | Greenhouse | reCAPTCHA Enterprise | #101 (click timeout) |
| `greenhouse.io/orkes` | Greenhouse | reCAPTCHA Enterprise | #99 (budget exhaustion) |
| `greenhouse.io/alphasense` | Greenhouse | reCAPTCHA Enterprise | #99 (budget exhaustion) |
| `jobs.lever.co/dexcarehealth` | Lever | **hCaptcha** | #101 (click timeout) |

Orkes and AlphaSense are the cleanest cases yet: **`invalid fields: 0`** — the form fully satisfied — then the full 15s budget with no confirmation and no rejection, on a page carrying reCAPTCHA. Exactly the state #95's budget branch was written to represent honestly and #99 was written to explain.

**Lever probe:** all four additional Lever boards sampled (`Instrumentl`, `LuminDigital`, `agile-defense`, `eneba`) carry hCaptcha on the apply form. Lever is **39 of the 82** cohort jobs.

**What is NOT established, and it matters.** Presence is not blocking — **Akuity carries reCAPTCHA and its submit was accepted** (code email 23:40:07), and ClickHouse carries none. These systems are score-based. So the sampled Lever boards are *suspect*, not proven blocked, and claiming otherwise would repeat the #104 mistake exactly.

**Honest scoreboard for tonight's ~12 attempts:** 4 confirmed captcha-blocked, 2 confirmed accepted-and-awaiting-code (ClickHouse, Akuity), the rest still working through. **The pipeline's real ceiling is bot protection, not form-filling** — the fill machinery now reaches `invalid fields: 0` routinely.

**This is the user's call, not mine.** If bot protection blocks a large share of the cohort, the only remedy is a paid solver (`improvements_paywall.md` #17), which is explicitly out of scope under this session's no-monetary-cost constraint. Worth deciding before investing further in fill-path work, which is no longer the bottleneck.

**Checked and dismissed:** the cohort monitor briefly reported `BLOCKED_CAPTCHA=2` after having reported `4`, which would mean rows leaving that status — nothing in the agent does that. Verified directly: **one** agent process (`4059119`), and the DB consistently holds 4, on exactly the four boards above. A transient read in the polling script, not a status regression, and not the orphan-process trap.

- **bugs.md #105 (Major) — `a8b77dd`.** The most expensive failure mode in the pipeline, recurring after #83 was meant to have closed it. `Remote` sent a **30,477-char** payload — comfortably inside #83's 40,000 ceiling — and burned the **entire 45-minute Ollama timeout** (01:46:03 → 02:31:03) before failing. #83 derived its ceiling from *reading* cost (~17.5 chars/s), which accounts for the prompt going in but not the answers coming out, and `SolveValidationErrors` must generate a value for **every** rejected field. Three live points separate on field count, not size: ClickHouse 11,140/3 → ~7 min ✓; Reddit 18,639/13 → ~15 min ✓; **Remote 30,477/34 → 45 min ✗**. Remote's payload is 1.6× Reddit's, its field count 2.6×, and it did not merely take longer — it never finished. `exceedsRetryTimeBudget` adds a **20-field** ceiling and the character ceiling drops **40,000 → 28,000**, below the observed failure. 3 new tests, one pinning the ceiling under the measured value so a future widening has to argue with the data.
  - **Retry path only.** `ExtractFormMapping` is not answering a list of rejected fields, so the field-count reasoning does not transfer; tightening it without evidence would refuse forms that currently work.
- **Restarted (25th) at 02:37 for #104/#105.** PID `4092461` (`verify84f`), 59 queued, key strings verified by `strings`. Sporty Group had only just begun scoring, so almost nothing was lost. **Remote requeued** with dedup cleared — it lost 45 minutes through no fault of its own. Monitor `bvb6n83wo`.

  - **Confirmed live at 02:47, on the same job that exposed it.** Remote, identical 34 fields and identical 19,481-char payload: narrowing at `02:47:28`, refusal at `02:47:30`. **45 minutes → 2 seconds**, `MANUAL_REQUIRED` with the tailored documents preserved instead of a `context deadline exceeded` that preserved nothing.

- **bugs.md #106 (Major) — `5507d6e`. The third shape of one defect.** Live: `Validation fix for "question_8242451101[]_54236360101" failed: selector matched no element (**tried 1 form(s)**)`. That count is the tell — a bare identifier normally gets five candidate forms, a `tag#id` selector three. Greenhouse names checkbox-group controls with `[]` in the id, and the brackets alone make `looksLikeCSSSelector` true, so the bare-identifier fallbacks are skipped; `splitTagID` then finds no `#`, so that branch adds nothing either. **Simultaneously too CSS-like for one path and not CSS enough for the other**, falling through both with zero fallbacks — and not valid CSS for an id, so the verbatim attempt matched nothing. **#73** fixed `input#430`, **#92** fixed `#question_...[]_...`, this is the bare form with no prefix. Fix appends attribute forms built from the whole selector; a test pins that `input[type='email']` still resolves first and tries nothing else. Verified failing against the old code first.
- **bugs.md #107 (Major) — `f8d549c`. Only visible because #97 logs the value.** `1 fix(es) reported success but left the control empty ... (tried "No")`. `applyValidationFix` handles checkboxes correctly — a negative calls `Uncheck`, with a comment that an explicit negative must not silently tick it — so `"No"` did the right thing. Then `verifyFixLanded` re-read the control with the generic *does it hold a value* check, saw `checked=false`, reported **not landed**, and routed to `ErrUncommittableField` → manual review. **A correct answer recorded as a failure, on every checkbox the model declines.** The verification knew what the control holds but never what was *asked for*; for a deliberately-unticked box those are identical and mean opposite things. `isNegativeCheckboxValue` is now the single source of truth for both the action and its verification so they cannot drift apart again. A test pins that `"Nope, I have no objection"`, `"none of the above"` and `"November"` are not negatives — exact match only, since a substring rule would silently untick real answers.
  - **Third instance this session of a check reading state without the intent behind it**: #102's stale `aria-invalid`, #103's `id|label`, now this. The common repair each time was to give the check access to what was *intended*, not just what is *present*.
  - Sporty Group is the cost: **11 invalid → 3**, then manual review over a checkbox it had answered correctly. Safe (documents preserved, field and value named) but wrong. **Requeue it after the next restart — #106 and #107 both target this exact job.**

- **bugs.md #108 (Major) — `c76e66f`.** Ethos reached **`invalid fields: 0`** — fully satisfied — exhausted the settle budget with **no bot-protection frame**, and was written up as `form content exceeds the local model's context window`. Every part of that is misleading: the form was complete and its narrowed payload had been **1,491 chars**. The submit produced no outcome, narrowing then found nothing to narrow, the code fell back to the whole 43,672-char document, and #105's ceiling refused it — **the size check was simply the last thing to touch the job, so it named the outcome.** `ErrSubmitProducedNoOutcome` now fires when nothing is flagged invalid *and* the previous verdict was budget exhaustion, before the fallback runs (which also saves a wasted inference cycle). Registered in `manualReviewErrors`; **verified by reading `cmd/agent`, not by trusting the build**, that it reaches #84's catch-all and logs its own accurate text.
  - **Genuinely distinct from #99/#104.** Those cover this state *when a provider frame is present*. Here the inbox showed **no Greenhouse email** and the page carried **no frame**, so neither explanation applies and **the true cause is still unknown**. It gets its own name rather than borrowing one that fits badly — the same reasoning that made #83's correct-about-size, wrong-about-cause diagnosis cost a day until #93 reframed it.

- **bugs.md #109 (Major) + #110 (Blocker) — `6a426fc`. #110 is the most consequential find of the session and it was not in any log** — a test written for #109 caught it.
  - **#109:** probed Sporty Group after #106/#107 made its last fields settable and it *still* rejected them. It renders a **single-choice question as three checkboxes sharing one `name`** — `Yes` / `No` / `Prefer not to say`. A model value of `"No"` means *tick the box labelled No*; `applyValidationFix` read it through the standalone-checkbox rule and **unticked** that box, so the group stayed empty and the form could never validate. Now resolved through `pickComboboxOption`, so #79's never-commit-the-wrong-entry rule covers checkbox groups and an unmatched value ticks **nothing**. `verifyFixLanded` checks the *matched* option, usually a different element than the selector resolved to.
  - **#107 made the check more correct and the outcome less useful, and only the live re-run showed it.** Before it, the wrong action read as not-landed and Sporty Group reached `MANUAL_REQUIRED` with documents preserved and the field named; after it, the same wrong action reported as **landed**, `lastNotLanded` was empty, and the job degraded to a bare `FAILED_SUBMIT`. Worth keeping as a caution about adjacent effects.
  - **#110:** `pickComboboxOption` matched by raw bidirectional `strings.Contains`. Normalisation strips punctuation but not word boundaries, so **a short label hides inside longer prose**: `"no"` sits inside `"prefer **no**t to say"`, and asking for **"Prefer not to say" selected the box labelled "No"**. `"male"` inside `"female"` is identical in shape. **On an EEO question that converts a declined answer into a substantive one submitted under the user's name** — the exact failure #79 exists to prevent, inside the function that enforces it. `optionTextMatches` now compares **whole words in sequence**; every match the old rule was written for survives with a test each, and **all six pre-existing `pickComboboxOption` tests pass unchanged**, which is the evidence the looseness was never needed.
- **Restarted (27th) at 04:09 for #109/#110, immediately rather than at a break.** #110 could commit a materially wrong EEO answer on a real application — outward-facing harm, the standard that justified the urgent restarts for #82 and #102. PID `4125158` (`verify84h`), 58 queued; verified the loose substring matcher is gone from the source and the group-refusal string is in the binary. Sporty Group and Ethos requeued. Monitor `bjejz88wb`.

### #108 confirmed live, and Ethos is a genuinely separate category (2026-07-26 04:48)

```
04:48:49 Submit verdict after 15.3s: no confirmation and no rejection ... invalid fields: 0
04:48:50 Ethos needs manual completion — queued for manual submission:
         form is fully filled but the submit produced no confirmation and no rejection
```

The accurate reason replaces `form content exceeds the local model's context window`, routed via #84's catch-all with documents preserved — and it returned in **one second** instead of spending a third ~10-minute model call on a form with nothing left to fix.

**Ethos is the one job that does not fit either explanation.** Fully filled (`invalid fields: 0`), **no** bot-protection frame on the page, **no** Greenhouse email, and the submit goes nowhere. Distinct from the five captcha-blocked boards and from the accepted-awaiting-code pair. **Its cause is still unknown** — #108 gives it a name and a countable sentinel so a second instance is recognisable, which is the whole point of shipping the diagnostic before the explanation.

### DEFINITIVE: Sporty Group reached a complete form and was stopped only by reCAPTCHA (2026-07-26 04:32)

The single clearest result of the session, on the job that has been its best diagnostic all night:

```
04:16:16 Narrowed ... still invalid: <11 fields>
04:31:45 Submit verdict after 8s: ... invalid fields: 1        <- only gdpr_processing_consent_given_1
04:32:32 Submit verdict after 15.5s: no confirmation and no rejection ... invalid fields: 0
04:32:32 Sporty Group is behind a bot-protection challenge — marked BLOCKED_CAPTCHA
         (submit produced no outcome; https://www.recaptcha.net/recaptcha/enterprise/anchor present)
```

**11 → 1 → 0.** The narrowed payload collapsed **6,389 → 631 chars**. Inbox checked: no Greenhouse email, so the submit never reached the server.

**What this proves.** The pipeline can now completely fill a hard Greenhouse form — 11 required custom questions spanning a **checkbox group** (#109), **EEO comboboxes** (#98/#110), a **GDPR consent box**, react-select autocompletes and free text — and reach `invalid fields: 0`. The only thing standing between that and a filed application is **reCAPTCHA**, correctly identified and labelled in 15 seconds instead of the ~30 minutes it used to cost.

**Sporty Group is also the session's clearest illustration of layered defects.** It surfaced **#90, #91, #92, #106, #107, #109 and #110** — each one invisible until the previous was fixed, and the last two only found because the job was re-run after every fix.

**Five boards now confirmed blocked** across both platforms: `reddit`, `orkes`, `alphasense`, `sportygroup` (reCAPTCHA) and `dexcarehealth` (hCaptcha).

### THE DECISION NUMBER: 6 of 7 completed fills were captcha-blocked (2026-07-26 05:10)

Measured from the log, not estimated. **Seven** jobs reached `invalid fields: 0` — a fully satisfied form:

| outcome | count |
| --- | --- |
| **Blocked by bot protection** | **6 boards** — `reddit`, `orkes`, `alphasense`, `sportygroup`, `pointwild` (reCAPTCHA); `dexcarehealth` (hCaptcha) |
| Fully filled, no provider frame, cause unknown | **1** — Ethos (#108) |
| Accepted, awaiting emailed code | 2 earlier — ClickHouse, Akuity (neither carried a blocking challenge) |

**The fill path succeeds on essentially everything it attempts. The submit path is blocked about 6 times in 7.**

**This corrects my own earlier framing twice over.** I first called the captcha scope "narrow" when only Reddit was known, then widened it to four boards while still treating it as a subset. With six of seven completed fills blocked, the honest read is that **bot protection is the normal case on this cohort, not the exception** — ClickHouse is the only board confirmed to carry no challenge at all.

**What follows.** Running the remaining `DISCOVERED` jobs will mostly generate accurate `BLOCKED_CAPTCHA` labels: useful for measuring the true rate, but it will not produce applications. The only two paths that can are (a) ClickHouse and Akuity completing through #102/#32's code-gate path, still untested end to end, and (b) a paid solver (`improvements_paywall.md` #17), which is outside this session's constraint and is now **the deciding factor rather than a nice-to-have**. That call is the user's and is worth making before more compute goes into this cohort.

### Lever escalates from suspect to evidenced: 2 of 2 blocked (2026-07-26 05:18)

Zimperium — one of the three jobs that ended earlier today with a bare `playwright: timeout: Timeout 30000ms exceeded` and **no cause at all** — now reports:

```
05:17:50 Location set to "Macomb, Michigan" on the initial fill        <- improvements #33 working on Lever
05:18:20 Zimperium is behind a bot-protection challenge — marked BLOCKED_CAPTCHA
         (submit click did not land; https://newassets.hcaptcha.com/... present)
```

**#101 retroactively explains one of the day's mystery timeouts**, which is what it was written to do.

**The Lever risk is no longer a hypothesis.** Earlier I deliberately refused to conclude that Lever was broadly blocked from widget presence alone — correctly, since Akuity carries reCAPTCHA and submits fine. That caution is now resolved by outcomes rather than presence: **both Lever jobs attempted (DexCare, Zimperium) were blocked**, on top of all four additional Lever boards sampled carrying hCaptcha. **Lever is 39 of the 82 cohort jobs.**

Seven boards now confirmed blocked. Combined with the 6-of-7 completed-fill figure above, the projection for the remaining queue is that most of it is unreachable without a paid solver.

### ACCEPTANCE IS INTERMITTENT ON THE SAME BOARD (2026-07-26 06:19)

Akuity's submit history tonight, cross-checked against the mailbox each time:

| submit | code email | accepted? |
| --- | --- | --- |
| 23:40:06 | 23:40:07 (`yN8V0cLO`) | **yes** |
| 05:59:19 | 05:59:19 (`82taTsxA`) | **yes** |
| **06:18:00** | **none** | **no** |

Same board, same form, same binary path — and the third submit was not accepted. **This is not a per-board property.** reCAPTCHA Enterprise is score-based, and the score degrades with repeated automated attempts from the same browser and address.

**Consequences that matter more than any individual fix.**

1. **"Boards that accept" is not a stable category.** I have been reasoning in those terms all session — ClickHouse and Akuity as "the two that can accept" — and that framing is wrong. Acceptance is probabilistic per attempt.
2. **Repeated attempts make it worse, not better.** Every requeue-and-retry cycle tonight has been lowering the score it depends on. The 27 restarts were necessary for the fixes, but they also degraded the thing being tested.
3. **It strengthens the case for the paid solver** (`improvements_paywall.md` #17) rather than weakening it: there is no configuration of free retries that makes a score-based challenge reliable.

**#111 confirmed working in the negative direction, which is the more important one.** It correctly did *not* claim acceptance when no code arrived — a false positive there would enter a stale code and record an application as filed when it was not. The 12-second gap between `Attempt 2: Solving validation errors...` and the narrowing line is the IMAP round-trip, so the check demonstrably ran.

### Where the ceiling actually is now

The fill path is essentially solved: `invalid fields: 0` is reached routinely, on Greenhouse and Lever alike. What remains is **two ceilings, neither of them form-filling**:

1. **Bot protection** — 4 boards confirmed blocked across both platforms; needs the user's decision on a paid solver (`improvements_paywall.md` #17), which is out of scope here.
2. **Local model throughput on large forms** — #105 now fails fast into `MANUAL_REQUIRED` with documents preserved, rather than burning 45 minutes to preserve nothing.

Further fill-path engineering has diminishing value while those two stand.

### The session's dominant pattern, stated plainly

Four of this session's fixes were defects in *earlier fixes from the same session*: **#95 → #102**, **#98 → #103**, **#104 → its own follow-up**, and #96/#97/#100 were each written because the previous diagnostic could not see the next failure. The recurring cause is always the same — **a signal that looks like evidence is really residue of the previous step** (#76's `el.value`, #81's `[data-value]`, #102's `aria-invalid`, #103's `id|label`). The defence that has actually worked is not more care up front; it is shipping the diagnostic *before* the root cause is known, which has now paid off four times in a row within a single cycle each.

### Investigated and dismissed

- **`Location set to` appears only 2× today against `Country set to` 13×.** Looked like location was silently failing on Greenhouse. It is not: `candidate-location` and `country` appear in **no** recent `still invalid:` list, and the only two `left the control empty` hits for them are at 15:39/15:49 — *before* #78/#79 shipped at ~15:52. The forms since simply do not flag location as outstanding. Benign absence; not filed.

## Run restarted onto the fixes

Killed PID `3755906`, confirmed dead, confirmed zero other agent binaries running. Audited the cohort rather than blanket-resetting: of 7 `FAILED_SUBMIT`, 5 are the known-dead postings (Netcraft, NABIS, Postscript, Sphinx Defense, chownow) — left untouched. Requeued exactly **Reddit** (#70's victim) and **Zimperium** (#71's victim), both with `-clear-dedup`, or `HasApplied` would have silently skipped the retry.

**Restarted (19th) at 21:16 for #32/#33** — PID `3978328` (`verify83c`), 67 queued, `Security-code retrieval enabled (IMAP)` confirmed in the startup log. Orkes requeued. Monitor `bx3r0ftfc`.

**Restarted (16th-18th) at 20:50 / 20:54 / 20:55** for the attestation values, then #93, then the user's fuller `pii.yaml`. Current: PID `3967109` (`verify83b`), 66 queued. Monitor `biss0etj6`. Orkes deliberately left in `MANUAL_REQUIRED` pending the user's inbox check (#89 duplicate risk).

**Restarted a fifteenth time at 20:34 for #91/#92.** PID `3957547` (`verify82y`), 63 queued. Sporty Group requeued again (scoped SQL, dedup cleared) — both of its remaining blockers are now addressed. Monitor `byoj0ahb0`.

**Restarted a fourteenth time at 20:10 for #90.** PID `3949037` (`verify82x`), 63 queued. Sporty Group requeued via scoped SQL (`cmd/requeue` does not accept `MANUAL_REQUIRED`) with its dedup row cleared, so it gets a real shot at completing. Monitor `braa9h8xb`.

**Restarted a thirteenth time at 19:47, urgently, for #89** — the risk it prevents is duplicate applications filed with real employers, which justifies the throughput cost the way #82 did. PID `3941348` (`verify82w`), 63 queued; verified the binary contains #89's re-check string. Monitor `bnqkrjy0n`.

**Restarted a twelfth time at 19:25 for #88.** PID `3932293` (`verify82v`), 64 queued. Monitor `btn6exv6c`.

**Restarted an eleventh time at 19:06 for #85/#86/#87** — the most consequential build of the day, and the first in which the submit path is end-to-end plausible. PID `3923685` (`verify82u`), 65 queued, 4 stale `PROCESSING` reaped. Orkes and Nova requeued (both failed purely on #87/#86). Monitor `b9udcon8b`.

**Restarted a tenth time at 18:01 for #83/#84.** PID `3898303` (`verify82t`), 68 queued, 3 orphaned `PROCESSING` reaped. ClickHouse requeued so it routes to `MANUAL_REQUIRED` properly. Monitor `b1ieftjwi`.

**Restarted a ninth time at 16:52 for #82** (urgent — the previous binary had #81 without the guard). PID `3878906` (`verify82s`), 69 queued. Monitor `b2pa3w306`.

**Restarted an eighth time at 16:43 for #81.** PID `3873741` (`verify82r`), 69 queued. Monitor `bgpgrt6y8`.

**Restarted a seventh time at 16:18 for #80's diagnostic.** PID `3864537` (`verify82q`). Queue loaded **69**, not 71 — Reddit was left in `FAILED_SUBMIT`; requeued afterwards so it is ready for the next restart (the queue is a startup snapshot). Any other Greenhouse job in the queue produces the same diagnostic. Monitor `bqin2f51s`.

**Restarted a sixth time at 15:52 for #78/#79 + improvements #29.** Killed PID `3820102`, cleared Reddit's dedup row and requeued it, rebuilt to `/tmp/career_agent_bin_verify82p`, **PID `3855561`**. Monitors `bt1j2yfpn` (cohort) and `bdwa415i5` (log).

**Restarted a fifth time at 14:21 for #77 + improvements #28.** Killed PID `3806979`, cleared Reddit's dedup row, rebuilt to `/tmp/career_agent_bin_verify82o`, **PID `3820102`**, `loaded 71 matching job(s)`. Monitors `b4y50ebc2` (cohort) and `bdwa415i5` (widened log filter).

**Restarted a fourth time at 13:52 for #76** — #74 and #75 are provably inert without it, so the in-flight attempt 3 could not succeed and there was nothing to wait for. Killed PID `3797112`, cleared Reddit's dedup row again, rebuilt to `/tmp/career_agent_bin_verify82n`, **PID `3806979`**, `loaded 71 matching job(s)`. Monitor `b0iokt81r`.

**Restarted a third time at 13:28 for #74** — the running binary could not pass Reddit's form at all (Location/Country required and uncommittable), so its remaining ~24 min of retries were guaranteed to fail. Killed PID `3791768`, confirmed dead. Reddit had a fresh `applied_jobs` dedup row, deleted via scoped DELETE or `HasApplied` would have silently skipped the retry; `PROCESSING` was reaped on startup. Rebuilt to `/tmp/career_agent_bin_verify82m`, **PID `3797112`**, `loaded 71 matching job(s)`. Monitor `bswuoo0oh`.

**Restarted at 13:16 for #72/#73** — those affect every Greenhouse form with numeric custom-question ids, so leaving the old binary running would have burned ~12 min per attempt on the identical failure. Killed PID `3778859`, confirmed dead and sole. Requeued Reddit again (`-clear-dedup`). Rebuilt to `/tmp/career_agent_bin_verify82l`, **PID `3791768`**, healthy: reaped 2 orphaned `PROCESSING` rows (Akuity, You.com), `loaded 71 matching job(s)`, RAG 9 chunks, restarted on Reddit. Cohort: `DISCOVERED=70 FAILED_SUBMIT=5 PROCESSING=1 SKIPPED=6`. Monitor `bjyegqfsv`; log monitor `bbk8sss42` (widened to catch #70-#73's diagnostics).

Rebuilt from HEAD to `/tmp/career_agent_bin_verify82k`. **PID `3778859`** (superseded), confirmed healthy at 12:43: reaped the 1 orphaned `PROCESSING` row, `loaded 71 matching job(s)`, RAG found 9 career chunks (bug #58's fix persists), and started on **Reddit** — the exact job #70 killed. Cohort at relaunch: `DISCOVERED=70 FAILED_SUBMIT=5 PROCESSING=1 SKIPPED=6` (82 total).

Monitors re-armed for the new PID: `b8wgiddyn` (cohort status + PID death). The log monitor `bpsfomclo` survived the restart since it tails the file, not the process.

## Open investigation: the validation-retry loop does not converge (RESOLVED — this became #70)

Evidence from `career_agent.log`:

```
12:15:03 Submission failed validation. Retrying...
12:15:03 Attempt 2: Solving validation errors...
12:15:03 Narrowed validation retry to the rejected fields only (53366 -> 5363 chars)
12:27:16 Submission failed validation. Retrying...
12:27:16 Attempt 3: Solving validation errors...
12:27:16 Narrowed validation retry to the rejected fields only (53228 -> 5439 chars)
12:32:28 Submission failed validation. Retrying...
12:32:28 Auto-Submit failed for Reddit: failed to submit application after 3 validation error attempts
```

Signals that this is a real defect, not just a hard form:
1. The page DOM barely changed between attempts (53366 → 53228 chars) and the narrowed slice *grew* (5363 → 5439) — consistent with the same fields being rejected each time, i.e. the LLM's fix is not landing.
2. **Nothing logs *which* fields were rejected or what the model tried to set them to.** That is itself a gap — it makes this class of failure undiagnosable from logs alone, and this is the single most expensive failure mode in the pipeline (~6 min per wasted attempt).

## LIVE CONFIRMATION — the combobox chain works (2026-07-25 16:02)

First successful combobox commit in the live agent, on PID `3855561`:

```
16:01:58 Location set to "Macomb Township, MI" on the initial fill (saved a validation-retry cycle)
16:02:18 Country set to "United States of America" on the initial fill (saved a validation-retry cycle)
16:02:18 Submission failed validation. Retrying...
16:02:18 Narrowed validation retry to the rejected fields only (54537 -> 7212 chars)
```

This closes out the whole #74 → #75 → #76 → #77 → #78 → #79 chain, and improvements #28's initial-fill path with it. Both fields were previously **impossible** to satisfy — required, react-select, and uncommittable — so every Greenhouse application was structurally doomed regardless of how many retries ran. The narrowed payload also fell 8249 → 7212 chars, consistent with two fewer invalid fields.

**#74, #77, #78, #79 can now be moved off "live confirmation pending".** #28 is confirmed end to end.

**Still 0 confirmed `APPLIED`** — the remaining blockers are the three unset `pii.yaml` values in the audit below, not code.

## Option-level audit of the blocking questions (2026-07-25 16:20, probe)

Read the actual option lists, rather than assuming what the form would accept:

| Field | Options | Verdict |
| --- | --- | --- |
| `430` gender identity | Agender, Genderfluid, Gender non-conforming, Genderqueer, Female, Male, Non-binary, Not listed, **I don't wish to answer** | **not a blocker** — declinable |
| `433` disability | Yes… / No… / **I don't wish to answer** | **not a blocker** — declinable |
| `434` veteran | 7 statuses / No military service / **I don't wish to answer** | **not a blocker** — declinable |
| `question_67942418` **authorized to work in the U.S.** | **Yes \| No — no decline option** | **HARD BLOCKER** |
| `question_67942419` **requires immigration sponsorship** | **Yes \| No — no decline option** | **HARD BLOCKER** |
| `question_67942416` how did you hear | no options returned — free text | minor; model can answer |

**Two consequences.**

1. **The blank EEO section is fine.** Every demographic question offers "I don't wish to answer", so the model declines correctly and none of them blocks submission. That validates the existing EEO design.
2. **Work authorization and sponsorship are genuinely unanswerable by the agent.** They are required, binary, and offer no decline. There is no honest value the model can select without the user's input — and a guess would place a **false legal attestation** on a real job application under the user's name. This is the strongest possible vindication of leaving those blank in #29 rather than inferring them from a US address.

Filling `work.authorized_to_work_us` and `work.requires_sponsorship` is now the single highest-value action available, and it is the user's to take.

**Probe footnote:** `page.Locator("#430")` returned NO ELEMENT — `#430` is invalid CSS, exactly the defect bugs.md #73 fixed in the agent. The agent resolves it via `[id="430"]`; the probe needed the same treatment. An independent re-confirmation of #73 from a completely different direction.

## The attestation block is cross-ATS, not a Greenhouse quirk (2026-07-25 18:26)

Four consecutive high-scoring jobs, **all refused on the same two questions**, and one of them is not Greenhouse:

| Job | Score | ATS | Blocked on |
| --- | --- | --- | --- |
| Reddit | 90 | Greenhouse | work authorization + visa sponsorship |
| ClickHouse | 90 | Greenhouse | work authorization + visa sponsorship |
| **DexCare** | **90** | **Lever** | work authorization + visa sponsorship |
| Stack AV | 80 | Greenhouse | visa sponsorship only |

I had been describing `work.authorized_to_work_us` / `work.requires_sponsorship` as "the last blocker for Greenhouse". **That was too narrow.** Lever asks them too, so these are standard US hiring questions and the block applies to the pipeline as a whole — including the ~3,100-job backlog behind this cohort, which will behave identically.

Note Stack AV matched **only** `visa sponsorship`, not both: #82's detection is discriminating per form rather than blanket-matching, which was the main false-positive risk.

**Economics:** each refusal still costs ~10 minutes of fit-scoring first, because a form's questions are unknowable until it is loaded, and all LLM calls serialise on this host. So until those two keys are set, the queue converts ~10 minutes of compute per job into a manual-review entry rather than an application.

## Required-field audit of Reddit's form (2026-07-25 15:54, probe, no submit)

Enumerated every control Greenhouse marks required, by reading `aria-required`/`requiredInput` rather than clicking submit — clicking it on a real posting could file an incomplete application under the user's name. **20 required fields.** Mapped against what the agent can now supply:

| Field | Label | Source | Status |
| --- | --- | --- | --- |
| `first_name`/`last_name`/`email`/`phone` | — | `handleGreenhouse` | covered |
| `candidate-location` / `country` | Location (City), Country | improvements #28 + bugs #78/#79 | covered |
| `question_67942415` | LinkedIn Profile | `links.linkedin` (#29) | covered |
| `question_67942417` | Current/most recent company | `work.current_employer` (#29) | covered |
| `question_67942420` | "I agree" acknowledgement | model picks the agree option | expected to resolve |
| `430`-`434`, `436` | gender identity, transgender experience, sexual orientation, disability, veteran, ethnicity | EEO section, intentionally blank | model must pick the decline option |
| **`question_67942416`** | **How did you hear about this job?** | **`work.how_did_you_hear` — BLANK** | **blocking** |
| **`question_67942418`** | **Authorized to work in the U.S.?** | **`work.authorized_to_work_us` — BLANK** | **blocking** |
| **`question_67942419`** | **Require immigration sponsorship?** | **`work.requires_sponsorship` — BLANK** | **blocking** |

**This is the concrete consequence of #29's deliberate blanks.** Two of the three blockers are the legal attestations left for the user by design; the third is a preference. Until they are set, this specific application cannot complete no matter how well the combobox machinery works — the model has no grounded value and is instructed not to fabricate one. Reported to the user with the exact keys.

`gdpr_demographic_data_consent_given_1` is already pre-checked by the page.

## Next Step

**Standing instruction: monitor the live run, fix what surfaces, log it in `bugs.md`, groom the backlog, keep this journal.** The user set this as a `/goal`; it does not complete until they say so. Standing authority: *"If a choice arises, do what you recommend, don't feel you need to ask, do not do anything that adds a monetary cost."*

### Live state (accurate as of 2026-07-26 08:55)

- **NOTHING IS RUNNING.** The targeted 2-job run (PID `5294`) finished at 08:50 — `Batch execution complete!`. No agent process is alive.
- **Working tree clean, `main` in sync with `origin/main`.** HEAD carries **#94-#117** (29 fixes this session).
- **1 confirmed `APPLIED`: Akuity** — the first in the database's history (was 0 across 3,884 rows at session start). See the *FIRST CONFIRMED APPLICATION* section above for the evidence.
- **82-cohort:** `DISCOVERED=52 FAILED_SUBMIT=11 BLOCKED_CAPTCHA=9 SKIPPED=6 MANUAL_REQUIRED=4`. Note this tally reads the `http://` rows and is unreliable for ~11 of 82 jobs — see **#112**.
- **Backlog:** 105 bug rows, **0 open**. `improvements.md` has 3 Pending (all ⚠️ below the 0.5 ROI floor), `improvements_paywall.md` 1.
- **Monitors: none armed.** They do not survive a session boundary. On resume, `pgrep -af watch_82.sh` and `pgrep -af 'tail -F.*career_agent.log'`, kill any orphans, arm fresh ones.

### RECOMMENDED NEXT ACTIONS, in order

#### 1. Restart the full 82-job cohort on current HEAD

Every previous cohort run used a much earlier binary; **no full run has ever executed with #94-#117**. This is the highest-value action and needs no user decision.

```bash
# Build on the HOST (distrobox Go is 1.18 and rejects go.mod's 1.26.5).
cd /var/home/howlcipher/dev/Career_Agent_Core
go build ./... && go vet ./... && go test ./...

# Launch INSIDE the distrobox (the agent renders blank pages on the bare host).
cat > /tmp/relaunch_full.sh <<'EOS'
#!/bin/bash
set -e
cd /var/home/howlcipher/dev/Career_Agent_Core
/usr/local/go/bin/go build -o /tmp/career_agent_bin_full1 ./cmd/agent
echo "BUILD OK"
TARGET_JOB_URL="$(paste -sd, applied_urls_verify82.txt)" \
  nohup /tmp/career_agent_bin_full1 > /tmp/full1_stdout.log 2>&1 &
disown
echo "LAUNCHED PID $!"
EOS
chmod +x /tmp/relaunch_full.sh
distrobox enter career-agent -- bash -lc "bash /tmp/relaunch_full.sh"
```

Then **verify the fixes are in the binary** rather than assuming (#84's lesson):

```bash
for s in "security-input-" "issued a security code after the last submit" \
         "Resubmit after the security code did not confirm" \
         "AVAILABLE OPTIONS FOR DROPDOWN FIELDS" "Submit verdict after"; do
  printf "%-50s %s\n" "${s:0:50}" "$(strings /tmp/career_agent_bin_full1 | grep -cF "$s")"
done
```

Arm both monitors (cohort + log). The cohort watcher, self-contained:

```bash
cat > /tmp/watch_82.sh <<'EOS'
#!/bin/bash
urls_file="/var/home/howlcipher/dev/Career_Agent_Core/applied_urls_verify82.txt"
db="/var/home/howlcipher/dev/Career_Agent_Core/applications.db"
in_clause=$(awk '{printf "%s%s%s", (NR>1?",":""), "\x27", $0"\x27"}' "$urls_file")
pid=<THE_NEW_PID>
prev=""
while true; do
  if ! kill -0 "$pid" 2>/dev/null; then echo "PROCESS $pid EXITED. Final: $prev"; break; fi
  cur=$(distrobox enter career-agent -- sqlite3 "$db" \
    "SELECT status || '=' || COUNT(*) FROM job_funnel WHERE url IN ($in_clause) GROUP BY status ORDER BY status;" 2>/dev/null | tr '\n' ' ')
  if [ -n "$cur" ] && [ "$cur" != "$prev" ]; then echo "STATUS CHANGE (82-cohort): $cur"; prev="$cur"; fi
  sleep 120
done
EOS
```

Log monitor filter (must include the newer diagnostics or their events are invisible — #77):

```
Submission confirmed|Submit verdict after|Auto-Submit failed|Proceeding with application|panic:|CRITICAL:|BLOCKED_CAPTCHA|queued for manual submission|Duplicate check|Batch execution complete|ATS board feeds contributed|validation fix\(es\) to:|Narrowed validation retry|Submission failed validation|committed .*autocomplete|left the control empty|on the initial fill|Security-code gate|Entered the emailed security code|issued a security code|Rejected despite being set|Fillable inputs present|Submit click failed|manual review|failed to record the dedup row
```

**Expect mostly `BLOCKED_CAPTCHA`.** Measured: **6 of 7** completed fills were blocked. That is accurate reporting, not a regression.

#### 2. Merge #112's duplicate funnel rows — needs a decision first

20 scheme-duplicate pairs (`http://x` and `https://x` for one posting), **11 holding different statuses**. The dedup half is fixed; the funnel half is not, because merging requires deciding which status wins when two disagree (`BLOCKED_CAPTCHA` vs `DISCOVERED` vs `FAILED_SUBMIT` are not obviously orderable). Picking wrong either strands a workable job or re-attempts a blocked one. **Ask the user before merging.** Inspect with:

```sql
SELECT b.status AS http_status, a.status AS https_status, COUNT(*) FROM job_funnel a JOIN job_funnel b
  ON replace(a.url,'https://','') = replace(b.url,'http://','')
 AND a.url LIKE 'https://%' AND b.url LIKE 'http://%' GROUP BY 1,2 ORDER BY 3 DESC;
```

#### 3. The bot-protection decision — the user's, and now the dominant constraint

**6 of 7 completed fills were blocked**, across both platforms: `reddit`, `orkes`, `alphasense`, `sportygroup`, `pointwild` (reCAPTCHA) and `dexcarehealth`, `zimperium`, `brightedge`, `syw` (hCaptcha on Lever — **4 of 4 Lever jobs attempted were blocked**, and Lever is 39 of 82). The fill path is solved; this is what stops applications. The only remedy is `improvements_paywall.md` **#17** (2captcha/capsolver), which is **paid and explicitly out of scope** under the no-monetary-cost constraint. **Do not act on this without the user.**

**Do NOT "optimise" by pre-skipping jobs whose page carries a captcha widget** — see the ⚠️ note in `bugs.md`'s Operational Trap section. Akuity carries reCAPTCHA *and* was accepted; presence is not blocking, and a presence-based skip recreates #45/#46.

### OPEN ITEMS FOR THE USER (do not decide unilaterally)

1. **ClickHouse has an accepted application awaiting its code** — Greenhouse emailed `p5Kqsn22` at 08:48:11. One code entry from complete. The agent missed it via #117 (now fixed). Completing it by hand, or requeuing it, is the user's call.
2. **Akuity received a DUPLICATE application** (08:01 and 08:32). Caused by an undetected success (#116) plus my requeue loop. #116 prevents recurrence; the duplicate cannot be undone. See the disclosure section above.
3. **~59 DB-wide `applied_jobs` rows predate the log**, on `DISCOVERED` jobs. Under #94 they are probably bogus, and clearing them would return a large block of the ~3,100-job backlog to circulation — but there is no positive evidence per row, and clearing one where an application landed files a duplicate.
4. **Per #110, check the EEO answers** on any pending application completed by hand. The loose option matcher was live for most of the session and could have selected a substantively wrong answer.

### STANDING PROCEDURE AND HARD-WON WARNINGS

- **Build on the HOST, run in the DISTROBOX.** Host Go is 1.26.5; the distrobox's is 1.18 and rejects `go.mod`. `sqlite3` and Playwright only work inside the distrobox.
- **Restart procedure:** kill the PID (verify dead — plain `kill` often does not stick, use `kill -9`), requeue affected rows, **clear the `applied_jobs` dedup row** or `HasApplied` silently skips the retry, rebuild with a bumped binary suffix, re-arm monitors against the new PID.
- **Requeue by the scheme-normalised key** (#112), or you will update only one of two rows:
  `DELETE FROM applied_jobs WHERE replace(replace(url,'https://',''),'http://','') IN (...)`.
  The DB stores `https://` while the log prints `http://` — a scoped `UPDATE` written from the log's spelling matches **zero rows and silently does nothing**. Always include a verification `SELECT` in the same statement.
- **Before requeuing a job that reached code entry, CHECK THE INBOX for a completion email.** That is the only place an undetected success is visible, and skipping it is what filed the Akuity duplicate.
- **Every signal this pipeline depends on arrives late. Never read one once.** Six bugs this session were exactly that: #95 (DOM before the submit landed), #102 (previous attempt's `aria-invalid`), #111 (unrendered gate), #113 (unrendered field), #116 (instant resubmit verdict), #117 (unindexed mailbox).
- **Ship the diagnostic before the root cause is known.** #80, #96, #97, #100 and #114 each found their cause within *one* cycle — #114 dumped the page's inputs and directly produced #115's unguessable eight-box widget. Guessing here costs a real application per attempt.
- **The inbox is ground truth; the page is not.** When they disagreed, the mailbox was right every time (#93, #95's proof, #111, #117, the duplicate).
- **Fixing one path is not fixing the capability.** Five instances (#65/#66→#67, #74→#75, #28→#31, #98's two prompt paths, #95→#116). `TestNoUnpolledPostClickConfirmationChecks` now guards the submit-verdict case structurally, because memory failed five times.
- **Probe the real page instead of iterating through the agent** — ~30s per hypothesis versus ~12 minutes. Build in the scratchpad with a `replace` back to the repo module, pin `playwright-go` to the repo's version, run inside the distrobox. **Never let a probe click submit on a live posting.**
- **Replicate the code path, not the expected outcome** (#76, #81, #91 all shipped inert because the probe reproduced my mental model of the sequence, not the code's).
- **Verify an edit landed, not just that the build passed** (#84), and **verify fixes are in the binary** with `strings` before trusting a run.
- **An absent signal is not evidence of an absent event** (#77, #84, #81) — and its inverse, **a benign-looking log line is not evidence of a benign event** (#94's `Duplicate check: Already applied`).
- **A flaky external service needs a clean re-test before its answer becomes fact** (#88's "Lever cannot find Macomb, MI" was rate-limiting, corrected in #33).
- **Acceptance is per-attempt, not per-board.** reCAPTCHA is score-based and the score degrades with repeated automated attempts. Akuity accepted, then didn't, then did. Do not reason in terms of "boards that accept".
- Ollama serves warm prompt caches — benchmark only on unseen jobs. `OLLAMA_FAST_MODEL` is intentionally unset (improvements #24).

### NOT YET EXERCISED

**improvements #26 (ATS feed discovery) still has not run live.** Every run this session used `TARGET_JOB_URL`, which skips `FunnelEngine.DiscoverJobs` entirely. `discoverWithATSFeeds` only executes on a *normal* batch start — watch then for `ATS board feeds contributed N new posting(s)` and check the title gate is not over-filtering.
