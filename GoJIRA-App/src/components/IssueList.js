import React, { useEffect, useState, useCallback } from 'react';
import CommonBar from './CommonBar';
import IssueDetailDialog from './IssueDetailDialog';
import config from '../config';
import './IssueList.css';

const API_BASE = config.API_BASE;

const NAV_ITEMS = [
  { key: 'issues', label: 'Issues' },
  { key: 'relationMap', label: 'Relation Map' },
  { key: 'satam', label: 'SATAM' },
];

// 옵션 키 → issueType 매핑
const TYPE_FILTER = {
  V: ['VOS'],
  F: ['Feature'],
  R: ['Requirement'],
  A: ['Architectueral Design'],
  D: ['Detaied Design'],
};

export default function IssueList({ project, onBack, currentPage, onNavigate }) {
  const [issues, setIssues] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [sortKey, setSortKey] = useState('key');
  const [sortAsc, setSortAsc] = useState(true);
  const [detailIssue, setDetailIssue] = useState(null);

  // 필터 옵션 (기본 모두 ON)
  const [showV, setShowV] = useState(true);
  const [showF, setShowF] = useState(true);
  const [showR, setShowR] = useState(true);
  const [showA, setShowA] = useState(true);
  const [showD, setShowD] = useState(true);

  const projectCode = project.JiraCode || '';

  const fetchIssues = useCallback(() => {
    setLoading(true);
    const xbody = JSON.stringify({ prjID: projectCode, infoType: '' });
    fetch(`${API_BASE}/Issue`, { headers: { XBody: xbody } })
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
      fetchIssues();
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

  const columns = [
    { key: 'key', label: 'Key', width: 120 },
    { key: 'subKey', label: 'SubKey', width: 100 },
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

  const barTitle = `${project.Name} (${projectCode})`;
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
                  <td colSpan={columns.length} className="il-empty">
                    이슈가 없습니다. Pull 버튼으로 가져오세요.
                  </td>
                </tr>
              ) : (
                sorted.map((issue, i) => (
                  <tr key={issue.key || i}>
                    {columns.map((col) => {
                      // Key, SubKey 클릭 → 상세 팝업
                      if (col.key === 'key' || col.key === 'subKey') {
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
        총 {sorted.length}건
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
