import type { ReactNode } from 'react';

interface MetricDisplayProps {
  value: ReactNode;
  label: string;
  status?: 'default' | 'active' | 'pending' | 'warning' | 'info';
  detail?: ReactNode;
  className?: string;
}

export function MetricDisplay({ value, label, status = 'default', detail, className = '' }: MetricDisplayProps) {
  return (
    <article className={`metric-display status-${status} ${className}`}>
      <div className="metric-value">{value}</div>
      <div className="metric-label">{label}</div>
      {detail && <div className="metric-detail">{detail}</div>}
    </article>
  );
}
