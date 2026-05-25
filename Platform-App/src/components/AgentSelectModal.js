import React, { useState } from 'react';

const LAST_KEY = 'gojira_last_agent';

// AgentSelectModal — 런처에 등록된 CLI 에이전트가 2개 이상일 때 사용자가 어떤 걸로
// spawn 할지 고르는 단순 모달. 마지막 선택을 localStorage 에 저장해 다음 호출의 기본값
// 으로 복원한다. 1개거나 비어있는 경우 호출하지 말 것 — 부모(App.js)에서 단축.
export default function AgentSelectModal({ agents, onSelect, onCancel }) {
  const initial = (() => {
    try {
      const last = localStorage.getItem(LAST_KEY);
      if (last && agents.includes(last)) return last;
    } catch {}
    return agents[0];
  })();
  const [picked, setPicked] = useState(initial);

  function confirm() {
    try { localStorage.setItem(LAST_KEY, picked); } catch {}
    onSelect(picked);
  }

  return (
    <div style={overlay} onClick={onCancel}>
      <div style={dialog} onClick={(e) => e.stopPropagation()}>
        <div style={title}>실행할 에이전트 선택</div>
        <div style={list}>
          {agents.map((a) => (
            <label key={a} style={{ ...item, ...(picked === a ? itemActive : null) }}>
              <input
                type="radio"
                name="gojira-agent-pick"
                checked={picked === a}
                onChange={() => setPicked(a)}
                style={{ marginRight: 8 }}
              />
              <code>{a}</code>
            </label>
          ))}
        </div>
        <div style={footer}>
          <button onClick={onCancel} style={btn}>취소</button>
          <button onClick={confirm} style={{ ...btn, ...btnPrimary }} autoFocus>실행</button>
        </div>
      </div>
    </div>
  );
}

const overlay = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.45)',
  display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000,
};
const dialog = {
  background: '#fff', borderRadius: 8, padding: 20, minWidth: 320,
  boxShadow: '0 6px 24px rgba(0,0,0,0.2)', fontFamily: 'inherit',
};
const title = { fontSize: 14, fontWeight: 600, marginBottom: 12, color: '#2c3e50' };
const list = { display: 'flex', flexDirection: 'column', gap: 6, marginBottom: 16 };
const item = {
  display: 'flex', alignItems: 'center', padding: '8px 10px',
  border: '1px solid #ddd', borderRadius: 4, cursor: 'pointer', fontSize: 13,
};
const itemActive = { borderColor: '#2c3e50', background: '#f0f4f8' };
const footer = { display: 'flex', justifyContent: 'flex-end', gap: 8 };
const btn = {
  padding: '6px 14px', fontSize: 13, border: '1px solid #ccc',
  background: '#fff', borderRadius: 4, cursor: 'pointer',
};
const btnPrimary = { background: '#2c3e50', color: '#fff', borderColor: '#2c3e50' };
