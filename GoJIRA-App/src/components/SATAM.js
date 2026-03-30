import React, { useEffect, useState, useCallback, useMemo } from 'react';
import CommonBar from './CommonBar';
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
  S: '#4cafe8',   // Sensitivity - 파랑
  M: '#ff9a1e',   // Mitigation(Trade-off) - 주황
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

function SATAMEditPopup({ designIssue, reqIssue, dicts, onSave, onClose }) {
  const designData = dicts.byDesign[designIssue.key] || { risks: [], sensitivities: [], mitigations: [] };
  const cellInfo = getCellInfo(designIssue, reqIssue, dicts);

  // 현재 선택된 타입 결정
  const getInitialType = () => {
    if (cellInfo.risks.length > 0) return 'Risk';
    if (cellInfo.sensitivities.length > 0) return 'Sensitivity';
    if (cellInfo.mitigations.length > 0) return 'Mitigation';
    return 'Nothing';
  };

  const [selectedType, setSelectedType] = useState(getInitialType);
  const [description, setDescription] = useState(() => {
    if (cellInfo.risks.length > 0) return cellInfo.risks[0].description;
    if (cellInfo.sensitivities.length > 0) return cellInfo.sensitivities[0].description;
    if (cellInfo.mitigations.length > 0) return cellInfo.mitigations[0].description;
    return '';
  });

  // Mitigation의 Counter-Risk 체크 상태
  const reqRisks = designData.risks.filter((r) => r.qa.includes(reqIssue.key));
  const [counterRiskChecked, setCounterRiskChecked] = useState(() => {
    if (cellInfo.mitigations.length > 0) {
      const m = cellInfo.mitigations[0];
      const checked = {};
      reqRisks.forEach((r) => {
        checked[r.id] = m.counterRisk.some((cr) => cr.startsWith(r.id + ':'));
      });
      return checked;
    }
    return {};
  });

  function getNextId(prefix, existing) {
    let max = 0;
    existing.forEach((item) => {
      const num = parseInt(item.id.replace(prefix, ''), 10);
      if (!isNaN(num) && num > max) max = num;
    });
    return prefix + (max + 1);
  }

  function handleSave() {
    // 기존 데이터를 복사
    let newRisks = [...designData.risks];
    let newSens = [...designData.sensitivities];
    let newMiti = [...designData.mitigations];

    // 먼저 기존 항목에서 이 requirement를 제거
    newRisks.forEach((r) => { r.qa = r.qa.filter((k) => k !== reqIssue.key); });
    newSens.forEach((s) => { s.qa = s.qa.filter((k) => k !== reqIssue.key); });
    newMiti.forEach((m) => {
      m.qa = m.qa.filter((k) => k !== reqIssue.key);
      m.counterRisk = m.counterRisk.filter((cr) => !cr.endsWith(':' + reqIssue.key));
    });

    // 빈 항목 제거
    newRisks = newRisks.filter((r) => r.qa.length > 0);
    newSens = newSens.filter((s) => s.qa.length > 0);
    newMiti = newMiti.filter((m) => m.qa.length > 0);

    if (selectedType === 'Risk') {
      // 기존 같은 description의 risk가 있으면 거기에 추가, 없으면 새로 생성
      let found = newRisks.find((r) => r.description === description);
      if (found) {
        found.qa.push(reqIssue.key);
      } else {
        newRisks.push({
          id: getNextId('R', newRisks),
          qa: [reqIssue.key],
          counterRisk: [],
          description,
        });
      }
    } else if (selectedType === 'Sensitivity') {
      let found = newSens.find((s) => s.description === description);
      if (found) {
        found.qa.push(reqIssue.key);
      } else {
        newSens.push({
          id: getNextId('S', newSens),
          qa: [reqIssue.key],
          counterRisk: [],
          description,
        });
      }
    } else if (selectedType === 'Mitigation') {
      const crList = Object.entries(counterRiskChecked)
        .filter(([, v]) => v)
        .map(([rId]) => `${rId}:${reqIssue.key}`);
      let found = newMiti.find((m) => m.description === description);
      if (found) {
        found.qa.push(reqIssue.key);
        found.counterRisk = [...found.counterRisk, ...crList];
      } else {
        newMiti.push({
          id: getNextId('M', newMiti),
          qa: [reqIssue.key],
          counterRisk: crList,
          description,
        });
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
          <span className="satam-popup-subkey" style={{ background: SUBTYPE_COLORS.ADSN }}>
            {designIssue.subKey}
          </span>
          <span className="satam-popup-summary">{designIssue.summary}</span>
        </div>

        {/* Is */}
        <div className="satam-popup-section">
          <div className="satam-popup-label">Is</div>
          <div className="satam-popup-checks">
            {['Sensitivity', 'Risk', 'Mitigation', 'Nothing'].map((t) => (
              <label key={t} className="satam-popup-check">
                <input
                  type="radio"
                  name="satam-type"
                  checked={selectedType === t}
                  onChange={() => setSelectedType(t)}
                />
                {t}
              </label>
            ))}
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
        {selectedType === 'Mitigation' && reqRisks.length > 0 && (
          <div className="satam-popup-section">
            <div className="satam-popup-label">Counter-Risk:</div>
            <div className="satam-popup-checks">
              {reqRisks.map((r) => (
                <label key={r.id} className="satam-popup-check">
                  <input
                    type="checkbox"
                    checked={!!counterRiskChecked[r.id]}
                    onChange={(e) =>
                      setCounterRiskChecked((prev) => ({ ...prev, [r.id]: e.target.checked }))
                    }
                  />
                  <span style={counterRiskChecked[r.id] ? { textDecoration: 'line-through' } : undefined}>
                    {r.id}
                  </span>
                </label>
              ))}
            </div>
          </div>
        )}

        {/* Description */}
        {selectedType !== 'Nothing' && (
          <div className="satam-popup-section">
            <div className="satam-popup-label">Description</div>
            <textarea
              className="satam-popup-textarea"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
            />
          </div>
        )}

        {/* 버튼 */}
        <div className="satam-popup-footer">
          <button className="satam-popup-save" onClick={handleSave}>저장</button>
          <button className="satam-popup-cancel" onClick={onClose}>취소</button>
        </div>
      </div>
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
        <span className="satam-legend-mark" style={{ background: MARK_COLORS.M }}>M</span> Mitigation / Trade-off
      </span>
    </div>
  );
}

/* ---------- Effects Description Table ---------- */

function EffectsTable({ designIssues, dicts }) {
  const rows = [];

  designIssues.forEach((d) => {
    const data = dicts.byDesign[d.key];
    if (!data) return;

    data.risks.forEach((r) => {
      rows.push({ designKey: d.subKey, id: r.id, type: 'Risk', description: r.description });
    });
    data.sensitivities.forEach((s) => {
      rows.push({ designKey: d.subKey, id: s.id, type: 'Sensitivity', description: s.description });
    });
    data.mitigations.forEach((m) => {
      // counterRisk 해결된 risk 표시
      const crText = m.counterRisk.length > 0
        ? ` [Counter: ${m.counterRisk.join(', ')}]`
        : '';
      rows.push({ designKey: d.subKey, id: m.id, type: 'TradeOff/Solution', description: m.description + crText });
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
              <td>{row.id}</td>
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

export default function SATAM({ project, onBack }) {
  const [issues, setIssues] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [filterASR, setFilterASR] = useState(false);
  const [showReserved, setShowReserved] = useState(true);
  const [editCell, setEditCell] = useState(null); // { design, req }

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
      if (i.issueType !== 'Architectueral Design') return false;
      if (!showReserved && (i.labels || []).includes('Reserved')) return false;
      return true;
    })),
    [issues, showReserved]
  );

  const reqIssues = useMemo(() =>
    sortIssues(issues.filter((i) => {
      if (i.issueType !== 'Requirement') return false;
      if (filterASR && !(i.labels || []).includes('ASR')) return false;
      if (!showReserved && (i.labels || []).includes('Reserved')) return false;
      return true;
    })),
    [issues, filterASR, showReserved]
  );

  /* 사전 빌드 */
  const dicts = useMemo(() => buildDictionaries(designIssues), [designIssues]);

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

  /* ---------- render ---------- */

  const barTitle = `${project.Name} (${projectCode}) - SATAM`;

  if (loading) {
    return (
      <div className="satam-wrapper">
        <CommonBar
          title={barTitle}
          onBack={onBack}
          onPull={handlePull}
          onEditChange={setEditMode}
          options={[
            { label: 'A', active: filterASR, handler: () => setFilterASR((v) => !v) },
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
        options={[
          { label: 'A', active: filterASR, handler: () => setFilterASR((v) => !v) },
          { label: 'R', active: !showReserved, handler: () => setShowReserved((v) => !v) },
        ]}
      />

      <div className="satam-content">
        {/* 프로젝트 라벨 */}
        <div className="satam-project-label">Project : {project.Name}</div>

        <div className="satam-table-wrap">
          <table className="satam-table">
            <colgroup>
              <col className="satam-col-req" />
              {designIssues.map((d) => (
                <col key={d.key} className="satam-col-design" />
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
                      <th className="satam-corner" rowSpan={designIssues.length} />
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
                        <span className="satam-stair-subkey" style={{ background: SUBTYPE_COLORS.ADSN }}>
                          {d.subKey}
                        </span>
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
                const color = SUBTYPE_COLORS[subtype] || '#999';
                return (
                  <tr key={req.key}>
                    {/* 요구사항 행 헤더 */}
                    <td className="satam-row-header">
                      <span className="satam-row-subkey" style={{ background: color }}>
                        {req.subKey}
                      </span>
                      <span className="satam-row-summary">{req.summary}</span>
                    </td>
                    {/* 교차 셀 */}
                    {designIssues.map((d) => {
                      const cell = getCellInfo(d, req, dicts);
                      const hasRST = cell.risks.length > 0 || cell.sensitivities.length > 0 || cell.mitigations.length > 0;
                      const typeCount = [cell.risks.length > 0, cell.sensitivities.length > 0, cell.mitigations.length > 0].filter(Boolean).length;

                      // 우선순위: Risk > Sensitivity > Trade-off
                      let primaryColor = null;
                      if (cell.risks.length > 0) {
                        primaryColor = MARK_COLORS.R;
                      } else if (cell.sensitivities.length > 0) {
                        primaryColor = MARK_COLORS.S;
                      } else if (cell.mitigations.length > 0) {
                        primaryColor = MARK_COLORS.M;
                      }

                      // Counter-Risk로 해결된 risk인지 확인
                      const resolvedRiskIds = new Set();
                      cell.mitigations.forEach((m) => {
                        m.counterRisk.forEach((cr) => {
                          const [rId] = cr.split(':');
                          resolvedRiskIds.add(rId);
                        });
                      });

                      return (
                        <td key={d.key} className="satam-cell">
                          {editMode ? (
                            <button
                              className="satam-cell-btn"
                              onClick={() => setEditCell({ design: d, req })}
                            >
                              {hasRST && (
                                <span className="satam-mark" style={{ background: primaryColor, borderWidth: cell.hasContains ? 3 : 0, borderColor: '#333', borderStyle: 'solid' }}>
                                  {typeCount >= 2 && <span className="satam-badge" />}
                                  {cell.risks.map((r) => (
                                    <span key={r.id} className={resolvedRiskIds.has(r.id) ? 'satam-resolved' : ''}>
                                      {r.id}
                                    </span>
                                  ))}
                                  {cell.sensitivities.map((s) => <span key={s.id}>{s.id}</span>)}
                                  {cell.mitigations.map((m) => <span key={m.id}>{m.id}</span>)}
                                </span>
                              )}
                              {!hasRST && cell.hasContains && (
                                <span className="satam-dot-contains" />
                              )}
                            </button>
                          ) : (
                            <>
                              {hasRST && (
                                <span
                                  className="satam-mark"
                                  style={{
                                    background: primaryColor,
                                    borderWidth: cell.hasContains ? 3 : 0,
                                    borderColor: '#333',
                                    borderStyle: 'solid',
                                  }}
                                >
                                  {typeCount >= 2 && <span className="satam-badge" />}
                                  {cell.risks.map((r) => (
                                    <span key={r.id} className={resolvedRiskIds.has(r.id) ? 'satam-resolved' : ''}>
                                      {r.id}
                                    </span>
                                  ))}
                                  {cell.sensitivities.map((s) => <span key={s.id}>{s.id}</span>)}
                                  {cell.mitigations.map((m) => <span key={m.id}>{m.id}</span>)}
                                </span>
                              )}
                              {!hasRST && cell.hasContains && (
                                <span className="satam-dot-contains" />
                              )}
                            </>
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
        <EffectsTable designIssues={designIssues} dicts={dicts} />
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
          dicts={dicts}
          onSave={handleSave}
          onClose={() => setEditCell(null)}
        />
      )}
    </div>
  );
}
