/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  transpilePackages: ['@aicity/bt-editor-spec', '@aicity/saga-dsl'],
  experimental: {
    typedRoutes: true,
  },
};

module.exports = nextConfig;
