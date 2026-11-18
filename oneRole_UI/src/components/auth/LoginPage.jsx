import React from 'react';
import { useApp } from '../../context/AppContext';
import { CompanyLogo } from '../common/CompanyLogo';

export default function LoginPage() {
  const { showToast } = useApp();

  const handleOAuthLogin = async (provider) => {
    showToast(`Redirecting to ${provider}...`, 'info');
    window.location.href = `/api/v1/auth/start?provider=${provider}`;
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-[#f7f8f6] px-4">
      <div className="w-full max-w-sm bg-white p-8 rounded-2xl border border-[#e8ece9] shadow-sm space-y-6">
        <div className="flex justify-center">
          <CompanyLogo />
        </div>
        
        <div className="space-y-1 text-center">
          <h1 className="text-xl font-bold text-[#1d2825]">Welcome back</h1>
          <p className="text-xs text-[#78827e]">Sign in to your OneRole enterprise account</p>
        </div>

        <button 
          onClick={() => handleOAuthLogin('google')}
          className="w-full py-2.5 bg-white border border-[#e2e8e4] hover:bg-[#f4f7f5] text-[#1d2825] text-sm font-bold rounded-lg transition flex items-center justify-center space-x-2"
        >
          <span>Continue with Google</span>
        </button>
        <button 
          onClick={() => handleOAuthLogin('github')}
          className="w-full py-2.5 bg-white border border-[#e2e8e4] hover:bg-[#f4f7f5] text-[#1d2825] text-sm font-bold rounded-lg transition flex items-center justify-center space-x-2"
        >
          <span>Continue with GitHub</span>
        </button>
      </div>
    </div>
  );
}
