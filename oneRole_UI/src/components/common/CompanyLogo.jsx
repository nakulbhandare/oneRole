import React, { useState } from 'react';

export function CompanyLogo({ company, domain, size = 'md' }) {
  const [imgError, setImgError] = useState(false);
  const cleanCompany = (company || 'Company').trim();
  const initials = cleanCompany.slice(0, 2).toUpperCase();

  const sizeClasses = {
    sm: 'w-6 h-6 text-[10px]',
    md: 'w-8 h-8 text-xs',
    lg: 'w-10 h-10 text-sm'
  }[size] || 'w-8 h-8 text-xs';

  const cleanDomain = domain || `${cleanCompany.toLowerCase().replace(/[^a-z0-9]/g, '')}.com`;
  const logoUrl = `https://www.google.com/s2/favicons?domain=${cleanDomain}&sz=64`;

  if (imgError) {
    return (
      <div className={`${sizeClasses} rounded-lg bg-[#e8ece9] text-[#24684f] font-bold flex items-center justify-center flex-shrink-0 shadow-xs border border-[#d6dcd8]`}>
        {initials}
      </div>
    );
  }

  return (
    <div className={`${sizeClasses} rounded-lg bg-white flex items-center justify-center flex-shrink-0 shadow-xs border border-[#e8ece9] p-1 overflow-hidden`}>
      <img
        src={logoUrl}
        alt={cleanCompany}
        className="w-full h-full object-contain"
        onError={() => setImgError(true)}
      />
    </div>
  );
}
