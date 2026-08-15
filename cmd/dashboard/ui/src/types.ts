export interface ConversionRow {
  total_applied: number;
  interviews: number;
  rejections: number;
  pending: number;
  interview_rate_pct: string;
}

export interface SourceConversionRow extends ConversionRow {
  source: string;
}

export interface VariantConversionRow extends ConversionRow {
  variant: string;
}

export interface DiscoverySourceCount {
  source: string;
  request_attempted: number;
  request_failed: number;
  circuit_open_skipped: number;
}

export interface Metrics {
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
  discovery_source_counts?: DiscoverySourceCount[];
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
  human_effort?: HumanEffortMetrics;
}

export interface OperatorSettings {
  application_mode: 'find_only' | 'assisted' | 'automatic';
  minimum_fit_score: number;
  scoring_active?: boolean;
  daemon_active?: boolean;
}

export interface QualifiedJob {
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

export interface AssistedAction {
  code: string;
  title: string;
  instruction: string;
  primary_button: string;
  requires_browser: boolean;
  documents_ready: boolean;
  requires_explicit_submit: boolean;
  can_continue: boolean;
}

export interface ApplicationQuestion {
  id: number;
  job_id: string;
  key: string;
  prompt: string;
  control_type: string;
  options?: string[];
  required: boolean;
  status: string;
  /**
   * 'routine' | 'sensitive' | 'generate_per_job'. A sensitive question needs a
   * second, separate acknowledgement before its answer may ever be reused, so
   * the UI must not collapse the two decisions into one checkbox.
   */
  sensitivity: string;
  /** Career Agent's proposal. Never an answer the operator already gave. */
  suggested?: string;
  source?: string;
  label_unsafe?: boolean;
  created_at: string;
}

export interface AssistedFillSummary {
  job_id: string;
  filled_count: number;
  reused_answers: number;
  documents: string[] | null;
  filled_labels: string[] | null;
  unresolved_count: number;
  recorded_at: string;
}

export interface AssistedEffort {
  band: string;
  low_minutes: number;
  high_minutes: number;
  signals?: string[];
}

export interface ApplySessionItem {
  id: number;
  position: number;
  job_id: string;
  company: string;
  role: string;
  state: string;
  terminal_reason?: string;
}

export interface ApplySession {
  id: number;
  state: 'running' | 'paused' | 'finished';
  auto_advance: boolean;
  stop_after_current: boolean;
  pause_reason?: string;
  current_job_id?: string;
  position: number;
  total: number;
  completed: number;
  confirmed: number;
  items: ApplySessionItem[];
}

export interface AnswerSubmission {
  key: string;
  answer: string;
  save_for_reuse: boolean;
  allow_sensitive_reuse: boolean;
  scope: string;
}

export interface PacketEntry {
  label: string;
  value: string;
  sensitive: boolean;
  /** Set only on this form's own questions: why there is nothing to copy. */
  status?: string;
  /** True when the entry is a question this employer's form actually asks. */
  from_this_form?: boolean;
}

/**
 * What Career Agent knows about the employer's own form.
 *
 * `state` is carried explicitly and is never derived from `question_count`.
 * A form that was read and asks nothing extra and a form nobody has ever
 * opened both produce zero questions, and telling the operator those are the
 * same thing is the defect this field exists to close (bugs.md #547).
 */
export type FormInventoryState = 'not_prepared' | 'preparing' | 'ready' | 'failed';

export interface FormInventory {
  state: FormInventoryState;
  question_count: number;
  /** The whole form's control count, which is larger than question_count. */
  field_count: number;
  /** A bounded code, never a driver message. */
  reason?: string;
  inspected_at?: string;
  source?: 'preflight' | 'assisted_session';
  /** False when asking for an inspection now would be refused anyway. */
  preparable: boolean;
  stale?: boolean;
}

export interface DocumentSummary {
  kind: string;
  ready: boolean;
  headline: string;
  changes: string[] | null;
  note?: string;
}

export interface HumanEffortMetrics {
  applications_confirmed: number;
  median_human_seconds: number;
  median_answering_seconds: number;
  fields_auto_filled: number;
  fields_needing_human: number;
  approved_answers_reused: number;
  auto_fill_rate_pct: string;
  mean_unresolved_per_application: number;
  sessions_completed: number;
  sessions_abandoned: number;
  applications_per_session: number;
  browser_handoffs: number;
  mapping_cache_hit_pct: string;
}

export interface AssistedJob {
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
  apply_url?: string;
  location?: string;
  requisition_id?: string;
  duplicate_siblings?: number;
  ambiguous?: boolean;
  completed: AssistedFillSummary;
  questions?: ApplicationQuestion[] | null;
  effort: AssistedEffort;
}

/**
 * Application Knowledge — what Career Agent needs to know across the whole
 * queue, rather than for one open application.
 */

/** The five reuse policies, derived by the server from what it stores. */
export type KnowledgePolicy =
  | 'safe_auto_fill'
  | 'approved_reusable'
  | 'suggest_ask'
  | 'human_review'
  | 'generate_per_job'
  | 'unknown';

/** One question, however many applications are waiting on it. */
export interface KnowledgeGroup {
  key: string;
  prompt: string;
  phrasings: string[];
  occurrences: number;
  applications: number;
  job_ids: string[];
  companies: string[];
  control_type: string;
  options?: string[];
  /** Employers offer genuinely different choices; one answer cannot serve all. */
  options_vary?: boolean;
  required: boolean;
  sensitivity: string;
  policy: KnowledgePolicy;
  suggested?: string;
  source?: string;
  resolved: boolean;
  company_scope_available: boolean;
  company_scope?: string;
  skill_subject?: string;
}

/** Demand-driven readiness, measured against the queue and nothing else. */
export interface KnowledgeReadiness {
  applications: number;
  fields: number;
  resolved: number;
  unresolved: number;
  unique_questions: number;
  sensitive_decisions: number;
  per_job_responses: number;
  fields_unlockable: number;
  answers_needed: number;
  applications_blocked: number;
}

export interface PreflightResult {
  job_id: string;
  company: string;
  role: string;
  state: 'inspected' | 'unavailable';
  reason?: string;
  ats?: string;
  control_count: number;
  inspected_at: string;
}

export interface KnowledgeSnapshot {
  readiness: KnowledgeReadiness;
  groups: KnowledgeGroup[];
  preflight: PreflightResult[];
}

export interface KnowledgeApproval {
  group_key: string;
  answer: string;
  save_for_reuse: boolean;
  allow_sensitive_reuse: boolean;
  confirmed_equivalent: boolean;
  scope: string;
}

export interface KnowledgeApproveResult {
  saved: boolean;
  answer_id?: number;
  canonical_question?: string;
  aliases_added: number;
  questions_resolved: number;
  applications_helped: number;
  still_unresolved: number;
}

export interface PreflightStatus {
  running: boolean;
  applications: number;
  started_at?: string;
  results: PreflightResult[];
}

/** One editable fact from pii.yaml. */
export interface ProfileField {
  key: string;
  label: string;
  value: string;
  section: string;
  sensitive?: boolean;
  note?: string;
}

export interface ProfileSnapshot {
  sections: string[];
  fields: ProfileField[];
  path: string;
}

/** One answer in the vault, as the management view shows it. */
export interface VaultAnswer {
  id: number;
  question: string;
  answer: string;
  sensitivity: string;
  scope: string;
  reuse_allowed: boolean;
  provenance: string;
  use_count: number;
  policy: KnowledgePolicy;
  aliases: string[];
  last_used_at?: string;
}
