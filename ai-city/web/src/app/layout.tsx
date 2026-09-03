import './globals.css';
import type { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'AI City',
  description: '全 Agent 城市平台',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="bg-gray-900 text-gray-100">{children}</body>
    </html>
  );
}
