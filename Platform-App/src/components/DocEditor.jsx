import React, { useState, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { modalAlert, modalConfirm } from '@platform/web-ui/components/AlertModal';
import CommonBar from '@platform/web-ui/components/CommonBar';
import DiffAnalysis from '@platform/web-ui/components/DiffAnalysis';
import config from '../config';
import { subscribe } from '@platform/web-ui/runtime/sseManager';
import './DocEditor.css';
import { authFetch } from '../platform/auth';

const API = config.API_BASE + '/DocRepo';
const PLATFORM_API = config.API_BASE;

const NAV_ITEMS = [
  { key: 'docs', label: 'Docs' },
  { key: 'issues', label: 'Issues' },
  { key: 'relationMap', label: 'Relation Map' },
  { key: 'satam', label: 'SATAM' },
];

function decodeBase64(base64) {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return new TextDecoder('utf-8').decode(bytes);
}

export default function DocEditor({ project, user, onBack, currentPage, onNavigate, projectAlert, onDocStateChange }) {
  const projectCode = project?.jiraCodes?.main?.jiraCode || '';
  const hasRepo = !!project?.documents?.repoName;

  const [mainTree, setMainTree] = useState(null);
  const [userTree, setUserTree] = useState(null);
  const [fileStatus, setFileStatus] = useState({}); // path → 'modified' | 'added' | 'deleted'
  const [activeTab, setActiveTab] = useState('workspace'); // 'main' | 'workspace'
  const [selectedFile, setSelectedFile] = useState(null);
  const [selectedMainOnly, setSelectedMainOnly] = useState(false);
  const [content, setContent] = useState('');
  const [showGraph, setShowGraph] = useState(false);
  const [diffMode, setDiffMode] = useState(false);
  const [diffTab, setDiffTab] = useState('semantic');
  const [mainContent, setMainContent] = useState('');
  const [loadingDiff, setLoadingDiff] = useState(false);

  // 문서 상태 갱신 통합 함수 — 트리 로드 + fileStatus 재계산
  // 모든 문서 변경 포인트에서 이 함수를 호출
  async function refreshDocState(retry = 0) {
    const branch = `docs/${user}`;
    try {
      const mRes = await authFetch(`${API}/${projectCode}/tree?ref=main`);
      if (!mRes.ok) throw new Error(`main tree ${mRes.status}`);
      const mTree = await mRes.json();
      if (!mTree?.tree) throw new Error('invalid main tree');
      setMainTree(mTree);

      const uRes = await authFetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent(branch)}`);
      if (!uRes.ok) throw new Error('user tree failed');
      const uTree = await uRes.json();
      setUserTree(uTree);

      // fileStatus 재계산
      const mainShaMap = {};
      const mainPathSet = new Set();
      (mTree.tree || []).forEach(f => { mainShaMap[f.path] = f.sha; mainPathSet.add(f.path); });
      const userPathSet = new Set();
      const status = {};
      (uTree.tree || []).forEach(f => {
        userPathSet.add(f.path);
        if (!mainPathSet.has(f.path)) status[f.path] = 'added';
        else if (mainShaMap[f.path] !== f.sha) status[f.path] = 'modified';
      });
      mainPathSet.forEach(p => { if (!userPathSet.has(p)) status[p] = 'deleted'; });
      setFileStatus(status);
      // 부모(GoJIRAApp)에 문서 상태 변경 알림 → projectAlert 재계산
      if (onDocStateChange) onDocStateChange();
    } catch {
      if (retry < 1) setTimeout(() => refreshDocState(retry + 1), 1000);
      else { setUserTree(null); setFileStatus({}); }
    }
  }

  // 진입 시 트리 비교(read-only). 과거 여기서 init-files/ensure-branch 를 서버로 POST 했으나
  // local-agent 모델에서 둘 다 제거됨(브랜치/초기파일은 로컬 런처 EnsureRepo 담당). 서버 호출은
  // 404 로 죽어 의미가 없어 제거 — 트리 갱신만 수행한다.
  useEffect(() => {
    if (!hasRepo) return;
    refreshDocState();
  }, [projectCode, hasRepo, user]);

  // SSE 구독 — main 변경 시 실시간 트리 갱신
  useEffect(() => {
    if (!hasRepo) return;
    return subscribe('doc_update', (e) => {
      try {
        const data = JSON.parse(e.data);
        if (data.type === 'project_alert' && data.projectCode === projectCode) {
          refreshDocState();
        }
        if (data.type === 'push' && data.repo) {
          refreshDocState();
        }
      } catch {}
    });
  }, [projectCode, hasRepo, user]);

  async function handleFileClick(path, isMainOnly) {
    // mainOnly 파일이면 main에서 조회
    const ref = isMainOnly ? 'main' : (activeTab === 'workspace' ? `docs/${user}` : 'main');
    const res = await authFetch(`${API}/${projectCode}/file?path=${encodeURIComponent(path)}&ref=${encodeURIComponent(ref)}`);
    if (res.ok) {
      const d = await res.json();
      const text = d.content ? decodeBase64(d.content) : '';
      setSelectedFile(path);
      setSelectedMainOnly(!!isMainOnly);
      setContent(text);
    }
  }

  async function handleDiffToggle() {
    if (diffMode) { setDiffMode(false); return; }
    setLoadingDiff(true);
    try {
      const status = fileStatus[selectedFile];
      let mainText = '';
      if (status !== 'added') {
        const res = await authFetch(`${API}/${projectCode}/file?path=${encodeURIComponent(selectedFile)}&ref=main`);
        if (res.ok) {
          const d = await res.json();
          mainText = d.content ? decodeBase64(d.content) : '';
        }
      }
      setMainContent(mainText);
      setDiffMode(true);
      setDiffTab('semantic');
    } catch (e) {
      modalAlert('Diff 로딩 실패');
    }
    setLoadingDiff(false);
  }

  const barTitleText = `${project?.name || ''} (${projectCode})`;
  const nav = onNavigate ? NAV_ITEMS.map(n => ({
    ...n, active: n.key === currentPage, handler: () => onNavigate(n.key),
  })) : [];

  if (!hasRepo) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <CommonBar title={barTitle} onBack={onBack} nav={nav} />
        <div className="de-empty">설계문서 저장소가 없습니다. 프로젝트맵에서 설계문서를 먼저 생성하세요.</div>
      </div>
    );
  }

  const currentTree = activeTab === 'main' ? mainTree : (userTree || mainTree);
  const baseItems = (currentTree?.tree || []).filter(i => i.type !== 'tree');
  // workspace 탭에서 main에만 있는 파일도 표시 (gray italic)
  let items;
  if (activeTab === 'workspace' && mainTree && userTree) {
    const userPaths = new Set(baseItems.map(i => i.path));
    const mainOnly = (mainTree?.tree || [])
      .filter(i => i.type !== 'tree' && !userPaths.has(i.path))
      .map(i => ({ ...i, _mainOnly: true }));
    items = [...baseItems, ...mainOnly].sort((a, b) => a.path.localeCompare(b.path));
  } else {
    items = baseItems.sort((a, b) => a.path.localeCompare(b.path));
  }
  const hasMainOnly = items.some(i => i._mainOnly);
  const mainOnlyPaths = new Set(items.filter(i => i._mainOnly).map(i => i.path));
  // mainOnly(main에만 있는 파일)는 사용자 변경이 아니므로 제외
  const hasChanges = Object.keys(fileStatus).some(p => !mainOnlyPaths.has(p));
  const barTitle = hasMainOnly ? (
    <>{barTitleText} <svg width="16" height="16" viewBox="0 0 16 16" style={{verticalAlign:'middle',marginLeft:4}}>
      <path d="M8 1L15 14H1L8 1Z" fill="tomato" stroke="#333" strokeWidth="0.8" />
      <text x="8" y="12" textAnchor="middle" fontSize="9" fontWeight="bold" fill="#fff">!</text>
    </svg></>
  ) : barTitleText;

  const statusBadge = (path) => {
    const diffMain = fileStatus[path]; // main과 차이
    if (diffMain) {
      const label = diffMain === 'added' ? 'A' : diffMain === 'deleted' ? 'D' : 'M';
      const cls = `de-file-badge de-badge-${diffMain}`;
      return <span className={cls}>{label}</span>;
    }
    return null;
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <CommonBar title={barTitle} onBack={onBack} nav={nav} projectAlert={projectAlert} />
      <div className="de-editor">
        <div className="de-sidebar">
          <div className="de-sidebar-tabs">
            <button className={`de-sidebar-tab ${activeTab === 'main' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => { setActiveTab('main'); setShowGraph(false); }}>Main</button>
            <button className={`de-sidebar-tab ${activeTab === 'workspace' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => { setActiveTab('workspace'); setShowGraph(false); }}>작업공간</button>
            <button
              className={`de-sidebar-tab de-graph-btn ${showGraph ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => { setShowGraph(!showGraph); if (!showGraph) setSelectedFile(null); }}
              title="Commit Graph"
            >
              <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
                <circle cx="4" cy="3" r="2.2" /><circle cx="4" cy="13" r="2.2" /><circle cx="12" cy="8" r="2.2" />
                <line x1="4" y1="5.2" x2="4" y2="10.8" stroke="currentColor" strokeWidth="1.8" />
                <path d="M4,5.2 C4,8 8,8 12,5.8" fill="none" stroke="currentColor" strokeWidth="1.8" />
              </svg>
            </button>
          </div>
          <div className="de-sidebar-header">
            <span>{activeTab === 'main' ? 'main' : `docs/${user}`}</span>
          </div>
          {activeTab === 'workspace' && !hasChanges && hasMainOnly && (
            <div className="de-sync-notice">
              main에 새 파일이 추가되었습니다. 로컬에서 pull 후 작업하세요.
            </div>
          )}
          {(() => {
            const folders = {};
            const rootFiles = [];
            items.forEach(item => {
              const slash = item.path.indexOf('/');
              if (slash > 0) {
                const folder = item.path.substring(0, slash);
                if (!folders[folder]) folders[folder] = [];
                folders[folder].push(item);
              } else {
                rootFiles.push(item);
              }
            });
            const folderNames = Object.keys(folders).sort();
            const renderFile = (item, displayName) => {
              const isMainOnly = item._mainOnly;
              const st = (activeTab === 'workspace' && !isMainOnly) ? fileStatus[item.path] : null;
              return (
                <div key={item.path}
                  className={`de-file ${selectedFile === item.path ? 'de-file-active' : ''} ${st ? `de-file-${st}` : ''} ${isMainOnly ? 'de-file-main-only' : ''}`}
                  onClick={() => { setShowGraph(false); setDiffMode(false); handleFileClick(item.path, item._mainOnly); }}>
                  {activeTab === 'workspace' && (isMainOnly
                    ? <span className="de-file-badge de-badge-mainonly">M</span>
                    : statusBadge(item.path))}
                  <span className="de-file-name">{displayName || item.path}</span>
                </div>
              );
            };
            return (
              <>
                {folderNames.map(folder => (
                  <React.Fragment key={folder}>
                    <div className="de-folder-header">
                      <span className="de-folder-icon">📁</span> {folder}
                    </div>
                    {folders[folder].map(item => (
                      <div key={item.path} style={{ paddingLeft: 16 }}>
                        {renderFile(item, item.path.substring(folder.length + 1))}
                      </div>
                    ))}
                  </React.Fragment>
                ))}
                {rootFiles.map(item => renderFile(item))}
              </>
            );
          })()}
        </div>
        <div className="de-main">
          {showGraph ? (
            <DocGitGraph projectCode={projectCode} />
          ) : selectedFile ? (
            <>
              <div className="de-toolbar">
                <span className="de-filename">
                  {selectedFile}
                  {selectedMainOnly && <span style={{fontSize:11,color:'#aaa',marginLeft:6}}>(main only — Pull로 동기화 필요)</span>}
                </span>
                <div className="de-actions">
                  {selectedMainOnly ? (
                    <button className="de-tab de-tab-active">미리보기</button>
                  ) : (
                    <>
                      <button className={`de-tab ${!diffMode ? 'de-tab-active' : ''}`} onClick={() => setDiffMode(false)}>미리보기</button>
                      {activeTab === 'workspace' && fileStatus[selectedFile] && (
                        <button
                          className={`de-btn ${diffMode ? 'de-btn-primary' : ''}`}
                          onClick={handleDiffToggle}
                          disabled={loadingDiff}
                        >
                          {loadingDiff ? '로딩...' : 'Diff 분석'}
                        </button>
                      )}
                    </>
                  )}
                </div>
              </div>
              <div className="de-body">
                {diffMode ? (
                  <DiffAnalysis oldText={mainContent} newText={content} fileName={selectedFile} activeTab={diffTab} onTabChange={setDiffTab} />
                ) : (
                  <div className={`de-preview ${selectedMainOnly ? 'de-preview-mainonly' : ''}`}>
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
                  </div>
                )}
              </div>
            </>
          ) : (
            <div className="de-placeholder">파일을 선택하세요</div>
          )}
        </div>
      </div>
    </div>
  );
}

/* ─── Commit Graph (DocManager의 GitGraph와 동일 로직) ─── */
function DocGitGraph({ projectCode }) {
  const [data, setData] = useState(null);
  const [offset, setOffset] = useState(0);
  const limit = 20;

  useEffect(() => {
    authFetch(`${API}/${projectCode}/graph?offset=${offset}&limit=${limit}`)
      .then(r => r.json()).then(d => {
        const shaToCommit = {};
        (d.commits || []).forEach(c => { shaToCommit[c.sha] = c; });
        d.commits?.forEach(c => {
          if (c.parents?.length > 1) {
            c.lanes = [];
            for (let i = 1; i < c.parents.length; i++) {
              const parentSha = c.parents[i];
              const parentCommit = shaToCommit[parentSha];
              if (parentCommit && parentCommit.lane !== undefined) c.lanes.push(parentCommit.lane);
            }
          }
        });
        if (offset === 0) setData(d);
        else setData(prev => ({ ...prev, commits: [...(prev?.commits || []), ...(d.commits || [])], hasMore: d.hasMore }));
      }).catch(() => {});
  }, [projectCode, offset]);

  if (!data) return <div className="de-graph-wrap">로딩 중...</div>;
  const { branches = [], commits = [], hasMore = false } = data;

  const shaMap = {};
  commits.forEach(c => { shaMap[c.sha] = c; });
  const mainHead = commits.find(c => c.refs?.includes('main'));
  const mainChain = new Set();
  if (mainHead) {
    let sha = mainHead.sha;
    while (sha && shaMap[sha]) { mainChain.add(sha); sha = shaMap[sha].parents?.[0]; }
  }
  const branchLaneNum = { main: 0 };
  let nextLane = 1;
  branches.filter(b => b !== 'main').forEach(b => { branchLaneNum[b] = nextLane++; });

  const laneOf = {};
  mainChain.forEach(sha => { laneOf[sha] = 0; });
  mainChain.forEach(sha => {
    const c = shaMap[sha];
    if (c.parents?.length > 1) {
      const secondSha = c.parents[1];
      const secondCommit = shaMap[secondSha];
      if (!secondCommit) return;
      let branchName = secondCommit.refs?.find(r => branchLaneNum[r] !== undefined);
      if (!branchName) {
        const msg = c.message || '';
        const match = msg.match(/from\s+(\S+)/);
        if (match) {
          const foundBranch = branches.find(b => match[1].includes(b));
          if (foundBranch) branchName = foundBranch;
        }
      }
      const lane = branchName ? branchLaneNum[branchName] : (nextLane++);
      let cur = secondSha;
      while (cur && shaMap[cur] && laneOf[cur] === undefined) { laneOf[cur] = lane; cur = shaMap[cur].parents?.[0]; }
    }
  });
  branches.filter(b => b !== 'main').forEach(b => {
    const head = commits.find(c => c.refs?.includes(b));
    if (head && laneOf[head.sha] === undefined) {
      const lane = branchLaneNum[b];
      let sha = head.sha;
      while (sha && shaMap[sha] && laneOf[sha] === undefined) { laneOf[sha] = lane; sha = shaMap[sha].parents?.[0]; }
    }
  });
  commits.forEach(c => { if (laneOf[c.sha] !== undefined) c.lane = laneOf[c.sha]; });

  const filteredCommits = [...commits].sort((a, b) => new Date(b.date || 0) - new Date(a.date || 0));

  const colors = ['#6b9bc3', '#7fb685', 'tomato', '#d4a862', '#8a8dc9', '#6abfb0', '#c4876e'];
  const branchColor = {};
  branches.forEach((b, i) => { branchColor[b] = colors[i % colors.length]; });

  const shaIdx = {};
  const usedLanes = new Set();
  filteredCommits.forEach((c, i) => { shaIdx[c.sha] = i; usedLanes.add(c.lane); });
  const sortedLanes = Array.from(usedLanes).sort((a, b) => a - b);

  const laneToBranch = {};
  Object.entries(branchLaneNum).forEach(([b, l]) => { laneToBranch[l] = b; });
  const laneColors = {};
  sortedLanes.forEach(lane => {
    const branch = laneToBranch[lane];
    laneColors[lane] = branch && branchColor[branch] ? branchColor[branch] : '#999';
  });

  const nodeR = 7, rowH = 40, colW = 32;
  const maxLane = sortedLanes.length > 0 ? Math.max(...sortedLanes) : 0;
  const svgW = (maxLane + 1) * colW + 20;
  const x = lane => lane * colW + 16;
  const y = i => i * rowH + 20;

  return (
    <div className="de-graph-wrap">
      <div className="de-graph-header">
        {branches.map(b => (
          <span key={b} className="de-graph-branch" style={{ borderLeft: `4px solid ${branchColor[b] || '#888'}` }}>{b}</span>
        ))}
      </div>
      <div className="de-graph-body">
        <svg width={svgW} height={filteredCommits.length * rowH + 20} style={{ flexShrink: 0 }}>
          {filteredCommits.map((c, i) => {
            const lines = [];
            if (!c.parents || c.parents.length === 0) return lines;
            const cx = x(c.lane), cy = y(i);
            c.parents.forEach((parentSha, pIdx) => {
              const pi = shaIdx[parentSha];
              if (pi === undefined) return;
              const parent = filteredCommits[pi];
              const px = x(parent.lane), py = y(pi);
              const lineColor = laneColors[parent.lane] || '#999';
              if (c.lane === parent.lane) {
                lines.push(<line key={`${c.sha}-p${pIdx}`} x1={cx} y1={cy + nodeR} x2={px} y2={py - nodeR} stroke={lineColor} strokeWidth={3} strokeLinecap="round" />);
              } else {
                lines.push(<path key={`${c.sha}-p${pIdx}`} d={`M${cx},${cy + nodeR} C${cx},${cy + rowH * 0.8} ${px},${py - rowH * 0.8} ${px},${py - nodeR}`} fill="none" stroke={lineColor} strokeWidth={3} strokeLinecap="round" />);
              }
            });
            return lines;
          })}
          {filteredCommits.map((c, i) => (
            <circle key={c.sha} cx={x(c.lane)} cy={y(i)} r={nodeR} fill={laneColors[c.lane] || '#999'} stroke="#fff" strokeWidth={2.5} />
          ))}
        </svg>
        <div className="de-graph-list">
          {filteredCommits.map(c => (
            <div key={c.sha} className="de-graph-row" style={{ height: rowH }}>
              <span className="de-graph-sha">{(c.sha || '').slice(0, 7)}</span>
              {c.refs?.map(ref => (
                <span key={ref} className="de-graph-ref" style={{ background: branchColor[ref] || '#888' }}>{ref}</span>
              ))}
              <span className="de-graph-msg">{(c.message || '').split('\n')[0]}</span>
              <span className="de-graph-author">{c.author}</span>
              <span className="de-graph-date">{(c.date || '').slice(0, 10)}</span>
            </div>
          ))}
          {hasMore && (
            <div style={{ textAlign: 'center', padding: '10px' }}>
              <button className="de-btn" onClick={() => setOffset(offset + limit)}>더 보기</button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
