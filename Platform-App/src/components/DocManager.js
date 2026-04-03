import React, { useEffect, useState, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import config from '../config';
import './DocManager.css';

const API = config.API_BASE + '/DocRepo';
const LINK_API = config.API_BASE + '/DocLink';

export default function DocManager({ user }) {
  const [repos, setRepos] = useState([]);
  const [selectedRepo, setSelectedRepo] = useState(null);
  const [view, setView] = useState('tree'); // tree | commits | prs | graph

  const fetchRepos = useCallback(() => {
    fetch(API).then(r => r.json()).then(d => setRepos(Array.isArray(d) ? d : [])).catch(() => {});
  }, []);

  useEffect(() => { fetchRepos(); }, [fetchRepos]);

  async function handleCreateRepo() {
    const name = prompt('Repo 이름 (예: GOJIRA-docs)');
    if (!name) return;
    const projectId = prompt('프로젝트 ID');
    await fetch(API, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ repoName: name, projectId, description: name + ' 설계 문서', createdBy: user }),
    });
    fetchRepos();
  }

  return (
    <div className="dm-wrapper">
      <div className="dm-sidebar">
        <div className="dm-sidebar-header">
          <span>Repositories</span>
          <button className="dm-add-btn" onClick={handleCreateRepo}>+</button>
        </div>
        {repos.map(r => (
          <div key={r._id} className={`dm-repo-item ${selectedRepo?._id === r._id ? 'dm-repo-active' : ''}`}
            onClick={() => { setSelectedRepo(r); setView('tree'); }}>
            <span className="dm-repo-name">{r.repoName}</span>
            <span className="dm-repo-owner">{r.repoOwner}</span>
          </div>
        ))}
      </div>

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
    fetch(`${API}/${repo._id}/tree?ref=${repo.defaultBranch || 'main'}`)
      .then(r => r.json())
      .then(d => setTree(d))
      .catch(() => {});
    setSelectedFile(null);
    setContent('');
  }, [repo]);

  function handleFileClick(path) {
    setSelectedFile(path);
    fetch(`${API}/${repo._id}/file?path=${encodeURIComponent(path)}&ref=${repo.defaultBranch || 'main'}`)
      .then(r => r.json())
      .then(d => {
        if (d.content && d.encoding === 'base64') {
          setContent(atob(d.content));
        } else {
          setContent(d.content || '');
        }
      })
      .catch(() => setContent('파일을 불러올 수 없습니다.'));
  }

  const items = tree?.tree || [];
  // 폴더/파일 구분 정렬
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
    fetch(`${API}/${repo._id}/commits?branch=${repo.defaultBranch || 'main'}&perPage=50`)
      .then(r => r.json())
      .then(d => setCommits(Array.isArray(d) ? d : []))
      .catch(() => {});
  }, [repo]);

  return (
    <div className="dm-commits">
      <table className="dm-table">
        <thead>
          <tr><th>SHA</th><th>메시지</th><th>작성자</th><th>일시</th></tr>
        </thead>
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

  const fetchPRs = useCallback(() => {
    fetch(`${API}/${repo._id}/prs?state=${filter}`)
      .then(r => r.json())
      .then(d => setPRs(Array.isArray(d) ? d : []))
      .catch(() => {});
  }, [repo, filter]);

  useEffect(() => { fetchPRs(); }, [fetchPRs]);

  async function handleReview(prNum, event) {
    const body = event === 'REQUEST_CHANGES' ? prompt('반려 사유') : '';
    if (event === 'REQUEST_CHANGES' && !body) return;
    await fetch(`${API}/${repo._id}/pr/${prNum}/review`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ event, body: body || '' }),
    });
    fetchPRs();
  }

  async function handleMerge(prNum) {
    if (!window.confirm('머지하시겠습니까?')) return;
    await fetch(`${API}/${repo._id}/pr/${prNum}/merge`, { method: 'POST' });
    fetchPRs();
  }

  const auditors = repo.auditors || [];
  const isAuditor = auditors.includes(user);

  return (
    <div className="dm-prs">
      <div className="dm-pr-toolbar">
        <div className="dm-pr-filters">
          {['open', 'closed', 'all'].map(f => (
            <button key={f} className={`dm-tab ${filter === f ? 'dm-tab-active' : ''}`} onClick={() => setFilter(f)}>{f}</button>
          ))}
        </div>
        <div className="dm-pr-auditors">
          승인자: {auditors.length > 0 ? auditors.join(', ') : <span className="dm-warn">미지정</span>}
        </div>
      </div>
      <table className="dm-table">
        <thead>
          <tr><th>#</th><th>제목</th><th>상태</th><th>작성자</th><th>일시</th><th></th></tr>
        </thead>
        <tbody>
          {prs.map(pr => (
            <tr key={pr.number}>
              <td>{pr.number}</td>
              <td>{pr.title}</td>
              <td><span className={`dm-pr-state dm-pr-${pr.state}`}>{pr.state}</span></td>
              <td>{pr.user?.login}</td>
              <td className="dm-date">{(pr.created_at || '').slice(0, 10)}</td>
              <td className="dm-pr-actions">
                {pr.state === 'open' && isAuditor && (
                  <>
                    <button className="dm-approve-btn" onClick={() => handleReview(pr.number, 'APPROVE')}>승인</button>
                    <button className="dm-reject-btn" onClick={() => handleReview(pr.number, 'REQUEST_CHANGES')}>반려</button>
                  </>
                )}
                {pr.state === 'open' && pr.mergeable !== false && (
                  <button className="dm-merge-btn" onClick={() => handleMerge(pr.number)}>머지</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// --- Git Graph (간단 SVG) ---
function GitGraph({ repo }) {
  const [commits, setCommits] = useState([]);
  const [branches, setBranches] = useState([]);

  useEffect(() => {
    fetch(`${API}/${repo._id}/commits?branch=${repo.defaultBranch || 'main'}&perPage=30`)
      .then(r => r.json()).then(d => setCommits(Array.isArray(d) ? d : [])).catch(() => {});
    fetch(`${API}/${repo._id}/branches`)
      .then(r => r.json()).then(d => setBranches(Array.isArray(d) ? d : [])).catch(() => {});
  }, [repo]);

  const rowH = 32;
  const svgW = 80;
  const dotR = 5;

  return (
    <div className="dm-graph">
      <div className="dm-graph-header">
        브랜치: {branches.map(b => (
          <span key={b.name} className="dm-branch-tag">{b.name}</span>
        ))}
      </div>
      <div className="dm-graph-body">
        <svg width={svgW} height={commits.length * rowH + 20}>
          {/* 세로 라인 */}
          <line x1={20} y1={10} x2={20} y2={commits.length * rowH + 10} stroke="#aaa" strokeWidth={2} />
          {/* 커밋 노드 */}
          {commits.map((c, i) => (
            <circle key={c.sha} cx={20} cy={i * rowH + 20} r={dotR} fill={i === 0 ? 'tomato' : '#2c3e50'} />
          ))}
        </svg>
        <div className="dm-graph-list">
          {commits.map(c => (
            <div key={c.sha} className="dm-graph-row" style={{ height: rowH }}>
              <span className="dm-sha">{(c.sha || '').slice(0, 7)}</span>
              <span className="dm-graph-msg">{c.commit?.message?.split('\n')[0]}</span>
              <span className="dm-graph-author">{c.commit?.author?.name}</span>
              <span className="dm-date">{(c.commit?.author?.date || '').slice(0, 10)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
