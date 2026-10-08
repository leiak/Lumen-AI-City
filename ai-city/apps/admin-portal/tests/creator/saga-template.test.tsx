import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { SagaTemplateList } from '@/app/creator/saga-templates/SagaTemplateList';
import { SagaTemplateEdit } from '@/app/creator/saga-templates/new/SagaTemplateEdit';

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

function jsonResponse(status: number, body: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  };
}

beforeEach(() => {
  mockFetch.mockReset();
  mockPush.mockReset();
});

describe('SagaTemplateList', () => {
  it('renders saga templates and the create action for a creator', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/auth/me') {
        return jsonResponse(200, { username: 'creator', role: 'creator' });
      }
      if (url.startsWith('/api/marketplace/saga-templates')) {
        return jsonResponse(200, [
          {
            id: 51,
            name: 'WelcomeSaga',
            semantic_version: '1.0.0',
            status: 'live',
            price_gold: 100,
            created_at: '2026-10-08T00:00:00Z',
          },
        ]);
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<SagaTemplateList />);

    expect(await screen.findByRole('heading', { name: 'Saga Templates' })).toBeInTheDocument();
    expect(screen.getByTestId('saga-template-row-51')).toHaveTextContent('WelcomeSaga');
    expect(screen.getByTestId('saga-template-row-51')).toHaveTextContent('1.0.0');
    expect(screen.getByTestId('create-saga-template')).toHaveAttribute(
      'href',
      '/creator/saga-templates/new',
    );
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/marketplace/saga-templates?limit=50&offset=0&status=live',
      expect.anything(),
    );
  });

  it('hides the create action for players', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/auth/me') {
        return jsonResponse(200, { username: 'player', role: 'player' });
      }
      if (url.startsWith('/api/marketplace/saga-templates')) {
        return jsonResponse(200, []);
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<SagaTemplateList />);

    expect(await screen.findByText('暂无 Saga 模板')).toBeInTheDocument();
    expect(screen.queryByTestId('create-saga-template')).not.toBeInTheDocument();
  });
});

describe('SagaTemplateEdit', () => {
  it('renders YAML, NPC deps, version, and price fields', () => {
    renderWithQuery(<SagaTemplateEdit />);

    expect(screen.getByLabelText(/名称/)).toBeInTheDocument();
    expect(screen.getByLabelText(/YAML 内容/)).toBeInTheDocument();
    expect(screen.getByLabelText(/依赖 NPC/)).toBeInTheDocument();
    expect(screen.getByLabelText(/语义化版本/)).toBeInTheDocument();
    expect(screen.getByLabelText(/售价/)).toBeInTheDocument();
  });

  it('submits a saga template payload', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse(201, { id: 51 }));

    renderWithQuery(<SagaTemplateEdit />);
    fireEvent.change(screen.getByLabelText(/名称/), {
      target: { value: 'WelcomeSaga' },
    });
    fireEvent.change(screen.getByLabelText(/依赖 NPC/), {
      target: { value: 'npc_1, npc_2' },
    });
    fireEvent.change(screen.getByLabelText(/售价/), { target: { value: '100' } });
    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/marketplace/saga-templates',
        expect.objectContaining({ method: 'POST' }),
      );
    });
    const options = mockFetch.mock.calls[0][1];
    expect(JSON.parse(options.body)).toMatchObject({
      name: 'WelcomeSaga',
      price_gold: 100,
      npc_deps: ['npc_1', 'npc_2'],
      semantic_version: '1.0.0',
    });
    expect(options.body).toContain('saga:');
    expect(mockPush).toHaveBeenCalledWith('/creator/saga-templates');
  });

  it('shows an API error and does not navigate', async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse(400, {
        error: { code: 'R_032', msg: 'YAML content invalid' },
      }),
    );

    renderWithQuery(<SagaTemplateEdit />);
    fireEvent.change(screen.getByLabelText(/名称/), {
      target: { value: 'BrokenSaga' },
    });
    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() => {
      expect(screen.getByText(/R_032/)).toBeInTheDocument();
      expect(screen.getByText(/YAML content invalid/)).toBeInTheDocument();
    });
    expect(mockPush).not.toHaveBeenCalled();
  });
});
