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

  const [tree, setTree] = useState(null);
  const [changedFiles, setChangedFiles] = useState(new Set());
  const [selectedFile, setSelectedFile] = useState(null);
  const [content, setContent] = useState('');
  const [original, setOriginal] = useState('');
  const [saving, setSaving] = useState(false);
  const [editMode, setEditMode] = useState(false);

  // 진입 시 main 트리 + 사용자 브랜치 트리 비교
  useEffect(() => {
    if (!hasRepo) return;
    const branch = `docs/${user}`;

    // main 트리
    fetch(`${API}/${projectCode}/tree?ref=main`)
      .then(r => r.json()).then(mainTree => {
        setTree(mainTree);

        // 사용자 브랜치 트리와 비교
        fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent(branch)}`)
          .then(r => { if (!r.ok) throw new Error(); return r.json(); })
          .then(userTree => {
            const mainShaMap = {};
            (mainTree?.tree || []).forEach(f => { mainShaMap[f.path] = f.sha; });
            const changed = new Set();
            (userTree?.tree || []).forEach(f => {
              if (mainShaMap[f.path] !== f.sha) changed.add(f.path);
            });
            // main에만 있는 파일 (삭제된 파일)은 제외
            setChangedFiles(changed);
          })
          .catch(() => setChangedFiles(new Set()));
      })
      .catch(() => {});
  }, [projectCode, hasRepo, user]);

  async function handleFileClick(path) {
    if (editMode && content !== original) {
      if (!await modalConfirm('저장하지 않은 변경이 있습니다. 이동하시겠습니까?')) return;
    }
    const res = await fetch(`${API}/${projectCode}/file?path=${encodeURIComponent(path)}&ref=main`);
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
      setChangedFiles(prev => new Set([...prev, selectedFile]));
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
        .then(r => r.json()).then(d => setTree(d)).catch(() => {});
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
      body: JSON.stringify({ title: `[${projectCode}] 설계문서 배포 요청` }),
    });
    if (res.ok) {
      modalAlert('발행 신청(PR)이 생성되었습니다.');
    } else {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || 'PR 생성 실패');
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
  const items = (tree?.tree || []).filter(i => i.type !== 'tree').sort((a, b) => a.path.localeCompare(b.path));
  const hasChanges = changedFiles.size > 0;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <CommonBar title={barTitle} onBack={onBack} nav={nav} />
      <div className="de-editor">
        <div className="de-sidebar">
          <div className="de-sidebar-header">
            <span>main</span>
            <button
              className={`de-btn ${hasChanges ? 'de-btn-primary' : ''}`}
              style={{ marginLeft: 'auto', padding: '3px 10px', fontSize: '11px' }}
              onClick={handleCreatePR}
              disabled={!hasChanges}
              title={hasChanges ? `${changedFiles.size}개 파일 변경됨` : '변경사항 없음'}
            >
              발행신청
            </button>
          </div>
          {items.map(item => {
            const isChanged = changedFiles.has(item.path);
            return (
              <div key={item.path}
                className={`de-file ${selectedFile === item.path ? 'de-file-active' : ''} ${isChanged ? 'de-file-changed' : ''}`}
                onClick={() => handleFileClick(item.path)}>
                {isChanged && <span className="de-file-badge">M</span>}
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
                      <button className="de-btn" onClick={() => setEditMode(true)}>편집</button>
                      <button className="de-btn de-btn-danger" onClick={handleDelete}>삭제</button>
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
