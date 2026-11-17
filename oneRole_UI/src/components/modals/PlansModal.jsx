import React from 'react';
import { useApp } from '../../context/AppContext';

export function PlansModal() {
  const { activeModal, setActiveModal, showToast } = useApp();

  if (activeModal !== 'plans') return null;

  const tiers = [
    {
      name: 'Starter',
      price: '$0',
      period: 'forever',
      features: ['5 Auto-applies / day', 'Basic ATS scoring', 'Application Tracker'],
      cta: 'Current Plan',
      current: true
    },
    {
      name: 'Pro Pilot',
      price: '$29',
      period: 'per month',
      features: ['50 Auto-applies / day', 'Real-time resume tailoring', 'Chromium daemon bypass', 'Direct referral outreach'],
      cta: 'Upgrade to Pro',
      highlighted: true
    },
    {
      name: 'Executive',
      price: '$79',
      period: 'per month',
      features: ['Unlimited Auto-applies', 'Dedicated proxy network', '1-on-1 interview coach agent', 'Direct recruiter email sync'],
      cta: 'Upgrade to Executive'
    }
  ];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40 backdrop-blur-xs">
      <div className="bg-white rounded-2xl max-w-3xl w-full p-6 shadow-2xl border border-[#e8ece9] relative">
        <button
          onClick={() => setActiveModal(null)}
          className="absolute right-4 top-4 p-1.5 text-gray-400 hover:text-gray-700 hover:bg-gray-100 rounded-lg"
        >
          ✕
        </button>

        <div className="text-center max-w-md mx-auto mb-6">
          <h2 className="text-xl font-bold text-[#1d2825]">Subscription Plans</h2>
          <p className="text-xs text-[#78827e] mt-1">
            Accelerate your career with automated enterprise ATS submissions and dynamic resume customization.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {tiers.map(tier => (
            <div
              key={tier.name}
              className={`rounded-xl p-5 border flex flex-col justify-between ${tier.highlighted ? 'border-[#152c25] bg-[#f7faf8] shadow-md ring-1 ring-[#152c25]' : 'border-[#e8ece9] bg-white'}`}
            >
              <div>
                <h3 className="font-bold text-sm text-[#1d2825]">{tier.name}</h3>
                <div className="mt-2 mb-4">
                  <span className="text-2xl font-black text-[#1d2825]">{tier.price}</span>
                  <span className="text-xs text-[#78827e] ml-1">/ {tier.period}</span>
                </div>

                <ul className="space-y-2 text-xs text-[#4b5563]">
                  {tier.features.map(f => (
                    <li key={f} className="flex items-center space-x-1.5">
                      <span className="text-[#152c25] font-bold">✓</span>
                      <span>{f}</span>
                    </li>
                  ))}
                </ul>
              </div>

              <button
                onClick={() => {
                  showToast(`Selected ${tier.name} plan`, 'success');
                  setActiveModal(null);
                }}
                className={`w-full py-2 rounded-lg text-xs font-bold mt-6 transition ${tier.highlighted ? 'bg-[#152c25] hover:bg-[#254b3e] text-white shadow-sm' : 'bg-gray-100 hover:bg-gray-200 text-[#374151]'}`}
              >
                {tier.cta}
              </button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
