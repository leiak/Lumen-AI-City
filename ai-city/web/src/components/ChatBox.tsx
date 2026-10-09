'use client';

import { useState } from 'react';
import { api } from '@/lib/api';
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

const NPC_ID = 'npc_wang_boss_001';
const NPC_NAME = '王老板';

export function ChatBox() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [options, setOptions] = useState<DialogOption[]>([]);
  const [sending, setSending] = useState(false);
  const playerId = useGameStore((s) => s.playerId);

  const talk = async (text: string, choiceId = 'root') => {
    if (!text.trim() || sending) return;
    setMessages((m) => [...m, { from: '我', content: text, isPlayer: true }]);
    setSending(true);
    setInput('');

    try {
      const reply = await api.postNpcTalk(
        NPC_ID,
        choiceId,
        playerId || 'guest-preview-player',
      );
      setMessages((m) => [...m, { from: NPC_NAME, content: reply.say, isPlayer: false }]);
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
            正在和 {NPC_NAME} 对话；点击选项或直接发送消息...
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
          placeholder={`和 ${NPC_NAME} 说点什么...`}
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
