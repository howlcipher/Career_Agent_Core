import { useState, useEffect, useRef } from 'react';
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
  invalid_url_malformed: number;
  invalid_url_expired: number;
  retry_exhausted: number;
	assisted_waiting: number;
  confirmed_today: number;
  confirmed_last_7_days: number;
  first_attempt_median?: string;
  last_confirmed_ago?: string;
  eligible_queue: number;
  eligible_never_attempted: number;
  discovery_last_finished_at?: string;
  discovery_new_eligible: number;
  discovery_error_class?: string;
  discovery_source_counts?: Array<{
    source: string;
    request_attempted: number;
    request_failed: number;
    circuit_open_skipped: number;
  }>;
  watchdog_alert?: string;
  watchdog_alert_at?: string;
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


interface OperatorSettings {
  application_mode: 'find_only' | 'assisted' | 'automatic';
  minimum_fit_score: number;
  scoring_active?: boolean;
  daemon_active?: boolean;
}

interface QualifiedJob {
  id: number;
  company: string;
  title: string;
  fit_score: number;
  provider: string;
  discovered_at: string;
  last_updated: string;
  location: string;
  remote: boolean;
  reason: string;
}

interface AssistedAction {
	code: string;
	title: string;
	instruction: string;
	primary_button: string;
	requires_browser: boolean;
	documents_ready: boolean;
	requires_explicit_submit: boolean;
	can_continue: boolean;
}

interface AssistedJob {
	id: string;
	company: string;
	role: string;
	fit_score?: number;
	provider: string;
	original_status: string;
	interruption: string;
	last_updated: string;
	resume_ready: boolean;
	cover_letter_ready: boolean;
	mapping_ready: boolean;
	completed_work: string;
	legacy: boolean;
	live_browser: boolean;
	assisted_attempt_count: number;
	priority_reason: string;
	next_action: AssistedAction;
	// Present only when next_action.code is 'open_in_own_browser' — an ATS
	// that rejects the assisted browser, so the operator needs the link.
	apply_url?: string;
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
  const [actionError, setActionError] = useState<string | null>(null);
	const [assistedJobs, setAssistedJobs] = useState<AssistedJob[]>([]);
	const [showAssisted, setShowAssisted] = useState<boolean>(false);
	const [confirmJob, setConfirmJob] = useState<AssistedJob | null>(null);
	const [confirmQualifiedJob, setConfirmQualifiedJob] = useState<QualifiedJob | null>(null);
	const [selectedJobs, setSelectedJobs] = useState<string[]>([]);
	const [batchIndex, setBatchIndex] = useState<number | null>(null);
	const [stopAfterCurrent, setStopAfterCurrent] = useState<boolean>(false);

  // #460: a single missed poll is expected noise (a request can legitimately
  // drop once), but a *run* of them means the numbers on screen may be
  // stale with nothing telling the user that. Counts consecutive failures;
  // any success resets it to 0. Only rendered once it crosses a small
  // threshold, so a lone miss stays silent.
  
  const [operatorSettings, setOperatorSettings] = useState<OperatorSettings | null>(null);
  const [draftSettings, setDraftSettings] = useState<OperatorSettings | null>(null);
  const [draftScoreStr, setDraftScoreStr] = useState<string>('');
  const [showModeConfirm, setShowModeConfirm] = useState<boolean>(false);
  const [savingSettings, setSavingSettings] = useState<boolean>(false);
  const [qualifiedJobs, setQualifiedJobs] = useState<QualifiedJob[]>([]);
  const [showQualified, setShowQualified] = useState<boolean>(false);

  const opRef = useRef<OperatorSettings | null>(null);
  const draftRef = useRef<OperatorSettings | null>(null);
  const scoreRef = useRef<string>('');

  useEffect(() => { opRef.current = operatorSettings; }, [operatorSettings]);
  useEffect(() => { draftRef.current = draftSettings; }, [draftSettings]);
  useEffect(() => { scoreRef.current = draftScoreStr; }, [draftScoreStr]);

  const [pollFailures, setPollFailures] = useState<number>(0);

