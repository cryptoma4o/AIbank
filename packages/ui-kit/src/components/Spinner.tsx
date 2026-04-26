import React from 'react';

export type SpinnerSize = 'sm' | 'md' | 'lg';

export interface SpinnerProps {
  size?: SpinnerSize;
  className?: string;
}

const sizeMap: Record<SpinnerSize, { svg: string; stroke: number }> = {
  sm: { svg: '16', stroke: 2 },
  md: { svg: '24', stroke: 2.5 },
  lg: { svg: '40', stroke: 3 },
};

export function Spinner({ size = 'md', className = '' }: SpinnerProps) {
  const { svg, stroke } = sizeMap[size];
  const r = (Number(svg) - stroke * 2) / 2;
  const cx = Number(svg) / 2;
  const circumference = 2 * Math.PI * r;

  return (
    <svg
      width={svg}
      height={svg}
      viewBox={`0 0 ${svg} ${svg}`}
      fill="none"
      aria-label="Loading"
      role="status"
      className={['animate-spin', className].filter(Boolean).join(' ')}
    >
      {/* Track */}
      <circle
        cx={cx}
        cy={cx}
        r={r}
        stroke="currentColor"
        strokeWidth={stroke}
        strokeOpacity={0.2}
      />
      {/* Arc */}
      <circle
        cx={cx}
        cy={cx}
        r={r}
        stroke="currentColor"
        strokeWidth={stroke}
        strokeLinecap="round"
        strokeDasharray={`${circumference * 0.75} ${circumference * 0.25}`}
        strokeDashoffset={0}
      />
    </svg>
  );
}
