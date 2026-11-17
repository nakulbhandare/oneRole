import React, { useState } from 'react';
import { useApp } from '../../context/AppContext';

export function FilterBar() {
  const {
    searchTags,
    addSearchTag,
    removeSearchTag,
    excludeKeyword,
    setExcludeKeyword,
    filters,
    setFilters,
    filterMetadata,
    reloadJobs,
    showToast
  } = useApp();

  const [tagInput, setTagInput] = useState('');
  const [activeDropdown, setActiveDropdown] = useState(null); // 'location' | 'date' | 'workplace' | 'experience' | 'role'
  const [locSearch, setLocSearch] = useState('');

  // Handle Tag Input
  function handleTagKeyDown(e) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      if (tagInput.trim()) {
        addSearchTag(tagInput.trim());
        setTagInput('');
      }
    }
  }

  // Location Toggle
  function handleToggleLocation(locVal) {
    const currentLocs = filters.locations || [];
    const exists = currentLocs.includes(locVal);
    const updated = exists ? currentLocs.filter(l => l !== locVal) : [...currentLocs, locVal];

    const nextFilters = { ...filters, locations: updated };
    setFilters(nextFilters);
    reloadJobs(nextFilters);
    showToast(`Location filter: ${updated.join(', ') || 'All Locations'}`, 'info');
  }

  // Workplace Mode Toggle
  function handleToggleWorkplace(mode) {
    const current = filters.workplaces || [];
    const exists = current.includes(mode);
    const updated = exists ? current.filter(m => m !== mode) : [...current, mode];

    const nextFilters = { ...filters, workplaces: updated };
    setFilters(nextFilters);
    reloadJobs(nextFilters);
  }

  // Date Preset Setter
  function handleSetDate(preset, label) {
    const nextFilters = { ...filters, datePreset: preset };
    setFilters(nextFilters);
    reloadJobs(nextFilters);
    setActiveDropdown(null);
    showToast(`Date filter: ${label}`, 'info');
  }

  // Experience Filter Setter
  function handleSetExperience(years, label) {
    const nextFilters = { ...filters, maxExperience: years };
    setFilters(nextFilters);
    reloadJobs(nextFilters);
    setActiveDropdown(null);
    showToast(`Experience filter: ${label}`, 'info');
  }

  // Visa Sponsorship Toggle
  function handleToggleVisa() {
    const nextFilters = { ...filters, visa: !filters.visa };
    setFilters(nextFilters);
    reloadJobs(nextFilters);
    showToast(`Visa sponsorship: ${nextFilters.visa ? 'Required' : 'Any'}`, 'info');
  }

  // Clear all filters
  function handleClearAll() {
    const cleanFilters = {
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
    };
    setFilters(cleanFilters);
    setExcludeKeyword('');
    reloadJobs(cleanFilters);
    showToast('All filters cleared', 'info');
  }

  // Build locations options list (Popular base + MongoDB facets)
  const popularBase = [
    { value: 'United States', label: 'United States', badge: 'Country' },
    { value: 'United Kingdom', label: 'United Kingdom', badge: 'Country' },
    { value: 'Canada', label: 'Canada', badge: 'Country' },
    { value: 'India', label: 'India', badge: 'Country' },
    { value: 'Germany', label: 'Germany', badge: 'Country' },
    { value: 'Philippines', label: 'Philippines', badge: 'Country' },
    { value: 'France', label: 'France', badge: 'Country' }
  ];

  const metaOptions = (filterMetadata?.location?.options || []).map(opt => ({
    value: opt.value || opt.label,
    label: opt.label || opt.value,
    badge: opt.count ? `${opt.count} jobs` : 'Location'
  }));

  const allLocations = [...popularBase];
  metaOptions.forEach(opt => {
    if (!allLocations.some(l => l.value.toLowerCase() === opt.value.toLowerCase())) {
      allLocations.push(opt);
    }
  });

  const filteredLocations = allLocations.filter(opt =>
    opt.label.toLowerCase().includes(locSearch.toLowerCase())
  );

  const hasActiveFilters = searchTags.length > 0 ||
    filters.locations?.length > 0 ||
    filters.workplaces?.length > 0 ||
    filters.datePreset !== 'any' ||
    filters.maxExperience !== null ||
    filters.visa ||
    excludeKeyword;

  return (
    <div className="search-filter-box space-y-2 relative">
      {/* Top Row: Tag Chips & Input */}
      <div className="search-input-row" id="tagInputContainer">
        <svg className="text-[#9ca6a1] flex-shrink-0" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="11" cy="11" r="7"/><path d="m21 21-4.35-4.35"/>
        </svg>

        {/* Dynamic Tag Chips */}
        <div id="activeTagChips" className="flex items-center flex-wrap gap-1.5">
          {searchTags.map(tag => (
            <span key={tag} className="tag-chip" data-tag={tag}>
              <span>{tag}</span>
              <span
                className="tag-chip-remove cursor-pointer"
                onClick={() => removeSearchTag(tag)}
              >
                ×
              </span>
            </span>
          ))}
        </div>

        <input
          type="text"
          id="tagSearchInputField"
          value={tagInput}
          onChange={e => setTagInput(e.target.value)}
          onKeyDown={handleTagKeyDown}
          placeholder={searchTags.length === 0 ? "Search by role, skill, or title..." : "Add another..."}
          className="tag-input-field flex-1"
        />
      </div>

      {/* Second Row: Exclude Input */}
      <div className="exclude-row">
        <span className="exclude-label">Exclude</span>
        <input
          type="text"
          id="excludeJobsInput"
          value={excludeKeyword}
          onChange={e => setExcludeKeyword(e.target.value)}
          placeholder="Hide jobs mentioning..."
          className="exclude-input flex-1"
        />
      </div>

      {/* Third Row: Filter Dropdowns & Toggles */}
      <div className="flex items-center flex-wrap gap-2 pt-1 border-t border-[#eef2ef] text-xs">
        {/* Date Filter */}
        <div className="relative">
          <button
            onClick={() => setActiveDropdown(activeDropdown === 'date' ? null : 'date')}
            className={`filter-btn-pill ${filters.datePreset !== 'any' ? 'active' : ''}`}
          >
            <span>📅 {filters.datePreset === 'any' ? 'Date' : filters.datePreset.replace('_', ' ')}</span>
            <span className="text-[10px]">▼</span>
          </button>

          {activeDropdown === 'date' && (
            <div className="filter-dropdown-menu absolute left-0 top-full mt-1.5 w-44 bg-white rounded-lg shadow-xl border border-[#e8ece9] p-1.5 z-40">
              {[
                { preset: 'any', label: 'Any time' },
                { preset: 'last_6_hours', label: 'Past 6 hours' },
                { preset: 'last_24_hours', label: 'Past 24 hours' },
                { preset: 'last_7_days', label: 'Past week' },
                { preset: 'last_30_days', label: 'Past month' }
              ].map(opt => (
                <div
                  key={opt.preset}
                  onClick={() => handleSetDate(opt.preset, opt.label)}
                  className={`px-3 py-1.5 rounded text-xs cursor-pointer hover:bg-[#eef5f1] transition ${filters.datePreset === opt.preset ? 'font-bold text-[#152c25] bg-[#eef5f1]' : 'text-[#374151]'}`}
                >
                  {opt.label}
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Location Filter Popover */}
        <div className="relative">
          <button
            onClick={() => setActiveDropdown(activeDropdown === 'location' ? null : 'location')}
            className={`filter-btn-pill ${filters.locations?.length > 0 ? 'active' : ''}`}
          >
            <span>📍 {filters.locations?.length > 0 ? `Location (${filters.locations.length})` : 'Location'}</span>
            <span className="text-[10px]">▼</span>
          </button>

          {activeDropdown === 'location' && (
            <div className="location-popover-menu absolute left-0 top-full mt-1.5 w-72 bg-white rounded-xl shadow-2xl border border-[#e8ece9] p-3 z-40">
              <input
                type="text"
                placeholder="Search locations..."
                value={locSearch}
                onChange={e => setLocSearch(e.target.value)}
                className="w-full px-2.5 py-1.5 text-xs bg-[#f4f6f4] rounded-md outline-none mb-2 border border-transparent focus:border-[#24684f]"
              />

              <div className="max-h-56 overflow-y-auto space-y-1">
                <div className="text-[10px] font-bold text-[#828d86] tracking-wider uppercase px-1 pb-1">POPULAR LOCATIONS</div>
                {filteredLocations.slice(0, 30).map(opt => {
                  const isChecked = filters.locations?.includes(opt.value);
                  return (
                    <label
                      key={opt.value}
                      className="location-option-row flex items-center justify-between px-2 py-1.5 rounded hover:bg-[#f3f7f4] cursor-pointer"
                    >
                      <div className="flex items-center space-x-2 truncate max-w-[190px]">
                        <input
                          type="checkbox"
                          checked={isChecked}
                          onChange={() => handleToggleLocation(opt.value)}
                          className="rounded text-[#24684f] focus:ring-0"
                        />
                        <span className="text-xs text-[#1d2825] truncate">{opt.label}</span>
                      </div>
                      <span className="text-[10px] text-[#9ca3af] font-medium flex-shrink-0">{opt.badge}</span>
                    </label>
                  );
                })}
              </div>
            </div>
          )}
        </div>

        {/* Workplace Modes Pills */}
        <div className="flex items-center space-x-1 border-l border-[#e8ece9] pl-2">
          {['remote', 'hybrid', 'on_site'].map(mode => {
            const isSelected = filters.workplaces?.includes(mode);
            const labels = { remote: 'Remote', hybrid: 'Hybrid', on_site: 'On-site' };
            return (
              <button
                key={mode}
                onClick={() => handleToggleWorkplace(mode)}
                className={`px-2.5 py-1 rounded-full text-xs font-medium border transition ${isSelected ? 'bg-[#152c25] text-white border-[#152c25]' : 'bg-[#fafbfa] text-[#4b5563] border-[#e5e7eb] hover:bg-[#f3f4f6]'}`}
              >
                {labels[mode]}
              </button>
            );
          })}
        </div>

        {/* Experience Dropdown */}
        <div className="relative">
          <button
            onClick={() => setActiveDropdown(activeDropdown === 'experience' ? null : 'experience')}
            className={`filter-btn-pill ${filters.maxExperience !== null ? 'active' : ''}`}
          >
            <span>🎓 {filters.maxExperience !== null ? `≤ ${filters.maxExperience} yrs exp` : 'Experience'}</span>
            <span className="text-[10px]">▼</span>
          </button>

          {activeDropdown === 'experience' && (
            <div className="filter-dropdown-menu absolute left-0 top-full mt-1.5 w-40 bg-white rounded-lg shadow-xl border border-[#e8ece9] p-1.5 z-40">
              {[
                { exp: null, label: 'Any experience' },
                { exp: 2, label: 'Entry (≤ 2 yrs)' },
                { exp: 5, label: 'Mid (≤ 5 yrs)' },
                { exp: 8, label: 'Senior (≤ 8 yrs)' }
              ].map(opt => (
                <div
                  key={opt.label}
                  onClick={() => handleSetExperience(opt.exp, opt.label)}
                  className="px-3 py-1.5 rounded text-xs cursor-pointer hover:bg-[#eef5f1] transition text-[#374151]"
                >
                  {opt.label}
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Visa Sponsorship Toggle */}
        <button
          onClick={handleToggleVisa}
          className={`filter-btn-pill ${filters.visa ? 'active' : ''}`}
        >
          <span>🛂 Visa sponsorship</span>
        </button>

        {/* Clear All CTA */}
        {hasActiveFilters && (
          <button
            onClick={handleClearAll}
            className="text-xs text-[#b91c1c] hover:underline font-semibold ml-auto pl-2"
          >
            Clear all
          </button>
        )}
      </div>
    </div>
  );
}
