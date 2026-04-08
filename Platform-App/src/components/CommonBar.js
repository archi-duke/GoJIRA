import { useState, useRef, useEffect } from 'react';
import './CommonBar.css';

export default function CommonBar({ title, onBack, onPull, onEditChange, options, nav, onExport, onAssist, assistLabel, assistLoading, assistIcon }) {
  const [editMode, setEditMode] = useState(false);
  const [pulling, setPulling] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [exporting, setExporting] = useState(false);
  const exportRef = useRef(null);

  function handleEditToggle() {
    const next = !editMode;
    setEditMode(next);
    if (onEditChange) onEditChange(next);
  }

  async function handlePull() {
    if (pulling || !onPull) return;
    setPulling(true);
    try {
      await onPull();
    } finally {
      setPulling(false);
    }
  }

  async function handleExport(type) {
    setExportOpen(false);
    if (!onExport) return;
    setExporting(true);
    try {
      await onExport(type);
    } finally {
      setExporting(false);
    }
  }

  useEffect(() => {
    if (!exportOpen) return;
    function close(e) {
      if (exportRef.current && !exportRef.current.contains(e.target)) setExportOpen(false);
    }
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [exportOpen]);

  return (
    <div className="cb-topbar">
      <span className="cb-title">{title}</span>

      {/* 네비게이션 */}
      {(nav || []).length > 0 && (
        <>
          {nav.map((item) => (
            <button key={item.key} className={`cb-nav-btn ${item.active ? 'cb-nav-active' : ''}`} onClick={item.handler}>
              {item.label}
            </button>
          ))}
          <span className="cb-divider" />
        </>
      )}

      {/* 옵션 버튼 */}
      {(options || []).length > 0 && (
        <>
          {options.map((opt, i) => (
            <button key={i} className={`cb-opt-btn ${opt.active ? 'cb-opt-btn-active' : ''}`} onClick={opt.handler}>
              {opt.label}
            </button>
          ))}
        </>
      )}

      <span className="cb-divider" />

      {/* Pull 아이콘 */}
      {onPull && (
        <button className="cb-icon-btn" onClick={handlePull} disabled={pulling} title="Pull">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
            <path d="M8 1v9M4 6l4 4 4-4M2 13h12" stroke="currentColor" strokeWidth="1.8" fill="none" strokeLinecap="round" />
          </svg>
        </button>
      )}

      {/* Export 아이콘 */}
      {onExport && (
        <div className="cb-export-wrap" ref={exportRef}>
          <button className="cb-icon-btn" onClick={() => setExportOpen((v) => !v)} title="Export">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{transform:'rotate(90deg)'}}>
              <path d="M4 12v7a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-7" />
              <polyline points="16 6 12 2 8 6" />
              <line x1="12" y1="2" x2="12" y2="15" />
            </svg>
          </button>
          {exportOpen && (
            <div className="cb-export-menu">
              <button className="cb-export-item" onClick={() => handleExport('html')}>HTML 파일</button>
              <button className="cb-export-item" onClick={() => handleExport('jpg')}>JPG 이미지</button>
              <button className="cb-export-item" onClick={() => handleExport('clipboard')}>HTML 클립보드</button>
            </div>
          )}
        </div>
      )}

      {/* Agent Assist 버튼 */}
      {onAssist && (
        <button className="cb-assist-btn" onClick={onAssist} disabled={assistLoading}>
          {assistLoading ? '분석 중...' : (assistLabel || 'Agent 분석')}{assistIcon}
        </button>
      )}

      {/* 편집모드 */}
      <button className={`cb-edit-toggle ${editMode ? 'cb-edit-active' : ''}`} onClick={handleEditToggle}>
        {editMode ? '편집모드 ON' : '편집모드'}
      </button>

      {(pulling || exporting) && <div className="cb-overlay"><div className="cb-spinner" /></div>}
    </div>
  );
}
