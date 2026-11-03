import React, { useEffect, useState, useCallback, useMemo } from 'react';
import config from '../config';
import { modalAlert } from '@platform/web-ui/components/AlertModal';
import DataGrid from '@platform/web-ui/components/DataGrid';
import './AccountManager.css';
import { authFetch } from '../platform/auth';

const BASE_API = config.API_BASE + '/Account';

// 사내 사용자 조회(CommonAPI 대리). 사외에서는 USER_API_BASE 가 비어 있어 503 이 정상이다.
// 서버가 { error } 형태로 사유를 내려주므로 그대로 사용자에게 보여준다.
async function fetchLookup(path) {
  const res = await authFetch(`${BASE_API}${path}`);
  const data = await res.json().catch(() => null);
  if (!res.ok) {
    throw new Error((data && data.error) || `조회 실패 (HTTP ${res.status})`);
  }
  return data;
}

// 조회 결과 → 등록 폼 초기값. 미등록자만 폼으로 넘어온다.
function lookupToUserForm(found) {
  return {
    knoxId: found.knoxId || '',
    name: found.name || '',
    email: found.email || '',
    authGroupIds: [],
    orgUnitIds: [],
  };
}

// 조회 결과의 부가 정보(부서/직급/회사) — 폼 상단 안내용.
function lookupSummary(found) {
  return [found.department, found.title, found.company].filter(Boolean).join(' · ');
}

const TABS = [
  { key: 'users', label: 'Users' },
  { key: 'authGroups', label: 'Auth Groups' },
  { key: 'orgUnits', label: 'Org Units' },
];

const PERM_LABELS = { C: 8, R: 4, U: 2, D: 1 };

const ORG_TYPE_ORDER = { company: 0, division: 1, team: 2, group: 3 };
const ORG_TYPES = ['company', 'division', 'team', 'group'];

// 앱 스코프 목록 (D9) — platform 관리 화면의 앱 선택 select 옵션. 신규 앱 추가 시 여기만 갱신.
const APPS = ['platform', 'gojira', 'ragaas'];

// 조직을 트리 깊이(최상위 → 하위) 순으로 정렬한다.
//
// type(company/division/team/group) 으로 정렬하던 것을 깊이 기준으로 바꿨다 —
// 실제 계층과 type 값이 어긋난 데이터가 있어(상위 조직이 team 인 경우 등) type 정렬로는
// 소속 경로가 뒤섞여 보였다. parentOrgUnitId 체인이 진실이다.
function sortOrgUnits(units, allUnits) {
  const pool = allUnits || units;
  const depth = orgDepthMap(pool);
  return [...units].sort((a, b) => {
    const d = (depth[a._id] ?? 99) - (depth[b._id] ?? 99);
    return d !== 0 ? d : String(a.name || '').localeCompare(String(b.name || ''));
  });
}

// orgDepthMap — 조직 id → 루트로부터의 깊이. 순환 참조는 그 지점에서 멈춘다.
function orgDepthMap(units) {
  const parent = {};
  units.forEach(o => { parent[o._id] = o.parentOrgUnitId || ''; });
  const depth = {};
  units.forEach(o => {
    let d = 0;
    const walked = new Set([o._id]);
    for (let cur = parent[o._id]; cur; cur = parent[cur]) {
      if (walked.has(cur)) break;
      walked.add(cur);
      d += 1;
    }
    depth[o._id] = d;
  });
  return depth;
}

// orgAncestorIds — 해당 조직의 상위 조직 id 들(자기 자신 제외, 가까운 순).
// 존재하지 않는 조직을 가리키면 거기서 멈춘다.
function orgAncestorIds(id, units) {
  const byId = {};
  units.forEach(o => { byId[o._id] = o; });
  const out = [];
  const walked = new Set([id]);
  let cur = byId[id]?.parentOrgUnitId || '';
  while (cur && byId[cur] && !walked.has(cur)) {
    walked.add(cur);
    out.push(cur);
    cur = byId[cur].parentOrgUnitId || '';
  }
  return out;
}

