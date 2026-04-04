import React, { useEffect, useState, useCallback } from 'react';
import config from '../config';
import './AccountManager.css';

const API = config.API_BASE + '/Account';

const TABS = [
  { key: 'users', label: 'Users' },
  { key: 'authGroups', label: 'Auth Groups' },
  { key: 'orgUnits', label: 'Org Units' },
];

const PERM_LABELS = { C: 8, R: 4, U: 2, D: 1 };

const ORG_TYPE_ORDER = { company: 0, division: 1, team: 2, group: 3 };
const ORG_TYPES = ['company', 'division', 'team', 'group'];

function sortOrgUnits(units) {
  return [...units].sort((a, b) => (ORG_TYPE_ORDER[a.type] ?? 99) - (ORG_TYPE_ORDER[b.type] ?? 99));
}

export default function AccountManager({ user }) {
  const [tab, setTab] = useState('users');
  const [users, setUsers] = useState([]);
  const [authGroups, setAuthGroups] = useState([]);
  const [orgUnits, setOrgUnits] = useState([]);
  const [editItem, setEditItem] = useState(null); // { mode: 'create'|'edit', tab, data }

  const fetchAll = useCallback(() => {
    fetch(`${API}/users`).then(r => r.json()).then(d => setUsers(Array.isArray(d) ? d : [])).catch(() => {});
    fetch(`${API}/authGroups`).then(r => r.json()).then(d => setAuthGroups(Array.isArray(d) ? d : [])).catch(() => {});
    fetch(`${API}/orgUnits`).then(r => r.json()).then(d => setOrgUnits(Array.isArray(d) ? d : [])).catch(() => {});
  }, []);

  useEffect(() => { fetchAll(); }, [fetchAll]);

  async function handleSave(tabKey, mode, data) {
    const url = mode === 'create' ? `${API}/${tabKey}` : `${API}/${tabKey}/${data._id}`;
    const method = mode === 'create' ? 'POST' : 'PUT';
    // 수정 시 메타 필드 제거
    const body = { ...data };
    if (mode === 'edit') {
      delete body._id;
      delete body.createdAt;
      delete body.updatedAt;
      delete body.createdBy;
      delete body.updatedBy;
      delete body.isActive;
    }
    const res = await fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      alert(err.error || '저장 실패');
      return;
    }
    setEditItem(null);
    fetchAll();
  }

  async function handleDelete(tabKey, id) {
    if (!window.confirm('비활성화 하시겠습니까?')) return;
    await fetch(`${API}/${tabKey}/${id}`, { method: 'DELETE' });
    fetchAll();
  }

  return (
    <div className="am-wrapper">
      <div className="am-tabs">
        {TABS.map(t => (
          <button key={t.key} className={`am-tab ${tab === t.key ? 'am-tab-active' : ''}`} onClick={() => setTab(t.key)}>
            {t.label}
          </button>
        ))}
      </div>

      <div className="am-content">
        {tab === 'users' && (
          <UserTable
            users={users} authGroups={authGroups} orgUnits={orgUnits}
            onEdit={(u) => setEditItem({ mode: 'edit', tab: 'users', data: { ...u } })}
            onDelete={(id) => handleDelete('users', id)}
            onCreate={() => setEditItem({ mode: 'create', tab: 'users', data: { knoxId: '', name: '', email: '', authGroupIds: [], orgUnitIds: [] } })}
          />
        )}
        {tab === 'authGroups' && (
          <AuthGroupTable
            authGroups={authGroups}
            onEdit={(g) => setEditItem({ mode: 'edit', tab: 'authGroups', data: { ...g } })}
            onDelete={(id) => handleDelete('authGroups', id)}
            onCreate={() => setEditItem({ mode: 'create', tab: 'authGroups', data: { code: '', name: '', description: '', authority: {} } })}
          />
        )}
        {tab === 'orgUnits' && (
          <OrgUnitTable
            orgUnits={orgUnits}
            onEdit={(o) => setEditItem({ mode: 'edit', tab: 'orgUnits', data: { ...o } })}
            onDelete={(id) => handleDelete('orgUnits', id)}
            onCreate={() => setEditItem({ mode: 'create', tab: 'orgUnits', data: { code: '', name: '', type: 'team', parentOrgUnitId: '' } })}
          />
        )}
      </div>

      {editItem && (
        <EditDialog
          item={editItem}
          authGroups={authGroups}
          orgUnits={orgUnits}
          onClose={() => setEditItem(null)}
          onSave={(data) => handleSave(editItem.tab, editItem.mode, data)}
        />
      )}
    </div>
  );
}

