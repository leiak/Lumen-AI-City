'use client';

import { useState } from 'react';

interface Message {
  from: string;
  content: string;
  isPlayer: boolean;
}

export function ChatBox() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');

  const send = async () => {
    if (!input.trim()) return;
    const userMsg: Message = { from: '我', content: input, isPlayer: true };
    setMessages((m) => [...m, userMsg]);

    // TODO: 通过 ws-gateway 发送
    const resp = await fetch(`${process.env.NEXT_PUBLIC_API_GATEWAY}/v1/npcs/npc_wang_boss_001/dialogue`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: input }),
    });
    const data = await resp.json();

    setMessages((m) => [...m, { from: '王老板', content: data.reply ?? '...', isPlayer: false }]);
    setInput('');
  };

  return (
    <div className="absolute bottom-4 left-4 right-4 max-w-2xl mx-auto bg-gray-800/90 backdrop-blur rounded-lg shadow-2xl">
      <div className="h-48 overflow-y-auto p-3 space-y-2">
        {messages.length === 0 && (
          <div className="text-gray-500 text-sm text-center py-8">点击地图移动，对话 NPC 开始...</div>
        )}
        {messages.map((m, i) => (
          <div key={i} className={`text-sm ${m.isPlayer ? 'text-right' : 'text-left'}`}>
            <span className="font-bold text-brand-500">{m.from}:</span> {m.content}
          </div>
        ))}
      </div>
      <div className="flex border-t border-gray-700">
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && send()}
          className="flex-1 bg-transparent p-3 outline-none"
          placeholder="输入消息..."
        />
        <button onClick={send} className="px-4 bg-brand-500 hover:bg-brand-700 transition">
          发送
        </button>
      </div>
    </div>
  );
}
