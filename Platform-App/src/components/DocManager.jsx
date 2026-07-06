import React, { useEffect, useState, useCallback, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
import { subscribe } from '../utils/sseManager';
import { modalAlert, modalConfirm, modalPrompt } from './AlertModal';
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
        if (await modalConfirm(`"${r.repoName}" — GitHub에 존재하지만 연결된 프로젝트가 없습니다.\nGitHub 레포지토리를 삭제하시겠습니까?`)) {
          // orphan은 projectCode가 없으므로 repoName으로 삭제
          await fetch(`${API}/${r.repoName}/github`, { method: 'DELETE' });
          cleaned = true;
        }
      }

      // 빨강: GitHub에 없지만 프로젝트에 연결됨 (missing)
      for (const r of data.filter(r => r.status === 'missing')) {
        if (await modalConfirm(`"${r.repoName}" — GitHub에 존재하지 않습니다.\n프로젝트 연결 정보를 삭제하시겠습니까?`)) {
          await fetch(`${API}/${r.projectCode}/purge`, { method: 'DELETE' });
          cleaned = true;
        }
      }

      if (cleaned) fetchRepos();
    } catch (err) {
      modalAlert('동기화 실패: ' + err.message);
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
                    {{tree:'Files', commits:'Commits', prs:'PRs', graph:'Graph'}[v]}
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
  const [allCommits, setAllCommits] = useState({});
  const [branches, setBranches] = useState([]);
  const [activeBranches, setActiveBranches] = useState(new Set());

  const colors = ['#6b9bc3', '#7fb685', 'tomato', '#d4a862', '#8a8dc9', '#6abfb0', '#c4876e'];

  useEffect(() => {
    fetch(`${API}/${repo.projectCode}/branches`)
      .then(r => r.json()).then(d => {
        const list = Array.isArray(d) ? d.map(b => b.name || b) : [];
        setBranches(list);
        // 기본: 모든 브랜치 활성화
        setActiveBranches(new Set(list));
      }).catch(() => {});
  }, [repo]);

  useEffect(() => {
    activeBranches.forEach(branch => {
      if (allCommits[branch]) return;
      fetch(`${API}/${repo.projectCode}/commits?branch=${branch}&perPage=50`)
        .then(r => r.json()).then(d => {
          setAllCommits(prev => ({ ...prev, [branch]: Array.isArray(d) ? d : [] }));
        }).catch(() => {});
    });
  }, [repo, activeBranches]);

  const toggleBranch = (branch) => {
    const next = new Set(activeBranches);
    if (next.has(branch)) next.delete(branch);
    else next.add(branch);
    setActiveBranches(next);
  };

  // 활성화된 브랜치의 커밋을 합치고 날짜순 정렬 + 중복 제거
  const merged = [];
  const seen = new Set();
  activeBranches.forEach(branch => {
    (allCommits[branch] || []).forEach(c => {
      if (!seen.has(c.sha)) {
        seen.add(c.sha);
        merged.push({ ...c, _branch: branch });
      }
    });
  });
  merged.sort((a, b) => new Date(b.commit?.author?.date || 0) - new Date(a.commit?.author?.date || 0));

  return (
    <div className="dm-commits">
      <div style={{ padding: '10px 10px 6px', display: 'flex', alignItems: 'center', gap: '6px', flexWrap: 'wrap' }}>
        {branches.map((b, i) => (
          <span
            key={b}
            onClick={() => toggleBranch(b)}
            style={{
              display: 'inline-block',
              padding: '3px 10px',
              fontSize: '12px',
              borderRadius: '4px',
              cursor: 'pointer',
              borderLeft: `4px solid ${colors[i % colors.length]}`,
              background: activeBranches.has(b) ? '#f0f0f0' : '#fff',
              color: activeBranches.has(b) ? '#333' : '#bbb',
              opacity: activeBranches.has(b) ? 1 : 0.5,
            }}
          >
            {b}
          </span>
        ))}
        <span style={{ fontSize: '12px', color: '#888', marginLeft: '4px' }}>{merged.length}개 커밋</span>
      </div>
      <table className="dm-table">
        <thead><tr><th>SHA</th><th>메시지</th><th>작성자</th><th>일시</th></tr></thead>
        <tbody>
          {merged.map(c => (
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
              // 사용자별 최신 리뷰만 유지 (PENDING, REQUEST_REVIEW, dismissed 제외).
              // Gitea 1.25 는 반려 state 를 'REQUEST_CHANGES' 로 돌려주는데 본 코드는 'CHANGES_REQUESTED' 를 기대 — 정규화.
              const byUser = {};
              reviews.forEach(rv => {
                if (rv.dismissed) return;
                if (rv.state === 'REQUEST_CHANGES') rv.state = 'CHANGES_REQUESTED';
                if (rv.state === 'PENDING' || rv.state === 'REQUEST_REVIEW') return;
                byUser[rv.user?.login] = rv;
              });
              const latest = Object.values(byUser);
              const approved = latest.filter(rv => rv.state === 'APPROVED');
              const rejected = latest.filter(rv => rv.state === 'CHANGES_REQUESTED');
              // pending = requested_reviewers 중 아직 승인/반려하지 않은 사용자
              const reviewedLogins = new Set(Object.keys(byUser));
              const pending = (pr.requested_reviewers || []).filter(r => !reviewedLogins.has(r.login));
              setReviewMap(prev => ({
                ...prev,
                [pr.number]: { reviews: latest, approved, rejected, pending }
              }));
            }).catch(() => {});
        });
      }).catch(() => {});
  }, [repo, filter]);

  useEffect(() => { fetchPRs(); }, [fetchPRs]);

  // SSE: doc_update / doc_review 이벤트 수신
  useEffect(() => {
    const unsub1 = subscribe('doc_update', () => { fetchPRs(); });
    const unsub2 = subscribe('doc_review', () => { fetchPRs(); });
    return () => { unsub1(); unsub2(); };
  }, [fetchPRs]);

  async function handleReview(prNum, event) {
    let body;
    if (event === 'REQUEST_CHANGES') {
      body = await modalPrompt('반려 사유');
      if (!body) return;
    } else {
      body = await modalPrompt('승인 의견', 'LGTM');
      if (body === null) return;
      if (!body) body = 'LGTM';
    }
    const res = await fetch(`${API}/${repo.projectCode}/pr/${prNum}/review`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ event, body: body || '' }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || '처리 실패');
    }
    fetchPRs();
  }

  async function handleMerge(prNum) {
    if (!await modalConfirm('머지하시겠습니까?')) return;
    const res = await fetch(`${API}/${repo.projectCode}/pr/${prNum}/merge`, {
      method: 'POST', headers: { 'X-User-Id': user },
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      const errMsg = err.error || '머지 실패';
      // 충돌 시 선택 다이얼로그
      if (res.status === 409 && errMsg.includes('충돌')) {
        const choice = await modalConfirm(
          `${errMsg}\n\n[확인] 반려 (PR 생성자에게 알림)\n[취소] 닫기`,
        );
        if (choice) {
          // 반려 처리 — review로 CHANGES_REQUESTED
          const reason = await modalPrompt('반려 사유', '충돌로 인한 반려');
          if (reason !== null) {
            await fetch(`${API}/${repo.projectCode}/pr/${prNum}/review`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
              body: JSON.stringify({ event: 'REQUEST_CHANGES', body: reason || '충돌로 인한 반려' }),
            });
            modalAlert('반려 처리 완료. PR 생성자에게 알림이 전송됩니다.');
          }
        }
      } else {
        modalAlert(errMsg);
      }
    } else {
      modalAlert('머지 완료');
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
  const [selectedBranches, setSelectedBranches] = useState(new Set());
  const [offset, setOffset] = useState(0);

  const limit = 30;

  useEffect(() => {
    fetch(`${API}/${repo.projectCode}/graph?offset=${offset}&limit=${limit}`)
      .then(r => r.json()).then(d => {
        // lanes 정보 재구성 (merge parent의 lane 정보)
        const shaToCommit = {};
        (d.commits || []).forEach(c => {
          shaToCommit[c.sha] = c;
        });

        d.commits?.forEach(c => {
          if (c.parents?.length > 1) {
            // merge 커밋: 2번째 parent부터의 lane 수집
            c.lanes = [];
            for (let i = 1; i < c.parents.length; i++) {
              const parentSha = c.parents[i];
              const parentCommit = shaToCommit[parentSha];
              if (parentCommit && parentCommit.lane !== undefined) {
                c.lanes.push(parentCommit.lane);
              }
            }
          }
        });

        if (offset === 0) {
          setData(d);
          // 초기: 모든 브랜치 선택 (전체 히스토리 표시)
          setSelectedBranches(new Set(d.branches || []));
        } else {
          // Show More: 기존 데이터에 추가
          setData(prev => ({
            ...prev,
            commits: [...(prev?.commits || []), ...(d.commits || [])],
            hasMore: d.hasMore,
          }));
        }
      }).catch(() => {});
  }, [repo, offset]);

  if (!data) return <div className="dm-graph">로딩 중...</div>;
  const { branches = [], commits = [], hasMore = false } = data;

  const handleBranchToggle = (branch) => {
    const newSet = new Set(selectedBranches);
    if (newSet.has(branch)) newSet.delete(branch);
    else newSet.add(branch);
    setSelectedBranches(newSet);
  };

  // ─── 프론트엔드 Lane 재계산 ───
  const shaMap = {};
  commits.forEach(c => { shaMap[c.sha] = c; });

  // 1) main first-parent chain → lane 0
  const mainHead = commits.find(c => c.refs?.includes('main'));
  const mainChain = new Set();
  if (mainHead) {
    let sha = mainHead.sha;
    while (sha && shaMap[sha]) {
      mainChain.add(sha);
      sha = shaMap[sha].parents?.[0];
    }
  }

  // 2) 브랜치별 lane 번호 할당
  const branchLaneNum = { main: 0 };
  let nextLane = 1;
  branches.filter(b => b !== 'main').forEach(b => {
    branchLaneNum[b] = nextLane++;
  });

  // 3) 커밋별 lane 할당
  const laneOf = {}; // sha → lane number

  // main chain → lane 0
  mainChain.forEach(sha => { laneOf[sha] = 0; });

  // merge 커밋의 second parent → 해당 브랜치 lane으로 추적
  mainChain.forEach(sha => {
    const c = shaMap[sha];
    if (c.parents?.length > 1) {
      const secondSha = c.parents[1];
      const secondCommit = shaMap[secondSha];
      if (!secondCommit) return;

      // second parent의 브랜치 이름 찾기
      let branchName = secondCommit.refs?.find(r => branchLaneNum[r] !== undefined);
      if (!branchName) {
        // refs가 없는 경우 (이미 merge된 커밋) → 같은 lane 패턴 추적
        // merge commit message에서 브랜치 이름 추출 시도
        const msg = c.message || '';
        const match = msg.match(/from\s+(\S+)/);
        if (match) {
          const foundBranch = branches.find(b => match[1].includes(b));
          if (foundBranch) branchName = foundBranch;
        }
      }

      const lane = branchName ? branchLaneNum[branchName] : (nextLane++);

      // second parent 부터 main chain에 도달할 때까지 같은 lane 할당
      let cur = secondSha;
      while (cur && shaMap[cur] && laneOf[cur] === undefined) {
        laneOf[cur] = lane;
        cur = shaMap[cur].parents?.[0];
      }
    }
  });

  // 아직 merge 안 된 브랜치 (docs/duke.kimm, docs/test 등)
  branches.filter(b => b !== 'main').forEach(b => {
    const head = commits.find(c => c.refs?.includes(b));
    if (head && laneOf[head.sha] === undefined) {
      const lane = branchLaneNum[b];
      let sha = head.sha;
      while (sha && shaMap[sha] && laneOf[sha] === undefined) {
        laneOf[sha] = lane;
        sha = shaMap[sha].parents?.[0];
      }
    }
  });

  // lane 할당 결과를 커밋에 적용
  commits.forEach(c => {
    if (laneOf[c.sha] !== undefined) c.lane = laneOf[c.sha];
  });

  // 날짜 역순 정렬 (Gitea/c2.png와 동일한 순서)
  const filteredCommits = [...commits].sort((a, b) => {
    const da = new Date(a.date || 0).getTime();
    const db = new Date(b.date || 0).getTime();
    return db - da; // 최신 먼저
  });

  // ─── 색상 및 좌표 ───
  const colors = ['#6b9bc3', '#7fb685', 'tomato', '#d4a862', '#8a8dc9', '#6abfb0', '#c4876e'];
  const branchColor = {};
  branches.forEach((b, i) => { branchColor[b] = colors[i % colors.length]; });

  // 사용 중인 lane 수집
  const shaIdx = {};
  const usedLanes = new Set();
  filteredCommits.forEach((c, i) => {
    shaIdx[c.sha] = i;
    usedLanes.add(c.lane);
  });
  const sortedLanes = Array.from(usedLanes).sort((a, b) => a - b);

  // lane → 브랜치 이름 매핑
  const laneToBranch = {};
  Object.entries(branchLaneNum).forEach(([b, l]) => { laneToBranch[l] = b; });

  // lane 색상
  const laneColors = {};
  sortedLanes.forEach(lane => {
    const branch = laneToBranch[lane];
    laneColors[lane] = branch && branchColor[branch] ? branchColor[branch] : '#999';
  });

  const nodeR = 7;
  const rowH = 40;
  const colW = 32;
  const maxLane = sortedLanes.length > 0 ? Math.max(...sortedLanes) : 0;
  const svgW = (maxLane + 1) * colW + 20;
  const x = lane => lane * colW + 16;
  const y = i => i * rowH + 20;

  return (
    <div className="dm-graph">
      <div className="dm-graph-header">
        {branches.map(b => (
          <span
            key={b}
            className="dm-branch-tag"
            style={{ borderLeft: `4px solid ${branchColor[b] || '#888'}` }}
          >
            {b}
          </span>
        ))}
      </div>
      <div className="dm-graph-body">
        <svg width={svgW} height={filteredCommits.length * rowH + 20} style={{ flexShrink: 0 }}>
          {/* 연결선: 각 커밋 → 부모 커밋 */}
          {filteredCommits.map((c, i) => {
            const lines = [];
            if (!c.parents || c.parents.length === 0) return lines;

            const cx = x(c.lane);
            const cy = y(i);

            c.parents.forEach((parentSha, pIdx) => {
              const pi = shaIdx[parentSha];
              if (pi === undefined) return;
              const parent = filteredCommits[pi];
              const px = x(parent.lane);
              const py = y(pi);

              // 라인 색상: 도착하는 쪽(부모) lane 색상
              const lineColor = laneColors[parent.lane] || '#999';

              if (c.lane === parent.lane) {
                // 같은 lane: 수직선
                lines.push(
                  <line key={`${c.sha}-p${pIdx}`}
                    x1={cx} y1={cy + nodeR} x2={px} y2={py - nodeR}
                    stroke={lineColor} strokeWidth={3} strokeLinecap="round" />
                );
              } else {
                // 다른 lane: S-커브 (c5.png 스타일)
                lines.push(
                  <path key={`${c.sha}-p${pIdx}`}
                    d={`M${cx},${cy + nodeR} C${cx},${cy + rowH * 0.8} ${px},${py - rowH * 0.8} ${px},${py - nodeR}`}
                    fill="none" stroke={lineColor} strokeWidth={3} strokeLinecap="round" />
                );
              }
            });
            return lines;
          })}

          {/* 노드 */}
          {filteredCommits.map((c, i) => {
            const color = laneColors[c.lane] || '#999';
            return (
              <circle key={c.sha}
                cx={x(c.lane)} cy={y(i)} r={nodeR}
                fill={color} stroke="#fff" strokeWidth={2.5} />
            );
          })}
        </svg>
        <div className="dm-graph-list">
          {filteredCommits.map((c, i) => (
            <div key={c.sha} className="dm-graph-row" style={{ height: rowH }}>
              <span className="dm-sha">{(c.sha || '').slice(0, 7)}</span>
              {c.refs?.map(ref => (
                <span key={ref} className="dm-graph-ref" style={{ background: branchColor[ref] || '#888' }}>{ref}</span>
              ))}
              <span className="dm-graph-msg">{(c.message || '').split('\n')[0]}</span>
              <span className="dm-graph-col-author">{c.author}</span>
              <span className="dm-graph-col-date">{(c.date || '').slice(0, 10)}</span>
            </div>
          ))}
          {hasMore && (
            <div style={{ textAlign: 'center', padding: '10px' }}>
              <button className="dm-btn" onClick={() => setOffset(offset + limit)}>더 보기 (Show More)</button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
