import React, { useEffect } from 'react';
import { useApp } from '../../context/AppContext';

export function Sidebar() {
  const { sidebarOpen, closeSidebar, currentTab, setCurrentTab, userProfile } = useApp();

  // Escape key listener to close sidebar cleanly
  useEffect(() => {
    function handleKeyDown(e) {
      if (e.key === 'Escape' && sidebarOpen) {
        closeSidebar();
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [sidebarOpen, closeSidebar]);

  const navItems = [
    { section: 'WORKSPACE' },
    { id: 'dashboard', label: 'Overview', icon: 'M4 4h7v7H4V4zm11 0h7v7h-7V4zM4 15h7v7H4v-7zm11 0h7v7h-7v-7z' },
    { id: 'dashboard', label: 'Discover Jobs', icon: 'M11 4a7 7 0 100 14 7 7 0 000-14zM21 21l-4.35-4.35' },
    { id: 'recommended', label: 'Recommended', icon: 'M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z' },
    { id: 'saved', label: 'Saved Jobs', icon: 'M19 21l-7-5-7 5V5a2 2 0 012-2h10a2 2 0 012 2z' },
    { id: 'auto-apply', label: 'Auto-apply', icon: 'M12 19l9 2-9-18-9 18 9-2zm0 0v-8' },
    { id: 'tracker', label: 'My Applications', icon: 'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z' },
    { section: 'YOUR ACCOUNT' },
    { id: 'profile', label: 'Resume & Profile', icon: 'M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z' },
    { id: 'settings', label: 'Settings', icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z' }
  ];

  return (
    <>
      {/* Backdrop for click-outside dismissal */}
      <div
        id="sidebarBackdrop"
        className={`sidebar-backdrop ${sidebarOpen ? 'active' : ''}`}
        onClick={closeSidebar}
      />

      {/* Slide-out Sidebar Drawer */}
      <aside
        id="appSidebar"
        className={`sidebar ${sidebarOpen ? 'open sidebar-open' : ''}`}
      >
        <div className="space-y-6">
          {/* Header Brand & Close Button */}
          <div className="flex items-center justify-between px-3">
            <div
              className="flex items-center space-x-2.5 cursor-pointer"
              onClick={() => {
                setCurrentTab('dashboard');
                closeSidebar();
              }}
            >
              <div className="w-7 h-7 rounded-[8px] bg-[#d8eadb] flex items-center justify-center flex-shrink-0 shadow-xs">
                <svg width="18" height="18" viewBox="0 0 32 32" fill="none">
                  <path d="M7 23L12 7L16 16L19 10L24 23M7 24H24" stroke="#174a38" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </div>
              <span className="text-[17px] font-bold tracking-tight text-white">OneRole</span>
            </div>

            {/* Prominent Close Button */}
            <button
              onClick={closeSidebar}
              className="p-1.5 text-[#adbbb4] hover:text-white hover:bg-[#254b3e] rounded-lg transition"
              title="Close Sidebar (Esc)"
              aria-label="Close Sidebar"
            >
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="18" y1="6" x2="6" y2="18"></line>
                <line x1="6" y1="6" x2="18" y2="18"></line>
              </svg>
            </button>
          </div>

          {/* Nav List */}
          <nav className="space-y-1">
            {navItems.map((item, idx) => {
              if (item.section) {
                return (
                  <div
                    key={`sec-${idx}`}
                    className="text-[10px] font-bold text-[#82968c] tracking-wider uppercase px-3 pt-3 pb-1"
                  >
                    {item.section}
                  </div>
                );
              }
              const isActive = currentTab === item.id;
              return (
                <div
                  key={`item-${idx}`}
                  onClick={() => {
                    setCurrentTab(item.id);
                    closeSidebar();
                  }}
                  className={`nav-item ${isActive ? 'active' : ''}`}
                >
                  <svg className="nav-icon" viewBox="0 0 24 24">
                    <path d={item.icon}/>
                  </svg>
                  <span>{item.label}</span>
                </div>
              );
            })}
          </nav>
        </div>

        {/* Bottom Cluster Telemetry & User Profile */}
        <div className="space-y-4 pt-4 border-t border-[#233e33]">
          {/* NATS Cluster Status Card */}
          <div className="bg-[#1c382e] p-3 rounded-lg border border-[#2d5646] text-white">
            <div className="flex items-center space-x-2 mb-1.5">
              <span className="w-2 h-2 rounded-full bg-[#10b981] animate-pulse"></span>
              <span className="text-[11px] font-semibold text-[#8ac5a0]">Source coverage</span>
            </div>
            <p className="text-[10px] text-[#a9beb2] leading-tight mb-2">
              Cluster: 4 Chromium Workers · NATS JetStream
            </p>
            <div className="w-full bg-[#152c25] h-1.5 rounded-full overflow-hidden">
              <div className="h-full bg-[#90c7a4] w-full rounded-full"></div>
            </div>
            <div className="flex justify-between items-center text-[9px] text-[#82968c] mt-2">
              <span>Production Cluster</span>
              <span className="text-[#8ac5a0] cursor-pointer hover:underline">Learn more</span>
            </div>
          </div>

          {/* User Profile Footer */}
          <div
            className="flex items-center space-x-3 px-2 py-1.5 rounded-lg hover:bg-[#1c382e] cursor-pointer transition"
            onClick={() => {
              setCurrentTab('profile');
              closeSidebar();
            }}
          >
            <div className="w-8 h-8 rounded-full bg-[#d8eadb] text-[#174a38] flex items-center justify-center text-xs font-bold flex-shrink-0">
              NB
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-xs font-bold text-white truncate">{userProfile.name}</p>
              <p className="text-[10px] text-[#9fb0a7] truncate">{userProfile.email}</p>
            </div>
          </div>
        </div>
      </aside>
    </>
  );
}
