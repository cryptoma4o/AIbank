import React from 'react';

export type AlertType = 'info' | 'success' | 'warning' | 'error';

export interface AlertProps {
  type: AlertType;
  message: string;
  onDismiss?: () => void;
}

type AlertStyle = {
  container: string;
  icon: string;
};

const STYLES: Record<AlertType, AlertStyle> = {
  info: {
    container: 'bg-blue-50 border-blue-200 text-blue-800',
    icon: 'ℹ️',
  },
  success: {
    container: 'bg-green-50 border-green-200 text-green-800',
    icon: '✅',
  },
  warning: {
    container: 'bg-yellow-50 border-yellow-200 text-yellow-800',
    icon: '⚠️',
  },
  error: {
    container: 'bg-red-50 border-red-200 text-red-800',
    icon: '❌',
  },
};

export function Alert({ type, message, onDismiss }: AlertProps) {
  const { container, icon } = STYLES[type];

  return (
    <div
      role="alert"
      className={[
        'flex items-start gap-3 rounded-xl border px-4 py-3 text-sm',
        container,
      ].join(' ')}
    >
      <span aria-hidden="true" className="mt-0.5 shrink-0 text-base leading-none">
        {icon}
      </span>

      <span className="flex-1">{message}</span>

      {onDismiss && (
        <button
          onClick={onDismiss}
          aria-label="Dismiss alert"
          className="ml-auto shrink-0 rounded p-0.5 opacity-60 hover:opacity-100 transition-opacity"
        >
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
            <path
              d="M11 3L3 11M3 3l8 8"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
            />
          </svg>
        </button>
      )}
    </div>
  );
}
