import type { ReactNode } from 'react';

export type StatusKind = 'active' | 'pending' | 'warning' | 'offline' | 'info';

interface StatusIndicatorProps {
  kind: StatusKind;
  label?: ReactNode;
  pulse?: boolean;
  className?: string;
}

const kindLabel: Record<StatusKind, string> = {
  active: 'active',
  pending: 'pending',
  warning: 'warning',
  offline: 'offline',
  info: 'info',
};

export function StatusIndicator({ kind, label, pulse = false, className = '' }: StatusIndicatorProps) {
  const text = label ?? kindLabel[kind];
  return (
    <span className={`status-indicator kind-${kind} ${pulse ? 'pulse' : ''} ${className}`}>
      <span className="status-lamp" aria-hidden="true" />
      <span className="status-label">{text}</span>
    </span>
  );
}
