import React, { useState, useRef, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
import './AgentChat.css';

const API = config.GOJIRA_API || config.API_BASE;

export default function AgentChat({ user, context, onClose }) {
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [sessionId, setSessionId] = useState(null);
  const messagesEndRef = useRef(null);
  const inputRef = useRef(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  async function handleSend() {
    const text = input.trim();
    if (!text || loading) return;

    setInput('');
    setMessages(prev => [...prev, { role: 'user', content: text }]);
    setLoading(true);

    try {
      const res = await fetch(`${API}/Agent/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user || '' },
        body: JSON.stringify({ sessionId, message: text, context }),
      });
      const data = await res.json();
      if (res.ok) {
        if (data.sessionId) setSessionId(data.sessionId);
        setMessages(prev => [...prev, {
          role: 'assistant',
          content: data.reply,
          actions: data.actions,
          suggestions: data.suggestions,
        }]);
      } else {
        setMessages(prev => [...prev, { role: 'assistant', content: `오류: ${data.error || res.statusText}` }]);
      }
    } catch (err) {
      setMessages(prev => [...prev, { role: 'assistant', content: `연결 오류: ${err.message}` }]);
    }
    setLoading(false);
  }

  function handleKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  function handleSuggestion(text) {
    setInput(text);
    setTimeout(() => inputRef.current?.focus(), 0);
  }

  return (
    <div className="ac-overlay" onClick={onClose}>
      <div className="ac-panel" onClick={e => e.stopPropagation()}>
        <div className="ac-header">
          <span className="ac-title">GoJIRA Agent</span>
          <button className="ac-close" onClick={onClose}>&times;</button>
        </div>
        <div className="ac-messages">
          {messages.length === 0 && (
            <div className="ac-welcome">
              <p>안녕하세요! GoJIRA Agent입니다.</p>
              <p>프로젝트 관리, 이슈 조회, 문서 분석 등을 도와드립니다.</p>
              <div className="ac-suggestions">
                <button onClick={() => handleSuggestion('프로젝트 이슈 현황 알려줘')}>이슈 현황 조회</button>
                <button onClick={() => handleSuggestion('일정 분석해줘')}>일정 분석</button>
                <button onClick={() => handleSuggestion('문서 요약해줘')}>문서 요약</button>
              </div>
            </div>
          )}
          {messages.map((msg, i) => (
            <div key={i} className={`ac-msg ac-msg-${msg.role}`}>
              <div className="ac-msg-content">
                {msg.role === 'assistant' ? (
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content || ''}</ReactMarkdown>
                ) : (
                  <span>{msg.content}</span>
                )}
              </div>
              {msg.actions && msg.actions.length > 0 && (
                <div className="ac-actions">
                  {msg.actions.map((a, j) => (
                    <div key={j} className="ac-action-card">
                      <span className="ac-action-skill">{a.skill}</span>
                      <span className="ac-action-status">완료</span>
                    </div>
                  ))}
                </div>
              )}
              {msg.suggestions && msg.suggestions.length > 0 && (
                <div className="ac-suggestions">
                  {msg.suggestions.map((s, j) => (
                    <button key={j} onClick={() => handleSuggestion(s)}>{s}</button>
                  ))}
                </div>
              )}
            </div>
          ))}
          {loading && (
            <div className="ac-msg ac-msg-assistant">
              <div className="ac-msg-content ac-typing">생각하는 중...</div>
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>
        <div className="ac-input-area">
          <textarea
            ref={inputRef}
            className="ac-input"
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="메시지를 입력하세요..."
            rows={1}
          />
          <button className="ac-send" onClick={handleSend} disabled={loading || !input.trim()}>전송</button>
        </div>
      </div>
    </div>
  );
}
