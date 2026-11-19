import React, { useState } from 'react';
import { CompanyLogo } from '../common/CompanyLogo';
import { useApp } from '../../context/AppContext';
import { EmptyState } from '../common/EmptyState';


export function ApplicationTracker() {
  const { applications, userProfile, assistApplication } = useApp();
  const [activeTab, setActiveTab] = useState('all');
  const [searchCompany, setSearchCompany] = useState('');

  const inFlightCount = applications.filter(a => a.status === 'in_flight' || a.status === 'submitting').length;
  const needsYouCount = applications.filter(a => a.status === 'needs_you').length;
  const failedCount = applications.filter(a => a.status === 'failed').length;
  const skippedCount = applications.filter(a => a.status === 'skipped').length;

  const filteredApps = applications.filter(app => {
    if (activeTab === 'in_flight' && app.status !== 'in_flight' && app.status !== 'submitting') return false;
    if (activeTab === 'needs_you' && app.status !== 'needs_you') return false;
    if (activeTab === 'failed' && app.status !== 'failed') return false;
    if (activeTab === 'skipped' && app.status !== 'skipped') return false;

    if (searchCompany && !(app.company || '').toLowerCase().includes(searchCompany.toLowerCase())) {
      return false;
    }
    return true;
  });

  return (
    <div className="space-y-4">
      {/* Tracker Header */}
      <div className="flex items-center justify-between pb-3 border-b border-[#e8ece9]">
        <div>
          <h1 className="text-xl font-bold tracking-tight text-[#1d2825]">
            Application Tracker
          </h1>
          <p className="text-xs text-[#78827e] mt-0.5">
            Real-time status of all autonomous applications submitted to employer ATS portals.
          </p>
        </div>

        <input
          type="text"
          placeholder="Filter by company..."
          value={searchCompany}
          onChange={e => setSearchCompany(e.target.value)}
          className="px-3 py-1.5 text-xs bg-white rounded-lg border border-[#e8ece9] outline-none focus:border-[#152c25] w-56"
        />
      </div>

      {/* Status Filter Tabs */}
      <div className="flex items-center space-x-2 text-xs border-b border-[#e8ece9] pb-2">
        {[
          { id: 'all', label: 'All', count: applications.length },
          { id: 'in_flight', label: 'In Flight', count: inFlightCount },
          { id: 'needs_you', label: 'Needs you', count: needsYouCount },
          { id: 'failed', label: 'Failed', count: failedCount },
          { id: 'skipped', label: 'Skipped', count: skippedCount }
        ].map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`px-3 py-1 rounded-full text-xs font-semibold transition flex items-center space-x-1.5 ${activeTab === tab.id ? 'bg-[#152c25] text-white shadow-xs' : 'text-[#55635c] hover:bg-[#e8ece9]'}`}
          >
            <span>{tab.label}</span>
            <span className={`px-1.5 py-0.2 rounded-full text-[10px] ${activeTab === tab.id ? 'bg-white/20 text-white' : 'bg-[#e8ece9] text-[#55635c]'}`}>
              {tab.count}
            </span>
          </button>
        ))}
      </div>

      {/* Applications Table */}
      <div className="bg-white rounded-xl border border-[#e8ece9] overflow-hidden shadow-xs">
        <table className="w-full text-left border-collapse text-xs">
          <thead>
            <tr className="bg-[#fafbfa] border-b border-[#e8ece9] text-[#78827e] font-semibold text-[11px]">
              <th className="py-3 px-4">COMPANY & ROLE</th>
              <th className="py-3 px-4">DATE SUBMITTED</th>
              <th className="py-3 px-4">RESUME</th>
              <th className="py-3 px-4">STATUS</th>
            </tr>
          </thead>
          <tbody id="appTrackerTableBody" className="divide-y divide-[#e8ece9]">
            {filteredApps.length === 0 ? (
              <tr>
                <td colSpan="4">
                  <EmptyState>
                    No applications found in this view. Click 'Apply' on any job card to submit.
                  </EmptyState>
                </td>
              </tr>
            ) : (
              filteredApps.map((app, idx) => {
                const domain = (app.company || '').toLowerCase().replace(/[^a-z0-9]/g, '') + '.com';
                const isSubmitted = app.status === 'submitted';
                const isInFlight = app.status === 'in_flight' || app.status === 'submitting';
                const isFailed = app.status === 'failed';

                return (
                  <tr key={app.id || idx} className="hover:bg-[#f9faf9] transition">
                    <td className="py-3 px-4">
                      <div className="flex items-center space-x-3">
                        <CompanyLogo company={app.company} domain={domain} size="sm" />
                        <div>
                          <div className="font-bold text-[#1d2825]">{app.company}</div>
                          <div className="text-[11px] text-[#78827e]">{app.title}</div>
                        </div>
                      </div>
                    </td>

                    <td className="py-3 px-4 text-[#55635c]">
                      {app.submitted_at ? new Date(app.submitted_at).toLocaleDateString() : 'Today'}
                    </td>

                    <td className="py-3 px-4 text-[#55635c]">
                      <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded bg-[#f4f6f4] text-[11px]">
                        <span>📄</span>
                        <span>{app.resume || 'Tailored'}</span>
                      </span>
                    </td>

                    <td className="py-3 px-4">
                      {isSubmitted ? (
                        <span className="status-submitted">
                          <span className="status-dot"></span>
                          <span>Submitted</span>
                        </span>
                      ) : isInFlight ? (
                        <span className="status-inflight">
                          <span className="status-dot"></span>
                          <span>In Flight</span>
                        </span>
                      ) : isFailed ? (
                        <div className="flex items-center space-x-2">
                           <span className="status-failed">
                            <span className="status-dot"></span>
                            <span>Failed</span>
                          </span>
                          <button 
                            onClick={() => assistApplication(app.id)}
                            className="px-2 py-1 bg-white border border-[#e8ece9] rounded hover:border-[#dc2626] hover:text-[#dc2626] transition"
                          >
                            Assist
                          </button>
                        </div>
                      ) : (
                        <span className="inline-flex items-center space-x-1.5 text-[#6b7280] font-semibold">
                          <span className="w-2 h-2 rounded-full bg-gray-400"></span>
                          <span>{app.status || 'Pending'}</span>
                        </span>
                      )}
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
