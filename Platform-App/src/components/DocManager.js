import React, { useEffect, useState, useCallback, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
import './DocManager.css';

const API = config.API_BASE + '/DocRepo';

export default function DocManager({ user }) {
  const [repos, setRepos] = useState([]);
  const [selectedRepo, setSelectedRepo] = useState(null);
  const [view, setView] = useState('tree');
  const [syncing, setSyncing] = useState(false);
  const [repoStatus, setRepoStatus] = useState({});
  const [openPRs, setOpenPRs] = useState({});
  const [sidebarWidth, setSidebarWidth] = useState(240);
  const resizing = useRef(false);

  const fetchRepos = useCallback(() => {
    fetch(API).then(r => r.json()).then(d => {
      const list = Array.isArray(d) ? d : [];
      setRepos(list);
      // 각 repo의 열린 PR 수 조회
      list.forEach(r => {
        fetch(`${API}/${r.projectCode}/prs?state=open`)
          .then(res => res.json())
          .then(prs => {
            if (Array.isArray(prs)) {
              setOpenPRs(prev => ({ ...prev, [r.projectCode]: prs.length }));
            }
          }).catch(() => {});
      });
    }).catch(() => {});
  }, []);

  useEffect(() => { fetchRepos(); }, [fetchRepos]);

  function handleResizeStart(e) {
    e.preventDefault();
    resizing.current = true;
    const startX = e.clientX;
    const startW = sidebarWidth;
    function onMove(ev) {
      if (!resizing.current) return;
      setSidebarWidth(Math.max(160, Math.min(500, startW + ev.clientX - startX)));
    }
    function onUp() { resizing.current = false; window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp); }
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }

  async function handleSync() {
    setSyncing(true);
    try {
      const res = await fetch(`${API}/sync`, { method: 'POST' });
      const data = await res.json();
      if (!Array.isArray(data)) return;

      const statusMap = {};
      data.forEach(r => { statusMap[r.repoName] = r; });
      setRepoStatus(statusMap);
      fetchRepos();

      let cleaned = false;

      // 주황: GitHub에 있지만 프로젝트 연결 없음 (orphan)
      for (const r of data.filter(r => r.status === 'orphan')) {
        if (window.confirm(`"${r.repoName}" — GitHub에 존재하지만 연결된 프로젝트가 없습니다.\nGitHub 레포지토리를 삭제하시겠습니까?`)) {
          // orphan은 projectCode가 없으므로 repoName으로 삭제
          await fetch(`${API}/${r.repoName}/github`, { method: 'DELETE' });
          cleaned = true;
        }
      }

      // 빨강: GitHub에 없지만 프로젝트에 연결됨 (missing)
      for (const r of data.filter(r => r.status === 'missing')) {
        if (window.confirm(`"${r.repoName}" — GitHub에 존재하지 않습니다.\n프로젝트 연결 정보를 삭제하시겠습니까?`)) {
          await fetch(`${API}/${r.projectCode}/purge`, { method: 'DELETE' });
          cleaned = true;
        }
      }

      if (cleaned) fetchRepos();
    } catch (err) {
      alert('동기화 실패: ' + err.message);
    } finally {
      setSyncing(false);
    }
  }

  return (
    <div className="dm-wrapper">
      <div className="dm-sidebar" style={{ width: sidebarWidth }}>
        <div className="dm-sidebar-header">
          <span>Repositories</span>
          <div className="dm-sidebar-btns">
            <button className="dm-sync-btn" onClick={handleSync} disabled={syncing} title="GitHub 동기화">
              {syncing ? '...' : '↻'}
            </button>
          </div>
        </div>
        {repos.map(r => {
          const st = repoStatus[r.repoName];
          const cls = st?.status === 'orphan' ? 'dm-repo-orphan' : st?.status === 'missing' ? 'dm-repo-missing' : '';
          return (
            <div key={r.projectCode}
              className={`dm-repo-item ${selectedRepo?.projectCode === r.projectCode ? 'dm-repo-active' : ''} ${cls}`}
              onClick={() => { setSelectedRepo(r); setView('tree'); }}>
              <span className="dm-repo-name">
                {r.repoName}
                {openPRs[r.projectCode] > 0 && (
                  <span className="dm-pr-badge">{openPRs[r.projectCode]}</span>
                )}
              </span>
              <span className="dm-repo-sub">{r.projectName}</span>
              <span className="dm-repo-sub">{r.repoOwner}</span>
            </div>
          );
        })}
      </div>
      <div className="dm-resize-handle" onMouseDown={handleResizeStart} />

      <div className="dm-main">
        {selectedRepo ? (
          <>
            <div className="dm-toolbar">
              <span className="dm-repo-title">{selectedRepo.repoOwner}/{selectedRepo.repoName}</span>
              <div className="dm-view-tabs">
                {['tree', 'commits', 'prs', 'graph'].map(v => (
                  <button key={v} className={`dm-tab ${view === v ? 'dm-tab-active' : ''}`} onClick={() => setView(v)}>
                    {v === 'tree' ? 'Files' : v === 'commits' ? 'Commits' : v === 'prs' ? 'PRs' : 'Graph'}
                  </button>
                ))}
              </div>
            </div>
            {view === 'tree' && <DocTree repo={selectedRepo} />}
            {view === 'commits' && <CommitHistory repo={selectedRepo} />}
            {view === 'prs' && <PRList repo={selectedRepo} user={user} />}
            {view === 'graph' && <GitGraph repo={selectedRepo} />}
          </>
        ) : (
          <div className="dm-placeholder">Repository를 선택하세요</div>
        )}
      </div>
    </div>
  );
}

