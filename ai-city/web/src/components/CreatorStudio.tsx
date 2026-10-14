'use client';

import { useCallback, useEffect, useState } from 'react';
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
  const [savingDraft, setSavingDraft] = useState(false);
  const [works, setWorks] = useState<Array<{ id: number; name: string; status: string }> | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const sessionStatus = useGameStore((s) => s.sessionStatus);

  const loadWorks = useCallback(async (templateKind: 'npc' | 'saga') => {
    setWorks(null);
    try {
      setWorks(
        templateKind === 'npc'
          ? await api.listCreatorNpcTemplates()
          : await api.listCreatorSagaTemplates(),
      );
    } catch {
      setWorks([]);
    }
  }, []);

  useEffect(() => {
    if (sessionStatus !== 'authenticated') return;
    void loadWorks(kind);
  }, [kind, loadWorks, sessionStatus]);

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
      await loadWorks(kind);
      onPublished?.();
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : '发布失败');
    } finally {
      setPublishing(false);
    }
  }

  async function saveDraft() {
    if (sessionStatus !== 'authenticated') return;
    const parsedPrice = Number(price);
    if (!Number.isInteger(parsedPrice) || parsedPrice < 10) {
      setError('价格必须是不低于 10 的整数');
      return;
    }
    setSavingDraft(true);
    setError(null);
    setSuccess(null);
    try {
      const response =
        kind === 'npc'
          ? await api.createNpcTemplate({
              name,
              priceGold: parsedPrice,
              ocean: { O: 0.5, C: 0.5, E: 0.5, A: 0.5, N: 0.5 },
              status: 'draft',
            })
          : await api.createSagaTemplate({
              name,
              priceGold: parsedPrice,
              semanticVersion: semanticVersion,
              description: description || undefined,
              yamlContent,
              npcDeps: npcDeps.split(',').map((value) => value.trim()).filter(Boolean),
              status: 'draft',
            });
      setSuccess(`已保存草稿 #${response.id}`);
      setName('');
      setDescription('');
      await loadWorks(kind);
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : '保存草稿失败');
    } finally {
      setSavingDraft(false);
    }
  }

  async function publishDraft(templateId: number) {
    if (sessionStatus !== 'authenticated') return;
    setSavingDraft(true);
    setError(null);
    setSuccess(null);
    try {
      await api.publishMarketTemplate({ kind, templateId });
      setSuccess(`草稿 #${templateId} 已发布`);
      await loadWorks(kind);
      onPublished?.();
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : '发布草稿失败');
    } finally {
      setSavingDraft(false);
    }
  }

  async function takeDown(templateId: number) {
    if (sessionStatus !== 'authenticated') return;
    setSavingDraft(true);
    setError(null);
    setSuccess(null);
    try {
      await api.takeDownMarketTemplate({ kind, templateId });
      setSuccess(`作品 #${templateId} 已下架`);
      await loadWorks(kind);
      onPublished?.();
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : '下架失败');
    } finally {
      setSavingDraft(false);
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
      <button
        type="button"
        disabled={savingDraft || !name}
        onClick={() => void saveDraft()}
        className="w-full rounded border border-brand-500 px-3 py-1 text-xs font-semibold text-brand-300 hover:bg-brand-900/30 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {savingDraft ? '处理中' : '存为草稿'}
      </button>
      <div className="space-y-1 border-t border-slate-700 pt-2">
        <div className="text-xs font-semibold text-slate-300">我的作品</div>
        {works === null ? (
          <div className="text-xs text-slate-400">作品加载中...</div>
        ) : works.length === 0 ? (
          <div className="text-xs text-slate-400">暂无作品</div>
        ) : (
          works.map((work) => (
            <div key={work.id} className="flex items-center justify-between gap-2 text-xs">
              <span className="min-w-0 truncate text-slate-300">
                {work.name} · {work.status === 'draft' ? '草稿' : work.status === 'live' ? '上架' : '下架'}
              </span>
              {work.status === 'draft' && (
                <button
                  type="button"
                  disabled={savingDraft}
                  onClick={() => void publishDraft(work.id)}
                  className="rounded bg-brand-500 px-2 py-1 font-semibold text-white hover:bg-brand-700 disabled:opacity-50"
                >
                  发布草稿 #{work.id}
                </button>
              )}
              {work.status === 'live' && (
                <button
                  type="button"
                  disabled={savingDraft}
                  onClick={() => void takeDown(work.id)}
                  className="rounded border border-red-500 px-2 py-1 font-semibold text-red-300 hover:bg-red-900/30 disabled:opacity-50"
                >
                  下架 #{work.id}
                </button>
              )}
            </div>
          ))
        )}
      </div>
      {success && <div className="text-xs text-green-400">{success}</div>}
      {error && <div className="text-xs text-red-400">{error}</div>}
    </div>
  );
}
