import React, { useEffect, useState, useCallback, useMemo } from 'react';
import CommonBar from './CommonBar';
import IssueDetailDialog from './IssueDetailDialog';
import './SATAM.css';

const API_BASE = 'http://localhost:9000/api/v2';

const SUBTYPE_COLORS = {
  VOS: '#70ad47', UC: '#ff9a1e', Feature: '#ff9a1e',
  QA: '#e83a1e', FR: '#e83a1e', CNST: '#e83a1e',
  ADSN: '#4cafe8', DDSN: '#7030a0',
};

const SUBTYPE_ORDER = { QA: 0, CNST: 1, FR: 2 };

const MARK_COLORS = {
  R: '#e83a1e',   // Risk - 빨강
  S: '#ff9a1e',   // Sensitivity - 주황
  M: '#4cafe8',   // Mitigation - 파랑
};

/* ---------- YAML-like 파서 ---------- */

function parseRST(yamlText) {
  if (!yamlText || typeof yamlText !== 'string') return [];
  const items = [];
  const lines = yamlText.split('\n');
  let current = null;

  for (const raw of lines) {
    const line = raw.trim();
    if (!line) continue;

    // 새 항목 시작: "- ID : R1" or "- ID: R1"
    const idMatch = line.match(/^-\s*ID\s*:\s*(.+)/i);
    if (idMatch) {
      if (current) items.push(current);
      current = { id: idMatch[1].trim(), qa: [], counterRisk: [], description: '' };
      continue;
    }

    if (!current) continue;

    // QA : [GOJIRA-001, GOJIRA-002]
    const qaMatch = line.match(/^QA\s*:\s*\[([^\]]*)\]/i);
    if (qaMatch) {
      current.qa = qaMatch[1].split(',').map((s) => s.trim()).filter(Boolean);
      continue;
    }

    // Counter Risk : ["R1:GOJIRA-001", ...]
    const crMatch = line.match(/^Counter\s*Risk\s*:\s*\[([^\]]*)\]/i);
    if (crMatch) {
      current.counterRisk = crMatch[1]
        .split(',')
        .map((s) => s.trim().replace(/"/g, ''))
        .filter(Boolean);
      continue;
    }

    // Description : ...
    const descMatch = line.match(/^Description\s*:\s*(.*)/i);
    if (descMatch) {
      current.description = descMatch[1].trim();
      continue;
    }
  }
  if (current) items.push(current);
  return items;
}

/* risk/sensitivity/mitigation 데이터를 직렬화 */
function serializeRST(items) {
  return items.map((item) => {
    let s = `- ID : ${item.id}`;
    if (item.qa && item.qa.length > 0) {
      s += `\n  QA : [${item.qa.join(', ')}]`;
    }
    if (item.counterRisk && item.counterRisk.length > 0) {
      s += `\n  Counter Risk : [${item.counterRisk.map((cr) => `"${cr}"`).join(', ')}]`;
    }
    if (item.description) {
      s += `\n  Description : ${item.description}`;
    }
    return s;
  }).join('\n');
}

/* ---------- 사전 빌드 ---------- */

function buildDictionaries(designIssues) {
  // designKey -> { risks: [...], sensitivities: [...], mitigations: [...] }
  const byDesign = {};
  // qaKey -> { risks: [...], sensitivities: [...], mitigations: [...] } (requirement key 기준)
  const byQA = {};
  // 전역 risk/sensitivity/mitigation 딕셔너리: id -> item (with designKey)
  const riskDict = {};
  const sensDict = {};
  const mitiDict = {};

  designIssues.forEach((d) => {
    const risks = parseRST(d.risk);
    const sensitivities = parseRST(d.sensitivity);
    const mitigations = parseRST(d.mitigation);

    byDesign[d.key] = { risks, sensitivities, mitigations };

    risks.forEach((r) => {
      riskDict[`${d.key}:${r.id}`] = { ...r, designKey: d.key };
      r.qa.forEach((qaKey) => {
        if (!byQA[qaKey]) byQA[qaKey] = { risks: [], sensitivities: [], mitigations: [] };
        byQA[qaKey].risks.push({ ...r, designKey: d.key });
      });
    });

    sensitivities.forEach((s) => {
      sensDict[`${d.key}:${s.id}`] = { ...s, designKey: d.key };
      s.qa.forEach((qaKey) => {
        if (!byQA[qaKey]) byQA[qaKey] = { risks: [], sensitivities: [], mitigations: [] };
        byQA[qaKey].sensitivities.push({ ...s, designKey: d.key });
      });
    });

    mitigations.forEach((m) => {
      mitiDict[`${d.key}:${m.id}`] = { ...m, designKey: d.key };
      m.qa.forEach((qaKey) => {
        if (!byQA[qaKey]) byQA[qaKey] = { risks: [], sensitivities: [], mitigations: [] };
        byQA[qaKey].mitigations.push({ ...m, designKey: d.key });
      });
    });
  });

  return { byDesign, byQA, riskDict, sensDict, mitiDict };
}