// --- DocTree + Viewer ---
function DocTree({ repo }) {
  const [tree, setTree] = useState(null);
  const [selectedFile, setSelectedFile] = useState(null);
  const [content, setContent] = useState('');

  useEffect(() => {
    fetch(`${API}/${repo.projectCode}/tree?ref=${repo.defaultBranch || 'main'}`)
      .then(r => r.json()).then(d => setTree(d)).catch(() => {});
    setSelectedFile(null);
    setContent('');
  }, [repo]);

  function handleFileClick(path) {
    setSelectedFile(path);
    fetch(`${API}/${repo.projectCode}/file?path=${encodeURIComponent(path)}&ref=${repo.defaultBranch || 'main'}`)
      .then(r => r.json())
      .then(d => {
        if (d.content && d.encoding === 'base64') {
          const bytes = Uint8Array.from(atob(d.content), c => c.charCodeAt(0));
          setContent(new TextDecoder('utf-8').decode(bytes));
        } else {
          setContent(d.content || '');
        }
      })
      .catch(() => setContent('파일을 불러올 수 없습니다.'));
  }

  const items = tree?.tree || [];
  const sorted = [...items].sort((a, b) => {
    if (a.type === 'tree' && b.type !== 'tree') return -1;
    if (a.type !== 'tree' && b.type === 'tree') return 1;
    return a.path.localeCompare(b.path);
  });

  return (
    <div className="dm-tree-wrap">
      <div className="dm-tree-list">
        {sorted.map(item => (
          <div key={item.path}
            className={`dm-tree-item ${item.type === 'tree' ? 'dm-tree-folder' : ''} ${selectedFile === item.path ? 'dm-tree-selected' : ''}`}
            style={{ paddingLeft: (item.path.split('/').length - 1) * 16 + 8 }}
            onClick={() => item.type !== 'tree' && handleFileClick(item.path)}>
            <span className="dm-tree-icon">{item.type === 'tree' ? '📁' : '📄'}</span>
            <span>{item.path.split('/').pop()}</span>
          </div>
        ))}
      </div>
      <div className="dm-file-viewer">
        {selectedFile ? (
          <>
            <div className="dm-file-header">{selectedFile}</div>
            <div className="dm-file-content">
              {selectedFile.endsWith('.md') ? (
                <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
              ) : (
                <pre>{content}</pre>
              )}
            </div>
          </>
        ) : (
          <div className="dm-placeholder">파일을 선택하세요</div>
        )}
      </div>
    </div>
  );
}

