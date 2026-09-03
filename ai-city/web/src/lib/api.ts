/**
 * API 客户端封装。
 */
const API_BASE = process.env.NEXT_PUBLIC_API_GATEWAY || 'http://localhost:8080';

class ApiClient {
  private token: string | null = null;

  setToken(token: string) {
    this.token = token;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
      ...(this.token ? { Authorization: `Bearer ${this.token}` } : {}),
      ...(options.headers ?? {}),
    };
    const resp = await fetch(`${API_BASE}${path}`, { ...options, headers });
    if (!resp.ok) {
      throw new Error(`API ${resp.status}: ${await resp.text()}`);
    }
    return resp.json();
  }

  login = (username: string, password: string) =>
    this.request<{ token: string }>('/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    });

  getNpc = (id: string) => this.request<unknown>(`/v1/npcs/${id}`);

  dialogue = (npcId: string, message: string) =>
    this.request<{ reply: string }>(`/v1/npcs/${npcId}/dialogue`, {
      method: 'POST',
      body: JSON.stringify({ message }),
    });
}

export const api = new ApiClient();
