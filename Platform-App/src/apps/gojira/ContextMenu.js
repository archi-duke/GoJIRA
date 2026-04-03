import React, { useEffect, useRef } from 'react';
import './ContextMenu.css';

export default function ContextMenu({ x, y, project, onClose, onAction }) {
  const ref = useRef(null);

  useEffect(() => {
    function handleClick(e) {
      if (ref.current && !ref.current.contains(e.target)) {
        onClose();
      }
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  const menuItems = [
    { key: 'detail', label: '상세정보' },
    { key: 'edit', label: '편집' },
    { key: 'delete', label: '삭제' },
    { key: 'divider' },
    { key: 'relationMap', label: 'Relation Map' },
    { key: 'satam', label: 'SATAM' },
    { key: 'divider' },
    { key: 'docgen', label: '설계문서 생성' },
  ];

  return (
    <div className="ctx-menu" style={{ left: x, top: y }} ref={ref}>
      <div className="ctx-menu-header">{project.name}</div>
      {menuItems.map((item, i) =>
        item.key === 'divider' ? (
          <div key={i} className="ctx-menu-divider" />
        ) : (
          <div
            key={item.key}
            className={`ctx-menu-item ${item.key === 'delete' ? 'ctx-menu-danger' : ''}`}
            onClick={() => { onAction(item.key, project); onClose(); }}
          >
            {item.label}
          </div>
        )
      )}
    </div>
  );
}
