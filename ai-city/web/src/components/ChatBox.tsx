'use client';

import { useEffect, useState } from 'react';
import { api } from '@/lib/api';
import { getNpcDisplayName } from '@/lib/npc-positions';
import { useGameStore } from '@/store/game';

interface Message {
  from: string;
  content: string;
  isPlayer: boolean;
}

interface DialogOption {
  id: string;
  text: string;
}

export function ChatBox() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [options, setOptions] = useState<DialogOption[]>([]);
  const [sending, setSending] = useState(false);
  const playerId = useGameStore((s) => s.playerId);
  const npcId = useGameStore((s) => s.selectedNpcId);
  const npcName = npcId ? getNpcDisplayName(npcId) : '';

  useEffect(() => {
    setMessages([]);
    setOptions([]);
    setInput('');
  }, [npcId]);

  const talk = async (text: string, choiceId = 'root') => {
    if (!npcId || !text.trim() || sending) return;
    setMessages((m) => [...m, { from: '我', content: text, isPlayer: true }]);
    setSending(true);
    setInput('');

    try {
      const reply = await api.postNpcTalk(npcId, choiceId, playerId || 'guest-preview-player');
      setMessages((m) => [...m, { from: npcName, content: reply.say, isPlayer: false }]);
      setOptions(reply.options || []);
    } catch (error) {
      setMessages((m) => [...m, {
        from: '系统',
        content: error instanceof Error ? error.message : '对话请求失败',
        isPlayer: false,
      }]);
    } finally {
      setSending(false);
    }
  };

  const send = () => {
    if (!npcId) return;
    void talk(input);
  };

  const choose = (option: DialogOption) => {
    void talk(option.text, option.id);
  };

  return (
    <div className="absolute bottom-4 left-4 right-4 max-w-2xl mx-auto bg-gray-800/90 backdrop-blur rounded-lg shadow-2xl">
      <div className="h-48 overflow-y-auto p-3 space-y-2">
        {messages.length === 0 && (
          <div className="text-gray-500 text-sm text-center py-8">
            {npcId
              ? `正在和 ${npcName} 对话；点击选项或直接发送消息...`
              : '点击地图上的 NPC 开始对话。'}
          </div>
        )}
        {messages.map((m, i) => (
          <div key={i} className={`text-sm ${m.isPlayer ? 'text-right' : 'text-left'}`}>
            <span className={`font-bold ${m.isPlayer ? 'text-brand-500' : 'text-emerald-400'}`}>
              {m.from}:
            </span>{' '}
            {m.content}
          </div>
        ))}
      </div>

      {options.length > 0 && (
        <div className="flex flex-wrap gap-2 border-t border-gray-700 px-3 py-2">
          {options.map((option) => (
            <button
              key={option.id}
              onClick={() => choose(option)}
              disabled={sending}
              className="rounded-full bg-gray-700 px-3 py-1 text-xs text-gray-100 hover:bg-gray-600 disabled:opacity-50"
            >
              {option.text}
            </button>
          ))}
        </div>
      )}

      <div className="flex border-t border-gray-700">
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && send()}
          className="flex-1 bg-transparent p-3 outline-none"
          disabled={!npcId}
          placeholder={npcId ? `和 ${npcName} 说点什么...` : '先选中一个 NPC'}
        />
        <button
          onClick={send}
          disabled={sending || !input.trim()}
          className="px-4 bg-brand-500 hover:bg-brand-700 disabled:opacity-50 transition"
        >
          发送
        </button>
      </div>
    </div>
  );
}
