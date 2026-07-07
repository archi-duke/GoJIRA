import React, { useEffect, useState, useCallback } from 'react';
import config from '../config';
import './NoticeManager.css';
import { authFetch } from '../platform/auth';

const BASE_API = config.API_BASE + '/Notice';

const STATUS_LABEL = { draft: '게시전', active: '게시중', expired: '게시종료' };
const STATUS_COLOR = { draft: '#888', active: '#2ecc71', expired: '#e74c3c' };

// 앱 스코프 목록 (D9) — platform 관리 화면의 앱 선택 select 옵션. 신규 앱 추가 시 여기만 갱신.
const APPS = ['platform', 'gojira', 'ragaas'];

// app: 앱 스코프(D8) — 이 스코프의 공지만 조회/작성한다. platform 스코프는 전체 조회(D9).
export default function NoticeManager({ user, app = 'platform' }) {
  const [notices, setNotices] = useState([]);
  const [editItem, setEditItem] = useState(null);

  const isPlatform = app === 'platform';
  const listApp = isPlatform ? 'all' : app;

  const withApp = useCallback((path = '') => `${BASE_API}${path}${path.includes('?') ? '&' : '?'}app=${encodeURIComponent(listApp)}`, [listApp]);

  const fetchNotices = useCallback(() => {
    authFetch(withApp()).then(r => r.json()).then(d => setNotices(Array.isArray(d) ? d : [])).catch(() => {});
  }, [withApp]);

  useEffect(() => { fetchNotices(); }, [fetchNotices]);

  async function handleSave(mode, data) {
    // platform 스코프의 생성/수정은 쿼리 파라미터 없이 body.app 에 의존, 앱 스코프는
    // 기존대로 withApp(쿼리 app=<scope>)가 body.app 없을 때 채운다 (D9).
    const path = mode === 'create' ? '' : `/${data._id}`;
    const url = isPlatform ? `${BASE_API}${path}` : withApp(path);
    const method = mode === 'create' ? 'POST' : 'PUT';
    const body = { ...data };
    if (mode === 'edit') {
      delete body._id; delete body.createdAt; delete body.updatedAt;
      delete body.createdBy; delete body.updatedBy; delete body.isActive;
    }
    body.createdBy = body.createdBy || user;
    body.updatedBy = user;
    await authFetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    setEditItem(null);
    fetchNotices();
  }

  async function handleDelete(id) {
    if (!window.confirm('삭제하시겠습니까?')) return;
    await authFetch(`${BASE_API}/${id}`, { method: 'DELETE' });
    fetchNotices();
  }

  function newNotice() {
    const now = new Date();
    const end = new Date(now); end.setDate(end.getDate() + 7);
    return {
      title: '', body: '', bodyType: 'markdown', status: 'draft', isEmergency: false,
      startDate: now.toISOString().slice(0, 16), endDate: end.toISOString().slice(0, 16),
      ...(isPlatform ? { app: 'platform' } : {}),
    };
  }

  return (
    <div className="nm-wrapper">
      <div className="nm-toolbar">
        <span className="nm-count">{notices.length}개</span>
        <button className="nm-add-btn" onClick={() => setEditItem({ mode: 'create', data: newNotice() })}>+ 공지 작성</button>
      </div>
      <table className="nm-table">
        <thead>
          <tr>
            <th>상태</th><th>제목</th>
            {isPlatform && <th>앱</th>}
            <th>작성자</th><th>게시기간</th><th>조회수</th><th>작성일</th><th></th>
          </tr>
        </thead>
        <tbody>
          {notices.map(n => (
            <tr key={n._id}>
              <td>
                <span className="nm-status" style={{ color: STATUS_COLOR[n.status] || '#888' }}>
                  {STATUS_LABEL[n.status] || n.status}
                </span>
              </td>
              <td className="nm-title-cell" onClick={() => setEditItem({ mode: 'edit', data: { ...n, startDate: (n.startDate || '').slice(0, 16), endDate: (n.endDate || '').slice(0, 16) } })}>
                {n.title}
              </td>
              {isPlatform && <td>{n.app || 'platform'}</td>}
              <td>{n.createdBy}</td>
              <td className="nm-date-cell">
                {(n.startDate || '').slice(0, 10)} ~ {(n.endDate || '').slice(0, 10)}
              </td>
              <td>{n.views || 0}</td>
              <td>{(n.createdAt || '').slice(0, 10)}</td>
              <td className="nm-actions">
                <button
                  className={`nm-emergency-btn ${n.isEmergency ? 'nm-emergency-active' : ''}`}
                  onClick={async () => {
                    const next = !n.isEmergency;
                    const msg = next ? `"${n.title}" 공지를 비상 공지로 게시하시겠습니까?` : `"${n.title}" 비상 공지를 해제하시겠습니까?`;
                    if (!window.confirm(msg)) return;
                    const updates = next ? { isEmergency: true, status: 'active' } : { isEmergency: false };
                    await authFetch(`${BASE_API}/${n._id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(updates) });
                    fetchNotices();
                  }}
                >비상</button>
                <button onClick={() => handleDelete(n._id)}>삭제</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {editItem && (
        <NoticeEditDialog
          mode={editItem.mode}
          data={editItem.data}
          isPlatform={isPlatform}
          onClose={() => setEditItem(null)}
          onSave={(data) => handleSave(editItem.mode, data)}
        />
      )}
    </div>
  );
}

function NoticeEditDialog({ mode, data, isPlatform, onClose, onSave }) {
  const [form, setForm] = useState(data);
  const isCreate = mode === 'create';

  function set(key, val) { setForm(prev => ({ ...prev, [key]: val })); }

  function handleSubmit() {
    const body = { ...form };
    // datetime-local → ISO
    if (body.startDate && !body.startDate.includes('Z')) body.startDate = body.startDate + ':00Z';
    if (body.endDate && !body.endDate.includes('Z')) body.endDate = body.endDate + ':00Z';
    onSave(body);
  }

  return (
    <div className="nm-dlg-overlay" onClick={onClose}>
      <div className="nm-dlg" onClick={e => e.stopPropagation()}>
        <div className="nm-dlg-header">
          <span>{isCreate ? '공지 작성' : '공지 수정'}</span>
          <button className="nm-dlg-close" onClick={onClose}>X</button>
        </div>
        <div className="nm-dlg-body">
          {isPlatform && (
            <label>앱
              {isCreate ? (
                <select value={form.app || 'platform'} onChange={e => set('app', e.target.value)}>
                  {APPS.map(a => <option key={a} value={a}>{a}</option>)}
                </select>
              ) : (
                <input value={form.app || 'platform'} disabled />
              )}
            </label>
          )}
          <label>제목<input value={form.title || ''} onChange={e => set('title', e.target.value)} /></label>
          <div className="nm-row">
            <label>상태
              <select value={form.status || 'draft'} onChange={e => set('status', e.target.value)}>
                <option value="draft">게시전</option>
                <option value="active">게시중</option>
                <option value="expired">게시종료</option>
              </select>
            </label>
            <label>본문 형식
              <select value={form.bodyType || 'markdown'} onChange={e => set('bodyType', e.target.value)}>
                <option value="markdown">Markdown</option>
                <option value="html">HTML</option>
              </select>
            </label>
          </div>
          <div className="nm-row">
            <label>게시 시작<input type="datetime-local" value={form.startDate || ''} onChange={e => set('startDate', e.target.value)} /></label>
            <label>게시 종료<input type="datetime-local" value={form.endDate || ''} onChange={e => set('endDate', e.target.value)} /></label>
          </div>
          <label>본문</label>
          <textarea className="nm-body-input" value={form.body || ''} onChange={e => set('body', e.target.value)} rows={12} />
        </div>
        <div className="nm-dlg-footer">
          <button className="nm-save-btn" onClick={handleSubmit}>저장</button>
          <button className="nm-cancel-btn" onClick={onClose}>취소</button>
        </div>
      </div>
    </div>
  );
}
