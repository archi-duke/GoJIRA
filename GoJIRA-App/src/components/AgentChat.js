import React, { useState, useRef, useEffect, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
import './AgentChat.css';

const API = config.GOJIRA_API || config.API_BASE;
const STREAM_API = config.AGENT_API || API;

export default function AgentChat({ user, authority, context, onClose, standalone, autoMessage, newSession, interviewPrompt }) {
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [sessionId, setSessionId] = useState(() => {
    if (newSession) { sessionStorage.removeItem('agent_sessionId'); return null; }
    return sessionStorage.getItem('agent_sessionId') || null;
  });
  const autoMessageSent = useRef(false);
  const [showHistory, setShowHistory] = useState(false);
  const [sessions, setSessions] = useState([]);
  const [searchQuery, setSearchQuery] = useState('');
  const messagesEndRef = useRef(null);
  const inputRef = useRef(null);

  // Master 그룹: authority.USR >= 15
  const isMaster = authority && (authority.USR || 0) >= 15;

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

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

  // interviewPrompt: 인터뷰 모드 — Agent가 먼저 질문을 표시
  useEffect(() => {
    if (interviewPrompt && !autoMessageSent.current) {
      autoMessageSent.current = true;
      const prompt = `**${interviewPrompt}** 작성을 위해 다음 질문에 답변해 주시면 됩니다:\n\n` +
        `**과제 배경:** 이 과제를 시작하게 된 배경이나 동기는 무엇인가요?\n\n` +
        `**과제 목적:** 이 과제가 달성하려는 핵심 목표는 무엇인가요?\n\n` +
        `**주요 이해관계자:** 이 과제의 주요 사용자나 이해관계자는 누구인가요?\n\n` +
        `**핵심 요구사항:** 반드시 충족해야 할 주요 요구사항이 있다면 알려주세요.\n\n` +
        `**제약사항:** 기술적 또는 비즈니스적 제약사항이 있나요? (없으면 '없음')\n\n` +
        `각 질문에 대한 답변을 주시면, 이를 바탕으로 문서를 생성하겠습니다.`;
      setMessages([{ role: 'assistant', content: prompt }]);
    }
  }, [interviewPrompt]); // eslint-disable-line

  // autoMessage: 외부에서 전달된 메시지 자동 전송
  useEffect(() => {
    if (autoMessage && !autoMessageSent.current) {
      autoMessageSent.current = true;
      setTimeout(() => {
        setInput(autoMessage);
        setTimeout(() => {
          const btn = document.querySelector('.ac-send');
          if (btn) btn.click();
        }, 100);
      }, 500);
    }
  }, [autoMessage]); // eslint-disable-line

  async function handleSend() {
    const text = input.trim();
    if (!text || loading) return;
    setInput('');
    setMessages(prev => [
      ...prev,
      { role: 'user', content: text },
      { role: 'process', steps: [], open: true },
    ]);
    setLoading(true);

    let contentBuf = '';
    let finalActions = [];

    function addStep(step) {
      setMessages(prev => {
        const idx = prev.findLastIndex(m => m.role === 'process');
        if (idx === -1) return prev;
        const proc = { ...prev[idx], steps: [...prev[idx].steps, step] };
        return [...prev.slice(0, idx), proc, ...prev.slice(idx + 1)];
      });
    }

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
          // thinking은 스피너 상태 유지 용도 (step에 추가하지 않음)
          break;
        case 'reasoning':
          addStep({ type: 'reasoning', data, ts });
          break;
        case 'tool_call':
          addStep({ type: 'tool_call', data, ts });
          break;
        case 'tool_result':
          addStep({ type: 'tool_result', data, ts });
          // ui_navigate 등 _action이 있으면 메인 앱에 전달
          if (data._action === 'navigate' && data._payload) {
            // 같은 origin → BroadcastChannel
            try { const ch = new BroadcastChannel('gojira-navigate'); ch.postMessage(data._payload); ch.close(); } catch {}
            // 다른 origin (Platform-App↔GoJIRA-App) → window.opener.postMessage
            if (window.opener) {
              window.opener.postMessage({ type: 'gojira-navigate', ...data._payload }, '*');
            }
          }
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
            const updated = filtered.map(m =>
              m.role === 'process' && m.steps.length > 0 ? { ...m, open: false } : m
            );
            return [...updated, {
              role: 'assistant',
              content: data.reply || contentBuf,
              actions: finalActions,
            }];
          });
          // applyToEditor: 생성된 문서를 편집기로 전달 후 채팅창 닫기
          if (window.opener && finalActions.length > 0) {
            const docAction = finalActions.find(a => a.result && a.result.applyToEditor);
            if (docAction) {
              window.opener.postMessage({
                type: 'gojira-doc-apply',
                filePath: docAction.result.filePath,
                content: docAction.result.content,
              }, '*');
              setTimeout(() => window.close(), 1500);
            }
          }
          break;
        case 'error':
          addStep({ type: 'error', data, ts });
          break;
        default: break;
      }
    }

    // 인터뷰 모드: 첫 전송 시 인터뷰 질문 + 답변을 묶어서 전송
    let sendText = text;
    const sendContext = interviewPrompt
      ? { ...context, interviewMode: true, targetFile: interviewPrompt }
      : context;
    if (interviewPrompt && messages.length <= 2) {
      // 첫 답변: 인터뷰 질문 컨텍스트를 포함
      sendText = `[${interviewPrompt} 작성을 ���한 인터뷰]\n\n` +
        `다음 질문에 대한 사용자의 답변입니다. 답변이 충분한지 확인하고, 부족하면 추가 질문하세요. ` +
        `모든 항목이 충분하면 projdesc_generate 스킬을 호출하여 문서를 생성하세요.\n\n` +
        `질문 항목: 과제 배경, 과제 목적, 주요 이해관계자, 핵심 요구사항, 제약사항\n\n` +
        `사용자 답변:\n${text}`;
    }

    try {
      const res = await fetch(`${STREAM_API}/Agent/chat/stream`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user || '' },
        body: JSON.stringify({ sessionId, message: sendText, context: sendContext }),
      });

      if (!res.ok || !res.body) {
        const fb = await fetch(`${API}/Agent/chat`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-User-Id': user || '' },
          body: JSON.stringify({ sessionId, message: sendText, context: sendContext }),
        });
        const data = await fb.json();
        if (data.sessionId) { setSessionId(data.sessionId); sessionStorage.setItem('agent_sessionId', data.sessionId); }
        setMessages(prev => [
          ...prev.filter(m => m.role !== 'process' || m.steps.length > 0),
          { role: 'assistant', content: data.reply, actions: data.actions, suggestions: data.suggestions },
        ]);
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
      setMessages(prev => {
        const filtered = prev.filter(m => m.role !== 'process' || m.steps.length > 0);
        return [...filtered, { role: 'assistant', content: `연결 오류: ${err.message}` }];
      });
    }
    setLoading(false);
  }

  function handleNewChat() {
    setSessionId(null); setMessages([]);
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
  function handleSelectSession(sid) { setShowHistory(false); loadSession(sid); }
  function handleSearch(e) { const q = e.target.value; setSearchQuery(q); loadSessions(q); }
  function handleKeyDown(e) { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend(); } }
  function handleSuggestion(text) { setInput(text); setTimeout(() => inputRef.current?.focus(), 0); }

  function toggleProcess(idx) {
    setMessages(prev => prev.map((m, i) => i === idx ? { ...m, open: !m.open } : m));
  }

  // ─── 디버그 상세(detail) 접기 ───
  function toggleDetail(msgIdx, stepIdx) {
    setMessages(prev => prev.map((m, i) => {
      if (i !== msgIdx || m.role !== 'process') return m;
      const steps = m.steps.map((s, j) => j === stepIdx ? { ...s, detailOpen: !s.detailOpen } : s);
      return { ...m, steps };
    }));
  }

  function renderProcess(msg, idx) {
    const steps = msg.steps || [];
    const toolCount = steps.filter(s => s.type === 'tool_result').length;
    const isRunning = loading && !messages.some(m => m._streaming) && idx === messages.findLastIndex(m => m.role === 'process');

    // 일반 사용자: 스피너 + "작업 중..."만 표시
    if (!isMaster) {
      if (isRunning) return (
        <div className="ac-process">
          <div className="ac-proc-header">
            <span className="ac-proc-spinner" />
            <span className="ac-proc-label">작업 중...</span>
          </div>
        </div>
      );
      // 완료 후에도 표시하지 않음
      return null;
    }

    // Master: 전체 디버그 뷰
    return (
      <div className="ac-process">
        <div className="ac-proc-header" onClick={() => toggleProcess(idx)}>
          {isRunning
            ? <span className="ac-proc-spinner" />
            : <span className="ac-proc-arrow">{msg.open ? '\u25BC' : '\u25B6'}</span>
          }
          <span className="ac-proc-label">{isRunning ? '작업 중...' : '작업 과정'}</span>
          {toolCount > 0 && <span className="ac-proc-badge">{toolCount} tools</span>}
        </div>
        {msg.open && (
          <div className="ac-proc-body">
            {steps.map((step, i) => (
              <div key={i} className={`ac-proc-step ac-ps-${step.type}`}>
                {step.type === 'reasoning' && (
                  <div className={`ac-ps-line ac-ps-reasoning ac-ps-${step.data.step || 'thought'}`}>
                    <span className={`ac-ps-tag ${
                      step.data.step === 'action' ? 'ac-tag-action' :
                      step.data.step === 'observation' ? 'ac-tag-obs' :
                      step.data.step === 'answer' ? 'ac-tag-answer' :
                      'ac-tag-reason'
                    }`}>
                      {{ thought: 'Thought', action: 'Action', observation: 'Observation', answer: 'Answer' }[step.data.step] || 'Thought'}
                    </span>
                    <span className="ac-ps-text">{step.data.content}</span>
                  </div>
                )}
                {step.type === 'tool_call' && (
                  <div className="ac-ps-line">
                    <span className="ac-ps-tag ac-tag-call">CALL</span>
                    <span className="ac-ps-text">
                      <strong>{step.data.tool}</strong>
                      {Object.keys(step.data.params || {}).length > 0 && (
                        <span className="ac-ps-params">
                          ({Object.entries(step.data.params).map(([k,v]) => `${k}=${v}`).join(', ')})
                        </span>
                      )}
                    </span>
                    <span className="ac-ps-ts">{step.ts}</span>
                  </div>
                )}
                {step.type === 'tool_result' && (
                  <div className="ac-ps-result-wrap">
                    <div className="ac-ps-line" onClick={() => toggleDetail(idx, i)} style={{cursor:'pointer'}}>
                      <span className={`ac-ps-tag ${step.data.status === 'success' ? 'ac-tag-ok' : 'ac-tag-err'}`}>
                        {step.data.status === 'success' ? 'OK' : 'ERR'}
                      </span>
                      <span className="ac-ps-text">
                        {step.data.tool} &rarr; {step.data.summary}
                      </span>
                      <span className="ac-ps-dur">{step.data.duration_ms}ms</span>
                      {step.data.detail && (
                        <span className="ac-ps-expand">{step.detailOpen ? '\u25BC' : '...'}</span>
                      )}
                    </div>
                    {step.detailOpen && step.data.detail && (
                      <pre className="ac-ps-detail">{step.data.detail}</pre>
                    )}
                  </div>
                )}
                {step.type === 'error' && (
                  <div className="ac-ps-line">
                    <span className="ac-ps-tag ac-tag-err">ERR</span>
                    <span className="ac-ps-text">{step.data.message}</span>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className={standalone ? "ac-standalone" : "ac-panel"}>
        <div className="ac-header">
          <span className="ac-title">GoJIRA Agent</span>
          <div className="ac-header-btns">
            <button className="ac-header-btn" onClick={handleShowHistory} title="대화 이력">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm0 12.5A5.5 5.5 0 1 1 8 2.5a5.5 5.5 0 0 1 0 11zM8.5 4H7v4.5l3.5 2 .75-1.23L8.5 7.5V4z"/></svg>
            </button>
            <button className="ac-header-btn" onClick={handleNewChat} title="새 대화">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1v6H2v2h6v6h2V9h6V7H10V1H8z"/></svg>
            </button>
            {!standalone && <button className="ac-close" onClick={onClose}>&times;</button>}
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
              {messages.map((msg, i) => {
                if (msg.role === 'process') {
                  const rendered = renderProcess(msg, i);
                  return rendered ? <div key={i} className="ac-msg ac-msg-process">{rendered}</div> : null;
                }
                return (
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
                );
              })}
              {loading && !messages.some(m => m._streaming) && !messages.some(m => m.role === 'process') && (
                <div className="ac-msg ac-msg-assistant">
                  <div className="ac-msg-content ac-typing">생각하는 중...</div>
                </div>
              )}
              <div ref={messagesEndRef} />
            </div>
            <div className="ac-input-resize"
              onMouseDown={e => {
                e.preventDefault();
                const area = e.target.nextElementSibling;
                const ta = area?.querySelector('textarea');
                if (!ta) return;
                const startY = e.clientY;
                const startH = ta.offsetHeight;
                const onMove = ev => { ta.style.height = Math.max(32, Math.min(300, startH - (ev.clientY - startY))) + 'px'; };
                const onUp = () => { document.removeEventListener('mousemove', onMove); document.removeEventListener('mouseup', onUp); };
                document.addEventListener('mousemove', onMove);
                document.addEventListener('mouseup', onUp);
              }}
            />
            <div className="ac-input-area">
              <textarea ref={inputRef} className="ac-input" value={input}
                onChange={e => {
                  setInput(e.target.value);
                  e.target.style.height = 'auto';
                  e.target.style.height = Math.min(e.target.scrollHeight, 300) + 'px';
                }}
                onKeyDown={handleKeyDown}
                placeholder="메시지를 입력하세요..." rows={1} />
              <button className="ac-send" onClick={handleSend} disabled={loading || !input.trim()}>전송</button>
            </div>
          </>
        )}
    </div>
  );
}
