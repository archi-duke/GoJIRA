import React, { useState, useRef, useEffect, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
import './AgentChat.css';

const API = config.GOJIRA_API || config.API_BASE;

export default function AgentChat({ user, context, onClose }) {
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [sessionId, setSessionId] = useState(() => sessionStorage.getItem('agent_sessionId') || null);
  const [showHistory, setShowHistory] = useState(false);
  const [sessions, setSessions] = useState([]);
  const [searchQuery, setSearchQuery] = useState('');
  const messagesEndRef = useRef(null);
  const inputRef = useRef(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // 세션 ID가 있으면 이전 대화 복원
  const loadSession = useCallback(async (sid) => {
    if (!sid) return;
    try {
      const res = await fetch(`${API}/Agent/session/${sid}`, {
        headers: { 'X-User-Id': user || '' },
      });
      if (res.ok) {
        const data = await res.json();
        const msgs = (data.messages || []).map(m => {
          const msg = m.role ? m : Object.fromEntries(Object.entries(m));
          return { role: msg.role, content: msg.content, actions: msg.actions, suggestions: msg.suggestions };
        }).filter(m => m.role && m.content);
        setMessages(msgs);
        setSessionId(sid);
        sessionStorage.setItem('agent_sessionId', sid);
      } else {
        // 세션 만료됨
        setSessionId(null);
        sessionStorage.removeItem('agent_sessionId');
      }
    } catch {
      setSessionId(null);
      sessionStorage.removeItem('agent_sessionId');
    }
  }, [user]);

  useEffect(() => {
    if (sessionId) loadSession(sessionId);
  }, []); // eslint-disable-line

  useEffect(() => {
    inputRef.current?.focus();
  }, [showHistory]);

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
        if (data.sessionId) {
          setSessionId(data.sessionId);
          sessionStorage.setItem('agent_sessionId', data.sessionId);
        }
        setMessages(prev => [...prev, {
          role: 'assistant', content: data.reply,
          actions: data.actions, suggestions: data.suggestions,
        }]);
      } else {
        setMessages(prev => [...prev, { role: 'assistant', content: `오류: ${data.error || res.statusText}` }]);
      }
    } catch (err) {
      setMessages(prev => [...prev, { role: 'assistant', content: `연결 오류: ${err.message}` }]);
    }
    setLoading(false);
  }

  function handleNewChat() {
    setSessionId(null);
    setMessages([]);
    sessionStorage.removeItem('agent_sessionId');
    setShowHistory(false);
    setTimeout(() => inputRef.current?.focus(), 0);
  }

  async function loadSessions(query) {
    const q = query ? `?q=${encodeURIComponent(query)}` : '';
    try {
      const res = await fetch(`${API}/Agent/sessions${q}`, {
        headers: { 'X-User-Id': user || '' },
      });
      if (res.ok) setSessions(await res.json());
    } catch {}
  }

  function handleShowHistory() {
    setShowHistory(true);
    setSearchQuery('');
    loadSessions();
  }

  function handleSelectSession(sid) {
    setShowHistory(false);
    loadSession(sid);
  }

  function handleSearch(e) {
    const q = e.target.value;
    setSearchQuery(q);
    loadSessions(q);
  }

  function handleKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend(); }
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
          <div className="ac-header-btns">
            <button className="ac-header-btn" onClick={handleShowHistory} title="대화 이력">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm0 12.5A5.5 5.5 0 1 1 8 2.5a5.5 5.5 0 0 1 0 11zM8.5 4H7v4.5l3.5 2 .75-1.23L8.5 7.5V4z"/></svg>
            </button>
            <button className="ac-header-btn" onClick={handleNewChat} title="새 대화">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1v6H2v2h6v6h2V9h6V7H10V1H8z"/></svg>
            </button>
            <button className="ac-close" onClick={onClose}>&times;</button>
          </div>
        </div>

        {showHistory ? (
          <div className="ac-history">
            <div className="ac-search">
              <input className="ac-search-input" value={searchQuery} onChange={handleSearch}
                placeholder="대화 내용 검색..." autoFocus />
            </div>
            <div className="ac-session-list">
              {sessions.length === 0 ? (
                <div className="ac-empty">대화 이력이 없습니다</div>
              ) : sessions.map(s => (
                <div key={s.id} className={`ac-session-item ${s.id === sessionId ? 'ac-session-active' : ''}`}
                  onClick={() => handleSelectSession(s.id)}>
                  <div className="ac-session-title">{s.title}</div>
                  <div className="ac-session-date">{s.updatedAt}</div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <>
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
              <textarea ref={inputRef} className="ac-input" value={input}
                onChange={e => setInput(e.target.value)} onKeyDown={handleKeyDown}
                placeholder="메시지를 입력하세요..." rows={1} />
              <button className="ac-send" onClick={handleSend} disabled={loading || !input.trim()}>전송</button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