// --- Commit History ---
function CommitHistory({ repo }) {
  const [commits, setCommits] = useState([]);
  useEffect(() => {
    fetch(`${API}/${repo.projectCode}/commits?branch=${repo.defaultBranch || 'main'}&perPage=50`)
      .then(r => r.json()).then(d => setCommits(Array.isArray(d) ? d : [])).catch(() => {});
  }, [repo]);

  return (
    <div className="dm-commits">
      <table className="dm-table">
        <thead><tr><th>SHA</th><th>메시지</th><th>작성자</th><th>일시</th></tr></thead>
        <tbody>
          {commits.map(c => (
            <tr key={c.sha}>
              <td className="dm-sha">{(c.sha || '').slice(0, 7)}</td>
              <td>{c.commit?.message?.split('\n')[0]}</td>
              <td>{c.commit?.author?.name || c.author?.login}</td>
              <td className="dm-date">{(c.commit?.author?.date || '').slice(0, 16).replace('T', ' ')}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// --- PR List ---
function PRList({ repo, user }) {
  const [prs, setPRs] = useState([]);
  const [filter, setFilter] = useState('open');
  const [reviewMap, setReviewMap] = useState({});
  const [expandedPR, setExpandedPR] = useState(null);

  const fetchPRs = useCallback(() => {
    fetch(`${API}/${repo.projectCode}/prs?state=${filter}`)
      .then(r => r.json()).then(d => {
        const list = Array.isArray(d) ? d : [];
        setPRs(list);
        // 열린 PR의 리뷰 상태 조회
        list.filter(pr => pr.state === 'open').forEach(pr => {
          fetch(`${API}/${repo.projectCode}/pr/${pr.number}/reviews`)
            .then(r => r.json()).then(reviews => {
              if (!Array.isArray(reviews)) return;
              // 사용자별 최신 리뷰만 유지 (PENDING, REQUEST_REVIEW 제외)
              const byUser = {};
              reviews.forEach(rv => {
                if (rv.state === 'PENDING' || rv.state === 'REQUEST_REVIEW') return;
                byUser[rv.user?.login] = rv;
              });
              const latest = Object.values(byUser);
              const approved = latest.filter(rv => rv.state === 'APPROVED');
              const rejected = latest.filter(rv => rv.state === 'CHANGES_REQUESTED');
              // pending = requested_reviewers 중 아직 승인/반려하지 않은 사용자
              const reviewed = new Set(Object.keys(byUser));
              const pending = (pr.requested_reviewers || []).filter(r => !reviewed.has(r.login));
              setReviewMap(prev => ({
                ...prev,
                [pr.number]: { reviews: latest, approved, rejected, pending }
              }));
            }).catch(() => {});
        });
      }).catch(() => {});
  }, [repo, filter]);

  useEffect(() => { fetchPRs(); }, [fetchPRs]);

  // SSE: doc_update 이벤트 수신
  useEffect(() => {
    const streamUrl = config.API_BASE.replace('/DocRepo', '') + '/Notice/stream';
    const es = new EventSource(streamUrl);
    es.addEventListener('doc_update', () => { fetchPRs(); });
    es.addEventListener('doc_review', () => { fetchPRs(); });
    return () => es.close();
  }, [fetchPRs]);

  async function handleReview(prNum, event) {
    let body;
    if (event === 'REQUEST_CHANGES') {
      body = prompt('반려 사유');
      if (!body) return;
    } else {
      body = prompt('승인 의견', 'LGTM');
      if (body === null) return;
      if (!body) body = 'LGTM';
    }
    const res = await fetch(`${API}/${repo.projectCode}/pr/${prNum}/review`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ event, body: body || '' }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      alert(err.error || '처리 실패');
    }
    fetchPRs();
  }

  async function handleMerge(prNum) {
    if (!window.confirm('머지하시겠습니까?')) return;
    const res = await fetch(`${API}/${repo.projectCode}/pr/${prNum}/merge`, {
      method: 'POST', headers: { 'X-User-Id': user },
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      alert(err.error || '머지 실패');
    }
    fetchPRs();
  }

  function ReviewStatus({ pr }) {
    const info = reviewMap[pr.number];
    if (!info) return null;
    const total = info.approved.length + info.rejected.length + info.pending.length;
    if (total === 0) return null;
    return (
      <div className="dm-review-status">
        {info.approved.map(rv => (
          <span key={rv.user?.login} className="dm-rv dm-rv-approved" title={rv.user?.login + ' 승인'}>✓ {rv.user?.login}</span>
        ))}
        {info.rejected.map(rv => (
          <span key={rv.user?.login} className="dm-rv dm-rv-rejected" title={rv.user?.login + ' 반려'}>✗ {rv.user?.login}</span>
        ))}
        {info.pending.map(u => (
          <span key={u.login} className="dm-rv dm-rv-pending" title={u.login + ' 대기중'}>◦ {u.login}</span>
        ))}
        <span className="dm-rv-summary">{info.approved.length}/{total}</span>
      </div>
    );
  }

  function isMergeable(pr) {
    const info = reviewMap[pr.number];
    if (!info) return false;
    const total = info.approved.length + info.rejected.length + info.pending.length;
    return total > 0 && info.rejected.length === 0 && info.pending.length === 0;
  }

  return (
    <div className="dm-prs">
      <div className="dm-pr-toolbar">
        <div className="dm-pr-filters">
          {['open', 'closed', 'all'].map(f => (
            <button key={f} className={`dm-tab ${filter === f ? 'dm-tab-active' : ''}`} onClick={() => setFilter(f)}>{f}</button>
          ))}
        </div>
      </div>
      <table className="dm-table">
        <thead><tr><th>#</th><th>제목</th><th>상태</th><th>리뷰</th><th>작성자</th><th>일시</th><th></th></tr></thead>
        <tbody>
          {prs.map(pr => {
            const info = reviewMap[pr.number];
            const lastReject = info?.rejected?.[info.rejected.length - 1];
            const isExpanded = expandedPR === pr.number;
            return (
              <React.Fragment key={pr.number}>
                <tr className="dm-pr-row" onClick={() => setExpandedPR(isExpanded ? null : pr.number)} style={{cursor:'pointer'}}>
                  <td>{pr.number}</td>
                  <td>
                    <span className="dm-pr-expand-icon">{isExpanded ? '▾' : '▸'}</span>
                    {pr.title}
                    {pr.html_url && (
                      <a href={pr.html_url} target="_blank" rel="noopener noreferrer" className="dm-pr-detail-link" title="Gitea에서 보기" onClick={e => e.stopPropagation()}>상세보기</a>
                    )}
                  </td>
                  <td><span className={`dm-pr-state ${pr.merged_at ? 'dm-pr-merged' : 'dm-pr-' + pr.state}`}>{pr.merged_at ? 'merged' : pr.state}</span></td>
                  <td><ReviewStatus pr={pr} /></td>
                  <td>{pr.user?.login}</td>
                  <td className="dm-date">{(pr.created_at || '').slice(0, 10)}</td>
                  <td className="dm-pr-actions" onClick={e => e.stopPropagation()}>
                    {pr.state === 'open' && (() => {
                      const myReview = reviewMap[pr.number]?.reviews?.find(rv => rv.user?.login === user);
                      const myState = myReview?.state;
                      return <>
                        {pr.user?.login !== user && (
                          <>
                            <button className="dm-approve-btn" disabled={myState === 'APPROVED'}
                              onClick={() => handleReview(pr.number, 'APPROVE')}>{myState === 'APPROVED' ? '승인됨' : '승인'}</button>
                            <button className="dm-reject-btn" disabled={myState === 'CHANGES_REQUESTED'}
                              onClick={() => handleReview(pr.number, 'REQUEST_CHANGES')}>{myState === 'CHANGES_REQUESTED' ? '반려됨' : '반려'}</button>
                          </>
                        )}
                        <button className="dm-merge-btn" onClick={() => handleMerge(pr.number)} disabled={!isMergeable(pr)}
                          title={isMergeable(pr) ? '머지' : '모든 승인이 완료되어야 머지할 수 있습니다'}>머지</button>
                      </>;
                    })()}
                  </td>
                </tr>
                {isExpanded && (
                  <tr className="dm-pr-detail-row">
                    <td></td>
                    <td colSpan={6}>
                      <div className="dm-pr-detail-panel">
                        {pr.body ? (
                          <div className="dm-pr-body">
                            <div className="dm-pr-detail-label">본문</div>
                            <pre className="dm-pr-body-text">{pr.body}</pre>
                          </div>
                        ) : (
                          <div className="dm-pr-body dm-pr-body-empty">본문 없음</div>
                        )}
                        <div className="dm-pr-reviews-section">
                          <div className="dm-pr-detail-label">리뷰 코멘트</div>
                          {info?.reviews?.length > 0 ? (
                            <div className="dm-pr-review-list">
                              {info.reviews.map((rv, i) => (
                                <div key={i} className={`dm-pr-review-item dm-rv-item-${rv.state === 'APPROVED' ? 'approved' : rv.state === 'CHANGES_REQUESTED' ? 'rejected' : 'pending'}`}>
                                  <div className="dm-pr-review-header">
                                    <span className="dm-pr-review-user">{rv.user?.login}</span>
                                    <span className={`dm-pr-review-state dm-rv-item-state-${rv.state === 'APPROVED' ? 'approved' : rv.state === 'CHANGES_REQUESTED' ? 'rejected' : 'pending'}`}>
                                      {rv.state === 'APPROVED' ? '✓ 승인' : rv.state === 'CHANGES_REQUESTED' ? '✗ 반려' : rv.state}
                                    </span>
                                    <span className="dm-date">{(rv.submitted_at || '').slice(0, 16).replace('T', ' ')}</span>
                                  </div>
                                  {rv.body && <div className="dm-pr-review-body">{rv.body}</div>}
                                </div>
                              ))}
                            </div>
                          ) : (
                            <div className="dm-pr-review-empty">리뷰 코멘트 없음</div>
                          )}
                        </div>
                      </div>
                    </td>
                  </tr>
                )}
                {!isExpanded && lastReject?.body && (
                  <tr className="dm-pr-msg-row">
                    <td></td>
                    <td colSpan={6}>
                      <div className="dm-pr-msg dm-pr-msg-reject">
                        <strong>{lastReject.user?.login}</strong>: {lastReject.body}
                      </div>
                    </td>
                  </tr>
                )}
                {!isExpanded && info?.approved?.length > 0 && info.pending?.length === 0 && info.rejected?.length === 0 && pr.state === 'open' && (
                  <tr className="dm-pr-msg-row">
                    <td></td>
                    <td colSpan={6}>
                      <div className="dm-pr-msg dm-pr-msg-approved">
                        모든 승인이 완료되었습니다. 머지할 수 있습니다.
                      </div>
                    </td>
                  </tr>
                )}
              </React.Fragment>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// --- Git Graph ---
function GitGraph({ repo }) {
  const [data, setData] = useState(null);
  useEffect(() => {
    fetch(`${API}/${repo.projectCode}/graph`)
      .then(r => r.json()).then(setData).catch(() => {});
  }, [repo]);

  if (!data) return <div className="dm-graph">로딩 중...</div>;
  const { branches = [], commits = [] } = data;

  // 브랜치별 색상
  const colors = ['#e74c3c', '#27ae60', '#2980b9', '#f39c12', '#8e44ad', '#1abc9c', '#d35400'];
  const branchColor = {};
  branches.forEach((b, i) => { branchColor[b] = colors[i % colors.length]; });

  // 커밋별 lane(컬럼) 할당 — parent 기반
  const shaIdx = {};
  commits.forEach((c, i) => { shaIdx[c.sha] = i; });
  const lanes = new Array(commits.length).fill(-1);
  const laneUsed = [];

  // 각 커밋에 lane 할당
  function assignLane(idx) {
    if (lanes[idx] >= 0) return lanes[idx];
    const c = commits[idx];
    // refs에 main이 있으면 lane 0
    if (c.refs?.includes('main')) {
      lanes[idx] = 0;
      if (!laneUsed.includes(0)) laneUsed.push(0);
      return 0;
    }
    // parent가 있으면 parent의 lane 우선 시도
    if (c.parents?.length) {
      const pIdx = shaIdx[c.parents[0]];
      if (pIdx !== undefined) {
        const pLane = assignLane(pIdx);
        // parent에 여러 자식이 있으면 새 lane
        const siblings = commits.filter((cc, ii) => ii !== idx && cc.parents?.[0] === c.parents[0] && lanes[ii] >= 0);
        if (siblings.length === 0 && lanes[idx] < 0) {
          lanes[idx] = pLane;
          return pLane;
        }
      }
    }
    if (lanes[idx] >= 0) return lanes[idx];
    // 새 lane 할당
    let newLane = 0;
    while (laneUsed.includes(newLane)) newLane++;
    laneUsed.push(newLane);
    lanes[idx] = newLane;
    return newLane;
  }

  // main 브랜치 커밋 먼저 lane 0 할당
  commits.forEach((c, i) => { if (c.refs?.includes('main')) lanes[i] = 0; });
  if (!laneUsed.includes(0) && commits.length) laneUsed.push(0);
  commits.forEach((_, i) => assignLane(i));

  const maxLane = Math.max(...lanes, 0);
  const rowH = 34;
  const colW = 24;
  const svgW = (maxLane + 1) * colW + 20;
  const x = lane => lane * colW + 16;
  const y = i => i * rowH + 18;

  // 커밋에 대한 색상 결정
  function nodeColor(c) {
    if (c.refs?.length) {
      for (const ref of c.refs) {
        if (branchColor[ref]) return branchColor[ref];
      }
    }
    return '#2c3e50';
  }

  return (
    <div className="dm-graph">
      <div className="dm-graph-header">
        {branches.map(b => (
          <span key={b} className="dm-branch-tag" style={{ borderLeft: `3px solid ${branchColor[b] || '#888'}` }}>{b}</span>
        ))}
      </div>
      <div className="dm-graph-body">
        <svg width={svgW} height={commits.length * rowH + 20} style={{ flexShrink: 0 }}>
          {/* 연결선: 자식→부모 */}
          {commits.map((c, i) => c.parents?.map(psha => {
            const pi = shaIdx[psha];
            if (pi === undefined) return null;
            const x1 = x(lanes[i]), y1 = y(i);
            const x2 = x(lanes[pi]), y2 = y(pi);
            if (x1 === x2) {
              return <line key={`${c.sha}-${psha}`} x1={x1} y1={y1} x2={x2} y2={y2} stroke={nodeColor(c)} strokeWidth={2} />;
            }
            return <path key={`${c.sha}-${psha}`} d={`M${x1},${y1} C${x1},${y1 + rowH / 2} ${x2},${y2 - rowH / 2} ${x2},${y2}`} fill="none" stroke={nodeColor(c)} strokeWidth={2} />;
          }))}
          {/* 노드 */}
          {commits.map((c, i) => {
            const isMerge = c.parents?.length > 1;
            return <circle key={c.sha} cx={x(lanes[i])} cy={y(i)} r={isMerge ? 6 : 4}
              fill={nodeColor(c)} stroke={isMerge ? '#fff' : 'none'} strokeWidth={isMerge ? 2 : 0} />;
          })}
        </svg>
        <div className="dm-graph-list">
          {commits.map(c => (
            <div key={c.sha} className="dm-graph-row" style={{ height: rowH }}>
              <span className="dm-sha">{(c.sha || '').slice(0, 7)}</span>
              {c.refs?.map(ref => (
                <span key={ref} className="dm-graph-ref" style={{ background: branchColor[ref] || '#888' }}>{ref}</span>
              ))}
              <span className="dm-graph-msg">{(c.message || '').split('\n')[0]}</span>
              <span className="dm-graph-author">{c.author}</span>
              <span className="dm-date">{(c.date || '').slice(0, 10)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
