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
    { key: 'divider' },
    // 과제 한 벌(이슈 + 설계문서 전체 이력)을 파일 하나로 내려받아 다른 GoJIRA
    // 시스템의 "과제 생성 → 파일에서 가져오기" 로 넘길 수 있다.
    { key: 'export', label: '과제 내보내기' },
    { key: 'divider2' },
    { key: 'relationMap', label: 'Relation Map' },
    { key: 'satam', label: 'SATAM' },
    { key: 'divider3' },
    { key: 'delete', label: '삭제' },
  ];

  return (
    <div className="ctx-menu" style={{ left: x, top: y }} ref={ref}>
      <div className="ctx-menu-header">{project.name}</div>
      {menuItems.map((item, i) =>
        item.key.startsWith('divider') ? (
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
