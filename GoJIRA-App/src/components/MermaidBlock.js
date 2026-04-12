import React, { useEffect, useRef, useState } from 'react';
import mermaid from 'mermaid';

// 전역에서 1회만 초기화
let _initialized = false;
function ensureInit() {
  if (_initialized) return;
  mermaid.initialize({
    startOnLoad: false,
    theme: 'default',
    securityLevel: 'loose',
    flowchart: { htmlLabels: true, curve: 'basis' },
    sequence: { showSequenceNumbers: false },
  });
  _initialized = true;
}

let _idCounter = 0;
function nextId() {
  _idCounter += 1;
  return `mmd-${Date.now().toString(36)}-${_idCounter}`;
}

export default function MermaidBlock({ code }) {
  const containerRef = useRef(null);
  const idRef = useRef(nextId());
  const [error, setError] = useState(null);

  useEffect(() => {
    ensureInit();
    let cancelled = false;
    const src = (code || '').trim();
    if (!src) {
      setError(null);
      if (containerRef.current) containerRef.current.innerHTML = '';
      return;
    }
    mermaid
      .render(idRef.current, src)
      .then(({ svg, bindFunctions }) => {
        if (cancelled || !containerRef.current) return;
        containerRef.current.innerHTML = svg;
        if (bindFunctions) bindFunctions(containerRef.current);
        setError(null);
      })
      .catch((e) => {
        if (cancelled) return;
        setError(e?.message || String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [code]);

  if (error) {
    return (
      <div className="mermaid-error">
        <div className="mermaid-error-title">⚠ 다이어그램 렌더링 오류</div>
        <div className="mermaid-error-msg">{error}</div>
        <pre className="mermaid-error-src">{code}</pre>
      </div>
    );
  }
  return <div ref={containerRef} className="mermaid-render" />;
}
