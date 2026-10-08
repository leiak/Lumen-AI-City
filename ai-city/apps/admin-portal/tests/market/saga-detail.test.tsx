import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { SagaTemplateDetail } from '@/app/market/saga-templates/[id]/SagaTemplateDetail';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
}

function jsonResponse(status: number, body: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  };
}

function mockTemplate(purchaseResponse: { status: number; body: unknown }) {
  mockFetch.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (
      url === '/api/marketplace/saga-templates/51' &&
      (!init || init.method === undefined || init.method === 'GET')
    ) {
      return jsonResponse(200, {
        id: 51,
        name: 'WelcomeSaga',
        status: 'live',
        price_gold: 100,
        description: 'A welcome story',
        yaml_content: 'saga:\n  name: WelcomeSaga\n',
        npc_deps: ['npc_wang_boss_001', 'npc_demo'],
      });
    }
    if (url === '/api/marketplace/purchase' && init?.method === 'POST') {
      return jsonResponse(purchaseResponse.status, purchaseResponse.body);
    }
    throw new Error(`unexpected fetch: ${url}`);
  });
}

beforeEach(() => {
  mockFetch.mockReset();
});

describe('SagaTemplateDetail', () => {
  it('renders YAML preview, NPC deps, and purchase action', async () => {
    mockTemplate({ status: 200, body: { purchase_id: 71 } });

    renderWithQuery(<SagaTemplateDetail templateId={51} />);

    expect(await screen.findByRole('heading', { name: 'WelcomeSaga' })).toBeInTheDocument();
    expect(screen.getByTestId('saga-yaml-preview')).toHaveTextContent('name: WelcomeSaga');
    expect(screen.getByText('npc_wang_boss_001')).toBeInTheDocument();
    expect(screen.getByText('npc_demo')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '购买' })).toBeInTheDocument();
  });

  it('posts a saga purchase with an idempotency key', async () => {
    mockTemplate({ status: 200, body: { purchase_id: 71 } });

    renderWithQuery(<SagaTemplateDetail templateId={51} />);
    fireEvent.click(await screen.findByRole('button', { name: '购买' }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/marketplace/purchase',
        expect.objectContaining({ method: 'POST' }),
      );
      expect(screen.getByText('购买成功')).toBeInTheDocument();
    });
    const body = JSON.parse(mockFetch.mock.calls[1][1].body);
    expect(body).toMatchObject({ template_kind: 'saga', template_id: 51 });
    expect(body.idempotency_key).toBeTruthy();
  });

  it('shows a readable detail error', async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse(404, {
        detail: { code: 'R_028', msg: 'template not found' },
      }),
    );

    renderWithQuery(<SagaTemplateDetail templateId={99} />);

    await waitFor(() => {
      expect(screen.getByText(/R_028/)).toBeInTheDocument();
      expect(screen.getByText(/template not found/)).toBeInTheDocument();
    });
  });

  it('shows a readable purchase error', async () => {
    mockTemplate({
      status: 410,
      body: { detail: { code: 'R_029', msg: 'template taken down' } },
    });

    renderWithQuery(<SagaTemplateDetail templateId={51} />);
    fireEvent.click(await screen.findByRole('button', { name: '购买' }));

    await waitFor(() => {
      expect(screen.getByText(/R_029/)).toBeInTheDocument();
      expect(screen.getByText(/template taken down/)).toBeInTheDocument();
    });
  });
});
