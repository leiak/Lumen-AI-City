'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

interface AgentTemplateDraft {
  id: number;
  agent_id: string;
  name: string;
  status: string;
  validation_trace: { valid?: boolean; depth?: number; nodes?: number };
  review_reason: string;
}

function describeError(error: unknown): { code: string; msg: string } {
  const raw = (error as Error)?.message ?? String(error);
  try {
    const parsed = JSON.parse(raw);
    return {
      code: parsed?.error?.code ?? 'UNKNOWN',
      msg: parsed?.error?.msg ?? raw,
    };
  } catch {
    return { code: 'UNKNOWN', msg: raw };
  }
}

export function AgentTemplateReviewClient() {
  const queryClient = useQueryClient();
  const [actionError, setActionError] = useState('');
  const [message, setMessage] = useState('');

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['agent-templates', 'submitted'],
    queryFn: async () => {
      const response = await fetch('/api/agent-templates?status=submitted', { credentials: 'same-origin' });
      const body = await response.json();
      if (!response.ok) throw new Error(JSON.stringify(body));
      return body.drafts as AgentTemplateDraft[];
    },
    retry: false,
  });

  const review = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: 'approve' | 'reject' }) => {
      const response = await fetch(`/api/agent-templates/${id}/review`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ action }),
      });
      const body = await response.json();
      if (!response.ok) throw new Error(JSON.stringify(body));
      return body;
    },
    onSuccess: async (_, variables) => {
      setActionError('');
      setMessage(`Template #${variables.id} ${variables.action === 'approve' ? 'approved' : 'rejected'}`);
      await refetch();
      queryClient.invalidateQueries({ queryKey: ['agent-templates'] });
    },
    onError: (error) => {
      setActionError(describeError(error).msg);
      setMessage('');
    },
  });

  const drafts = data ?? [];
  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Agent NPC Review</h1>
      {message && <p className="mb-4 text-sm text-green-700">{message}</p>}
      {actionError && <p className="mb-4 text-sm text-red-600">{actionError}</p>}
      {isLoading && <p className="text-gray-500">加载中…</p>}
      {error && (
        <p className="text-red-600">
          错误 [{describeError(error).code}]: {describeError(error).msg}
        </p>
      )}
      {!isLoading && drafts.length === 0 && (
        <div className="bg-white border border-gray-200 rounded p-6 text-gray-600">没有待审核模板</div>
      )}
      <div className="space-y-4">
        {drafts.map((draft) => (
          <div key={draft.id} className="bg-white border border-gray-200 rounded-lg p-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 className="font-semibold text-gray-900">
                  #{draft.id} {draft.name}
                </h2>
                <p className="text-sm text-gray-600">
                  agent: {draft.agent_id} · status: {draft.status} · nodes:{' '}
                  {draft.validation_trace?.nodes ?? '—'} · depth: {draft.validation_trace?.depth ?? '—'}
                </p>
              </div>
              <div className="flex gap-2">
                <button
                  type="button"
                  disabled={review.isPending}
                  onClick={() => review.mutate({ id: draft.id, action: 'approve' })}
                  className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
                >
                  Approve
                </button>
                <button
                  type="button"
                  disabled={review.isPending}
                  onClick={() => review.mutate({ id: draft.id, action: 'reject' })}
                  className="px-3 py-1 text-sm border border-red-300 text-red-700 rounded hover:bg-red-50 disabled:opacity-50"
                >
                  Reject
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
