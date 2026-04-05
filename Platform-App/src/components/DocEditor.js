import React, { useState, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { modalAlert, modalConfirm } from './AlertModal';
import CommonBar from './CommonBar';
import config from '../config';
import './DocEditor.css';

const API = config.API_BASE + '/DocRepo';

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

  // 진입 시 main 트리 + 사용자 브랜치 트리 비교
  useEffect(() => {
    if (!hasRepo) return;
    const branch = `docs/${user}`;

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
      <CommonBar title={barTitle} onBack={onBack} nav={nav} onEditChange={handleEditModeChange} />
      <div className="de-editor">
        <div className="de-sidebar">
          <div className="de-sidebar-tabs">
            <button className={`de-sidebar-tab ${activeTab === 'main' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => setActiveTab('main')}>Main</button>
            <button className={`de-sidebar-tab ${activeTab === 'workspace' ? 'de-sidebar-tab-active' : ''}`}
              onClick={() => setActiveTab('workspace')}>작업공간</button>
          </div>
          <div className="de-sidebar-header">
            <span>{activeTab === 'main' ? 'main' : `docs/${user}`}</span>
            {activeTab === 'workspace' && (
              <button
                className={`de-btn ${hasChanges ? 'de-btn-primary' : ''}`}
                style={{ marginLeft: 'auto', padding: '3px 10px', fontSize: '11px' }}
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
                onClick={() => handleFileClick(item.path)}>
                {activeTab === 'workspace' && statusBadge(item.path)}
                {item.path}
              </div>
            );
          })}
        </div>
        <div className="de-main">
          {selectedFile ? (
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
                      <button className="de-tab de-tab-active">미리보기</button>
                      {globalEditMode && <button className="de-btn" onClick={() => setEditMode(true)}>편집</button>}
                      {globalEditMode && <button className="de-btn de-btn-danger" onClick={handleDelete}>삭제</button>}
                    </>
                  )}
                </div>
              </div>
              <div className="de-body">
                {editMode ? (
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
