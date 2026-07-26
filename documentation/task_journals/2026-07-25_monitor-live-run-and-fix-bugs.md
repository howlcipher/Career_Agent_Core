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
- **Backlog groomed:** dated groom-pass note added to `bugs.md`. No re-ranking was warranted — all bug rows Resolved, both remaining `improvements.md` Pending rows below the ROI floor.
- **Cleaned up 5 stale leftover monitor shells** (PIDs 2166044/2295635/2368407/2476472/2543238) from long-disconnected sessions, all targeting confirmed-dead PIDs 2165142-2542429. The 07-21 journal predicted they would self-terminate; they had not, and each held a `tail -F` on the log. Verified every target PID dead before killing.

## Run restarted onto the fixes

Killed PID `3755906`, confirmed dead, confirmed zero other agent binaries running. Audited the cohort rather than blanket-resetting: of 7 `FAILED_SUBMIT`, 5 are the known-dead postings (Netcraft, NABIS, Postscript, Sphinx Defense, chownow) — left untouched. Requeued exactly **Reddit** (#70's victim) and **Zimperium** (#71's victim), both with `-clear-dedup`, or `HasApplied` would have silently skipped the retry.

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

**Keep monitoring PID `3778859` and keep fixing what surfaces.** That is the standing instruction for this session; it does not complete until the user says so.

The immediate question is now sharply posed: **Reddit (90) and Zimperium (85) are both back in the queue running against #70's and #71's fixes.** Reddit is being processed first (started 12:43:52). Watch for:
- `Attempt N applied X/Y validation fix(es) to: <selectors>` — new in #70. If the *same* selectors recur across attempts, the fix still is not landing and the diagnosis was incomplete.
- `found N submit control(s) but none visible` — new in #71. If Zimperium produces this, the real question ("why does Lever show no visible submit control?") is finally askable, and that becomes the next bug.
- `Submission confirmed for ...` — the first genuine `APPLIED` of the whole 82-job effort, still 0 as of this writing.

**Do not restart PID `3778859`** unless there is a new fix worth picking up. It carries everything through `4234fba`, including #69 and #26, which the previous run did not.

**Note #26 (ATS feed discovery) still has not run live** — this is an isolated `TARGET_JOB_URL` run, which skips `FunnelEngine.DiscoverJobs` entirely. `discoverWithATSFeeds` only executes on the next *normal* batch start. Watch then for `ATS board feeds contributed N new posting(s)` and check the title gate is not over-filtering.

**Standing warnings carried forward:** monitor liveness is best checked by reading the monitor's own output file, not `TaskList`; Ollama serves warm prompt caches so benchmark only on unseen jobs; `OLLAMA_FAST_MODEL` is intentionally unset (improvements #24).
