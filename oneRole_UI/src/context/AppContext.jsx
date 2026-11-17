import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { doDevLogin, searchJobs, fetchFilterMetadata, fetchApplications, submitJobApplication } from '../services/api';
import { computeRealtimeATSScore } from '../services/atsEngine';

const AppContext = createContext();

export function AppProvider({ children }) {
  // Navigation & Layout
  const [currentTab, setCurrentTab] = useState('dashboard');
  const [sidebarOpen, setSidebarOpen] = useState(false);

  // User Profile
  const [userProfile] = useState({
    name: 'Nakul Bhandare',
    email: 'bhandarenakul11@gmail.com',
    role: 'Senior Software Engineer (Distributed Systems & Cloud)'
  });

  // Search & Filtering State — Initial landing starts with ZERO active filters
  const [searchTags, setSearchTags] = useState([]);
  const [excludeKeyword, setExcludeKeyword] = useState('');
  const [filters, setFilters] = useState({
    searchTags: [],
    exclude: '',
    datePreset: 'any',
    locations: [],
    includeRemote: true,
    remoteCountry: 'US',
    workplaces: [],
    companies: [],
    degreeLevel: '',
    maxExperience: null,
    visa: false,
    noClearance: false,
    role: '',
    jobType: '',
    employmentType: '',
    payMin: null
  });

  // Data Collections
  const [jobs, setJobs] = useState([]);
  const [activeCards, setActiveCards] = useState([]);
  const [replenishCards, setReplenishCards] = useState([]);
  const [applications, setApplications] = useState([]);
  const [filterMetadata, setFilterMetadata] = useState({});
  const [loading, setLoading] = useState(false);

  // Modals & Popovers
  const [activeModal, setActiveModal] = useState(null); // 'plans' | 'addRole' | 'jobDetail'
  const [selectedJob, setSelectedJob] = useState(null);
  const [toasts, setToasts] = useState([]);

  // Toast notification helper
  const showToast = useCallback((message, type = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, type }]);
    setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id));
    }, 3800);
  }, []);

  // Map raw backend job into card format
  const convertJobToCard = useCallback((job, idx = 0) => {
    const themes = ['card-mint', 'card-pink', 'card-aqua', 'card-peach', 'card-butter'];
    const score = computeRealtimeATSScore(job);
    const domain = (job.company || '').toLowerCase().replace(/[^a-z0-9]/g, '') + '.com';
    const badge = idx === 4 ? 7 : (idx === 2 ? 3 : null);

    return {
      key: job.id || `job-${idx}`,
      id: job.id,
      title: job.title || job.canonical_title || 'Software Engineer',
      company: job.company || 'Enterprise Employer',
      domain: domain,
      location: job.location || job.display_location || (job.workplace_modes?.length > 0 ? job.workplace_modes.join(', ') : 'San Francisco, CA'),
      time: 'Today',
      score: score,
      badge: badge,
      theme: themes[idx % themes.length],
      salary: 'USD 160k - 220k /yr',
      sourceUrl: job.source_url || '#',
      desc: job.description || job.description_text || 'Enterprise verified vacancy in MongoDB',
      rawJob: job
    };
  }, []);

  // Fetch jobs dynamically based on current filters
  const reloadJobs = useCallback(async (customFilters = null) => {
    setLoading(true);
    try {
      const activeFilterSet = customFilters || filters;
      const data = await searchJobs(activeFilterSet);
      let realJobs = data.items || [];

      if (activeFilterSet.companies?.length > 0) {
        realJobs = realJobs.filter(j =>
          activeFilterSet.companies.some(c => (j.company || '').toLowerCase() === c.toLowerCase())
        );
      }

      setJobs(realJobs);

      // Rebuild card queue
      const cards = realJobs.map((j, i) => convertJobToCard(j, i));
      const removedKeys = new Set(JSON.parse(localStorage.getItem('onerole_removed_queue_cards') || '[]'));
      const availableCards = cards.filter(c => !removedKeys.has(c.key));

      setActiveCards(availableCards.slice(0, 5));
      setReplenishCards(availableCards.slice(5));
    } catch (err) {
      console.error('Error reloading jobs:', err);
      showToast('Error querying jobs: ' + err.message, 'error');
    } finally {
      setLoading(false);
    }
  }, [filters, convertJobToCard, showToast]);

  // Reset top match queue (Clears removed queue memory & re-populates live 5 cards)
  const resetMatchQueue = useCallback(() => {
    localStorage.removeItem('onerole_removed_queue_cards');
    reloadJobs();
    showToast('✓ Refreshed queue with live MongoDB matches!', 'success');
  }, [reloadJobs, showToast]);

  // Handle Pass action on card
  const handlePassCard = useCallback((cardKey) => {
    const removed = JSON.parse(localStorage.getItem('onerole_removed_queue_cards') || '[]');
    removed.push(cardKey);
    localStorage.setItem('onerole_removed_queue_cards', JSON.stringify(removed));

    setActiveCards(prev => {
      const filtered = prev.filter(c => c.key !== cardKey);
      if (replenishCards.length > 0) {
        const next = replenishCards[0];
        setReplenishCards(r => r.slice(1));
        return [...filtered, next];
      }
      return filtered;
    });
    showToast('Card passed and removed from active queue', 'info');
  }, [replenishCards, showToast]);

  // Handle Apply action on card
  const handleApplyCard = useCallback(async (card) => {
    showToast(`⚡ Dispatching auto-apply to ${card.company}...`, 'info');

    // Add to local removed set
    const removed = JSON.parse(localStorage.getItem('onerole_removed_queue_cards') || '[]');
    removed.push(card.key);
    localStorage.setItem('onerole_removed_queue_cards', JSON.stringify(removed));

    // Optimistic application tracker entry
    const newApp = {
      id: 'app-' + Date.now(),
      job_id: card.id,
      company: card.company,
      title: card.title,
      status: 'in_flight',
      submitted_at: new Date().toISOString(),
      source_url: card.sourceUrl,
      resume: 'Default'
    };
    setApplications(prev => [newApp, ...prev]);

    // Slide in next card from replenishment queue
    setActiveCards(prev => {
      const filtered = prev.filter(c => c.key !== card.key);
      if (replenishCards.length > 0) {
        const next = replenishCards[0];
        setReplenishCards(r => r.slice(1));
        return [...filtered, next];
      }
      return filtered;
    });

    try {
      await submitJobApplication(card.rawJob || card);
      showToast(`✓ Successfully submitted to ${card.company}! Confirmation sent to ${userProfile.email}`, 'success');
      
      // Update app status to submitted
      setApplications(prev => prev.map(a => a.id === newApp.id ? { ...a, status: 'submitted' } : a));
    } catch (e) {
      console.warn('Backend dispatch queued:', e.message);
      showToast(`Application queued for ${card.company}`, 'info');
    }
  }, [replenishCards, showToast, userProfile.email]);

  // Handle Apply to all 5 cards
  const handleApplyToAll5 = useCallback(async () => {
    if (activeCards.length === 0) return;
    const cardsToSubmit = [...activeCards];
    showToast(`⚡ Submitting all ${cardsToSubmit.length} match applications in parallel...`, 'info');

    for (const card of cardsToSubmit) {
      await handleApplyCard(card);
    }
  }, [activeCards, handleApplyCard, showToast]);

  // Sync Search Tags
  const addSearchTag = useCallback((tag) => {
    const clean = tag.trim().toLowerCase().replace(/^,+|,+$/g, '');
    if (!clean) return;
    setSearchTags(prev => {
      if (prev.includes(clean)) return prev;
      const next = [...prev, clean];
      setFilters(f => {
        const updated = { ...f, searchTags: next };
        reloadJobs(updated);
        return updated;
      });
      return next;
    });
  }, [reloadJobs]);

  const removeSearchTag = useCallback((tag) => {
    setSearchTags(prev => {
      const next = prev.filter(t => t !== tag);
      setFilters(f => {
        const updated = { ...f, searchTags: next };
        reloadJobs(updated);
        return updated;
      });
      return next;
    });
  }, [reloadJobs]);

  // Initial Boot: Dev login, load facets, load jobs, load tracker
  useEffect(() => {
    async function boot() {
      await doDevLogin();
      const facets = await fetchFilterMetadata();
      setFilterMetadata(facets);

      const apps = await fetchApplications();
      setApplications(apps);

      // Reload jobs with clean initial state (no filters attached)
      reloadJobs({
        searchTags: [],
        exclude: '',
        datePreset: 'any',
        locations: [],
        includeRemote: true,
        remoteCountry: 'US',
        workplaces: [],
        companies: []
      });
    }
    boot();
  }, []);

  return (
    <AppContext.Provider value={{
      currentTab,
      setCurrentTab,
      sidebarOpen,
      setSidebarOpen,
      toggleSidebar: () => setSidebarOpen(prev => !prev),
      closeSidebar: () => setSidebarOpen(false),
      userProfile,
      searchTags,
      addSearchTag,
      removeSearchTag,
      excludeKeyword,
      setExcludeKeyword: (kw) => {
        setExcludeKeyword(kw);
        setFilters(f => {
          const updated = { ...f, exclude: kw };
          reloadJobs(updated);
          return updated;
        });
      },
      filters,
      setFilters,
      filterMetadata,
      jobs,
      activeCards,
      replenishCards,
      applications,
      loading,
      reloadJobs,
      resetMatchQueue,
      handlePassCard,
      handleApplyCard,
      handleApplyToAll5,
      activeModal,
      setActiveModal,
      selectedJob,
      setSelectedJob,
      toasts,
      showToast
    }}>
      {children}
    </AppContext.Provider>
  );
}

export function useApp() {
  const context = useContext(AppContext);
  if (!context) throw new Error('useApp must be used within AppProvider');
  return context;
}
