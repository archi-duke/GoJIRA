import { useState, useRef, useEffect } from 'react';
import './CommonBar.css';

export default function CommonBar({ title, onBack, onPull, onEditChange, options, nav, onExport }) {
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
    if (pulling) return;
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

  // 드롭다운 외부 클릭 시 닫기
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
      <button className="cb-back-btn" onClick={onBack}>&larr; Back</button>
      <span className="cb-title">{title}</span>

      {/* 컴포넌트 네비게이션 */}
      {(nav || []).length > 0 && (
        <>
          {nav.map((item) => (
            <button
              key={item.key}
              className={`cb-nav-btn ${item.active ? 'cb-nav-active' : ''}`}
              onClick={item.handler}
            >
              {item.label}
            </button>
          ))}
          <span className="cb-divider" />
        </>
      )}

      {/* 컴포넌트별 옵션 버튼 */}
      {(options || []).length > 0 && (
        <>
          <span className="cb-options-label">Options:</span>
          {options.map((opt, i) => (
            <button
              key={i}
              className={`cb-opt-btn ${opt.active ? 'cb-opt-btn-active' : ''}`}
              onClick={opt.handler}
            >
              {opt.label}
            </button>
          ))}
        </>
      )}

      <span className="cb-divider" />
      <button className="cb-btn" onClick={handlePull} disabled={pulling}>
        {pulling ? 'Pulling...' : 'Pull'}
      </button>

      {onExport && (
        <div className="cb-export-wrap" ref={exportRef}>
          <button className="cb-btn" onClick={() => setExportOpen((v) => !v)}>Export</button>
          {exportOpen && (
            <div className="cb-export-menu">
              <button className="cb-export-item" onClick={() => handleExport('html')}>HTML 파일</button>
              <button className="cb-export-item" onClick={() => handleExport('jpg')}>JPG 이미지</button>
              <button className="cb-export-item" onClick={() => handleExport('clipboard')}>HTML 클립보드</button>
            </div>
          )}
        </div>
      )}

      <button
        className={`cb-edit-toggle ${editMode ? 'cb-edit-active' : ''}`}
        onClick={handleEditToggle}
      >
        {editMode ? '편집모드 ON' : '편집모드'}
      </button>

      {(pulling || exporting) && <div className="cb-overlay"><div className="cb-spinner" /></div>}
    </div>
  );
}
