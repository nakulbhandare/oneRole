import React from 'react';
import { CompanyLogo } from '../common/CompanyLogo';
import { useApp } from '../../context/AppContext';
import { computeRealtimeATSScore } from '../../services/atsEngine';

export function JobDetailModal() {
  const { selectedJob, setSelectedJob, activeModal, setActiveModal, handleApplyCard } = useApp();

  if (activeModal !== 'jobDetail' || !selectedJob) return null;

  const { score: atsScore, matchedSkills } = computeRealtimeATSScore(selectedJob);
  const domain = (selectedJob.company || '').toLowerCase().replace(/[^a-z0-9]/g, '') + '.com';

  function handleClose() {
    setActiveModal(null);
    setSelectedJob(null);
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40 backdrop-blur-xs">
      <div className="bg-white rounded-2xl max-w-2xl w-full max-h-[85vh] flex flex-col shadow-2xl border border-[#e8ece9] overflow-hidden">
        {/* Modal Header */}
        <div className="p-5 border-b border-[#e8ece9] flex items-start justify-between bg-[#fafbfa]">
          <div className="flex items-start space-x-3.5 pr-4">
            <CompanyLogo company={selectedJob.company} domain={domain} size="lg" />
            <div>
              <h2 className="text-base font-bold text-[#1d2825] leading-snug">
                {selectedJob.title || selectedJob.canonical_title}
              </h2>
              <p className="text-xs text-[#55635c] mt-0.5">
                {selectedJob.company} · {selectedJob.location || selectedJob.display_location || 'Remote'}
              </p>
            </div>
          </div>

          <div className="flex items-center space-x-2">
            <div className="w-10 h-10 rounded-full bg-[#eaf4ee] text-[#174a38] border border-[#c2decc] flex flex-col items-center justify-center">
              <span className="text-[11px] font-black leading-none">{atsScore}%</span>
              <span className="text-[7px] font-bold uppercase mt-0.5">ATS</span>
            </div>

            <button
              onClick={handleClose}
              className="p-1.5 text-gray-400 hover:text-gray-700 hover:bg-gray-100 rounded-lg transition"
            >
              ✕
            </button>
          </div>
        </div>

        {/* Modal Body / Job Description */}
        <div className="p-5 overflow-y-auto space-y-4 text-xs text-[#374151] leading-relaxed">
          <div>
            <h4 className="font-bold text-[#1d2825] mb-1">Role Description</h4>
            {selectedJob.description_html ? (
              <div
                className="prose prose-xs max-w-none text-[#4b5563]"
                dangerouslySetInnerHTML={{ __html: selectedJob.description_html }}
              />
            ) : (
              <p className="whitespace-pre-line text-[#4b5563]">
                {selectedJob.description || selectedJob.description_text || 'Verified job listing stored in MongoDBonerole database.'}
              </p>
            )}
          </div>

          {selectedJob.responsibilities_text && (
            <div>
              <h4 className="font-bold text-[#1d2825] mb-1">Responsibilities</h4>
              <p className="whitespace-pre-line text-[#4b5563]">
                {selectedJob.responsibilities_text}
              </p>
            </div>
          )}

          {matchedSkills.length > 0 && (
            <div>
              <h4 className="font-bold text-[#1d2825] mb-1">Matched Skills</h4>
              <div className="flex flex-wrap gap-1">
                {matchedSkills.map(skill => (
                  <span key={skill} className="bg-[#eaf4ee] text-[#174a38] text-[10px] px-2 py-0.5 rounded-full font-medium">
                    {skill}
                  </span>
                ))}
              </div>
            </div>
          )}

          {/* MOCK TAILORED DIFF VIEW (TSenta Alignment) */}
          <div className="bg-[#f9fafb] p-4 rounded-lg border border-[#e5e7eb]">
            <h4 className="font-bold text-[#1d2825] mb-2 flex items-center justify-between">
              <span>AI Tailored Application Preview</span>
              <span className="text-[10px] bg-[#d1fae5] text-[#065f46] px-1.5 py-0.5 rounded">Tailored</span>
            </h4>
            <div className="grid grid-cols-2 gap-4 text-[10px]">
              <div className="space-y-1">
                <span className="text-[#9ca3af] font-bold">ORIGINAL</span>
                <p className="bg-red-50 text-red-800 p-2 rounded">Basic applicant with standard experience...</p>
              </div>
              <div className="space-y-1">
                <span className="text-[#9ca3af] font-bold">TAILORED</span>
                <p className="bg-green-50 text-green-800 p-2 rounded">Experienced engineer with focus on distributed systems...</p>
              </div>
            </div>
          </div>

          {selectedJob.qualifications_text && (
            <div>
              <h4 className="font-bold text-[#1d2825] mb-1">Qualifications</h4>
              <p className="whitespace-pre-line text-[#4b5563]">
                {selectedJob.qualifications_text}
              </p>
            </div>
          )}
        </div>

        {/* Modal Footer */}
        <div className="p-4 border-t border-[#e8ece9] flex items-center justify-between bg-[#fafbfa]">
          <span className="text-xs text-[#78827e]">
            Candidate match evaluated for <span className="font-semibold text-[#1d2825]">Nakul Bhandare</span>
          </span>

          <div className="flex items-center space-x-2">
            <button
              onClick={handleClose}
              className="px-3.5 py-1.5 border border-[#d1d5db] text-[#374151] hover:bg-gray-50 rounded-lg text-xs font-semibold"
            >
              Close
            </button>

            <button
              onClick={() => {
                handleApplyCard({
                  key: selectedJob.id,
                  id: selectedJob.id,
                  company: selectedJob.company,
                  title: selectedJob.title || selectedJob.canonical_title,
                  rawJob: selectedJob
                });
                handleClose();
              }}
              className="px-4 py-1.5 bg-[#152c25] hover:bg-[#254b3e] text-white rounded-lg text-xs font-bold shadow-sm transition"
            >
              ⚡ Auto Apply Now
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