/* ---------- 셀 교차점 계산 ---------- */

function getCellInfo(designIssue, reqIssue, dicts) {
  const designData = dicts.byDesign[designIssue.key] || { risks: [], sensitivities: [], mitigations: [] };

  // 해당 requirement key가 등록된 R/S/M
  const risks = designData.risks.filter((r) => r.qa.includes(reqIssue.key));
  const sensitivities = designData.sensitivities.filter((s) => s.qa.includes(reqIssue.key));
  const mitigations = designData.mitigations.filter((m) => m.qa.includes(reqIssue.key));

  // issueLink contains 확인
  const hasContains = (reqIssue.issueLinks || []).some((link) => {
    const linkType = link.type?.name || '';
    const isContains = linkType.toLowerCase().includes('contains');
    const targetKey = link.inwardIssue?.key || link.outwardIssue?.key;
    return isContains && targetKey === designIssue.key;
  });

  return { risks, sensitivities, mitigations, hasContains };
}

/* ---------- subtype / sorting helpers (공통) ---------- */

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

function sortIssues(issues) {
  return [...issues].sort((a, b) => {
    const aType = SUBTYPE_ORDER[getSubtype(a)] ?? 99;
    const bType = SUBTYPE_ORDER[getSubtype(b)] ?? 99;
    if (aType !== bType) return aType - bType;
    const aASR = (a.labels || []).includes('ASR') ? 0 : 1;
    const bASR = (b.labels || []).includes('ASR') ? 0 : 1;
    if (aASR !== bASR) return aASR - bASR;
    return (a.subKey || '').localeCompare(b.subKey || '', undefined, { numeric: true });
  });
}

/* ---------- 편집 팝업 컴포넌트 ---------- */

