import React from 'react';
import { useApp } from '../../context/AppContext';

export function ProfileView() {
  const { userProfile, showToast } = useApp();

  const skills = [
    'Golang (Go)', 'Distributed Systems', 'Kubernetes (K8s)', 'Kafka', 'gRPC',
    'Docker', 'AWS (EC2, S3, Lambda)', 'GCP', 'PostgreSQL', 'MongoDB',
    'Microservices', 'High-Throughput Concurrency', 'REST APIs', 'CI/CD Pipelines'
  ];

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="pb-3 border-b border-[#e8ece9]">
        <h1 className="text-xl font-bold tracking-tight text-[#1d2825]">
          Candidate Profile & Credentials
        </h1>
        <p className="text-xs text-[#78827e] mt-0.5">
          Your active candidate profile and master resume used for automated ATS matching.
        </p>
      </div>

      {/* Main Profile Info Card */}
      <div className="bg-white p-6 rounded-xl border border-[#e8ece9] shadow-xs flex items-start justify-between">
        <div className="flex items-center space-x-4">
          <div className="w-16 h-16 rounded-full bg-[#d8eadb] text-[#174a38] text-xl font-bold flex items-center justify-center shadow-xs">
            NB
          </div>
          <div>
            <h2 className="text-base font-bold text-[#1d2825]">{userProfile.name}</h2>
            <p className="text-xs text-[#152c25] font-semibold">{userProfile.role}</p>
            <p className="text-xs text-[#78827e] mt-0.5">{userProfile.email} · San Francisco, CA (Open to Remote)</p>
          </div>
        </div>

        <a
          href="/Nakul_Bhandare_Resume.pdf"
          target="_blank"
          rel="noopener noreferrer"
          className="px-3.5 py-1.5 bg-[#152c25] hover:bg-[#254b3e] text-white text-xs font-bold rounded-lg shadow-sm transition flex items-center space-x-1.5"
        >
          <span>📄</span>
          <span>View Master Resume</span>
        </a>
      </div>

      {/* Master Skills Matrix */}
      <div className="bg-white p-6 rounded-xl border border-[#e8ece9] shadow-xs space-y-3">
        <h3 className="text-sm font-bold text-[#1d2825]">Verified Technical Expertise</h3>
        <p className="text-xs text-[#78827e]">
          These competencies are cross-referenced against employer JDs by the ATS Match Engine to generate fit scores.
        </p>

        <div className="flex flex-wrap gap-2 pt-2">
          {skills.map(skill => (
            <span
              key={skill}
              className="px-3 py-1 bg-[#f4f7f5] text-[#1d2825] text-xs font-semibold rounded-md border border-[#e2e8e4]"
            >
              {skill}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}
