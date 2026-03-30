import React, { useEffect, useRef, useState, useCallback } from 'react';
import './RelationMap.css';

const API_BASE = 'http://localhost:9000/api/v2';

const COLUMN_DEFS = [
  { id: 'vos', label: 'VOS', types: ['VOS'], addBtns: ['VOS'] },
  { id: 'feature', label: 'Feature', types: ['Feature'], addBtns: ['Feature'] },
  { id: 'requirement', label: 'Requirement', types: ['Requirement'], addBtns: ['QA', 'FR', 'CNST'] },
  { id: 'design', label: 'Design', types: ['Architectueral Design', 'Detaied Design'], addBtns: ['ADSN', 'DDSN'] },
];

const SUBTYPE_COLORS = {
  VOS: '#70ad47',
  UC: '#ff9a1e',
  Feature: '#ff9a1e',
  QA: '#e83a1e',
  FR: '#e83a1e',
  CNST: '#e83a1e',
  ADSN: '#4cafe8',
  DDSN: '#7030a0',
};

/* ---------- helpers ---------- */

function getSubtype(issue) {
  const prefix = (issue.subKey || '').split('_')[0];
  if (SUBTYPE_COLORS[prefix]) return prefix;
  switch (issue.issueType) {
    case 'VOS': return 'VOS';
    case 'Feature': return 'UC';
    case 'Requirement': return issue.reqtype || 'FR';
    case 'Architectueral Design': return 'ADSN';
    case 'Detaied Design': return 'DDSN';
    default: return 'VOS';
  }
}

function buildConnections(issues) {
  const keySet = new Set(issues.map((i) => i.key));
  const issueMap = {};
  issues.forEach((i) => { issueMap[i.key] = i; });
  const conns = [];
  const seen = new Set();
  issues.forEach((issue) => {
    (issue.issueLinks || []).forEach((link) => {
      const target = link.outwardIssue;
      if (target && keySet.has(target.key)) {
        const id = `${issue.key}->${target.key}`;
        if (!seen.has(id)) {
          const targetIssue = issueMap[target.key];
          const sameType = targetIssue && targetIssue.issueType === issue.issueType;
          conns.push({ id, linkId: link.id, from: issue.key, to: target.key, type: link.type?.name || '', sameType });
          seen.add(id);
        }
      }
    });
  });
  // Same-type connections first (rendered behind), cross-type on top
  conns.sort((a, b) => (a.sameType === b.sameType ? 0 : a.sameType ? -1 : 1));
  return conns;
}

function getHighlightChain(selectedKey, connections) {
  if (!selectedKey) return { cards: new Set(), conns: new Set() };

  // Build directional adjacency: from→to (children) and to→from (parents)
  const children = {};  // from -> [{ key: to, id }]
  const parents = {};   // to -> [{ key: from, id }]
  connections.forEach((c) => {
    if (!children[c.from]) children[c.from] = [];
    if (!parents[c.to]) parents[c.to] = [];
    children[c.from].push({ key: c.to, id: c.id });
    parents[c.to].push({ key: c.from, id: c.id });
  });

  const cards = new Set([selectedKey]);
  const highlightedConns = new Set();

  // Traverse children (forward: selected → to → to → ...)
  const fwd = [selectedKey];
  while (fwd.length > 0) {
    const cur = fwd.shift();
    (children[cur] || []).forEach(({ key, id }) => {
      highlightedConns.add(id);
      if (!cards.has(key)) { cards.add(key); fwd.push(key); }
    });
  }

  // Traverse parents (backward: ... → from → from → selected)
  const bwd = [selectedKey];
  while (bwd.length > 0) {
    const cur = bwd.shift();
    (parents[cur] || []).forEach(({ key, id }) => {
      highlightedConns.add(id);
      if (!cards.has(key)) { cards.add(key); bwd.push(key); }
    });
  }

  return { cards, conns: highlightedConns };
}

function bezierPath(x1, y1, x2, y2) {
  const dx = Math.abs(x2 - x1);
  const cp = Math.max(dx * 0.4, 50);
  return `M ${x1} ${y1} C ${x1 + cp} ${y1}, ${x2 - cp} ${y2}, ${x2} ${y2}`;
}

