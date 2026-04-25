import React, { useEffect, useState } from 'react';
import config from '../config';

export default function PlantUMLBlock({ code }) {
  const [svg, setSvg] = useState('');
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const src = (code || '').trim();
    if (!src) {
      setError(null);
      setSvg('');
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    fetch(config.PLANTUML_SERVER, {
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
  }, [code]);

  if (error) {
    return (
      <div className="plantuml-error mermaid-error">
        <div className="mermaid-error-title">⚠ PlantUML 렌더링 오류</div>
        <div className="mermaid-error-msg">{error}</div>
        <pre className="mermaid-error-src">{code}</pre>
      </div>
    );
  }
  if (loading) {
    return <div className="plantuml-loading">Rendering PlantUML…</div>;
  }
  return (
    <div
      className="plantuml-render mermaid-render"
      // SVG body is fetched from a trusted local PlantUML server (config.PLANTUML_SERVER)
      // and only rendered for already-trusted markdown documents.
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
