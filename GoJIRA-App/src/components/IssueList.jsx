import React, { useEffect, useState, useCallback } from 'react';
import CommonBar from '@platform/web-ui/components/CommonBar';
import IssueDetailDialog from './IssueDetailDialog';
import config from '../config';
import './IssueList.css';
import { authFetch } from '../platform/auth';
import useIssueSource from '../hooks/useIssueSource';

const API_BASE = config.API_BASE;

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
  const projectCode = project.jiraCodes?.main?.jiraCode || '';
  // 웹 이슈 뷰 이원화 Phase 1 — 공식본(main/Mongo) ↔ 내 작업본(로컬 런처) 전환.
  const { source, setSource, sourceAvailable, issues, loading, refetch, localMeta } =
    useIssueSource({ projectCode, user });
  // 웹은 이슈 조회 전용 (local-agent-issue-model.md §3.3). 편집·생성·삭제는 로컬
  // (런처→VS Code+CLI)로 이전 → editMode 는 항상 false, 편집 토글 미노출.
  const [editMode] = useState(false);
  const [sortKey, setSortKey] = useState('key');
  const [sortAsc, setSortAsc] = useState(true);
  const [detailIssue, setDetailIssue] = useState(null);

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

  /* Pull — 공식본(Mongo) 재동기화 후 현재 뷰 재조회. */
  const handlePull = useCallback(async () => {
    try {
      await authFetch(`${API_BASE}/Issue/fetch`, {
        headers: { XBody: JSON.stringify({ prjID: projectCode }) },
      });
      await refetch();
    } catch (err) {
      alert('Pull 실패: ' + err.message);
    }
  }, [projectCode, refetch]);

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
      await authFetch(`${API_BASE}/Issue`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, data }),
      });
      refetch();
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
      await authFetch(`${API_BASE}/Issue`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: issue.key, prjID: projectCode }),
      });
      refetch();
    } catch (err) {
      alert('삭제 실패: ' + err.message);
    }
  }

  function handleSummaryBlur(issue, newSummary) {
    if (newSummary === issue.summary) return;
    authFetch(`${API_BASE}/Issue`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key: issue.key, data: { summary: newSummary } }),
    })
      .then(() => refetch())
      .catch((err) => alert('수정 실패: ' + err.message));
  }

  function handleSelectChange(issue, field, newName) {
    const oldVal = issue[field];
    const oldName = oldVal && typeof oldVal === 'object' ? oldVal.name : oldVal || '';
    if (newName === oldName) return;
    const newVal = { ...(typeof oldVal === 'object' ? oldVal : {}), name: newName };
    authFetch(`${API_BASE}/Issue`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key: issue.key, data: { [field]: newVal } }),
    })
      .then(() => refetch())
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

  // 웹 이슈 뷰 이원화 Phase 1 — 런처 감지 + 로컬 clone 있을 때만 토글 노출(그 외는 현행과 동일).
  const sourceOptions = sourceAvailable ? [
    { label: '공식본', active: source === 'main', handler: () => setSource('main') },
    { label: '내 작업본', active: source === 'local', handler: () => setSource('local') },
  ] : [];

  return (
    <div className="il-wrapper">
      <CommonBar
        title={barTitle}
        onBack={onBack}
        onPull={handlePull}
        nav={nav}
        options={[
          ...sourceOptions,
          { label: 'V', active: showV, handler: () => setShowV((v) => !v) },
          { label: 'F', active: showF, handler: () => setShowF((v) => !v) },
          { label: 'R', active: showR, handler: () => setShowR((v) => !v) },
          { label: 'A', active: showA, handler: () => setShowA((v) => !v) },
          { label: 'D', active: showD, handler: () => setShowD((v) => !v) },
        ]}
      />
      {/* 내 작업본 출처 배지 — 눈에 띄되 과하지 않게 */}
      {source === 'local' && (
        <div className="il-source-badge">
          내 작업본 (로컬, 미발행 변경 포함){localMeta ? ` · 이슈 ${localMeta.count}건` : ''}
        </div>
      )}

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
