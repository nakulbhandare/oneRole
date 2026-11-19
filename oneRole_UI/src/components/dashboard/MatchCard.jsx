import React from 'react';
import { CompanyLogo } from '../common/CompanyLogo';
import { useApp } from '../../context/AppContext';

export function MatchCard({ card }) {
  const { handlePassCard, handleApplyCard, setSelectedJob, setActiveModal } = useApp();

  const themeStyles = {
    'card-mint': 'bg-[#cef3df] border-[#b5ebd0] hover:border-[#a0dcc5]',
    'card-pink': 'bg-[#ffdce4] border-[#fbc7d2] hover:border-[#f9b5c3]',
    'card-aqua': 'bg-[#d1f5ea] border-[#b8f0df] hover:border-[#a0e8d2]',
    'card-peach': 'bg-[#fde6ca] border-[#fad7b1] hover:border-[#f8c898]',
    'card-butter': 'bg-[#fef0be] border-[#fde499] hover:border-[#fcd97d]'
  }[card.theme] || 'bg-[#cef3df] border-[#b5ebd0]';

  return (
    <div
      className={`job-match-card ${card.theme || 'card-mint'} rounded-[18px] p-4 flex flex-col justify-between border transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md cursor-pointer ${themeStyles}`}
      style={{ minHeight: '190px' }}
      onClick={() => {
        setSelectedJob(card.rawJob || card);
        setActiveModal('jobDetail');
      }}
    >
      {/* Top Header: Location, Badge, ATS Match Circle */}
      <div className="flex items-start justify-between">
        <div className="min-w-0 pr-2">
          <div className="flex items-center space-x-1.5 flex-wrap">
            <span className="text-[11px] text-[#55635c] font-medium truncate max-w-[125px]">
              {card.location || 'San Francisco, CA'}
            </span>
            {card.badge && (
              <span className="inline-flex items-center space-x-0.5 px-1.5 py-0.5 rounded-full bg-white/70 text-[9px] font-bold text-[#42504a]">
                <span>👤</span>
                <span>{card.badge}</span>
              </span>
            )}
          </div>
          <span className="text-[10px] text-[#7d8b84] font-medium block mt-0.5">
            {card.time || 'Today'}
          </span>
        </div>

        {/* ATS Match Score Circle Badge */}
        <div className="w-10 h-10 rounded-full border-[1.8px] border-black/80 bg-white/50 flex flex-col items-center justify-center flex-shrink-0">
          <span className="text-[11px] font-black text-black leading-none">{card.score}%</span>
          <span className="text-[7px] font-black text-black uppercase tracking-tighter leading-none mt-0.5">MATCH</span>
        </div>
      </div>

      {/* Middle: Job Title */}
      <div className="my-3">
        <h3
          className="text-[13px] font-bold text-[#14231d] leading-snug line-clamp-2"
          title={card.title}
        >
          {card.title}
        </h3>
      </div>

      {/* Bottom Footer: Company Logo & Actions */}
      <div className="flex items-center justify-between pt-2 border-t border-black/5" onClick={e => e.stopPropagation()}>
        <div className="flex items-center space-x-2 truncate max-w-[100px]">
          <CompanyLogo company={card.company} domain={card.domain} size="sm" />
          <span className="text-xs font-bold text-[#1d2825] truncate">{card.company}</span>
        </div>

        <div className="flex items-center space-x-1 flex-shrink-0">
          <button
            onClick={(e) => {
              e.stopPropagation();
              window.open(card.sourceUrl, '_blank');
            }}
            className="px-2.5 py-1 text-[11px] font-semibold text-[#374151] hover:text-[#111827] bg-white border border-black/5 hover:bg-gray-50 rounded-full transition shadow-sm focus:outline-none"
            title="View in ATS portal"
          >
            View
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              handlePassCard(card.key);
            }}
            className="px-2.5 py-1 text-[11px] font-semibold text-[#374151] hover:text-[#111827] bg-white border border-black/5 hover:bg-gray-50 rounded-full transition shadow-sm focus:outline-none focus:ring-2 focus:ring-[#174a38] focus:ring-opacity-50"
            title="Pass on this job"
          >
            Pass
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              handleApplyCard(card);
            }}
            className="btn-card-apply px-3 py-1 text-[11px] font-bold text-white bg-[#18181b] hover:bg-black rounded-full transition shadow-sm flex items-center space-x-1 focus:outline-none focus:ring-2 focus:ring-[#174a38] focus:ring-opacity-50"
            title="Auto apply to this job"
          >
            <span>Apply</span>
          </button>
        </div>
      </div>
    </div>
  );
}