function SATAMEditPopup({ designIssue, reqIssue, editItem, mode, dicts, onSave, onClose }) {
  const designData = dicts.byDesign[designIssue.key] || { risks: [], sensitivities: [], mitigations: [] };
  const cellInfo = getCellInfo(designIssue, reqIssue, dicts);
  const isAddMode = mode === 'add';
  const isReadonly = mode === 'readonly';

  // editItem: { id, type('R'|'S'|'M'), description, counterRisk } — 수정 대상 항목
  const TYPE_MAP = { R: 'Risk', S: 'Sensitivity', M: 'Mitigation' };
  const initialType = isAddMode ? 'Risk' : (editItem ? TYPE_MAP[editItem.type] : 'Risk');

  // 5.2 라디오 활성화 규칙 (add 모드면 모두 활성)
  const getEnabledTypes = () => {
    if (isAddMode) return { Sensitivity: true, Risk: true, Mitigation: true };
    if (!editItem) return { Sensitivity: true, Risk: true, Mitigation: true };

    const isResolved = editItem.type === 'R' && globalResolvedRiskIds && globalResolvedRiskIds.has(editItem.id);

    if (editItem.type === 'R' && !isResolved) {
      // mitigate 되지 않은 risk → Sensitivity로만 변경 가능
      return { Sensitivity: true, Risk: true, Mitigation: false };
    }
    if (editItem.type === 'S') {
      // Sensitivity → Risk로만 변경 가능
      return { Sensitivity: true, Risk: true, Mitigation: false };
    }
    // 나머지 (mitigated risk, mitigation 등) → 타입 변경 불가, description만 수정
    return { Sensitivity: false, Risk: false, Mitigation: false };
  };

  // globalResolvedRiskIds는 부모에서 전달받지 않으므로 직접 계산
  const globalResolvedRiskIds = useMemo(() => {
    const resolved = new Set();
    Object.values(dicts.byDesign).forEach((data) => {
      data.mitigations.forEach((m) => {
        m.counterRisk.forEach((cr) => {
          const [rId] = cr.split(':');
          resolved.add(rId);
        });
      });
    });
    return resolved;
  }, [dicts]);

  const enabledTypes = getEnabledTypes();

  const [selectedType, setSelectedType] = useState(initialType);
  const [description, setDescription] = useState(editItem ? editItem.description : '');

  // 해당 requirement를 대상으로 하는 모든 risk 목록
  const allReqRisks = (dicts.byQA[reqIssue.key] || { risks: [] }).risks;
  const [counterRiskChecked, setCounterRiskChecked] = useState(() => {
    if (editItem && editItem.type === 'M' && editItem.counterRisk) {
      const checked = {};
      allReqRisks.forEach((r) => {
        checked[r.id] = editItem.counterRisk.some((cr) => cr.startsWith(r.id + ':'));
      });
      return checked;
    }
    return {};
  });

  // 언더바 표시용
  const underlineMap = {
    Sensitivity: cellInfo.sensitivities.length > 0,
    Risk: cellInfo.risks.length > 0,
    Mitigation: cellInfo.mitigations.length > 0,
  };

  function getNextId(prefix) {
    // 전체 디자인의 R/S/M ID를 대상으로 최대값+1 산출
    let max = 0;
    Object.values(dicts.byDesign).forEach((data) => {
      const lists = { R: data.risks, S: data.sensitivities, M: data.mitigations };
      (lists[prefix] || []).forEach((item) => {
        const num = parseInt(item.id.replace(prefix, ''), 10);
        if (!isNaN(num) && num > max) max = num;
      });
    });
    return prefix + (max + 1);
  }

  function handleSave() {
    const newRisks = designData.risks.map((r) => ({ ...r, qa: [...r.qa], counterRisk: [...r.counterRisk] }));
    const newSens = designData.sensitivities.map((s) => ({ ...s, qa: [...s.qa], counterRisk: [...s.counterRisk] }));
    const newMiti = designData.mitigations.map((m) => ({ ...m, qa: [...m.qa], counterRisk: [...m.counterRisk] }));

    if (isAddMode) {
      // 추가 모드: 새 항목 생성
      if (selectedType === 'Risk') {
        newRisks.push({ id: getNextId('R'), qa: [reqIssue.key], counterRisk: [], description });
      } else if (selectedType === 'Sensitivity') {
        newSens.push({ id: getNextId('S'), qa: [reqIssue.key], counterRisk: [], description });
      } else if (selectedType === 'Mitigation') {
        const crList = Object.entries(counterRiskChecked).filter(([, v]) => v).map(([rId]) => `${rId}:${reqIssue.key}`);
        newMiti.push({ id: getNextId('M'), qa: [reqIssue.key], counterRisk: crList, description });
      }
    } else if (editItem) {
      // 5.3 수정 모드: 해당 ID의 내용 수정 + 타입 변경 처리
      const origType = editItem.type;
      const PREFIX_MAP = { Risk: 'R', Sensitivity: 'S', Mitigation: 'M' };
      const newTypeKey = PREFIX_MAP[selectedType];

      if (origType === newTypeKey) {
        // 같은 타입 → description/counterRisk만 수정
        const lists = { R: newRisks, S: newSens, M: newMiti };
        const item = lists[origType].find((i) => i.id === editItem.id);
        if (item) {
          item.description = description;
          if (origType === 'M') {
            item.counterRisk = Object.entries(counterRiskChecked).filter(([, v]) => v).map(([rId]) => `${rId}:${reqIssue.key}`);
          }
        }
      } else {
        // 타입 변경: 기존 항목에서 이 req를 제거하고, 새 타입에 추가
        const origLists = { R: newRisks, S: newSens, M: newMiti };
        const origItem = origLists[origType].find((i) => i.id === editItem.id);
        if (origItem) {
          origItem.qa = origItem.qa.filter((k) => k !== reqIssue.key);
        }
        // 빈 항목 제거
        const filterEmpty = (arr) => arr.filter((i) => i.qa.length > 0);
        if (origType === 'R') { origLists.R.splice(0, origLists.R.length, ...filterEmpty(origLists.R)); }
        if (origType === 'S') { origLists.S.splice(0, origLists.S.length, ...filterEmpty(origLists.S)); }

        // 새 타입에 추가
        if (selectedType === 'Risk') {
          newRisks.push({ id: getNextId('R'), qa: [reqIssue.key], counterRisk: [], description });
        } else if (selectedType === 'Sensitivity') {
          newSens.push({ id: getNextId('S'), qa: [reqIssue.key], counterRisk: [], description });
        } else if (selectedType === 'Mitigation') {
          const crList = Object.entries(counterRiskChecked).filter(([, v]) => v).map(([rId]) => `${rId}:${reqIssue.key}`);
          newMiti.push({ id: getNextId('M'), qa: [reqIssue.key], counterRisk: crList, description });
        }
      }
    }

    const data = {
      risk: serializeRST(newRisks),
      sensitivity: serializeRST(newSens),
      mitigation: serializeRST(newMiti),
    };
    onSave(designIssue.key, data);
  }

  return (
    <div className="satam-popup-overlay" onClick={onClose}>
      <div className="satam-popup" onClick={(e) => e.stopPropagation()}>
        {/* Design 정보 */}
        <div className="satam-popup-issue-box">
          <span className="satam-popup-subkey" style={{ background: SUBTYPE_COLORS[getSubtype(designIssue)] || SUBTYPE_COLORS.ADSN }}>
            {designIssue.subKey}
          </span>
          <span className="satam-popup-summary">{designIssue.summary}</span>
        </div>

        {/* Is */}
        <div className="satam-popup-section">
          <div className="satam-popup-label">Is</div>
          <div className="satam-popup-checks">
            {['Sensitivity', 'Risk', 'Mitigation'].map((t) => {
              const enabled = !isReadonly && (isAddMode || enabledTypes[t]);
              return (
                <label key={t} className={`satam-popup-check ${!enabled ? 'satam-popup-disabled' : ''}`}>
                  <input
                    type="radio"
                    name="satam-type"
                    checked={selectedType === t}
                    disabled={!enabled}
                    onChange={() => setSelectedType(t)}
                  />
                  <span style={underlineMap[t] ? { textDecoration: 'underline' } : undefined}>
                    {t}
                  </span>
                </label>
              );
            })}
          </div>
        </div>

        {/* Of */}
        <div className="satam-popup-section">
          <div className="satam-popup-label">Of</div>
        </div>

        {/* Requirement 정보 */}
        <div className="satam-popup-issue-box">
          <span className="satam-popup-subkey" style={{ background: SUBTYPE_COLORS[getSubtype(reqIssue)] }}>
            {reqIssue.subKey}
          </span>
          <span className="satam-popup-summary">{reqIssue.summary}</span>
        </div>

        {/* Counter-Risk (Mitigation인 경우에만) */}
        {selectedType === 'Mitigation' && (
          <div className="satam-popup-section">
            <div className="satam-popup-label">Counter-Risk:</div>
            <div className="satam-popup-checks">
              {allReqRisks.length > 0 ? allReqRisks.map((r) => (
                <label key={r.id} className="satam-popup-check">
                  <input
                    type="checkbox"
                    checked={!!counterRiskChecked[r.id]}
                    disabled={isReadonly}
                    onChange={(e) =>
                      setCounterRiskChecked((prev) => ({ ...prev, [r.id]: e.target.checked }))
                    }
                  />
                  <span style={counterRiskChecked[r.id] ? { textDecoration: 'line-through' } : undefined}>
                    {r.id}
                  </span>
                </label>
              )) : (
                <span className="satam-popup-empty">등록된 Risk 없음</span>
              )}
            </div>
          </div>
        )}

        {/* Description */}
        <div className="satam-popup-section">
          <div className="satam-popup-label">Description</div>
          <textarea
            className="satam-popup-textarea"
            value={description}
            readOnly={isReadonly}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
          />
        </div>

        {/* 버튼 */}
        <div className="satam-popup-footer">
          {!isReadonly && <button className="satam-popup-save" onClick={handleSave}>저장</button>}
          <button className="satam-popup-cancel" onClick={onClose}>{isReadonly ? '닫기' : '취소'}</button>
        </div>
      </div>
    </div>
  );
}

