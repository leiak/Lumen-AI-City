/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  transpilePackages: ['@aicity/client-reconciler', '@aicity/offline-sync', '@aicity/bypass-filter'],
  experimental: {
    typedRoutes: true,
  },
  env: {
    NEXT_PUBLIC_API_GATEWAY: process.env.NEXT_PUBLIC_API_GATEWAY || 'http://localhost:8080',
    NEXT_PUBLIC_WS_GATEWAY: process.env.NEXT_PUBLIC_WS_GATEWAY || 'ws://localhost:8082/ws',
  },
};

module.exports = nextConfig;
