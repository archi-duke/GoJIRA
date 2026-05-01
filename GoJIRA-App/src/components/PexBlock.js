import React, { useEffect, useRef, useState } from 'react';
import config from '../config';
import { loadPexCore } from '../utils/pexLoader';
import './PexBlock.css';

// source → 렌더된 SVG 캐시. 부모(ReactMarkdown components) 가 매 렌더마다 code 콜백을
// 새 함수로 만들어 PexBlock 인스턴스가 언마운트/재마운트되어도, 같은 source 면 즉시 SVG 를
// 표시해서 loading 텍스트로 인한 높이 깜빡임을 막는다.
const svgCache = new Map();

/**
 * PexBlock — pumlex 인라인 편집 PlantUML 블록.
 *
 * pex-vscode 의 markdownItPlugin + preview.js 패턴을 React 로 포팅:
 *  1. /render-with-layout 으로 SVG fetch → inline DOM
 *  2. 마우스 hover 시 ✎ 편집 버튼 노출
 *  3. 편집 클릭 → window.PexInline.activate(...) — SVG 위 핸들 오버레이
 *  4. ✓ 적용 → onCommit(originalSource, newSrc) 호출 → host (DocEditor) 가
 *     본문이 originalSource 와 일치하는 ```plantuml fence 의 본문을 newSrc 로 replace
 *
 * pex-core 는 public/pex-core/{geom,meta,inline}.js 가 plain script 로 로드되어
 * window.PexInline 이 노출됨 (UMD).
 *
 * Props:
 *  - source     : ```plantuml fence 본문 (string). 호스트가 fence 식별자로도 사용
 *  - onCommit   : (originalSource, newSource) => void. 미지정 시 read-only 모드
 *  - readOnly   : true 면 편집 버튼 표시 안 함 (textarea 모드 등)
 */
export default function PexBlock({ source, onCommit, readOnly }) {
  // 초기 state 를 캐시로 시드 — 같은 source 로 재마운트되면 loading=true 상태를 거치지 않음
  const [svg, setSvg] = useState(() => svgCache.get(source) || '');
  const [loading, setLoading] = useState(() => !svgCache.has(source));
  const [error, setError] = useState(null);
  const [editing, setEditing] = useState(false);

  const blockRef = useRef(null);
  const renderRef = useRef(null);
  const sessionRef = useRef(null);
  const pendingSrcRef = useRef(source);

  // 편집 가능 = onCommit 콜백 있고 readOnly 아님
  const editable = !readOnly && typeof onCommit === 'function';

  // pex-core 사전 로드 — 편집 버튼 클릭 시 await 가 즉시 resolve 되도록 prefetch.
  // 마이크로프론트엔드 컨테이너에서는 정적 script 태그가 없으므로 동적 inject.
  useEffect(() => {
    if (!editable) return;
    loadPexCore().catch((e) => {
      // eslint-disable-next-line no-console
      console.warn('[PexBlock] pex-core prefetch 실패', e);
    });
  }, [editable]);

  // SVG 렌더 — source 변경 시 재요청
  useEffect(() => {
    let cancelled = false;
    pendingSrcRef.current = source;
    const src = (source || '').trim();
    if (!src) {
      setError(null);
      setSvg('');
      setLoading(false);
      return undefined;
    }
    // 캐시 hit 면 fetch 건너뜀 (이미 초기 state 에서 시드된 상태)
    if (svgCache.has(source)) {
      setSvg(svgCache.get(source));
      setLoading(false);
      setError(null);
      return undefined;
    }
    setLoading(true);
    setError(null);
    fetch(`${config.PUMLEX_SERVER}/render-with-layout`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain; charset=utf-8' },
      body: src,
    })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.text();
      })
      .then((text) => {
        if (cancelled) return;
        svgCache.set(source, text);
        setSvg(text);
        setLoading(false);
      })
      .catch((e) => {
        if (cancelled) return;
        setError(e?.message || String(e));
        setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [source]);

  // unmount / source 변경 시 활성 편집 세션 정리
  useEffect(() => {
    return () => {
      if (sessionRef.current) {
        try { sessionRef.current.deactivate(); } catch (_) { /* noop */ }
        sessionRef.current = null;
      }
    };
  }, [source]);

  // svg 를 ref 로 직접 inject — dangerouslySetInnerHTML 을 쓰면 부모 setState 로 인한
  // 재렌더 때 React 가 같은 string 을 비교하더라도 props object reference 가 매번
  // 새로 만들어지면서 setInnerHTML 을 다시 호출, PexInline 이 단 listener 를 쓸어버림.
  // → div 의 children 을 React 모델 밖으로 두고 svg state 가 실제로 바뀔 때만 갱신.
  useEffect(() => {
    const el = renderRef.current;
    if (!el) return undefined;
    el.innerHTML = svg || '';
    return () => {
      // unmount 또는 svg 교체 시 비워둠 — PexInline 핸들러는 deactivate effect 가 정리
    };
  }, [svg]);

  async function startEdit() {
    if (!editable) return;
    if (!renderRef.current?.querySelector('svg')) return;
    try {
      await loadPexCore();
    } catch (e) {
      // eslint-disable-next-line no-alert
      alert(`pex-core 로드 실패: ${e?.message || e}`);
      return;
    }
    if (typeof window === 'undefined' || !window.PexInline) {
      // eslint-disable-next-line no-alert
      alert('PexInline 모듈이 로드되지 않았습니다 (public/pex-core/*.js 확인).');
      return;
    }
    // 비동기 await 후 컴포넌트가 unmount 됐을 가능성
    if (!renderRef.current?.querySelector('svg')) return;

    pendingSrcRef.current = source;
    sessionRef.current = window.PexInline.activate(renderRef.current, {
      source,
      useDrafts: false,
      useDirtyBadge: false,
      onSourceChange: (newSrc) => {
        pendingSrcRef.current = newSrc;
      },
    });
    setEditing(true);
  }

  function commitEdit() {
    const newSrc = pendingSrcRef.current;
    if (sessionRef.current) {
      try { sessionRef.current.deactivate(); } catch (_) { /* noop */ }
      sessionRef.current = null;
    }
    setEditing(false);
    if (newSrc && newSrc !== source && typeof onCommit === 'function') {
      onCommit(source, newSrc);
    }
  }

  function cancelEdit() {
    if (sessionRef.current) {
      try { sessionRef.current.deactivate(); } catch (_) { /* noop */ }
      sessionRef.current = null;
    }
    pendingSrcRef.current = source;
    setEditing(false);
  }

  return (
    <div
      ref={blockRef}
      className={`pumlex-block ${editing ? 'pumlex-editing' : ''} ${loading ? 'pumlex-loading' : ''}`}
    >
      {error ? (
        <div className="pumlex-error">
          <div className="pumlex-error-title">⚠ PlantUML 렌더링 오류</div>
          <div className="pumlex-error-msg">{error}</div>
        </div>
      ) : loading ? (
        <div className="pumlex-loading-text">렌더링 중…</div>
      ) : (
        <div ref={renderRef} className="pumlex-render" />
      )}

      {editable && !loading && !error && !editing && (
        <button type="button" className="pumlex-btn pumlex-edit-btn" onClick={startEdit}>
          ✎ 편집
        </button>
      )}
      {editable && editing && (
        <>
          <button type="button" className="pumlex-btn pumlex-commit-btn" onClick={commitEdit}>
            ✓ 적용
          </button>
          <button type="button" className="pumlex-btn pumlex-cancel-btn" onClick={cancelEdit}>
            ✗ 취소
          </button>
        </>
      )}
    </div>
  );
}
