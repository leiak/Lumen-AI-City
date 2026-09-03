/**
 * AI City TypeScript SDK
 *
 * 详细设计见 docs/06-A2A协议.md §20.17
 */

export interface AgentCard {
  agent_id: string;
  name: string;
  description?: string;
  url: string;
  provider?: string;
  version?: string;
  capabilities?: string[];
  auth?: Record<string, string>;
}

export class A2AClient {
  constructor(
    private gatewayUrl: string,
    private apiKey: string,
  ) {}

  async registerCard(card: AgentCard): Promise<{ card_id: string }> {
    const resp = await fetch(`${this.gatewayUrl}/v1/cards`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${this.apiKey}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(card),
    });
    if (!resp.ok) throw new Error(`registerCard failed: ${resp.status}`);
    return resp.json();
  }

  async discover(capability: string): Promise<AgentCard[]> {
    const resp = await fetch(`${this.gatewayUrl}/v1/discover?capability=${capability}`, {
      headers: { 'Authorization': `Bearer ${this.apiKey}` },
    });
    if (!resp.ok) throw new Error(`discover failed: ${resp.status}`);
    return resp.json();
  }

  async sendMessage(toAgentId: string, payload: unknown): Promise<{ delivered: boolean }> {
    const resp = await fetch(`${this.gatewayUrl}/v1/messages`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${this.apiKey}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ to_agent_id: toAgentId, payload }),
    });
    if (!resp.ok) throw new Error(`sendMessage failed: ${resp.status}`);
    return resp.json();
  }
}
