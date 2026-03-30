import { useState } from 'react';
import './CommonBar.css';

export default function CommonBar({ title, onBack, onPull, onEditChange, options }) {
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

      {/* 컴포넌트별 옵션 버튼 */}
      {(options || []).map((opt, i) => (
        <button key={i} className="cb-btn" onClick={opt.handler}>
          {opt.label}
        </button>
      ))}

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