// orgDescendantIds — 해당 조직의 하위 조직 id 들(자기 자신 제외).
function orgDescendantIds(id, units) {
  const children = {};
  units.forEach(o => {
    const p = o.parentOrgUnitId || '';
    if (!children[p]) children[p] = [];
    children[p].push(o._id);
  });
  const out = [];
  const queue = [...(children[id] || [])];
  const seen = new Set(queue);
  while (queue.length) {
    const cur = queue.shift();
    out.push(cur);
    (children[cur] || []).forEach(c => {
      if (!seen.has(c)) { seen.add(c); queue.push(c); }
    });
  }
  return out;
}

// withOrgAncestors — 선택한 조직들에 상위 조직을 더한다(백엔드 db.WithOrgAncestors 와 같은 규칙).
// 저장 시 서버가 한 번 더 확장하지만, 화면에서도 즉시 보여야 사용자가 결과를 예측할 수 있다.
function withOrgAncestors(ids, units) {
  const out = [];
  const seen = new Set();
  const add = (id) => { if (id && !seen.has(id)) { seen.add(id); out.push(id); } };
  (ids || []).forEach(id => {
    add(id);
    orgAncestorIds(id, units).forEach(add);
  });
  return out;
}

// app: 앱 스코프(D8) — 'platform'(기본) | 'gojira' | 'ragaas'. authGroups/notices 는
// 이 스코프로 분리 관리된다(전역 신원 + 앱별 권한). orgUnits/userProject/connection 은
// 조직도처럼 전역 성격이라 스코프하지 않는다(계약 미확정이면 이 함수 시그니처만 바뀜).
export default function AccountManager({ user, app = 'platform' }) {
  const [tab, setTab] = useState('users');
  const [users, setUsers] = useState([]);
  const [authGroups, setAuthGroups] = useState([]);
  const [orgUnits, setOrgUnits] = useState([]);
  const [editItem, setEditItem] = useState(null); // { mode: 'create'|'edit', tab, data }

  // platform 스코프 = 전체 조회(메타 관리 콘솔), 그 외 스코프는 자기 앱 것만 (D9).
  const isPlatform = app === 'platform';
  const listApp = isPlatform ? 'all' : app;

  const withApp = useCallback((path) => `${BASE_API}${path}${path.includes('?') ? '&' : '?'}app=${encodeURIComponent(listApp)}`, [listApp]);

  const fetchAll = useCallback(() => {
    authFetch(withApp('/users')).then(r => r.json()).then(d => setUsers(Array.isArray(d) ? d : [])).catch(() => {});
    authFetch(withApp('/authGroups')).then(r => r.json()).then(d => setAuthGroups(Array.isArray(d) ? d : [])).catch(() => {});
    authFetch(`${BASE_API}/orgUnits`).then(r => r.json()).then(d => setOrgUnits(Array.isArray(d) ? d : [])).catch(() => {});
  }, [withApp]);

  useEffect(() => { fetchAll(); }, [fetchAll]);

  async function handleSave(tabKey, mode, data) {
    const path = mode === 'create' ? `/${tabKey}` : `/${tabKey}/${data._id}`;
    // authGroups 생성/수정: platform 스코프는 body.app 에 의존(쿼리 파라미터 없음),
    // 앱 스코프는 기존대로 쿼리 파라미터(app=<scope>)가 body.app 없을 때 채운다 (D9).
    const url = tabKey === 'authGroups' && !isPlatform ? withApp(path) : `${BASE_API}${path}`;
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
    const res = await authFetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      modalAlert(err.error || '저장 실패');
      return;
    }
    setEditItem(null);
    fetchAll();
  }

  async function handleDelete(tabKey, id) {
    if (!window.confirm('비활성화 하시겠습니까?')) return;
    await authFetch(`${BASE_API}/${tabKey}/${id}`, { method: 'DELETE' });
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
            users={users} authGroups={authGroups} orgUnits={orgUnits} isPlatform={isPlatform}
            onEdit={(u) => setEditItem({ mode: 'edit', tab: 'users', data: { ...u } })}
            onDelete={(id) => handleDelete('users', id)}
            onCreate={(data, summary) => setEditItem({
              mode: 'create', tab: 'users',
              data: data || { knoxId: '', name: '', email: '', authGroupIds: [], orgUnitIds: [] },
              summary,
            })}
          />
        )}
        {tab === 'authGroups' && (
          <AuthGroupTable
            authGroups={authGroups} isPlatform={isPlatform}
            onEdit={(g) => setEditItem({ mode: 'edit', tab: 'authGroups', data: { ...g } })}
            onDelete={(id) => handleDelete('authGroups', id)}
            onCreate={() => setEditItem({ mode: 'create', tab: 'authGroups', data: isPlatform
              ? { code: '', name: '', description: '', authority: {}, app: 'platform' }
              : { code: '', name: '', description: '', authority: {} } })}
          />
        )}
        {tab === 'orgUnits' && (
          <OrgUnitTable
            orgUnits={orgUnits}
            onEdit={(o) => setEditItem({ mode: 'edit', tab: 'orgUnits', data: { ...o } })}
            onDelete={(id) => handleDelete('orgUnits', id)}
            onCreate={() => setEditItem({ mode: 'create', tab: 'orgUnits', data: { name: '', type: 'team', parentOrgUnitId: '' } })}
          />
        )}
      </div>

      {editItem && (
        <EditDialog
          item={editItem}
          authGroups={authGroups}
          orgUnits={orgUnits}
          isPlatform={isPlatform}
          onClose={() => setEditItem(null)}
          onSave={(data) => handleSave(editItem.tab, editItem.mode, data)}
        />
      )}
    </div>
  );
}

