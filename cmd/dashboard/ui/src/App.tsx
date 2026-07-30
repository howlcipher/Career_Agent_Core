import { useState, useEffect } from 'react';
import './index.css';
import './App.css';

// One row of a conversion table. The two breakdowns differ only in what
// their first column is keyed by, so they share a shape and a renderer -
// see ConversionTable below. Mirrors SourceConversionStat and
// VariantConversionStat in cmd/dashboard/main.go; these were typed `any[]`
// here, which is part of why bug #437 went unnoticed for so long.
interface ConversionRow {
  total_applied: number;
  interviews: number;
  rejections: number;
  pending: number;
  interview_rate_pct: string;
}

interface SourceConversionRow extends ConversionRow {
  source: string;
}

interface VariantConversionRow extends ConversionRow {
  variant: string;
}

interface Metrics {
  discovered: number;
  processing: number;
  skipped: number;
  applied: number;
  failed: number;
  failed_score: number;
  failed_submit: number;
  manual_required: number;
  manual_required_only: number;
  awaiting_review: number;
  blocked_captcha: number;
  invalid_url: number;
  last_applied_company?: string;
  last_applied_title?: string;
  last_applied_url?: string;
  last_applied_at?: string;
  last_applied_processing_time?: string;
  current_company?: string;
  current_title?: string;
  current_since?: string;
  last_skipped_company?: string;
  last_skipped_title?: string;
  last_skipped_reason?: string;
  last_skipped_at?: string;
  last_skipped_processing_time?: string;
  last_failed_company?: string;
  last_failed_title?: string;
  last_failed_reason?: string;
  last_failed_at?: string;
  last_failed_processing_time?: string;
  last_manual_company?: string;
  last_manual_title?: string;
  last_manual_reason?: string;
  last_manual_at?: string;
  last_manual_processing_time?: string;
  status_legend?: Record<string, string>;
  total_applied_tracked: number;
  interviews: number;
  rejections: number;
  interview_rate_pct?: string;
  by_source?: SourceConversionRow[];
  by_variant?: VariantConversionRow[];
}

