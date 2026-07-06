import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { modalAlert, modalConfirm, modalPrompt } from './AlertModal';
import CommonBar from './CommonBar';
import DiffAnalysis from './DiffAnalysis';
import MermaidBlock from './MermaidBlock';
import PexBlock from './PexBlock';
import config from '../config';
import { subscribe } from '../utils/sseManager';
import { detectLauncher, requestEdit, requestPush, requestFiles, requestLocalFile, requestSync, requestWrite, requestPullFromMain, requestDelete, requestRename } from '../utils/launcher';
import LauncherInstallModal from './LauncherInstallModal';
import './DocEditor.css';
import { authFetch } from '../platform/auth';

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
  const [localTree, setLocalTree] = useState(null);
  const [fileStatus, setFileStatus] = useState({}); // path → 'modified' | 'added' | 'deleted'
  const [fileCommitted, setFileCommitted] = useState({}); // path → bool — true: 차이가 docs/{user} HEAD 에 커밋됨(+), false: 미커밋(-)
  const [ahead, setAhead] = useState(0); // docs/{user} HEAD 가 upstream 대비 앞서 있는 commit 수. -1 = upstream 미설정(신규 브랜치).
  const [activeTab, setActiveTab] = useState('local'); // 'main' | 'local'
  const [selectedFile, setSelectedFile] = useState(null);
  const [selectedMainOnly, setSelectedMainOnly] = useState(false);
  const [content, setContent] = useState('');
  const [launcher, setLauncher] = useState(null); // 로컬 런처 감지 결과 { ready, agents, token } | null
  const [launching, setLaunching] = useState(false); // /edit 호출 진행 중
  const [loadingFiles, setLoadingFiles] = useState(false); // 런처 /files 호출 진행 중 (clone 미존재 시 프로비저닝 동안 블록됨)
  const [installOpen, setInstallOpen] = useState(false); // 설치 안내 모달
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
  const [viewMode, setViewMode] = useState('preview'); // 'preview' | 'raw' | 'diff' — 셋 중 하나만.
  const diffMode = viewMode === 'diff'; // 기존 변수명을 유지해 하위 분기 안 깨지게.
  const [editing, setEditing] = useState(false); // 인라인 편집 모드 여부.
  const [editContent, setEditContent] = useState(''); // textarea 바인딩. 진입 시 content 로 프리필.
  const [saving, setSaving] = useState(false);
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
  // 프로비저닝 인디케이터 — /files 가 350ms 넘게 걸릴 때만 노출.
  // clone 이 이미 있는 빠른 재로드(수십 ms)에선 깜빡이지 않고, 최초 clone 같은 느린 경우만 표시.
  const [showProvisioning, setShowProvisioning] = useState(false);
  useEffect(() => {
    if (!loadingFiles) { setShowProvisioning(false); return; }
    const t = setTimeout(() => setShowProvisioning(true), 350);
    return () => clearTimeout(t);
  }, [loadingFiles]);
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
  // 문서 상태 갱신 — Main 은 서버 Gitea, Local 은 런처(로컬 clone working tree)에서 가져온다.
  // 런처 미감지 시 localTree=null → Local 탭은 빈 상태 + 설치 안내(설계: 데이터소스 분리).
  // fileStatus 는 main sha vs local sha 비교. sha 포맷이 동일한 git blob 해시라 그대로 매칭됨.
  async function refreshDocState(retry = 0) {
    try {
      const mRes = await authFetch(`${API}/${projectCode}/tree?ref=main`);
      if (!mRes.ok) throw new Error(`main tree ${mRes.status}`);
      const mTree = await mRes.json();
      if (!mTree?.tree) throw new Error('invalid main tree');
      setMainTree(mTree);

      let lTree = null;
      if (launcher?.ready) {
        // clone 미존재 시 /files 가 EnsureRepoLocal(전체 clone)로 블록됨 → 그 동안을 "프로비저닝 중" 으로 표시.
        setLoadingFiles(true);
        try {
          const r = await requestFiles({ code: projectCode, user, token: launcher.token });
          if (r.ok) lTree = r.data;
        } finally {
          setLoadingFiles(false);
        }
      }
      setLocalTree(lTree);
      setAhead(lTree?.ahead ?? 0);

      if (!lTree) {
        setFileStatus({});
        setFileCommitted({});
        if (onDocStateChange) onDocStateChange();
        return;
      }
      // sha 맵 3개: main(서버) · wt(런처 working tree) · head(런처 HEAD).
      const mainSha = {}, wtSha = {}, headSha = {};
      const mainPaths = new Set(), wtPaths = new Set(), headPaths = new Set();
      (mTree.tree || []).forEach(f => { mainSha[f.path] = f.sha; mainPaths.add(f.path); });
      (lTree.tree || []).forEach(f => { wtSha[f.path] = f.sha; wtPaths.add(f.path); });
      (lTree.headTree || []).forEach(f => { headSha[f.path] = f.sha; headPaths.add(f.path); });

      // main vs working tree 차이 분류 + 그 차이가 HEAD 에 반영됐는지(=committed) 판정.
      // committed = (현재 디스크의 내용이 HEAD 의 같은 path 와 동일) — 즉 사용자가 commit 까지 마쳤는지.
      // added : path 가 wt 에만 → head 에도 있으면 +A (committed), 없으면 -A (untracked/uncommitted).
      // modified : wt sha != main sha → wt sha == head sha 면 +M, 아니면 -M.
      // deleted : path 가 main 에만 → head 에서도 빠졌으면 +D, head 에 있으면 -D (디스크에서만 지움, 아직 git rm/commit 안 함).
      const status = {}, committed = {};
      wtPaths.forEach(p => {
        if (!mainPaths.has(p)) {
          status[p] = 'added';
          committed[p] = headPaths.has(p);
        } else if (mainSha[p] !== wtSha[p]) {
          status[p] = 'modified';
          committed[p] = wtSha[p] === headSha[p];
        }
      });
      mainPaths.forEach(p => {
        if (!wtPaths.has(p)) {
          status[p] = 'deleted';
          committed[p] = !headPaths.has(p);
        }
      });
      setFileStatus(status);
      setFileCommitted(committed);
      if (onDocStateChange) onDocStateChange();
    } catch {
      if (retry < 1) setTimeout(() => refreshDocState(retry + 1), 1000);
      else { setLocalTree(null); setFileStatus({}); setFileCommitted({}); setAhead(0); }
    }
  }

  // 외부 변경(SSE push) 으로 현재 열린 파일 재로드 (웹은 조회 전용).
  // SSE 구독 콜백이 매 렌더의 state 를 참조하도록 ref 경유.
  const reloadRef = React.useRef(null);
  reloadRef.current = async () => {
    if (!selectedFile) return;
    try {
      let text = '';
      if (selectedMainOnly || activeTab === 'main') {
        const res = await authFetch(`${API}/${projectCode}/file?path=${encodeURIComponent(selectedFile)}&ref=main`);
        if (!res.ok) return;
        const d = await res.json();
        text = d.content ? decodeBase64(d.content) : '';
      } else if (activeTab === 'local' && launcher?.ready) {
        const r = await requestLocalFile({ code: projectCode, user, path: selectedFile, token: launcher.token });
        if (!r.ok) return;
        text = r.data?.content ? decodeBase64(r.data.content) : '';
      } else {
        return;
      }
      if (text !== content) setContent(text);
    } catch {}
  };
  async function reloadSelectedFile() { return reloadRef.current && reloadRef.current(); }

  // 진입 시 + 런처 ready 상태 변동 시 트리 상태 갱신.
  // 런처는 비동기 감지되므로 처음엔 launcher=null → main 만 로드, ready 후 재호출되어 local 트리 채움.
  useEffect(() => {
    if (!hasRepo) return;
    refreshDocState();
  }, [projectCode, hasRepo, user, launcher?.ready]); // eslint-disable-line

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
        // push 는 현재 열린 프로젝트의 레포일 때만 — 남의 프로젝트 push 마다
        // Gitea 왕복(main 트리 재조회)이 생기는 낭비 방지.
        if (data.type === 'push' && data.repo && data.repo === project?.documents?.repoName) {
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

  // "편집" — 인라인 편집 모드 진입. 런처 미감지면 설치 모달.
  // 런처는 디스크 write 만 담당 (commit/push 는 Local 탭 "등록" 으로).
  function handleStartEdit() {
    if (!selectedFile || selectedMainOnly) return;
    if (!launcher?.ready) { setInstallOpen(true); return; }
    setEditContent(content);
    setEditing(true);
  }

  // 편집 중 "저장" — 런처 /write 로 로컬 디스크에 쓰고 편집 종료. 변경 트리 재로드.
  async function handleSaveEdit() {
    if (!selectedFile || !launcher?.ready) return;
    setSaving(true);
    try {
      const r = await requestWrite({
        code: projectCode, user, path: selectedFile,
        content: editContent, token: launcher.token,
      });
      if (!r.ok) { modalAlert(r.error || '저장에 실패했습니다.'); return; }
      setContent(editContent); // 미리보기/Raw 가 새 내용으로 반영되도록.
      setEditing(false);
      refreshDocState();
    } finally {
      setSaving(false);
    }
  }

  // 미리보기 인라인 PlantUML 편집 커밋 — PexBlock 이 ✓ 적용 시 (원본본문, 새본문) 으로 호출.
  // 본문이 originalSource 와 일치하는 ```plantuml|puml fence 의 본문만 newSource 로 교체하고
  // 런처 /write 로 로컬 디스크에 저장한다 (전체 raw 편집과 동일한 저장 경로).
  async function handlePexCommit(originalSource, newSource) {
    if (!selectedFile || selectedMainOnly || !launcher?.ready) return;
    if (newSource == null || newSource === originalSource) return;
    // content 는 로컬 워킹트리에서 온 원본이라 Windows 면 CRLF 일 수 있는데, ReactMarkdown 이
    // 준 originalSource 는 LF 로 정규화돼 있어 바이트 매칭이 어긋난다. → LF 공간에서 매칭/교체한
    // 뒤, 원본이 CRLF 였으면 일괄 CRLF 로 복원해 줄바꿈 일관성을 유지한다.
    const usesCRLF = /\r\n/.test(content);
    const lf = content.replace(/\r\n/g, '\n');
    const want = String(originalSource).replace(/\r\n/g, '\n').trim();
    const newBody = String(newSource).replace(/\r\n/g, '\n');
    let replaced = false;
    let out = lf.replace(
      /(```(?:plantuml|puml)[^\n]*\n)([\s\S]*?)(\n?```)/g,
      (whole, open, body, close) => {
        if (!replaced && body.trim() === want) {
          replaced = true;
          return `${open}${newBody}${close}`;
        }
        return whole;
      },
    );
    if (!replaced) { modalAlert('편집 대상 PlantUML 블록을 본문에서 찾지 못했습니다.'); return; }
    if (usesCRLF) out = out.replace(/\n/g, '\r\n');
    setSaving(true);
    try {
      const r = await requestWrite({
        code: projectCode, user, path: selectedFile,
        content: out, token: launcher.token,
      });
      if (!r.ok) { modalAlert(r.error || '저장에 실패했습니다.'); return; }
      setContent(out); // 미리보기가 교체된 다이어그램으로 즉시 반영되도록.
      refreshDocState();
    } finally {
      setSaving(false);
    }
  }

  // ReactMarkdown components 를 렌더마다 새 객체/함수로 만들면 code/pre 가 매번 새 컴포넌트
  // 타입으로 취급되어 모든 PexBlock/MermaidBlock 이 unmount→remount 된다. 저장 한 번에
  // setSaving/setContent/refreshDocState 로 리렌더가 4~5회 발생하므로 그때마다 전체
  // 다이어그램이 재마운트되며 깜박였다. → identity 를 고정해 리렌더 시 인스턴스를 유지한다.
  // handlePexCommit 은 content 등 최신 state 를 봐야 하므로 ref 경유로 고정 래퍼를 만든다.
  const pexCommitRef = useRef(null);
  pexCommitRef.current = handlePexCommit;
  const handlePexCommitStable = useCallback((orig, next) => pexCommitRef.current?.(orig, next), []);
  const pexEditable = activeTab === 'local' && !selectedMainOnly && !!launcher?.ready;
  const markdownComponents = useMemo(() => ({
    code({ className, children, ...props }) {
      const lang = /language-(\w+)/.exec(className || '')?.[1];
      if (lang === 'mermaid') {
        return <MermaidBlock code={String(children).replace(/\n$/, '')} />;
      }
      if (lang === 'plantuml' || lang === 'puml') {
        const src = String(children).replace(/\n$/, '');
        // 로컬(docs/{user}) 워킹트리 + 런처 감지 시 미리보기에서 다이어그램을
        // 보면서 인라인 편집 (✎ 편집 상시 노출 → ✓ 적용 시 해당 fence 본문만 교체).
        // 그 외(main 탭·main-only·런처 미감지)는 조회 전용.
        return pexEditable
          ? <PexBlock source={src} onCommit={handlePexCommitStable} />
          : <PexBlock source={src} readOnly />;
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
  }), [pexEditable, handlePexCommitStable]);

  // 편집 중 "취소" — 변경 폐기 + 편집 종료.
  function handleCancelEdit() {
    setEditing(false);
    setEditContent('');
  }

  // 편집 중 "VS Code" — 인라인 편집 내용을 먼저 disk 저장(보존)한 뒤 VS Code 띄움.
  // 편집 모드 종료, 이후는 VS Code 가 책임.
  async function handleOpenVSCode() {
    if (!selectedFile || !launcher?.ready) return;
    setLaunching(true);
    try {
      if (editing && editContent !== content) {
        const w = await requestWrite({
          code: projectCode, user, path: selectedFile,
          content: editContent, token: launcher.token,
        });
        if (!w.ok) { modalAlert(w.error || '저장 실패 — VS Code 열기 중단'); return; }
        setContent(editContent);
      }
      const res = await requestEdit({
        code: projectCode, user, file: selectedFile, token: launcher.token,
      });
      if (!res.ok) modalAlert(res.error || '로컬 런처 호출에 실패했습니다.');
      setEditing(false);
      refreshDocState();
    } finally {
      setLaunching(false);
    }
  }

  // 설치 완료 후 "다시 시도" — 재감지 → 성공이면 모달 닫기.
  async function handleInstallRetry() {
    const r = await detectLauncher();
    setLauncher(r);
    if (r?.ready) setInstallOpen(false);
  }

  async function handleFileClick(path, isMainOnly) {
    if (editing) {
      const dirty = editContent !== content;
      if (dirty && !await modalConfirm('편집 중인 변경사항이 있습니다. 폐기하고 다른 파일을 열까요?')) return;
      setEditing(false);
    }
    let text = '';
    if (isMainOnly || activeTab === 'main') {
      const res = await authFetch(`${API}/${projectCode}/file?path=${encodeURIComponent(path)}&ref=main`);
      if (!res.ok) return;
      const d = await res.json();
      text = d.content ? decodeBase64(d.content) : '';
    } else if (activeTab === 'local' && launcher?.ready) {
      const r = await requestLocalFile({ code: projectCode, user, path, token: launcher.token });
      if (!r.ok) return;
      text = r.data?.content ? decodeBase64(r.data.content) : '';
    } else {
      return;
    }
    setSelectedFile(path);
    setSelectedMainOnly(!!isMainOnly);
    setContent(text);
  }

  async function handleDiffToggle() {
    if (diffMode) { setViewMode('preview'); return; }
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
      setViewMode('diff');
      setDiffTab('semantic');
    } catch (e) {
      modalAlert('Diff 로딩 실패');
    }
    setLoadingDiff(false);
  }


  // "등록" — 미커밋(dirty) 이거나 unpushed commit 이 있을 때 한 번에 origin 까지 올린다.
  // 런처 /sync 가 dirty 면 git add -A + commit, 그 다음 push 수행. 완료 후 트리 재로드.
  async function handleSync() {
    if (!launcher || !launcher.token) return;
    const hasUncommitted = Object.values(fileCommitted).some(v => v === false);
    const msg = hasUncommitted && ahead !== 0
      ? `미커밋 변경과 미푸시 커밋을 모두 origin 으로 등록하시겠습니까?`
      : hasUncommitted
      ? `미커밋 변경을 커밋·푸시하시겠습니까?`
      : `미푸시 커밋을 push 하시겠습니까?`;
    if (!await modalConfirm(msg)) return;
    setPublishing(true);
    try {
      const r = await requestSync({ code: projectCode, user, token: launcher.token });
      if (!r.ok) { modalAlert(r.error || '등록에 실패했습니다.'); return; }
      refreshDocState();
    } finally {
      setPublishing(false);
    }
  }

  // 파일 삭제 — 로컬 working tree 에서 제거(런처 /delete). commit/push 안 함 — "등록"(/sync)
  // 의 git add -A 가 삭제를 포착해 반영한다. 삭제 후 트리 재로드 + 열려있던 파일이면 닫는다.
  async function handleDeleteFile(path) {
    if (!launcher || !launcher.token) return;
    if (!await modalConfirm(`"${path}" 파일을 로컬에서 삭제하시겠습니까?\n(등록 시 origin 에 반영됩니다)`)) return;
    const r = await requestDelete({ code: projectCode, user, path, token: launcher.token });
    if (!r.ok) { modalAlert(r.error || '삭제에 실패했습니다.'); return; }
    if (selectedFile === path) { setSelectedFile(null); }
    refreshDocState();
  }

  // 파일 이름변경/이동 — 로컬 working tree 에서 from→to(런처 /rename). commit/push 안 함(/sync 포착).
  // 새 경로를 입력받아(폴더 이동도 경로에 / 포함하면 가능) 이동 후 트리 재로드.
  async function handleRenameFile(path) {
    if (!launcher || !launcher.token) return;
    const to = await modalPrompt(`"${path}" 의 새 경로를 입력하세요 (폴더 이동은 경로에 / 포함):`, path);
    if (to == null) return;
    const next = to.trim();
    if (!next || next === path) return;
    const r = await requestRename({ code: projectCode, user, from: path, to: next, token: launcher.token });
    if (!r.ok) { modalAlert(r.error || '이름변경에 실패했습니다.'); return; }
    if (selectedFile === path) { setSelectedFile(next); }
    refreshDocState();
  }

  // "Pull" — 로컬 docs/{user} 를 origin/main 으로 완전히 덮어씌운다 (destructive).
  // 사용자 커밋·미커밋 변경 모두 폐기. 강한 confirm 필수.
  async function handlePullFromMain() {
    if (!launcher || !launcher.token) return;
    const hasUncommitted = Object.values(fileCommitted).some(v => v === false);
    const warn = hasUncommitted
      ? `⚠ 로컬 docs/${user} 의 변경이 모두 폐기됩니다.\n\n• 미커밋 변경 (있음): 사라집니다\n• 로컬 커밋: 사라집니다\n• 결과: main 과 완전히 동일한 상태\n\n복구는 직전 HEAD sha 로 git reset --hard 만 가능 (응답에 표시됨). 계속하시겠습니까?`
      : `로컬 docs/${user} 를 main 의 내용으로 완전히 덮어씌웁니다.\n\n• 사용자 push 안 한 로컬 커밋이 있다면 사라집니다\n• 결과: main 과 완전히 동일한 상태\n\n계속하시겠습니까?`;
    if (!await modalConfirm(warn)) return;
    setPublishing(true);
    try {
      const r = await requestPullFromMain({ code: projectCode, user, token: launcher.token });
      if (!r.ok) { modalAlert(r.error || 'Pull 실패'); return; }
      const d = r.data || {};
      if (d.status === 'noop') {
        modalAlert(d.message || '이미 main 의 최신 내용과 동일합니다.');
      } else {
        modalAlert(d.message || 'main 의 내용으로 덮어씌움 완료');
      }
      refreshDocState();
    } finally {
      setPublishing(false);
    }
  }

  // "발행" — origin/docs/{user} 가 이미 origin/main 과 다를 때 (모두 커밋+푸시 완료된 상태)
  // 서버에 PR 생성만 요청. /push 는 호출하지 않는다 (이미 동기화된 상태).
  async function handleLocalPublish() {
    if (!launcher || !launcher.token) return;
    if (!await modalConfirm(`origin/docs/${user} 변경을 발행 신청(PR)하시겠습니까?`)) return;
    setPublishing(true);
    try {
      const res = await authFetch(`${API}/${projectCode}/pr`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
        body: JSON.stringify({
          title: `[${projectCode}] 설계문서 배포 요청`,
          head: `docs/${user}`,
          skipAutoCommit: true,
        }),
      });
      if (res.ok) {
        modalAlert('발행 신청(PR)을 생성했습니다.');
        refreshDocState();
      } else {
        const err = await res.json().catch(() => ({}));
        modalAlert(err.error || 'PR 생성 실패');
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

  // Local 탭에서 런처 미감지 시 currentTree=null → 사이드바 비움 + 설치 안내(아래 헤더 영역).
  const currentTree = activeTab === 'main' ? mainTree : localTree;

  const baseItems = (currentTree?.tree || []).filter(i => i.type !== 'tree');
  // Local 탭에서 main에만 있는 파일도 표시 (gray italic — Pull로 동기화 필요 표시).
  let items;
  if (activeTab === 'local' && mainTree && localTree) {
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
    if (!diffMain) return null;
    // prefix: + = 차이가 docs/{user} HEAD 에 커밋된 상태(발행 시 PR 에 포함됨),
    //         - = working tree 의 미커밋 변경(commit 전이라 PR 에 안 들어감).
    const sign = fileCommitted[path] ? '+' : '-';
    const letter = diffMain === 'added' ? 'A' : diffMain === 'deleted' ? 'D' : 'M';
    const cls = `de-file-badge de-badge-${diffMain}${fileCommitted[path] ? '' : ' de-badge-uncommitted'}`;
    return <span className={cls}>{sign}{letter}</span>;
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <CommonBar title={barTitle} onBack={onBack} nav={nav} projectAlert={projectAlert} />
      {installOpen && (
        <LauncherInstallModal
          onClose={() => setInstallOpen(false)}
          onRetry={handleInstallRetry}
        />
      )}
      <div className="de-editor">
        <div className="de-sidebar" style={{ width: sidebarWidth }}>
          <div className="de-sidebar-tabs">
            <button className={`de-sidebar-tab ${activeTab === 'main' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => { setActiveTab('main'); setShowGraph(false); }}>Main</button>
            <button className={`de-sidebar-tab ${activeTab === 'local' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => { setActiveTab('local'); setShowGraph(false); }}>Local</button>
            {/* Local 탭의 데이터소스는 런처 /files (로컬 working tree). 런처 미감지 시 빈 상태 + 설치 안내. */}
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
            <span>{activeTab === 'main' ? 'main' : `local · docs/${user}`}</span>
            {activeTab === 'main' && launcher?.ready && (() => {
              // Main 탭의 Pull 버튼 — 항상 활성. dirty 면 클릭 시 경고 confirm 후 진행
              // (서버가 자동 stash → pull → stash pop 처리).
              const hasUncommitted = Object.values(fileCommitted).some(v => v === false);
              const showPullHint = hasMainOnly && !hasUncommitted;
              return (
                <button
                  className="de-btn"
                  style={{
                    marginLeft: 'auto',
                    padding: '3px 10px',
                    fontSize: '11px',
                    background: showPullHint ? '#2563eb' : undefined,
                    color: showPullHint ? '#fff' : undefined,
                  }}
                  onClick={handlePullFromMain}
                  disabled={publishing}
                  title={
                    hasUncommitted
                      ? 'main 의 최신 내용을 끌어옵니다 (미커밋 변경은 자동 stash → pop)'
                      : 'main 의 최신 내용을 내 로컬 docs/{user} 로 끌어옵니다'
                  }
                >Pull</button>
              );
            })()}
            {activeTab === 'local' && launcher?.ready && (() => {
              // 미커밋(-) 또는 unpushed commit 이 하나라도 있으면 "등록" 모드.
              // (ahead === -1 은 upstream 미설정 → 신규 브랜치라 push 필요로 간주.)
              const hasUncommitted = Object.values(fileCommitted).some(v => v === false);
              const hasUnpushed = ahead !== 0;
              const needsRegister = hasUncommitted || hasUnpushed;
              const label = publishing
                ? (needsRegister ? '등록 중...' : '발행 중...')
                : (needsRegister ? '등록' : '발행');
              const title = needsRegister
                ? '미커밋/미푸시 변경을 origin 으로 올립니다'
                : 'origin/docs/{user} 변경을 발행 신청(PR)합니다';
              return (
                <button
                  className="de-btn"
                  style={{ marginLeft: 'auto', padding: '3px 10px', fontSize: '11px' }}
                  onClick={needsRegister ? handleSync : handleLocalPublish}
                  disabled={publishing}
                  title={title}
                >{label}</button>
              );
            })()}
          </div>
          {activeTab === 'local' && !launcher?.ready && (
            <div className="de-sync-notice">
              로컬 런처가 감지되지 않았습니다.
              <button className="de-btn"
                style={{ marginLeft: 8, padding: '2px 8px', fontSize: 11 }}
                onClick={() => setInstallOpen(true)}>런처 안내</button>
            </div>
          )}
          {activeTab === 'local' && launcher?.ready && !hasChanges && hasMainOnly && (
            <div className="de-sync-notice">
              main 에 새 파일이 있습니다. <b>Main</b> 탭의 <b>Pull</b> 버튼으로 동기화하세요.
            </div>
          )}
          {activeTab === 'local' && launcher?.ready && showProvisioning && (
            <div className="de-provisioning" role="status" aria-live="polite">
              <span className="de-provisioning-spin" aria-hidden="true">⟳</span>
              <span>워크스페이스 프로비저닝 중…</span>
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
              const st = (activeTab === 'local' && !item._mainOnly) ? fileStatus[item.path] : null;
              const isMainOnly = item._mainOnly;
              return (
                <div key={item.path}
                  className={`de-file ${selectedFile === item.path ? 'de-file-active' : ''} ${st ? `de-file-${st}` : ''} ${isMainOnly ? 'de-file-main-only' : ''}`}
                  onClick={() => { setShowGraph(false); setViewMode('preview'); handleFileClick(item.path, item._mainOnly); }}>
                  {activeTab === 'local' && (isMainOnly
                    ? <span className="de-file-badge de-badge-mainonly">M</span>
                    : statusBadge(item.path))}
                  <span className="de-file-name">{item._name || item.path}</span>
                  {activeTab === 'local' && !isMainOnly && launcher?.ready && (
                    <span className="de-file-actions">
                      <button className="de-file-act" title="이름변경/이동"
                        onClick={(e) => { e.stopPropagation(); handleRenameFile(item.path); }}>✎</button>
                      <button className="de-file-act de-file-act-del" title="삭제"
                        onClick={(e) => { e.stopPropagation(); handleDeleteFile(item.path); }}>🗑</button>
                    </span>
                  )}
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
                  ) : editing ? (
                    <>
                      <button className="de-btn" onClick={handleCancelEdit} disabled={saving || launching}>취소</button>
                      <button className="de-btn" onClick={handleOpenVSCode} disabled={saving || launching}
                        title="현재 편집 내용을 저장한 뒤 VS Code 로 엽니다">
                        {launching ? '여는 중...' : 'VS Code'}
                      </button>
                      <button className="de-btn de-btn-primary" onClick={handleSaveEdit} disabled={saving || launching}
                        title="로컬 디스크에만 저장합니다 (커밋/푸시는 Local 탭의 '등록' 으로)">
                        {saving ? '저장 중...' : '저장'}
                      </button>
                    </>
                  ) : (
                    <>
                      <button className={`de-tab ${viewMode === 'preview' ? 'de-tab-active' : ''}`} onClick={() => setViewMode('preview')}>미리보기</button>
                      <button className={`de-tab ${viewMode === 'raw' ? 'de-tab-active' : ''}`} onClick={() => setViewMode('raw')}>Raw 보기</button>
                      {activeTab === 'local' && fileStatus[selectedFile] && (
                        <button
                          className={`de-btn ${diffMode ? 'de-btn-primary' : ''}`}
                          onClick={handleDiffToggle}
                          disabled={loadingDiff}
                        >
                          {loadingDiff ? '로딩...' : 'Diff 분석'}
                        </button>
                      )}
                      <button
                        className="de-btn de-btn-primary"
                        onClick={handleStartEdit}
                        title={launcher?.ready
                          ? '인라인 편집 모드로 진입합니다'
                          : '로컬 런처가 감지되지 않았습니다 — 클릭하면 설치 안내'}
                      >편집</button>
                    </>
                  )}
                </div>
              </div>
              <div className="de-body" style={{ position: 'relative' }}>
                {editing ? (
                  <textarea
                    className="de-editor-textarea"
                    value={editContent}
                    onChange={e => setEditContent(e.target.value)}
                    spellCheck={false}
                    autoFocus
                  />
                ) : viewMode === 'diff' ? (
                  <DiffAnalysis oldText={mainContent} newText={content} fileName={selectedFile} activeTab={diffTab} onTabChange={setDiffTab} />
                ) : viewMode === 'raw' ? (
                  <pre className="de-raw">{content}</pre>
                ) : (
                  <div className={`de-preview ${selectedMainOnly ? 'de-preview-mainonly' : ''}`}>
                    {/\.ya?ml$/i.test(selectedFile || '') ? (
                      <YamlView text={content} />
                    ) : (
                      <ReactMarkdown
                        remarkPlugins={[remarkGfm]}
                        components={markdownComponents}
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
