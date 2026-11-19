import React from 'react';

export function FormError({ message }) {
  if (!message) return null;
  return (
    <div className="text-[var(--error-text)] text-xs font-semibold bg-[var(--error-bg)] border border-[var(--error-border)] px-3 py-2 rounded-lg mt-1">
      {message}
    </div>
  );
}
