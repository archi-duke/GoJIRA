import React, { useState, useRef, useEffect, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
import './AgentChat.css';

const API = config.GOJIRA_API || config.API_BASE;
const STREAM_API = config.AGENT_API || API;

// ── <thinking> 태그 파서: 스트리밍 중 안전하게 thinking / response 분리 ──
const OPEN_TAGS = ['<thinking>', '<think>'];
const CLOSE_TAGS = ['</thinking>', '</think>'];

function partialTagLen(s, tags) {
  let max = 0;
  for (const tag of tags) {
    const maxLen = Math.min(tag.length - 1, s.length);
    for (let len = maxLen; len > max; len--) {
      if (s.slice(-len) === tag.slice(0, len)) { max = len; break; }
    }
  }
  return max;
}

function splitContent(buf, startInThinking) {
  const segs = [];
  let cursor = 0;
  let inThinking = startInThinking;
  while (cursor < buf.length) {
    const tags = inThinking ? CLOSE_TAGS : OPEN_TAGS;
    let nextIdx = -1, matched = null;
    for (const tag of tags) {
      const idx = buf.indexOf(tag, cursor);
      if (idx !== -1 && (nextIdx === -1 || idx < nextIdx)) { nextIdx = idx; matched = tag; }
    }
    if (nextIdx === -1) {
      const rest = buf.slice(cursor);
      const partial = partialTagLen(rest, tags);
      const emit = rest.length - partial;
      if (emit > 0) segs.push({ type: inThinking ? 'thinking' : 'response', text: rest.slice(0, emit) });
      return { segments: segs, remainder: rest.slice(emit), inThinking };
    }
    if (nextIdx > cursor) segs.push({ type: inThinking ? 'thinking' : 'response', text: buf.slice(cursor, nextIdx) });
    cursor = nextIdx + matched.length;
    inThinking = !inThinking;
  }
  return { segments: segs, remainder: '', inThinking };
}

function formatParams(params) {
  return Object.entries(params || {}).map(([k, v]) => {
    const str = typeof v === 'string' ? v : JSON.stringify(v);
    const short = str.length > 40 ? str.slice(0, 40) + '…' : str;
    return `${k}=${short}`;
  }).join(', ');
}

function mergeSegments(existing, incoming) {
  const out = [...existing];
  for (const s of incoming) {
    const last = out[out.length - 1];
    if (last && last.type === s.type && (s.type === 'thinking' || s.type === 'response')) {
      out[out.length - 1] = { ...last, text: (last.text || '') + (s.text || '') };
    } else {
      out.push({ ...s });
    }
  }
  return out;
}

function stepsToSegments(steps) {
  let segs = [];
  let inThinking = false;
  let pendingContent = '';
  for (const s of steps || []) {
    if (s.type === 'reasoning') {
      const txt = s.data?.content || '';
      if (txt) segs = mergeSegments(segs, [{ type: 'thinking', text: txt.endsWith('\n') ? txt : txt + '\n' }]);
    } else if (s.type === 'tool_call') {
      segs.push({ type: 'action', tool: s.data?.tool || '?', params: s.data?.params || {}, status: 'running' });
    } else if (s.type === 'tool_result') {
      let i = segs.length - 1;
      while (i >= 0 && !(segs[i].type === 'action' && segs[i].status === 'running')) i--;
      const patch = {
        status: s.data?.status === 'success' ? 'ok' : 'err',
        summary: s.data?.summary || '',
        detail: s.data?.detail || '',
      };
      if (i >= 0) segs[i] = { ...segs[i], ...patch };
      else segs.push({ type: 'action', tool: s.data?.tool || '?', params: {}, ...patch });
    } else if (s.type === 'content') {
      const combined = pendingContent + (s.data?.text || '');
      const { segments, remainder, inThinking: next } = splitContent(combined, inThinking);
      pendingContent = remainder;
      inThinking = next;
      segs = mergeSegments(segs, segments);
    } else if (s.type === 'error') {
      segs = mergeSegments(segs, [{ type: 'response', text: `\n[ERROR] ${s.data?.message || ''}\n` }]);
    }
  }
  if (pendingContent) {
    segs = mergeSegments(segs, [{ type: inThinking ? 'thinking' : 'response', text: pendingContent }]);
  }
  return segs;
}

function contentToSegments(content) {
  if (!content) return [];
  const { segments } = splitContent(content, false);
  return mergeSegments([], segments);
}

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
  const [showThinking, setShowThinking] = useState(true);

  async function handleCancel() {
    if (abortRef.current) {
      try { abortRef.current.abort(); } catch(_){}
    }
    // stale _streaming 플래그 클리어 — 다음 턴이 이 메시지에 흘러들지 않게
    setMessages(prev => prev.map(m => m._streaming ? { ...m, _streaming: false } : m));
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
        const raw = data.messages || [];
        const msgs = [];
        for (let i = 0; i < raw.length; i++) {
          const m = raw[i].role ? raw[i] : Object.fromEntries(Object.entries(raw[i]));
          if (m.role === 'process') {
            const segs = stepsToSegments(m.steps || []);
            const next = raw[i + 1];
            // process 다음의 assistant.content 는 process 의 content 스텝과 중복 — 건너뛰되
            // actions/suggestions 만 승계
            if (next && next.role === 'assistant') {
              i++;
              if (segs.length) msgs.push({ role: 'assistant', segments: segs, actions: next.actions, suggestions: next.suggestions });
            } else if (segs.length) {
              msgs.push({ role: 'assistant', segments: segs });
            }
          } else if (m.role === 'assistant') {
            if (!m.content) continue;
            msgs.push({ role: 'assistant', segments: contentToSegments(m.content), actions: m.actions, suggestions: m.suggestions });
          } else if (m.content) {
            msgs.push({ role: m.role, content: m.content });
          }
        }
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
    // 스트리밍 파서 상태 (assistant 메시지 단위)
    let inThinking = false;
    let pendingContent = '';

    function updateStreaming(fn) {
      setMessages(prev => {
        let idx = prev.length - 1;
        while (idx >= 0 && !(prev[idx].role === 'assistant' && prev[idx]._streaming)) idx--;
        if (idx < 0) {
          const msg = { role: 'assistant', _streaming: true, segments: fn([]) };
          return [...prev, msg];
        }
        const msg = prev[idx];
        return [...prev.slice(0, idx), { ...msg, segments: fn(msg.segments || []) }, ...prev.slice(idx + 1)];
      });
    }

    function appendSegments(list) {
      if (!list.length) return;
      updateStreaming(segs => mergeSegments(segs, list));
    }

    function pushAction(action) {
      updateStreaming(segs => [...segs, action]);
    }

    function patchLastRunningAction(patch) {
      updateStreaming(segs => {
        for (let i = segs.length - 1; i >= 0; i--) {
          if (segs[i].type === 'action' && segs[i].status === 'running') {
            return [...segs.slice(0, i), { ...segs[i], ...patch }, ...segs.slice(i + 1)];
          }
        }
        return [...segs, { type: 'action', tool: '?', params: {}, ...patch }];
      });
    }

    function handleContent(delta) {
      const combined = pendingContent + (delta || '');
      const { segments, remainder, inThinking: nextState } = splitContent(combined, inThinking);
      pendingContent = remainder;
      inThinking = nextState;
      appendSegments(segments);
    }

    function handleEvt(type, data) {
      switch (type) {
        case 'session':
        case 'thinking':
          break;
        case 'reasoning':
          if (data.content) appendSegments([{ type: 'thinking', text: data.content }]);
          break;
        case 'tool_call':
          pushAction({ type: 'action', tool: data.tool, params: data.params || {}, status: 'running' });
          break;
        case 'tool_result':
          patchLastRunningAction({
            status: data.status === 'success' ? 'ok' : 'err',
            summary: data.summary || '',
            detail: data.detail || '',
          });
          break;
        case 'content':
          handleContent(data.text);
          break;
        case 'done':
          if (pendingContent) {
            appendSegments([{ type: inThinking ? 'thinking' : 'response', text: pendingContent }]);
            pendingContent = '';
          }
          setMessages(prev => prev.map(m => m._streaming ? { ...m, _streaming: false } : m));
          break;
        case 'error':
          appendSegments([{ type: 'response', text: `\n[ERROR] ${data.message || ''}\n` }]);
          break;
        default:
          break;
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
    if (!text) return;
    // 대화 중이면 기존 스트림 정리 (subscriber 는 chat/send 의 reset_subscriber 가 처리)
    if (loading && abortRef.current) {
      try { abortRef.current.abort(); } catch(_){}
    }
    setInput('');
    // 이전 턴 stale _streaming 플래그 클리어 후 user 메시지 append
    setMessages(prev => [
      ...prev.map(m => m._streaming ? { ...m, _streaming: false } : m),
      { role: 'user', content: text },
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
      setMessages(prev => [...prev, { role: 'assistant', content: `연결 오류: ${err.message}` }]);
    }
    setLoading(false);
  }

  function handleNewChat() {
    setSessionId(null); setMessages([]);
    sessionStorage.removeItem('agent_sessionId');
    setShowHistory(false);
    setTimeout(() => inputRef.current?.focus(), 0);
  }

  async function handleReconnect() {
    if (!sessionId) return;
    if (abortRef.current) {
      try { abortRef.current.abort(); } catch(_){}
    }
    setMessages(prev => prev.map(m => m._streaming ? { ...m, _streaming: false } : m));

    // Mongo 에서 저장 상태 다시 로드 (완료된 턴들 정확히)
    let inProgress = false;
    try {
      const res = await fetch(`${STREAM_API}/Agent/session/${sessionId}`, { headers: { 'X-User-Id': user || '' } });
      if (res.ok) {
        const data = await res.json();
        const raw = data.messages || [];
        const msgs = [];
        for (let i = 0; i < raw.length; i++) {
          const m = raw[i].role ? raw[i] : Object.fromEntries(Object.entries(raw[i]));
          if (m.role === 'process') {
            const segs = stepsToSegments(m.steps || []);
            const next = raw[i + 1];
            if (next && next.role === 'assistant') {
              i++;
              if (segs.length) msgs.push({ role: 'assistant', segments: segs, actions: next.actions, suggestions: next.suggestions });
            } else if (segs.length) {
              msgs.push({ role: 'assistant', segments: segs });
            }
          } else if (m.role === 'assistant') {
            if (!m.content) continue;
            msgs.push({ role: 'assistant', segments: contentToSegments(m.content), actions: m.actions, suggestions: m.suggestions });
          } else if (m.content) {
            msgs.push({ role: m.role, content: m.content });
          }
        }
        setMessages(msgs);
        // 턴 진행 중 판정: Mongo 의 마지막 원본 메시지가 user 면 아직 assistant 가 저장 안 됨 = 진행 중
        const lastRaw = raw[raw.length - 1];
        inProgress = !!(lastRaw && lastRaw.role === 'user');
      }
    } catch(_) {}

    // 진행 중인 턴만 EventSource 재연결 (완료 턴 재생으로 인한 중복 방지)
    if (inProgress) {
      await connectEventSource(sessionId);
    }
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

  function toggleAction(msgIdx, segIdx) {
    setMessages(prev => prev.map((m, i) => {
      if (i !== msgIdx || !m.segments) return m;
      return { ...m, segments: m.segments.map((s, j) => j === segIdx ? { ...s, open: !s.open } : s) };
    }));
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
            <button className="ac-header-btn" onClick={handleReconnect}
              title="세션 재연결 (SSE 끊고 다시 연결)" style={{opacity: sessionId ? 1 : 0.3}}>
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M23 4v6h-6"/>
                <path d="M1 20v-6h6"/>
                <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10"/>
                <path d="M20.49 15a9 9 0 0 1-14.85 3.36L1 14"/>
              </svg>
            </button>
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
                if (msg.role === 'user') {
                  return (
                    <div key={i} className="ac-msg ac-msg-user">
                      <div className="ac-msg-content"><span>{msg.content}</span></div>
                    </div>
                  );
                }
                // assistant
                const segments = msg.segments || (msg.content ? [{ type: 'response', text: msg.content }] : []);
                return (
                  <div key={i} className="ac-msg ac-msg-assistant">
                    {segments.map((seg, j) => {
                      if (seg.type === 'thinking') {
                        if (!showThinking) return null;
                        return (
                          <div key={j} className="ac-seg-thinking">{seg.text}</div>
                        );
                      }
                      if (seg.type === 'action') {
                        if (!showThinking) return null;
                        const statusLabel = seg.status === 'ok' ? 'OK' : seg.status === 'err' ? 'ERR' : '…';
                        const statusCls = seg.status === 'ok' ? 'ac-seg-status-ok' : seg.status === 'err' ? 'ac-seg-status-err' : 'ac-seg-status-run';
                        return (
                          <div key={j} className="ac-seg-action">
                            <div className="ac-seg-action-header" onClick={() => toggleAction(i, j)}>
                              <span className="ac-seg-arrow">{seg.open ? '▼' : '▶'}</span>
                              <code className="ac-seg-tool">{seg.tool}({formatParams(seg.params)})</code>
                              <span className={`ac-seg-status ${statusCls}`}>{statusLabel}</span>
                            </div>
                            {seg.open && (seg.summary || seg.detail) && (
                              <div className="ac-seg-action-body">
                                {seg.summary && <div className="ac-seg-summary">{seg.summary}</div>}
                                {seg.detail && <pre className="ac-seg-detail">{seg.detail}</pre>}
                              </div>
                            )}
                          </div>
                        );
                      }
                      // response
                      return (
                        <div key={j} className="ac-msg-content ac-seg-response">
                          <ReactMarkdown remarkPlugins={[remarkGfm]}>{seg.text || ''}</ReactMarkdown>
                        </div>
                      );
                    })}
                    {msg._streaming && (
                      <div className="ac-seg-pending">
                        <span className="ac-dot" /><span className="ac-dot" /><span className="ac-dot" />
                        <span className="ac-seg-pending-label">추론 중...</span>
                      </div>
                    )}
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
              {loading && !messages.some(m => m._streaming) && (
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
                onClick={() => setShowThinking(p => !p)}
                style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  fontSize: '11px', color: showThinking ? '#6d28d9' : '#999',
                  padding: '2px 6px', marginRight: '4px', flexShrink: 0
                }}
                title={showThinking ? '추론/Action 감추기' : '추론/Action 보기'}
              >{showThinking ? '◉' : '○'}</button>
              <textarea ref={inputRef} className="ac-input" value={input}
                onChange={e => {
                  setInput(e.target.value);
                  e.target.style.height = 'auto';
                  e.target.style.height = Math.min(e.target.scrollHeight, 300) + 'px';
                }}
                onKeyDown={handleKeyDown}
                placeholder="메시지를 입력하세요..." rows={1} />
              {loading && (
                <button className="ac-send" onClick={handleCancel} style={{ background: '#c33', color: '#fff' }} title="현재 응답 중지">중지</button>
              )}
              <button className="ac-send" onClick={handleSend} disabled={!input.trim()} title={loading ? '현재 응답 중단 후 새 메시지 전송' : '전송'}>전송</button>
            </div>
          </>
        )}
    </div>
  );
}
