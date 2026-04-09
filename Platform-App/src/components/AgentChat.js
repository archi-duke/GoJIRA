import React, { useState, useRef, useEffect, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
import './AgentChat.css';

const API = config.GOJIRA_API || config.API_BASE;
// Python Agent Service 스트리밍 엔드포인트
const STREAM_API = config.AGENT_API || API;

export default function AgentChat({ user, context, onClose }) {
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [sessionId, setSessionId] = useState(() => sessionStorage.getItem('agent_sessionId') || null);
  const [showHistory, setShowHistory] = useState(false);
  const [sessions, setSessions] = useState([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [scratchpad, setScratchpad] = useState([]);
  const [scratchOpen, setScratchOpen] = useState(true);
  const messagesEndRef = useRef(null);
  const inputRef = useRef(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, scratchpad]);

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
    setScratchpad([]);
    setScratchOpen(true);

    let contentBuf = '';
    let finalActions = [];

    function handleSSE(type, data) {
      const ts = new Date().toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
      switch (type) {
        case 'session':
          if (data.sessionId) {
            setSessionId(data.sessionId);
            sessionStorage.setItem('agent_sessionId', data.sessionId);
          }
          break;
        case 'thinking':
          setScratchpad(prev => [...prev, { type: 'thinking', data, ts }]);
          break;
        case 'tool_call':
          setScratchpad(prev => [...prev, { type: 'tool_call', data, ts }]);
          break;
        case 'tool_result':
          setScratchpad(prev => [...prev, { type: 'tool_result', data, ts }]);
          break;
        case 'content':
          contentBuf += data.text;
          setMessages(prev => {
            const last = prev[prev.length - 1];
            if (last && last._streaming) {
              return [...prev.slice(0, -1), { ...last, content: contentBuf }];
            }
            return [...prev, { role: 'assistant', content: contentBuf, _streaming: true }];
          });
          break;
        case 'done':
          finalActions = data.actions || [];
          setMessages(prev => {
            const filtered = prev.filter(m => !m._streaming);
            return [...filtered, {
              role: 'assistant',
              content: data.reply || contentBuf,
              actions: finalActions,
            }];
          });
          setScratchpad(prev => [...prev, { type: 'done', data: { message: '완료' }, ts }]);
          break;
        case 'error':
          setScratchpad(prev => [...prev, { type: 'error', data, ts }]);
          break;
        default: break;
      }
    }

    try {
      const res = await fetch(`${STREAM_API}/Agent/chat/stream`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user || '' },
        body: JSON.stringify({ sessionId, message: text, context }),
      });

      if (!res.ok || !res.body) {
        // fallback: 기존 방식
        const fb = await fetch(`${API}/Agent/chat`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-User-Id': user || '' },
          body: JSON.stringify({ sessionId, message: text, context }),
        });
        const data = await fb.json();
        if (data.sessionId) { setSessionId(data.sessionId); sessionStorage.setItem('agent_sessionId', data.sessionId); }
        setMessages(prev => [...prev, { role: 'assistant', content: data.reply, actions: data.actions, suggestions: data.suggestions }]);
        setLoading(false);
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';
        let eventType = '';
        for (const line of lines) {
          if (line.startsWith('event: ')) { eventType = line.slice(7).trim(); }
          else if (line.startsWith('data: ') && eventType) {
            try { handleSSE(eventType, JSON.parse(line.slice(6))); } catch {}
            eventType = '';
          }
        }
      }
    } catch (err) {
      setMessages(prev => [...prev, { role: 'assistant', content: `연결 오류: ${err.message}` }]);
    }
    setLoading(false);
  }

  function handleNewChat() {
    setSessionId(null); setMessages([]); setScratchpad([]);
    sessionStorage.removeItem('agent_sessionId');
    setShowHistory(false);
    setTimeout(() => inputRef.current?.focus(), 0);
  }

  async function loadSessions(query) {
    const q = query ? `?q=${encodeURIComponent(query)}` : '';
    try {
      const res = await fetch(`${API}/Agent/sessions${q}`, { headers: { 'X-User-Id': user || '' } });
      if (res.ok) setSessions(await res.json());
    } catch {}
  }

  function handleShowHistory() { setShowHistory(true); setSearchQuery(''); loadSessions(); }
  function handleSelectSession(sid) { setShowHistory(false); setScratchpad([]); loadSession(sid); }
  function handleSearch(e) { const q = e.target.value; setSearchQuery(q); loadSessions(q); }
  function handleKeyDown(e) { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend(); } }
  function handleSuggestion(text) { setInput(text); setTimeout(() => inputRef.current?.focus(), 0); }

  function renderScratchpad() {
    if (scratchpad.length === 0 && !loading) return null;
    return (
      <div className="ac-scratch">
        <div className="ac-scratch-header" onClick={() => setScratchOpen(!scratchOpen)}>
          <span className="ac-scratch-arrow">{scratchOpen ? '\u25BC' : '\u25B6'}</span>
          <span className="ac-scratch-title">Agent 동작 과정</span>
          <span className="ac-scratch-count">{scratchpad.length}</span>
        </div>
        {scratchOpen && (
          <div className="ac-scratch-body">
            {scratchpad.map((item, i) => (
              <div key={i} className={`ac-scratch-item ac-si-${item.type}`}>
                <span className="ac-scratch-ts">{item.ts}</span>
                {item.type === 'thinking' && <span className="ac-si-text">{item.data.message}</span>}
                {item.type === 'tool_call' && (
                  <span className="ac-si-text">
                    <strong>{item.data.tool}</strong>
                    <span className="ac-si-params">({Object.entries(item.data.params || {}).map(([k,v]) => `${k}=${v}`).join(', ')})</span>
                  </span>
                )}
                {item.type === 'tool_result' && (
                  <span className="ac-si-text">
                    <span className={item.data.status === 'success' ? 'ac-si-ok' : 'ac-si-err'}>{item.data.status === 'success' ? 'OK' : 'ERR'}</span>
                    {' '}{item.data.tool} &rarr; {item.data.summary}
                    <span className="ac-si-dur">{item.data.duration_ms}ms</span>
                  </span>
                )}
                {item.type === 'done' && <span className="ac-si-text ac-si-done">{item.data.message}</span>}
                {item.type === 'error' && <span className="ac-si-text ac-si-err">{item.data.message}</span>}
              </div>
            ))}
            {loading && scratchpad.length === 0 && (
              <div className="ac-scratch-item ac-si-thinking"><span className="ac-si-text">처리 시작...</span></div>
            )}
          </div>
        )}
      </div>
    );
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
                <div key={s.sessionId || s.id} className={`ac-session-item ${(s.sessionId || s.id) === sessionId ? 'ac-session-active' : ''}`}
                  onClick={() => handleSelectSession(s.sessionId || s.id)}>
                  <div className="ac-session-title">{s.title}</div>
                  <div className="ac-session-date">{s.updatedAt}</div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <>
            <div className="ac-messages">
              {messages.length === 0 && !loading && (
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
                          <span className="ac-action-status">{a.error ? '실패' : '완료'}</span>
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
              {loading && !messages.some(m => m._streaming) && (
                <div className="ac-msg ac-msg-assistant">
                  <div className="ac-msg-content ac-typing">생각하는 중...</div>
                </div>
              )}
              <div ref={messagesEndRef} />
            </div>
            {renderScratchpad()}
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
