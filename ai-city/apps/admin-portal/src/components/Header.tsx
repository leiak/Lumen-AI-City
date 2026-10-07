'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { PlayerSelector } from './PlayerSelector';

interface Session {
  username: string;
  role: string;
}

export function Header() {
  const [session, setSession] = useState<Session | null>(null);

  useEffect(() => {
    const hasToken = document.cookie.includes('admin-token=');
    if (hasToken) {
      setSession({ username: 'admin', role: 'admin' });
    }
  }, []);

  return (
    <header className="h-14 border-b border-gray-200 bg-white px-6 flex items-center justify-between">
      <div className="flex items-center gap-6">
        <Link href="/" className="font-bold text-lg">AI City Admin</Link>
        <PlayerSelector />
      </div>
      <div className="flex items-center gap-3">
        {session ? (
          <>
            <span className="text-sm text-gray-600">
              登录身份：<strong>{session.username}</strong> ({session.role})
            </span>
            <form action="/api/auth/logout" method="POST">
              <button
                type="submit"
                className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
              >
                退出
              </button>
            </form>
          </>
        ) : (
          <Link href="/login" className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700">
            管理员登录
          </Link>
        )}
      </div>
    </header>
  );
}