// ConversionTable renders one conversion breakdown. The <caption> and the
// six scope="col" headers are not decoration: they are improvement #34's
// shipped accessibility work, deleted by the #426 rewrite along with the
// tables themselves and restored here (bug #437). A table whose row header
// is a platform or tone name needs scope="row" on that cell too, so a
// screen reader can announce "Greenhouse, Interviews, 3" rather than a
// bare number.
function ConversionTable<Row extends ConversionRow>({
  caption,
  keyHeader,
  rows,
  rowKey,
}: {
  caption: string;
  keyHeader: string;
  rows: Row[];
  rowKey: (row: Row) => string;
}) {
  // The old template hid each section outright when its array was empty,
  // rather than showing an empty table with headers. Preserved: an empty
  // breakdown means nothing has been tracked yet, and a bare header row
  // reads as a broken table.
  if (rows.length === 0) return null;

  return (
    <div className="conversion-breakdown">
      <table>
        <caption>{caption}</caption>
        <thead>
          <tr>
            <th scope="col">{keyHeader}</th>
            <th scope="col">Applied</th>
            <th scope="col">Interviews</th>
            <th scope="col">Rejections</th>
            <th scope="col">Pending</th>
            <th scope="col">Rate</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={rowKey(row)}>
              <th scope="row">{rowKey(row)}</th>
              <td>{row.total_applied}</td>
              <td>{row.interviews}</td>
              <td>{row.rejections}</td>
              <td>{row.pending}</td>
              <td>{row.interview_rate_pct || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function App() {
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [agentRunning, setAgentRunning] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(true);

  const fetchMetrics = async () => {
    try {
      const res = await fetch('/api/metrics');
      if (res.ok) {
        const data = await res.json();
        setMetrics(data);
      }
    } catch (e) {
      console.error(e);
    }
  };

  const checkAgent = async () => {
    try {
      const res = await fetch('/api/agent/status');
      if (res.ok) {
        const data = await res.json();
        setAgentRunning(data.running);
      }
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchMetrics();
    checkAgent();
    const int = setInterval(() => {
      fetchMetrics();
      checkAgent();
    }, 2000);
    return () => clearInterval(int);
  }, []);

  const handleStart = async () => {
    await fetch('/api/agent/start', { method: 'POST' });
    checkAgent();
  };

  const handleStop = async () => {
    await fetch('/api/agent/stop', { method: 'POST' });
    checkAgent();
  };

  if (loading) return <div>Loading...</div>;

  // The counted-only statuses have no "last job" card of their own, so the
  // legend the API serves is the only thing that explains what the number
  // means (bug #435).
  const legend = metrics?.status_legend ?? {};
  const explain = (status: string) => legend[status] ?? '';

  // explainPair captions a tile that counts two unrelated statuses as one
  // number (bug #451: the Failed and Manual Queue tiles each do this). A
  // caption naming only one status was wrong whenever the other one
  // dominated the bucket, so this names whichever statuses actually
  // contributed. "; " rather than a bare separator glyph, since these are
  // two full sentences with no terminal punctuation of their own and a
  // screen reader needs an actual pause between them, not a middot it may
  // not announce at all. Falls back to naming both when the count is
  // genuinely zero, matching every sibling tile (Skipped, Blocked, Invalid),
  // which always show their static reason regardless of count -- an empty
  // caption here would be the only tile in the grid that goes bare at 0.
  const explainPair = (statusA: string, countA: number, statusB: string, countB: number) => {
    const reasons = [];
    if (countA > 0) reasons.push(explain(statusA));
    if (countB > 0) reasons.push(explain(statusB));
    if (reasons.length === 0) return `${explain(statusA)}; ${explain(statusB)}`;
    return reasons.join('; ');
  };

  return (
    <main>
      <h1>🚀 Career Agent Live Metrics</h1>
      <div className="agent-control">
        <button className="btn btn-start" onClick={handleStart} disabled={agentRunning}>▶ Start Agent</button>
        <button className="btn btn-stop" onClick={handleStop} disabled={!agentRunning}>🛑 Stop Agent</button>
      </div>

      <div className="dashboard-container" aria-live="polite" aria-atomic="false">
        <div className="grid">
          <div className="card discovered">
            <h2>{metrics?.discovered || 0}</h2>
            <p>In Queue</p>
          </div>
          <div className="card processing">
            <h2>{metrics?.processing || 0}</h2>
            <p>Processing</p>
          </div>
          <div className="card applied">
            <h2>{metrics?.applied || 0}</h2>
            <p>Applied</p>
          </div>
          <div className="card skipped">
            <h2>{metrics?.skipped || 0}</h2>
            <p>Skipped</p>
            <span className="card-reason">{explain('SKIPPED')}</span>
          </div>
          <div className="card failed">
            <h2>{metrics?.failed || 0}</h2>
            <p>Failed</p>
            <span className="card-reason">
              {explainPair('FAILED_SCORE', metrics?.failed_score ?? 0, 'FAILED_SUBMIT', metrics?.failed_submit ?? 0)}
            </span>
          </div>
          <div className="card manual">
            <h2>{metrics?.manual_required || 0}</h2>
            <p>Manual Queue</p>
            <span className="card-reason">
              {explainPair(
                'MANUAL_REQUIRED',
                metrics?.manual_required_only ?? 0,
                'AWAITING_REVIEW',
                metrics?.awaiting_review ?? 0
              )}
            </span>
          </div>
          <div className="card blocked">
            <h2>{metrics?.blocked_captcha || 0}</h2>
            <p>Blocked</p>
            <span className="card-reason">{explain('BLOCKED_CAPTCHA')}</span>
          </div>
          <div className="card invalid">
            <h2>{metrics?.invalid_url || 0}</h2>
            <p>Not A Posting</p>
            <span className="card-reason">{explain('INVALID_URL')}</span>
          </div>
          {/* interview_rate_pct is omitempty on the Go side: absent until at
              least one application has been tracked to an outcome, which is
              why this falls back to an em dash rather than "0%". */}
          <div className="card interview">
            <h2>{metrics?.interview_rate_pct || '—'}</h2>
            <p>Interview Rate</p>
            <span className="card-reason">
              {metrics?.interviews ?? 0} interview{metrics?.interviews === 1 ? '' : 's'} and{' '}
              {metrics?.rejections ?? 0} rejection{metrics?.rejections === 1 ? '' : 's'} across{' '}
              {metrics?.total_applied_tracked ?? 0} tracked application
              {metrics?.total_applied_tracked === 1 ? '' : 's'}
            </span>
          </div>
        </div>

        <section className="detail-grid">
          <article className="detail">
            <h3>Last Applied</h3>
            {metrics?.last_applied_company ? (
              <>
                <p className="detail-job">{metrics.last_applied_company} — {metrics.last_applied_title}</p>
                <p className="detail-meta">
                  {metrics.last_applied_at}
                  {metrics.last_applied_processing_time && ` · ${metrics.last_applied_processing_time} in pipeline`}
                </p>
              </>
            ) : (
              <p className="detail-meta">No confirmed application yet</p>
            )}
          </article>

          <article className="detail">
            <h3>Awaiting You</h3>
            {metrics?.last_manual_company ? (
              <>
                <p className="detail-job">{metrics.last_manual_company} — {metrics.last_manual_title}</p>
                {/* The whole point of #435: AWAITING_REVIEW (click a filled
                    form) and MANUAL_REQUIRED (do it all by hand) share this
                    card and are completely different asks. */}
                <p className="detail-reason">{metrics.last_manual_reason}</p>
                <p className="detail-meta">
                  {metrics.last_manual_at}
                  {metrics.last_manual_processing_time && ` · ${metrics.last_manual_processing_time} in pipeline`}
                </p>
              </>
            ) : (
              <p className="detail-meta">Nothing waiting on you</p>
            )}
          </article>

          <article className="detail">
            <h3>Last Skipped</h3>
            {metrics?.last_skipped_company ? (
              <>
                <p className="detail-job">{metrics.last_skipped_company} — {metrics.last_skipped_title}</p>
                <p className="detail-reason">{metrics.last_skipped_reason}</p>
                <p className="detail-meta">
                  {metrics.last_skipped_at}
                  {metrics.last_skipped_processing_time && ` · ${metrics.last_skipped_processing_time} in pipeline`}
                </p>
              </>
            ) : (
              <p className="detail-meta">Nothing skipped yet</p>
            )}
          </article>

          <article className="detail">
            <h3>Last Failure</h3>
            {metrics?.last_failed_company ? (
              <>
                <p className="detail-job">{metrics.last_failed_company} — {metrics.last_failed_title}</p>
                <p className="detail-reason">{metrics.last_failed_reason}</p>
                <p className="detail-meta">
                  {metrics.last_failed_at}
                  {metrics.last_failed_processing_time && ` · ${metrics.last_failed_processing_time} in pipeline`}
                </p>
              </>
            ) : (
              <p className="detail-meta">No failures recorded</p>
            )}
          </article>
        </section>

        {/* Improvement #15 (per-ATS conversion) and improvement #13 (per
            cover-letter-tone conversion). Both were shipped, marked Done,
            and then silently lost when #426 replaced the template with a
            tiles-only React app while the API kept serving the data. */}
        <ConversionTable
          caption="Conversion by Platform"
          keyHeader="Platform"
          rows={metrics?.by_source ?? []}
          rowKey={(row) => row.source}
        />
        <ConversionTable
          caption="Conversion by Cover-Letter Tone Variant"
          keyHeader="Variant"
          rows={metrics?.by_variant ?? []}
          rowKey={(row) => row.variant}
        />
      </div>
    </main>
  );
}

export default App;
