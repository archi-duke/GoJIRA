import React, { useEffect, useState, useCallback, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
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
                {['tree', 'edit', 'commits', 'prs', 'graph'].map(v => (
                  <button key={v} className={`dm-tab ${view === v ? 'dm-tab-active' : ''}`} onClick={() => setView(v)}>
                    {{tree:'Files', edit:'Edit', commits:'Commits', prs:'PRs', graph:'Graph'}[v]}
                  </button>
                ))}
              </div>
            </div>
            {view === 'tree' && <DocTree repo={selectedRepo} />}
            {view === 'edit' && <DocEditor repo={selectedRepo} user={user} />}
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

// --- Doc Editor ---
function DocEditor({ repo, user }) {
  const [started, setStarted] = useState(false);
  const [branch, setBranch] = useState('');
  const [tree, setTree] = useState(null);
  const [selectedFile, setSelectedFile] = useState(null);
  const [content, setContent] = useState('');
  const [original, setOriginal] = useState('');
  const [saving, setSaving] = useState(false);
  const [previewMode, setPreviewMode] = useState(false);
  const [hasPending, setHasPending] = useState(false);

  // 미완료 작업 확인
  useEffect(() => {
    fetch(`${API}/${repo.projectCode}/edit/status`, { headers: { 'X-User-Id': user } })
      .then(r => r.json()).then(d => {
        setHasPending(d.hasPending);
        if (d.hasPending) setBranch(d.branch);
      }).catch(() => {});
  }, [repo, user]);

  // 편집 시작
  async function startEdit() {
    const res = await fetch(`${API}/${repo.projectCode}/edit/start`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
    });
    if (!res.ok) { modalAlert('작업 공간 생성 실패'); return; }
    const d = await res.json();
    setBranch(d.branch);
    setStarted(true);
    fetchTree();
  }

  // 이어서 작업
  function resumeEdit() {
    setStarted(true);
    fetchTree();
  }

  // 작업 폐기
  async function discardEdit() {
    if (!await modalConfirm('작업 내용을 폐기하시겠습니까?')) return;
    await fetch(`${API}/${repo.projectCode}/edit/discard`, {
      method: 'DELETE', headers: { 'X-User-Id': user },
    });
    setHasPending(false);
    setStarted(false);
    setSelectedFile(null);
    setContent('');
  }

  // 파일 트리 로드 (main 기준)
  function fetchTree() {
    fetch(`${API}/${repo.projectCode}/tree?ref=main`)
      .then(r => r.json()).then(d => setTree(d)).catch(() => {});
  }

  // 파일 선택 → worktree에서 읽기
  async function handleFileClick(path) {
    if (content !== original && selectedFile) {
      if (!await modalConfirm('저장하지 않은 변경이 있습니다. 이동하시겠습니까?')) return;
    }
    const res = await fetch(`${API}/${repo.projectCode}/edit/file?path=${encodeURIComponent(path)}`, {
      headers: { 'X-User-Id': user },
    });
    if (res.ok) {
      const d = await res.json();
      setSelectedFile(path);
      setContent(d.content);
      setOriginal(d.content);
      setPreviewMode(false);
    } else {
      // worktree에 아직 없으면 main에서 가져오기
      const res2 = await fetch(`${API}/${repo.projectCode}/file?path=${encodeURIComponent(path)}&ref=main`);
      if (res2.ok) {
        const d = await res2.json();
        const text = d.content ? atob(d.content) : '';
        setSelectedFile(path);
        setContent(text);
        setOriginal(text);
        setPreviewMode(false);
      }
    }
  }

  // 파일 저장 (worktree에만, 커밋 없이)
  async function handleSave() {
    setSaving(true);
    await fetch(`${API}/${repo.projectCode}/edit/file`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ path: selectedFile, content }),
    });
    setOriginal(content);
    setSaving(false);
  }

  // 커밋 + 푸시
  async function handleCommit() {
    const msg = await modalPrompt('커밋 메시지', `${selectedFile || '문서'} 수정`);
    if (!msg) return;
    // 미저장 변경이 있으면 먼저 저장
    if (content !== original && selectedFile) await handleSave();
    const res = await fetch(`${API}/${repo.projectCode}/edit/commit`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ message: msg }),
    });
    if (res.ok) {
      modalAlert('커밋 완료');
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || '커밋 실패');
    }
  }

  // 배포(PR) 요청
  async function handleCreatePR() {
    const title = await modalPrompt('PR 제목', `[${repo.projectCode}] 설계문서 배포 요청`);
    if (!title) return;
    // 미저장 변경이 있으면 먼저 저장
    if (content !== original && selectedFile) await handleSave();
    const res = await fetch(`${API}/${repo.projectCode}/edit/pr`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ title }),
    });
    if (res.ok) {
      modalAlert('PR이 생성되었습니다. PRs 탭에서 확인하세요.');
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || 'PR 생성 실패');
    }
  }

  const modified = content !== original;
  const items = (tree?.tree || []).filter(i => i.type !== 'tree').sort((a, b) => a.path.localeCompare(b.path));

  if (!started) {
    return (
      <div className="dm-editor-start">
        {hasPending ? (
          <div className="dm-pending-notice">
            <p>이전에 작업하던 내용이 있습니다. ({branch})</p>
            <button className="dm-btn" onClick={resumeEdit}>이어서 작업</button>
            <button className="dm-btn dm-btn-danger" onClick={discardEdit}>폐기</button>
          </div>
        ) : (
          <div>
            <p>문서를 편집하려면 작업을 시작하세요.</p>
            <button className="dm-btn dm-btn-primary" onClick={startEdit}>편집 시작</button>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="dm-editor">
      <div className="dm-editor-sidebar">
        <div className="dm-editor-sidebar-header">
          <span>{branch}</span>
        </div>
        {items.map(item => (
          <div key={item.path}
            className={`dm-editor-file ${selectedFile === item.path ? 'dm-editor-file-active' : ''}`}
            onClick={() => handleFileClick(item.path)}>
            {item.path}
          </div>
        ))}
      </div>
      <div className="dm-editor-main">
        {selectedFile ? (
          <>
            <div className="dm-editor-toolbar">
              <span className="dm-editor-filename">{selectedFile} {modified && <span className="dm-modified">●</span>}</span>
              <div className="dm-editor-actions">
                <button className={`dm-tab ${!previewMode ? 'dm-tab-active' : ''}`} onClick={() => setPreviewMode(false)}>편집</button>
                <button className={`dm-tab ${previewMode ? 'dm-tab-active' : ''}`} onClick={() => setPreviewMode(true)}>미리보기</button>
                <button className="dm-btn" onClick={handleSave} disabled={!modified || saving}>{saving ? '저장중...' : '저장'}</button>
                <button className="dm-btn" onClick={handleCommit}>커밋/푸시</button>
                <button className="dm-btn dm-btn-primary" onClick={handleCreatePR}>배포(PR) 요청</button>
              </div>
            </div>
            <div className="dm-editor-body">
              {previewMode ? (
                <div className="dm-editor-preview">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
                </div>
              ) : (
                <textarea className="dm-editor-textarea" value={content}
                  onChange={e => setContent(e.target.value)} spellCheck={false} />
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
      modalAlert(err.error || '머지 실패');
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

  // 선택된 브랜치의 모든 커밋 포함 (HEAD부터 모든 ancestor)
  // ancestry 기반 필터링: 각 선택 브랜치의 HEAD부터 부모를 재귀 추적
  const shaMap = {};
  commits.forEach(c => {
    shaMap[c.sha] = c;
  });

  const includedShas = new Set();

  // 각 선택된 브랜치의 HEAD 찾기
  selectedBranches.forEach(branch => {
    const headCommit = commits.find(c => c.refs && c.refs.includes(branch));
    if (headCommit) {
      // HEAD부터 시작해서 모든 부모를 재귀 추적
      const queue = [headCommit.sha];
      const visited = new Set();

      while (queue.length > 0) {
        const sha = queue.shift();
        if (visited.has(sha)) continue;
        visited.add(sha);
        includedShas.add(sha);

        const commit = shaMap[sha];
        if (commit && commit.parents) {
          commit.parents.forEach(p => {
            if (!visited.has(p)) {
              queue.push(p);
            }
          });
        }
      }
    }
  });

  const filteredCommits = commits.filter(c => includedShas.has(c.sha));

  const handleBranchToggle = (branch) => {
    const newSet = new Set(selectedBranches);
    if (newSet.has(branch)) {
      newSet.delete(branch);
    } else {
      newSet.add(branch);
    }
    setSelectedBranches(newSet);
  };

  // 브랜치별 색상
  const colors = ['#e74c3c', '#27ae60', '#2980b9', '#f39c12', '#8e44ad', '#1abc9c', '#d35400'];
  const branchColor = {};
  branches.forEach((b, i) => { branchColor[b] = colors[i % colors.length]; });

  // Backend에서 제공한 lane 정보 사용
  // 필터링된 커밋들의 lane을 재매핑 (필터링으로 인해 lane 번호 변경될 수 있음)
  const shaIdx = {};
  const lanesToUse = new Set();
  filteredCommits.forEach((c, i) => {
    shaIdx[c.sha] = i;
    lanesToUse.add(c.lane);
    if (c.lanes) {
      c.lanes.forEach(l => lanesToUse.add(l));
    }
  });

  // lane 번호를 연속된 인덱스로 재매핑
  const laneRemap = {};
  const sortedLanes = Array.from(lanesToUse).sort((a, b) => a - b);
  sortedLanes.forEach((lane, idx) => {
    laneRemap[lane] = idx;
  });

  const maxLane = sortedLanes.length > 0 ? Math.max(...sortedLanes.map(l => laneRemap[l])) : 0;
  const rowH = 34;
  const colW = 45;
  const svgW = (maxLane + 1) * colW + 60;
  const x = lane => laneRemap[lane] * colW + 30;
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

  // 각 lane별 색상 매핑
  const laneColors = {};
  sortedLanes.forEach((lane, idx) => {
    laneColors[lane] = colors[idx % colors.length];
  });

  return (
    <div className="dm-graph">
      <div className="dm-graph-header">
        {branches.map(b => (
          <span
            key={b}
            className={`dm-branch-tag ${selectedBranches.has(b) ? 'dm-branch-active' : 'dm-branch-inactive'}`}
            style={{ borderLeft: `4px solid ${branchColor[b] || '#888'}`, cursor: 'pointer' }}
            onClick={() => handleBranchToggle(b)}
            title="클릭으로 브랜치 표시/숨김"
          >
            {b}
          </span>
        ))}
      </div>
      <div className="dm-graph-body">
        <svg width={svgW} height={filteredCommits.length * rowH + 20} style={{ flexShrink: 0 }}>
          {/* 배경 lane 라인 */}
          {sortedLanes.map(lane => (
            <line
              key={`bg-lane-${lane}`}
              x1={x(lane)}
              y1={0}
              x2={x(lane)}
              y2={filteredCommits.length * rowH + 20}
              stroke={laneColors[lane]}
              strokeWidth={1}
              opacity={0.1}
            />
          ))}

          {/* 연결선: 각 커밋 → 부모 커밋 */}
          {filteredCommits.map((c, i) => {
            const lines = [];
            if (!c.parents || c.parents.length === 0) return lines;

            const x1 = x(c.lane);
            const y1 = y(i);

            // 첫 번째 parent (main line)
            const mainParent = c.parents[0];
            const parentIdx = shaIdx[mainParent];
            if (parentIdx !== undefined) {
              const pCommit = filteredCommits[parentIdx];
              const x2 = x(pCommit.lane);
              const y2 = y(parentIdx);

              if (Math.abs(x1 - x2) < 1) {
                // 같은 lane: 수직선
                lines.push(
                  <line
                    key={`line-${c.sha}`}
                    x1={x1} y1={y1 + 10}
                    x2={x2} y2={y2 - 10}
                    stroke={laneColors[c.lane]}
                    strokeWidth={2}
                    strokeLinecap="round"
                  />
                );
              } else {
                // 다른 lane: 곡선
                const midY = (y1 + y2) / 2;
                lines.push(
                  <path
                    key={`line-${c.sha}`}
                    d={`M${x1},${y1 + 10} C${x1},${midY} ${x2},${midY} ${x2},${y2 - 10}`}
                    fill="none"
                    stroke={laneColors[c.lane]}
                    strokeWidth={2}
                    strokeLinecap="round"
                  />
                );
              }
            }

            // 다른 parents (merge lines)
            for (let j = 1; j < c.parents.length; j++) {
              const mergeParent = c.parents[j];
              const mergeParentIdx = shaIdx[mergeParent];
              if (mergeParentIdx !== undefined) {
                const mCommit = filteredCommits[mergeParentIdx];
                const x2 = x(mCommit.lane);
                const y2 = y(mergeParentIdx);

                const midY = (y1 + y2) / 2;
                lines.push(
                  <path
                    key={`merge-${c.sha}-${j}`}
                    d={`M${x1},${y1 + 10} C${x1},${midY} ${x2},${midY} ${x2},${y2 - 10}`}
                    fill="none"
                    stroke={laneColors[mCommit.lane] || '#bbb'}
                    strokeWidth={1.5}
                    strokeLinecap="round"
                    opacity={0.7}
                  />
                );
              }
            }

            return lines;
          })}

          {/* 노드 (Gitea 스타일 다이아몬드) */}
          {filteredCommits.map((c, i) => {
            const isMerge = c.parents?.length > 1;
            const nodeCol = isMerge ? '#e74c3c' : '#27ae60';
            const cx = x(c.lane);
            const cy = y(i);
            const size = 6;

            return (
              <g key={c.sha}>
                {/* 다이아몬드 모양 노드 */}
                <polygon
                  points={`${cx},${cy - size} ${cx + size},${cy} ${cx},${cy + size} ${cx - size},${cy}`}
                  fill={nodeCol}
                  stroke="#fff"
                  strokeWidth={1.5}
                />
              </g>
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
              <span className="dm-graph-author">{c.author}</span>
              <span className="dm-date">{(c.date || '').slice(0, 10)}</span>
              {c.parents?.length > 1 && <span style={{ fontSize: '10px', color: '#999' }}> [merge]</span>}
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
