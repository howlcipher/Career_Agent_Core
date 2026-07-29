import { useState, useEffect } from 'react';
import './index.css';
import './App.css';

interface Metrics {
  discovered: number;
  processing: number;
  skipped: number;
  applied: number;
  failed: number;
  manual_required: number;
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
  by_source?: any[];
  by_variant?: any[];
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
            <span className="card-reason">{explain('FAILED_SUBMIT')}</span>
          </div>
          <div className="card manual">
            <h2>{metrics?.manual_required || 0}</h2>
            <p>Manual Queue</p>
            <span className="card-reason">{explain('MANUAL_REQUIRED')}</span>
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
      </div>
    </main>
  );
}

export default App;
