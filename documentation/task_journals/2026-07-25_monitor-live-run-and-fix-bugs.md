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
- **Backlog groomed:** dated groom-pass note added to `bugs.md`. No re-ranking was warranted — all bug rows Resolved, both remaining `improvements.md` Pending rows below the ROI floor.
- **Cleaned up 5 stale leftover monitor shells** (PIDs 2166044/2295635/2368407/2476472/2543238) from long-disconnected sessions, all targeting confirmed-dead PIDs 2165142-2542429. The 07-21 journal predicted they would self-terminate; they had not, and each held a `tail -F` on the log. Verified every target PID dead before killing.

## Run restarted onto the fixes

Killed PID `3755906`, confirmed dead, confirmed zero other agent binaries running. Audited the cohort rather than blanket-resetting: of 7 `FAILED_SUBMIT`, 5 are the known-dead postings (Netcraft, NABIS, Postscript, Sphinx Defense, chownow) — left untouched. Requeued exactly **Reddit** (#70's victim) and **Zimperium** (#71's victim), both with `-clear-dedup`, or `HasApplied` would have silently skipped the retry.

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

## Next Step

**Keep monitoring PID `3778859` and keep fixing what surfaces.** That is the standing instruction for this session; it does not complete until the user says so.

The immediate question is now sharply posed: **Reddit (90) and Zimperium (85) are both back in the queue running against #70's and #71's fixes.** Reddit is being processed first (started 12:43:52). Watch for:
- `Attempt N applied X/Y validation fix(es) to: <selectors>` — new in #70. If the *same* selectors recur across attempts, the fix still is not landing and the diagnosis was incomplete.
- `found N submit control(s) but none visible` — new in #71. If Zimperium produces this, the real question ("why does Lever show no visible submit control?") is finally askable, and that becomes the next bug.
- `Submission confirmed for ...` — the first genuine `APPLIED` of the whole 82-job effort, still 0 as of this writing.

**Do not restart PID `3778859`** unless there is a new fix worth picking up. It carries everything through `4234fba`, including #69 and #26, which the previous run did not.

**Note #26 (ATS feed discovery) still has not run live** — this is an isolated `TARGET_JOB_URL` run, which skips `FunnelEngine.DiscoverJobs` entirely. `discoverWithATSFeeds` only executes on the next *normal* batch start. Watch then for `ATS board feeds contributed N new posting(s)` and check the title gate is not over-filtering.

**Standing warnings carried forward:** monitor liveness is best checked by reading the monitor's own output file, not `TaskList`; Ollama serves warm prompt caches so benchmark only on unseen jobs; `OLLAMA_FAST_MODEL` is intentionally unset (improvements #24).
