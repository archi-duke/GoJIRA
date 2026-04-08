import React, { useState, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { modalAlert, modalConfirm } from './AlertModal';
import CommonBar from './CommonBar';
import DiffAnalysis from './DiffAnalysis';
import config from '../config';
import './DocEditor.css';

const API = config.PLATFORM_API + '/DocRepo';

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

export default function DocEditor({ project, user, onBack, currentPage, onNavigate }) {
  const projectCode = project?.jiraCodes?.main?.jiraCode || '';
  const hasRepo = !!project?.documents?.repoName;

  const [mainTree, setMainTree] = useState(null);
  const [userTree, setUserTree] = useState(null);
  const [fileStatus, setFileStatus] = useState({}); // path → 'modified' | 'added' | 'deleted'
  const [activeTab, setActiveTab] = useState('workspace'); // 'main' | 'workspace'
  const [selectedFile, setSelectedFile] = useState(null);
  const [content, setContent] = useState('');
  const [original, setOriginal] = useState('');
  const [saving, setSaving] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [readOnly, setReadOnly] = useState(false);
  const [conflictDiff, setConflictDiff] = useState(null);
  const [pendingFiles, setPendingFiles] = useState(new Set());
  const [globalEditMode, setGlobalEditMode] = useState(false);
  const [showGraph, setShowGraph] = useState(false);
  const [diffMode, setDiffMode] = useState(false);
  const [diffTab, setDiffTab] = useState('semantic');
  const [mainContent, setMainContent] = useState('');
  const [loadingDiff, setLoadingDiff] = useState(false);

  // 진입 시 필수 파일 확인 + 사용자 브랜치 생성 + 트리 비교
  useEffect(() => {
    if (!hasRepo) return;
    const branch = `docs/${user}`;

    // 1. 필수 파일 보충 + 사용자 브랜치 생성 완료 후 트리 로드
    Promise.all([
      fetch(`${API}/${projectCode}/init-files`, { method: 'POST' }).catch(() => {}),
      fetch(`${API}/${projectCode}/ensure-branch`, {
        method: 'POST', headers: { 'X-User-Id': user },
      }).catch(() => {}),
    ]).then(() => {

    fetch(`${API}/${projectCode}/tree?ref=main`)
      .then(r => r.json()).then(mTree => {
        setMainTree(mTree);

        fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent(branch)}`)
          .then(r => { if (!r.ok) throw new Error(); return r.json(); })
          .then(uTree => {
            setUserTree(uTree);

            const mainShaMap = {};
            const mainPathSet = new Set();
            (mTree?.tree || []).forEach(f => { mainShaMap[f.path] = f.sha; mainPathSet.add(f.path); });

            const userPathSet = new Set();
            const status = {};
            (uTree?.tree || []).forEach(f => {
              userPathSet.add(f.path);
              if (!mainPathSet.has(f.path)) {
                status[f.path] = 'added';
              } else if (mainShaMap[f.path] !== f.sha) {
                status[f.path] = 'modified';
              }
            });
            mainPathSet.forEach(p => {
              if (!userPathSet.has(p)) status[p] = 'deleted';
            });
            setFileStatus(status);
          })
          .catch(() => { setUserTree(null); setFileStatus({}); });
      })
      .catch(() => {});
    }); // Promise.all then
  }, [projectCode, hasRepo, user]);

  async function handleFileClick(path) {
    if (editMode && content !== original) {
      if (!await modalConfirm('저장하지 않은 변경이 있습니다. 이동하시겠습니까?')) return;
    }
    const ref = activeTab === 'workspace' ? `docs/${user}` : 'main';
    const res = await fetch(`${API}/${projectCode}/file?path=${encodeURIComponent(path)}&ref=${encodeURIComponent(ref)}`);
    if (res.ok) {
      const d = await res.json();
      const text = d.content ? decodeBase64(d.content) : '';
      setSelectedFile(path);
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
      setFileStatus(prev => ({ ...prev, [selectedFile]: 'modified' }));
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
      fetch(`${API}/${projectCode}/tree?ref=main`)
        .then(r => r.json()).then(d => setMainTree(d)).catch(() => {});
      modalAlert('삭제 완료');
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || '삭제 실패');
    }
  }

  async function handleNewFile() {
    const name = window.prompt('새 파일 이름을 입력하세요 (.md 확장자 자동 추가)', '');
    if (!name || !name.trim()) return;
    let fileName = name.trim();
    if (!fileName.endsWith('.md')) fileName += '.md';
    // 중복 확인
    if (items.some(i => i.path === fileName)) {
      modalAlert(`"${fileName}" 파일이 이미 존재합니다.`);
      return;
    }
    const template = `# ${fileName.replace('.md', '')}\n\n`;
    const res = await fetch(`${API}/${projectCode}/file`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ path: fileName, content: template, message: `${fileName} 생성` }),
    });
    if (res.ok) {
      setFileStatus(prev => ({ ...prev, [fileName]: 'added' }));
      // 사용자 트리 새로고침
      fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent(`docs/${user}`)}`)
        .then(r => r.json()).then(d => setUserTree(d)).catch(() => {});
      // 새 파일 바로 열기
      setSelectedFile(fileName);
      setContent(template);
      setOriginal(template);
      setEditMode(true);
      setShowGraph(false);
      setDiffMode(false);
      modalAlert(`"${fileName}" 파일이 생성되었습니다.`);
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || '파일 생성 실패');
    }
  }

  async function handleRenameFile(oldPath) {
    const ext = oldPath.endsWith('.md') ? '' : '.md';
    const base = oldPath.replace(/\.md$/, '');
    const newName = window.prompt('새 파일 이름', base);
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
    fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent(ref)}`)
      .then(r => r.json()).then(d => setUserTree(d)).catch(() => {});
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
      setFileStatus(prev => { const next = { ...prev }; next[path] = 'deleted'; return next; });
      fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent(`docs/${user}`)}`)
        .then(r => r.json()).then(d => setUserTree(d)).catch(() => {});
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

  async function handleGitPull() {
    if (!await modalConfirm('main 브랜치의 최신 내용을 작업공간에 동기화하시겠습니까?')) return;
    try {
      // ensure-branch로 브랜치 보장
      await fetch(`${API}/${projectCode}/ensure-branch`, {
        method: 'POST', headers: { 'X-User-Id': user },
      });
      // edit/rebase로 main 동기화
      const res = await fetch(`${API}/${projectCode}/edit/rebase`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      });
      if (res.ok) {
        // 트리 새로고침
        const mRes = await fetch(`${API}/${projectCode}/tree?ref=main`);
        if (mRes.ok) setMainTree(await mRes.json());
        const uRes = await fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent(`docs/${user}`)}`);
        if (uRes.ok) {
          const uTree = await uRes.json();
          setUserTree(uTree);
          // fileStatus 재계산
          const mainShaMap = {};
          const mainPathSet = new Set();
          ((await mRes.clone().json())?.tree || []).forEach(f => { mainShaMap[f.path] = f.sha; mainPathSet.add(f.path); });
          const userPathSet = new Set();
          const status = {};
          (uTree?.tree || []).forEach(f => {
            userPathSet.add(f.path);
            if (!mainPathSet.has(f.path)) status[f.path] = 'added';
            else if (mainShaMap[f.path] !== f.sha) status[f.path] = 'modified';
          });
          mainPathSet.forEach(p => { if (!userPathSet.has(p)) status[p] = 'deleted'; });
          setFileStatus(status);
        }
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

  const barTitle = `${project?.name || ''} (${projectCode})`;
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

  const modified = content !== original;
  const currentTree = activeTab === 'main' ? mainTree : (userTree || mainTree);
  const items = (currentTree?.tree || []).filter(i => i.type !== 'tree').sort((a, b) => a.path.localeCompare(b.path));
  const hasChanges = Object.keys(fileStatus).length > 0;

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
        pullIcon={
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="6" cy="4" r="2" /><circle cx="6" cy="20" r="2" /><circle cx="18" cy="12" r="2" />
            <line x1="6" y1="6" x2="6" y2="18" />
            <path d="M18 14v4c0 1-1 2-2 2H8" />
          </svg>
        }
      />
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
                발행신청
              </button>
            )}
          </div>
          {items.map(item => {
            const st = activeTab === 'workspace' ? fileStatus[item.path] : null;
            return (
              <div key={item.path}
                className={`de-file ${selectedFile === item.path ? 'de-file-active' : ''} ${st ? `de-file-${st}` : ''}`}
                onClick={() => { setShowGraph(false); setDiffMode(false); handleFileClick(item.path); }}>
                {activeTab === 'workspace' && statusBadge(item.path)}
                <span className="de-file-name">{item.path}</span>
                {activeTab === 'workspace' && globalEditMode && !['business.md', 'srs.md', 'architecture.md'].includes(item.path) && (
                  <span className="de-file-actions">
                    <button className="de-file-act" title="이름 변경" onClick={e => { e.stopPropagation(); handleRenameFile(item.path); }}>✎</button>
                    <button className="de-file-act de-file-act-del" title="삭제" onClick={e => { e.stopPropagation(); handleDeleteFile(item.path); }}>✕</button>
                  </span>
                )}
              </div>
            );
          })}
        </div>
        <div className="de-main">
          {showGraph ? (
            <DocGitGraph projectCode={projectCode} />
          ) : selectedFile ? (
            <>
              <div className="de-toolbar">
                <span className="de-filename">{selectedFile} {modified && <span className="de-modified">●</span>}</span>
                <div className="de-actions">
                  {editMode ? (
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
                      {globalEditMode && <button className="de-btn de-btn-danger" onClick={handleDelete}>삭제</button>}
                    </>
                  )}
                </div>
              </div>
              <div className="de-body">
                {diffMode ? (
                  <DiffAnalysis oldText={mainContent} newText={content} fileName={selectedFile} activeTab={diffTab} onTabChange={setDiffTab} />
                ) : editMode ? (
                  <textarea className="de-textarea" value={content}
                    onChange={e => setContent(e.target.value)} spellCheck={false} />
                ) : (
                  <div className="de-preview">
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