  // Poll responses can resolve out of order (bug #447: a slow request can
  // finish after a later, faster one). Each poll tags itself with the
  // sequence number current at send time, and a response is applied only if
  // it is still the most recent request in flight when it resolves.
  const pollSeq = useRef(0);

  // Assisted Apply is requested independently of the metrics poll. Reusing
  // pollSeq here made a large queue disappear whenever its response arrived
  // after the next two-second metrics poll advanced that counter.
  const assistedSeq = useRef(0);

  const fetchMetrics = async (seq: number) => {
    try {
      const res = await fetch('/api/metrics');
      if (res.ok) {
        const data = await res.json();
        if (seq === pollSeq.current) {
          setMetrics(data);
          setPollFailures(0);
        }
      } else if (seq === pollSeq.current) {
        setPollFailures((n) => n + 1);
      }
    } catch (e) {
      console.error(e);
      if (seq === pollSeq.current) setPollFailures((n) => n + 1);
    }
  };

  const checkAgent = async (seq: number) => {
    try {
      const res = await fetch('/api/agent/status');
      if (res.ok) {
        const data = await res.json();
        if (seq === pollSeq.current) setAgentRunning(data.running);
      }
    } catch (e) {
      console.error(e);
    } finally {
      if (seq === pollSeq.current) setLoading(false);
    }
  };

	
  const fetchOperatorSettings = async () => {
    try {
      const res = await fetch('/api/operator-settings');
      if (res.ok) {
        const settings = await res.json();
        const prevOp = opRef.current;
        const prevDraft = draftRef.current;
        const prevScore = scoreRef.current;
        
        const isDirty = prevOp != null && prevDraft != null && 
            (JSON.stringify(prevDraft) !== JSON.stringify(prevOp) || prevScore !== prevOp.minimum_fit_score.toString());
        
        setOperatorSettings(settings);
        if (!isDirty || prevOp == null) {
          setDraftSettings(settings);
          setDraftScoreStr(settings.minimum_fit_score.toString());
        }
      }
    } catch (e) { console.error(e); }
  };

  const fetchQualifiedJobs = async () => {
    try {
      const res = await fetch('/api/qualified-jobs');
      if (res.ok) {
        setQualifiedJobs(await res.json());
      }
    } catch (e) { console.error(e); }
  };

