import { useEffect, useState } from 'react';
import type { DocumentSummary } from '../types';

interface DocumentChangesProps {
  jobId: string;
  /**
   * These panels live inside a collapsed <details>. Fetching on mount would
   * mean two extra requests per card on every queue render, for data nobody
   * has asked to see, so the fetch waits until the panel is actually opened.
   */
  active: boolean;
}

const kindLabel: Record<string, string> = {
  resume: 'Résumé',
  cover_letter: 'Cover letter',
};

/**
 * What is in this application's documents, without making the operator reread
 * them for every job.
 *
 * The résumé and the cover letter genuinely differ here and the summary says
 * so: Assisted Apply always attaches the master résumé, so there is nothing to
 * diff and it reports that plainly, while a tailored cover letter gets a real
 * change list — including any figure that appears in it but not in the master,
 * which is the one thing worth reading every time.
 */
export function DocumentChanges({ jobId, active }: DocumentChangesProps) {
  const [documents, setDocuments] = useState<DocumentSummary[] | null>(null);

  useEffect(() => {
    if (!active) return;
    let live = true;
    fetch(`/api/assisted/document-summary?job_id=${encodeURIComponent(jobId)}`)
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error('unavailable'))))
      .then((data) => {
        if (live) setDocuments(data.documents ?? []);
      })
      .catch(() => {
        if (live) setDocuments([]);
      });
    return () => {
      live = false;
    };
  }, [jobId, active]);

  if (!documents) return <p className="detail-meta">Checking your documents…</p>;
  if (documents.length === 0) return <p className="detail-meta">No document summary available.</p>;

  return (
    <div className="document-changes">
      {documents.map((document) => (
        <div key={document.kind} className="document-change">
          <h5>
            {kindLabel[document.kind] ?? document.kind}
            {!document.ready && ' — not prepared'}
          </h5>
          <p className="document-headline">{document.headline}</p>
          {(document.changes ?? []).length > 0 && (
            <ul>
              {(document.changes ?? []).map((change) => (
                <li key={change}>{change}</li>
              ))}
            </ul>
          )}
          {document.note && <p className="detail-meta">{document.note}</p>}
        </div>
      ))}
    </div>
  );
}
