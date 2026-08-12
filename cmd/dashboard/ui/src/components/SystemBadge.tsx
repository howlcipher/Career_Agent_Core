import type { ReactNode } from 'react';

interface SystemBadgeProps {
  children: ReactNode;
  variant?: 'default' | 'active' | 'pending' | 'warning' | 'info';
}

export function SystemBadge({ children, variant = 'default' }: SystemBadgeProps) {
  return <span className={`system-badge variant-${variant}`}>{children}</span>;
}
