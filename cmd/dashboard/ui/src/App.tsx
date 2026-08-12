import { useState } from 'react';
import './index.css';
import './App.css';
import { useDashboard } from './hooks/useDashboard';
import type { AssistedJob, QualifiedJob } from './types';
import { CommandHeader } from './components/CommandHeader';
import { CommandSection } from './components/CommandSection';
import { MetricDisplay } from './components/MetricDisplay';
import { TelemetryRow } from './components/TelemetryRow';
import { ConversionTable } from './components/ConversionTable';
import { AssistedJobCard } from './components/AssistedJobCard';
import { QualifiedJobCard } from './components/QualifiedJobCard';
import { ConfirmDialog, ConfirmActions } from './components/ConfirmDialog';
import { ConsoleButton } from './components/ConsoleButton';
import { SystemBadge } from './components/SystemBadge';

function App() {
  const {
    metrics,
    agentRunning,
    loading,
    actionError,
    pollFailures,
    setActionError,
    operatorSettings,
    draftSettings,
    setDraftSettings,
    draftScoreStr,
    setDraftScoreStr,
    savingSettings,
    saveOperatorSettings,
    qualifiedJobs,
    showQualified,
    setShowQualified,
    assistedJobs,
    showAssisted,
    setShowAssisted,
    handleStart,
    handleStop,
    qualifiedAction,
    confirmApplied,
    markAssistedNotFound,
    requestContinue,
    launchAssisted,
    revalidateAssisted,
    openDocument,
    assistedBrowserOpen,
    fetchQualifiedJobs,
  } = useDashboard();

  const [showModeConfirm, setShowModeConfirm] = useState<boolean>(false);
  const [confirmJob, setConfirmJob] = useState<AssistedJob | null>(null);
  const [confirmNotFoundJob, setConfirmNotFoundJob] = useState<AssistedJob | null>(null);
  const [confirmQualifiedJob, setConfirmQualifiedJob] = useState<QualifiedJob | null>(null);
  const [selectedJobs, setSelectedJobs] = useState<string[]>([]);
  const [batchIndex, setBatchIndex] = useState<number | null>(null);
  const [stopAfterCurrent, setStopAfterCurrent] = useState<boolean>(false);

  const legend = metrics?.status_legend ?? {};
  const explain = (status: string) => legend[status] ?? '';

  const explainPair = (statusA: string, countA: number, statusB: string, countB: number) => {
    const reasons = [];
    if (countA > 0) reasons.push(explain(statusA));
    if (countB > 0) reasons.push(explain(statusB));
    if (reasons.length === 0) return `${explain(statusA)}; ${explain(statusB)}`;
    return reasons.join('; ');
  };

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
    const yahooHealth =
      yahoo && (yahoo.request_failed > 0 || yahoo.circuit_open_skipped > 0)
        ? ` Yahoo had ${yahoo.request_failed} request failure${yahoo.request_failed === 1 ? '' : 's'} and ${yahoo.circuit_open_skipped} circuit-open skip${yahoo.circuit_open_skipped === 1 ? '' : 's'}.`
        : '';
    if (metrics.discovery_error_class) return `Latest discovery refresh had a ${metrics.discovery_error_class} error.${yahooHealth}`;
    if (metrics.discovery_new_eligible === 0) return `Latest discovery refresh found no new eligible jobs.${yahooHealth}`;
    return `Latest discovery refresh added ${metrics.discovery_new_eligible} eligible job${metrics.discovery_new_eligible === 1 ? '' : 's'}.${yahooHealth}`;
  };

  const toggleSelected = (id: string) =>
    setSelectedJobs((ids) => (ids.includes(id) ? ids.filter((value) => value !== id) : [...ids, id]));

  const startSelected = async () => {
    if (selectedJobs.length === 0 || assistedBrowserOpen) return;
    setBatchIndex(0);
    setStopAfterCurrent(false);
    const first = assistedJobs.find((job) => job.id === selectedJobs[0]);
    if (!first || !(await launchAssisted(first))) setBatchIndex(null);
  };

  const nextSelected = async () => {
    if (batchIndex === null || stopAfterCurrent) {
      setBatchIndex(null);
      return;
    }
    if (assistedBrowserOpen) {
      setActionError('Close the current assisted browser before opening the next selected application.');
      return;
    }
    const nextIndex = batchIndex + 1;
    if (nextIndex >= selectedJobs.length) {
      setBatchIndex(null);
      return;
    }
    const next = assistedJobs.find((job) => job.id === selectedJobs[nextIndex]);
    if (next && (await launchAssisted(next))) setBatchIndex(nextIndex);
  };

  const unsaved =
    operatorSettings &&
    (JSON.stringify(draftSettings) !== JSON.stringify(operatorSettings) ||
      draftScoreStr !== operatorSettings.minimum_fit_score.toString());

  const settingsInvalid =
    draftScoreStr.trim() === '' ||
    isNaN(parseInt(draftScoreStr)) ||
    parseInt(draftScoreStr) < 0 ||
    parseInt(draftScoreStr) > 100;

  if (loading) return <div className="initial-loading">Loading...</div>;

  return (
    <div className="app-root">
      <div className="command-rail" role="navigation" aria-label="Command modules">
        <div className="rail-brand">
          <span className="rail-title">CAC</span>
          <span className="rail-subtitle">Command Deck</span>
        </div>
        <nav className="rail-groups">
          <div className="rail-group">
            <span className="rail-group-label">Core</span>
            <a href="#agent-control" className="rail-link active">Dashboard</a>
            <a href="#operator-settings" className="rail-link">Agent</a>
          </div>
          <div className="rail-group">
            <span className="rail-group-label">Operations</span>
            <a href="#action-queues" className="rail-link">Queues</a>
            <a href="#mission-metrics" className="rail-link">Applications</a>
          </div>
          <div className="rail-group">
            <span className="rail-group-label">System</span>
            <a href="#activity-log" className="rail-link">Activity</a>
          </div>
        </nav>
      </div>

      <main className="command-deck">
        <CommandHeader agentRunning={agentRunning} pollFailures={pollFailures} />

        <section className="deck-section" id="agent-control" aria-labelledby="agent-control-title">
          <CommandSection title="Agent Control" subtitle="Start or stop the autonomous application worker." accent="brass">
            <div className="agent-control">
              <ConsoleButton variant="primary" className="btn btn-start" onClick={handleStart} disabled={agentRunning}>
                Start Agent
              </ConsoleButton>
              <ConsoleButton variant="danger" className="btn btn-stop" onClick={handleStop} disabled={!agentRunning}>
                Stop Agent
              </ConsoleButton>
            </div>
          </CommandSection>
        </section>

        <section className="deck-section" id="operator-settings" aria-labelledby="operator-settings-title">
          <CommandSection title="Operator Settings" subtitle="Application mode and fit-score threshold." accent="brass">
            {draftSettings ? (
              <div className="operator-settings">
                <label>
                  Mode
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
                  Minimum Fit Score
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
                  <p className="status-message info">Settings are saved but waiting for the daemon's next cycle to activate.</p>
                )}
                {operatorSettings && operatorSettings.daemon_active && !unsaved && (
                  <p className="status-message success">Settings are saved and currently active.</p>
                )}

                {unsaved && (
                  <div className="settings-actions">
                    <span className="unsaved-warning">You have unsaved changes.</span>
                    <ConsoleButton
                      variant="primary"
                      onClick={() => setShowModeConfirm(true)}
                      disabled={savingSettings || settingsInvalid}
                    >
                      Apply Changes
                    </ConsoleButton>
                    <ConsoleButton
                      variant="ghost"
                      onClick={() => {
                        setDraftSettings(operatorSettings);
                        setDraftScoreStr(operatorSettings?.minimum_fit_score.toString() ?? '');
                        setShowModeConfirm(false);
                        setActionError(null);
                      }}
                      disabled={savingSettings}
                    >
                      Discard Changes
                    </ConsoleButton>
                  </div>
                )}

                {showModeConfirm && (
                  <div className="modal-overlay">
                    <div className="modal confirm-modal">
                      <h3>Confirm Settings Change</h3>
                      {draftSettings.application_mode === 'find_only' && (
                        <p>
                          Career Agent will find and score jobs and list qualified matches.
                          <br />
                          It will not prepare documents, fill application forms, or submit applications.
                        </p>
                      )}
                      {draftSettings.application_mode === 'assisted' && (
                        <p>
                          Career Agent will prepare qualified applications and fill supported fields.
                          <br />
                          You must review and submit every application.
                          <br />
                          Career Agent will never click the employer's final Submit button.
                        </p>
                      )}
                      {draftSettings.application_mode === 'automatic' && (
                        <p>
                          <strong>Warning:</strong> Career Agent may click the employer's Submit button on supported forms.
                          <br />
                          CAPTCHA, login, legal-attestation, and unsupported cases still require you.
                        </p>
                      )}
                      <div className="modal-actions">
                        <ConsoleButton
                          variant="primary"
                          onClick={() => saveOperatorSettings(draftSettings)}
                          disabled={savingSettings || settingsInvalid}
                        >
                          {savingSettings ? 'Saving...' : 'Confirm'}
                        </ConsoleButton>
                        <ConsoleButton variant="ghost" onClick={() => setShowModeConfirm(false)} disabled={savingSettings}>
                          Cancel
                        </ConsoleButton>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <p className="detail-meta">Loading settings...</p>
            )}
          </CommandSection>
        </section>

        <section className="deck-section" id="action-queues">
          <div className="banner-grid">
            <div className="queue-banner" aria-label="Qualified Jobs">
              <div>
                <strong>Qualified jobs found: {qualifiedJobs.length}</strong>
                <span> Review jobs that met your minimum fit score.</span>
              </div>
              <ConsoleButton variant="secondary" className="btn btn-qualified" onClick={() => setShowQualified(true)}>
                View Qualified Jobs
              </ConsoleButton>
            </div>
            <div className="queue-banner" aria-label="Assisted Apply">
              <div>
                <strong>Assisted applications waiting: {metrics?.assisted_waiting ?? 0}</strong>
                <span> Complete the next safe human step without restarting the application.</span>
              </div>
              <ConsoleButton variant="primary" className="btn btn-assisted" onClick={() => setShowAssisted(true)}>
                Open Assisted Apply
              </ConsoleButton>
            </div>
          </div>
        </section>

        {actionError && <p className="action-error" role="alert">{actionError}</p>}
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

        {showQualified && (
          <section className="deck-section qualified-queue" aria-labelledby="qualified-heading">
            <CommandSection title="Qualified Jobs (Find Only Mode)" id="qualified-heading">
              <div className="queue-actions">
                <ConsoleButton variant="ghost" onClick={() => setShowQualified(false)}>
                  Return to dashboard
                </ConsoleButton>
              </div>
              {qualifiedJobs.length === 0 ? (
                <p className="detail-meta">No qualified jobs waiting.</p>
              ) : (
                qualifiedJobs.map((job) => (
                  <QualifiedJobCard
                    key={job.id}
                    job={job}
                    onOpen={() => qualifiedAction(job.id, 'open')}
                    onPromote={() => qualifiedAction(job.id, 'promote')}
                    onConfirmManual={() => setConfirmQualifiedJob(job)}
                    onSkip={() => qualifiedAction(job.id, 'skip')}
                  />
                ))
              )}
            </CommandSection>
          </section>
        )}

        {showAssisted && (
          <section className="deck-section assisted-queue" aria-labelledby="assisted-heading">
            <CommandSection title="Assisted Apply" id="assisted-heading">
              <div className="queue-actions">
                <ConsoleButton variant="ghost" onClick={() => setShowAssisted(false)}>
                  Return to dashboard
                </ConsoleButton>
              </div>
              {assistedJobs.length > 0 && (
                <div className="batch-controls">
                  <ConsoleButton
                    variant="primary"
                    onClick={startSelected}
                    disabled={selectedJobs.length === 0 || batchIndex !== null || assistedBrowserOpen}
                  >
                    Start Selected Applications
                  </ConsoleButton>
                  {batchIndex !== null && (
                    <>
                      <SystemBadge variant="info">Application {batchIndex + 1} of {selectedJobs.length}</SystemBadge>
                      <ConsoleButton variant="ghost" onClick={() => setStopAfterCurrent(true)}>
                        Stop After This Application
                      </ConsoleButton>
                      <ConsoleButton
                        variant="ghost"
                        onClick={nextSelected}
                        disabled={assistedBrowserOpen}
                      >
                        {assistedBrowserOpen ? 'Close Current Application First' : 'Open Next Selected Application'}
                      </ConsoleButton>
                    </>
                  )}
                </div>
              )}
              {assistedJobs.length === 0 ? (
                <p className="detail-meta">
                  There is nothing to complete right now. New handoffs will appear here when human action is needed.
                </p>
              ) : (
                assistedJobs.map((job) => (
                  <AssistedJobCard
                    key={job.id}
                    job={job}
                    selected={selectedJobs.includes(job.id)}
                    batchRunning={batchIndex !== null}
                    assistedBrowserOpen={assistedBrowserOpen}
                    onToggleSelect={toggleSelected}
                    onLaunch={launchAssisted}
                    onRevalidate={revalidateAssisted}
                    onContinue={requestContinue}
                    onConfirm={setConfirmJob}
                    onMarkNotFound={setConfirmNotFoundJob}
                    onOpenDocument={openDocument}
                  />
                ))
              )}
            </CommandSection>
          </section>
        )}

        {confirmJob && (
          <ConfirmDialog
            title="Confirm application received"
            titleId="confirm-title"
            onCancel={() => setConfirmJob(null)}
            actions={
              <ConfirmActions
                confirmLabel="Confirmed — Mark Applied"
                onConfirm={async () => {
                  const ok = await confirmApplied(confirmJob);
                  if (ok) setConfirmJob(null);
                }}
                onCancel={() => setConfirmJob(null)}
              />
            }
          >
            <p className="confirm-subject">{assistedJobLabel(confirmJob)}</p>
            {(confirmJob.duplicate_siblings ?? 0) > 0 && (
              <p className="assisted-duplicate-warning" role="alert">
                {`${confirmJob.duplicate_siblings} other queued application${confirmJob.duplicate_siblings === 1 ? ' shares' : 's share'} this company and role. `}
                Marking the wrong one applied removes it from the queue permanently and blocks a future attempt, so make sure this is the posting you actually submitted.
              </p>
            )}
            <p>Only mark this applied if the employer's site showed that your application was received or successfully submitted.</p>
          </ConfirmDialog>
        )}

        {confirmNotFoundJob && (
          <ConfirmDialog
            title="Mark posting as not found"
            titleId="confirm-not-found-title"
            onCancel={() => setConfirmNotFoundJob(null)}
            actions={
              <ConfirmActions
                confirmLabel="Posting Not Found / Expired"
                variant="danger"
                onConfirm={async () => {
                  const ok = await markAssistedNotFound(confirmNotFoundJob);
                  if (ok) setConfirmNotFoundJob(null);
                }}
                onCancel={() => setConfirmNotFoundJob(null)}
              />
            }
          >
            <p className="confirm-subject">{assistedJobLabel(confirmNotFoundJob)}</p>
            <p>
              Mark this posting as not found or expired. The job will leave the assisted queue and be recorded as an invalid URL.
            </p>
          </ConfirmDialog>
        )}

        {confirmQualifiedJob && (
          <ConfirmDialog
            title="Mark this job applied?"
            titleId="confirm-qualified-title"
            onCancel={() => setConfirmQualifiedJob(null)}
            actions={
              <ConfirmActions
                confirmLabel="Confirmed — Mark Applied"
                onConfirm={async () => {
                  try {
                    const res = await fetch('/api/qualified-jobs/confirm', {
                      method: 'POST',
                      headers: { 'Content-Type': 'application/json' },
                      body: JSON.stringify({ job_id: confirmQualifiedJob.id, confirmed_received: true }),
                    });
                    if (res.ok) fetchQualifiedJobs();
                    else setActionError('Failed to confirm job');
                  } catch (e) {
                    console.error(e);
                    setActionError('Error confirming job');
                  }
                  setConfirmQualifiedJob(null);
                }}
                onCancel={() => setConfirmQualifiedJob(null)}
              />
            }
          >
            <p>Only continue if the employer showed that your application was received or successfully submitted.</p>
          </ConfirmDialog>
        )}

        <section className="deck-section" id="mission-metrics">
          <CommandSection title="Mission Metrics" subtitle="Live counts from the job funnel." accent="brass">
            <div className="metrics-grid">
              <MetricDisplay value={metrics?.discovered || 0} label="In Queue" status="info" />
              <MetricDisplay value={metrics?.processing || 0} label="Processing" status="pending" />
              <MetricDisplay value={metrics?.applied || 0} label="Applied" status="active" />
              <MetricDisplay
                value={metrics?.skipped || 0}
                label="Skipped"
                status="pending"
                detail={explain('SKIPPED')}
              />
              <MetricDisplay
                value={metrics?.failed || 0}
                label="Failed"
                status="warning"
                detail={explainPair('FAILED_SCORE', metrics?.failed_score ?? 0, 'FAILED_SUBMIT', metrics?.failed_submit ?? 0)}
              />
              <MetricDisplay
                value={metrics?.manual_required || 0}
                label="Manual Queue"
                status="pending"
                detail={explainPair(
                  'MANUAL_REQUIRED',
                  metrics?.manual_required_only ?? 0,
                  'AWAITING_REVIEW',
                  metrics?.awaiting_review ?? 0
                )}
              />
              <MetricDisplay
                value={metrics?.blocked_captcha || 0}
                label="Blocked"
                status="warning"
                detail={explain('BLOCKED_CAPTCHA')}
              />
              <MetricDisplay
                value={metrics?.invalid_url || 0}
                label="Not A Posting"
                status="info"
                detail={explainInvalidURL(metrics?.invalid_url_malformed ?? 0, metrics?.invalid_url_expired ?? 0)}
              />
              <MetricDisplay
                value={metrics?.retry_exhausted || 0}
                label="Retry Exhausted"
                status="warning"
                detail={explain('RETRY_EXHAUSTED')}
              />
              <MetricDisplay
                value={metrics?.interview_rate_pct || '—'}
                label="Interview Rate"
                status="active"
                detail={`${metrics?.interviews ?? 0} interview${metrics?.interviews === 1 ? '' : 's'} and ${
                  metrics?.rejections ?? 0
                } rejection${metrics?.rejections === 1 ? '' : 's'} across ${
                  metrics?.total_applied_tracked ?? 0
                } tracked application${metrics?.total_applied_tracked === 1 ? '' : 's'}`}
              />
            </div>
          </CommandSection>
        </section>

        <section className="deck-section" id="activity-log">
          <CommandSection title="System Activity" subtitle="Recent operational events and queue health." accent="brass">
            <div className="detail-grid">
              <div className="detail mission-metrics">
                <h3>Mission Progress</h3>
                <dl>
                  <TelemetryRow label="Confirmed today">{metrics?.confirmed_today ?? 0}</TelemetryRow>
                  <TelemetryRow label="Confirmed last 7 days">{metrics?.confirmed_last_7_days ?? 0}</TelemetryRow>
                  <TelemetryRow label="Median first attempt">{metrics?.first_attempt_median ?? '—'}</TelemetryRow>
                  <TelemetryRow label="Since last confirmed">
                    {metrics?.last_confirmed_ago ?? 'No confirmed application yet'}
                  </TelemetryRow>
                  <TelemetryRow
                    label="Eligible queue"
                    meta={`(${metrics?.eligible_never_attempted ?? 0} never attempted)`}
                  >
                    {metrics?.eligible_queue ?? 0}
                  </TelemetryRow>
                  <TelemetryRow label="Discovery health" meta={metrics?.discovery_last_finished_at}>
                    {discoveryStatus()}
                  </TelemetryRow>
                </dl>
              </div>

              <div className="detail">
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
              </div>

              <div className="detail">
                <h3>Awaiting You</h3>
                {metrics?.last_manual_company ? (
                  <>
                    <p className="detail-job">{metrics.last_manual_company} — {metrics.last_manual_title}</p>
                    <p className="detail-reason">{metrics.last_manual_reason}</p>
                    <p className="detail-meta">
                      {metrics.last_manual_at}
                      {metrics.last_manual_processing_time && ` · ${metrics.last_manual_processing_time} in pipeline`}
                    </p>
                  </>
                ) : (
                  <p className="detail-meta">Nothing waiting on you</p>
                )}
              </div>

              <div className="detail">
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
              </div>

              <div className="detail">
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
              </div>
            </div>
          </CommandSection>
        </section>

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
      </main>
    </div>
  );
}

function assistedJobLabel(job: AssistedJob): string {
  const distinguisher = [job.location, job.requisition_id && `Req ${job.requisition_id}`].filter(Boolean).join(' · ');
  return distinguisher ? `${job.company} — ${job.role} (${distinguisher})` : `${job.company} — ${job.role}`;
}

export default App;
