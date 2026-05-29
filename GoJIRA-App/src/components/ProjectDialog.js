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
  const [selectedMsIdx, setSelectedMsIdx] = useState(null);
  // Gitea repo 이름 중복/유효성 검사 상태 — 신규 과제에서만 사용
  // status: idle | checking | ok | duplicate | invalid | error
  const [repoCheck, setRepoCheck] = useState({ status: 'idle', slug: '', suggestion: '', message: '' });

  useEffect(() => {
    if (project) {
      setForm({ ...project });
      setRepoCheck({ status: 'idle', slug: '', suggestion: '', message: '' });
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
  const isNew = isEdit && !form._id;
  const title = isEdit ? '과제 편집' : '과제 상세정보';

  async function checkRepoName(raw) {
    if (!raw || !raw.trim()) {
      setRepoCheck({ status: 'idle', slug: '', suggestion: '', message: '' });
      return;
    }
    setRepoCheck({ status: 'checking', slug: '', suggestion: '', message: '확인 중...' });
    try {
      const res = await fetch(`${PLATFORM_API}/DocRepo/check?name=${encodeURIComponent(raw)}`);
      const data = await res.json();
      if (!res.ok) {
        setRepoCheck({ status: 'error', slug: '', suggestion: '', message: data.error || '검사 실패' });
        return;
      }
      if (!data.valid) {
        setRepoCheck({ status: 'invalid', slug: '', suggestion: '', message: '영문/숫자/하이픈 1자 이상 필요' });
        return;
      }
      if (data.exists) {
        setRepoCheck({
          status: 'duplicate',
          slug: data.slug,
          suggestion: data.suggestion || '',
          message: `'${data.slug}' 이미 존재`,
        });
        return;
      }
      setRepoCheck({ status: 'ok', slug: data.slug, suggestion: '', message: `사용 가능 (${data.slug})` });
    } catch (e) {
      setRepoCheck({ status: 'error', slug: '', suggestion: '', message: '검사 실패: ' + (e.message || e) });
    }
  }

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
    // 신규 과제는 Gitea repo 이름 입력 + 중복 통과 필수
    if (isNew) {
      const raw = (form.repoName || '').trim();
      if (!raw) {
        alert('Doc Repo 이름을 입력하세요.');
        return;
      }
      if (repoCheck.status === 'duplicate') {
        alert(`이미 존재하는 repo 이름입니다${repoCheck.suggestion ? ` (제안: ${repoCheck.suggestion})` : ''}.`);
        return;
      }
      if (repoCheck.status === 'invalid') {
        alert('Doc Repo 이름은 영문/숫자/하이픈 1자 이상이어야 합니다.');
        return;
      }
      if (repoCheck.status !== 'ok') {
        // blur 안 한 채 저장 누른 경우 — 한 번 더 확인
        await checkRepoName(raw);
        return; // state 반영 후 사용자가 다시 저장 눌러야 안전
      }
    }
    if (form.devTeam) {
      await ensureOrgUnit(form.devTeam, 'team');
    }
    if (form.devGroup) {
      await ensureOrgUnit(form.devGroup, 'group', form.devTeam);
    }
    if (onSave) {
      const result = await onSave(form);
      // onSave 가 false 를 명시 반환하면 다이얼로그를 닫지 않는다 — 신규 과제 생성 시
      // 런처가 동작하지 않아 게이트에서 막힌 경우 등. 다른 반환값(undefined 포함)은 OK.
      if (result === false) return;
    }
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
          <UserMultiField label="Auditors" sectionLabel value={form.auditors || []} users={users} editable={isEdit}
            onChange={(arr) => handleChange('auditors', arr)} />

          {/* Schedules */}
          <div className="dlg-section">마일스톤</div>
          <MilestoneGantt
            schedules={schedules}
            editable={isEdit}
            selectedIdx={selectedMsIdx}
            onSelect={setSelectedMsIdx}
            onChange={handleScheduleChange}
          />

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
            {isNew ? (
              <RepoNameField
                value={form.repoName || ''}
                onChange={(v) => {
                  handleChange('repoName', v);
                  setRepoCheck({ status: 'idle', slug: '', suggestion: '', message: '' });
                }}
                onBlur={() => checkRepoName(form.repoName)}
                check={repoCheck}
                onApplySuggestion={() => {
                  handleChange('repoName', repoCheck.suggestion);
                  checkRepoName(repoCheck.suggestion);
                }}
              />
            ) : (
              <Field label="Doc Repo" value={
                form.documents?.repoName
                  ? `${form.documents.repoOwner || ''}/${form.documents.repoName}`
                  : '(미등록)'
              } editable={false} />
            )}
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

function RepoNameField({ value, onChange, onBlur, check, onApplySuggestion }) {
  const status = check.status;
  let hintColor = '#888';
  if (status === 'ok') hintColor = '#1a7f37';
  else if (status === 'duplicate' || status === 'invalid' || status === 'error') hintColor = '#cf222e';

  return (
    <div className="dlg-field" style={{ gridColumn: '1 / -1' }}>
      <label className="dlg-label">
        Doc Repo 이름 <span style={{ color: '#cf222e' }}>*</span>
      </label>
      <input
        className="dlg-input"
        value={value}
        placeholder="영문/숫자/하이픈 (예: my-project)"
        onChange={(e) => onChange(e.target.value)}
        onBlur={onBlur}
      />
      {check.message && (
        <div style={{ fontSize: 12, color: hintColor, marginTop: 4 }}>
          {check.message}
          {status === 'duplicate' && check.suggestion && (
            <button
              type="button"
              onClick={onApplySuggestion}
              style={{
                marginLeft: 8,
                padding: '2px 8px',
                fontSize: 12,
                cursor: 'pointer',
                border: '1px solid #1a7f37',
                background: '#fff',
                color: '#1a7f37',
                borderRadius: 3,
              }}
            >
              제안: {check.suggestion} 사용
            </button>
          )}
        </div>
      )}
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

function UserMultiField({ label, sectionLabel, value, users, editable, onChange }) {
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

  const labelEl = sectionLabel
    ? <div className="dlg-section">{label}</div>
    : <label className="dlg-label" style={{ marginTop: 8 }}>{label}</label>;

  if (!editable && list.length === 0) {
    return (
      <>
        {labelEl}
        <span className="dlg-value">-</span>
      </>
    );
  }

  return (
    <>
      {labelEl}
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

function MilestoneGantt({ schedules, editable, selectedIdx, onSelect, onChange }) {
  const wrapRef = React.useRef(null);
  const [width, setWidth] = useState(480);
  const H = 60;
  const PAD_L = 24;
  const PAD_R = 24;
  const innerW = Math.max(width - PAD_L - PAD_R, 1);
  const yLine = 28;
  const R = 7;

  useEffect(() => {
    if (!wrapRef.current) return;
    const ro = new ResizeObserver(([entry]) => {
      setWidth(Math.max(entry.contentRect.width, 200));
    });
    ro.observe(wrapRef.current);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    if (!editable || selectedIdx == null) return;
    const onKey = (e) => { if (e.key === 'Escape') onSelect(null); };
    const onMouseDown = (e) => {
      if (e.target.closest('.dlg-ms-g')) return;
      if (e.target.closest('.dlg-ms-edit-popup')) return;
      onSelect(null);
    };
    document.addEventListener('keydown', onKey);
    document.addEventListener('mousedown', onMouseDown);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('mousedown', onMouseDown);
    };
  }, [editable, selectedIdx, onSelect]);

  const items = (schedules || []).map((s, i) => {
    const name = Object.keys(s)[0];
    const dateStr = Object.values(s)[0];
    const date = dateStr ? new Date(dateStr) : null;
    return { idx: i, name, dateStr, date };
  });

  const validDates = items.map((m) => m.date).filter(Boolean);
  const minDate = validDates.length ? new Date(Math.min(...validDates)) : null;
  const maxDate = validDates.length ? new Date(Math.max(...validDates)) : null;
  const span = minDate && maxDate ? maxDate - minDate : 0;
  const allHaveDate = items.length > 0 && items.every((m) => m.date);

  function xFor(idx, date) {
    if (allHaveDate && span > 0) {
      return PAD_L + ((date - minDate) / span) * innerW;
    }
    if (items.length === 1) return PAD_L + innerW / 2;
    return PAD_L + (idx / (items.length - 1)) * innerW;
  }

  const positioned = items.map((m) => ({ ...m, x: xFor(m.idx, m.date) }));

  if (positioned.length === 0) {
    return (
      <div className="dlg-ms-wrap" ref={wrapRef}>
        <div className="dlg-text" style={{ color: '#888' }}>마일스톤이 없습니다.</div>
      </div>
    );
  }

  const firstX = positioned[0].x;
  const lastX = positioned[positioned.length - 1].x;
  const selected = selectedIdx != null ? positioned[selectedIdx] : null;
  const inputDate = selected && selected.dateStr ? selected.dateStr.slice(0, 10) : '';

  return (
    <div className="dlg-ms-wrap" ref={wrapRef}>
      <svg className="dlg-ms-svg" width={width} height={H}>
        <line x1={firstX} y1={yLine} x2={lastX} y2={yLine} className="dlg-ms-line" />
        {positioned.map((m, i) => {
          const isSel = selectedIdx === i;
          return (
            <g
              key={i}
              className={`dlg-ms-g ${editable ? 'editable' : ''} ${isSel ? 'selected' : ''}`}
              onClick={editable ? () => onSelect(i) : undefined}
            >
              <text x={m.x} y={yLine - R - 4} textAnchor="middle" className="dlg-ms-name">
                {m.name}
              </text>
              <circle cx={m.x} cy={yLine} r={R} className="dlg-ms-circle" />
              <text x={m.x} y={yLine + R + 12} textAnchor="middle" className="dlg-ms-date">
                {m.dateStr ? m.dateStr.slice(5, 10) : '-'}
              </text>
            </g>
          );
        })}
      </svg>
      {editable && selected && (() => {
        const isFirst = selected.idx === 0;
        const isLast = selected.idx === positioned.length - 1;
        const tx = isFirst ? '0%' : isLast ? '-100%' : '-50%';
        return (
        <div
          className="dlg-ms-edit-popup"
          style={{ left: selected.x, top: yLine + R + 16, transform: `translateX(${tx})` }}
        >
          <span className="dlg-ms-edit-name">{selected.name}</span>
          <input
            type="date"
            className="dlg-input dlg-ms-edit-input"
            value={inputDate}
            autoFocus
            onChange={(e) => onChange(selected.idx, e.target.value + 'T00:00:00.000Z')}
            onKeyDown={(e) => { if (e.key === 'Escape') onSelect(null); }}
          />
        </div>
        );
      })()}
    </div>
  );
}
