import React, { useState, useEffect, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { modalAlert, modalConfirm, modalPrompt } from './AlertModal';
import CommonBar from './CommonBar';
import DiffAnalysis from './DiffAnalysis';
import MermaidBlock from './MermaidBlock';
import PexBlock from './PexBlock';
import config from '../config';
import { subscribe } from '../utils/sseManager';
import { detectLauncher, requestEdit, requestPush } from '../utils/launcher';
import './DocEditor.css';

/**
 * markdown 안에서 본문이 originalSource 와 일치하는 첫 ```plantuml/```puml fence 의
 * 본문을 newBody 로 교체. plantumlCounterRef + idx 매핑은 ReactMarkdown 의
 * components.code 호출 타이밍과 어긋나는 케이스가 있어, fence 본문 자체를 식별자로
 * 사용한다 (PexBlock 의 source prop 이 그대로 전달되므로 신뢰 가능).
 */
function replacePlantumlFenceBySource(md, originalSource, newBody) {
  const lines = (md || '').split('\n');
  const fenceOpen = /^[ \t]*```\s*(plantuml|puml)\b/i;
  const fenceClose = /^[ \t]*```\s*$/;
  let openLine = -1;
  for (let i = 0; i < lines.length; i++) {
    if (openLine === -1) {
      if (fenceOpen.test(lines[i])) openLine = i;
    } else if (fenceClose.test(lines[i])) {
      const body = lines.slice(openLine + 1, i).join('\n');
      if (body === originalSource) {
        const before = lines.slice(0, openLine + 1);
        const after = lines.slice(i);
        const bodyLines = (newBody || '').split('\n');
        return [...before, ...bodyLines, ...after].join('\n');
      }
      openLine = -1;
    }
  }
  return md; // 없으면 원본 반환
}

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

export default function DocEditor({ project, user, authority, onBack, currentPage, onNavigate, projectAlert, onDocStateChange }) {
  const projectCode = project?.jiraCodes?.main?.jiraCode || '';
  const hasRepo = !!project?.documents?.repoName;

  const [mainTree, setMainTree] = useState(null);
  const [userTree, setUserTree] = useState(null);
  const [fileStatus, setFileStatus] = useState({}); // path → 'modified' | 'added' | 'deleted'
  const [activeTab, setActiveTab] = useState('workspace'); // 'main' | 'workspace'
  const [selectedFile, setSelectedFile] = useState(null);
  const [selectedMainOnly, setSelectedMainOnly] = useState(false);
  const [content, setContent] = useState('');
  const [original, setOriginal] = useState('');
  const [saving, setSaving] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [readOnly, setReadOnly] = useState(false);
  const [conflictDiff, setConflictDiff] = useState(null);
  const [pendingFiles, setPendingFiles] = useState(new Set());
  const [globalEditMode, setGlobalEditMode] = useState(false);
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

  // 외부 변경(SSE push) 으로 현재 열린 파일 재로드. 편집 중이면 건너뜀.
  // SSE 구독 콜백이 매 렌더의 state 를 참조하도록 ref 경유.
  const reloadRef = React.useRef(null);
  reloadRef.current = async () => {
    if (!selectedFile) return;
    if (editMode && content !== original) return;
    const ref = selectedMainOnly ? 'main' : (activeTab === 'workspace' ? `docs/${user}` : 'main');
    try {
      const res = await fetch(`${API}/${projectCode}/file?path=${encodeURIComponent(selectedFile)}&ref=${encodeURIComponent(ref)}`);
      if (!res.ok) return;
      const d = await res.json();
      const text = d.content ? decodeBase64(d.content) : '';
      if (text !== content) {
        setContent(text);
        setOriginal(text);
      }
    } catch {}
  };
  async function reloadSelectedFile() { return reloadRef.current && reloadRef.current(); }

  // 진입 시 초기 파일 확인 → 사용자 브랜치 생성 → 트리 비교 (순차 실행)
  useEffect(() => {
    if (!hasRepo) return;
    fetch(`${API}/${projectCode}/init-files`, { method: 'POST' })
      .catch(() => {})
      .then(() => fetch(`${API}/${projectCode}/ensure-branch`, {
        method: 'POST', headers: { 'X-User-Id': user },
      }).catch(() => {}))
      .then(() => refreshDocState());
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
  // 서버측 write(handleSave/edit-start)와 별개로, 사용자 PC 로컬 작업본에서 편집한다.
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
    if (editMode && content !== original) {
      if (!await modalConfirm('저장하지 않은 변경이 있습니다. 이동하시겠습니까?')) return;
    }
    // mainOnly 파일이면 main에서 조회
    const ref = isMainOnly ? 'main' : (activeTab === 'workspace' ? `docs/${user}` : 'main');
    const res = await fetch(`${API}/${projectCode}/file?path=${encodeURIComponent(path)}&ref=${encodeURIComponent(ref)}`);
    if (res.ok) {
      const d = await res.json();
      const text = d.content ? decodeBase64(d.content) : '';
      setSelectedFile(path);
      setSelectedMainOnly(!!isMainOnly);
      setContent(text);
      setOriginal(text);
      setEditMode(false);
    }
  }

  async function handleSave() {
    if (!await modalConfirm(`"${selectedFile}" 파일을 저장(커밋)하시겠습니까?`)) return;
    setSaving(true);
    const res = await fetch(`${API}/${projectCode}/file`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ path: selectedFile, content, message: `${selectedFile} 수정` }),
    });
    setSaving(false);
    if (res.ok) {
      setOriginal(content);
      setEditMode(false);
      refreshDocState();
      modalAlert('저장 완료');
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || '저장 실패');
    }
  }


  async function handleDelete() {
    if (!await modalConfirm(`"${selectedFile}" 파일을 삭제하시겠습니까?`)) return;
    const res = await fetch(`${API}/${projectCode}/file`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ path: selectedFile, message: `${selectedFile} 삭제` }),
    });
    if (res.ok) {
      setSelectedFile(null);
      setContent('');
      setOriginal('');
      setEditMode(false);
      refreshDocState();
      modalAlert('삭제 완료');
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || '삭제 실패');
    }
  }

  async function handleNewFile() {
    const input = await modalPrompt('경로를 입력하세요\n(예: 파일명.md 또는 폴더명/파일명.md)', '');
    if (!input || !input.trim()) return;
    let filePath = input.trim().replace(/\\/g, '/');
    // 폴더만 입력한 경우 (슬래시로 끝나거나 확장자 없음)
    if (filePath.endsWith('/')) {
      const folderFile = await modalPrompt(`"${filePath}" 폴더에 생성할 파일 이름`, '');
      if (!folderFile || !folderFile.trim()) return;
      let fn = folderFile.trim();
      if (!fn.endsWith('.md')) fn += '.md';
      filePath = filePath + fn;
    } else if (!filePath.includes('.')) {
      // 확장자 없고 슬래시도 없으면 폴더인지 파일인지 확인
      const isFolder = await modalConfirm(`"${filePath}"을(를) 폴더로 생성하시겠습니까?\n(취소 시 파일로 생성)`);
      if (isFolder) {
        const folderFile = await modalPrompt(`"${filePath}/" 폴더에 생성할 파일 이름`, '');
        if (!folderFile || !folderFile.trim()) return;
        let fn = folderFile.trim();
        if (!fn.endsWith('.md')) fn += '.md';
        filePath = filePath + '/' + fn;
      } else {
        filePath += '.md';
      }
    } else if (!filePath.endsWith('.md')) {
      filePath += '.md';
    }
    // 중복 확인
    if (items.some(i => i.path === filePath)) {
      modalAlert(`"${filePath}" 파일이 이미 존재합니다.`);
      return;
    }
    const baseName = filePath.split('/').pop().replace('.md', '');
    const template = `# ${baseName}\n\n`;
    const res = await fetch(`${API}/${projectCode}/file`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ path: filePath, content: template, message: `${filePath} 생성` }),
    });
    if (res.ok) {
      refreshDocState();
      setSelectedFile(filePath);
      setContent(template);
      setOriginal(template);
      setEditMode(true);
      setShowGraph(false);
      setDiffMode(false);
      modalAlert(`"${filePath}" 파일이 생성되었습니다.`);
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || '파일 생성 실패');
    }
  }

  async function handleRenameFile(oldPath) {
    const ext = oldPath.endsWith('.md') ? '' : '.md';
    const base = oldPath.replace(/\.md$/, '');
    const newName = await modalPrompt('새 파일 이름 (폴더/파일명 가능)', base);
    if (!newName || !newName.trim()) return;
    let newPath = newName.trim();
    if (!newPath.endsWith('.md')) newPath += '.md';
    if (newPath === oldPath) return;
    if (items.some(i => i.path === newPath)) {
      modalAlert(`"${newPath}" 파일이 이미 존재합니다.`);
      return;
    }
    // 기존 파일 내용 읽기
    const ref = `docs/${user}`;
    const getRes = await fetch(`${API}/${projectCode}/file?path=${encodeURIComponent(oldPath)}&ref=${encodeURIComponent(ref)}`);
    if (!getRes.ok) { modalAlert('파일 읽기 실패'); return; }
    const fileData = await getRes.json();
    const fileContent = fileData.content ? decodeBase64(fileData.content) : '';
    // 새 파일 생성
    const createRes = await fetch(`${API}/${projectCode}/file`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ path: newPath, content: fileContent, message: `${oldPath} → ${newPath} 이름 변경` }),
    });
    if (!createRes.ok) { modalAlert('파일 생성 실패'); return; }
    // 기존 파일 삭제
    await fetch(`${API}/${projectCode}/file`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ path: oldPath, message: `${oldPath} 삭제 (이름 변경)` }),
    });
    // 상태 갱신
    setFileStatus(prev => {
      const next = { ...prev };
      delete next[oldPath];
      next[newPath] = prev[oldPath] || 'added';
      return next;
    });
    refreshDocState();
    setSelectedFile(newPath);
    setContent(fileContent);
    setOriginal(fileContent);
    modalAlert(`"${oldPath}" → "${newPath}" 이름 변경 완료`);
  }

  async function handleDeleteFile(path) {
    if (!await modalConfirm(`"${path}" 파일을 삭제하시겠습니까?`)) return;
    const res = await fetch(`${API}/${projectCode}/file`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ path, message: `${path} 삭제` }),
    });
    if (res.ok) {
      if (selectedFile === path) {
        setSelectedFile(null);
        setContent('');
        setOriginal('');
        setEditMode(false);
      }
      refreshDocState();
      modalAlert('삭제 완료');
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || '삭제 실패');
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

  async function handleCreatePR() {
    if (!await modalConfirm(`docs/${user} 브랜치의 변경사항을 발행 신청(PR)하시겠습니까?`)) return;
    const res = await fetch(`${API}/${projectCode}/pr`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ title: `[${projectCode}] 설계문서 배포 요청`, head: `docs/${user}` }),
    });
    if (res.ok) {
      modalAlert('발행 신청(PR)이 생성되었습니다.');
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || 'PR 생성 실패');
    }
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

  async function handleGitPull() {
    if (!await modalConfirm('main 브랜치의 최신 내용을 작업공간에 동기화하시겠습니까?\n(작업공간의 미커밋 변경사항은 초기화됩니다)')) return;
    try {
      // sync-branch로 사용자 브랜치를 main HEAD로 동기화
      const res = await fetch(`${API}/${projectCode}/sync-branch`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      });
      if (res.ok) {
        await refreshDocState();
        setSelectedMainOnly(false);
        modalAlert('main 동기화 완료');
      } else {
        const err = await res.json().catch(() => ({}));
        modalAlert(err.error || '동기화 실패');
      }
    } catch (e) {
      modalAlert('동기화 오류: ' + e.message);
    }
  }

  async function handleEditModeChange(isEdit) {
    setGlobalEditMode(isEdit);
    if (isEdit && hasRepo) {
      // 편집모드 진입 시 /edit/start 호출 (worktree 생성 + 세션 lock)
      const res = await fetch(`${API}/${projectCode}/edit/start`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      });
      if (res.ok) {
        const data = await res.json();
        setReadOnly(!!data.readOnly);
        if (data.conflict) {
          setConflictDiff(data.diff);
          modalAlert(data.conflictMessage || '충돌이 발생하여 main 기준으로 초기화되었습니다.');
        }
        // 미커밋 파일 목록 조회
        fetch(`${API}/${projectCode}/edit/pending`, {
          headers: { 'X-User-Id': user },
        }).then(r => r.ok ? r.json() : []).then(files => setPendingFiles(new Set(files))).catch(() => {});
      } else if (res.status === 409) {
        const data = await res.json();
        setReadOnly(true);
        modalAlert(data.message || '다른 사용자가 편집 중입니다.');
      } else {
        modalAlert('편집 모드 시작 실패');
        return;
      }
    } else if (!isEdit && hasRepo) {
      // 편집모드 종료 시 세션 해제
      fetch(`${API}/${projectCode}/edit/release`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      });
      setReadOnly(false);
      setConflictDiff(null);
      setPendingFiles(new Set());
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

  const modified = content !== original;
  const currentTree = activeTab === 'main' ? mainTree : (userTree || mainTree);

  // PexBlock 의 ✓ 적용 콜백 — markdown 내 N번째 plantuml fence 본문을 newSource 로 replace.
  // setContent → ReactMarkdown 자동 재렌더 → modified = true → 사용자가 저장 버튼 누르면 영속화.
  // PexBlock 의 ✓ 적용 — markdown 의 해당 fence 본문을 newSource (메타 포함) 로 교체.
  // 사용자 시나리오: 편집모드 ON → 다이어그램 편집 → ✓ 적용 시 디스크에도 자동 저장
  // (textarea 모드의 handleSave 와 달리 modalConfirm 안 띄움 — 인라인 편집은 즉각 반영).
  async function handlePlantumlCommit(originalSource, newSource) {
    const next = replacePlantumlFenceBySource(content, originalSource, newSource);
    if (next === content) return;
    setContent(next);
    try {
      const res = await fetch(`${API}/${projectCode}/file`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
        body: JSON.stringify({ path: selectedFile, content: next, message: `${selectedFile} 다이어그램 편집` }),
      });
      if (res.ok) {
        setOriginal(next);
        refreshDocState();
      } else {
        const err = await res.json().catch(() => ({}));
        modalAlert(err.error || '다이어그램 자동 저장 실패');
      }
    } catch (e) {
      modalAlert('다이어그램 자동 저장 오류: ' + e.message);
    }
  }
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
    const isPending = pendingFiles.has(path); // 미커밋

    if (isPending && diffMain) {
      // 둘 다: tomato ●
      return <span className="de-file-badge de-badge-both">●</span>;
    }
    if (isPending) {
      // 미커밋만: orange ●
      return <span className="de-file-badge de-badge-pending">●</span>;
    }
    if (diffMain) {
      // main과만 차이: M/A/D
      const label = diffMain === 'added' ? 'A' : diffMain === 'deleted' ? 'D' : 'M';
      const cls = `de-file-badge de-badge-${diffMain}`;
      return <span className={cls}>{label}</span>;
    }
    return null;
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <CommonBar title={barTitle} onBack={onBack} nav={nav} onEditChange={handleEditModeChange}
        onPull={handleGitPull}
        pullIcon={<img src="/icons/git-fetch.svg" alt="Git Fetch" width="18" height="18" />}
        projectAlert={projectAlert}
      />
      <div className="de-editor">
        <div className="de-sidebar" style={{ width: sidebarWidth }}>
          <div className="de-sidebar-tabs">
            <button className={`de-sidebar-tab ${activeTab === 'main' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => { setActiveTab('main'); setShowGraph(false); }}>Main</button>
            <button className={`de-sidebar-tab ${activeTab === 'workspace' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => { setActiveTab('workspace'); setShowGraph(false); }}>작업공간</button>
            {(authority?.USR || 0) >= 15 && (
              <button
                className="de-sidebar-tab de-graph-btn"
                onClick={async () => {
                  try {
                    const res = await fetch(`${API}/${projectCode}/open-folder`, {
                      method: 'POST', headers: { 'X-User-Id': user || '' }
                    });
                    if (!res.ok) {
                      const err = await res.json().catch(() => ({}));
                      alert(err.error || '폴더 열기 실패');
                    }
                  } catch { alert('서버 연결 실패'); }
                }}
                title="작업공간 폴더 열기"
              >
                <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M1 3.5A1.5 1.5 0 012.5 2h3.879a1.5 1.5 0 011.06.44l1.122 1.12A1.5 1.5 0 009.62 4H13.5A1.5 1.5 0 0115 5.5v7a1.5 1.5 0 01-1.5 1.5h-11A1.5 1.5 0 011 12.5v-9z"/>
                </svg>
              </button>
            )}
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
            {activeTab === 'workspace' && globalEditMode && (
              <button
                className="de-btn"
                style={{ marginLeft: 'auto', padding: '3px 8px', fontSize: '11px' }}
                onClick={handleNewFile}
                title="새 파일 추가"
              >+</button>
            )}
            {activeTab === 'workspace' && (
              <button
                className={`de-btn ${hasChanges ? 'de-btn-primary' : ''}`}
                style={{ marginLeft: globalEditMode ? '4px' : 'auto', padding: '3px 10px', fontSize: '11px' }}
                onClick={handleCreatePR}
                disabled={!hasChanges}
                title={hasChanges ? `${Object.keys(fileStatus).length}개 파일 변경됨` : '변경사항 없음'}
              >
                <img src="/icons/git-pull.svg" alt="" width="12" height="12" style={{marginRight:4,verticalAlign:'middle',filter:hasChanges?'invert(1)':'invert(0.6)'}} />
                발행신청
              </button>
            )}
            {activeTab === 'workspace' && launcher?.ready && (
              <button
                className="de-btn"
                style={{ marginLeft: '4px', padding: '3px 10px', fontSize: '11px' }}
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
                  {activeTab === 'workspace' && globalEditMode && (
                    <span className="de-file-actions">
                      <button className="de-file-act" title="이름 변경" onClick={e => { e.stopPropagation(); handleRenameFile(item.path); }}>✎</button>
                      <button className="de-file-act de-file-act-del" title="삭제" onClick={e => { e.stopPropagation(); handleDeleteFile(item.path); }}>✕</button>
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
                  {selectedFile} {modified && !selectedMainOnly && <span className="de-modified">●</span>}
                  {selectedMainOnly && <span style={{fontSize:11,color:'#aaa',marginLeft:6}}>(main only — Pull로 동기화 필요)</span>}
                </span>
                <div className="de-actions">
                  {selectedMainOnly ? (
                    <button className="de-tab de-tab-active">미리보기</button>
                  ) : editMode ? (
                    <>
                      <button className="de-tab" onClick={() => setEditMode(false)}>미리보기</button>
                      <button className="de-btn" onClick={handleSave} disabled={!modified || saving}>{saving ? '저장중...' : '저장'}</button>
                    </>
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
                      {globalEditMode && <button className="de-btn" onClick={() => setEditMode(true)}>편집</button>}
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
                      {globalEditMode && <button className="de-btn de-btn-danger" onClick={handleDelete}>삭제</button>}
                    </>
                  )}
                </div>
              </div>
              <div className="de-body" style={{ position: 'relative' }}>
                {diffMode ? (
                  <DiffAnalysis oldText={mainContent} newText={content} fileName={selectedFile} activeTab={diffTab} onTabChange={setDiffTab} />
                ) : editMode && !selectedMainOnly ? (
                  <textarea className="de-textarea" value={content}
                    onChange={e => setContent(e.target.value)} spellCheck={false} />
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
                              // 편집모드 ON (globalEditMode) + textarea 가 아닌 preview 일 때만 인라인 편집 활성.
                              // editMode (textarea) 에선 markdown 텍스트가 진실이므로 인라인 편집 비활성.
                              const inlineEditable = globalEditMode && !editMode;
                              return (
                                <PexBlock
                                  source={src}
                                  onCommit={inlineEditable ? handlePlantumlCommit : undefined}
                                  readOnly={!inlineEditable}
                                />
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
