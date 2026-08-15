import { useCallback, useEffect, useRef, useState } from 'react';
import { ConsoleButton } from './ConsoleButton';
import type { FormInventory, PacketEntry } from '../types';

interface CopyPacketProps {
  jobId: string;
  /**
   * These panels live inside a collapsed <details>. Fetching on mount would
   * mean two extra requests per card on every queue render, for data nobody
   * has asked to see, so the fetch waits until the panel is actually opened.
   */
  active: boolean;
}

/** How often the packet re-reads itself while an inspection is in flight. */
const PREPARING_POLL_MS = 3000;

/**
 * Why an inspection did not happen, in the operator's words.
 *
 * The wire carries a closed set of codes rather than messages, because the
 * underlying failures come from a browser driver whose own error text quotes
 * page content (ADR-006). This map is the only place a code becomes a
 * sentence, and an unrecognised code falls back to the honest general case
 * rather than being printed raw.
 */
const INSPECTION_REASONS: Record<string, string> = {
  auth_required: 'this form cannot be read without being signed in',
  posting_dead: 'the posting is no longer available',
  captcha_blocked: 'the page is behind a bot check',
  quarantined: 'the page was withheld as unsafe to read',
  navigation_failed: 'the page could not be opened',
  no_form_found: 'no application form was found on the page',
  browser_rejected: "this employer's site refuses Career Agent's browser",
  already_applied: 'this application is already complete',
  unclassified: 'the inspection did not complete',
};

const reasonText = (code?: string): string =>
  (code && INSPECTION_REASONS[code]) || 'the inspection did not complete';

/**
 * Every prepared value in one place, each with a copy button.
 *
 * This is the floor under the whole feature: when automation cannot fill a form
 * at all — an unsupported ATS, a broken mapping, an employer's own browser —
 * the operator still gets the benefit of Career Agent having prepared
 * everything, instead of hunting through pii.yaml by hand.
 *
 * Sensitive values are hidden until deliberately revealed. Not because the
 * operator should not see their own data, but because a dashboard left open on
 * a second monitor should not be displaying a work-authorization declaration.
 *
 * The panel always states what it knows about the employer's own form, in
 * every state including the ones with nothing to show. Silence there used to
 * mean two opposite things — "read, asks nothing more" and "never read" — and
 * on a real Lever application it meant the second while looking like the first
 * (bugs.md #547). Preparation is offered here, where the operator notices the
 * gap, rather than in a panel they would have to go find.
 */
