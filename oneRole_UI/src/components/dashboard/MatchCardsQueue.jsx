import React from 'react';
import { MatchCard } from './MatchCard';
import { useApp } from '../../context/AppContext';

export function MatchCardsQueue() {
  const {
    activeCards,
    resetMatchQueue,
    handleApplyToAll5,
    setActiveModal,
    setCurrentTab
  } = useApp();

  return (
    <div className="space-y-3">
      {/* Top Header Row */}
      <div className="flex items-center justify-between">
        <h2 className="text-[17px] font-bold text-[#1d2825]">Top job matches</h2>

        <div className="flex items-center space-x-2 text-xs">
          <button
            onClick={resetMatchQueue}
            className="px-2.5 py-1 text-xs text-[#49554f] font-semibold hover:text-[#1d2825] transition"
            title="Reload verified vacancies from MongoDB"
          >
            ↺ Reload
          </button>

          <button
            onClick={() => setActiveModal('addRole')}
            className="px-2.5 py-1 text-xs text-[#49554f] font-semibold hover:text-[#1d2825] transition"
          >
            + Add your own
          </button>

          <button
            onClick={() => {
              setCurrentTab('browse');
              const el = document.getElementById('catalogHeading');
              if (el) el.scrollIntoView({ behavior: 'smooth' });
            }}
            className="px-2.5 py-1 text-xs text-[#49554f] font-semibold hover:text-[#1d2825] transition"
          >
            Browse jobs
          </button>

          <button
            onClick={handleApplyToAll5}
            disabled={activeCards.length === 0}
            className={`px-3.5 py-1.5 bg-[#152c25] hover:bg-[#254b3e] text-white text-xs font-bold rounded-[7px] flex items-center space-x-1.5 shadow-sm transition ${activeCards.length === 0 ? 'opacity-40 cursor-not-allowed' : ''}`}
            title="Submit applications to all 5 match cards in parallel"
          >
            <span>⚡</span>
            <span>Apply to all 5</span>
            <span>→</span>
          </button>
        </div>
      </div>

      {/* Cards Row / Empty State */}
      {activeCards.length === 0 ? (
        <div className="w-full card p-8 text-center text-[#78827e] text-xs bg-[#fafbfa] rounded-[10px] border border-[#e8ece9] flex flex-col items-center justify-center space-y-3">
          <div className="font-medium text-[#2d3a33]">
            ✓ All queued job matches have been submitted to employer ATS portals and confirmed in your Application Tracker!
          </div>
          <button
            onClick={resetMatchQueue}
            className="px-4 py-2 bg-[#13382c] text-white text-xs font-bold rounded-[7px] hover:bg-[#1a4a3a] transition shadow-sm flex items-center space-x-1.5"
          >
            <span>↺</span>
            <span>Reload 5 Verified Matches</span>
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-3.5">
          {activeCards.map(card => (
            <MatchCard key={card.key} card={card} />
          ))}
        </div>
      )}
    </div>
  );
}