// --- Users 테이블 (Kendo Grid — 소팅/필터, 계약 04 web-ui DataGrid 소비) ---
function UserTable({ users, authGroups, orgUnits, isPlatform, onEdit, onDelete, onCreate }) {
  const [lookupId, setLookupId] = useState('');
  const [searching, setSearching] = useState(false);

  // Knox ID 로 임직원을 찾아 바로 등록 폼을 연다. 이미 등록된 사람은 폼을 열지 않는다.
  async function searchByKnoxId() {
    const id = lookupId.trim();
    if (!id) return;
    setSearching(true);
    try {
      const found = await fetchLookup(`/lookup/${encodeURIComponent(id)}`);
      if (found.registered) {
        modalAlert(`'${found.knoxId}' (${found.name}) 은(는) 이미 등록된 사용자입니다.`);
        return;
      }
      onCreate(lookupToUserForm(found), lookupSummary(found));
      setLookupId('');
    } catch (err) {
      modalAlert(err.message);
    } finally {
      setSearching(false);
    }
  }

  // 파생 표시 필드를 행에 미리 평탄화 — 소팅/필터가 표시 문자열 기준으로 동작하게 한다.
  const rows = useMemo(() => {
    const groupMap = {};
    authGroups.forEach(g => { groupMap[g._id] = g; });
    return users.map(u => {
      const ids = u.authGroupIds || [];
      // 매핑에 없는 id(다른 앱 그룹 등, platform 스코프가 아니면 로드 안 됨)는 표시하지 않는다 —
      // raw id 노출 방지 (D9). platform 스코프에서는 그룹이 다 로드되므로 전부 이름으로 뜬다.
      const groupNames = ids.map(id => {
        const g = groupMap[id];
        if (!g) return null;
        return isPlatform ? `${g.name}(${g.app})` : g.name;
      }).filter(Boolean).join(', ');
      // platform 스코프 전용 "소속 앱" 컬럼 — 사용자가 속한 그룹들의 app distinct
      const apps = isPlatform
        ? [...new Set(ids.map(id => groupMap[id]?.app).filter(Boolean))].join(', ')
        : undefined;
      return {
        ...u,
        groupNames,
        apps,
        orgPath: sortOrgUnits((u.orgUnitIds || []).map(id => orgUnits.find(o => o._id === id)).filter(Boolean), orgUnits)
          .map(o => o.name).join(' > '),
      };
    });
  }, [users, authGroups, orgUnits, isPlatform]);

  const ActionsCell = (props) => (
    <td className="am-actions">
      <button onClick={() => onEdit(props.dataItem)}>편집</button>
      <button onClick={() => onDelete(props.dataItem._id)}>삭제</button>
    </td>
  );

  const columns = [
    { field: 'knoxId', title: 'Knox ID', width: '160px' },
    { field: 'name', title: '이름', width: '140px' },
    { field: 'email', title: '이메일' },
    { field: 'groupNames', title: '권한그룹' },
    ...(isPlatform ? [{ field: 'apps', title: '소속 앱', width: '140px' }] : []),
    { field: 'orgPath', title: '조직' },
    { title: ' ', cell: ActionsCell, filterable: false, sortable: false, width: '130px' },
  ];

  return (
    <>
      <div className="am-toolbar">
        <span className="am-count">{users.length}명</span>
        <input
          className="am-lookup-input"
          placeholder="Knox ID 로 임직원 검색"
          value={lookupId}
          onChange={e => setLookupId(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') searchByKnoxId(); }}
        />
        <button className="am-lookup-btn" onClick={searchByKnoxId} disabled={searching}>
          {searching ? '검색 중...' : '검색'}
        </button>
        <button className="am-add-btn" onClick={() => onCreate()}>+ 사용자 추가</button>
      </div>
      <DataGrid
        data={rows}
        initialSort={[{ field: 'knoxId', dir: 'asc' }]}
        columns={columns}
      />
    </>
  );
}

// --- AuthGroups 테이블 --- (platform 스코프에서만 ID/앱 컬럼 노출 — 여러 앱 그룹이 섞여
// 보이므로 구분자가 필요하다. D9)
function AuthGroupTable({ authGroups, isPlatform, onEdit, onDelete, onCreate }) {
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
          <tr>
            <th>이름</th><th>코드</th>
            {isPlatform && <th>앱</th>}
            <th>설명</th><th>권한</th><th></th>
          </tr>
        </thead>
        <tbody>
          {authGroups.map(g => (
            <tr key={g._id}>
              <td>{g.name}</td>
              <td>{g.code}</td>
              {isPlatform && <td>{g.app}</td>}
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
          <tr><th>이름</th><th>유형</th><th>상위 조직</th><th></th></tr>
        </thead>
        <tbody>
          {sorted.map(o => (
            <tr key={o._id}>
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
// 동명이인 선택 목록.
//
// 같은 이름이 계열사까지 포함해 여럿 나오므로 부서·회사·ID 로 좁힐 수 있게 한다.
// 이미 등록된 사람은 회색으로 표시하고 선택할 수 없다 — 중복 등록은 어차피 실패한다.
// 바깥을 클릭하면 이 목록만 닫힌다(편집 다이얼로그는 그대로 열려 있어야 한다).
function KnoxCandidateList({ candidates, onPick, onClose }) {
  const [dept, setDept] = useState('');
  const [company, setCompany] = useState('');
  const [id, setId] = useState('');
  const ref = React.useRef(null);

  useEffect(() => {
    function onDocMouseDown(e) {
      if (ref.current && !ref.current.contains(e.target)) onClose();
    }
    document.addEventListener('mousedown', onDocMouseDown);
    return () => document.removeEventListener('mousedown', onDocMouseDown);
  }, [onClose]);

  const has = (v, q) => String(v || '').toLowerCase().includes(q.trim().toLowerCase());
  const shown = candidates.filter(c =>
    (!dept || has(c.department, dept)) &&
    (!company || has(c.company, company)) &&
    (!id || has(c.knoxId, id)));

  return (
    <div className="am-candidates" ref={ref}>
      <div className="am-candidates-head">
        <span>동명이인 {candidates.length}명 — 선택하세요</span>
        <button type="button" className="am-tag-remove" onClick={onClose}>x</button>
      </div>
      <div className="am-candidates-filters">
        <input placeholder="부서 필터" value={dept} onChange={e => setDept(e.target.value)} />
        <input placeholder="회사 필터" value={company} onChange={e => setCompany(e.target.value)} />
        <input placeholder="ID 필터" value={id} onChange={e => setId(e.target.value)} />
      </div>
      <ul className="am-candidates-list">
        {shown.length === 0 && <li className="am-candidate-empty">조건에 맞는 사람이 없습니다.</li>}
        {shown.map(c => (
          <li
            key={c.knoxId}
            className={`am-candidate${c.registered ? ' am-candidate-disabled' : ''}`}
            onClick={() => { if (!c.registered) onPick(c); }}
          >
            <span className="am-candidate-name">{c.name}</span>
            <span className="am-candidate-id">{c.knoxId}</span>
            <span className="am-candidate-meta">
              {[c.department, c.company, c.title].filter(Boolean).join(' · ')}
            </span>
            {c.registered && <span className="am-candidate-badge">등록됨</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}

function EditDialog({ item, authGroups, orgUnits, isPlatform, onClose, onSave }) {
  const [form, setForm] = useState(item.data);
  // 사내 조회로 채워진 부가정보(부서·직급·회사) — 폼 상단 안내
  const [summary, setSummary] = useState(item.summary || '');
  // 이름 검색 결과가 여러 명이면 선택 목록을 띄운다. null 이면 닫힘.
  const [candidates, setCandidates] = useState(null);
  const [busy, setBusy] = useState(false);
  const isCreate = item.mode === 'create';
  const title = isCreate ? '추가' : '편집';

  // 조회 결과를 폼에 반영. 이미 등록된 사람은 호출 측에서 걸러진다.
  function applyLookup(found) {
    setForm(prev => ({ ...prev, knoxId: found.knoxId || '', name: found.name || '', email: found.email || '' }));
    setSummary(lookupSummary(found));
  }

  async function searchById() {
    const id = (form.knoxId || '').trim();
    if (!id) { modalAlert('Knox ID 를 입력하세요.'); return; }
    setBusy(true);
    try {
      const found = await fetchLookup(`/lookup/${encodeURIComponent(id)}`);
      if (found.registered) { modalAlert(`'${found.knoxId}' (${found.name}) 은(는) 이미 등록된 사용자입니다.`); return; }
      applyLookup(found);
    } catch (err) {
      modalAlert(err.message);
    } finally {
      setBusy(false);
    }
  }

  async function searchByName() {
    const name = (form.name || '').trim();
    if (!name) { modalAlert('이름을 입력하세요.'); return; }
    setBusy(true);
    try {
      const list = await fetchLookup(`/lookupByName?name=${encodeURIComponent(name)}`);
      if (!Array.isArray(list) || list.length === 0) {
        modalAlert(`'${name}' 임직원 정보를 찾을 수 없습니다.`);
        return;
      }
      if (list.length === 1) {
        const found = list[0];
        if (found.registered) { modalAlert(`'${found.knoxId}' (${found.name}) 은(는) 이미 등록된 사용자입니다.`); return; }
        applyLookup(found);
        return;
      }
      setCandidates(list); // 동명이인 — 사용자가 고른다
    } catch (err) {
      modalAlert(err.message);
    } finally {
      setBusy(false);
    }
  }

  function set(key, val) { setForm(prev => ({ ...prev, [key]: val })); }

  function toggleArrayItem(key, id) {
    setForm(prev => {
      const arr = prev[key] || [];
      return { ...prev, [key]: arr.includes(id) ? arr.filter(x => x !== id) : [...arr, id] };
    });
  }

  // 조직 추가 — 상위 조직을 함께 넣는다. 저장 시 서버도 같은 확장을 하지만
  // 화면에서 즉시 보여야 사용자가 결과를 예측할 수 있다.
  function addOrgUnit(id) {
    setForm(prev => ({ ...prev, orgUnitIds: withOrgAncestors([...(prev.orgUnitIds || []), id], orgUnits) }));
  }

  // 조직 제거 — 상위를 빼면 그 아래 소속도 함께 뺀다.
  // 상위 없이 하위만 남으면 저장 시 서버가 상위를 되살려 제거가 무효가 되기 때문이다.
  function removeOrgUnit(id) {
    const drop = new Set([id, ...orgDescendantIds(id, orgUnits)]);
    setForm(prev => ({ ...prev, orgUnitIds: (prev.orgUnitIds || []).filter(x => !drop.has(x)) }));
  }

  // 편집 진입 시 누락된 상위 조직을 미리 채운다 — 규칙 도입 이전에 저장된 사용자는
  // 하위 조직만 갖고 있어, 그대로 두면 화면과 저장 결과가 어긋난다.
  useEffect(() => {
    if (item.tab !== 'users' || !orgUnits.length) return;
    setForm(prev => {
      const cur = prev.orgUnitIds || [];
      if (!cur.length) return prev;
      const expanded = withOrgAncestors(cur, orgUnits);
      return expanded.length === cur.length ? prev : { ...prev, orgUnitIds: expanded };
    });
  }, [item.tab, orgUnits]);

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
              {summary && <div className="am-lookup-summary">{summary}</div>}
              <label>Knox ID
                <div className="am-field-row">
                  <input value={form.knoxId || ''} onChange={e => set('knoxId', e.target.value)} disabled={!isCreate} />
                  {isCreate && (
                    <button type="button" className="am-lookup-btn" onClick={searchById} disabled={busy}>검색</button>
                  )}
                </div>
              </label>
              <label>이름
                <div className="am-field-row">
                  <input value={form.name || ''} onChange={e => set('name', e.target.value)} />
                  {isCreate && (
                    <button type="button" className="am-lookup-btn" onClick={searchByName} disabled={busy}>검색</button>
                  )}
                </div>
              </label>
              {candidates && (
                <KnoxCandidateList
                  candidates={candidates}
                  onPick={(c) => { applyLookup(c); setCandidates(null); }}
                  onClose={() => setCandidates(null)}
                />
              )}
              <label>이메일<input value={form.email || ''} onChange={e => set('email', e.target.value)} /></label>
              <label>권한그룹</label>
              <div className="am-tag-list">
                {(form.authGroupIds || []).map(id => {
                  const grp = authGroups.find(g => g._id === id);
                  // 다른 앱 스코프의 그룹(현재 스코프에 로드 안 됨)은 raw id 를 노출하지 않고
                  // 숨긴다 — Users 테이블의 groupNames 와 동일한 규칙(D9).
                  if (!grp) return null;
                  const label = isPlatform ? `${grp.name}(${grp.app})` : grp.name;
                  return (
                    <span key={id} className="am-tag">
                      {label}
                      <button className="am-tag-remove" onClick={() => toggleArrayItem('authGroupIds', id)}>x</button>
                    </span>
                  );
                })}
              </div>
              <select className="am-tag-select" value="" onChange={e => { if (e.target.value) toggleArrayItem('authGroupIds', e.target.value); }}>
                <option value="">권한그룹 추가...</option>
                {authGroups.filter(g => !(form.authGroupIds || []).includes(g._id)).map(g => (
                  <option key={g._id} value={g._id}>
                    {isPlatform ? `${g.name} (${g.code}, ${g.app})` : `${g.name} (${g.code})`}
                  </option>
                ))}
              </select>
              <label>조직 (상위 조직 자동 포함)</label>
              <div className="am-tag-list">
                {sortOrgUnits((form.orgUnitIds || []).map(id => orgUnits.find(o => o._id === id)).filter(Boolean), orgUnits).map(org => (
                  <span key={org._id} className="am-tag">
                    {org.name} ({org.type})
                    <button className="am-tag-remove" onClick={() => removeOrgUnit(org._id)}>x</button>
                  </span>
                ))}
              </div>
              <select className="am-tag-select" value="" onChange={e => { if (e.target.value) addOrgUnit(e.target.value); }}>
                <option value="">조직 추가...</option>
                {sortOrgUnits(orgUnits).filter(o => !(form.orgUnitIds || []).includes(o._id)).map(o => (
                  <option key={o._id} value={o._id}>{o.name} ({o.type})</option>
                ))}
              </select>
            </>
          )}
          {item.tab === 'authGroups' && (
            <>
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
