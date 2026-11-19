import React from 'react';

export function FormFieldError({ message }) {
  if (!message) return null;
  return (
    <p className="text-[var(--error-text)] text-xs mt-1 font-medium">{message}</p>
  );
}
