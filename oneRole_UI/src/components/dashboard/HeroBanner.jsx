import React from 'react';
import { useApp } from '../../context/AppContext';

export function HeroBanner() {
  const { setActiveModal } = useApp();

  return (
    <div className="flex items-center justify-between py-2 border-b border-[#e8ece9] pb-4">
      <div>
        <h1 className="text-xl font-bold tracking-tight text-[#1d2825]">
          Automate your job applications.
        </h1>
        <p className="text-xs text-[#78827e] mt-0.5 max-w-2xl">
          Tailor your resume, ace interviews, and land a job. Fast. Our AI matches your skills to the best roles and applies with a plan.
        </p>
      </div>

      <button
        onClick={() => setActiveModal('plans')}
        className="px-4 py-1.5 rounded-full bg-[#152c25] hover:bg-[#254b3e] text-white text-xs font-semibold shadow-sm transition"
      >
        See plans &gt;
      </button>
    </div>
  );
}
