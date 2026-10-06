import Link from 'next/link';
import { cookies } from 'next/headers';
import { COOKIE_NAME, decodeToken, isAdmin } from '@/lib/auth';

const dashboards = [
  { name: 'NPC 管理', href: '/npc', desc: '20+ NPC 模板与配置' },
  { name: 'Saga Dashboard', href: '/saga', desc: '5 个核心指标（§32.7）' },
  { name: 'Saga 可视化', href: '/saga-viz', desc: '只读流程图（Phase B）' },
  { name: 'BT 编辑器', href: '/bt-editor', desc: '行为树可视化（§E.1）' },
  { name: 'Saga DSL IDE', href: '/saga-dsl-ide', desc: 'DSL 编辑器（§E.2）' },
  { name: '创作者市场', href: '/marketplace', desc: 'NPC / 剧本 / BT 上架' },
];

function getJwtSecret(): string {
  return (
    process.env.ADMIN_PORTAL_JWT_SECRET ||
    process.env.JWT_SECRET ||
    'dev-secret-change-me'
  );
}

export default async function Home() {
  const cookieStore = await cookies();
  const token = cookieStore.get(COOKIE_NAME)?.value ?? '';
  const session = decodeToken(token, getJwtSecret());
  const loggedIn = isAdmin(session);

  return (
    <main className="min-h-screen p-8">
      <header className="mb-8 flex items-start justify-between">
        <div>
          <h1 className="text-3xl font-bold">AI City Admin Portal</h1>
          <p className="text-gray-600">运营后台 v0.1</p>
        </div>
        <div className="flex items-center gap-3">
          {loggedIn ? (
            <>
              <span
                data-testid="session-user"
                className="text-sm text-gray-600"
              >
                登录身份：<strong>{session!.username}</strong> (admin)
              </span>
              <form action="/api/auth/logout" method="POST">
                <button
                  type="submit"
                  data-testid="logout-btn"
                  className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
                >
                  退出
                </button>
              </form>
            </>
          ) : (
            <Link
              href={'/login' as unknown as __next_route_internal_types__.RouteImpl<string>}
              data-testid="login-link"
              className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
            >
              管理员登录
            </Link>
          )}
        </div>
      </header>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {dashboards.map((d) => (
          <Link
            key={d.href}
            href={d.href as unknown as __next_route_internal_types__.RouteImpl<string>}
            className="block p-6 bg-white rounded-lg border border-gray-200 hover:border-brand-500 transition"
          >
            <h2 className="text-lg font-semibold">{d.name}</h2>
            <p className="text-sm text-gray-500 mt-1">{d.desc}</p>
          </Link>
        ))}
      </div>
    </main>
  );
}