import React from 'react';
import { useApp } from '../../context/AppContext';

export function ToastContainer() {
  const { toasts } = useApp();

  if (toasts.length === 0) return null;

  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col space-y-2 pointer-events-none">
      {toasts.map(toast => {
        const bgColors = {
          success: 'bg-[#152c25] text-white border border-[#2d5646]',
          error: 'bg-[#991b1b] text-white border border-[#b91c1c]',
          info: 'bg-[#1f2937] text-white border border-[#374151]'
        }[toast.type] || 'bg-[#152c25] text-white';

        return (
          <div
            key={toast.id}
            className={`px-4 py-2.5 rounded-lg text-xs font-semibold shadow-xl flex items-center space-x-2.5 transition-all transform translate-y-0 opacity-100 ${bgColors}`}
          >
            <span>{toast.message}</span>
          </div>
        );
      })}
    </div>
  );
}
