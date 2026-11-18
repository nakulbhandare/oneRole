/* Dynamic Skeleton Loader Component */
import React from 'react';

export function SkeletonCard() {
  return (
    <div className="card p-4 rounded-[10px] border border-[#e8ece9] bg-[#fafbfa] relative overflow-hidden">
      <div className="absolute inset-0 bg-gradient-to-r from-[#fafbfa] via-[#f0f2f0] to-[#fafbfa] animate-shimmer" style={{ backgroundSize: '200% 100%' }}></div>
      <div className="flex items-center space-x-3 mb-4 z-10 relative">
        <div className="w-10 h-10 bg-gray-200 rounded-full"></div>
        <div className="space-y-2">
          <div className="h-3 w-24 bg-gray-200 rounded"></div>
          <div className="h-2 w-16 bg-gray-200 rounded"></div>
        </div>
      </div>
      <div className="space-y-3 z-10 relative">
        <div className="h-4 w-full bg-gray-200 rounded"></div>
        <div className="h-3 w-4/5 bg-gray-200 rounded"></div>
      </div>
    </div>
  );
}
