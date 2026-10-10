'use client';

import { useState } from 'react';
import { api } from '@/lib/api';
import { useGameStore } from '@/store/game';

export function CreatorStudio({ onPublished }: { onPublished?: () => void }) {
  const [kind, setKind] = useState<'npc' | 'saga'>('npc');
  const [name, setName] = useState('');
  const [price, setPrice] = useState('30');
  const [semanticVersion, setSemanticVersion] = useState('1.0.0');
  const [description, setDescription] = useState('');
  const [yamlContent, setYamlContent] = useState('saga:\n  name: example\n');
  const [npcDeps, setNpcDeps] = useState('');
  const [publishing, setPublishing] = useState(false);
  const [success, setSuccess] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const sessionStatus = useGameStore((s) => s.sessionStatus);

  async function publish() {
    if (sessionStatus !== 'authenticated') return;
    const parsedPrice = Number(price);
    if (!Number.isInteger(parsedPrice) || parsedPrice < 10) {
      setError('价格必须是不低于 10 的整数');
      return;
    }
    setPublishing(true);
    setError(null);
    setSuccess(null);
    try {
      if (kind === 'npc') {
        const response = await api.createNpcTemplate({
          name,
          priceGold: parsedPrice,
          ocean: { O: 0.5, C: 0.5, E: 0.5, A: 0.5, N: 0.5 },
        });
        setSuccess(`已发布 NPC 模板 #${response.id}`);
      } else {
        const response = await api.createSagaTemplate({
          name,
          priceGold: parsedPrice,
          semanticVersion,
          description: description || undefined,
          yamlContent,
          npcDeps: npcDeps
            .split(',')
            .map((value) => value.trim())
            .filter(Boolean),
        });
        setSuccess(`已发布剧本模板 #${response.id}`);
      }
      setName('');
      setDescription('');
      onPublished?.();
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : '发布失败');
    } finally {
      setPublishing(false);
    }
  }

  return (
    <div className="mt-2 space-y-2 border-t border-slate-700 pt-2">
      <div className="flex items-center gap-2 text-xs">
        <select
          value={kind}
          onChange={(event) => setKind(event.target.value as 'npc' | 'saga')}
          className="rounded bg-slate-900 px-2 py-1 text-slate-100"
        >
          <option value="npc">NPC 模板</option>
          <option value="saga">剧本模板</option>
        </select>
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="作品名称"
          className="min-w-0 flex-1 rounded bg-slate-900 px-2 py-1 text-slate-100"
        />
      </div>
      <div className="flex items-center gap-2">
        <input
          type="number"
          min={10}
          value={price}
          onChange={(event) => setPrice(event.target.value)}
          className="w-20 rounded bg-slate-900 px-2 py-1 text-xs text-slate-100"
        />
        {kind === 'saga' && (
          <input
            value={semanticVersion}
            onChange={(event) => setSemanticVersion(event.target.value)}
            placeholder="1.0.0"
            className="w-24 rounded bg-slate-900 px-2 py-1 text-xs text-slate-100"
          />
        )}
      </div>
      {kind === 'saga' && (
        <>
          <input
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder="简介（可选）"
            className="w-full rounded bg-slate-900 px-2 py-1 text-xs text-slate-100"
          />
          <input
            value={npcDeps}
            onChange={(event) => setNpcDeps(event.target.value)}
            placeholder="NPC 依赖，逗号分隔"
            className="w-full rounded bg-slate-900 px-2 py-1 text-xs text-slate-100"
          />
          <textarea
            value={yamlContent}
            onChange={(event) => setYamlContent(event.target.value)}
            rows={4}
            className="w-full rounded bg-slate-900 px-2 py-1 font-mono text-xs text-slate-100"
          />
        </>
      )}
      <button
        type="button"
        disabled={publishing || !name}
        onClick={() => void publish()}
        className="w-full rounded bg-brand-500 px-3 py-1 text-xs font-semibold text-white hover:bg-brand-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {publishing ? '发布中' : '发布到市场'}
      </button>
      {success && <div className="text-xs text-green-400">{success}</div>}
      {error && <div className="text-xs text-red-400">{error}</div>}
    </div>
  );
}
