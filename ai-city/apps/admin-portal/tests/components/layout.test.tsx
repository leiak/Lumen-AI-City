import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Layout } from '@/components/Layout';

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('Layout', () => {
  it('sidebar renders 6 nav items', () => {
    renderWithQuery(<Layout><div>test</div></Layout>);
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Wallet')).toBeInTheDocument();
    expect(screen.getByText('Transactions')).toBeInTheDocument();
    expect(screen.getByText('Admin Tools')).toBeInTheDocument();
    expect(screen.getByText('BT Editor')).toBeInTheDocument();
    expect(screen.getByText('Saga Viz')).toBeInTheDocument();
  });

  it('renders children in main content area', () => {
    renderWithQuery(<Layout><div data-testid="child">hello</div></Layout>);
    expect(screen.getByTestId('child')).toBeInTheDocument();
  });
});