import React from 'react';
import { useApp } from '../../context/AppContext';

export function Navbar() {
  const { currentTab, setCurrentTab, toggleSidebar, setActiveModal, isAuthenticated } = useApp();

  const navTabs = [
    { id: 'dashboard', label: 'Dashboard' },
    { id: 'browse', label: 'Browse jobs' },
    { id: 'auto-apply', label: 'Auto Apply' },
    { id: 'tracker', label: 'Tracker' },
    { id: 'profile', label: 'Profile' }
  ];

  return (
    <header className="top-header">
      {/* Left: Brand Logo, Sidebar Toggle Button & Navigation Tabs */}
      <div className="flex items-center space-x-3">
        {/* OneRole Brand Logo */}
        <div
          className="flex items-center space-x-2.5 cursor-pointer mr-2 select-none"
          onClick={() => setCurrentTab('dashboard')}
        >
          <div className="w-7 h-7 rounded-[8px] bg-[#d8eadb] flex items-center justify-center flex-shrink-0 shadow-xs">
            <svg width="18" height="18" viewBox="0 0 32 32" fill="none">
              <path d="M7 23L12 7L16 16L19 10L24 23M7 24H24" stroke="#174a38" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"/>
            </svg>
          </div>
          <span className="text-[17px] font-bold tracking-tight text-[#152c25]">OneRole</span>
        </div>

        {/* Sidebar Toggle Icon */}
        <button
          onClick={(e) => {
            e.stopPropagation();
            toggleSidebar();
          }}
          className="p-1.5 text-[#6b7280] hover:text-[#18181b] rounded transition"
          title="Toggle Sidebar Navigation"
          aria-label="Toggle Sidebar Navigation"
        >
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <rect x="3" y="3" width="18" height="18" rx="2" ry="2"/>
            <line x1="9" y1="3" x2="9" y2="21"/>
          </svg>
        </button>

        {/* Horizontal Nav Tabs */}
        <nav className="flex items-center space-x-1 ml-2">
          {navTabs.map(tab => (
            <button
              key={tab.id}
              onClick={() => {
                setCurrentTab(tab.id);
                if (tab.id === 'browse') {
                  const el = document.getElementById('catalogHeading');
                  if (el) el.scrollIntoView({ behavior: 'smooth' });
                }
              }}
              className={`nav-tab-pill ${currentTab === tab.id ? 'active' : ''} focus:outline-none focus:ring-2 focus:ring-[#174a38] focus:ring-opacity-50`}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Right: Settings & Credits & Profile */}
      <div className="flex items-center space-x-3">
        {isAuthenticated ? (
          <>
            <button
              onClick={() => setCurrentTab('settings')}
              className="text-xs text-[#526359] hover:text-[#18181b] font-medium transition focus:outline-none focus:underline"
            >
              Settings
            </button>

            <button
              onClick={() => setCurrentTab('profile')}
              className="w-7 h-7 rounded-full bg-[#e3d0c1] text-[#6b472e] flex items-center justify-center text-xs font-bold shadow-xs hover:ring-2 hover:ring-[#174a38] transition focus:outline-none focus:ring-2 focus:ring-[#174a38] focus:ring-offset-2"
              title="Nakul Bhandare (Profile)"
            >
              NB
            </button>
          </>
        ) : (
          <button
            onClick={() => setCurrentTab('login')}
            className="px-3.5 py-1.5 bg-[#152c25] hover:bg-[#254b3e] text-white text-xs font-bold rounded-[7px] transition focus:outline-none focus:ring-2 focus:ring-[#174a38] focus:ring-offset-2"
          >
            Login
          </button>
        )}
      </div>
    </header>
  );
}
