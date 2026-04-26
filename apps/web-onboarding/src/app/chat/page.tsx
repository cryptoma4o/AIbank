"use client";
import { useState, useRef, useEffect } from "react";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { sendChatMessage } from "@/lib/api";
import type { ChatMessage } from "@/types";

export default function ChatPage() {
  const [messages, setMessages] = useState<ChatMessage[]>([
    { role: "assistant", content: "Здравствуйте! Я помогу вам открыть расчётный счёт. Как могу помочь?" }
  ]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); }, [messages]);

  async function send() {
    if (!input.trim() || loading) return;
    const userMsg: ChatMessage = { role: "user", content: input };
    setMessages(m => [...m, userMsg]);
    setInput("");
    setLoading(true);
    try {
      const data = await sendChatMessage("session-1", "demo-bank", "app-1", input, messages);
      setMessages(m => [...m, { role: "assistant", content: data.assistant_message }]);
    } catch {
      setMessages(m => [...m, { role: "assistant", content: "Извините, сервис временно недоступен." }]);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Помощник по открытию счёта</h1>
      <Card className="flex flex-col h-[60vh]">
        <div className="flex-1 overflow-y-auto flex flex-col gap-3 mb-4">
          {messages.map((m, i) => (
            <div key={i} className={`flex ${m.role === "user" ? "justify-end" : "justify-start"}`}>
              <div className={`max-w-[75%] px-4 py-2.5 rounded-2xl text-sm leading-relaxed
                ${m.role === "user" ? "bg-primary text-white rounded-br-sm" : "bg-gray-100 text-gray-800 rounded-bl-sm"}`}>
                {m.content}
              </div>
            </div>
          ))}
          {loading && <div className="flex justify-start"><div className="bg-gray-100 px-4 py-2.5 rounded-2xl rounded-bl-sm text-sm text-gray-400">Печатает…</div></div>}
          <div ref={bottomRef} />
        </div>
        <div className="flex gap-3 pt-3 border-t border-gray-100">
          <input value={input} onChange={e => setInput(e.target.value)}
            onKeyDown={e => e.key === "Enter" && !e.shiftKey && send()}
            placeholder="Введите сообщение…"
            className="flex-1 border border-gray-300 rounded-xl px-4 py-2.5 text-sm outline-none focus:ring-2 focus:ring-primary focus:border-transparent" />
          <Button onClick={send} disabled={loading || !input.trim()}>Отправить</Button>
        </div>
      </Card>
    </div>
  );
}
