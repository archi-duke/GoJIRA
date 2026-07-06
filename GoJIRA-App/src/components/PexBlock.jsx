import React, { useEffect, useLayoutEffect, useRef, useState } from 'react';
import config from '../config';
import { loadPexCore } from '../utils/pexLoader';
import './PexBlock.css';

// source → 렌더된 SVG 캐시. 재마운트(파일 전환 등) 시 같은 source 면 fetch 없이 즉시
// SVG 를 표시해서 loading 텍스트로 인한 높이 깜빡임을 막는다.
const svgCache = new Map();

// 캐시 키 정규화 — 같은 다이어그램이 경로에 따라 개행이 달라져 캐시 미스가 나는 것을 막는다.
//  · commitEdit 캡처본 키: pex-core buildSource() 결과 (embedMeta 는 끝에 \n 을 붙임)
//  · 마운트 시 조회 키: ReactMarkdown 이 준 fence 본문 (LF, 끝 개행 strip)
//  · 워킹트리 원본: Windows 면 CRLF
// → LF + trim 공간으로 통일 (DocEditor 의 fence 매칭과 같은 정규화).
function cacheKey(source) {
  return String(source || '').replace(/\r\n/g, '\n').trim();
}

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
  const [svg, setSvg] = useState(() => svgCache.get(cacheKey(source)) || '');
  const [loading, setLoading] = useState(() => !svgCache.has(cacheKey(source)));
  const [error, setError] = useState(null);
  const [editing, setEditing] = useState(false);

  const blockRef = useRef(null);
  const renderRef = useRef(null);
  const sessionRef = useRef(null);
  const pendingSrcRef = useRef(source);

  // 편집 가능 = onCommit 콜백 있고 readOnly 아님
  const editable = !readOnly && typeof onCommit === 'function';

  // pex-core 사전 로드 — fitViewBox(모든 표시 SVG 의 viewBox 재-fit)와 편집(클릭 시 await
  // 즉시 resolve) 양쪽에 필요하므로 editable 여부와 무관하게 로드한다. loadPexCore 는
  // 싱글턴이라 PexBlock 이 많아도 1회만 로드. MFE 컨테이너엔 정적 script 가 없어 동적 inject.
  useEffect(() => {
    loadPexCore().catch((e) => {
      // eslint-disable-next-line no-console
      console.warn('[PexBlock] pex-core prefetch 실패', e);
    });
  }, []);

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
    if (svgCache.has(cacheKey(source))) {
      setSvg(svgCache.get(cacheKey(source)));
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
        svgCache.set(cacheKey(source), text);
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
  // useLayoutEffect: paint 전에 주입 + fitViewBox 까지 적용해야 마운트/재마운트 시
  // "빈 div 한 프레임 → SVG 채워짐" 공백 프레임과 viewBox 점프가 안 보인다.
  useLayoutEffect(() => {
    const el = renderRef.current;
    if (!el) return undefined;
    el.innerHTML = svg || '';
    if (!svg) return undefined;
    // 표시되는 SVG 의 viewBox 를 브라우저 getBBox 기준으로 콘텐츠에 맞게 재-fit 한다.
    // 로직은 pex-core(PexInline.fitViewBox) 한곳에만 있고 — 편집용 adjustViewBox 와 같은
    // 알고리즘 — 웹/vscode 등 모든 클라이언트가 재사용한다(클라이언트마다 재구현 X).
    // 서버 /render-with-layout 의 viewBox 는 서버측 getBBox 근사라 저장→리로드 시 다이어그램이
    // 틀어지거나 잘리는데, 브라우저에서 다시 fit 하면 편집뷰와 프레이밍이 정확히 일치한다.
    let cancelled = false;
    const fit = () => {
      if (cancelled) return;
      const node = renderRef.current?.querySelector('svg');
      if (node && window.PexInline && typeof window.PexInline.fitViewBox === 'function') {
        window.PexInline.fitViewBox(node, { pad: 10 });
      }
    };
    fit(); // pex-core 가 이미 로드돼 있으면 즉시 적용
    loadPexCore().then(fit).catch(() => { /* 로드 실패 시 서버 viewBox 유지 */ });
    return () => {
      cancelled = true;
      // PexInline 핸들러는 deactivate effect 가 정리
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
    const willCommit = newSrc && newSrc !== source && typeof onCommit === 'function';
    // 편집 중 화면에 보이던 SVG 를 그대로 캡처해 새 source 의 캐시로 시드한다.
    // 저장 후 서버 /render-with-layout 재렌더는 서버측 getBBox 근사라 viewBox/위치가
    // 미세하게 어긋나 "전체 뷰영역이 틀어지거나 잘리는" 현상이 생긴다. deactivate 전에
    // (핸들 오버레이만 제거하고) 캡처해 두면, 재마운트 시 fetch 대신 이 캡처본을 써서
    // 저장 전후 뷰가 픽셀 단위로 동일하게 유지된다.
    if (willCommit) {
      const live = renderRef.current?.querySelector('svg');
      if (live) {
        const clone = live.cloneNode(true);
        clone.querySelectorAll('g.pex-handle-layer, .pex-handle, .pex-handle-guide').forEach((n) => n.remove());
        svgCache.set(cacheKey(newSrc), clone.outerHTML);
      }
    }
    if (sessionRef.current) {
      try { sessionRef.current.deactivate(); } catch (_) { /* noop */ }
      sessionRef.current = null;
    }
    setEditing(false);
    if (willCommit) {
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
