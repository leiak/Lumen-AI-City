import Link from 'next/link';

export default function Home() {
  return (
    <main className="min-h-screen flex flex-col items-center justify-center p-8">
      <h1 className="text-5xl font-bold mb-4">AI 城邦</h1>
      <p className="text-gray-400 mb-8">基于真实或半虚构地图的 AI Agent 城市平台</p>
      <Link
        href="/city"
        className="px-6 py-3 bg-brand-500 hover:bg-brand-700 rounded-lg transition"
      >
        进入城市
      </Link>
    </main>
  );
}
