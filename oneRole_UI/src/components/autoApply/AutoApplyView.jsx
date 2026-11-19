import React, { useState } from 'react';
import { useApp } from '../../context/AppContext';

export function AutoApplyView() {
  const { showToast } = useApp();
  const [enabled, setEnabled] = useState(true);
  const [minMatch, setMinMatch] = useState(85);
  const [dailyLimit, setDailyLimit] = useState(25);
  const [autoTailor, setAutoTailor] = useState(true);

  function handleSave() {
    try {
      localStorage.setItem('onerole_min_match_score', minMatch);
      localStorage.setItem('onerole_auto_apply_enabled', enabled);
      localStorage.setItem('onerole_daily_limit', dailyLimit);
      localStorage.setItem('onerole_auto_tailor', autoTailor);
    } catch (e) {}
    showToast('✓ Auto-Apply preferences saved successfully', 'success');
  }

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="pb-3 border-b border-[#e8ece9]">
        <h1 className="text-xl font-bold tracking-tight text-[#1d2825]">
          Autonomous Job Application Engine
        </h1>
        <p className="text-xs text-[#78827e] mt-0.5">
          Configure rules, match criteria, and limits for the 4-worker Chromium ATS submission daemon.
        </p>
      </div>

      {/* Main Switch Card */}
      <div className="bg-white p-5 rounded-xl border border-[#e8ece9] shadow-xs flex items-center justify-between">
        <div>
          <h3 className="text-sm font-bold text-[#1d2825]">Autonomous Submissions</h3>
          <p className="text-xs text-[#78827e] mt-0.5">
            When enabled, OneRole will automatically apply to new vacancies matching your criteria.
          </p>
        </div>

        <button
          onClick={() => {
            setEnabled(!enabled);
            showToast(enabled ? 'Auto-apply paused' : 'Auto-apply enabled', 'info');
          }}
          className={`w-12 h-6 flex items-center rounded-full p-1 transition-colors duration-200 ease-in-out ${enabled ? 'bg-[#152c25]' : 'bg-[#d1d5db]'}`}
        >
          <div className={`bg-white w-4 h-4 rounded-full shadow-md transform transition-transform duration-200 ease-in-out ${enabled ? 'translate-x-6' : 'translate-x-0'}`} />
        </button>
      </div>

      {/* SYSTEM STATUS (TSenta Alignment - Data Dense) */}
      <div className="grid grid-cols-3 gap-4 text-xs">
        <div className="bg-white p-4 rounded-xl border border-[#e8ece9] shadow-xs">
          <span className="text-[#78827e]">Engine Status</span>
          <p className="font-bold text-[#065f46] mt-1">Running (4 Workers)</p>
        </div>
        <div className="bg-white p-4 rounded-xl border border-[#e8ece9] shadow-xs">
          <span className="text-[#78827e]">Last Sync</span>
          <p className="font-bold text-[#1d2825] mt-1">2 mins ago</p>
        </div>
        <div className="bg-white p-4 rounded-xl border border-[#e8ece9] shadow-xs">
          <span className="text-[#78827e]">Stuck Apps</span>
          <p className="font-bold text-[#991b1b] mt-1">0</p>
        </div>
      </div>

      {/* Configuration Form */}
      <div className="bg-white p-5 rounded-xl border border-[#e8ece9] shadow-xs space-y-5 text-xs">
        <div>
          <label className="font-bold text-[#1d2825] block mb-1.5">
            Minimum ATS Match Threshold: <span className="text-[#152c25]">{minMatch}%</span>
          </label>
          <input
            type="range"
            min="60"
            max="98"
            value={minMatch}
            onChange={e => setMinMatch(Number(e.target.value))}
            className="w-full accent-[#152c25]"
          />
          <div className="flex justify-between text-[10px] text-[#9ca3af] mt-1">
            <span>60% (Broad)</span>
            <span>85% (Recommended)</span>
            <span>98% (Strict)</span>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-3 border-t border-[#e8ece9]">
          <div>
            <label className="font-bold text-[#1d2825] block mb-1">Maximum Daily Submissions</label>
            <input
              type="number"
              value={dailyLimit}
              onChange={e => setDailyLimit(Number(e.target.value))}
              className="w-full px-3 py-1.5 border border-[#e8ece9] rounded-lg outline-none focus:border-[#152c25]"
            />
          </div>

          <div>
            <label className="font-bold text-[#1d2825] block mb-1">Resume Adaptation</label>
            <label className="flex items-center space-x-2 mt-2 cursor-pointer">
              <input
                type="checkbox"
                checked={autoTailor}
                onChange={e => setAutoTailor(e.target.checked)}
                className="rounded text-[#152c25] focus:ring-0"
              />
              <span className="text-[#374151]">Dynamically tailor resume for each employer JD</span>
            </label>
          </div>
        </div>

        <div className="pt-3 border-t border-[#e8ece9] flex justify-end">
          <button
            onClick={handleSave}
            className="px-4 py-2 bg-[#152c25] hover:bg-[#254b3e] text-white text-xs font-bold rounded-lg shadow-sm transition"
          >
            Save Automation Settings
          </button>
        </div>
      </div>
    </div>
  );
}
