import React, { useState, useEffect, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { modalAlert, modalConfirm } from './AlertModal';
import CommonBar from './CommonBar';
import DiffAnalysis from './DiffAnalysis';
import MermaidBlock from './MermaidBlock';
import PexBlock from './PexBlock';
import config from '../config';
import { subscribe } from '../utils/sseManager';
import { detectLauncher, requestEdit, requestPush } from '../utils/launcher';
import './DocEditor.css';

const API = config.PLATFORM_API + '/DocRepo';
const PLATFORM_API = config.PLATFORM_API;

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

// 가벼운 YAML 뷰어 — 들여쓰기 보존 + 키/값/주석/문자열 색상화 (라인 단위 정규식)
function YamlView({ text }) {
  const lines = (text || '').split('\n');
  return (
    <pre className="de-yaml">
      {lines.map((raw, i) => (
        <div key={i} className="de-yaml-line">{renderYamlLine(raw)}</div>
      ))}
    </pre>
  );
}

function renderYamlLine(line) {
  // 주석
  const cm = line.match(/^(\s*)(#.*)$/);
  if (cm) {
    return (<><span>{cm[1]}</span><span className="de-yaml-comment">{cm[2]}</span></>);
  }
  // 리스트 시작 "- "
  const listMatch = line.match(/^(\s*)(-\s+)(.*)$/);
  if (listMatch) {
    return (
      <>
        <span>{listMatch[1]}</span>
        <span className="de-yaml-bullet">{listMatch[2]}</span>
        {renderYamlValue(listMatch[3])}
      </>
    );
  }
  // key: value
  const kv = line.match(/^(\s*)([^\s:][^:]*?)(\s*:)(\s*)(.*)$/);
  if (kv) {
    const [, indent, key, colon, sp, value] = kv;
    return (
      <>
        <span>{indent}</span>
        <span className="de-yaml-key">{key}</span>
        <span className="de-yaml-colon">{colon}</span>
        <span>{sp}</span>
        {renderYamlValue(value)}
      </>
    );
  }
  return <span>{line || '\u00A0'}</span>;
}

function renderYamlValue(v) {
  if (v === '' || v === undefined) return null;
  // 인용 문자열
  if (/^['"].*['"]$/.test(v)) return <span className="de-yaml-string">{v}</span>;
  // 숫자/불린/null
  if (/^(true|false|null|~|\d+(\.\d+)?)$/.test(v)) return <span className="de-yaml-literal">{v}</span>;
  // 블록 스칼라 표시
  if (/^[|>][-+]?\d*$/.test(v)) return <span className="de-yaml-block">{v}</span>;
  // flow 시퀀스/맵 [...] {...}
  if (/^[\[{].*[\]}]$/.test(v)) return <span className="de-yaml-flow">{v}</span>;
  return <span className="de-yaml-value">{v}</span>;
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
  const [launcher, setLauncher] = useState(null); // 로컬 런처 감지 결과 { ready, agents, token } | null
  const [launching, setLaunching] = useState(false); // /edit 호출 진행 중
  const [publishing, setPublishing] = useState(false); // 로컬 push→PR 진행 중
  const [showGraph, setShowGraph] = useState(false);
  const [collapsedFolders, setCollapsedFolders] = useState(() => new Set());
  const toggleFolder = (path) => {
    setCollapsedFolders(prev => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path); else next.add(path);
      return next;
    });
  };
  const [diffMode, setDiffMode] = useState(false);
  const [diffTab, setDiffTab] = useState('semantic');
  const [mainContent, setMainContent] = useState('');
  const [loadingDiff, setLoadingDiff] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState(() => {
    const saved = parseInt(localStorage.getItem('docEditorSidebarWidth'), 10);
    return Number.isFinite(saved) && saved >= 150 && saved <= 600 ? saved : 220;
  });
  const [resizing, setResizing] = useState(false);
  useEffect(() => {
    localStorage.setItem('docEditorSidebarWidth', String(sidebarWidth));
  }, [sidebarWidth]);
  const handleResizerMouseDown = (e) => {
    e.preventDefault();
    const startX = e.clientX;
    const startWidth = sidebarWidth;
    setResizing(true);
    const onMove = (ev) => {
      const next = Math.max(150, Math.min(600, startWidth + ev.clientX - startX));
      setSidebarWidth(next);
    };
    const onUp = () => {
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      document.body.style.userSelect = '';
      document.body.style.cursor = '';
      setResizing(false);
    };
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
    document.body.style.userSelect = 'none';
    document.body.style.cursor = 'col-resize';
  };
  // 문서 상태 갱신 통합 함수 — 트리 로드 + fileStatus 재계산
  async function refreshDocState(retry = 0) {
    const branch = `docs/${user}`;
    try {
      const mRes = await fetch(`${API}/${projectCode}/tree?ref=main`);
      if (!mRes.ok) throw new Error(`main tree ${mRes.status}`);
      const mTree = await mRes.json();
      if (!mTree?.tree) throw new Error('invalid main tree');
      setMainTree(mTree);

      const uRes = await fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent(branch)}`);
      if (!uRes.ok) throw new Error('user tree failed');
      const uTree = await uRes.json();
      setUserTree(uTree);

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
      if (onDocStateChange) onDocStateChange();
    } catch {
      if (retry < 1) setTimeout(() => refreshDocState(retry + 1), 1000);
      else { setUserTree(null); setFileStatus({}); }
    }
  }

  // 외부 변경(SSE push) 으로 현재 열린 파일 재로드 (웹은 조회 전용).
  // SSE 구독 콜백이 매 렌더의 state 를 참조하도록 ref 경유.
  const reloadRef = React.useRef(null);
  reloadRef.current = async () => {
    if (!selectedFile) return;
    const ref = selectedMainOnly ? 'main' : (activeTab === 'workspace' ? `docs/${user}` : 'main');
    try {
      const res = await fetch(`${API}/${projectCode}/file?path=${encodeURIComponent(selectedFile)}&ref=${encodeURIComponent(ref)}`);
      if (!res.ok) return;
      const d = await res.json();
      const text = d.content ? decodeBase64(d.content) : '';
      if (text !== content) setContent(text);
    } catch {}
  };
  async function reloadSelectedFile() { return reloadRef.current && reloadRef.current(); }

  // 진입 시 트리 상태 갱신만. 웹은 조회 전용 — 브랜치 생성/파일 초기화는 런처(로컬 clone)가
  // 담당하므로 서버 init-files/ensure-branch 호출 제거 (트랙 E — 서버 worktree 제거).
  useEffect(() => {
    if (!hasRepo) return;
    refreshDocState();
  }, [projectCode, hasRepo, user]); // eslint-disable-line

  // WebSocket 구독 — main 변경 시 실시간 트리 갱신
  useEffect(() => {
    if (!hasRepo) return;
    return subscribe('doc_update', (e) => {
      try {
        const data = JSON.parse(e.data);
        if (data.type === 'project_alert' && data.projectCode === projectCode) {
          refreshDocState();
          reloadSelectedFile();
        }
        if (data.type === 'push' && data.repo) {
          refreshDocState();
          reloadSelectedFile();
        }
        if (data.type === 'user_doc_update' && data.projectCode === projectCode && data.user === user) {
          refreshDocState();
          reloadSelectedFile();
        }
      } catch {}
    });
  }, [projectCode, hasRepo, user]); // eslint-disable-line

  // 로컬 런처 감지 — 기동/설치돼 있으면 "로컬 수정" 버튼을 활성화 (local-agent-mvp §2.1).
  // 마운트 시 + 창 포커스 복귀 시 재감지(런처를 나중에 켰을 수 있으므로).
  useEffect(() => {
    let alive = true;
    const probe = () => detectLauncher().then((r) => { if (alive) setLauncher(r); });
    probe();
    window.addEventListener('focus', probe);
    return () => { alive = false; window.removeEventListener('focus', probe); };
  }, []);

  // "로컬 수정" — 런처에 /edit 요청해 VS Code + CLI(opencode|claude)를 소환.
  // 웹은 조회 전용이며, 편집은 사용자 PC 로컬 작업본에서 이뤄진다 (local-agent-mvp §4).
  async function handleLocalEdit() {
    if (!launcher || !selectedFile) return;
    setLaunching(true);
    try {
      const res = await requestEdit({
        code: projectCode,
        user,
        file: selectedFile,
        token: launcher.token,
      });
      if (res.ok) {
        modalAlert(`로컬에서 "${selectedFile}" 을(를) 열었습니다.\nVS Code 와 ${res.data.agent || '에이전트'} 가 실행됩니다. 편집 후 push → PR 로 반영하세요.`);
      } else {
        modalAlert(res.error || '로컬 런처 호출에 실패했습니다.');
      }
    } finally {
      setLaunching(false);
    }
  }

  async function handleFileClick(path, isMainOnly) {
    // mainOnly 파일이면 main에서 조회
    const ref = isMainOnly ? 'main' : (activeTab === 'workspace' ? `docs/${user}` : 'main');
    const res = await fetch(`${API}/${projectCode}/file?path=${encodeURIComponent(path)}&ref=${encodeURIComponent(ref)}`);
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
        const res = await fetch(`${API}/${projectCode}/file?path=${encodeURIComponent(selectedFile)}&ref=main`);
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


  // 로컬 push → 발행(PR). 로컬 모델에서는 커밋이 사용자 PC 에 있으므로
  // (1) 런처가 docs/{user} 를 push 한 뒤 (2) 서버 /pr 을 skipAutoCommit 으로 호출한다.
  // 서버측 worktree commit 을 건너뛰어 로컬에서 올린 변경분으로 PR 을 만든다 (§5-3).
  async function handleLocalPublish() {
    if (!launcher || !launcher.token) return;
    if (!await modalConfirm(`로컬 docs/${user} 브랜치를 push 하고 발행 신청(PR)하시겠습니까?`)) return;
    setPublishing(true);
    try {
      const push = await requestPush({ code: projectCode, user, token: launcher.token });
      if (!push.ok) {
        modalAlert(push.error || '로컬 push 에 실패했습니다.');
        return;
      }
      if (push.data && push.data.pushed === false) {
        modalAlert(push.data.message || 'push 할 새 커밋이 없습니다. VS Code 에서 변경을 commit 한 뒤 다시 시도하세요.');
        return;
      }
      // push 성공 → 서버 worktree commit 없이 PR 생성.
      const res = await fetch(`${API}/${projectCode}/pr`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
        body: JSON.stringify({
          title: `[${projectCode}] 설계문서 배포 요청`,
          head: `docs/${user}`,
          skipAutoCommit: true,
        }),
      });
      if (res.ok) {
        modalAlert('로컬 변경을 push 하고 발행 신청(PR)을 생성했습니다.');
        refreshDocState();
      } else {
        const err = await res.json().catch(() => ({}));
        modalAlert(err.error || 'PR 생성 실패 (push 는 완료됨)');
      }
    } finally {
      setPublishing(false);
    }
  }

  const barTitleText = `${project?.name || ''} (${projectCode})`;
  const nav = onNavigate ? NAV_ITEMS.map(n => ({
    ...n, active: n.key === currentPage, handler: () => onNavigate(n.key),
  })) : [];

  if (!hasRepo) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <CommonBar title={barTitleText} onBack={onBack} nav={nav} />
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
  const mainOnlyPaths = new Set(items.filter(i => i._mainOnly).map(i => i.path));
  const hasChanges = Object.keys(fileStatus).some(p => !mainOnlyPaths.has(p));
  const hasMainOnly = items.some(i => i._mainOnly);
  const barTitle = hasMainOnly ? (
    <>{barTitleText} <svg width="16" height="16" viewBox="0 0 16 16" style={{verticalAlign:'middle',marginLeft:4}}>
      <path d="M8 1L15 14H1L8 1Z" fill="tomato" stroke="#333" strokeWidth="0.8" />
      <text x="8" y="12" textAnchor="middle" fontSize="9" fontWeight="bold" fill="#fff">!</text>
    </svg></>
  ) : barTitleText;

  const statusBadge = (path) => {
    const diffMain = fileStatus[path]; // main과 차이
    if (diffMain) {
      // main과 차이: M/A/D
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
        <div className="de-sidebar" style={{ width: sidebarWidth }}>
          <div className="de-sidebar-tabs">
            <button className={`de-sidebar-tab ${activeTab === 'main' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => { setActiveTab('main'); setShowGraph(false); }}>Main</button>
            <button className={`de-sidebar-tab ${activeTab === 'workspace' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => { setActiveTab('workspace'); setShowGraph(false); }}>작업공간</button>
            {/* 작업공간 폴더 열기(서버 open-folder)는 런처(로컬 clone)로 대체 — 제거 (트랙 E) */}
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
            {activeTab === 'workspace' && launcher?.ready && (
              <button
                className="de-btn"
                style={{ marginLeft: 'auto', padding: '3px 10px', fontSize: '11px' }}
                onClick={handleLocalPublish}
                disabled={publishing}
                title="로컬에서 commit 한 변경을 push 하고 발행 신청(PR)합니다"
              >
                {publishing ? '발행 중...' : '로컬 push & 발행'}
              </button>
            )}
          </div>
          {activeTab === 'workspace' && !hasChanges && hasMainOnly && (
            <div className="de-sync-notice">
              main에 새 파일이 추가되었습니다. Git Pull로 동기화 후 작업하세요.
            </div>
          )}
          {(() => {
            // 다단 폴더 트리 구성
            const root = { dirs: {}, files: [] };
            items.forEach(item => {
              const parts = item.path.split('/');
              let node = root;
              for (let i = 0; i < parts.length - 1; i++) {
                const seg = parts[i];
                if (!node.dirs[seg]) node.dirs[seg] = { dirs: {}, files: [] };
                node = node.dirs[seg];
              }
              node.files.push({ ...item, _name: parts[parts.length - 1] });
            });
            const renderFile = (item) => {
              const st = (activeTab === 'workspace' && !item._mainOnly) ? fileStatus[item.path] : null;
              const isMainOnly = item._mainOnly;
              return (
                <div key={item.path}
                  className={`de-file ${selectedFile === item.path ? 'de-file-active' : ''} ${st ? `de-file-${st}` : ''} ${isMainOnly ? 'de-file-main-only' : ''}`}
                  onClick={() => { setShowGraph(false); setDiffMode(false); handleFileClick(item.path, item._mainOnly); }}>
                  {activeTab === 'workspace' && (isMainOnly
                    ? <span className="de-file-badge de-badge-mainonly">M</span>
                    : statusBadge(item.path))}
                  <span className="de-file-name">{item._name || item.path}</span>
                </div>
              );
            };
            const renderNode = (node, prefix, depth) => {
              const dirNames = Object.keys(node.dirs).sort();
              return (
                <>
                  {dirNames.map(name => {
                    const fullPath = prefix ? `${prefix}/${name}` : name;
                    const collapsed = collapsedFolders.has(fullPath);
                    return (
                      <React.Fragment key={fullPath}>
                        <div className="de-folder-header"
                          style={{ paddingLeft: 12 + depth * 12, cursor: 'pointer' }}
                          onClick={() => toggleFolder(fullPath)}>
                          <span className="de-folder-toggle">{collapsed ? '▶' : '▼'}</span>
                          <span className="de-folder-icon">📁</span> {name}
                        </div>
                        {!collapsed && (
                          <div style={{ paddingLeft: (depth + 1) * 12 }}>
                            {renderNode(node.dirs[name], fullPath, depth + 1)}
                          </div>
                        )}
                      </React.Fragment>
                    );
                  })}
                  {node.files.map(item => renderFile(item))}
                </>
              );
            };
            return renderNode(root, '', 0);
          })()}
        </div>
        <div className={`de-resizer${resizing ? ' active' : ''}`} onMouseDown={handleResizerMouseDown} />
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
                      {launcher?.ready && (
                        <button
                          className="de-btn de-btn-primary"
                          onClick={handleLocalEdit}
                          disabled={launching}
                          title="로컬 VS Code + 에이전트(opencode/claude)로 이 문서를 엽니다"
                        >
                          {launching ? '여는 중...' : '로컬 수정'}
                        </button>
                      )}
                    </>
                  )}
                </div>
              </div>
              <div className="de-body" style={{ position: 'relative' }}>
                {diffMode ? (
                  <DiffAnalysis oldText={mainContent} newText={content} fileName={selectedFile} activeTab={diffTab} onTabChange={setDiffTab} />
                ) : (
                  <div className={`de-preview ${selectedMainOnly ? 'de-preview-mainonly' : ''}`}>
                    {/\.ya?ml$/i.test(selectedFile || '') ? (
                      <YamlView text={content} />
                    ) : (
                      <ReactMarkdown
                        remarkPlugins={[remarkGfm]}
                        components={{
                          code({ className, children, ...props }) {
                            const lang = /language-(\w+)/.exec(className || '')?.[1];
                            if (lang === 'mermaid') {
                              return <MermaidBlock code={String(children).replace(/\n$/, '')} />;
                            }
                            if (lang === 'plantuml' || lang === 'puml') {
                              const src = String(children).replace(/\n$/, '');
                              // 웹은 조회 전용 — 인라인 편집은 로컬 pex-vscode 로 이전 (local-agent-mvp §4).
                              return (
                                <PexBlock source={src} readOnly />
                              );
                            }
                            return <code className={className} {...props}>{children}</code>;
                          },
                          pre({ children, ...props }) {
                            // mermaid/plantuml 코드블록은 <pre> 래퍼를 없애 회색 박스가 생기지 않도록 처리
                            const arr = React.Children.toArray(children);
                            const only = arr.length === 1 ? arr[0] : null;
                            const cls = only?.props?.className || '';
                            if (cls.includes('language-mermaid') || cls.includes('language-plantuml') || cls.includes('language-puml')) {
                              return <>{children}</>;
                            }
                            return <pre {...props}>{children}</pre>;
                          },
                        }}
                      >{content}</ReactMarkdown>
                    )}
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
  const limit = 30;

  useEffect(() => {
    fetch(`${API}/${projectCode}/graph?offset=${offset}&limit=${limit}`)
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
