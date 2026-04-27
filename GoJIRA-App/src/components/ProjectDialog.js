import React, { useState, useEffect } from 'react';
import config from '../config';
import './ProjectDialog.css';

const PLATFORM_API = config.PLATFORM_API;

function unitId(u) { return (u && (u._id || u.id)) || ''; }
function findUnit(units, type, name) {
  const trimmed = (name || '').trim();
  if (!trimmed) return null;
  return units.find((u) => u.type === type && u.name === trimmed) || null;
}

const STATUS_OPTIONS = ['To Do', 'In Progress', 'In Review', 'Done', 'Closed'];

function userLabel(u) { return `${u.name} (${u.knoxId})`; }
function findUserByInput(users, input) {
  const trimmed = (input || '').trim();
  if (!trimmed) return null;
  return (
    users.find((u) => userLabel(u) === trimmed) ||
    users.find((u) => u.knoxId === trimmed) ||
    users.find((u) => u.name === trimmed) ||
    null
  );
}

export default function ProjectDialog({ project, mode, onClose, onSave }) {
  const [form, setForm] = useState({});
  const [orgUnits, setOrgUnits] = useState([]);
  const [users, setUsers] = useState([]);

  useEffect(() => {
    if (project) {
      setForm({ ...project });
    }
  }, [project]);

  useEffect(() => {
    fetch(`${PLATFORM_API}/Account/orgUnits`)
      .then((r) => r.json())
      .then((d) => setOrgUnits(Array.isArray(d) ? d : []))
      .catch(() => {});
    fetch(`${PLATFORM_API}/Account/users`)
      .then((r) => r.json())
      .then((d) => setUsers(Array.isArray(d) ? d : []))
      .catch(() => {});
  }, []);

  if (!project) return null;

  const isEdit = mode === 'edit';
  const title = isEdit ? '과제 편집' : '과제 상세정보';

  const teamUnits = orgUnits.filter((u) => u.type === 'team');
  const groupUnits = orgUnits.filter((u) => u.type === 'group');
  const currentTeamUnit = findUnit(teamUnits, 'team', form.devTeam);
  const groupOptions = groupUnits
    .filter((u) => !currentTeamUnit || u.parentOrgUnitId === unitId(currentTeamUnit))
    .map((u) => u.name);

  function handleChange(field, value) {
    setForm((prev) => ({ ...prev, [field]: value }));
  }

  function handleScheduleChange(idx, value) {
    const schedules = [...(form.schedules || [])];
    const key = Object.keys(schedules[idx])[0];
    schedules[idx] = { [key]: value };
    setForm((prev) => ({ ...prev, schedules: schedules }));
  }

  async function ensureOrgUnit(name, type, parentName) {
    const trimmed = (name || '').trim();
    if (!trimmed) return null;
    const existing = findUnit(orgUnits, type, trimmed);
    if (existing) return unitId(existing);
    let parentOrgUnitId = '';
    if (type === 'group' && parentName) {
      const parent = findUnit(orgUnits, 'team', parentName);
      if (parent) parentOrgUnitId = unitId(parent);
    }
    try {
      const res = await fetch(`${PLATFORM_API}/Account/orgUnits`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: trimmed, type, parentOrgUnitId, sortOrder: 0 }),
      });
      const data = await res.json();
      return (data && (data._id || data.id)) || null;
    } catch {
      return null;
    }
  }

  async function handleSubmit() {
    if (form.devTeam) {
      await ensureOrgUnit(form.devTeam, 'team');
    }
    if (form.devGroup) {
      await ensureOrgUnit(form.devGroup, 'group', form.devTeam);
    }
    if (onSave) onSave(form);
    onClose();
  }

  const schedules = form.schedules || [];
  const architect = form.architect || {};
  const pl = form.PL || {};

  return (
    <div className="dlg-overlay" onClick={onClose}>
      <div className="dlg-container" onClick={(e) => e.stopPropagation()}>
        <div className="dlg-header">
          <span className="dlg-title">{title}</span>
          <button className="dlg-close" onClick={onClose}>X</button>
        </div>

        <div className="dlg-body">
          {/* Basic Info */}
          <div className="dlg-section">기본 정보</div>
          <div className="dlg-grid">
            <Field label="과제명" value={form.name} editable={isEdit}
              onChange={(v) => handleChange('name', v)} />
            <Field label="JIRA Code" value={form.jiraCodes?.main?.jiraCode || ''} editable={isEdit && !form._id}
              onChange={(v) => setForm((prev) => ({ ...prev, jiraCodes: { main: { jiraCode: v } } }))} />
            <StatusField label="상태" value={form.status} editable={isEdit}
              onChange={(v) => handleChange('status', v)} />
            <Field label="Feature" value={form.feature} editable={isEdit}
              onChange={(v) => handleChange('feature', v)} />
            <OrgUnitField label="개발팀" value={form.devTeam} editable={isEdit}
              options={teamUnits.map((u) => u.name)}
              onChange={(v) => handleChange('devTeam', v)} />
            <OrgUnitField label="개발그룹" value={form.devGroup} editable={isEdit}
              options={groupOptions}
              onChange={(v) => handleChange('devGroup', v)} />
          </div>

          {/* People */}
          <div className="dlg-section">담당자</div>
          <div className="dlg-grid">
            <UserSingleField label="Architect" value={architect} users={users} editable={isEdit}
              onChange={(u) => handleChange('architect', u || {})} />
            <UserSingleField label="PL" value={pl} users={users} editable={isEdit}
              onChange={(u) => handleChange('PL', u || {})} />
          </div>
          <UserMultiField label="Members" value={form.members || []} users={users} editable={isEdit}
            onChange={(arr) => handleChange('members', arr)} />
          <UserMultiField label="Auditors" value={form.auditors || []} users={users} editable={isEdit}
            onChange={(arr) => handleChange('auditors', arr)} />

          {/* Schedules */}
          <div className="dlg-section">마일스톤</div>
          <table className="dlg-table">
            <thead>
              <tr><th>이름</th><th>기한</th></tr>
            </thead>
            <tbody>
              {schedules.map((s, i) => {
                const name = Object.keys(s)[0];
                const dateVal = Object.values(s)[0];
                const display = dateVal ? dateVal.slice(0, 10) : '';
                return (
                  <tr key={i}>
                    <td>{name}</td>
                    <td>
                      {isEdit ? (
                        <input
                          type="date"
                          className="dlg-input"
                          value={display}
                          onChange={(e) => handleScheduleChange(i, e.target.value + 'T00:00:00.000Z')}
                        />
                      ) : (
                        display
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>

          {/* PLM Code */}
          {form.PLMCode && form.PLMCode.length > 0 && (
            <>
              <div className="dlg-section">PLM Code</div>
              <div className="dlg-text">{form.PLMCode.join(', ')}</div>
            </>
          )}

          {/* 설계문서 저장소 */}
          <div className="dlg-section">설계문서</div>
          <div className="dlg-grid">
            <Field label="Doc Repo" value={
              form.documents?.repoName
                ? `${form.documents.repoOwner || ''}/${form.documents.repoName}`
                : isEdit ? '(설계문서 생성 시 자동 등록)' : '(미등록)'
            } editable={false} />
          </div>
        </div>

        <div className="dlg-footer">
          {isEdit && (
            <button className="dlg-btn dlg-btn-primary" onClick={handleSubmit}>
              저장
            </button>
          )}
          <button className="dlg-btn" onClick={onClose}>
            {isEdit ? '취소' : '닫기'}
          </button>
        </div>
      </div>
    </div>
  );
}

function Field({ label, value, editable, onChange }) {
  return (
    <div className="dlg-field">
      <label className="dlg-label">{label}</label>
      {editable ? (
        <input
          className="dlg-input"
          value={value || ''}
          onChange={(e) => onChange(e.target.value)}
        />
      ) : (
        <span className="dlg-value">{value || '-'}</span>
      )}
    </div>
  );
}

function StatusField({ label, value, editable, onChange }) {
  // 기존 데이터에 표준 외 값이 있을 수 있으므로 동적으로 옵션 머지
  const options = STATUS_OPTIONS.includes(value) || !value
    ? STATUS_OPTIONS
    : [value, ...STATUS_OPTIONS];
  return (
    <div className="dlg-field">
      <label className="dlg-label">{label}</label>
      {editable ? (
        <select
          className="dlg-input"
          value={value || ''}
          onChange={(e) => onChange(e.target.value)}
        >
          <option value="">(선택)</option>
          {options.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
      ) : (
        <span className="dlg-value">{value || '-'}</span>
      )}
    </div>
  );
}

function OrgUnitField({ label, value, editable, options, onChange }) {
  const listId = React.useId();
  return (
    <div className="dlg-field">
      <label className="dlg-label">{label}</label>
      {editable ? (
        <>
          <input
            className="dlg-input"
            list={listId}
            value={value || ''}
            onChange={(e) => onChange(e.target.value)}
          />
          <datalist id={listId}>
            {(options || []).map((opt) => (
              <option key={opt} value={opt} />
            ))}
          </datalist>
        </>
      ) : (
        <span className="dlg-value">{value || '-'}</span>
      )}
    </div>
  );
}

function UserSingleField({ label, value, users, editable, onChange }) {
  const listId = React.useId();
  const currentLabel = value && value.name && value.id ? userLabel({ knoxId: value.id, name: value.name }) : '';
  const [text, setText] = React.useState(currentLabel);
  React.useEffect(() => { setText(currentLabel); }, [currentLabel]);

  if (!editable) {
    return (
      <div className="dlg-field">
        <label className="dlg-label">{label}</label>
        <span className="dlg-value">{(value && value.name) || '-'}</span>
      </div>
    );
  }

  return (
    <div className="dlg-field">
      <label className="dlg-label">{label}</label>
      <input
        className="dlg-input"
        list={listId}
        value={text}
        placeholder="이름 또는 ID"
        onChange={(e) => {
          const v = e.target.value;
          setText(v);
          if (!v.trim()) { onChange(null); return; }
          const u = findUserByInput(users, v);
          if (u && v === userLabel(u)) onChange({ id: u.knoxId, name: u.name });
        }}
        onBlur={() => {
          const u = findUserByInput(users, text);
          if (u) {
            const lab = userLabel(u);
            setText(lab);
            onChange({ id: u.knoxId, name: u.name });
          } else {
            setText(currentLabel);
          }
        }}
      />
      <datalist id={listId}>
        {users.map((u) => <option key={u.knoxId} value={userLabel(u)} />)}
      </datalist>
    </div>
  );
}

function UserMultiField({ label, value, users, editable, onChange }) {
  const listId = React.useId();
  const list = value || [];
  const userById = (knoxId) => users.find((u) => u.knoxId === knoxId);

  function addUser(input) {
    const u = findUserByInput(users, input);
    if (!u) return false;
    if (list.includes(u.knoxId)) return true;
    onChange([...list, u.knoxId]);
    return true;
  }

  function removeAt(i) {
    onChange(list.filter((_, idx) => idx !== i));
  }

  if (!editable && list.length === 0) {
    return (
      <div className="dlg-field" style={{ marginTop: 8 }}>
        <label className="dlg-label">{label}</label>
        <span className="dlg-value">-</span>
      </div>
    );
  }

  return (
    <>
      <label className="dlg-label" style={{ marginTop: 8 }}>{label}</label>
      <div className="dlg-auditor-list">
        {list.map((knoxId, i) => {
          const u = userById(knoxId);
          return (
            <span key={i} className="dlg-auditor-tag">
              {u ? `${u.name} (${u.knoxId})` : knoxId}
              {editable && (
                <button className="dlg-auditor-remove" onClick={() => removeAt(i)}>x</button>
              )}
            </span>
          );
        })}
        {editable && (
          <>
            <input
              className="dlg-auditor-input"
              list={listId}
              placeholder="이름 또는 ID 입력 후 Enter"
              onChange={(e) => {
                const v = e.target.value;
                const u = findUserByInput(users, v);
                if (u && v === userLabel(u)) {
                  if (addUser(v)) e.target.value = '';
                }
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  if (addUser(e.target.value)) e.target.value = '';
                }
              }}
            />
            <datalist id={listId}>
              {users
                .filter((u) => !list.includes(u.knoxId))
                .map((u) => <option key={u.knoxId} value={userLabel(u)} />)}
            </datalist>
          </>
        )}
      </div>
    </>
  );
}
