import './globals.css';
import type { Metadata } from 'next';
import { Layout } from '@/components/Layout';

export const metadata: Metadata = {
  title: 'AI City Admin',
  description: 'AI 城邦运营后台',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="text-gray-900">
        <Layout>{children}</Layout>
      </body>
    </html>
  );
}
