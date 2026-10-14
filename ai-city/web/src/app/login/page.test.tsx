// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';

const push = vi.fn();

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
}));

vi.mock('@/lib/api', () => ({
  api: {
    register: vi.fn(),
    login: vi.fn(),
    setToken: vi.fn(),
  },
}));

import { api } from '@/lib/api';
import LoginPage from './page';

describe('LoginPage registration', () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.mocked(api.register).mockReset();
    vi.mocked(api.login).mockReset();
    vi.mocked(api.setToken).mockReset();
    push.mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  it('registers a new account then logs in and enters the city', async () => {
    vi.mocked(api.register).mockResolvedValue({
      player_id: 'player-1',
      username: 'newuser',
      display_name: 'New Player',
    });
    vi.mocked(api.login).mockResolvedValue({
      token: 'test-token',
      player_id: 'player-1',
      username: 'newuser',
      display_name: 'New Player',
      expires_at: '',
    });

    render(<LoginPage />);

    fireEvent.click(screen.getByText('没有账号？注册一个'));
    fireEvent.change(screen.getByLabelText('用户名'), {
      target: { value: 'newuser' },
    });
    fireEvent.change(screen.getByLabelText('邮箱'), {
      target: { value: 'newuser@example.com' },
    });
    fireEvent.change(screen.getByLabelText('显示昵称'), {
      target: { value: 'New Player' },
    });
    fireEvent.change(screen.getByLabelText('密码'), {
      target: { value: 'password123' },
    });
    fireEvent.click(screen.getByRole('button', { name: '注册并进入城市' }));

    await waitFor(() => {
      expect(api.register).toHaveBeenCalledWith({
        username: 'newuser',
        email: 'newuser@example.com',
        password: 'password123',
        displayName: 'New Player',
      });
      expect(api.login).toHaveBeenCalledWith('newuser', 'password123');
      expect(push).toHaveBeenCalledWith('/city');
    });

    expect(api.setToken).toHaveBeenCalledWith('test-token');
    expect(window.localStorage.getItem('aicity_token')).toBe('test-token');
    expect(window.localStorage.getItem('aicity_player_id')).toBe('player-1');
  });
});
