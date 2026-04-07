import React, { useEffect, useState, useCallback } from 'react';
import CommonBar from '../../components/CommonBar';
import IssueDetailDialog from './IssueDetailDialog';
import config from '../../config';
import './IssueList.css';

const API_BASE = config.GOJIRA_API;

const STATUS_OPTIONS = ['To Do', 'In Progress', 'Done'];
const PRIORITY_OPTIONS = ['Highest', 'High', 'Medium', 'Low', 'Lowest'];

const NAV_ITEMS = [
  { key: 'docs', label: 'Docs' },
  { key: 'issues', label: 'Issues' },
  { key: 'relationMap', label: 'Relation Map' },
  { key: 'satam', label: 'SATAM' },
];

// 옵션 키 → issueType 매핑
const TYPE_FILTER = {
  V: ['VOS'],
  F: ['Feature'],
  R: ['Requirement'],
  A: ['Architectural Design'],
  D: ['Detailed Design'],
};

export default function IssueList({ project, user, onBack, currentPage, onNavigate }) {
  const [issues, setIssues] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [sortKey, setSortKey] = useState('key');
  const [sortAsc, setSortAsc] = useState(true);
  const [detailIssue, setDetailIssue] = useState(null);
  const [assistLoading, setAssistLoading] = useState(false);
  const [assistResult, setAssistResult] = useState(null);

  // 필터 옵션 (sessionStorage에서 복원, 기본 모두 ON)
  const [showV, setShowV] = useState(() => sessionStorage.getItem('gojira_il_V') !== 'false');
  const [showF, setShowF] = useState(() => sessionStorage.getItem('gojira_il_F') !== 'false');
  const [showR, setShowR] = useState(() => sessionStorage.getItem('gojira_il_R') !== 'false');
  const [showA, setShowA] = useState(() => sessionStorage.getItem('gojira_il_A') !== 'false');
  const [showD, setShowD] = useState(() => sessionStorage.getItem('gojira_il_D') !== 'false');
  useEffect(() => { sessionStorage.setItem('gojira_il_V', String(showV)); }, [showV]);
  useEffect(() => { sessionStorage.setItem('gojira_il_F', String(showF)); }, [showF]);
  useEffect(() => { sessionStorage.setItem('gojira_il_R', String(showR)); }, [showR]);
  useEffect(() => { sessionStorage.setItem('gojira_il_A', String(showA)); }, [showA]);
  useEffect(() => { sessionStorage.setItem('gojira_il_D', String(showD)); }, [showD]);

  const projectCode = project.jiraCodes?.main?.jiraCode || '';

  const fetchIssues = useCallback(() => {
    setLoading(true);
    return fetch(`${API_BASE}/Issue`, { headers: { XBody: JSON.stringify({ prjID: projectCode, infoType: '' }) } })
      .then((res) => res.json())
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

  // 허용된 issueType 목록
  const allowedTypes = [
    ...(showV ? TYPE_FILTER.V : []),
    ...(showF ? TYPE_FILTER.F : []),
    ...(showR ? TYPE_FILTER.R : []),
    ...(showA ? TYPE_FILTER.A : []),
    ...(showD ? TYPE_FILTER.D : []),
  ];

  // Filter
  const filtered = issues.filter((i) => allowedTypes.includes(i.issueType));

  // Sort
  const sorted = [...filtered].sort((a, b) => {
    let va = a[sortKey] || '';
    let vb = b[sortKey] || '';
    if (typeof va === 'object') va = va.name || '';
    if (typeof vb === 'object') vb = vb.name || '';
    const cmp = String(va).localeCompare(String(vb), undefined, { numeric: true });
    return sortAsc ? cmp : -cmp;
  });

  function handleSort(key) {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(true); }
  }

  function sortIndicator(key) {
    if (sortKey !== key) return '';
    return sortAsc ? ' \u25B2' : ' \u25BC';
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

  async function handleDeleteIssue(issue) {
    const isOwn = issue.key.startsWith(projectCode + '-');
    const msg = isOwn
      ? `"${issue.key}" 이슈를 완전히 삭제하시겠습니까?`
      : `"${issue.key}"는 타 과제 이슈입니다. 이 과제 목록에서만 제거하시겠습니까?`;
    if (!window.confirm(msg)) return;
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

  function handleSelectChange(issue, field, newName) {
    const oldVal = issue[field];
    const oldName = oldVal && typeof oldVal === 'object' ? oldVal.name : oldVal || '';
    if (newName === oldName) return;
    const newVal = { ...(typeof oldVal === 'object' ? oldVal : {}), name: newName };
    fetch(`${API_BASE}/Issue`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key: issue.key, data: { [field]: newVal } }),
    })
      .then(() => setIssues((prev) => prev.map((i) => (i.key === issue.key ? { ...i, [field]: newVal } : i))))
      .catch((err) => alert('수정 실패: ' + err.message));
  }

  const columns = [
    { key: 'key', label: 'Key', width: 120 },
    { key: 'subkey', label: 'SubKey', width: 100 },
    { key: 'issueType', label: 'Type', width: 140 },
    { key: 'summary', label: 'Summary', width: 0 },
    { key: 'status', label: 'Status', width: 100 },
    { key: 'priority', label: 'Priority', width: 90 },
  ];

  async function handleAssistGenerate(issueType) {
    if (assistLoading) return;
    setAssistLoading(true);
    setAssistResult(null);
    try {
      const res = await fetch(`${API_BASE}/Agent/assist`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user || '' },
        body: JSON.stringify({
          action: 'generate',
          context: { page: 'issues', projectCode, issueType },
        }),
      });
      const data = await res.json();
      if (res.ok && data.result?.suggestions?.length > 0) {
        setAssistResult({ issueType, suggestions: data.result.suggestions });
      } else {
        alert(data.error || '이슈 제안을 생성할 수 없습니다.');
      }
    } catch (err) {
      alert('Agent Assist 오류: ' + err.message);
    }
    setAssistLoading(false);
  }

  async function handleAssistApply() {
    if (!assistResult?.suggestions) return;
    setAssistLoading(true);
    let created = 0;
    for (const s of assistResult.suggestions) {
      try {
        await fetch(`${API_BASE}/Agent/chat`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-User-Id': user || '' },
          body: JSON.stringify({
            message: `${projectCode}에 ${s.issueType || assistResult.issueType} 이슈를 생성해줘. 제목: ${s.summary}. 설명: ${s.description || ''}`,
          }),
        });
        created++;
      } catch {}
    }
    setAssistResult(null);
    setAssistLoading(false);
    // 이슈 목록 새로고침
    if (created > 0) {
      alert(`${created}건의 이슈가 생성되었습니다.`);
      handlePull();
    }
  }

  function cellValue(issue, col) {
    const v = issue[col.key];
    if (col.key === 'issueType' && v === 'Requirement' && issue.reqtype) {
      return issue.reqtype;
    }
    if (v && typeof v === 'object') return v.name || '';
    return v || '';
  }

  const barTitle = `${project.name} (${projectCode})`;
  const nav = onNavigate ? NAV_ITEMS.map((n) => ({
    ...n, active: n.key === currentPage, handler: () => onNavigate(n.key),
  })) : [];

  return (
    <div className="il-wrapper">
      <CommonBar
        title={barTitle}
        onBack={onBack}
        onPull={handlePull}
        onEditChange={setEditMode}
        nav={nav}
        options={[
          { label: 'V', active: showV, handler: () => setShowV((v) => !v) },
          { label: 'F', active: showF, handler: () => setShowF((v) => !v) },
          { label: 'R', active: showR, handler: () => setShowR((v) => !v) },
          { label: 'A', active: showA, handler: () => setShowA((v) => !v) },
          { label: 'D', active: showD, handler: () => setShowD((v) => !v) },
        ]}
      />

      <div className="il-grid-container">
        {loading ? (
          <div className="il-loading">로딩 중...</div>
        ) : (
          <table className="il-table">
            <thead>
              <tr>
                {editMode && <th style={{ width: 40 }} />}
                {columns.map((col) => (
                  <th
                    key={col.key}
                    style={col.width ? { width: col.width } : {}}
                    onClick={() => handleSort(col.key)}
                  >
                    {col.label}{sortIndicator(col.key)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sorted.length === 0 ? (
                <tr>
                  <td colSpan={columns.length + (editMode ? 1 : 0)} className="il-empty">
                    이슈가 없습니다. Pull 버튼으로 가져오세요.
                  </td>
                </tr>
              ) : (
                sorted.map((issue, i) => (
                  <tr key={issue.key || i}>
                    {editMode && (
                      <td className="il-delete-cell">
                        <button
                          className="il-delete-btn"
                          onClick={() => handleDeleteIssue(issue)}
                          title="삭제"
                        >X</button>
                      </td>
                    )}
                    {columns.map((col) => {
                      // Key, SubKey 클릭 → 상세 팝업
                      if (col.key === 'key' || col.key === 'subkey') {
                        return (
                          <td key={col.key}>
                            <span
                              className="il-clickable"
                              onClick={() => setDetailIssue(issue)}
                            >
                              {cellValue(issue, col)}
                            </span>
                          </td>
                        );
                      }
                      // Summary 편집모드
                      if (col.key === 'summary' && editMode) {
                        return (
                          <td key={col.key}>
                            <div
                              className="il-summary-edit"
                              contentEditable
                              suppressContentEditableWarning
                              onBlur={(e) => handleSummaryBlur(issue, e.target.textContent)}
                            >
                              {issue.summary || ''}
                            </div>
                          </td>
                        );
                      }
                      // Status 편집모드
                      if (col.key === 'status' && editMode) {
                        const cur = issue.status && typeof issue.status === 'object' ? issue.status.name : issue.status || '';
                        return (
                          <td key={col.key}>
                            <select
                              className="il-select"
                              value={cur}
                              onChange={(e) => handleSelectChange(issue, 'status', e.target.value)}
                            >
                              {STATUS_OPTIONS.map((o) => <option key={o} value={o}>{o}</option>)}
                            </select>
                          </td>
                        );
                      }
                      // Priority 편집모드
                      if (col.key === 'priority' && editMode) {
                        const cur = issue.priority && typeof issue.priority === 'object' ? issue.priority.name : issue.priority || '';
                        return (
                          <td key={col.key}>
                            <select
                              className="il-select"
                              value={cur}
                              onChange={(e) => handleSelectChange(issue, 'priority', e.target.value)}
                            >
                              {PRIORITY_OPTIONS.map((o) => <option key={o} value={o}>{o}</option>)}
                            </select>
                          </td>
                        );
                      }
                      return <td key={col.key}>{cellValue(issue, col)}</td>;
                    })}
                  </tr>
                ))
              )}
            </tbody>
          </table>
        )}
      </div>

      <div className="il-statusbar">
        <span>총 {sorted.length}건</span>
        {editMode && (
          <div className="il-assist-btns">
            <button className="il-assist-btn" onClick={() => handleAssistGenerate('VOS')} disabled={assistLoading}>VOS 자동생성</button>
            <button className="il-assist-btn" onClick={() => handleAssistGenerate('Feature')} disabled={assistLoading}>Feature 자동생성</button>
            <button className="il-assist-btn" onClick={() => handleAssistGenerate('Requirement')} disabled={assistLoading}>Req 자동생성</button>
            <button className="il-assist-btn" onClick={() => handleAssistGenerate('Architectural Design')} disabled={assistLoading}>ADSN 자동생성</button>
            {assistLoading && <span className="il-assist-loading">생성 중...</span>}
          </div>
        )}
      </div>

      {/* Assist Preview */}
      {assistResult && (
        <div className="il-assist-preview">
          <div className="il-assist-preview-header">
            <span>Agent 제안: {assistResult.issueType} {assistResult.suggestions.length}건</span>
            <div>
              <button className="il-assist-btn il-assist-apply" onClick={handleAssistApply}>전체 적용</button>
              <button className="il-assist-btn" onClick={() => setAssistResult(null)}>닫기</button>
            </div>
          </div>
          <div className="il-assist-preview-list">
            {assistResult.suggestions.map((s, i) => (
              <div key={i} className="il-assist-preview-item">
                <strong>{s.issueType || assistResult.issueType}</strong>: {s.summary}
                {s.description && <p className="il-assist-desc">{s.description}</p>}
              </div>
            ))}
          </div>
        </div>
      )}

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
