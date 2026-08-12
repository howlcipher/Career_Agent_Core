import type { ButtonHTMLAttributes, ReactNode } from 'react';

export type ConsoleButtonVariant = 'primary' | 'secondary' | 'danger' | 'caution' | 'ghost';

interface ConsoleButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ConsoleButtonVariant;
  children: ReactNode;
}

export function ConsoleButton({
  variant = 'secondary',
  children,
  className = '',
  ...rest
}: ConsoleButtonProps) {
  return (
    <button type="button" className={`console-btn variant-${variant} ${className}`} {...rest}>
      {children}
    </button>
  );
}
