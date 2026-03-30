import React, { useEffect, useState } from 'react';
import './IssueList.css';

const ISSUE_TYPES = [
  'VOS', 'Feature', 'Requirement',
  'Architectueral Design', 'Detaied Design', 'Test',
];

const API_BASE = 'http://localhost:9000/api/v2';

export default function IssueList({ project, onBack }) {
  const [issues, setIssues] = useState([]);
  const [loading, setLoading] = useState(false);
  const [pulling, setPulling] = useState(false);
  const [sortKey, setSortKey] = useState('key');
  const [sortAsc, setSortAsc] = useState(true);
  const [filterType, setFilterType] = useState('');

  const projectCode = project.JiraCode || '';

  useEffect(() => {
    fetchIssues();
  }, [projectCode]); // eslint-disable-line react-hooks/exhaustive-deps

  function fetchIssues() {
    setLoading(true);
    const xbody = JSON.stringify({ prjID: projectCode, infoType: '' });
    fetch(`${API_BASE}/Issue`, { headers: { XBody: xbody } })
      .then((res) => res.json())
      .then((data) => {
        if (Array.isArray(data)) setIssues(data);
        else setIssues([]);
      })
      .catch(() => setIssues([]))
      .finally(() => setLoading(false));
  }

  function handlePull() {
    setPulling(true);
    const xbody = JSON.stringify({
      prjID: projectCode,
      query: ISSUE_TYPES.join(','),
    });
    fetch(`${API_BASE}/Issue/fetch`, { headers: { XBody: xbody } })
      .then((res) => res.json())
      .then((data) => {
        alert(`${data.upserted || 0}건 가져옴 (전체 ${data.total || 0}건)`);
        fetchIssues();
      })
      .catch((err) => alert('Pull 실패: ' + err.message))
      .finally(() => setPulling(false));
  }

  // Sort
  const sorted = [...issues].sort((a, b) => {
    let va = a[sortKey] || '';
    let vb = b[sortKey] || '';
    if (typeof va === 'object') va = va.name || '';
    if (typeof vb === 'object') vb = vb.name || '';
    const cmp = String(va).localeCompare(String(vb), undefined, { numeric: true });
    return sortAsc ? cmp : -cmp;
  });

  // Filter by issue type
  const filtered = filterType
    ? sorted.filter((i) => i.issueType === filterType)
    : sorted;

  function handleSort(key) {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(true); }
  }

  function sortIndicator(key) {
    if (sortKey !== key) return '';
    return sortAsc ? ' \u25B2' : ' \u25BC';
  }

  const columns = [
    { key: 'key', label: 'Key', width: 120 },
    { key: 'issueType', label: 'Type', width: 140 },
    { key: 'summary', label: 'Summary', width: 0 },
    { key: 'status', label: 'Status', width: 100 },
    { key: 'priority', label: 'Priority', width: 90 },
  ];

  function cellValue(issue, col) {
    const v = issue[col.key];
    // Requirement → reqtype (FR, QA, CNST)
    if (col.key === 'issueType' && v === 'Requirement' && issue.reqtype) {
      return issue.reqtype;
    }
    if (v && typeof v === 'object') return v.name || '';
    return v || '';
  }

  return (
    <div className="il-wrapper">
      <div className="il-topbar">
        <button className="il-back-btn" onClick={onBack}>&larr; Project Map</button>
        <span className="il-title">{project.Name} ({projectCode}) - Issues</span>
        <div className="il-controls">
          <select
            className="il-filter"
            value={filterType}
            onChange={(e) => setFilterType(e.target.value)}
          >
            <option value="">All Types</option>
            {ISSUE_TYPES.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
          <button
            className="il-pull-btn"
            onClick={handlePull}
            disabled={pulling}
          >
            {pulling ? 'Pulling...' : 'Issue Pull'}
          </button>
        </div>
      </div>

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
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={columns.length} className="il-empty">
                    이슈가 없습니다. Issue Pull 버튼으로 가져오세요.
                  </td>
                </tr>
              ) : (
                filtered.map((issue, i) => (
                  <tr key={issue.key || i}>
                    {columns.map((col) => (
                      <td key={col.key}>{cellValue(issue, col)}</td>
                    ))}
                  </tr>
                ))
              )}
            </tbody>
          </table>
        )}
      </div>

      <div className="il-statusbar">
        총 {filtered.length}건{filterType ? ` (${filterType} 필터)` : ''}
      </div>
    </div>
  );
}
