import React, { useState, useEffect } from 'react';
import config from '../../config';
import './ProjectDialog.css';

export default function ProjectDialog({ project, mode, onClose, onSave }) {
  const [form, setForm] = useState({});
  const [gitUsers, setGitUsers] = useState([]);

  useEffect(() => {
    if (project) {
      setForm({ ...project });
    }
  }, [project]);

  useEffect(() => {
    if (mode === 'edit') {
      fetch(`${config.API_BASE}/Git/users`)
        .then(r => r.json())
        .then(users => { if (Array.isArray(users)) setGitUsers(users); })
        .catch(() => {});
    }
  }, [mode]);

  if (!project) return null;

  const isEdit = mode === 'edit';
  const title = isEdit ? '과제 편집' : '과제 상세정보';

  function handleChange(field, value) {
    setForm((prev) => ({ ...prev, [field]: value }));
  }

  function handleScheduleChange(idx, value) {
    const schedules = [...(form.schedules || [])];
    const key = Object.keys(schedules[idx])[0];
    schedules[idx] = { [key]: value };
    setForm((prev) => ({ ...prev, schedules: schedules }));
  }

  function handleSubmit() {
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
            <Field label="상태" value={form.status} editable={isEdit}
              onChange={(v) => handleChange('status', v)} />
            <Field label="Feature" value={form.feature} editable={isEdit}
              onChange={(v) => handleChange('feature', v)} />
            <Field label="개발팀" value={form.devTeam} editable={isEdit}
              onChange={(v) => handleChange('devTeam', v)} />
            <Field label="개발그룹" value={form.devGroup} editable={isEdit}
              onChange={(v) => handleChange('devGroup', v)} />
          </div>

          {/* People */}
          <div className="dlg-section">담당자</div>
          <div className="dlg-grid">
            <Field label="Architect" value={architect.name || ''} editable={false} />
            <Field label="PL" value={pl.name || ''} editable={false} />
            <Field label="Members"
              value={(form.members || []).join(', ')} editable={false} />
          </div>
          {isEdit && (
            <>
              <label className="dlg-label" style={{marginTop:8}}>Auditors</label>
              <div className="dlg-auditor-list">
                {(form.auditors || []).map((a, i) => (
                  <span key={i} className="dlg-auditor-tag">
                    {a}
                    <button className="dlg-auditor-remove" onClick={() => {
                      setForm(prev => ({ ...prev, auditors: (prev.auditors || []).filter((_, idx) => idx !== i) }));
                    }}>x</button>
                  </span>
                ))}
                <select
                  className="dlg-auditor-select"
                  value=""
                  onChange={(e) => {
                    const val = e.target.value;
                    if (val && !(form.auditors || []).includes(val)) {
                      setForm(prev => ({ ...prev, auditors: [...(prev.auditors || []), val] }));
                    }
                  }}
                >
                  <option value="">+ 승인자 추가</option>
                  {gitUsers
                    .filter(u => !(form.auditors || []).includes(u.login))
                    .map(u => (
                      <option key={u.login} value={u.login}>
                        {u.login}{u.fullName ? ` (${u.fullName})` : ''}
                      </option>
                    ))}
                </select>
                {gitUsers.length === 0 && (
                  <span className="dlg-auditor-warn">Git 사용자 목록을 불러올 수 없습니다</span>
                )}
              </div>
            </>
          )}
          {!isEdit && (form.auditors || []).length > 0 && (
            <div className="dlg-field" style={{marginTop:8}}>
              <label className="dlg-label">Auditors</label>
              <span className="dlg-value">{(form.auditors || []).join(', ')}</span>
            </div>
          )}

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