export function CopyPacket({ jobId, active }: CopyPacketProps) {
  const [entries, setEntries] = useState<PacketEntry[] | null>(null);
  const [inventory, setInventory] = useState<FormInventory | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [prepareError, setPrepareError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const live = useRef(true);

  const load = useCallback(async () => {
    try {
      const res = await fetch(`/api/assisted/packet?job_id=${encodeURIComponent(jobId)}`);
      if (!res.ok) throw new Error('packet unavailable');
      const data = await res.json();
      if (!live.current) return;
      setEntries(data.entries ?? []);
      setInventory(data.form_inventory ?? null);
    } catch {
      if (live.current) setError('Could not load your prepared details.');
    }
  }, [jobId]);

  useEffect(() => {
    if (!active) return;
    live.current = true;
    void load();
    return () => {
      live.current = false;
    };
  }, [active, load]);

  // While an inspection is running, the packet re-reads itself until the state
  // moves on, so the questions appear where the operator is already looking
  // instead of behind a manual refresh they have no reason to expect.
  useEffect(() => {
    if (!active || inventory?.state !== 'preparing') return;
    const timer = window.setInterval(() => void load(), PREPARING_POLL_MS);
    return () => window.clearInterval(timer);
  }, [active, inventory?.state, load]);

  const prepare = async () => {
    setPrepareError(null);
    setStarting(true);
    try {
      const res = await fetch('/api/knowledge/preflight', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_ids: [jobId] }),
      });
      if (!res.ok) {
        // A refusal here is a real answer, not a failure to communicate: the
        // run is already going, or an assisted browser is open. Both are
        // shown as sent rather than reworded.
        setPrepareError((await res.text()).trim() || 'Preparation could not be started.');
        return;
      }
      await load();
    } catch {
      setPrepareError('Preparation could not be started.');
    } finally {
      if (live.current) setStarting(false);
    }
  };

  const copy = async (entry: PacketEntry) => {
    try {
      await navigator.clipboard.writeText(entry.value);
      setCopied(entry.label);
      window.setTimeout(() => setCopied(null), 1500);
    } catch {
      setError('Your browser blocked clipboard access. Select the value and copy it manually.');
    }
  };

  if (error) return <p className="detail-meta">{error}</p>;
  if (!entries) return <p className="detail-meta">Loading your prepared details…</p>;

  // Prepared values first, then what this employer actually asks. The second
  // list is the one that turns copying from a memory exercise into a checklist,
  // and it only exists once the form has been inspected.
  const prepared = entries.filter((entry) => !entry.from_this_form);
  const asked = entries.filter((entry) => entry.from_this_form);
  const state = inventory?.state ?? 'not_prepared';
  const canPrepare = Boolean(inventory?.preparable) && state !== 'preparing' && !starting;

  const row = (entry: PacketEntry) => {
    const hidden = entry.sensitive && !revealed[entry.label];
    const nothingToCopy = entry.value.trim() === '';
    return (
      <li key={`${entry.label}-${entry.value}-${entry.status ?? ''}`}>
        <span className="copy-packet-label">{entry.label}</span>
        <span className="copy-packet-value">
          {nothingToCopy ? <em className="detail-meta">{entry.status}</em> : hidden ? '••••••••' : entry.value}
        </span>
        {entry.sensitive && !nothingToCopy && (
          <ConsoleButton
            variant="ghost"
            onClick={() => setRevealed((current) => ({ ...current, [entry.label]: !hidden }))}
          >
            {hidden ? 'Show' : 'Hide'}
          </ConsoleButton>
        )}
        {!nothingToCopy && (
          <ConsoleButton variant="ghost" onClick={() => copy(entry)}>
            Copy
          </ConsoleButton>
        )}
        {entry.status && !nothingToCopy && <span className="detail-meta"> {entry.status}</span>}
      </li>
    );
  };

  // One action, in the place the operator noticed the gap. It starts the same
  // preparation run the Prepare panel starts — the same endpoint, the same
  // child process — rather than a second way of reading a form.
  const prepareButton = (label: string) =>
    canPrepare && (
      <ConsoleButton variant="ghost" onClick={prepare}>
        {label}
      </ConsoleButton>
    );

  const inventorySection = () => {
    switch (state) {
      case 'preparing':
        return (
          <>
            <h5 className="copy-packet-heading">Form inventory</h5>
            <p className="detail-meta">
              Reading the employer's form… No fields are filled and nothing is submitted.
            </p>
          </>
        );

      case 'ready':
        if (asked.length === 0) {
          return (
            <>
              <h5 className="copy-packet-heading">Form inventory</h5>
              <p className="detail-meta">
                Form read. Career Agent found no questions beyond the details above
                {inventory?.field_count ? `, across ${inventory.field_count} fields` : ''}.
                {inventory?.stale
                  ? ' That reading is more than two weeks old, so the posting may have changed since.'
                  : ''}
              </p>
            </>
          );
        }
        return (
          <>
            <h5 className="copy-packet-heading">This form also asks ({asked.length})</h5>
            <p className="detail-meta">
              Read from the employer's own form. Anything marked “needs you” has no prepared answer, so
              you will be writing it rather than pasting it.
              {inventory?.stale
                ? ' This reading is more than two weeks old, so the posting may have changed since.'
                : ''}
            </p>
            <ul className="copy-packet-list">{asked.map(row)}</ul>
          </>
        );

      case 'failed':
        return (
          <>
            <h5 className="copy-packet-heading">Form inventory</h5>
            <p className="detail-meta">
              Career Agent could not read this form, because {reasonText(inventory?.reason)}. Treat the
              details above as a starting point rather than a complete packet.
            </p>
            {asked.length > 0 && (
              <>
                <p className="detail-meta">
                  What an earlier reading found is still listed below, and may be out of date.
                </p>
                <ul className="copy-packet-list">{asked.map(row)}</ul>
              </>
            )}
            {prepareButton('Try preparing this application again')}
          </>
        );

      default:
        return (
          <>
            <h5 className="copy-packet-heading">Form inventory</h5>
            <p className="detail-meta">
              Not prepared yet. Career Agent has not read this employer's form, so it cannot say what
              else the form asks — this packet may be incomplete.
            </p>
            {!inventory?.preparable && (
              <p className="detail-meta">
                It cannot be read now either, because {reasonText(inventory?.reason)}.
              </p>
            )}
            {prepareButton('Prepare this application')}
          </>
        );
    }
  };

  return (
    <div className="copy-packet">
      <p className="detail-meta">
        Every value Career Agent has prepared, for any field it could not fill itself.
      </p>
      <p aria-live="polite" className="copy-packet-status">
        {copied ? `${copied} copied` : ''}
      </p>
      {prepared.length > 0 ? (
        <ul className="copy-packet-list">{prepared.map(row)}</ul>
      ) : (
        <p className="detail-meta">No stored details are configured yet.</p>
      )}

      {/*
        Rendered in every state, including the ones with nothing to list. An
        empty section here is the whole point: silence is what let a form
        nobody had read look like a form with nothing left to ask.
      */}
      <section aria-live="polite">{inventorySection()}</section>
      {prepareError && <p className="detail-meta">{prepareError}</p>}
    </div>
  );
}
