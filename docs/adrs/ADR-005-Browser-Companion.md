# ADR-005: Normal-Browser Career Agent Companion

## Status

**Proposed — design only. No extension code exists in this repository.**

This ADR records the contract a browser companion would have to satisfy before
it is built, so that the decision is made once, in the open, rather than
implicitly by whatever the first implementation happened to do. It is filed
alongside `improvements.md` #536, which tracks the work itself.

## Context

Some ATS platforms cannot be served by the assisted browser at all. Lever is the
concrete case: bug #520 recorded that applications completed in the guarded
Playwright browser are rejected with "There was an error verifying your
application", while the identical application submitted from the operator's
ordinary browser succeeds. Career Agent does not attempt to disguise the
assisted browser to get past that — `storage.AssistedBrowserRejectionReason`
detects those platforms and `actionForOperatorBrowser` routes the operator to
their own browser instead.

That routing is correct and stays. But today it is also where Career Agent's
help stops: the operator gets a URL and a prepared résumé, and fills the form by
hand. The Copy Application Packet (`/api/assisted/packet`) narrows that gap, and
a browser companion would close it — the operator's own authenticated browser,
with their own session and their own cookies, assisted locally by the same
prepared application packet.

The goal is explicitly **not** to make automation harder to detect. It is the
opposite: use the real browser, as the real person, and let Career Agent supply
the preparation rather than the identity.

## Decision

If built, the companion must satisfy all of the following. These are
requirements, not preferences.

1. **Local only.** The extension communicates with `http://127.0.0.1:<port>` and
   nothing else. No remote endpoint, no telemetry, no analytics, no update
   channel that carries page or application data.

2. **Paired, not ambient.** The dashboard mints a per-session token which the
   operator pastes into the extension once. Every request carries it. A
   localhost port with no authentication is reachable by any page in the
   browser, which would make the operator's PII readable by any site they visit.

3. **Origin allowlist on both sides.** The Go side accepts requests only from
   the extension's own `chrome-extension://<id>` origin, in addition to the
   existing same-origin dashboard guard. The extension talks only to the paired
   local port.

4. **Structure out, values in.** The extension sends a *sanitized field
   descriptor* — the same shape `submitter.FormControl` already defines: key,
   label, control type, options, required, and whether the control is empty. It
   never sends page text, page HTML, cookies, headers, URLs beyond the origin,
   or the values already in the form. The local service replies with values for
   the fields it can resolve and a list of the ones it cannot.

5. **No submit verb exists.** The protocol has no operation that clicks a
   button. Not a discouraged one, not a gated one — the message schema simply
   does not contain it, so an extension bug, a compromised page, or a future
   contributor cannot reach one. The operator clicks the employer's Submit
   control themselves, in their own browser, as they do today.

6. **Confirmation stays human and explicit.** The extension may offer a "I saw a
   confirmation" action that calls the existing `/api/assisted/confirm`
   endpoint, which already requires an explicit acknowledgement and records
   `manual_user_confirmation` provenance. It must never infer success from a URL
   change, a page's contents, or a tab closing — the same rule
   `PauseApplySessionForClosedBrowser` already enforces on the assisted path.

7. **Reuses the existing model.** Answer resolution goes through
   `answers.Store.Resolve`, so sensitive categories behave identically to the
   assisted path: proposed, never auto-filled, never learned without two
   explicit acknowledgements. The companion adds a surface, not a second policy.

## Consequences

**Positive**

- The ATS platforms Career Agent deliberately refuses to automate stop being the
  ones where it helps least.
- The operator's real browser session removes an entire class of failure
  (verification rejections, bot-protection friction) without any evasion.
- The security boundary is unchanged in kind: local-only, explicit approval,
  human submit.

**Negative**

- A browser extension is a new distribution and update problem, and an
  unpacked development extension is a poor fit for a tool handling PII.
- The paired token is one more thing the operator has to set up correctly, and
  getting it wrong fails open in the worst way if requirement 2 is skipped.
- Field-descriptor extraction has to be written a second time, in JavaScript,
  against the same DOM shapes `pkg/submitter/questions.go` already handles in
  Go. Two implementations of one idea will drift.

**Deferred deliberately.** The backend contract above is written down; the
extension is not built. The Copy Application Packet already recovers most of the
operator's time in the handoff case, at none of this complexity, so the
remaining benefit does not yet justify shipping a browser extension that handles
personal data. Revisit when the packet proves insufficient in practice.
