import React from 'react';

export interface CardProps {
  title?: string;
  children: React.ReactNode;
  className?: string;
}

export function Card({ title, children, className = '' }: CardProps) {
  return (
    <div
      className={[
        'rounded-xl shadow-sm border border-gray-100 bg-white p-6',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      {title && (
        <h3 className="mb-4 text-base font-semibold text-gray-900">{title}</h3>
      )}
      {children}
    </div>
  );
}
