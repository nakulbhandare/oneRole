import React, { useState } from 'react';
import { useApp } from '../../context/AppContext';
import { CompanyLogo } from '../common/CompanyLogo';

export default function SignupPage() {
  const { showToast } = useApp();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  const handleSignup = (e) => {
    e.preventDefault();
    showToast('Creating account...', 'info');
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-[#f7f8f6] px-4">
      <div className="w-full max-w-sm bg-white p-8 rounded-2xl border border-[#e8ece9] shadow-sm space-y-6">
        <div className="flex justify-center">
          <CompanyLogo />
        </div>
        
        <div className="space-y-1 text-center">
          <h1 className="text-xl font-bold text-[#1d2825]">Join OneRole</h1>
          <p className="text-xs text-[#78827e]">Create your enterprise account</p>
        </div>

        <form onSubmit={handleSignup} className="space-y-4">
          <input
            type="email"
            placeholder="Work email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="w-full px-4 py-2.5 bg-[#f4f7f5] border border-[#e2e8e4] rounded-lg text-sm"
          />
          <input
            type="password"
            placeholder="Password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full px-4 py-2.5 bg-[#f4f7f5] border border-[#e2e8e4] rounded-lg text-sm"
          />
          <button 
            type="submit"
            className="w-full py-2.5 bg-[#152c25] hover:bg-[#254b3e] text-white text-sm font-bold rounded-lg transition"
          >
            Create account
          </button>
        </form>

        <div className="relative">
          <div className="absolute inset-0 flex items-center">
            <div className="w-full border-t border-[#e8ece9]"></div>
          </div>
          <div className="relative flex justify-center text-xs">
            <span className="bg-white px-2 text-[#78827e]">Or</span>
          </div>
        </div>

        <button className="w-full py-2.5 bg-white border border-[#e2e8e4] hover:bg-[#f4f7f5] text-[#1d2825] text-sm font-bold rounded-lg transition flex items-center justify-center space-x-2">
          <span>Continue with Google</span>
        </button>
      </div>
    </div>
  );
}

