import React, { useState, useEffect } from 'react';
import './IssueDetailDialog.css';

const STATUS_OPTIONS = ['To Do', 'In Progress', 'Done'];
const PRIORITY_OPTIONS = ['Highest', 'High', 'Medium', 'Low', 'Lowest'];

/* Basic tab fields (common to all types) */
const BASIC_FIELDS = [
  { key: 'summary', label: 'Summary' },
  { key: 'description', label: 'Description', multiline: true },
  { key: 'status', label: 'Status', type: 'select', options: STATUS_OPTIONS, path: 'name' },
  { key: 'priority', label: 'Priority', type: 'select', options: PRIORITY_OPTIONS, path: 'name' },
  { key: 'duedate', label: 'Due Date', type: 'date' },
  { key: 'reporter', label: 'Reporter', readonly: true, path: 'name' },
];

/* Tab definitions per issue type */
const TAB_DEFS = {
  VOS: [
    { name: 'Basic', fields: [
      ...BASIC_FIELDS,
      { key: 'client', label: 'Client' },
    ]},
  ],
  Feature: [
    { name: 'Basic', fields: BASIC_FIELDS },
    { name: 'Use Scenario', fields: [
      { key: 'actor', label: 'Actor' },
      { key: 'precondition', label: 'Pre Condition', multiline: true },
      { key: 'basicflow', label: 'Basic Flow', multiline: true },
      { key: 'alternativeflow', label: 'Alternative Flow', multiline: true },
      { key: 'postcondition', label: 'Post Condition', multiline: true },
    ]},
  ],
  Requirement: [
    { name: 'Basic', fields: [
      ...BASIC_FIELDS,
      { key: 'reqtype', label: 'Type', readonly: true },
    ]},
  ],
  Requirement_QA: [
    { name: 'Basic', fields: [
      ...BASIC_FIELDS,
      { key: 'reqtype', label: 'Type', readonly: true },
    ]},
    { name: 'Quality Attribute Scenario', fields: [
      { key: 'qa', label: 'Quality' },
      { key: 'source', label: 'Source' },
      { key: 'stimulus', label: 'Stimulus' },
      { key: 'environment', label: 'Environment' },
      { key: 'artifact', label: 'Artifact' },
      { key: 'response', label: 'Response' },
      { key: 'measure', label: 'Measure' },
    ]},
    { name: 'Priority Score', fields: [
      { key: 'qad', label: 'QAD', multiline: true },
      { key: 'qai', label: 'QAI', multiline: true },
      { key: 'qas', label: 'QAS', multiline: true },
    ]},
  ],
  'Architectueral Design': [
    { name: 'Basic', fields: [
      { key: 'summary', label: 'Summary' },
      { key: 'status', label: 'Status', type: 'select', options: STATUS_OPTIONS, path: 'name' },
      { key: 'priority', label: 'Priority', type: 'select', options: PRIORITY_OPTIONS, path: 'name' },
      { key: 'duedate', label: 'Due Date', type: 'date' },
      { key: 'reporter', label: 'Reporter', readonly: true, path: 'name' },
    ]},
    { name: 'Architecture Design', fields: [
      { key: 'adiagram', label: 'Structure Diagram', multiline: true },
      { key: 'bdiagram', label: 'Behavior Diagram', multiline: true },
      { key: 'objectives', label: 'Design Objective', multiline: true },
      { key: 'description', label: 'Description', multiline: true },
      { key: 'pros', label: 'Pros', multiline: true },
      { key: 'cons', label: 'Cons', multiline: true },
      { key: 'score', label: 'Score' },
      { key: 'eval', label: 'Evaluation', multiline: true },
      { key: 'decision', label: 'Decision', multiline: true },
    ]},
    { name: 'Trade Off', fields: [
      { key: 'risk', label: 'Risks', multiline: true },
      { key: 'mitigation', label: 'TradeOffs', multiline: true },
      { key: 'sensitivity', label: 'Sensitivities', multiline: true },
    ]},
  ],
  'Detaied Design': [
    { name: 'Basic', fields: [
      { key: 'summary', label: 'Summary' },
      { key: 'status', label: 'Status', type: 'select', options: STATUS_OPTIONS, path: 'name' },
      { key: 'priority', label: 'Priority', type: 'select', options: PRIORITY_OPTIONS, path: 'name' },
      { key: 'duedate', label: 'Due Date', type: 'date' },
      { key: 'reporter', label: 'Reporter', readonly: true, path: 'name' },
    ]},
    { name: 'Detailed Design', fields: [
      { key: 'adiagram', label: 'Structure Diagram', multiline: true },
      { key: 'bdiagram', label: 'Behavior Diagram', multiline: true },
      { key: 'objectives', label: 'Design Objective', multiline: true },
      { key: 'description', label: 'Description', multiline: true },
      { key: 'dmodel', label: 'Design Model', multiline: true },
    ]},
  ],
  Test: [
    { name: 'Basic', fields: [
      { key: 'summary', label: 'Summary' },
      { key: 'status', label: 'Status', type: 'select', options: STATUS_OPTIONS, path: 'name' },
      { key: 'priority', label: 'Priority', type: 'select', options: PRIORITY_OPTIONS, path: 'name' },
      { key: 'duedate', label: 'Due Date', type: 'date' },
      { key: 'reporter', label: 'Reporter', readonly: true, path: 'name' },
    ]},
  ],
};

