import Link from 'next/link';

const dashboards = [
  { name: 'NPC 管理', href: '/npc', desc: '20+ NPC 模板与配置' },
  { name: 'Saga Dashboard', href: '/saga', desc: '5 个核心指标（§32.7）' },
  { name: 'BT 编辑器', href: '/bt-editor', desc: '行为树可视化（§E.1）' },
  { name: 'Saga DSL IDE', href: '/saga-dsl-ide', desc: 'DSL 编辑器（§E.2）' },
  { name: '创作者市场', href: '/marketplace', desc: 'NPC / 剧本 / BT 上架' },
];

export default function Home() {
  return (
    <main className="min-h-screen p-8">
      <header className="mb-8">
        <h1 className="text-3xl font-bold">AI City Admin Portal</h1>
        <p className="text-gray-600">运营后台 v0.1</p>
      </header>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {dashboards.map((d) => (
          <Link
            key={d.href}
            href={d.href}
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
