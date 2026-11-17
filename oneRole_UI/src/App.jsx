import React from 'react';
import { useApp } from './context/AppContext';
import { Navbar } from './components/layout/Navbar';
import { Sidebar } from './components/layout/Sidebar';
import { HeroBanner } from './components/dashboard/HeroBanner';
import { MatchCardsQueue } from './components/dashboard/MatchCardsQueue';
import { FilterBar } from './components/dashboard/FilterBar';
import { JobCatalogGrid } from './components/dashboard/JobCatalogGrid';
import { ApplicationTracker } from './components/tracker/ApplicationTracker';
import { AutoApplyView } from './components/autoApply/AutoApplyView';
import { ProfileView } from './components/profile/ProfileView';
import { JobDetailModal } from './components/modals/JobDetailModal';
import { PlansModal } from './components/modals/PlansModal';
import { AddRoleModal } from './components/modals/AddRoleModal';
import { ToastContainer } from './components/common/Toast';

export function App() {
  const { currentTab } = useApp();

  return (
    <div className="min-h-screen flex flex-col bg-[#f7f8f6]">
      {/* Top Fixed Navbar */}
      <Navbar />

      {/* Slide-out Collapsible Sidebar Drawer */}
      <Sidebar />

      {/* Main Content Area */}
      <main className="main-content-canvas max-w-7xl mx-auto w-full px-4 sm:px-6 py-5 flex-1">
        {currentTab === 'dashboard' || currentTab === 'browse' ? (
          <div className="space-y-5">
            <HeroBanner />
            <MatchCardsQueue />
            <FilterBar />
            <JobCatalogGrid />
          </div>
        ) : currentTab === 'tracker' ? (
          <ApplicationTracker />
        ) : currentTab === 'auto-apply' ? (
          <AutoApplyView />
        ) : currentTab === 'profile' ? (
          <ProfileView />
        ) : (
          <div className="bg-white p-8 rounded-xl border border-[#e8ece9] text-center space-y-2">
            <h2 className="text-base font-bold text-[#1d2825] capitalize">{currentTab}</h2>
            <p className="text-xs text-[#78827e]">
              Connected to OneRole distributed cluster and NATS JetStream event bus.
            </p>
          </div>
        )}
      </main>

      {/* Global Modals & Toasts */}
      <JobDetailModal />
      <PlansModal />
      <AddRoleModal />
      <ToastContainer />
    </div>
  );
}
export default App;
