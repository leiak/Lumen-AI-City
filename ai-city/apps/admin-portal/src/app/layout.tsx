import './globals.css';
import type { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'AI City Admin',
  description: 'AI 城邦运营后台',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="bg-gray-50 text-gray-900">{children}</body>
    </html>
  );
}
