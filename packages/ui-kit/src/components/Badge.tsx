import React from 'react';

export interface BadgeProps {
  status: string;
  label: string;
}

type ColorScheme = {
  bg: string;
  text: string;
  dot: string;
};

const STATUS_COLORS: Record<string, ColorScheme> = {
  active:      { bg: 'bg-green-100',  text: 'text-green-800',  dot: 'bg-green-500' },
  completed:   { bg: 'bg-green-100',  text: 'text-green-800',  dot: 'bg-green-500' },
  approved:    { bg: 'bg-green-100',  text: 'text-green-800',  dot: 'bg-green-500' },
  rejected:    { bg: 'bg-red-100',    text: 'text-red-800',    dot: 'bg-red-500' },
  manual_review: { bg: 'bg-yellow-100', text: 'text-yellow-800', dot: 'bg-yellow-500' },
  draft:       { bg: 'bg-gray-100',   text: 'text-gray-700',   dot: 'bg-gray-400' },
  pending:     { bg: 'bg-gray-100',   text: 'text-gray-700',   dot: 'bg-gray-400' },
  risk_scoring:              { bg: 'bg-blue-100', text: 'text-blue-800', dot: 'bg-blue-500' },
  validation_in_progress:    { bg: 'bg-blue-100', text: 'text-blue-800', dot: 'bg-blue-500' },
};

const DEFAULT_COLOR: ColorScheme = {
  bg: 'bg-gray-100',
  text: 'text-gray-700',
  dot: 'bg-gray-400',
};

export function Badge({ status, label }: BadgeProps) {
  const colors = STATUS_COLORS[status] ?? DEFAULT_COLOR;

  return (
    <span
      className={[
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
        colors.bg,
        colors.text,
      ].join(' ')}
    >
      <span className={['h-1.5 w-1.5 rounded-full', colors.dot].join(' ')} aria-hidden="true" />
      {label}
    </span>
  );
}
