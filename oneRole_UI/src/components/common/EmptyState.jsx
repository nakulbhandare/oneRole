import React from 'react';

export function EmptyState({ children, className = '' }) {
  return (
    <div className={`p-8 text-center text-xs text-[#78827e] bg-white rounded-xl border border-[#e8ece9] ${className}`}>
      {children}
    </div>
  );
}