// --- Users 테이블 ---
function UserTable({ users, authGroups, orgUnits, onEdit, onDelete, onCreate }) {
  const groupMap = {};
  authGroups.forEach(g => { groupMap[g._id] = g.name; });
  const orgMap = {};
  orgUnits.forEach(o => { orgMap[o._id] = o.name; });

  return (
    <>
      <div className="am-toolbar">
        <span className="am-count">{users.length}명</span>
        <button className="am-add-btn" onClick={onCreate}>+ 사용자 추가</button>
      </div>
      <table className="am-table">
        <thead>
          <tr>
            <th>Knox ID</th><th>이름</th><th>이메일</th><th>권한그룹</th><th>조직</th><th></th>
          </tr>
        </thead>
        <tbody>
          {users.map(u => (
            <tr key={u._id}>
              <td>{u.knoxId}</td>
              <td>{u.name}</td>
              <td>{u.email}</td>
              <td>{(u.authGroupIds || []).map(id => groupMap[id] || id).join(', ')}</td>
              <td>{sortOrgUnits((u.orgUnitIds || []).map(id => orgUnits.find(o => o._id === id)).filter(Boolean)).map(o => o.name).join(' > ')}</td>
              <td className="am-actions">
                <button onClick={() => onEdit(u)}>편집</button>
                <button onClick={() => onDelete(u._id)}>삭제</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

// --- AuthGroups 테이블 ---
function AuthGroupTable({ authGroups, onEdit, onDelete, onCreate }) {
  function renderAuth(authority) {
    if (!authority) return '-';
    return Object.entries(authority).map(([code, bits]) => {
      const perms = Object.entries(PERM_LABELS).filter(([, b]) => bits & b).map(([l]) => l).join('');
      return `${code}:${perms}`;
    }).join(', ');
  }

  return (
    <>
      <div className="am-toolbar">
        <span className="am-count">{authGroups.length}개</span>
        <button className="am-add-btn" onClick={onCreate}>+ 권한그룹 추가</button>
      </div>
      <table className="am-table">
        <thead>
          <tr><th>코드</th><th>이름</th><th>설명</th><th>권한</th><th></th></tr>
        </thead>
        <tbody>
          {authGroups.map(g => (
            <tr key={g._id}>
              <td>{g.code}</td>
              <td>{g.name}</td>
              <td>{g.description}</td>
              <td className="am-auth-cell">{renderAuth(g.authority)}</td>
              <td className="am-actions">
                <button onClick={() => onEdit(g)}>편집</button>
                <button onClick={() => onDelete(g._id)}>삭제</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

// --- OrgUnits 테이블 ---
function OrgUnitTable({ orgUnits, onEdit, onDelete, onCreate }) {
  const parentMap = {};
  orgUnits.forEach(o => { parentMap[o._id] = o.name; });
  const sorted = sortOrgUnits(orgUnits);

  return (
    <>
      <div className="am-toolbar">
        <span className="am-count">{orgUnits.length}개</span>
        <button className="am-add-btn" onClick={onCreate}>+ 조직 추가</button>
      </div>
      <table className="am-table">
        <thead>
          <tr><th>코드</th><th>이름</th><th>유형</th><th>상위 조직</th><th></th></tr>
        </thead>
        <tbody>
          {sorted.map(o => (
            <tr key={o._id}>
              <td>{o.code}</td>
              <td>{o.name}</td>
              <td>{o.type}</td>
              <td>{parentMap[o.parentOrgUnitId] || o.parentOrgUnitId || '-'}</td>
              <td className="am-actions">
                <button onClick={() => onEdit(o)}>편집</button>
                <button onClick={() => onDelete(o._id)}>삭제</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

// --- 편집 다이얼로그 ---
function EditDialog({ item, authGroups, orgUnits, onClose, onSave }) {
  const [form, setForm] = useState(item.data);
  const isCreate = item.mode === 'create';
  const title = isCreate ? '추가' : '편집';

  function set(key, val) { setForm(prev => ({ ...prev, [key]: val })); }

  function toggleArrayItem(key, id) {
    setForm(prev => {
      const arr = prev[key] || [];
      return { ...prev, [key]: arr.includes(id) ? arr.filter(x => x !== id) : [...arr, id] };
    });
  }

  function toggleAuthBit(code, bit) {
    setForm(prev => {
      const auth = { ...(prev.authority || {}) };
      auth[code] = (auth[code] || 0) ^ bit;
      return { ...prev, authority: auth };
    });
  }

  function addAuthCode() {
    const code = prompt('권한 코드 입력 (예: ALLPRJ, USR, ORG, PRJ)');
    if (!code) return;
    setForm(prev => ({ ...prev, authority: { ...(prev.authority || {}), [code]: 0 } }));
  }

  return (
    <div className="am-dlg-overlay" onClick={onClose}>
      <div className="am-dlg" onClick={e => e.stopPropagation()}>
        <div className="am-dlg-header">
          <span>{title}</span>
          <button className="am-dlg-close" onClick={onClose}>X</button>
        </div>
        <div className="am-dlg-body">
          {item.tab === 'users' && (
            <>
              <label>Knox ID<input value={form.knoxId || ''} onChange={e => set('knoxId', e.target.value)} disabled={!isCreate} /></label>
              <label>이름<input value={form.name || ''} onChange={e => set('name', e.target.value)} /></label>
              <label>이메일<input value={form.email || ''} onChange={e => set('email', e.target.value)} /></label>
              <label>권한그룹</label>
              <div className="am-tag-list">
                {(form.authGroupIds || []).map(id => {
                  const grp = authGroups.find(g => g._id === id);
                  return (
                    <span key={id} className="am-tag">
                      {grp ? `${grp.name} (${grp.code})` : id}
                      <button className="am-tag-remove" onClick={() => toggleArrayItem('authGroupIds', id)}>x</button>
                    </span>
                  );
                })}
              </div>
              <select className="am-tag-select" value="" onChange={e => { if (e.target.value) toggleArrayItem('authGroupIds', e.target.value); }}>
                <option value="">권한그룹 추가...</option>
                {authGroups.filter(g => !(form.authGroupIds || []).includes(g._id)).map(g => (
                  <option key={g._id} value={g._id}>{g.name} ({g.code})</option>
                ))}
              </select>
              <label>조직</label>
              <div className="am-tag-list">
                {sortOrgUnits((form.orgUnitIds || []).map(id => orgUnits.find(o => o._id === id)).filter(Boolean)).map(org => (
                  <span key={org._id} className="am-tag">
                    {org.name} ({org.type})
                    <button className="am-tag-remove" onClick={() => toggleArrayItem('orgUnitIds', org._id)}>x</button>
                  </span>
                ))}
              </div>
              <select className="am-tag-select" value="" onChange={e => { if (e.target.value) toggleArrayItem('orgUnitIds', e.target.value); }}>
                <option value="">조직 추가...</option>
                {sortOrgUnits(orgUnits).filter(o => !(form.orgUnitIds || []).includes(o._id)).map(o => (
                  <option key={o._id} value={o._id}>{o.name} ({o.type})</option>
                ))}
              </select>
            </>
          )}
          {item.tab === 'authGroups' && (
            <>
              <label>코드<input value={form.code || ''} onChange={e => set('code', e.target.value)} disabled={!isCreate} /></label>
              <label>이름<input value={form.name || ''} onChange={e => set('name', e.target.value)} /></label>
              <label>설명<input value={form.description || ''} onChange={e => set('description', e.target.value)} /></label>
              <label>권한 <button className="am-small-btn" onClick={addAuthCode}>+ 코드 추가</button></label>
              <table className="am-auth-table">
                <thead>
                  <tr><th>코드</th><th>C</th><th>R</th><th>U</th><th>D</th><th></th></tr>
                </thead>
                <tbody>
                  {Object.entries(form.authority || {}).map(([code, bits]) => (
                    <tr key={code}>
                      <td>{code}</td>
                      {[8, 4, 2, 1].map(b => (
                        <td key={b}>
                          <input type="checkbox" checked={!!(bits & b)} onChange={() => toggleAuthBit(code, b)} />
                        </td>
                      ))}
                      <td><button className="am-small-btn" onClick={() => {
                        setForm(prev => {
                          const auth = { ...(prev.authority || {}) };
                          delete auth[code];
                          return { ...prev, authority: auth };
                        });
                      }}>x</button></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
          {item.tab === 'orgUnits' && (
            <>
              <label>코드<input value={form.code || ''} onChange={e => set('code', e.target.value)} disabled={!isCreate} /></label>
              <label>이름<input value={form.name || ''} onChange={e => set('name', e.target.value)} /></label>
              <label>유형
                <select value={form.type || 'team'} onChange={e => set('type', e.target.value)}>
                  {ORG_TYPES.map(t => <option key={t} value={t}>{t.charAt(0).toUpperCase() + t.slice(1)}</option>)}
                </select>
              </label>
              <label>상위 조직
                <select value={form.parentOrgUnitId || ''} onChange={e => set('parentOrgUnitId', e.target.value)}>
                  <option value="">없음</option>
                  {sortOrgUnits(orgUnits).filter(o => o._id !== form._id).map(o => (
                    <option key={o._id} value={o._id}>{o.name} ({o.type})</option>
                  ))}
                </select>
              </label>
            </>
          )}
        </div>
        <div className="am-dlg-footer">
          <button className="am-save-btn" onClick={() => onSave(form)}>저장</button>
          <button className="am-cancel-btn" onClick={onClose}>취소</button>
        </div>
      </div>
    </div>
  );
}