/* ---------- main component ---------- */

export default function RelationMap({ project, onBack }) {
  const [issues, setIssues] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [selectedKey, setSelectedKey] = useState(null);
  const [sortByHighlight, setSortByHighlight] = useState(false);
  const [positions, setPositions] = useState({});
  const [linking, setLinking] = useState(null);   // { fromKey }
  const [mousePos, setMousePos] = useState(null);
  const [hoverConn, setHoverConn] = useState(null);

  const canvasRef = useRef(null);
  const cardElems = useRef({});

  const projectCode = project.JiraCode || '';

  /* fetch issues */
  const fetchIssues = useCallback(() => {
    setLoading(true);
    const xbody = JSON.stringify({ prjID: projectCode, infoType: '' });
    fetch(`${API_BASE}/Issue`, { headers: { XBody: xbody } })
      .then((r) => r.json())
      .then((data) => setIssues(Array.isArray(data) ? data : []))
      .catch(() => setIssues([]))
      .finally(() => setLoading(false));
  }, [projectCode]);

  useEffect(() => { fetchIssues(); }, [fetchIssues]);

  const connections = buildConnections(issues);
  const highlighted = getHighlightChain(selectedKey, connections);

  /* subtype sort order within columns */
  const SUBTYPE_ORDER = { QA: 0, CNST: 1, FR: 2, ADSN: 0, DDSN: 1, VOS: 0, UC: 0, Feature: 0 };

  /* group into columns with sorting per spec §9 */
  const columns = COLUMN_DEFS.map((col) => ({
    ...col,
    issues: issues
      .filter((i) => col.types.includes(i.issueType))
      .sort((a, b) => {
        // 0순위: 강조된 이슈 우선 (Shift+클릭 시에만)
        if (sortByHighlight && selectedKey) {
          const aHL = highlighted.cards.has(a.key) ? 0 : 1;
          const bHL = highlighted.cards.has(b.key) ? 0 : 1;
          if (aHL !== bHL) return aHL - bHL;
        }

        // 1순위: 서브타입 순서
        const aType = SUBTYPE_ORDER[getSubtype(a)] ?? 99;
        const bType = SUBTYPE_ORDER[getSubtype(b)] ?? 99;
        if (aType !== bType) return aType - bType;

        // 2순위: ASR 라벨 있는 이슈 우선
        const aASR = (a.labels || []).includes('ASR') ? 0 : 1;
        const bASR = (b.labels || []).includes('ASR') ? 0 : 1;
        if (aASR !== bASR) return aASR - bASR;

        // 3순위: 서브키 오름차순
        return (a.subKey || '').localeCompare(b.subKey || '', undefined, { numeric: true });
      }),
  }));

  /* measure card positions for SVG bezier drawing */
  const measurePositions = useCallback(() => {
    if (!canvasRef.current) return;
    const cr = canvasRef.current.getBoundingClientRect();
    const pos = {};
    Object.entries(cardElems.current).forEach(([key, el]) => {
      if (!el) return;
      const r = el.getBoundingClientRect();
      pos[key] = {
        left: r.left - cr.left,
        right: r.right - cr.left,
        cy: r.top - cr.top + r.height / 2,
      };
    });
    setPositions(pos);
  }, []);

  useEffect(() => {
    const id = requestAnimationFrame(measurePositions);
    return () => cancelAnimationFrame(id);
  }, [issues, editMode, sortByHighlight, selectedKey, measurePositions]);

  useEffect(() => {
    if (!canvasRef.current) return;
    const obs = new ResizeObserver(measurePositions);
    obs.observe(canvasRef.current);
    return () => obs.disconnect();
  }, [measurePositions]);

  /* ESC to cancel linking */
  useEffect(() => {
    function onKey(e) { if (e.key === 'Escape') setLinking(null); }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  /* mouse tracking for link drawing */
  function handleCanvasMouseMove(e) {
    if (!linking || !canvasRef.current) return;
    const cr = canvasRef.current.getBoundingClientRect();
    setMousePos({ x: e.clientX - cr.left, y: e.clientY - cr.top });
  }

  function handleCanvasClick(e) {
    if (e.target.closest('.rm-card')) return;
    setSelectedKey(null);
    setSortByHighlight(false);
    if (linking) setLinking(null);
  }

  /* card ref setter */
  const setCardRef = useCallback((key) => (el) => {
    if (el) cardElems.current[key] = el;
    else delete cardElems.current[key];
  }, []);

  /* ---------- edit mode actions ---------- */

  async function toggleLabel(issue, label) {
    const labels = issue.labels || [];
    const has = labels.includes(label);
    const newLabels = has ? labels.filter((l) => l !== label) : [...labels, label];
    try {
      await fetch(`${API_BASE}/Issue`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: issue.key, data: { labels: newLabels } }),
      });
      setIssues((prev) => prev.map((i) => (i.key === issue.key ? { ...i, labels: newLabels } : i)));
    } catch (err) {
      alert('라벨 업데이트 실패: ' + err.message);
    }
  }

  async function deleteIssue(issue) {
    if (!window.confirm(`"${issue.key}" 이슈를 삭제하시겠습니까?`)) return;
    try {
      await fetch(`${API_BASE}/Issue`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: issue.key }),
      });
      setIssues((prev) => prev.filter((i) => i.key !== issue.key));
    } catch (err) {
      alert('삭제 실패: ' + err.message);
    }
  }

  async function createLink(fromKey, toKey) {
    try {
      await fetch(`${API_BASE}/Issue/link`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ inwardKey: fromKey, outwardKey: toKey }),
      });
      fetchIssues();
    } catch (err) {
      alert('연결 생성 실패: ' + err.message);
    }
  }

  async function deleteLink(conn) {
    console.log('[deleteLink] clicked:', conn.id, 'linkId:', conn.linkId, 'from:', conn.from, 'to:', conn.to);
    if (!window.confirm('이 연결을 삭제하시겠습니까?')) return;
    try {
      await fetch(`${API_BASE}/Issue/link`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ linkId: conn.linkId, fromKey: conn.from, toKey: conn.to }),
      });
      fetchIssues();
    } catch (err) {
      alert('연결 삭제 실패: ' + err.message);
    }
  }

  async function addIssue(subtype) {
    let issueType, reqtype = '';
    switch (subtype) {
      case 'VOS':     issueType = 'VOS'; break;
      case 'Feature': issueType = 'Feature'; break;
      case 'FR': case 'QA': case 'CNST':
        issueType = 'Requirement'; reqtype = subtype; break;
      case 'ADSN':    issueType = 'Architectueral Design'; break;
      case 'DDSN':    issueType = 'Detaied Design'; break;
      default:        issueType = subtype;
    }
    try {
      await fetch(`${API_BASE}/Issue`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          projectKey: projectCode, issueType, reqtype, summary: '새 이슈',
          labels: (subtype === 'QA' || subtype === 'CNST') ? ['ASR'] : [],
        }),
      });
      fetchIssues();
    } catch (err) {
      alert('이슈 생성 실패: ' + err.message);
    }
  }

  function handleSummaryBlur(issue, newSummary) {
    if (newSummary === issue.summary) return;
    fetch(`${API_BASE}/Issue`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...issue, summary: newSummary }),
    })
      .then(() => setIssues((prev) => prev.map((i) => (i.key === issue.key ? { ...i, summary: newSummary } : i))))
      .catch((err) => alert('수정 실패: ' + err.message));
  }

  /* link drawing dots */
  function handleRightDot(key, e) {
    e.stopPropagation();
    if (!canvasRef.current) return;
    setLinking({ fromKey: key });
    const cr = canvasRef.current.getBoundingClientRect();
    setMousePos({ x: e.clientX - cr.left, y: e.clientY - cr.top });
  }

  function handleLeftDot(key, e) {
    e.stopPropagation();
    if (linking && linking.fromKey !== key) {
      createLink(linking.fromKey, key);
      setLinking(null);
    }
  }

  /* ---------- render ---------- */

  if (loading) {
    return (
      <div className="rm-wrapper">
        <div className="rm-topbar">
          <button className="rm-back-btn" onClick={onBack}>&larr; Project Map</button>
          <span className="rm-title">{project.Name} - Relation Map</span>
        </div>
        <div className="rm-loading">로딩 중...</div>
      </div>
    );
  }

  return (
    <div className="rm-wrapper">
      {/* top bar */}
      <div className="rm-topbar">
        <button className="rm-back-btn" onClick={onBack}>&larr; Project Map</button>
        <span className="rm-title">{project.Name} ({projectCode}) - Relation Map</span>
        <button
          className="rm-pull-btn"
          onClick={async () => {
            try {
              await fetch(`${API_BASE}/Issue/fetch`, {
                headers: { XBody: JSON.stringify({ prjID: projectCode }) },
              });
              fetchIssues();
            } catch (err) {
              alert('Pull 실패: ' + err.message);
            }
          }}
        >
          Pull
        </button>
        <button
          className={`rm-edit-toggle ${editMode ? 'rm-edit-active' : ''}`}
          onClick={() => { setEditMode(!editMode); setLinking(null); }}
        >
          {editMode ? '편집모드 ON' : '편집모드'}
        </button>
      </div>

      {/* content area */}
      <div
        className={`rm-content ${linking ? 'rm-linking' : ''}`}
        onMouseMove={handleCanvasMouseMove}
        onClick={handleCanvasClick}
      >
        <div className="rm-canvas" ref={canvasRef}>
          {/* columns */}
          <div className="rm-columns">
            {columns.map((col) => (
              <div key={col.id} className="rm-column">
                <div className="rm-column-header">{col.label}</div>

                {col.issues.map((issue) => {
                  const subtype = getSubtype(issue);
                  const color = SUBTYPE_COLORS[subtype] || '#999';
                  const isSelected = selectedKey === issue.key;
                  const isHL = highlighted.cards.has(issue.key);
                  const isDimmed = selectedKey && !isHL;
                  const hasASR = (issue.labels || []).includes('ASR');
                  const hasReserved = (issue.labels || []).includes('Reserved');
                  const isReq = issue.issueType === 'Requirement';
                  const isFRnoASR = subtype === 'FR' && !hasASR;
                  const isADSNnoDecision = subtype === 'ADSN' && (!issue.decision || issue.decision === '');
                  const cardColor = isFRnoASR ? '#ffb4b4' : isADSNnoDecision ? 'lightgray' : color;
                  const subkeyTextColor = (isFRnoASR || isADSNnoDecision) ? '#000' : '#fff';

                  return (
                    <div
                      key={issue.key}
                      ref={setCardRef(issue.key)}
                      className={
                        'rm-card' +
                        (isSelected ? ' rm-card-selected' : '') +
                        (isHL && !isSelected ? ' rm-card-highlighted' : '') +
                        (isDimmed ? ' rm-card-dimmed' : '')
                      }
                      style={isSelected || isHL ? { borderColor: cardColor } : undefined}
                      onClick={(e) => {
                        e.stopPropagation();
                        if (!linking) {
                          const newKey = isSelected ? null : issue.key;
                          setSelectedKey(newKey);
                          setSortByHighlight(e.shiftKey && newKey !== null);
                        }
                      }}
                    >
                      {/* left dot (link target) */}
                      {editMode && subtype !== 'VOS' && (
                        <div
                          className="rm-dot rm-dot-left"
                          style={{ background: '#777' }}
                          onClick={(e) => handleLeftDot(issue.key, e)}
                        />
                      )}

                      {/* card header: subkey badge + key or action buttons */}
                      <div className="rm-card-header">
                        <span className="rm-subkey" style={{ background: cardColor, color: subkeyTextColor, fontWeight: hasASR ? 700 : 'normal' }}>
                          {issue.subKey || subtype}
                        </span>
                        {editMode ? (
                          <span className="rm-card-actions">
                            {isReq && subtype === 'FR' && (
                              <button
                                className={`rm-action-btn rm-btn-a ${hasASR ? 'rm-btn-active' : ''}`}
                                onClick={(e) => { e.stopPropagation(); toggleLabel(issue, 'ASR'); }}
                                title="ASR 토글"
                              >A</button>
                            )}
                            <button
                              className={`rm-action-btn rm-btn-r ${hasReserved ? 'rm-btn-active' : ''}`}
                              onClick={(e) => { e.stopPropagation(); toggleLabel(issue, 'Reserved'); }}
                              title="Reserved 토글"
                            >R</button>
                            <button
                              className="rm-action-btn rm-btn-x"
                              onClick={(e) => { e.stopPropagation(); deleteIssue(issue); }}
                              title="삭제"
                            >X</button>
                          </span>
                        ) : (
                          <span className="rm-key">{issue.key}</span>
                        )}
                      </div>

                      {/* summary (editable in edit mode) */}
                      {editMode ? (
                        <div
                          className="rm-card-summary rm-card-summary-edit"
                          contentEditable
                          suppressContentEditableWarning
                          onBlur={(e) => handleSummaryBlur(issue, e.target.textContent)}
                          onClick={(e) => e.stopPropagation()}
                        >
                          {issue.summary || ''}
                        </div>
                      ) : (
                        <div className="rm-card-summary">{issue.summary || ''}</div>
                      )}

                      {/* right dot (link source) */}
                      {editMode && subtype !== 'DDSN' && (
                        <div
                          className="rm-dot rm-dot-right"
                          style={{ background: '#777' }}
                          onClick={(e) => handleRightDot(issue.key, e)}
                        />
                      )}
                    </div>
                  );
                })}

                {/* add buttons (edit mode) */}
                {editMode && (
                  <div className="rm-add-section">
                    <span className="rm-add-label">Add:</span>
                    {col.addBtns.map((btn) => (
                      <button
                        key={btn}
                        className="rm-add-btn"
                        style={{ borderColor: SUBTYPE_COLORS[btn] || '#999', color: SUBTYPE_COLORS[btn] || '#999' }}
                        onClick={() => addIssue(btn)}
                      >
                        {btn}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>

          {/* SVG visible connections (behind cards, z-index: 0) */}
          <svg className="rm-svg">
            {connections.map((conn) => {
              const fp = positions[conn.from];
              const tp = positions[conn.to];
              if (!fp || !tp) return null;

              const d = bezierPath(fp.right, fp.cy, tp.left, tp.cy);
              const isHL = highlighted.conns.has(conn.id);
              const isDimmed = selectedKey && !isHL;
              const isHover = hoverConn === conn.id;

              return (
                <path
                  key={conn.id}
                  d={d}
                  fill="none"
                  className={
                    'rm-conn' +
                    (isHL ? ' rm-conn-hl' : '') +
                    (isDimmed ? ' rm-conn-dimmed' : '') +
                    (isHover ? ' rm-conn-hover' : '')
                  }
                  strokeWidth={isHover ? 3 : editMode ? 2 : 1}
                  style={{ pointerEvents: 'none' }}
                />
              );
            })}
          </svg>

          {/* SVG hit areas (above cards, z-index: 2) — always rendered for hover, click only in edit mode */}
          <svg className="rm-svg rm-svg-edit">
            {connections.map((conn) => {
              const fp = positions[conn.from];
              const tp = positions[conn.to];
              if (!fp || !tp) return null;

              const d = bezierPath(fp.right, fp.cy, tp.left, tp.cy);

              return (
                <path
                  key={conn.id}
                  d={d}
                  fill="none"
                  stroke="transparent"
                  strokeWidth={14}
                  style={{ cursor: editMode ? 'pointer' : 'default', pointerEvents: 'stroke' }}
                  onMouseEnter={() => setHoverConn(conn.id)}
                  onMouseLeave={() => setHoverConn(null)}
                  onClick={editMode ? (e) => { e.stopPropagation(); deleteLink(conn); } : undefined}
                />
              );
            })}

            {/* temporary linking line */}
            {linking && mousePos && positions[linking.fromKey] && (
              <path
                d={bezierPath(
                  positions[linking.fromKey].right,
                  positions[linking.fromKey].cy,
                  mousePos.x,
                  mousePos.y,
                )}
                fill="none"
                stroke="#e67e22"
                strokeWidth={2}
                strokeDasharray="6 3"
                style={{ pointerEvents: 'none' }}
              />
            )}
          </svg>
        </div>
      </div>

      {/* status bar */}
      <div className="rm-statusbar">
        총 {issues.length}건
        {selectedKey && ` | 선택: ${selectedKey}`}
        {linking && ' | 연결 그리기 중 (ESC 취소)'}
      </div>
    </div>
  );
}
