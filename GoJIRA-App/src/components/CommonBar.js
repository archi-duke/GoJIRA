import { useState } from 'react';
import './CommonBar.css';

export default function CommonBar({ title, onBack, onPull, onEditChange, options, nav }) {
  const [editMode, setEditMode] = useState(false);

  function handleEditToggle() {
    const next = !editMode;
    setEditMode(next);
    if (onEditChange) onEditChange(next);
  }

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
      <button className="cb-btn" onClick={onPull}>Pull</button>
      <button
        className={`cb-edit-toggle ${editMode ? 'cb-edit-active' : ''}`}
        onClick={handleEditToggle}
      >
        {editMode ? '편집모드 ON' : '편집모드'}
      </button>
    </div>
  );
}
