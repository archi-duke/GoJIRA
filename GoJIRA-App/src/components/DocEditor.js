import React, { useState, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { modalAlert, modalConfirm, modalPrompt } from './AlertModal';
import CommonBar from './CommonBar';
import config from '../config';
import './DocEditor.css';

const API = config.PLATFORM_API + '/DocRepo';

const NAV_ITEMS = [
  { key: 'docs', label: 'Docs' },
  { key: 'issues', label: 'Issues' },
  { key: 'relationMap', label: 'Relation Map' },
  { key: 'satam', label: 'SATAM' },
];

export default function DocEditor({ project, user, onBack, currentPage, onNavigate }) {
  const projectCode = project?.jiraCodes?.main?.jiraCode || '';
  const hasRepo = !!project?.documents?.repoName;

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
    if (!hasRepo) return;
    fetch(`${API}/${projectCode}/edit/status`, { headers: { 'X-User-Id': user } })
      .then(r => r.json()).then(d => {
        setHasPending(d.hasPending);
        if (d.hasPending) setBranch(d.branch);
      }).catch(() => {});
  }, [projectCode, user, hasRepo]);

  async function startEdit() {
    const res = await fetch(`${API}/${projectCode}/edit/start`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
    });
    if (!res.ok) { modalAlert('작업 공간 생성 실패'); return; }
    const d = await res.json();
    setBranch(d.branch);
    setStarted(true);
    fetchTree();
  }

  function resumeEdit() { setStarted(true); fetchTree(); }

  async function discardEdit() {
    if (!await modalConfirm('작업 내용을 폐기하시겠습니까?')) return;
    await fetch(`${API}/${projectCode}/edit/discard`, {
      method: 'DELETE', headers: { 'X-User-Id': user },
    });
    setHasPending(false); setStarted(false); setSelectedFile(null); setContent('');
  }

  function fetchTree() {
    fetch(`${API}/${projectCode}/tree?ref=main`)
      .then(r => r.json()).then(d => setTree(d)).catch(() => {});
  }

  async function handleFileClick(path) {
    if (content !== original && selectedFile) {
      if (!await modalConfirm('저장하지 않은 변경이 있습니다. 이동하시겠습니까?')) return;
    }
    const res = await fetch(`${API}/${projectCode}/edit/file?path=${encodeURIComponent(path)}`, {
      headers: { 'X-User-Id': user },
    });
    if (res.ok) {
      const d = await res.json();
      setSelectedFile(path); setContent(d.content); setOriginal(d.content); setPreviewMode(false);
    } else {
      const res2 = await fetch(`${API}/${projectCode}/file?path=${encodeURIComponent(path)}&ref=main`);
      if (res2.ok) {
        const d = await res2.json();
        const text = d.content ? atob(d.content) : '';
        setSelectedFile(path); setContent(text); setOriginal(text); setPreviewMode(false);
      }
    }
  }

  async function handleSave() {
    setSaving(true);
    await fetch(`${API}/${projectCode}/edit/file`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ path: selectedFile, content }),
    });
    setOriginal(content); setSaving(false);
  }

  async function handleCommit() {
    const msg = await modalPrompt('커밋 메시지', `${selectedFile || '문서'} 수정`);
    if (!msg) return;
    if (content !== original && selectedFile) await handleSave();
    const res = await fetch(`${API}/${projectCode}/edit/commit`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ message: msg }),
    });
    if (res.ok) modalAlert('커밋 완료');
    else { const err = await res.json().catch(() => ({})); modalAlert(err.error || '커밋 실패'); }
  }

  async function handleCreatePR() {
    const title = await modalPrompt('PR 제목', `[${projectCode}] 설계문서 배포 요청`);
    if (!title) return;
    if (content !== original && selectedFile) await handleSave();
    const res = await fetch(`${API}/${projectCode}/edit/pr`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'X-User-Id': user },
      body: JSON.stringify({ title }),
    });
    if (res.ok) modalAlert('PR이 생성되었습니다.');
    else { const err = await res.json().catch(() => ({})); modalAlert(err.error || 'PR 생성 실패'); }
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
  const items = (tree?.tree || []).filter(i => i.type !== 'tree').sort((a, b) => a.path.localeCompare(b.path));

  if (!started) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <CommonBar title={barTitle} onBack={onBack} nav={nav} />
        <div className="de-start">
        {hasPending ? (
          <div>
            <p>이전에 작업하던 내용이 있습니다. ({branch})</p>
            <button className="de-btn" onClick={resumeEdit}>이어서 작업</button>
            <button className="de-btn de-btn-danger" onClick={discardEdit}>폐기</button>
          </div>
        ) : (
          <div>
            <p>문서를 편집하려면 작업을 시작하세요.</p>
            <button className="de-btn de-btn-primary" onClick={startEdit}>편집 시작</button>
          </div>
        )}
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <CommonBar title={barTitle} onBack={onBack} nav={nav} />
      <div className="de-editor">
      <div className="de-sidebar">
        <div className="de-sidebar-header">{branch}</div>
        {items.map(item => (
          <div key={item.path}
            className={`de-file ${selectedFile === item.path ? 'de-file-active' : ''}`}
            onClick={() => handleFileClick(item.path)}>
            {item.path}
          </div>
        ))}
      </div>
      <div className="de-main">
        {selectedFile ? (
          <>
            <div className="de-toolbar">
              <span className="de-filename">{selectedFile} {modified && <span className="de-modified">●</span>}</span>
              <div className="de-actions">
                <button className={`de-tab ${!previewMode ? 'de-tab-active' : ''}`} onClick={() => setPreviewMode(false)}>편집</button>
                <button className={`de-tab ${previewMode ? 'de-tab-active' : ''}`} onClick={() => setPreviewMode(true)}>미리보기</button>
                <button className="de-btn" onClick={handleSave} disabled={!modified || saving}>{saving ? '저장중...' : '저장'}</button>
                <button className="de-btn" onClick={handleCommit}>커밋/푸시</button>
                <button className="de-btn de-btn-primary" onClick={handleCreatePR}>배포(PR) 요청</button>
              </div>
            </div>
            <div className="de-body">
              {previewMode ? (
                <div className="de-preview">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
                </div>
              ) : (
                <textarea className="de-textarea" value={content}
                  onChange={e => setContent(e.target.value)} spellCheck={false} />
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
