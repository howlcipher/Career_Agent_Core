import type { ReactNode } from 'react';
import { ConsoleButton } from './ConsoleButton';

interface ConfirmDialogProps {
  title: string;
  titleId: string;
  children: ReactNode;
  onCancel: () => void;
  actions: ReactNode;
}

export function ConfirmDialog({ title, titleId, children, onCancel, actions }: ConfirmDialogProps) {
  return (
    <div className="confirm-backdrop" role="presentation" onClick={onCancel}>
      <section
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id={titleId}>{title}</h2>
        <div className="confirm-body">{children}</div>
        <div className="confirm-actions">{actions}</div>
      </section>
    </div>
  );
}

interface ConfirmActionsProps {
  confirmLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
  disabled?: boolean;
  variant?: 'primary' | 'danger';
}

export function ConfirmActions({ confirmLabel, onConfirm, onCancel, disabled, variant = 'primary' }: ConfirmActionsProps) {
  return (
    <>
      <ConsoleButton variant={variant} onClick={onConfirm} disabled={disabled}>{confirmLabel}</ConsoleButton>
      <ConsoleButton variant="ghost" onClick={onCancel} disabled={disabled}>Cancel</ConsoleButton>
    </>
  );
}
