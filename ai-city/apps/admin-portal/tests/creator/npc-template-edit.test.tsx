import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NpcTemplateEdit } from '@/app/creator/npc-templates/new/NpcTemplateEdit';

const mockFetch = vi.fn();
const mockPush = vi.fn();

vi.stubGlobal('fetch', mockFetch);
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}));

function renderWithQuery(ui: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  mockFetch.mockReset();
  mockPush.mockReset();
});

describe('NpcTemplateEdit', () => {
  it('renders OCEAN sliders, BT editor, products, and price', () => {
    renderWithQuery(<NpcTemplateEdit />);

    expect(screen.getByLabelText(/名称/)).toBeInTheDocument();
    expect(screen.getByLabelText(/开放性 O/)).toBeInTheDocument();
    expect(screen.getByLabelText(/尽责性 C/)).toBeInTheDocument();
    expect(screen.getByLabelText(/外向性 E/)).toBeInTheDocument();
    expect(screen.getByLabelText(/宜人性 A/)).toBeInTheDocument();
    expect(screen.getByLabelText(/神经质 N/)).toBeInTheDocument();
    expect(screen.getByLabelText(/BT 骨架/)).toBeInTheDocument();
    expect(screen.getByLabelText(/售价/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '添加商品' })).toBeInTheDocument();
  });

  it('submits a creator template payload', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({ id: 41 }),
    });

    renderWithQuery(<NpcTemplateEdit />);
    fireEvent.change(screen.getByLabelText(/名称/), {
      target: { value: 'Merchant NPC' },
    });
    fireEvent.change(screen.getByLabelText(/BT 骨架/), {
      target: { value: 'selector: greet_or_idle' },
    });
    fireEvent.change(screen.getByLabelText(/售价/), { target: { value: '120' } });
    fireEvent.click(screen.getByRole('button', { name: '添加商品' }));
    fireEvent.change(screen.getByLabelText('商品名称'), {
      target: { value: 'Greeting' },
    });
    fireEvent.change(screen.getByLabelText('商品价格'), {
      target: { value: '10' },
    });
    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/marketplace/npc-templates',
        expect.objectContaining({ method: 'POST' }),
      );
    });
    const options = mockFetch.mock.calls[0][1];
    expect(JSON.parse(options.body)).toEqual({
      name: 'Merchant NPC',
      price_gold: 120,
      ocean_json: { O: 0.5, C: 0.5, E: 0.5, A: 0.5, N: 0.5 },
      bt_skeleton: 'selector: greet_or_idle',
      product_catalog: [{ name: 'Greeting', price_gold: 10 }],
    });
    expect(mockPush).toHaveBeenCalledWith('/creator/npc-templates');
  });

  it('shows an API error and does not navigate', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 400,
      json: async () => ({
        error: { code: 'R_031', msg: 'BT skeleton invalid' },
      }),
    });

    renderWithQuery(<NpcTemplateEdit />);
    fireEvent.change(screen.getByLabelText(/名称/), {
      target: { value: 'Broken NPC' },
    });
    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() => {
      expect(screen.getByText(/R_031/)).toBeInTheDocument();
      expect(screen.getByText(/BT skeleton invalid/)).toBeInTheDocument();
    });
    expect(mockPush).not.toHaveBeenCalled();
  });
});
