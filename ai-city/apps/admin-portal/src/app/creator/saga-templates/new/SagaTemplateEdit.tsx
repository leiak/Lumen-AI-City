'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';

interface ApiError {
  code: string;
  msg: string;
}

const DEFAULT_YAML = 'saga:\n  name: my-saga\n  steps:\n    - task: hello\n';

function describeError(error: unknown): ApiError {
  const raw = (error as Error)?.message ?? String(error);
  try {
    const parsed = JSON.parse(raw);
    const apiError = parsed?.error as ApiError | undefined;
    return {
      code: apiError?.code ?? 'UNKNOWN',
      msg: apiError?.msg ?? raw,
    };
  } catch {
    return { code: 'UNKNOWN', msg: raw };
  }
}

export function SagaTemplateEdit() {
  const router = useRouter();
  const [name, setName] = useState('');
  const [yamlContent, setYamlContent] = useState(DEFAULT_YAML);
  const [semanticVersion, setSemanticVersion] = useState('1.0.0');
  const [npcDeps, setNpcDeps] = useState('');
  const [priceGold, setPriceGold] = useState('10');
  const [description, setDescription] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    const payload = {
      name: name.trim(),
      yaml_content: yamlContent,
      semantic_version: semanticVersion,
      npc_deps: npcDeps
        .split(',')
        .map((dep) => dep.trim())
        .filter(Boolean),
      price_gold: Number(priceGold),
      ...(description.trim() ? { description: description.trim() } : {}),
    };

    try {
      const response = await fetch('/api/marketplace/saga-templates', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify(payload),
      });
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(JSON.stringify(body));
      }
      router.push('/creator/saga-templates');
    } catch (requestError) {
      setError(describeError(requestError));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} className="max-w-3xl space-y-6" data-testid="saga-template-edit">
      <div>
        <h1 className="text-2xl font-bold">Create Saga Template</h1>
      </div>

      {error && (
        <p className="text-red-600">
          错误 [{error.code}]: {error.msg}
        </p>
      )}

      <div className="bg-white border border-gray-200 rounded p-4 grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label htmlFor="saga-name" className="block text-sm font-medium text-gray-700">
            名称
          </label>
          <input
            id="saga-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            maxLength={64}
            className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
          />
        </div>
        <div>
          <label htmlFor="saga-price-gold" className="block text-sm font-medium text-gray-700">
            售价（Gold）
          </label>
          <input
            id="saga-price-gold"
            type="number"
            min={10}
            value={priceGold}
            onChange={(event) => setPriceGold(event.target.value)}
            required
            className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
          />
        </div>
        <div>
          <label htmlFor="saga-version" className="block text-sm font-medium text-gray-700">
            语义化版本
          </label>
          <input
            id="saga-version"
            value={semanticVersion}
            onChange={(event) => setSemanticVersion(event.target.value)}
            required
            pattern="\d+\.\d+\.\d+"
            className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
          />
        </div>
        <div>
          <label htmlFor="saga-npc-deps" className="block text-sm font-medium text-gray-700">
            依赖 NPC
          </label>
          <input
            id="saga-npc-deps"
            value={npcDeps}
            onChange={(event) => setNpcDeps(event.target.value)}
            placeholder="npc_wang_boss_001, npc_demo"
            className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
          />
        </div>
        <div className="md:col-span-2">
          <label htmlFor="saga-description" className="block text-sm font-medium text-gray-700">
            描述
          </label>
          <textarea
            id="saga-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            rows={3}
            className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
          />
        </div>
      </div>

      <div className="bg-white border border-gray-200 rounded p-4">
        <label htmlFor="saga-yaml" className="block text-sm font-medium text-gray-700">
          YAML 内容
        </label>
        <textarea
          id="saga-yaml"
          value={yamlContent}
          onChange={(event) => setYamlContent(event.target.value)}
          rows={12}
          required
          className="mt-1 w-full border border-gray-300 rounded px-3 py-2 font-mono"
        />
      </div>

      <button
        type="submit"
        disabled={submitting}
        className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
      >
        {submitting ? '提交中…' : '创建模板'}
      </button>
    </form>
  );
}
