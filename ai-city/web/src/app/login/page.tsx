'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '@/lib/api';

export default function LoginPage() {
  const router = useRouter();
  const [username, setUsername] = useState('demo');
  const [password, setPassword] = useState('demo123');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const data = await api.login(username, password);
      api.setToken(data.token);
      // 同步到 localStorage（多 tab 共享）
      localStorage.setItem('aicity_token', data.token);
      localStorage.setItem('aicity_player_id', data.player_id);
      localStorage.setItem('aicity_username', data.username);
      router.push('/city');
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="min-h-screen flex items-center justify-center p-8">
      <div className="w-full max-w-md bg-gray-800 rounded-xl shadow-2xl p-8">
        <h1 className="text-3xl font-bold mb-2">AI 城邦</h1>
        <p className="text-gray-400 mb-6">登录进入城市</p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm text-gray-400 mb-1">用户名</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 outline-none focus:border-brand-500"
              required
            />
          </div>

          <div>
            <label className="block text-sm text-gray-400 mb-1">密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 outline-none focus:border-brand-500"
              required
            />
          </div>

          {error && (
            <div className="bg-red-900/30 border border-red-700 text-red-300 px-3 py-2 rounded text-sm">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full bg-brand-500 hover:bg-brand-700 disabled:opacity-50 px-4 py-2 rounded transition font-semibold"
          >
            {loading ? '登录中...' : '登录'}
          </button>
        </form>

        <div className="mt-6 text-xs text-gray-500 text-center">
          开发账号：demo / demo123
        </div>
      </div>
    </main>
  );
}