'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function Home() {
  const router = useRouter();

  useEffect(() => {
    const token = localStorage.getItem('aicity_token');
    router.push(token ? '/city' : '/login');
  }, [router]);

  return (
    <main className="min-h-screen flex items-center justify-center">
      <div className="text-gray-400">载入中...</div>
    </main>
  );
}