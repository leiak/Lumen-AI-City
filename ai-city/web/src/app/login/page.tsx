'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '@/lib/api';

type AuthMode = 'login' | 'register';

const FRIENDLY_ERRORS: Array<[string, string]> = [
  ['username_or_email_taken', '用户名或邮箱已被使用'],
  ['invalid_credentials', '用户名或密码不正确'],
  ['account_disabled', '账号已被禁用'],
];

function friendlyError(error: unknown) {
  const message = error instanceof Error ? error.message : '';
  return (
    FRIENDLY_ERRORS.find(([code]) => message.includes(code))?.[1] ??
    message ??
    '操作失败'
  );
}

export default function LoginPage() {
  const router = useRouter();
  const [mode, setMode] = useState<AuthMode>('login');
  const [username, setUsername] = useState('demo');
  const [password, setPassword] = useState('demo123');
  const [email, setEmail] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  function switchMode(nextMode: AuthMode) {
    setMode(nextMode);
    setError('');
    if (nextMode === 'register') {
      setUsername('');
      setPassword('');
      setEmail('');
      setDisplayName('');
    } else {
      setUsername('demo');
      setPassword('demo123');
      setEmail('');
      setDisplayName('');
    }
  }

  async function afterLogin(data: {
    token: string;
    player_id: string;
    username: string;
  }) {
    api.setToken(data.token);
    window.localStorage.setItem('aicity_token', data.token);
    window.localStorage.setItem('aicity_player_id', data.player_id);
    window.localStorage.setItem('aicity_username', data.username);
    router.push('/city');
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    setLoading(true);

    try {
      if (mode === 'register') {
        await api.register({
          username,
          email,
          password,
          displayName,
        });
      }
      const data = await api.login(username, password);
      await afterLogin(data);
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="flex min-h-screen items-center justify-center p-8">
      <div className="w-full max-w-md rounded-xl bg-gray-800 p-8 shadow-2xl">
        <h1 className="mb-2 text-3xl font-bold">AI 城邦</h1>
        <p className="mb-6 text-gray-400">
          {mode === 'login' ? '登录进入城市' : '注册并进入城市'}
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="username" className="mb-1 block text-sm text-gray-400">
              用户名
            </label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              className="w-full rounded border border-gray-700 bg-gray-900 px-3 py-2 outline-none focus:border-brand-500"
              required
              minLength={3}
              maxLength={64}
            />
          </div>

          {mode === 'register' && (
            <>
              <div>
                <label htmlFor="email" className="mb-1 block text-sm text-gray-400">
                  邮箱
                </label>
                <input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  className="w-full rounded border border-gray-700 bg-gray-900 px-3 py-2 outline-none focus:border-brand-500"
                  required
                />
              </div>
              <div>
                <label htmlFor="display-name" className="mb-1 block text-sm text-gray-400">
                  显示昵称
                </label>
                <input
                  id="display-name"
                  type="text"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                  className="w-full rounded border border-gray-700 bg-gray-900 px-3 py-2 outline-none focus:border-brand-500"
                  required
                  maxLength={64}
                />
              </div>
            </>
          )}

          <div>
            <label htmlFor="password" className="mb-1 block text-sm text-gray-400">
              密码
            </label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              className="w-full rounded border border-gray-700 bg-gray-900 px-3 py-2 outline-none focus:border-brand-500"
              required
              minLength={6}
              maxLength={128}
            />
          </div>

          {error && (
            <div className="rounded border border-red-700 bg-red-900/30 px-3 py-2 text-sm text-red-300">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full rounded bg-brand-500 px-4 py-2 font-semibold transition hover:bg-brand-700 disabled:opacity-50"
          >
            {loading
              ? mode === 'login'
                ? '登录中...'
                : '注册中...'
              : mode === 'login'
                ? '登录'
                : '注册并进入城市'}
          </button>
        </form>

        <div className="mt-4 text-center text-sm text-gray-400">
          {mode === 'login' ? (
            <button
              type="button"
              onClick={() => switchMode('register')}
              className="text-brand-400 hover:text-brand-300"
            >
              没有账号？注册一个
            </button>
          ) : (
            <button
              type="button"
              onClick={() => switchMode('login')}
              className="text-brand-400 hover:text-brand-300"
            >
              已有账号？直接登录
            </button>
          )}
        </div>

        {mode === 'login' && (
          <div className="mt-6 text-center text-xs text-gray-500">
            开发账号：demo / demo123
          </div>
        )}
      </div>
    </main>
  );
}