/* ---------- 셀 컨텍스트 메뉴 (5.4) ---------- */

function SATAMCellMenu({ x, y, hasEditItem, onAction, onClose, hasContains }) {
  const ref = React.useRef(null);

  useEffect(() => {
    function handleClick(e) {
      if (ref.current && !ref.current.contains(e.target)) onClose();
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  return (
    <div className="satam-ctx-menu" style={{ left: x, top: y }} ref={ref}>
      {hasEditItem && (
        <div className="satam-ctx-item" onClick={() => { onAction('edit'); onClose(); }}>수정</div>
      )}
      {hasEditItem && (
        <div className="satam-ctx-item" onClick={() => { onAction('delete'); onClose(); }}>삭제</div>
      )}
      <div className="satam-ctx-item" onClick={() => { onAction('add'); onClose(); }}>추가</div>
      <div className="satam-ctx-divider" />
      {!hasContains && (
        <div className="satam-ctx-item" onClick={() => { onAction('linkSolution'); onClose(); }}>Link as Solution</div>
      )}
      {hasContains && (
        <div className="satam-ctx-item" onClick={() => { onAction('unlinkSolution'); onClose(); }}>Remove from Solution</div>
      )}
    </div>
  );
}

/* ---------- Effects List (하단) ---------- */

function EffectsLegend() {
  return (
    <div className="satam-legend">
      <span className="satam-legend-title">Effects List</span>
      <span className="satam-legend-item">
        <span className="satam-legend-dot" style={{ background: '#333' }} /> Contains
      </span>
      <span className="satam-legend-item">
        <span className="satam-legend-mark" style={{ background: MARK_COLORS.S }}>S</span> Sensitivity
      </span>
      <span className="satam-legend-item">
        <span className="satam-legend-mark" style={{ background: MARK_COLORS.R }}>R</span> Risk
      </span>
      <span className="satam-legend-item">
        <span className="satam-legend-mark" style={{ background: MARK_COLORS.M }}>M</span> Mitigation
      </span>
    </div>
  );
}

/* ---------- Effects Description Table ---------- */

function EffectsTable({ designIssues, dicts, resolvedRiskIds }) {
  const rows = [];

  designIssues.forEach((d) => {
    const data = dicts.byDesign[d.key];
    if (!data) return;

    data.risks.forEach((r) => {
      rows.push({ designKey: d.subKey, id: r.id, type: 'Risk', description: r.description, resolved: resolvedRiskIds.has(r.id) });
    });
    data.sensitivities.forEach((s) => {
      rows.push({ designKey: d.subKey, id: s.id, type: 'Sensitivity', description: s.description, resolved: false });
    });
    data.mitigations.forEach((m) => {
      const crText = m.counterRisk.length > 0
        ? ` [Counter: ${m.counterRisk.join(', ')}]`
        : '';
      rows.push({ designKey: d.subKey, id: m.id, type: 'Mitigation', description: m.description + crText, resolved: false });
    });
  });

  if (rows.length === 0) return null;

  return (
    <div className="satam-effects-table">
      <table>
        <thead>
          <tr>
            <th>Counter Strategy</th>
            <th>Type</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i}>
              <td className={row.resolved ? 'satam-resolved' : ''}>{row.id}</td>
              <td>{row.type}</td>
              <td>{row.description}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ---------- 메인 SATAM 컴포넌트 ---------- */

const NAV_ITEMS = [
  { key: 'issues', label: 'Issues' },
  { key: 'relationMap', label: 'Relation Map' },
  { key: 'satam', label: 'SATAM' },
];

export default function SATAM({ project, onBack, currentPage, onNavigate }) {
  const [issues, setIssues] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [filterASR, setFilterASR] = useState(false);
  const [showReserved, setShowReserved] = useState(true);
  const [showFR, setShowFR] = useState(true);            // F: FR 포함 (기본 ON)
  const [onlyDecision, setOnlyDecision] = useState(false); // S: Decision 있는 디자인만
  const [showDDSN, setShowDDSN] = useState(true);       // D: Detailed Design 포함 (기본 ON)
  const [editCell, setEditCell] = useState(null); // { design, req, editItem, mode }
  const [cellMenu, setCellMenu] = useState(null); // { x, y, design, req, editItem }
  const [hoverCell, setHoverCell] = useState(null); // { row: reqKey, col: designKey }
  const [detailIssue, setDetailIssue] = useState(null);

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

  const handlePull = useCallback(async () => {
    try {
      await fetch(`${API_BASE}/Issue/fetch`, {
        headers: { XBody: JSON.stringify({ prjID: projectCode }) },
      });
      fetchIssues();
    } catch (err) {
      alert('Pull 실패: ' + err.message);
    }
  }, [projectCode, fetchIssues]);

  /* 디자인/요구사항 분리, 필터링, 정렬 */
  const designIssues = useMemo(() =>
    sortIssues(issues.filter((i) => {
      if (i.issueType === 'Architectueral Design') {
        if (!showReserved && (i.labels || []).includes('Reserved')) return false;
        if (onlyDecision && (!i.decision || i.decision === '')) return false;
        return true;
      }
      if (i.issueType === 'Detaied Design') {
        if (!showDDSN) return false;
        if (!showReserved && (i.labels || []).includes('Reserved')) return false;
        return true;
      }
      return false;
    })),
    [issues, showReserved, onlyDecision, showDDSN]
  );

  const reqIssues = useMemo(() =>
    sortIssues(issues.filter((i) => {
      if (i.issueType !== 'Requirement') return false;
      if (!showFR && i.reqtype === 'FR') return false;
      if (filterASR && !(i.labels || []).includes('ASR')) return false;
      if (!showReserved && (i.labels || []).includes('Reserved')) return false;
      return true;
    })),
    [issues, filterASR, showReserved, showFR]
  );

  /* 사전 빌드 */
  const dicts = useMemo(() => buildDictionaries(designIssues), [designIssues]);

  /* 전역 resolved Risk ID (모든 디자인의 mitigation counterRisk에서 수집) */
  const globalResolvedRiskIds = useMemo(() => {
    const resolved = new Set();
    designIssues.forEach((d) => {
      const data = dicts.byDesign[d.key];
      if (!data) return;
      data.mitigations.forEach((m) => {
        m.counterRisk.forEach((cr) => {
          const [rId] = cr.split(':');
          resolved.add(rId);
        });
      });
    });
    return resolved;
  }, [designIssues, dicts]);

  /* 이슈 추가 (RelationMap과 동일) */
  const SUBKEY_PREFIX = {
    QA: 'QA', CNST: 'CNST', FR: 'FR',
    ADSN: 'ADSN', DDSN: 'DDSN',
  };

  function getNextSubKey(subtype) {
    const prefix = (SUBKEY_PREFIX[subtype] || subtype) + '_';
    let maxNum = 0;
    issues.forEach((i) => {
      const sk = i.subKey || '';
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
      case 'FR': case 'QA': case 'CNST':
        issueType = 'Requirement'; reqtype = subtype; break;
      case 'ADSN': issueType = 'Architectueral Design'; break;
      case 'DDSN': issueType = 'Detaied Design'; break;
      default: issueType = subtype;
    }
    const subKey = getNextSubKey(subtype);
    try {
      await fetch(`${API_BASE}/Issue`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          projectKey: projectCode, issueType, reqtype, summary: '새 이슈',
          labels: (subtype === 'QA' || subtype === 'CNST') ? ['ASR'] : [],
          subKey,
        }),
      });
      fetchIssues();
    } catch (err) {
      alert('이슈 생성 실패: ' + err.message);
    }
  }

  /* 이슈 업데이트 */
  async function handleSave(key, data) {
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
    setEditCell(null);
  }

  /* contains 링크 생성 */
  async function handleCreateLink(fromKey, toKey) {
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

  /* contains 링크 삭제 */
  async function handleDeleteLink(linkId, fromKey, toKey) {
    try {
      await fetch(`${API_BASE}/Issue/link`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ linkId, fromKey, toKey }),
      });
      fetchIssues();
    } catch (err) {
      alert('연결 삭제 실패: ' + err.message);
    }
  }

  /* contains 링크 ID 찾기 */
  function findContainsLinkId(reqIssue, designKey) {
    for (const link of (reqIssue.issueLinks || [])) {
      const linkType = link.type?.name || '';
      if (!linkType.toLowerCase().includes('contains')) continue;
      const targetKey = link.inwardIssue?.key || link.outwardIssue?.key;
      if (targetKey === designKey) return link.id;
    }
    return null;
  }

  /* 5.4 셀 삭제: 해당 항목에서 req key를 제거 */
  async function handleDeleteItem(designIssue, reqKey, editItem) {
    const designData = dicts.byDesign[designIssue.key] || { risks: [], sensitivities: [], mitigations: [] };
    const newRisks = designData.risks.map((r) => ({ ...r, qa: [...r.qa], counterRisk: [...r.counterRisk] }));
    const newSens = designData.sensitivities.map((s) => ({ ...s, qa: [...s.qa], counterRisk: [...s.counterRisk] }));
    const newMiti = designData.mitigations.map((m) => ({ ...m, qa: [...m.qa], counterRisk: [...m.counterRisk] }));

    const lists = { R: newRisks, S: newSens, M: newMiti };
    const item = lists[editItem.type].find((i) => i.id === editItem.id);
    if (item) {
      item.qa = item.qa.filter((k) => k !== reqKey);
    }

    const data = {
      risk: serializeRST(newRisks.filter((r) => r.qa.length > 0)),
      sensitivity: serializeRST(newSens.filter((s) => s.qa.length > 0)),
      mitigation: serializeRST(newMiti.filter((m) => m.qa.length > 0)),
    };
    await handleSave(designIssue.key, data);
  }

  /* 5.4 컨텍스트 메뉴 액션 */
  function handleCellMenuAction(action) {
    if (!cellMenu) return;
    const { design, req, editItem } = cellMenu;

    if (action === 'edit' && editItem) {
      setEditCell({ design, req, editItem, mode: 'edit' });
    } else if (action === 'delete' && editItem) {
      handleDeleteItem(design, req.key, editItem);
    } else if (action === 'add') {
      setEditCell({ design, req, editItem: null, mode: 'add' });
    } else if (action === 'linkSolution') {
      handleCreateLink(req.key, design.key);
    } else if (action === 'unlinkSolution') {
      const linkId = findContainsLinkId(req, design.key);
      if (linkId) handleDeleteLink(linkId, req.key, design.key);
    }
    setCellMenu(null);
  }

  /* ---------- render ---------- */

  const barTitle = `${project.Name} (${projectCode})`;
  const nav = onNavigate ? NAV_ITEMS.map((n) => ({
    ...n, active: n.key === currentPage, handler: () => onNavigate(n.key),
  })) : [];

  if (loading) {
    return (
      <div className="satam-wrapper">
        <CommonBar
          title={barTitle}
          onBack={onBack}
          onPull={handlePull}
          onEditChange={setEditMode}
          nav={nav}
          options={[
            { label: 'A', active: filterASR, handler: () => setFilterASR((v) => !v) },
            { label: 'F', active: showFR, handler: () => setShowFR((v) => !v) },
            { label: 'S', active: onlyDecision, handler: () => setOnlyDecision((v) => !v) },
            { label: 'D', active: showDDSN, handler: () => setShowDDSN((v) => !v) },
            { label: 'R', active: !showReserved, handler: () => setShowReserved((v) => !v) },
          ]}
        />
        <div className="satam-loading">로딩 중...</div>
      </div>
    );
  }

  return (
    <div className="satam-wrapper">
      <CommonBar
        title={barTitle}
        onBack={onBack}
        onPull={handlePull}
        onEditChange={(mode) => { setEditMode(mode); if (!mode) setEditCell(null); }}
        nav={nav}
        options={[
          { label: 'A', active: filterASR, handler: () => setFilterASR((v) => !v) },
          { label: 'F', active: showFR, handler: () => setShowFR((v) => !v) },
          { label: 'S', active: onlyDecision, handler: () => setOnlyDecision((v) => !v) },
          { label: 'D', active: showDDSN, handler: () => setShowDDSN((v) => !v) },
          { label: 'R', active: !showReserved, handler: () => setShowReserved((v) => !v) },
        ]}
      />

      <div className="satam-content">
        {/* 프로젝트 라벨 */}
        <div className="satam-table-wrap">
          <table className="satam-table">
            <colgroup>
              <col className="satam-col-req" />
              {designIssues.map((d) => (
                <col key={d.key} className={`satam-col-design ${hoverCell && hoverCell.col === d.key ? 'satam-col-hl' : ''}`} />
              ))}
              {/* 라벨 확장용 여분 컬럼 */}
              <col className="satam-col-extra" />
            </colgroup>
            <thead>
              {/* 계단식 디자인 헤더: 디자인 수만큼 행 생성 */}
              {designIssues.map((d, idx) => {
                const isLast = idx === designIssues.length - 1;
                return (
                  <tr key={d.key} className="satam-stair-tr">
                    {/* 첫 번째 디자인 행에서만 rowSpan으로 좌상단 빈칸 */}
                    {idx === 0 && (
                      <th className="satam-corner" rowSpan={designIssues.length}>
                      {editMode && (
                        <div className="satam-corner-top-right">
                          <button className="satam-corner-btn" onClick={() => addIssue('ADSN')}>+ADSN</button>
                          <button className="satam-corner-btn" onClick={() => addIssue('DDSN')}>+DDSN</button>
                        </div>
                      )}
                      <span className="satam-corner-label">Project : {project.Name}</span>
                      {editMode && (
                        <div className="satam-corner-bottom-left">
                          <button className="satam-corner-btn" onClick={() => addIssue('QA')}>+QA</button>
                          <button className="satam-corner-btn" onClick={() => addIssue('FR')}>+FR</button>
                          <button className="satam-corner-btn" onClick={() => addIssue('CNST')}>+CNST</button>
                        </div>
                      )}
                    </th>
                    )}
                    {/* 이 디자인 컬럼 앞의 빈 셀들 (개별 셀로 테두리 표시) */}
                    {Array.from({ length: idx }, (_, ci) => (
                      <td key={ci} className="satam-stair-empty-cell" />
                    ))}
                    {/* 이 디자인의 라벨: 남은 디자인 컬럼 + 여분 컬럼까지 차지 */}
                    <td
                      colSpan={designIssues.length - idx + 1}
                      className={`satam-stair-cell ${isLast ? 'satam-stair-cell-last' : ''}`}
                    >
                      <div className="satam-stair-label">
                        {(() => {
                          const sub = getSubtype(d);
                          const isADSNnoDecision = sub === 'ADSN' && (!d.decision || d.decision === '');
                          const bg = isADSNnoDecision ? 'lightgray' : (SUBTYPE_COLORS[sub] || SUBTYPE_COLORS.ADSN);
                          const clr = isADSNnoDecision ? '#000' : '#fff';
                          return (
                            <span
                              className="satam-stair-subkey satam-subkey-click"
                              style={{ background: bg, color: clr, fontWeight: isADSNnoDecision ? 'normal' : 700 }}
                              onClick={() => setDetailIssue(d)}
                            >
                              {d.subKey}
                            </span>
                          );
                        })()}
                        <span className="satam-stair-summary">{d.summary}</span>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </thead>
            <tbody>
              {reqIssues.map((req) => {
                const subtype = getSubtype(req);
                const hasASR = (req.labels || []).includes('ASR');
                const isFRnoASR = subtype === 'FR' && !hasASR;
                const bgColor = isFRnoASR ? '#ffb4b4' : (SUBTYPE_COLORS[subtype] || '#999');
                const textColor = isFRnoASR ? '#000' : '#fff';
                return (
                  <tr key={req.key}>
                    {/* 요구사항 행 헤더 */}
                    <td className={`satam-row-header ${hoverCell && hoverCell.row === req.key ? 'satam-row-hl' : ''}`}>
                      <span
                        className="satam-row-subkey satam-subkey-click"
                        style={{ background: bgColor, color: textColor, fontWeight: hasASR ? 700 : 'normal' }}
                        onClick={() => setDetailIssue(req)}
                      >
                        {req.subKey}
                      </span>
                      <span className="satam-row-summary">{req.summary}</span>
                    </td>
                    {/* 교차 셀 */}
                    {designIssues.map((d) => {
                      const cell = getCellInfo(d, req, dicts);
                      const hasRST = cell.risks.length > 0 || cell.sensitivities.length > 0 || cell.mitigations.length > 0;

                      // 항목별 개별 마크 (R > S > M 우선순위, 같은 타입도 개별 표시)
                      const marks = [
                        ...cell.risks.map((item) => ({ color: MARK_COLORS.R, item, type: 'R' })),
                        ...cell.sensitivities.map((item) => ({ color: MARK_COLORS.S, item, type: 'S' })),
                        ...cell.mitigations.map((item) => ({ color: MARK_COLORS.M, item, type: 'M' })),
                      ];

                      const renderMark = (mark) => (
                        <span
                          key={mark.item.id}
                          className="satam-mark"
                          style={{
                            background: mark.color,
                            borderWidth: cell.hasContains ? 3 : 0,
                            borderColor: '#333',
                            borderStyle: 'solid',
                          }}
                        >
                          <span className={mark.type === 'R' && globalResolvedRiskIds.has(mark.item.id) ? 'satam-resolved' : ''}>
                            {mark.item.id}
                          </span>
                        </span>
                      );

                      const hasContent = marks.length > 0 || cell.hasContains;

                      return (
                        <td
                          key={d.key}
                          className={`satam-cell ${hoverCell && hoverCell.row === req.key ? 'satam-cell-hl' : ''}`}
                          onMouseEnter={hasContent ? () => setHoverCell({ row: req.key, col: d.key }) : undefined}
                          onMouseLeave={hasContent ? () => setHoverCell(null) : undefined}
                        >
                          {editMode ? (
                            <div className="satam-cell-edit">
                              {marks.length > 0 ? marks.map((mark) => (
                                <button
                                  key={mark.item.id}
                                  className="satam-cell-btn"
                                  onClick={(e) => {
                                    setCellMenu({
                                      x: e.clientX, y: e.clientY,
                                      design: d, req,
                                      editItem: { id: mark.item.id, type: mark.type, description: mark.item.description, counterRisk: mark.item.counterRisk || [] },
                                    });
                                  }}
                                >
                                  {renderMark(mark)}
                                </button>
                              )) : (
                                <button
                                  className="satam-cell-btn"
                                  onClick={(e) => {
                                    setCellMenu({
                                      x: e.clientX, y: e.clientY,
                                      design: d, req,
                                      editItem: null,
                                    });
                                  }}
                                >
                                  {cell.hasContains && <span className="satam-dot-contains" />}
                                </button>
                              )}
                            </div>
                          ) : (
                            <div className="satam-cell-marks">
                              {marks.map((mark) => (
                                <span
                                  key={mark.item.id}
                                  className="satam-mark-click"
                                  onClick={() => setEditCell({
                                    design: d, req,
                                    editItem: { id: mark.item.id, type: mark.type, description: mark.item.description, counterRisk: mark.item.counterRisk || [] },
                                    mode: 'readonly',
                                  })}
                                >
                                  {renderMark(mark)}
                                </span>
                              ))}
                              {!hasRST && cell.hasContains && (
                                <span className="satam-dot-contains" />
                              )}
                            </div>
                          )}
                        </td>
                      );
                    })}
                    {/* 여분 컬럼 빈 셀 */}
                    <td className="satam-cell-extra" />
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        {/* Effects 범례 */}
        <EffectsLegend />

        {/* Effects 상세 테이블 */}
        <EffectsTable designIssues={designIssues} dicts={dicts} resolvedRiskIds={globalResolvedRiskIds} />
      </div>

      {/* 상태바 */}
      <div className="satam-statusbar">
        Design {designIssues.length}건 × Requirement {reqIssues.length}건
      </div>

      {/* 편집 팝업 */}
      {editCell && (
        <SATAMEditPopup
          designIssue={editCell.design}
          reqIssue={editCell.req}
          editItem={editCell.editItem}
          mode={editCell.mode}
          dicts={dicts}
          onSave={handleSave}
          onClose={() => setEditCell(null)}
        />
      )}

      {/* 5.4 셀 컨텍스트 메뉴 */}
      {cellMenu && (
        <SATAMCellMenu
          x={cellMenu.x}
          y={cellMenu.y}
          hasEditItem={!!cellMenu.editItem}
          hasContains={(() => {
            const ci = getCellInfo(cellMenu.design, cellMenu.req, dicts);
            return ci.hasContains;
          })()}
          onAction={handleCellMenuAction}
          onClose={() => setCellMenu(null)}
        />
      )}

      {/* Issue Detail Dialog */}
      {detailIssue && (
        <IssueDetailDialog
          issue={detailIssue}
          editMode={editMode}
          onClose={() => setDetailIssue(null)}
          onSave={handleSave}
        />
      )}
    </div>
  );
}
