import { useEffect, useState } from 'react';
import { ConsoleButton } from './ConsoleButton';
import type { PacketEntry } from '../types';

interface CopyPacketProps {
  jobId: string;
  /**
   * These panels live inside a collapsed <details>. Fetching on mount would
   * mean two extra requests per card on every queue render, for data nobody
   * has asked to see, so the fetch waits until the panel is actually opened.
   */
  active: boolean;
}

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
 */
export function CopyPacket({ jobId, active }: CopyPacketProps) {
  const [entries, setEntries] = useState<PacketEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (!active) return;
    let live = true;
    fetch(`/api/assisted/packet?job_id=${encodeURIComponent(jobId)}`)
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error('packet unavailable'))))
      .then((data) => {
        if (live) setEntries(data.entries ?? []);
      })
      .catch(() => {
        if (live) setError('Could not load your prepared details.');
      });
    return () => {
      live = false;
    };
  }, [jobId, active]);

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
  if (entries.length === 0) return <p className="detail-meta">Nothing prepared for this application yet.</p>;

  // Prepared values first, then what this employer actually asks. The second
  // list is the one that turns copying from a memory exercise into a checklist,
  // and it only exists once the form has been inspected.
  const prepared = entries.filter((entry) => !entry.from_this_form);
  const asked = entries.filter((entry) => entry.from_this_form);

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

  return (
    <div className="copy-packet">
      <p className="detail-meta">
        Every value Career Agent has prepared, for any field it could not fill itself.
      </p>
      <p aria-live="polite" className="copy-packet-status">
        {copied ? `${copied} copied` : ''}
      </p>
      <ul className="copy-packet-list">{prepared.map(row)}</ul>

      {asked.length > 0 && (
        <>
          <h5 className="copy-packet-heading">This form also asks ({asked.length})</h5>
          <p className="detail-meta">
            Read from the employer's own form. Anything marked “needs you” has no prepared answer, so
            you will be writing it rather than pasting it.
          </p>
          <ul className="copy-packet-list">{asked.map(row)}</ul>
        </>
      )}
    </div>
  );
}