function getTabDefs(issue) {
  if (issue.issueType === 'Requirement' && (issue.reqtype === 'QA' || issue.reqtype === 'NFR')) {
    return TAB_DEFS.Requirement_QA;
  }
  return TAB_DEFS[issue.issueType] || TAB_DEFS.Test;
}

function getAllFields(tabs) {
  return tabs.flatMap((t) => t.fields);
}

function getFieldValue(issue, field) {
  const val = issue[field.key];
  if (val == null) return '';
  if (field.path && typeof val === 'object') return val[field.path] || '';
  return val;
}

function renderField(f, form, editMode, handleChange) {
  if (!editMode || f.readonly) {
    return <div className="idd-value">{form[f.key] || '—'}</div>;
  }
  if (f.type === 'select') {
    return (
      <select className="idd-input" value={form[f.key]} onChange={(e) => handleChange(f.key, e.target.value)}>
        {f.options.map((opt) => <option key={opt} value={opt}>{opt}</option>)}
      </select>
    );
  }
  if (f.type === 'date') {
    return <input className="idd-input" type="date" value={form[f.key] || ''} onChange={(e) => handleChange(f.key, e.target.value)} />;
  }
  if (f.multiline) {
    return <textarea className="idd-input idd-textarea" value={form[f.key]} onChange={(e) => handleChange(f.key, e.target.value)} />;
  }
  return <input className="idd-input" type="text" value={form[f.key]} onChange={(e) => handleChange(f.key, e.target.value)} />;
}

export default function IssueDetailDialog({ issue, editMode, onClose, onSave }) {
  const [form, setForm] = useState({});
  const [activeTab, setActiveTab] = useState(0);
  const tabs = getTabDefs(issue);
  const allFields = getAllFields(tabs);

  useEffect(() => {
    const initial = {};
    allFields.forEach((f) => {
      initial[f.key] = getFieldValue(issue, f);
    });
    setForm(initial);
    setActiveTab(0);
  }, [issue]); // eslint-disable-line react-hooks/exhaustive-deps

  function handleChange(key, value) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  function handleSave() {
    const data = {};
    allFields.forEach((f) => {
      if (f.readonly) return;
      const original = getFieldValue(issue, f);
      const current = form[f.key];
      if (current !== original) {
        if (f.path && typeof issue[f.key] === 'object') {
          data[f.key] = { ...issue[f.key], [f.path]: current };
        } else {
          data[f.key] = current;
        }
      }
    });
    if (Object.keys(data).length > 0) {
      onSave(issue.key, data);
    }
    onClose();
  }

  const currentFields = tabs[activeTab]?.fields || [];

  return (
    <div className="idd-overlay" onClick={onClose}>
      <div className="idd-dialog" onClick={(e) => e.stopPropagation()}>
        <div className="idd-header">
          <span className="idd-title">{issue.subKey || issue.key} — {issue.issueType}</span>
          <button className="idd-close" onClick={onClose}>&times;</button>
        </div>

        {/* Tabs */}
        {tabs.length > 1 && (
          <div className="idd-tabs">
            {tabs.map((tab, i) => (
              <button
                key={i}
                className={`idd-tab ${activeTab === i ? 'idd-tab-active' : ''}`}
                onClick={() => setActiveTab(i)}
              >
                {tab.name}
              </button>
            ))}
          </div>
        )}

        <div className="idd-body">
          {(() => {
            const ROW_GROUPS = [
              new Set(['status', 'priority', 'duedate']),
              new Set(['qad', 'qai', 'qas']),
            ];
            const getGroup = (key) => ROW_GROUPS.findIndex((g) => g.has(key));
            const result = [];
            let rowGroup = [];
            let rowIdx = -1;
            const flushRow = () => {
              if (rowGroup.length > 0) {
                result.push(
                  <div key={`inline-row-${rowIdx}`} className="idd-inline-row">
                    {rowGroup.map((rf) => (
                      <div key={rf.key} className="idd-field idd-field-inline">
                        <label className="idd-label">{rf.label}</label>
                        {renderField(rf, form, editMode, handleChange)}
                      </div>
                    ))}
                  </div>
                );
                rowGroup = [];
              }
            };
            currentFields.forEach((f) => {
              const gi = getGroup(f.key);
              if (gi >= 0) {
                if (rowGroup.length > 0 && rowIdx !== gi) flushRow();
                rowIdx = gi;
                rowGroup.push(f);
              } else {
                flushRow();
                result.push(
                  <div key={f.key} className="idd-field">
                    <label className="idd-label">{f.label}</label>
                    {renderField(f, form, editMode, handleChange)}
                  </div>
                );
              }
            });
            flushRow();
            return result;
          })()}
        </div>

        <div className="idd-footer">
          {editMode && (
            <button className="idd-save-btn" onClick={handleSave}>저장</button>
          )}
          <button className="idd-cancel-btn" onClick={onClose}>
            {editMode ? '취소' : '닫기'}
          </button>
        </div>
      </div>
    </div>
  );
}
