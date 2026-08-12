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
}
