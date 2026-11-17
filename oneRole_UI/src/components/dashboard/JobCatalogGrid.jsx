import React, { useState } from 'react';
import { CompanyLogo } from '../common/CompanyLogo';
import { useApp } from '../../context/AppContext';
import { computeRealtimeATSScore } from '../../services/atsEngine';

export function JobCatalogGrid() {
  const { jobs, loading, handleApplyCard, setSelectedJob, setActiveModal } = useApp();
  const [currentPage, setCurrentPage] = useState(1);
  const rowsPerPage = 12;

  const totalPages = Math.ceil(jobs.length / rowsPerPage) || 1;
  const paginatedJobs = jobs.slice((currentPage - 1) * rowsPerPage, currentPage * rowsPerPage);

  return (
    <div className="space-y-4 pt-4 border-t border-[#e8ece9]" id="catalogHeading">
      {/* Feed Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-[17px] font-bold text-[#1d2825]">
            Live Verified Vacancies
          </h2>
          <p className="text-xs text-[#78827e]">
            {loading ? 'Refreshing database query...' : `Showing ${jobs.length} open roles matching your search criteria from MongoDB.`}
          </p>
        </div>
      </div>

      {/* Grid of Verified Vacancies */}
      {jobs.length === 0 && !loading ? (
        <div className="p-8 text-center text-xs text-[#78827e] bg-white rounded-xl border border-[#e8ece9]">
          No jobs found matching your filters. Try clearing tags or selecting different locations.
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3.5">
          {paginatedJobs.map((job, idx) => {
            const atsScore = computeRealtimeATSScore(job);
            const domain = (job.company || '').toLowerCase().replace(/[^a-z0-9]/g, '') + '.com';

            return (
              <div
                key={job.id || idx}
                onClick={() => {
                  setSelectedJob(job);
                  setActiveModal('jobDetail');
                }}
                className="bg-white rounded-[12px] p-4 border border-[#e8ece9] hover:border-[#152c25] hover:shadow-sm transition cursor-pointer flex flex-col justify-between"
                style={{ minHeight: '160px' }}
              >
                <div>
                  {/* Top: Company Logo + Score */}
                  <div className="flex items-start justify-between mb-2">
                    <div className="flex items-center space-x-2.5 truncate pr-2">
                      <CompanyLogo company={job.company} domain={domain} size="md" />
                      <div className="min-w-0">
                        <span className="text-xs font-bold text-[#1d2825] truncate block">
                          {job.company}
                        </span>
                        <span className="text-[11px] text-[#78827e] truncate block">
                          {job.location || job.display_location || 'United States'}
                        </span>
                      </div>
                    </div>

                    <div className="w-8 h-8 rounded-full bg-[#eaf4ee] text-[#174a38] border border-[#c2decc] flex flex-col items-center justify-center flex-shrink-0">
                      <span className="text-[10px] font-black leading-none">{atsScore}%</span>
                    </div>
                  </div>

                  {/* Title */}
                  <h3 className="text-[13px] font-bold text-[#1d2825] leading-snug line-clamp-2 mt-1">
                    {job.title || job.canonical_title}
                  </h3>
                </div>

                {/* Footer: Tags + Apply Button */}
                <div className="flex items-center justify-between pt-3 mt-2 border-t border-[#f3f4f6]" onClick={e => e.stopPropagation()}>
                  <div className="flex items-center space-x-1.5 flex-wrap">
                    {(job.workplace_modes || ['Remote']).slice(0, 2).map((m, i) => (
                      <span key={i} className="text-[10px] px-2 py-0.5 rounded bg-[#f4f6f4] text-[#4b5563] font-medium">
                        {m}
                      </span>
                    ))}
                  </div>

                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleApplyCard({
                        key: job.id,
                        id: job.id,
                        company: job.company,
                        title: job.title || job.canonical_title,
                        rawJob: job
                      });
                    }}
                    className="px-3 py-1 bg-[#152c25] hover:bg-[#254b3e] text-white text-xs font-bold rounded-[6px] shadow-2xs transition"
                  >
                    Apply
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Pagination Controls */}
      {totalPages > 1 && (
        <div className="flex items-center justify-center space-x-2 pt-4">
          <button
            onClick={() => setCurrentPage(p => Math.max(p - 1, 1))}
            disabled={currentPage === 1}
            className="px-3 py-1 text-xs border rounded-md disabled:opacity-40 bg-white hover:bg-gray-50"
          >
            Previous
          </button>
          <span className="text-xs text-[#78827e]">
            Page {currentPage} of {totalPages}
          </span>
          <button
            onClick={() => setCurrentPage(p => Math.min(p + 1, totalPages))}
            disabled={currentPage === totalPages}
            className="px-3 py-1 text-xs border rounded-md disabled:opacity-40 bg-white hover:bg-gray-50"
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}
