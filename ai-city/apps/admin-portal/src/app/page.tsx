import Link from 'next/link';

const dashboards = [
  { name: 'Wallet', href: '/wallet', desc: '查余额 + 详情' },
  { name: 'Transactions', href: '/transactions', desc: '历史分页' },
  { name: 'Admin Tools', href: '/admin', desc: 'emit / sink 面板' },
];

export default function Home() {
  return (
    <div>
      <h1 className="text-2xl font-bold mb-2">Dashboard</h1>
      <p className="text-gray-600 mb-6">运营后台 v0.1 — 3.0 经济系统</p>

      <h2 className="text-lg font-semibold mb-3">3.0 Wallet Tools</h2>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {dashboards.map((d) => (
          <Link
            key={d.href}
            href={d.href as any}
            className="block p-6 bg-white rounded-lg border border-gray-200 hover:border-blue-500 transition"
          >
            <h3 className="font-semibold">{d.name}</h3>
            <p className="text-sm text-gray-500 mt-1">{d.desc}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}
