import React, { useState, useRef, useEffect, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
import './AgentChat.css';

const API = config.GOJIRA_API || config.API_BASE;
const STREAM_API = config.AGENT_API || API;

export default function AgentChat({ user, authority, context, onClose, standalone, autoMessage, newSession, interviewPrompt, continueTo }) {
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
  const abortRef = useRef(null);
  const [showProcess, setShowProcess] = useState(true);

  async function handleCancel() {
    if (abortRef.current) {
      try { abortRef.current.abort(); } catch(_){}
    }
    if (sessionId) {
      try {
        await fetch(`${STREAM_API}/Agent/session/${sessionId}/cancel`, {
          method: 'POST',
          headers: { 'X-User-Id': user || '' },
        });
      } catch(_){}
    }
    setLoading(false);
  }

  // LLM 모델 선택 상태
  const [llmActive, setLlmActive] = useState('');
  const [llmModel, setLlmModel] = useState('');
  const [llmProviders, setLlmProviders] = useState([]);
  const [llmMenuOpen, setLlmMenuOpen] = useState(false);
  const [llmExpanded, setLlmExpanded] = useState(null);   // 현재 확장된 프로바이더 이름
  const [llmModels, setLlmModels] = useState({});          // {provider: [{id, ownedBy}]}
  const [llmLoading, setLlmLoading] = useState({});        // {provider: bool}

  // Master 그룹: authority.USR >= 15
  const isMaster = authority && (authority.USR || 0) >= 15;

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: loading ? 'auto' : 'smooth' });
  }, [messages, loading]);

  const loadSession = useCallback(async (sid) => {
    if (!sid) return;
    try {
      const res = await fetch(`${STREAM_API}/Agent/session/${sid}`, {
        headers: { 'X-User-Id': user || '' },
      });
      if (res.ok) {
        const data = await res.json();
        const msgs = (data.messages || []).map(m => {
          const msg = m.role ? m : Object.fromEntries(Object.entries(m));
          if (msg.role === 'process') {
            return { role: 'process', steps: msg.steps || [], open: false };
          }
          return { role: msg.role, content: msg.content, actions: msg.actions, suggestions: msg.suggestions };
        }).filter(m => m.role && (m.content || m.role === 'process'));
        setMessages(msgs);
        setSessionId(sid);
        sessionStorage.setItem('agent_sessionId', sid);
        // OpenCode 세션이 아직 진행 중이면 자동으로 EventSource 연결
        if (data.active) {
          connectEventSource(sid);
        }
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

  // LLM 프로바이더 목록 + 활성 조회 (초기 1회)
  useEffect(() => {
    (async () => {
      try {
        const res = await fetch(`${STREAM_API}/Agent/llm`);
        if (!res.ok) return;
        const d = await res.json();
        setLlmActive(d.active || '');
        setLlmModel(d.activeModel || '');
        setLlmProviders(d.providers || []);
      } catch {}
    })();
  }, []);

  async function handleSwitchLlm(providerName, modelId = null) {
    setLlmMenuOpen(false);
    setLlmExpanded(null);
    if (providerName === llmActive && (!modelId || modelId === llmModel)) return;
    try {
      const body = { active: providerName };
      if (modelId) body.model = modelId;
      const res = await fetch(`${STREAM_API}/Agent/llm`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      const d = await res.json();
      if (d.ok) {
        setLlmActive(d.active);
        setLlmModel(d.activeModel);
        // providers 목록도 해당 provider 의 model 을 업데이트
        setLlmProviders(prev => prev.map(p => p.name === d.active ? { ...p, model: d.activeModel } : p));
      } else {
        alert('LLM 전환 실패: ' + (d.error || '알 수 없음'));
      }
    } catch (e) {
      alert('LLM 전환 실패: ' + e.message);
    }
  }

  async function handleExpandProvider(providerName, forceRefresh = false) {
    if (llmExpanded === providerName && !forceRefresh) {
      setLlmExpanded(null);
      return;
    }
    setLlmExpanded(providerName);
    if (llmModels[providerName] && !forceRefresh) return;
    setLlmLoading(prev => ({ ...prev, [providerName]: true }));
    try {
      const q = forceRefresh ? '&refresh=true' : '';
      const res = await fetch(`${STREAM_API}/Agent/llm/models?provider=${encodeURIComponent(providerName)}${q}`);
      const d = await res.json();
      if (d.error) {
        alert(`${providerName} 모델 조회 실패: ${d.error}`);
        setLlmLoading(prev => ({ ...prev, [providerName]: false }));
        return;
      }
      setLlmModels(prev => ({ ...prev, [providerName]: d.models || [] }));
    } catch (e) {
      alert('모델 조회 실패: ' + e.message);
    } finally {
      setLlmLoading(prev => ({ ...prev, [providerName]: false }));
    }
  }

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

  // 외부(opener=DocEditor)에서 후속 작업 지시 — 인터뷰 저장 후 워크플로우 이어가기
  useEffect(() => {
    function onContinue(e) {
      if (e.data?.type !== 'gojira-continue') return;
      const { target, projectCode } = e.data;
      if (!target) return;
      const TARGET_LABEL = {
        'business/use-cases.md': '유즈케이스 문서',
        'business/requirements.md': '요구사양서',
      };
      const label = TARGET_LABEL[target] || target;
      const msg = `이어서 ${projectCode || ''} 과제의 ${label}를 작성해줘.`;
      setTimeout(() => {
        setInput(msg);
        setTimeout(() => {
          const btn = document.querySelector('.ac-send');
          if (btn) btn.click();
        }, 100);
      }, 300);
    }
    window.addEventListener('message', onContinue);
    return () => window.removeEventListener('message', onContinue);
  }, []);

  function connectEventSource(sid) {
    let contentBuf = '';
    let finalActions = [];

    function addStep(step) {
      setMessages(prev => {
        const idx = prev.findLastIndex(m => m.role === 'process');
        if (idx === -1) {
          return [...prev, { role: 'process', steps: [step], open: false }];
        }
        const proc = { ...prev[idx], steps: [...prev[idx].steps, step] };
        return [...prev.slice(0, idx), proc, ...prev.slice(idx + 1)];
      });
    }

    function handleEvt(type, data) {
      const ts = new Date().toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
      switch (type) {
        case 'session': break;
        case 'thinking': break;
        case 'reasoning': addStep({ type: 'reasoning', data, ts }); break;
        case 'tool_call': addStep({ type: 'tool_call', data, ts }); break;
        case 'tool_result':
          addStep({ type: 'tool_result', data, ts });
          if (contentBuf) {
            setMessages(prev => prev.map(m => m._streaming ? { ...m, _streaming: false, _intermediate: true } : m));
            contentBuf = '';
          }
          break;
        case 'content':
          contentBuf += data.text;
          setMessages(prev => {
            const idx = prev.findLastIndex(m => m.role === 'process');
            let next = [...prev];
            if (idx >= 0) {
              const proc = next[idx];
              const steps = [...proc.steps];
              const lastStep = steps[steps.length - 1];
              if (lastStep && lastStep.type === 'content') {
                steps[steps.length - 1] = { ...lastStep, data: { text: contentBuf } };
              } else {
                steps.push({ type: 'content', data: { text: contentBuf }, ts });
              }
              next = [...next.slice(0, idx), { ...proc, steps }, ...next.slice(idx + 1)];
            }
            const last = next[next.length - 1];
            if (last && last._streaming) {
              next = [...next.slice(0, -1), { ...last, content: contentBuf }];
            } else {
              next = [...next, { role: 'assistant', content: contentBuf, _streaming: true }];
            }
            return next;
          });
          break;
        case 'done':
          finalActions = data.actions || [];
          setMessages(prev => {
            let next = prev.filter(m => !m._streaming);
            next = next.filter(m => m.role !== 'thought');
            const reply = data.reply || contentBuf;
            if (reply) {
              next = next.filter(m => !(m._intermediate && m.content === reply));
            }
            return [...next, { role: 'assistant', content: reply, actions: finalActions }];
          });
          break;
        case 'error':
          addStep({ type: 'error', data, ts });
          break;
        default: break;
      }
    }

    setLoading(true);
    return new Promise((resolve) => {
      const evtUrl = `${STREAM_API}/Agent/session/${sid}/events`;
      const es = new EventSource(evtUrl);
      abortRef.current = { abort: () => { es.close(); setLoading(false); resolve(); } };

      function onEvent(e) {
        try { handleEvt(e.type, JSON.parse(e.data)); } catch {}
        if (e.type === 'done') { es.close(); setLoading(false); resolve(); }
      }
      ['session','thinking','reasoning','tool_call','tool_result','content','progress','done','error'].forEach(
        t => es.addEventListener(t, onEvent)
      );
      es.onerror = () => {
        if (es.readyState === EventSource.CLOSED) { setLoading(false); resolve(); }
      };
    });
  }

  async function handleSend() {
    const text = input.trim();
    if (!text || loading) return;
    setInput('');
    setMessages(prev => [
      ...prev,
      { role: 'user', content: text },
      { role: 'process', steps: [], open: false },
    ]);

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
      // 1) 메시지 전송 (즉시 반환)
      const sendRes = await fetch(`${STREAM_API}/Agent/chat/send`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user || '' },
        body: JSON.stringify({ sessionId, message: sendText, context: sendContext }),
      });
      if (!sendRes.ok) throw new Error(`전송 실패: ${sendRes.status}`);
      const sendData = await sendRes.json();
      const activeSid = sendData.sessionId;
      if (activeSid) { setSessionId(activeSid); sessionStorage.setItem('agent_sessionId', activeSid); }

      // 2) EventSource 로 이벤트 수신 (재사용 가능한 connectEventSource)
      await connectEventSource(activeSid);
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
      const res = await fetch(`${STREAM_API}/Agent/sessions${q}`, { headers: { 'X-User-Id': user || '' } });
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

    // 일반 사용자: 스피너 + 진행 메시지(progress 단계만) 표시
    if (!isMaster) {
      const progressSteps = steps.filter(s => s.type === 'progress');
      if (!isRunning && progressSteps.length === 0) return null;
      return (
        <div className="ac-process">
          <div className="ac-proc-header">
            {isRunning && <span className="ac-proc-spinner" />}
            <span className="ac-proc-label">{isRunning ? '작업 중...' : '완료'}</span>
          </div>
          {progressSteps.length > 0 && (
            <div className="ac-proc-body">
              {progressSteps.map((s, i) => (
                <div key={i} className="ac-ps-line">
                  <span className="ac-ps-text">• {s.data.message}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      );
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
                {step.type === 'progress' && (
                  <div className="ac-ps-line">
                    <span className="ac-ps-tag ac-tag-action">PROG</span>
                    <span className="ac-ps-text">
                      <strong>{step.data.stage}</strong> — {step.data.message}
                    </span>
                    <span className="ac-ps-ts">{step.ts}</span>
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
          {llmModel && (
            <div className="ac-llm-picker">
              <button className="ac-llm-btn" onClick={() => setLlmMenuOpen(o => !o)} title="LLM 모델 전환">
                {llmActive} · {llmModel}
                <span className="ac-llm-caret">▾</span>
              </button>
              {llmMenuOpen && (
                <div className="ac-llm-menu">
                  {llmProviders.map(p => (
                    <div key={p.name} className="ac-llm-group">
                      <div
                        className={`ac-llm-item ${p.name === llmActive ? 'ac-llm-item-active' : ''} ${!p.apiKeyPresent ? 'ac-llm-item-disabled' : ''}`}
                        title={!p.apiKeyPresent ? `${p.apiKeyEnv} 환경변수 미설정` : p.endpoint}
                      >
                        <div className="ac-llm-item-main" onClick={() => p.apiKeyPresent && handleSwitchLlm(p.name)}>
                          <div className="ac-llm-item-name">{p.name}</div>
                          <div className="ac-llm-item-model">{p.model}</div>
                          {!p.apiKeyPresent && <div className="ac-llm-item-warn">키 없음</div>}
                        </div>
                        {p.apiKeyPresent && (
                          <button
                            className="ac-llm-expand"
                            onClick={(e) => { e.stopPropagation(); handleExpandProvider(p.name); }}
                            title="모델 목록"
                          >
                            {llmExpanded === p.name ? '▾' : '▸'}
                          </button>
                        )}
                      </div>
                      {llmExpanded === p.name && (
                        <div className="ac-llm-sub">
                          {llmLoading[p.name] ? (
                            <div className="ac-llm-sub-loading">모델 조회 중...</div>
                          ) : (llmModels[p.name] || []).length === 0 ? (
                            <div className="ac-llm-sub-loading">모델 없음</div>
                          ) : (
                            <>
                              <div className="ac-llm-sub-header">
                                <span>사용 가능 모델</span>
                                <button className="ac-llm-refresh" onClick={(e) => { e.stopPropagation(); handleExpandProvider(p.name, true); }} title="새로고침">⟳</button>
                              </div>
                              {(llmModels[p.name] || []).map(m => (
                                <div
                                  key={m.id}
                                  className={`ac-llm-sub-item ${(p.name === llmActive && m.id === llmModel) ? 'ac-llm-sub-item-active' : ''}`}
                                  onClick={() => handleSwitchLlm(p.name, m.id)}
                                >
                                  {m.id}
                                </div>
                              ))}
                            </>
                          )}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
          <div className="ac-header-btns">
            {isMaster && (
              <button className="ac-header-btn" onClick={async () => {
                if (!sessionId) return;
                try {
                  await fetch(`${STREAM_API}/Agent/session/${sessionId}/terminal`, { method: 'POST' });
                } catch {}
              }} title="터미널에서 OpenCode 세션 보기" style={{opacity: sessionId ? 1 : 0.3}}>
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="2" y="4" width="20" height="16" rx="2" />
                  <polyline points="7 9 10 12 7 15" />
                  <line x1="13" y1="15" x2="17" y2="15" />
                </svg>
              </button>
            )}
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
            {sessions.length > 0 && (
              <div style={{ textAlign: 'right', padding: '0 8px 6px' }}>
                <button style={{ background: 'none', border: 'none', color: '#c33', cursor: 'pointer', fontSize: '12px' }}
                  onClick={async () => {
                    if (!window.confirm('모든 대화 이력을 삭제하시겠습니까?')) return;
                    await fetch(`${STREAM_API}/Agent/sessions`, { method: 'DELETE', headers: { 'X-User-Id': user || '' } });
                    setSessions([]); setSessionId(null); sessionStorage.removeItem('agent_sessionId'); setMessages([]);
                  }}>전체 삭제</button>
              </div>
            )}
            <div className="ac-session-list">
              {sessions.length === 0 ? (
                <div className="ac-empty">대화 이력이 없습니다</div>
              ) : sessions.map(s => (
                <div key={s.sessionId || s.id} className={`ac-session-item ${(s.sessionId || s.id) === sessionId ? 'ac-session-active' : ''}`}
                  onClick={() => handleSelectSession(s.sessionId || s.id)}>
                  <div className="ac-session-title">{s.title}</div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div className="ac-session-date">{s.updatedAt}</div>
                    <button style={{ background: 'none', border: 'none', color: '#999', cursor: 'pointer', fontSize: '11px' }}
                      onClick={async (e) => {
                        e.stopPropagation();
                        const sid = s.sessionId || s.id;
                        await fetch(`${STREAM_API}/Agent/session/${sid}`, { method: 'DELETE', headers: { 'X-User-Id': user || '' } });
                        setSessions(prev => prev.filter(x => (x.sessionId || x.id) !== sid));
                        if (sid === sessionId) { setSessionId(null); sessionStorage.removeItem('agent_sessionId'); setMessages([]); }
                      }}>삭제</button>
                  </div>
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
                  if (!showProcess) {
                    // showProcess OFF: content step만 말풍선으로 렌더링
                    const contentSteps = (msg.steps || []).filter(s => s.type === 'content');
                    if (contentSteps.length === 0) return null;
                    return contentSteps.map((step, j) => (
                      <div key={`${i}-c${j}`} className="ac-msg ac-msg-assistant">
                        <div className="ac-msg-content">
                          <ReactMarkdown remarkPlugins={[remarkGfm]}>{step.data.text || ''}</ReactMarkdown>
                        </div>
                      </div>
                    ));
                  }
                  const steps = msg.steps || [];
                  if (steps.length === 0 && !(loading && i === messages.findLastIndex(m => m.role === 'process'))) return null;
                  const isRunning = loading && i === messages.findLastIndex(m => m.role === 'process');
                  return (
                    <div key={i} className="ac-msg ac-msg-process">
                      <div className="ac-proc-body">
                        {steps.map((step, j) => (
                          <div key={j} className={`ac-proc-step ac-ps-${step.type}`}>
                            {step.type === 'reasoning' && (
                              <div className={`ac-ps-line ac-ps-reasoning ac-ps-${step.data.step || 'thought'}`}>
                                <span className="ac-ps-tag ac-tag-reason">Thought</span>
                                <span className="ac-ps-text">{step.data.content}</span>
                              </div>
                            )}
                            {step.type === 'tool_call' && (
                              <div className="ac-ps-line">
                                <span className="ac-ps-tag ac-tag-call">CALL</span>
                                <span className="ac-ps-text"><strong>{step.data.tool}</strong>
                                  {Object.keys(step.data.params || {}).length > 0 && (
                                    <span className="ac-ps-params"> ({Object.entries(step.data.params).map(([k,v]) => `${k}=${v}`).join(', ')})</span>
                                  )}
                                </span>
                                <span className="ac-ps-ts">{step.ts}</span>
                              </div>
                            )}
                            {step.type === 'tool_result' && (
                              <div className="ac-ps-result-wrap">
                                <div className="ac-ps-line" onClick={() => toggleDetail(i, j)} style={{cursor:'pointer'}}>
                                  <span className={`ac-ps-tag ${step.data.status === 'success' ? 'ac-tag-ok' : 'ac-tag-err'}`}>
                                    {step.data.status === 'success' ? 'OK' : 'ERR'}
                                  </span>
                                  <span className="ac-ps-text">{step.data.tool} &rarr; {step.data.summary}</span>
                                  <span className="ac-ps-dur">{step.data.duration_ms}ms</span>
                                  {step.data.detail && <span className="ac-ps-expand">{step.detailOpen ? '\u25BC' : '...'}</span>}
                                </div>
                                {step.detailOpen && step.data.detail && <pre className="ac-ps-detail">{step.data.detail}</pre>}
                              </div>
                            )}
                            {step.type === 'error' && (
                              <div className="ac-ps-line"><span className="ac-ps-tag ac-tag-err">ERR</span><span className="ac-ps-text">{step.data.message}</span></div>
                            )}
                            {step.type === 'content' && (
                              <div className="ac-msg ac-msg-assistant" style={{margin: '8px 0'}}>
                                <div className="ac-msg-content">
                                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{step.data.text || ''}</ReactMarkdown>
                                </div>
                              </div>
                            )}
                          </div>
                        ))}
                        {isRunning && <div className="ac-ps-line"><span className="ac-proc-spinner" /> <span className="ac-ps-text">작업 중...</span></div>}
                      </div>
                    </div>
                  );
                }
                if (msg.role === 'thought') return null;
                if (msg.role === 'assistant' && msg._intermediate) return null;
                if (msg.role === 'assistant' && showProcess && messages.some(m => m.role === 'process')) return null;
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
              <button
                onClick={() => setShowProcess(p => !p)}
                style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  fontSize: '11px', color: showProcess ? '#e67e22' : '#999',
                  padding: '2px 6px', marginRight: '4px', flexShrink: 0
                }}
                title={showProcess ? '추론 과정 감추기' : '추론 과정 보기'}
              >{showProcess ? '◉' : '○'}</button>
              <textarea ref={inputRef} className="ac-input" value={input}
                onChange={e => {
                  setInput(e.target.value);
                  e.target.style.height = 'auto';
                  e.target.style.height = Math.min(e.target.scrollHeight, 300) + 'px';
                }}
                onKeyDown={handleKeyDown}
                placeholder="메시지를 입력하세요..." rows={1} />
              {loading ? (
                <button className="ac-send" onClick={handleCancel} style={{ background: '#c33', color: '#fff' }}>중지</button>
              ) : (
                <button className="ac-send" onClick={handleSend} disabled={!input.trim()}>전송</button>
              )}
            </div>
          </>
        )}
    </div>
  );
}
