import React, { useEffect, useRef, useState, useCallback } from 'react';
import CommonBar from '../../components/CommonBar';
import IssueDetailDialog from './IssueDetailDialog';
import { exportContent } from '../../utils/exportUtils';
import config from '../../config';
import './RelationMap.css';

const API_BASE = config.GOJIRA_API;

const COLUMN_DEFS_BASE = [
  { id: 'vos', label: 'VOS', types: ['VOS'], addBtns: ['VOS'] },
  { id: 'feature', label: 'Feature', types: ['Feature'], addBtns: ['Feature'] },
  { id: 'requirement', label: 'Requirement', types: ['Requirement'], addBtns: ['QA', 'FR', 'CNST'] },
];
const COLUMN_DESIGN = { id: 'design', label: 'Design', types: ['Architectural Design', 'Detailed Design'], addBtns: ['ADSN', 'DDSN'] };
const COLUMN_TEST = { id: 'test', label: 'Test', types: ['Test'], addBtns: ['Test'] };

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
  const prefix = (issue.subkey || '').split('_')[0];
  if (SUBTYPE_COLORS[prefix]) return prefix;
  switch (issue.issueType) {
    case 'VOS': return 'VOS';
    case 'Feature': return 'UC';
    case 'Requirement': return issue.reqtype || 'FR';
    case 'Architectural Design': return 'ADSN';
    case 'Detailed Design': return 'DDSN';
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

const NAV_ITEMS = [
  { key: 'docs', label: 'Docs' },
  { key: 'issues', label: 'Issues' },
  { key: 'relationMap', label: 'Relation Map' },
  { key: 'satam', label: 'SATAM' },
];

export default function RelationMap({ project, user, onBack, currentPage, onNavigate }) {
  const [issues, setIssues] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [filterASR, setFilterASR] = useState(() => sessionStorage.getItem('gojira_rm_A') === 'true');
  const [showReserved, setShowReserved] = useState(() => sessionStorage.getItem('gojira_rm_R') !== 'false');
  const [showTest, setShowTest] = useState(() => sessionStorage.getItem('gojira_rm_T') === 'true');
  useEffect(() => { sessionStorage.setItem('gojira_rm_A', String(filterASR)); }, [filterASR]);
  useEffect(() => { sessionStorage.setItem('gojira_rm_R', String(showReserved)); }, [showReserved]);
  useEffect(() => { sessionStorage.setItem('gojira_rm_T', String(showTest)); }, [showTest]);
  const [selectedKey, setSelectedKey] = useState(null);
  const [assistLoading, setAssistLoading] = useState(false);
  const [sortByHighlight, setSortByHighlight] = useState(false);
  const [positions, setPositions] = useState({});
  const [linking, setLinking] = useState(null);   // { fromKey }
  const [mousePos, setMousePos] = useState(null);
  const [hoverConn, setHoverConn] = useState(null);
  const [detailIssue, setDetailIssue] = useState(null);

  const canvasRef = useRef(null);
  const contentRef = useRef(null);
  const cardElems = useRef({});

  const projectCode = project.jiraCodes?.main?.jiraCode || '';
  const handleExport = useCallback((type) => exportContent(contentRef.current, type, `RelationMap_${projectCode}`), [projectCode]);

  /* fetch issues */
  const fetchIssues = useCallback(() => {
    setLoading(true);
    return fetch(`${API_BASE}/Issue`, { headers: { XBody: JSON.stringify({ prjID: projectCode, infoType: '' }) } })
      .then((r) => r.json())
      .then((data) => setIssues(Array.isArray(data) ? data : []))
      .catch(() => setIssues([]))
      .finally(() => setLoading(false));
  }, [projectCode]);

  useEffect(() => { fetchIssues(); }, [fetchIssues]);

  const handlePull = useCallback(async () => {
    try {
      await fetch(`${API_BASE}/Issue/fetch`, {
        headers: { XBody: JSON.stringify({ prjID: projectCode }) },
      });
      await fetchIssues();
    } catch (err) {
      alert('Pull 실패: ' + err.message);
    }
  }, [projectCode, fetchIssues]);

  /* filter issues based on options */
  const filteredIssues = issues.filter((i) => {
    if (filterASR && i.issueType === 'Requirement' && !(i.labels || []).includes('ASR')) return false;
    if (!showReserved && (i.labels || []).includes('Reserved')) return false;
    return true;
  });

  const connections = buildConnections(filteredIssues);
  const highlighted = getHighlightChain(selectedKey, connections);

  /* dynamic column defs based on T option */
  const COLUMN_DEFS = [...COLUMN_DEFS_BASE, showTest ? COLUMN_TEST : COLUMN_DESIGN];

  /* subtype sort order within columns */
  const SUBTYPE_ORDER = { QA: 0, CNST: 1, FR: 2, ADSN: 0, DDSN: 1, VOS: 0, UC: 0, Feature: 0, Test: 0 };

  /* group into columns with sorting per spec §9 */
  const columns = COLUMN_DEFS.map((col) => ({
    ...col,
    issues: filteredIssues
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
        return (a.subkey || '').localeCompare(b.subkey || '', undefined, { numeric: true });
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
  }, [issues, editMode, sortByHighlight, selectedKey, filterASR, showReserved, showTest, measurePositions]);

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
        body: JSON.stringify({ key: issue.key, prjID: projectCode }),
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

  const SUBKEY_PREFIX = {
    VOS: 'VOS', Feature: 'UC',
    QA: 'QA', CNST: 'CNST', FR: 'FR',
    ADSN: 'ADSN', DDSN: 'DDSN',
  };

  function getNextSubKey(subtype) {
    const prefix = (SUBKEY_PREFIX[subtype] || subtype) + '_';
    let maxNum = 0;
    issues.forEach((i) => {
      const sk = i.subkey || '';
      if (sk.startsWith(prefix)) {
        const num = parseInt(sk.substring(prefix.length), 10);
        if (!isNaN(num) && num > maxNum) maxNum = num;
      }
    });
    return prefix + String(maxNum + 1).padStart(3, '0');
  }

  async function addIssue(subtype) {
    let issueType, reqtype = '';
    switch (subtype) {
      case 'VOS':     issueType = 'VOS'; break;
      case 'Feature': issueType = 'Feature'; break;
      case 'FR': case 'QA': case 'CNST':
        issueType = 'Requirement'; reqtype = subtype === 'QA' ? 'NFR' : subtype; break;
      case 'ADSN':    issueType = 'Architectural Design'; break;
      case 'DDSN':    issueType = 'Detailed Design'; break;
      default:        issueType = subtype;
    }
    const subkey = getNextSubKey(subtype);
    const payload = {
      projectKey: projectCode, issueType, reqtype, summary: '새 이슈',
      labels: (subtype === 'QA' || subtype === 'CNST') ? ['ASR'] : [],
      subkey,
    };

    for (let attempt = 1; attempt <= 3; attempt++) {
      try {
        const res = await fetch(`${API_BASE}/Issue`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        if (res.ok) {
          fetchIssues();
          return;
        }
        const errData = await res.json().catch(() => ({}));
        if (attempt === 3) {
          const action = window.confirm(
            `이슈 생성 실패 (${attempt}회 시도)\n${errData.error || res.statusText}\n\n확인: 재시도 / 취소: 삭제`
          );
          if (action) { attempt = 0; continue; } // 재시도
          return; // 삭제 (서버에 저장 안 됨)
        }
      } catch (err) {
        if (attempt === 3) {
          const action = window.confirm(
            `이슈 생성 실패 (${attempt}회 시도)\n${err.message}\n\n확인: 재시도 / 취소: 삭제`
          );
          if (action) { attempt = 0; continue; }
          return;
        }
      }
    }
  }

  async function handleDetailSave(key, data) {
    try {
      await fetch(`${API_BASE}/Issue`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, data }),
      });
      setIssues((prev) => prev.map((i) => (i.key === key ? { ...i, ...data } : i)));
    } catch (err) {
      alert('저장 실패: ' + err.message);
    }
  }

  function handleSummaryBlur(issue, newSummary) {
    if (newSummary === issue.summary) return;
    fetch(`${API_BASE}/Issue`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key: issue.key, data: { summary: newSummary } }),
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

  const barTitle = `${project.name} (${projectCode})`;
  const nav = onNavigate ? NAV_ITEMS.map((n) => ({
    ...n, active: n.key === currentPage, handler: () => onNavigate(n.key),
  })) : [];

  if (loading) {
    return (
      <div className="rm-wrapper">
        <CommonBar
          title={barTitle}
          onBack={onBack}
          onPull={handlePull}
          onEditChange={setEditMode}
          nav={nav}
          options={[
            { label: 'A', active: filterASR, handler: () => setFilterASR((v) => !v) },
            { label: 'R', active: showReserved, handler: () => setShowReserved((v) => !v) },
            { label: 'T', active: showTest, handler: () => setShowTest((v) => !v) },
          ]}
        />
        <div className="rm-loading">로딩 중...</div>
      </div>
    );
  }

  return (
    <div className="rm-wrapper">
      <CommonBar
        title={barTitle}
        onBack={onBack}
        onPull={handlePull}
        onEditChange={(mode) => { setEditMode(mode); if (!mode) setLinking(null); }}
        nav={nav}
        options={[
          { label: 'A', active: filterASR, handler: () => setFilterASR((v) => !v) },
          { label: 'R', active: showReserved, handler: () => setShowReserved((v) => !v) },
          { label: 'T', active: showTest, handler: () => setShowTest((v) => !v) },
        ]}
        onExport={editMode ? undefined : handleExport}
        onAssist={editMode ? undefined : async () => {
          setAssistLoading(true);
          try {
            const res = await fetch(`${API_BASE}/Agent/chat`, {
              method: 'POST', headers: { 'Content-Type': 'application/json', 'X-User-Id': user || '' },
              body: JSON.stringify({ message: `${projectCode} 프로젝트의 이슈 관계를 분석해줘`, context: { page: 'relationMap', projectCode } }),
            });
            const d = await res.json();
            alert(d.reply || '분석 완료');
          } catch (e) { alert('오류: ' + e.message); }
          setAssistLoading(false);
        }}
        assistLabel="관계 분석"
        assistLoading={assistLoading}
      />

      {/* content area */}
      <div
        ref={contentRef}
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
                        <span
                          className={`rm-subkey${hasReserved ? ' rm-subkey-reserved' : ''}`}
                          style={hasReserved ? { cursor: 'pointer' } : { background: cardColor, color: subkeyTextColor, cursor: 'pointer' }}
                          onClick={(e) => { e.stopPropagation(); setDetailIssue(issue); }}
                        >
                          {issue.subkey || subtype}
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

      {/* Issue Detail Dialog */}
      {detailIssue && (
        <IssueDetailDialog
          issue={detailIssue}
          editMode={editMode}
          onClose={() => setDetailIssue(null)}
          onSave={handleDetailSave}
        />
      )}
    </div>
  );
}