  const saveOperatorSettings = async (settings: OperatorSettings) => {
    setSavingSettings(true);
    try {
      const res = await fetch('/api/operator-settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings),
      });
      if (res.ok) {
        const updated = await res.json();
        setOperatorSettings(updated);
        setDraftSettings(updated);
        setDraftScoreStr(updated.minimum_fit_score.toString());
        setShowModeConfirm(false);
        setActionError(null);
      } else {
        setActionError("Failed to save settings");
      }
    } catch (e) {
      console.error(e);
      setActionError("Error saving settings");
    } finally {
      setSavingSettings(false);
    }
  };


  const qualifiedAction = async (jobId: number, action: 'open' | 'promote' | 'skip' | 'confirm') => {
    try {
      const res = await fetch(`/api/qualified-jobs/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_id: jobId }),
      });
      if (res.ok) {
        fetchQualifiedJobs();
      } else {
        setActionError(`Failed to ${action} job`);
      }
    } catch(e) {
      console.error(e);
      setActionError(`Error performing ${action}`);
    }
  };

	const fetchAssisted = async () => {
		const seq = ++assistedSeq.current;
		try {
			const res = await fetch('/api/assisted');
			if (!res.ok) {
				setActionError('Could not load Assisted Apply. Check the dashboard log and try again.');
				return;
			}
			const data = await res.json();
			if (seq === assistedSeq.current) {
				setAssistedJobs(data.jobs ?? []);
				setActionError(null);
			}
		} catch (e) {
			console.error(e);
			setActionError('Could not load Assisted Apply. Check the dashboard log and try again.');
		}
	};

  const poll = () => {
    const seq = ++pollSeq.current;
    fetchMetrics(seq);
    checkAgent(seq);

    fetchOperatorSettings();
    fetchQualifiedJobs();

  };

  useEffect(() => {
    poll();
    const int = setInterval(poll, 2000);
    return () => clearInterval(int);
  }, []);

	useEffect(() => {
		if (!showAssisted) return;
		fetchAssisted();
		const assistedInterval = setInterval(fetchAssisted, 2000);
		return () => clearInterval(assistedInterval);
	}, [showAssisted]);

  const handleStart = async () => {
    setActionError(null);
    try {
      const res = await fetch('/api/agent/start', { method: 'POST' });
      if (!res.ok) setActionError('Failed to start agent — check the log');
    } catch (e) {
      console.error(e);
      setActionError('Failed to start agent — check the log');
    }
    poll();
  };

	const handleStop = async () => {
    setActionError(null);
    try {
      const res = await fetch('/api/agent/stop', { method: 'POST' });
      if (!res.ok) setActionError('Failed to stop agent — check the log');
    } catch (e) {
      console.error(e);
      setActionError('Failed to stop agent — check the log');
    }
    poll();
  };

	const confirmApplied = async () => {
		if (!confirmJob) return;
		setActionError(null);
		try {
			const res = await fetch('/api/assisted/confirm', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ job_id: confirmJob.id, confirmed: true }) });
			if (!res.ok) throw new Error('confirmation rejected');
			setConfirmJob(null);
			fetchAssisted();
			poll();
		} catch (e) {
			console.error(e);
			setActionError('Could not record the application confirmation. Keep it pending and try again after verifying the employer site.');
		}
	};

	const requestContinue = async (job: AssistedJob) => {
		try {
			const res = await fetch('/api/assisted/continue', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ job_id: job.id }) });
			if (!res.ok) throw new Error('continue rejected');
			fetchAssisted();
		} catch (e) {
			console.error(e);
			setActionError('The assisted browser is no longer active. Open the application again before continuing.');
		}
	};

	const launchAssisted = async (job: AssistedJob): Promise<boolean> => {
		setActionError(null);
		try {
			const res = await fetch('/api/assisted/launch', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ job_id: job.id }) });
			if (!res.ok) throw new Error('launch rejected');
			await fetchAssisted();
			return true;
		} catch (e) { console.error(e); setActionError('Could not open the assisted application. Close any current assisted browser, then try again.'); return false; }
	};

	const revalidateAssisted = async (job: AssistedJob) => {
		setActionError(null);
		try {
			const res = await fetch('/api/assisted/revalidate', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ job_id: job.id }) });
			if (!res.ok) throw new Error('revalidation rejected');
			fetchAssisted();
		} catch (e) {
			console.error(e);
			setActionError('Could not check the current employer page. No browser was opened; try again later.');
		}
	};

	const toggleSelected = (id: string) => setSelectedJobs((ids) => ids.includes(id) ? ids.filter((value) => value !== id) : [...ids, id]);
	const startSelected = async () => {
		if (selectedJobs.length === 0 || assistedBrowserOpen) return;
		setBatchIndex(0); setStopAfterCurrent(false);
		const first = assistedJobs.find((job) => job.id === selectedJobs[0]);
		if (!first || !await launchAssisted(first)) setBatchIndex(null);
	};
	const nextSelected = async () => {
		if (batchIndex === null || stopAfterCurrent) { setBatchIndex(null); return; }
		if (assistedBrowserOpen) {
			setActionError('Close the current assisted browser before opening the next selected application.');
			return;
		}
		const nextIndex = batchIndex + 1;
		if (nextIndex >= selectedJobs.length) { setBatchIndex(null); return; }
		const next = assistedJobs.find((job) => job.id === selectedJobs[nextIndex]);
		if (next && await launchAssisted(next)) setBatchIndex(nextIndex);
	};

	const openDocument = (job: AssistedJob, kind: 'resume' | 'cover_letter') => {
		window.open(`/api/assisted/document?job_id=${encodeURIComponent(job.id)}&kind=${kind}`, '_blank', 'noopener,noreferrer');
	};
	const assistedBrowserOpen = assistedJobs.some((job) => job.live_browser);

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

  // explainInvalidURL is explainPair's counterpart for INVALID_URL
  // (improvements.md #468): unlike Failed/Manual, this is one status split by
  // a persisted sub-reason rather than two statuses, so it can't reuse the
  // legend lookup — a live measurement against applications.db found ~88% of
  // this bucket is a real posting that expired, not the "never a real
  // posting" case the single old caption implied for the whole count.
  const explainInvalidURL = (malformed: number, expired: number) => {
    const parts = [];
    if (malformed > 0) parts.push(`${malformed} never a real posting (board index, marketing page, or blocked URL)`);
    if (expired > 0) parts.push(`${expired} a real posting that has since expired`);
    if (parts.length === 0) return explain('INVALID_URL');
    return parts.join('; ');
  };

  const discoveryStatus = () => {
    if (!agentRunning) return 'Agent is stopped; start it to refresh discovery and process eligible jobs.';
    if (!metrics?.discovery_last_finished_at) return 'Agent is running; waiting for the first discovery refresh.';
    const yahoo = metrics.discovery_source_counts?.find((source) => source.source === 'yahoo');
    const yahooHealth = yahoo && (yahoo.request_failed > 0 || yahoo.circuit_open_skipped > 0)
      ? ` Yahoo had ${yahoo.request_failed} request failure${yahoo.request_failed === 1 ? '' : 's'} and ${yahoo.circuit_open_skipped} circuit-open skip${yahoo.circuit_open_skipped === 1 ? '' : 's'}.`
      : '';
    if (metrics.discovery_error_class) return `Latest discovery refresh had a ${metrics.discovery_error_class} error.${yahooHealth}`;
    if (metrics.discovery_new_eligible === 0) return `Latest discovery refresh found no new eligible jobs.${yahooHealth}`;
    return `Latest discovery refresh added ${metrics.discovery_new_eligible} eligible job${metrics.discovery_new_eligible === 1 ? '' : 's'}.${yahooHealth}`;
  };

  return (
    <main>
      <header className="command-header">
        <p className="eyebrow">CAREER AGENT CORE // COMMAND DECK</p>
        <div className="command-title-row">
          <h1>Operations Console</h1>
          <span className={agentRunning ? 'system-state online' : 'system-state standby'}>{agentRunning ? 'SYSTEM ACTIVE' : 'SYSTEM STANDBY'}</span>
        </div>
        <p className="command-subtitle">Queue telemetry, human handoffs, and bounded agent control from one local console.</p>
      </header>
      <div className="agent-control">
        <button className="btn btn-start" onClick={handleStart} disabled={agentRunning}>▶ Start Agent</button>
        <button className="btn btn-stop" onClick={handleStop} disabled={!agentRunning}>🛑 Stop Agent</button>
      </div>
		
      <section className="settings-panel">
        <h2>Application Mode</h2>
        {draftSettings ? (
          <div className="operator-settings">
            <label>
              Mode:
              <select
                value={draftSettings.application_mode}
                onChange={(e) => {
                  setDraftSettings({ ...draftSettings, application_mode: e.target.value as any });
                }}
              >
                <option value="find_only">Find Only (Review every application)</option>
                <option value="assisted">Assisted Apply (Fill form but wait to submit)</option>
                <option value="automatic">Automatic Apply (Fully autonomous)</option>
              </select>
            </label>
            <label>
              Minimum Fit Score:
              <input
                type="text"
                value={draftScoreStr}
                onChange={(e) => {
                  const val = e.target.value;
                  setDraftScoreStr(val);
                  const parsed = parseInt(val);
                  if (!isNaN(parsed)) {
                    setDraftSettings({ ...draftSettings, minimum_fit_score: parsed });
                  }
                }}
              />
            </label>
            {!draftSettings.scoring_active && (
              <p className="warning">Scoring is disabled in your profile.yaml; fit score will be ignored.</p>
            )}
            
            {operatorSettings && !operatorSettings.daemon_active && (
              <p className="status-message info">
                Settings are saved but waiting for the daemon's next cycle to activate.
              </p>
            )}
            {operatorSettings && operatorSettings.daemon_active && JSON.stringify(draftSettings) === JSON.stringify(operatorSettings) && draftScoreStr === operatorSettings?.minimum_fit_score.toString() && (
              <p className="status-message success">
                Settings are saved and currently active.
              </p>
            )}
            
            {(JSON.stringify(draftSettings) !== JSON.stringify(operatorSettings) || draftScoreStr !== (operatorSettings?.minimum_fit_score.toString() ?? '')) && (
              <div className="settings-actions">
                <span className="unsaved-warning">You have unsaved changes.</span>
                <button 
                  className="btn btn-primary" 
                  disabled={
                    savingSettings || 
                    draftScoreStr.trim() === '' || 
                    isNaN(parseInt(draftScoreStr)) || 
                    parseInt(draftScoreStr) < 0 || 
                    parseInt(draftScoreStr) > 100 ||
                    (JSON.stringify(draftSettings) === JSON.stringify(operatorSettings) && draftScoreStr === operatorSettings?.minimum_fit_score.toString())
                  }
                  onClick={() => setShowModeConfirm(true)}>
                    Apply Changes
                </button>
                <button 
                  className="btn btn-secondary" 
                  disabled={savingSettings}
                  onClick={() => {
                    setDraftSettings(operatorSettings);
                    setDraftScoreStr(operatorSettings?.minimum_fit_score.toString() ?? '');
                    setShowModeConfirm(false);
                    setActionError(null);
                  }}>
                    Discard Changes
                </button>
              </div>
            )}
            
            {showModeConfirm && (
              <div className="modal-overlay">
                <div className="modal confirm-modal">
                  <h3>Confirm Settings Change</h3>
                  {draftSettings.application_mode === 'find_only' && (
                    <p>Career Agent will find and score jobs and list qualified matches.<br/>It will not prepare documents, fill application forms, or submit applications.</p>
                  )}
                  {draftSettings.application_mode === 'assisted' && (
                    <p>Career Agent will prepare qualified applications and fill supported fields.<br/>You must review and submit every application.<br/>Career Agent will never click the employer’s final Submit button.</p>
                  )}
                  {draftSettings.application_mode === 'automatic' && (
                    <p><strong>Warning:</strong> Career Agent may click the employer’s Submit button on supported forms.<br/>CAPTCHA, login, legal-attestation, and unsupported cases still require you.</p>
                  )}
                  <div className="modal-actions">
                    <button className="btn btn-primary" disabled={savingSettings} onClick={() => saveOperatorSettings(draftSettings)}>
                      {savingSettings ? 'Saving...' : 'Confirm'}
                    </button>
                    <button className="btn btn-secondary" disabled={savingSettings} onClick={() => setShowModeConfirm(false)}>Cancel</button>
                  </div>
                </div>
              </div>
            )}
          </div>
        ) : (
          <p>Loading settings...</p>
        )}
      </section>

      <section className="qualified-banner" aria-label="Qualified Jobs">
        <div><strong>Qualified jobs found: {qualifiedJobs.length}</strong><span> Review jobs that met your minimum fit score.</span></div>
        <button className="btn btn-qualified" onClick={() => setShowQualified(true)}>View Qualified Jobs</button>
      </section>

		<section className="assisted-banner" aria-label="Assisted Apply">
			<div><strong>Assisted applications waiting: {metrics?.assisted_waiting ?? 0}</strong><span> Complete the next safe human step without restarting the application.</span></div>
			<button className="btn btn-assisted" onClick={() => setShowAssisted(true)}>Open Assisted Apply</button>
		</section>
      {actionError && (
        <p className="action-error" role="alert">{actionError}</p>
      )}
      {/* #460: role="status" rather than "alert" — persistent staleness is
          worth a passive announcement, not an interruption, and the numbers
          below are still the last real ones fetched, not an error state. */}
        {pollFailures >= 2 && (
        <p className="poll-warning" role="status">
          Metrics may be out of date — the last {pollFailures} polls failed
        </p>
        )}
        {metrics?.watchdog_alert && (
          <p className="watchdog-alert" role="alert">
            Daemon watchdog: {metrics.watchdog_alert}
            {metrics.watchdog_alert_at && ` (${metrics.watchdog_alert_at})`}
          </p>
        )}

      <div className="dashboard-container" aria-live="polite" aria-atomic="false">
			
      {showQualified && (
        <section className="qualified-queue">
          <div className="assisted-heading">
            <h2>Qualified Jobs (Find Only Mode)</h2>
            <button className="text-button" onClick={() => setShowQualified(false)}>Return to dashboard</button>
          </div>
          {qualifiedJobs.length === 0 ? (
            <p>No qualified jobs waiting.</p>
          ) : (
            qualifiedJobs.map(job => (
              <article className="assisted-job" key={job.id}>
                <div>
                  <h3>{job.company} — {job.title}</h3>
                  <p className="detail-meta">
                    Fit score: {job.fit_score} · Provider: {job.provider} · Location: {job.location || 'Unknown'} {job.remote ? '(Remote)' : ''}
                  </p>
                  <p className="detail-meta">Discovered: {new Date(job.discovered_at).toLocaleString()}</p>
                </div>
                <div className="assisted-instruction">
                  <button className="text-button" onClick={() => qualifiedAction(job.id, 'open')}>Open Current Job</button>
                  <button className="btn btn-assisted" onClick={() => qualifiedAction(job.id, 'promote')}>Move to Assisted Apply</button>
                  <button className="text-button" onClick={() => setConfirmQualifiedJob(job)}>Mark Applied Manually</button>
                  <button className="text-button text-danger" onClick={() => qualifiedAction(job.id, 'skip')}>Skip</button>
                </div>
              </article>
            ))
          )}
        </section>
      )}

			{showAssisted && (
				<section className="assisted-queue" aria-labelledby="assisted-heading">
					<div className="assisted-heading"><h2 id="assisted-heading">Assisted Apply</h2><button className="text-button" onClick={() => setShowAssisted(false)}>Return to dashboard</button></div>
					{assistedJobs.length > 0 && <div className="batch-controls"><button className="btn btn-assisted" onClick={startSelected} disabled={selectedJobs.length === 0 || batchIndex !== null || assistedBrowserOpen}>Start Selected Applications</button>{batchIndex !== null && <><span>Application {batchIndex + 1} of {selectedJobs.length}</span><button className="text-button" onClick={() => setStopAfterCurrent(true)}>Stop After This Application</button><button className="text-button" onClick={nextSelected} disabled={assistedBrowserOpen}>{assistedBrowserOpen ? 'Close Current Application First' : 'Open Next Selected Application'}</button></>}</div>}
					{assistedJobs.length === 0 ? <p className="detail-meta">There is nothing to complete right now. New handoffs will appear here when human action is needed.</p> : assistedJobs.map((job) => (
						<article className="assisted-job" key={job.id}>
							<div><label className="batch-select"><input type="checkbox" checked={selectedJobs.includes(job.id)} onChange={() => toggleSelected(job.id)} disabled={batchIndex !== null || !job.next_action.requires_browser} /> Select for sequential batch</label><h3>{job.company} — {job.role}</h3><p className="detail-meta">{job.priority_reason}{job.fit_score !== undefined && ` · Fit score ${job.fit_score}`} · ATS: {job.provider} · Original status: {job.original_status}{job.legacy && ' · Legacy handoff'} · Updated {new Date(job.last_updated).toLocaleString()}</p></div>
							<ol className="assisted-stepper" aria-label="Application progress"><li className="done">Prepared</li><li className="active">Human action</li><li>Continue filling</li><li className={job.next_action.requires_explicit_submit ? 'active' : ''}>Review and submit</li><li>Confirmed</li></ol>
						<div className="assisted-instruction"><h4>What you need to do</h4><p>{job.next_action.instruction}</p>{job.apply_url ? (
							// This ATS refuses the assisted browser, so there is nothing
							// to launch — the operator opens the posting themselves. A
							// real anchor, not a fetch, is the point of the hand-off.
							<a className="btn btn-assisted" href={job.apply_url} target="_blank" rel="noopener noreferrer">{job.next_action.primary_button}</a>
						) : (
							<button className="btn btn-assisted" onClick={() => job.next_action.requires_browser ? launchAssisted(job) : revalidateAssisted(job)} disabled={job.next_action.requires_browser && assistedBrowserOpen}>{job.next_action.requires_browser && job.live_browser ? 'Assisted Application Open' : job.next_action.requires_browser && assistedBrowserOpen ? 'Finish Open Application First' : job.next_action.primary_button}</button>
						)}{job.next_action.can_continue && <button className="text-button" onClick={() => requestContinue(job)} disabled={!job.live_browser}>I completed this step — Continue</button>}{job.next_action.requires_explicit_submit && <button className="text-button" onClick={() => setConfirmJob(job)}>I saw a confirmation — Mark Applied</button>}</div>
							<details><summary>Career Agent already completed</summary><p>{job.completed_work}</p><p className="detail-meta">Résumé: {job.resume_ready ? 'ready' : 'will be prepared when safe'} · Cover letter: {job.cover_letter_ready ? 'ready' : 'will be prepared when safe'} · Form mapping: {job.mapping_ready ? 'ready' : 'not yet confirmed'} · Assisted attempts: {job.assisted_attempt_count}</p>{job.resume_ready && <button className="text-button" onClick={() => openDocument(job, 'resume')}>View Résumé</button>}{job.cover_letter_ready && <button className="text-button" onClick={() => openDocument(job, 'cover_letter')}>View Cover Letter</button>}</details>
							<p className="detail-meta">What happens next: {job.next_action.can_continue ? 'return here after the human step and continue filling.' : 'confirm the employer accepted the application before marking it applied.'}</p>
						</article>
					))}
				</section>
			)}
			{confirmJob && <div className="confirm-backdrop" role="presentation"><section className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title"><h2 id="confirm-title">Confirm application received</h2><p>Only mark this applied if the employer’s site showed that your application was received or successfully submitted.</p><div><button className="btn btn-assisted" onClick={confirmApplied}>Confirmed — Mark Applied</button><button className="text-button" onClick={() => setConfirmJob(null)}>Not confirmed</button></div></section></div>}
			{confirmQualifiedJob && (
				<div className="confirm-backdrop" role="presentation">
					<section className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-qualified-title">
						<h2 id="confirm-qualified-title">Mark this job applied?</h2>
						<p>Only continue if the employer showed that your application was received or successfully submitted.</p>
						<div>
							<button className="text-button" onClick={() => setConfirmQualifiedJob(null)}>Cancel</button>
							<button className="btn btn-assisted" onClick={async () => {
								try {
									const res = await fetch(`/api/qualified-jobs/confirm`, {
										method: 'POST',
										headers: { 'Content-Type': 'application/json' },
										body: JSON.stringify({ job_id: confirmQualifiedJob.id, confirmed_received: true }),
									});
									if (res.ok) fetchQualifiedJobs();
									else setActionError('Failed to confirm job');
								} catch(e) { console.error(e); setActionError('Error confirming job'); }
								setConfirmQualifiedJob(null);
							}}>Confirmed — Mark Applied</button>
						</div>
					</section>
				</div>
			)}
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
            <span className="card-reason">
              {explainInvalidURL(metrics?.invalid_url_malformed ?? 0, metrics?.invalid_url_expired ?? 0)}
            </span>
          </div>
          <div className="card retry-exhausted">
            <h2>{metrics?.retry_exhausted || 0}</h2>
            <p>Retry Exhausted</p>
            <span className="card-reason">{explain('RETRY_EXHAUSTED')}</span>
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
          <article className="detail mission-metrics">
            <h3>Mission Progress</h3>
            <dl>
              <div><dt>Confirmed today</dt><dd>{metrics?.confirmed_today ?? 0}</dd></div>
              <div><dt>Confirmed last 7 days</dt><dd>{metrics?.confirmed_last_7_days ?? 0}</dd></div>
              <div><dt>Median first attempt</dt><dd>{metrics?.first_attempt_median ?? '—'}</dd></div>
              <div><dt>Since last confirmed</dt><dd>{metrics?.last_confirmed_ago ?? 'No confirmed application yet'}</dd></div>
              <div><dt>Eligible queue</dt><dd>{metrics?.eligible_queue ?? 0} <span>({metrics?.eligible_never_attempted ?? 0} never attempted)</span></dd></div>
              <div><dt>Discovery health</dt><dd>{discoveryStatus()} {metrics?.discovery_last_finished_at && <span>({metrics.discovery_last_finished_at})</span>}</dd></div>
            </dl>
          </article>
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
