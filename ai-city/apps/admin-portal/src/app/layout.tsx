import './globals.css';
import type { Metadata } from 'next';
import { Layout } from '@/components/Layout';
import { Providers } from '@/components/Providers';

export const metadata: Metadata = {
  title: 'AI City Admin',
  description: 'AI 城邦运营后台',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="text-gray-900">
        <Providers>
          <Layout>{children}</Layout>
        </Providers>
      </body>
    </html>
  );
}
