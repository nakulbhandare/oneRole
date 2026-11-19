import React, { useState } from 'react';
import { useApp } from '../../context/AppContext';
import { FormError } from '../common/FormError';

export function AddRoleModal() {
  const { activeModal, setActiveModal, showToast, handleApplyCard } = useApp();
  const [url, setUrl] = useState('');
  const [company, setCompany] = useState('');
  const [title, setTitle] = useState('');
  const [error, setError] = useState('');

  if (activeModal !== 'addRole') return null;

  function handleSubmit(e) {
    e.preventDefault();
    if (!url.trim() && !title.trim()) {
      setError('Please provide a job URL or role title');
      return;
    }
    setError('');

    const customJob = {
      id: 'custom-' + Date.now(),
      company: company || 'Custom Enterprise Job',
      title: title || 'Software Engineer',
      source_url: url,
      location: 'Remote',
      description: `Custom job submitted via portal: ${url}`
    };

    handleApplyCard({
      key: customJob.id,
      id: customJob.id,
      company: customJob.company,
      title: customJob.title,
      rawJob: customJob
    });

    setActiveModal(null);
    showToast(`✓ Queued custom job application for ${customJob.company}`, 'success');
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40 backdrop-blur-xs">
      <div className="bg-white rounded-2xl max-w-md w-full p-6 shadow-2xl border border-[#e8ece9] relative">
        <button
          onClick={() => setActiveModal(null)}
          className="absolute right-4 top-4 p-1.5 text-gray-400 hover:text-gray-700 hover:bg-gray-100 rounded-lg"
        >
          ✕
        </button>

        <h2 className="text-lg font-bold text-[var(--ink-primary)] mb-1">Add Job Vacancy</h2>
        <p className="text-xs text-[var(--ink-muted)] mb-4">
          Paste any job posting URL to queue an automated ATS application submission.
        </p>

        <form onSubmit={handleSubmit} className="space-y-3.5 text-xs">
          <FormError message={error} />
          <div>
            <label className="font-bold text-[var(--ink-primary)] block mb-1">Job Posting URL</label>
            <input
              type="url"
              placeholder="https://boards.greenhouse.io/... or https://jobs.ashbyhq.com/..."
              value={url}
              onChange={e => {
                setUrl(e.target.value);
                if (error) setError('');
              }}
              className="w-full px-3 py-2 border border-[var(--card-border)] rounded-lg outline-none focus:border-[#152c25]"
            />
          </div>

          <div>
            <label className="font-bold text-[var(--ink-primary)] block mb-1">Company Name (Optional)</label>
            <input
              type="text"
              placeholder="e.g. Anthropic, Stripe, OpenAI"
              value={company}
              onChange={e => {
                setCompany(e.target.value);
                if (error) setError('');
              }}
              className="w-full px-3 py-2 border border-[var(--card-border)] rounded-lg outline-none focus:border-[#152c25]"
            />
          </div>

          <div>
            <label className="font-bold text-[var(--ink-primary)] block mb-1">Job Title (Optional)</label>
            <input
              type="text"
              placeholder="e.g. Senior Backend Engineer (Go)"
              value={title}
              onChange={e => {
                setTitle(e.target.value);
                if (error) setError('');
              }}
              className="w-full px-3 py-2 border border-[var(--card-border)] rounded-lg outline-none focus:border-[#152c25]"
            />
          </div>

          <div className="pt-2 flex justify-end space-x-2">
            <button
              type="button"
              onClick={() => setActiveModal(null)}
              className="px-3.5 py-1.5 border border-[#d1d5db] text-[#374151] rounded-lg font-semibold hover:bg-gray-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="px-4 py-1.5 bg-[var(--sidebar-bg)] hover:bg-[var(--sidebar-active)] text-white rounded-lg font-bold shadow-sm transition"
            >
              Queue Auto-Apply
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
