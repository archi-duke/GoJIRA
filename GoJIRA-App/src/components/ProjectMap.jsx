import React, { useEffect, useLayoutEffect, useRef, useState, useCallback } from 'react';
import './ProjectMap.css';
import ContextMenu from './ContextMenu';
import ProjectDialog from './ProjectDialog';
import { modalAlert, modalPrompt } from '@platform/web-ui/components/AlertModal';
import config from '../config';
import { detectLauncher, requestPrewarm, requestRemoveClone } from '@platform/web-ui/runtime/launcher';
import LauncherInstallModal from './LauncherInstallModal';
import { authFetch } from '../platform/auth';

const API_BASE = config.API_BASE;
const PLATFORM_API = config.PLATFORM_API;

function getJiraCode(p) {
  return p?.jiraCodes?.main?.jiraCode || '';
}

const MIN_MONTH_WIDTH = 30;
const ROW_HEIGHT = 48;
const MILESTONE_R = 10;
const SIDEBAR_WIDTH = 180;
const DEFAULT_SPAN_YEARS = 3;

function toDate(str) {
  return new Date(str);
}

function daysBetween(a, b) {
  return Math.round((b - a) / (1000 * 60 * 60 * 24));
}

function dayOfMonth(date) {
  return date.getDate();
}

function addMonths(date, n) {
  const d = new Date(date);
  d.setMonth(d.getMonth() + n);
  return d;
}

function monthsBetween(a, b) {
  return (b.getFullYear() - a.getFullYear()) * 12 + (b.getMonth() - a.getMonth());
}

// Slider absolute range: today -5 years ~ today +5 years
function getSliderRange() {
  const now = new Date();
  return {
    min: new Date(now.getFullYear() - 5, now.getMonth(), 1),
    max: new Date(now.getFullYear() + 5, now.getMonth(), 1),
  };
}

function getDefaultView() {
  const now = new Date();
  return {
    start: new Date(now.getFullYear() - 1, now.getMonth(), 1),
    end: addMonths(new Date(now.getFullYear() - 1, now.getMonth(), 1), DEFAULT_SPAN_YEARS * 12),
  };
}

export default function ProjectMap({ onProjectClick, onRelationMap, onSATAM, user }) {
  const [projects, setProjects] = useState([]);
  const [graphWidth, setGraphWidth] = useState(0);
  // 신규 과제 생성 시 런처 미감지 → 설치/실행 안내 모달.
  // { onRetry, onClose } 를 담아 retry 결과를 ensureLauncherReady 의 promise 로 resolve.
  const [launcherGate, setLauncherGate] = useState(null);
  const [viewRange, setViewRange] = useState(() => {
    try {
      const s = JSON.parse(sessionStorage.getItem('gojira_pm_viewRange'));
      if (s) return { start: new Date(s.start), end: new Date(s.end) };
    } catch {}
    return getDefaultView();
  });
  const [alerts, setAlerts] = useState({}); // jiraCode → { issueUpdated, branchDiff, hasOpenPR }
  const [ctxMenu, setCtxMenu] = useState(null); // { x, y, project }
  const [dialog, setDialog] = useState(null);   // { project, mode: 'detail'|'edit' }
  const [saving, setSaving] = useState(null);   // null | { title, sub } — 저장 진행 인디케이터
  const [sortKey, setSortKey] = useState(() => sessionStorage.getItem('gojira_pm_sortKey') || 'startDate');
  const [sortAsc, setSortAsc] = useState(() => sessionStorage.getItem('gojira_pm_sortAsc') !== 'false');
  useEffect(() => { sessionStorage.setItem('gojira_pm_sortKey', sortKey); }, [sortKey]);
  useEffect(() => { sessionStorage.setItem('gojira_pm_sortAsc', String(sortAsc)); }, [sortAsc]);
  useEffect(() => { sessionStorage.setItem('gojira_pm_viewRange', JSON.stringify(viewRange)); }, [viewRange]);

  const graphRef = useRef(null);
  const headerRef = useRef(null);
  const sidebarRef = useRef(null);
  const containerRef = useRef(null);

  useEffect(() => {
    authFetch(`${API_BASE}/Project`, {
      headers: { XBody: JSON.stringify({}) },
    })
      .then((res) => res.json())
      .then((data) => {
        if (Array.isArray(data)) setProjects(data.filter((p) => p.status !== 'Deleted'));
      })
      .catch(() => setProjects([]));

    // 프로젝트 알림 상태 조회
    authFetch(`${API_BASE}/Project/alerts?user=${encodeURIComponent(user || '')}`)
      .then((res) => res.json())
      .then((data) => {
        if (Array.isArray(data)) {
          const map = {};
          data.forEach((a) => { map[a.jiraCode] = a; });
          setAlerts(map);
        }
      })
      .catch(() => {});

    // SSE 구독 — main 머지 시 실시간 알림 갱신
    const es = new EventSource(`${PLATFORM_API}/Notice/stream`);
    es.addEventListener('doc_update', (e) => {
      try {
        const data = JSON.parse(e.data);
        if (data.type === 'project_alert' && data.branchAlerts) {
          // 웹훅으로 갱신된 브랜치 알림 반영
          setAlerts((prev) => {
            const next = { ...prev };
            const code = data.projectCode;
            const hasDiff = (data.branchAlerts || []).includes(user);
            if (hasDiff) {
              next[code] = { ...next[code], jiraCode: code, branchDiff: true };
            } else {
              // 차이 해소됨
              if (next[code]) {
                next[code] = { ...next[code], branchDiff: false };
                if (!next[code].hasOpenPR) delete next[code];
              }
            }
            return next;
          });
        } else if (data.type === 'pr_update') {
          // PR 상태 변경 시 alerts 재조회
          authFetch(`${API_BASE}/Project/alerts?user=${encodeURIComponent(user || '')}`)
            .then((res) => res.json())
            .then((arr) => {
              if (Array.isArray(arr)) {
                const map = {};
                arr.forEach((a) => { map[a.jiraCode] = a; });
                setAlerts(map);
              }
            }).catch(() => {});
        }
      } catch {}
    });
    return () => es.close();
  }, [user]);

  function handleContextMenu(e, project) {
    e.preventDefault();
    setCtxMenu({ x: e.clientX, y: e.clientY, project });
  }

  async function handleMenuAction(action, project) {
    switch (action) {
      case 'detail':
        setDialog({ project, mode: 'detail' });
        break;
      case 'edit':
        setDialog({ project, mode: 'edit' });
        break;
      case 'delete': {
        const code = getJiraCode(project);
        const input = await modalPrompt(
          `"${project.name}" 과제를 영구 삭제합니다.\n서버에 백업 zip 이 만들어진 후 이슈/문서/repo 와 내 PC 로컬 클론이 모두 정리됩니다.\n계속하려면 과제명을 정확히 입력해주세요.`,
          '',
          {
            dangerous: true,
            confirmLabel: '삭제',
            validate: (v) => v === project.name,
          }
        );
        if (input !== project.name) return;
        try {
          const res = await authFetch(`${PLATFORM_API}/Project/${encodeURIComponent(code)}/archive`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'X-User-Id': user || 'batman' },
            body: JSON.stringify({ confirm: true }),
          });
          const data = await res.json();
          if (!res.ok) {
            await modalAlert('삭제 실패: ' + (data.error || res.statusText));
            return;
          }
          setProjects((prev) => prev.filter((p) => getJiraCode(p) !== code));

          // 후처리: 내 PC 로컬 clone 삭제 — launcher 담당(서버는 못 닿음).
          // best-effort: launcher 오프라인/실패해도 전체 삭제는 중단하지 않고 상태만 보고.
          let cloneMsg = '\n로컬 클론: 런처 미감지 — 수동 삭제 필요';
          try {
            const launcher = await detectLauncher();
            if (launcher?.ready) {
              const rc = await requestRemoveClone({ code, user, token: launcher.token });
              cloneMsg = rc.ok
                ? (rc.data?.removed ? '\n로컬 클론: 삭제' : '\n로컬 클론: 없음(이미 정리됨)')
                : `\n로컬 클론: 삭제 실패(${rc.error || '알 수 없음'})`;
            }
          } catch (e) {
            cloneMsg = `\n로컬 클론: 삭제 시도 오류(${e.message})`;
          }

          await modalAlert(`삭제 완료\n백업: ${data.backupPath || '(없음)'}\nGitHub repo: ${data.githubDeleted ? '삭제' : '미삭제'}${cloneMsg}`);
        } catch (e) {
          await modalAlert('삭제 중 오류: ' + e.message);
        }
        break;
      }
      case 'relationMap':
        if (onRelationMap) onRelationMap(project);
        break;
      case 'satam':
        if (onSATAM) onSATAM(project);
        break;
      default:
        break;
    }
  }

  // 신규 과제 생성 게이트: launcher 가 동작 중이어야 진행. 미감지 시 LauncherInstallModal
  // 띄우고 사용자가 "설치 완료 — 다시 시도" 누를 때까지 대기. 사용자가 닫기 누르면 null 반환.
  // 성공 시 launcher 객체 ({version, agents, ready, token}) 반환.
  function ensureLauncherReady() {
    return new Promise((resolve) => {
      (async () => {
        const first = await detectLauncher();
        if (first && first.token) {
          resolve(first);
          return;
        }
        // 모달 띄우고 retry 까지 대기
        setLauncherGate({
          onRetry: async () => {
            const again = await detectLauncher();
            if (again && again.token) {
              setLauncherGate(null);
              resolve(again);
            }
            // 감지 안 되면 모달 유지 — 사용자가 다시 시도하거나 닫기 가능
          },
          onClose: () => {
            setLauncherGate(null);
            resolve(null);
          },
        });
      })();
    });
  }

  async function handleDialogSave(updated) {
    // New project (POST) vs existing (PUT) — 새 과제는 Gitea repo 생성을 포함해 수 초 걸린다.
    const isNew = !projects.some((p) => getJiraCode(p) === getJiraCode(updated));

    // 신규 과제는 launcher 가 반드시 동작해야 한다 — 생성 직후 prewarm 으로 로컬 clone 을
    // 미리 받아두기 위해. 미감지 시 설치/실행 안내 모달로 게이트.
    let launcher = null;
    if (isNew) {
      launcher = await ensureLauncherReady();
      if (!launcher) {
        // 사용자가 게이트를 닫음 — 다이얼로그 유지하고 진행 중단.
        return false;
      }
    }

    setSaving({
      title: isNew ? '과제 생성 중...' : '과제 정보 저장 중...',
      sub: isNew ? '설계문서 저장소 자동 생성을 포함해 수 초 걸릴 수 있습니다.' : '',
    });
    try {
      const res = await authFetch(`${API_BASE}/Project`, {
        method: isNew ? 'POST' : 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updated),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        await modalAlert((isNew ? '과제 생성 실패' : '과제 저장 실패') + ': ' + (err.error || res.statusText));
        return;
      }
      // 신규 생성 직후 — 이미 게이트에서 launcher 확보됨. 백그라운드 prewarm 으로 미리 clone.
      // 사용자가 곧 이 과제를 로고 클릭으로 열 가능성이 높아 EnsureRepo 를 선행하면
      // 에이전트 spawn 까지의 체감 지연이 크게 줄어든다. 실패는 silent.
      if (isNew && launcher && launcher.token) {
        const code = getJiraCode(updated);
        if (code) {
          requestPrewarm({ code, user, token: launcher.token }).catch(() => {});
        }
      }
      // 성공 — 목록 재조회
      const list = await authFetch(`${API_BASE}/Project`, { headers: { XBody: JSON.stringify({}) } }).then((r) => r.json());
      if (Array.isArray(list)) setProjects(list.filter((p) => p.status !== 'Deleted'));
    } catch (e) {
      await modalAlert((isNew ? '과제 생성 오류: ' : '과제 저장 오류: ') + (e.message || e));
    } finally {
      setSaving(null);
    }
  }

  useEffect(() => {
    function measure() {
      if (containerRef.current) {
        setGraphWidth(containerRef.current.clientWidth - SIDEBAR_WIDTH);
      }
    }
    measure();
    const observer = new ResizeObserver(measure);
    if (containerRef.current) observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, []);

  function newProjectTemplate() {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const addMonths = (n) => {
      const d = new Date(today);
      d.setMonth(d.getMonth() + n);
      return d.toISOString();
    };
    return {
      name: '',
      repoName: '',
      jiraCodes: { main: { jiraCode: '' } },
      status: 'In Progress',
      devTeam: '',
      devGroup: '',
      feature: '',
      members: [],
      architect: { name: '', knoxID: '', department: '' },
      PL: { name: '', knoxID: '', department: '' },
      schedules: [
        { PA: today.toISOString() },
        { EA: addMonths(1) },
        { ER1: addMonths(2) },
        { ER2: addMonths(3) },
        { CA: addMonths(4) },
      ],
      PLMCode: [],
    };
  }

  function handleScroll() {
    const el = graphRef.current;
    if (!el) return;
    if (headerRef.current) headerRef.current.scrollLeft = el.scrollLeft;
    if (sidebarRef.current) sidebarRef.current.scrollTop = el.scrollTop;
  }

  // 21: Sort projects
  function getFirstDate(p) {
    const s = (p.schedules || [])[0];
    return s ? toDate(Object.values(s)[0]) : new Date(0);
  }
  function getLastDate(p) {
    const arr = p.schedules || [];
    const s = arr[arr.length - 1];
    return s ? toDate(Object.values(s)[0]) : new Date(0);
  }

  const sortedProjects = [...projects].sort((a, b) => {
    let cmp = 0;
    switch (sortKey) {
      case 'startDate': cmp = getFirstDate(a) - getFirstDate(b); break;
      case 'endDate': cmp = getLastDate(a) - getLastDate(b); break;
      case 'name': cmp = (a.name || '').localeCompare(b.name || ''); break;
      case 'dept': cmp = (a.devTeam || '').localeCompare(b.devTeam || ''); break;
      default: cmp = 0;
    }
    return sortAsc ? cmp : -cmp;
  });

  const allDates = [];
  sortedProjects.forEach((p) => {
    (p.schedules || []).forEach((s) => {
      const val = Object.values(s)[0];
      if (val) allDates.push(toDate(val));
    });
  });

  if (allDates.length === 0 || graphWidth <= 0) {
    return (
      <div className="pm-wrapper" ref={containerRef}>
        <div className="pm-empty">프로젝트 데이터가 없습니다.</div>
      </div>
    );
  }

  const { start: rangeStart, end: rangeEnd } = viewRange;

  // Build month list for the user-selected view range
  const baseMonthList = [];
  const cur = new Date(rangeStart.getFullYear(), rangeStart.getMonth(), 1);
  while (cur < rangeEnd) {
    const monthStart = new Date(cur);
    const monthEnd = new Date(cur.getFullYear(), cur.getMonth() + 1, 1);
    const days = daysBetween(monthStart, monthEnd);
    baseMonthList.push({ year: cur.getFullYear(), month: cur.getMonth() + 1, days });
    cur.setMonth(cur.getMonth() + 1);
  }

  // 가장 긴 프로젝트명 흰 rect 가 첫 마일스톤 좌측에 붙도록, 그만큼 viewRange 이전의
  // 월을 prepend 해 헤더/그리드를 자연스럽게 좌측까지 이어붙인다 (기존 빈 leftPadding 영역 제거).
  let nameOverlayWidth = 0;
  sortedProjects.forEach((p) => {
    const w = (p.name || '').length * 7 + 16;
    if (w > nameOverlayWidth) nameOverlayWidth = w;
  });
  const firstPassEvenWidth = baseMonthList.length > 0 ? graphWidth / baseMonthList.length : MIN_MONTH_WIDTH;
  const extraMonths = firstPassEvenWidth > 0 ? Math.ceil(nameOverlayWidth / firstPassEvenWidth) : 0;
  const prepended = [];
  for (let i = extraMonths; i > 0; i--) {
    const d = addMonths(new Date(rangeStart.getFullYear(), rangeStart.getMonth(), 1), -i);
    const days = daysBetween(
      new Date(d.getFullYear(), d.getMonth(), 1),
      new Date(d.getFullYear(), d.getMonth() + 1, 1)
    );
    prepended.push({ year: d.getFullYear(), month: d.getMonth() + 1, days });
  }
  const monthList = [...prepended, ...baseMonthList];

  const monthCount = monthList.length;
  const evenWidth = monthCount > 0 ? graphWidth / monthCount : MIN_MONTH_WIDTH;
  const months = monthList.map((m) => ({
    ...m,
    label: `${m.year}.${String(m.month).padStart(2, '0')}`,
    width: Math.max(1, evenWidth),
  }));

  // Year groups
  const years = [];
  let curYearGroup = null;
  for (const m of months) {
    if (!curYearGroup || curYearGroup.year !== m.year) {
      curYearGroup = { year: m.year, startIdx: months.indexOf(m), count: 1 };
      years.push(curYearGroup);
    } else {
      curYearGroup.count++;
    }
  }

  // Assign x positions starting at 0 (prepended months absorb what used to be leftPadding)
  let accX = 0;
  for (const m of months) {
    m.x = accX;
    accX += m.width;
  }
  const totalWidth = accX;

  // 가시 타임라인 시작/끝 — overflow 판정 기준은 prepend 포함 확장 범위를 사용
  const visibleStart = months.length > 0 ? toMonthDate(months[0].label) : rangeStart;
  const visibleEnd = rangeEnd;
  const totalHeight = sortedProjects.length * ROW_HEIGHT;

  function dateToX(date) {
    for (let i = 0; i < months.length; i++) {
      const mStart = toMonthDate(months[i].label);
      const mEnd =
        i + 1 < months.length
          ? toMonthDate(months[i + 1].label)
          : new Date(mStart.getFullYear(), mStart.getMonth() + 1, 1);
      if (date >= mStart && date < mEnd) {
        const ratio = daysBetween(mStart, date) / months[i].days;
        return months[i].x + ratio * months[i].width;
      }
    }
    if (months.length > 0 && date < toMonthDate(months[0].label)) {
      // Return negative x for dates before range
      const firstStart = toMonthDate(months[0].label);
      const daysOff = daysBetween(date, firstStart);
      return -(daysOff * (months[0].width / months[0].days));
    }
    // After range
    const lastM = months[months.length - 1];
    const lastStart = toMonthDate(lastM.label);
    const daysOff = daysBetween(lastStart, date);
    return lastM.x + daysOff * (lastM.width / lastM.days);
  }

  function toMonthDate(label) {
    const [y, m] = label.split('.').map(Number);
    return new Date(y, m - 1, 1);
  }

  return (
    <div className="pm-wrapper" ref={containerRef}>
      {/* Slider bar + Add button */}
      <div className="pm-topbar">
        <RangeSlider viewRange={viewRange} onChange={setViewRange} />
        <div className="pm-sort">
          <span className="pm-sort-label">정렬:</span>
          {[
            { key: 'startDate', label: '시작일' },
            { key: 'endDate', label: '종료일' },
            { key: 'name', label: '과제명' },
            { key: 'dept', label: '부서명' },
          ].map((opt) => (
            <button
              key={opt.key}
              className={`pm-sort-btn ${sortKey === opt.key ? 'pm-sort-active' : ''}`}
              onClick={() => {
                if (sortKey === opt.key) {
                  setSortAsc(!sortAsc);
                } else {
                  setSortKey(opt.key);
                  setSortAsc(true);
                }
              }}
            >
              {opt.label} {sortKey === opt.key ? (sortAsc ? '\u25B2' : '\u25BC') : ''}
            </button>
          ))}
        </div>
        <button
          className="pm-add-btn"
          onClick={() => setDialog({ project: newProjectTemplate(), mode: 'edit' })}
        >
          + Add
        </button>
      </div>

      <div className="pm-container">
        <div className="pm-corner">과제</div>

        {/* Header */}
        <div className="pm-header" ref={headerRef}>
          <div className="pm-header-inner" style={{ width: totalWidth }}>
            <div className="pm-year-row">
              {years.map((yr, i) => {
                const startMonth = months[yr.startIdx];
                const yearWidth = months
                  .slice(yr.startIdx, yr.startIdx + yr.count)
                  .reduce((sum, m) => sum + m.width, 0);
                return (
                  <div
                    key={i}
                    className="pm-year-tick"
                    style={{ left: startMonth.x, width: yearWidth }}
                  >
                    {String(yr.year).slice(-2)}
                  </div>
                );
              })}
            </div>
            <div className="pm-month-row">
              {months.map((m, i) => (
                <div
                  key={i}
                  className="pm-month-tick"
                  style={{ left: m.x, width: m.width }}
                >
                  {m.month}
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Sidebar */}
        <div className="pm-sidebar" ref={sidebarRef}>
          {sortedProjects.map((p, i) => {
            const isCompleted = ['done', 'completed'].includes((p.status || '').toLowerCase());
            const hasNoIssues = (p._issueCount ?? -1) === 0;
            const rowCls = isCompleted && hasNoIssues ? 'pm-sidebar-completed-empty'
                         : isCompleted              ? 'pm-sidebar-completed'
                         : hasNoIssues              ? 'pm-sidebar-empty'
                         : '';
            return (
              <div
                key={i}
                className={`pm-sidebar-row pm-sidebar-clickable${rowCls ? ' ' + rowCls : ''}`}
                style={{ height: ROW_HEIGHT }}
                onClick={() => onProjectClick && onProjectClick(p)}
                onContextMenu={(e) => handleContextMenu(e, p)}
                title={`${p.name} 이슈 보기`}
              >
                {p.name}
              </div>
            );
          })}
        </div>

        {/* Graph */}
        <div className="pm-graph" ref={graphRef} onScroll={handleScroll}>
          <svg width={totalWidth} height={totalHeight}>
            <defs>
              <clipPath id="graphClip">
                <rect x={0} y={0} width={totalWidth} height={totalHeight} />
              </clipPath>
            </defs>
            {/* Row grid lines */}
            {sortedProjects.map((_, i) => (
              <line
                key={`grid-${i}`}
                x1={0}
                y1={(i + 1) * ROW_HEIGHT}
                x2={totalWidth}
                y2={(i + 1) * ROW_HEIGHT}
                stroke="#eee"
                strokeWidth={1}
              />
            ))}
            {/* Month/Year grid lines */}
            {months.map((m, i) => (
              <line
                key={`mgrid-${i}`}
                x1={m.x}
                y1={0}
                x2={m.x}
                y2={totalHeight}
                stroke={m.month === 1 ? '#2c3e50' : '#f0f0f0'}
                strokeWidth={m.month === 1 ? 2 : 1}
              />
            ))}
            {/* Today line */}
            {(() => {
              const todayX = dateToX(new Date());
              if (todayX >= 0 && todayX <= totalWidth) {
                return (
                  <line
                    x1={todayX} y1={0} x2={todayX} y2={totalHeight}
                    stroke="#e74c3c" strokeWidth={1.5} strokeDasharray="4 3"
                  />
                );
              }
              return null;
            })()}
            {/* Project rows (clipped) */}
            <g clipPath="url(#graphClip)">
              {sortedProjects.map((p, rowIdx) => {
                const now = new Date();
                // 18: Status === "Done" -> completed
                const isCompleted = ['done', 'completed'].includes((p.status || '').toLowerCase());
                const hasNoIssues = (p._issueCount ?? -1) === 0;
                // 19: overdue = last milestone passed but not Done
                const scheds = (p.schedules || []);
                const lastDate = scheds.length > 0
                  ? scheds.reduce((max, s) => {
                      const d = toDate(Object.values(s)[0]);
                      return d > max ? d : max;
                    }, new Date(0))
                  : null;
                const isOverdue = !isCompleted && lastDate && lastDate < now;
                return (
                  <ProjectRow
                    key={rowIdx}
                    project={p}
                    rowIdx={rowIdx}
                    dateToX={dateToX}
                    totalWidth={totalWidth}
                    rangeStart={visibleStart}
                    rangeEnd={visibleEnd}
                    isCompleted={isCompleted}
                    isOverdue={isOverdue}
                    hasNoIssues={hasNoIssues}
                    onContextMenu={handleContextMenu}
                    onProjectClick={onProjectClick}
                    alert={alerts[getJiraCode(p)]}
                  />
                );
              })}
            </g>
          </svg>
        </div>

        {/* "과제" 컬럼 ↔ 헤더/그래프 구분선 — sidebar/header 자식이 아닌 pm-container 최상위
            레이어로 두어 그래프 SVG 컨텐츠에 묻히지 않게 한다 (ProjectMap.css 주석 참조). */}
        <div className="pm-column-divider" />
      </div>

      {/* Context Menu */}
      {ctxMenu && (
        <ContextMenu
          x={ctxMenu.x}
          y={ctxMenu.y}
          project={ctxMenu.project}
          onClose={() => setCtxMenu(null)}
          onAction={handleMenuAction}
        />
      )}

      {/* Project Dialog */}
      {dialog && (
        <ProjectDialog
          project={dialog.project}
          mode={dialog.mode}
          onClose={() => setDialog(null)}
          onSave={handleDialogSave}
        />
      )}

      {/* 과제 생성/수정 진행 인디케이터 */}
      {saving && (
        <div className="pm-saving-overlay">
          <div className="pm-saving-box">
            <div className="pm-saving-spinner" />
            <div className="pm-saving-title">{saving.title}</div>
            {saving.sub && <div className="pm-saving-sub">{saving.sub}</div>}
          </div>
        </div>
      )}

      {/* 신규 과제 생성 시 launcher 미감지 → 설치/실행 안내 */}
      {launcherGate && (
        <LauncherInstallModal
          onClose={launcherGate.onClose}
          onRetry={launcherGate.onRetry}
        />
      )}

    </div>
  );
}

// --- Range Slider ---
function RangeSlider({ viewRange, onChange }) {
  const sliderRange = getSliderRange();
  const totalMonths = monthsBetween(sliderRange.min, sliderRange.max);
  const trackRef = useRef(null);
  const dragging = useRef(null); // 'start' | 'end' | null
  const shiftHeld = useRef(false);

  const startOffset = monthsBetween(sliderRange.min, viewRange.start);
  const endOffset = monthsBetween(sliderRange.min, viewRange.end);

  const startPct = (startOffset / totalMonths) * 100;
  const endPct = (endOffset / totalMonths) * 100;

  useEffect(() => {
    function onKeyDown(e) { if (e.key === 'Shift') shiftHeld.current = true; }
    function onKeyUp(e) { if (e.key === 'Shift') shiftHeld.current = false; }
    window.addEventListener('keydown', onKeyDown);
    window.addEventListener('keyup', onKeyUp);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
      window.removeEventListener('keyup', onKeyUp);
    };
  }, []);

  const handleMouseDown = useCallback((knob) => (e) => {
    e.preventDefault();
    dragging.current = knob;

    function onMouseMove(ev) {
      if (!dragging.current || !trackRef.current) return;
      const rect = trackRef.current.getBoundingClientRect();
      const pct = Math.max(0, Math.min(1, (ev.clientX - rect.left) / rect.width));
      const monthOffset = Math.round(pct * totalMonths);
      const newDate = addMonths(sliderRange.min, monthOffset);

      // Use current span (preserves Shift-expanded range)
      const spanMonths = monthsBetween(viewRange.start, viewRange.end);

      if (shiftHeld.current) {
        // Free mode: move only the dragged knob
        if (dragging.current === 'start') {
          const clamped = newDate < sliderRange.min ? sliderRange.min : newDate > viewRange.end ? viewRange.end : newDate;
          onChange({ start: clamped, end: viewRange.end });
        } else {
          const clamped = newDate > sliderRange.max ? sliderRange.max : newDate < viewRange.start ? viewRange.start : newDate;
          onChange({ start: viewRange.start, end: clamped });
        }
      } else {
        // Linked mode: maintain 3-year span
        if (dragging.current === 'start') {
          const s = newDate < sliderRange.min ? sliderRange.min : newDate;
          let e = addMonths(s, spanMonths);
          if (e > sliderRange.max) e = sliderRange.max;
          onChange({ start: s, end: e });
        } else {
          const e = newDate > sliderRange.max ? sliderRange.max : newDate;
          let s = addMonths(e, -spanMonths);
          if (s < sliderRange.min) s = sliderRange.min;
          onChange({ start: s, end: e });
        }
      }
    }

    function onMouseUp() {
      dragging.current = null;
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    }

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }, [viewRange, onChange, totalMonths, sliderRange.min, sliderRange.max]);

  return (
    <div className="pm-slider">
      <div className="pm-slider-track" ref={trackRef}>
        {/* Selected range */}
        <div
          className="pm-slider-range"
          style={{ left: `${startPct}%`, width: `${endPct - startPct}%` }}
        />
        {/* Start knob */}
        <div
          className="pm-slider-knob"
          style={{ left: `${startPct}%` }}
          onMouseDown={handleMouseDown('start')}
          title={`${viewRange.start.getFullYear()}.${String(viewRange.start.getMonth() + 1).padStart(2, '0')}`}
        />
        {/* End knob */}
        <div
          className="pm-slider-knob"
          style={{ left: `${endPct}%` }}
          onMouseDown={handleMouseDown('end')}
          title={`${viewRange.end.getFullYear()}.${String(viewRange.end.getMonth() + 1).padStart(2, '0')}`}
        />
      </div>
      <div className="pm-slider-label">
        조회기간 : {viewRange.start.getFullYear()}.{String(viewRange.start.getMonth() + 1).padStart(2, '0')}~{viewRange.end.getFullYear()}.{String(viewRange.end.getMonth() + 1).padStart(2, '0')}
      </div>
    </div>
  );
}

// --- Project Row ---
function ProjectRow({ project, rowIdx, dateToX, totalWidth, rangeStart, rangeEnd, isCompleted, isOverdue, hasNoIssues, onContextMenu, onProjectClick, alert }) {
  // 비례폰트라 length*charPx 추정이 부정확 — 실제 렌더된 텍스트 폭을 SVG getBBox 로 측정
  const textRef = useRef(null);
  const [measuredTextW, setMeasuredTextW] = useState(null);
  useLayoutEffect(() => {
    if (textRef.current) {
      try {
        const b = textRef.current.getBBox();
        if (b && b.width > 0 && b.width !== measuredTextW) setMeasuredTextW(b.width);
      } catch { /* SVG not mounted */ }
    }
  });

  const schedules = project.schedules || [];
  const allMilestones = schedules
    .map((s) => {
      const name = Object.keys(s)[0];
      const dateStr = Object.values(s)[0];
      if (!dateStr) return null;
      const date = toDate(dateStr);
      return { name, date, x: dateToX(date) };
    })
    .filter(Boolean)
    .sort((a, b) => a.date - b.date);

  if (allMilestones.length === 0) return null;

  const y = rowIdx * ROW_HEIGHT + ROW_HEIGHT / 2;

  // Check if entire project is outside range
  const firstDate = allMilestones[0].date;
  const lastDate = allMilestones[allMilestones.length - 1].date;
  const allBefore = lastDate < rangeStart;
  const allAfter = firstDate >= rangeEnd;

  // Triangle indicator for projects entirely outside range
  if (allBefore) {
    return (
      <g>
        <polygon
          points={`8,${y - 8} 8,${y + 8} 0,${y}`}
          className="pm-offscreen-indicator"
        />
        <text x={14} y={y + 4} className="pm-prj-name-off">
          {project.name}
        </text>
      </g>
    );
  }
  if (allAfter) {
    return (
      <g>
        <polygon
          points={`${totalWidth - 8},${y - 8} ${totalWidth - 8},${y + 8} ${totalWidth},${y}`}
          className="pm-offscreen-indicator"
        />
        <text x={totalWidth - 14} y={y + 4} textAnchor="end" className="pm-prj-name-off">
          {project.name}
        </text>
      </g>
    );
  }

  const firstX = allMilestones[0].x;
  const lastX = allMilestones[allMilestones.length - 1].x;

  // 가시 영역 [0, totalWidth] 안에 들어오는 마일스톤만 line/이름 앵커로 사용
  // → 음수 firstX 에서 끌려오는 "허공의 라인" 제거.
  const visibleMilestones = allMilestones.filter((m) => m.x >= 0 && m.x <= totalWidth);
  const firstVisibleX = visibleMilestones.length > 0 ? visibleMilestones[0].x : firstX;
  const lastVisibleX = visibleMilestones.length > 0 ? visibleMilestones[visibleMilestones.length - 1].x : lastX;

  // Left overflow indicator
  const hasLeftOverflow = firstDate < rangeStart;
  // Right overflow indicator
  const hasRightOverflow = lastDate >= rangeEnd;

  const cls = isCompleted && hasNoIssues ? 'completed-empty'
            : isCompleted              ? 'completed'
            : hasNoIssues              ? 'empty'
            : isOverdue                ? 'overdue'
            : 'active';
  const hasAlert = alert && alert.branchDiff;
  const hasPR = alert && alert.hasOpenPR;
  // 알람: "확인할 변경 있음" 의 주의(caution) 톤 — 위험(red) 아닌 앰버 계열
  const alertColor = hasPR ? '#27ae60' : hasAlert ? '#e67e22' : null;

  return (
    <g
      className={`pm-row-${cls}`}
      style={{ cursor: 'context-menu' }}
      onContextMenu={(e) => onContextMenu && onContextMenu(e, project)}
    >
      {/* Invisible hit area for right-click */}
      <rect
        x={Math.min(firstX - 80, 0)}
        y={y - ROW_HEIGHT / 2}
        width={Math.max(lastX - firstX + 160, totalWidth)}
        height={ROW_HEIGHT}
        fill="transparent"
      />

      {/* Connecting line — 가시 마일스톤 사이만 */}
      {visibleMilestones.length >= 1 && (
        <line
          x1={firstVisibleX}
          y1={y}
          x2={lastVisibleX}
          y2={y}
          className="pm-line"
          style={alertColor ? { stroke: alertColor, strokeWidth: 3 } : undefined}
        />
      )}

      {/* Left overflow triangle */}
      {hasLeftOverflow && (
        <polygon
          points={`8,${y - 7} 8,${y + 7} 0,${y}`}
          className="pm-offscreen-indicator"
        />
      )}

      {/* Right overflow triangle */}
      {hasRightOverflow && (
        <polygon
          points={`${totalWidth - 8},${y - 7} ${totalWidth - 8},${y + 7} ${totalWidth},${y}`}
          className="pm-offscreen-indicator"
        />
      )}

      {/* Milestone circles */}
      {allMilestones.map((m, i) => {
        // 3.1: days since previous milestone (0 for first)
        const daysDiff = i === 0 ? 0 : daysBetween(allMilestones[i - 1].date, m.date);
        return (
        <g key={i}>
          <circle cx={m.x} cy={y} r={MILESTONE_R} className="pm-milestone" style={alertColor ? { stroke: alertColor } : undefined} />
          <text x={m.x} y={y + 4} textAnchor="middle" className="pm-day" style={alertColor ? { fill: alertColor } : undefined}>
            {daysDiff}
          </text>
          <text
            x={m.x}
            y={y - MILESTONE_R - 4}
            textAnchor="middle"
            className="pm-ms-label"
            style={alertColor ? { fill: alertColor } : undefined}
          >
            {m.name}
          </text>
        </g>
        );
      })}

      {/* Project name (rendered last = on top) with white background + alert icon */}
      {visibleMilestones.length > 0 && (() => {
        // 1차 렌더는 추정값으로 위치 잡고, useLayoutEffect 로 측정한 후 2차 렌더에서 정확한 폭 사용
        const fallbackTextW = (project.name || '').length * 7;
        const textW = measuredTextW != null ? measuredTextW : fallbackTextW;
        const iconWidth = 16;
        const iconGap = 5; // 아이콘 우측 끝과 과제명 첫 글자 사이
        const iconLeftPad = 2; // 흰 rect 좌측 ↔ 아이콘 좌측 여백
        const rectRightPad = 2; // 텍스트 우측 ↔ 흰 rect 우측 여백
        const hasIcon = !!alertColor;
        const blockW = (hasIcon ? iconLeftPad + iconWidth + iconGap : 0) + textW + rectRightPad;
        // 마일스톤 왼쪽에 이름을 놓는다. 공간이 부족한 경우(예: 첫 마일스톤 날짜가 조회 시작
        // 이전이라 그래프 좌측 여백이 거의 없을 때) 예전에는 Math.max 로 강제로 밀어넣어
        // 마일스톤 원과 겹쳤다. 이제는 밀어넣지 않고 blockLeft 가 음수로 나가게 둔다 —
        // <g clipPath="url(#graphClip)"> (x>=0 만 표시) 가 자동으로 잘라내므로, 공간이
        // 없으면 이름이 "과제" 사이드바 뒤로 가려진 것처럼 자연스럽게 안 보이게 된다.
        const nameX = firstVisibleX - MILESTONE_R - 10;
        const blockLeft = nameX - (blockW - rectRightPad);
        const iconX = blockLeft + iconLeftPad;
        return (
          <>
            <rect
              x={blockLeft}
              y={y - 9}
              width={blockW}
              height={18}
              fill="#fff"
              rx={2}
              style={{ cursor: 'pointer' }}
              onClick={() => onProjectClick && onProjectClick(project)}
            />
            {hasIcon && (
              <>
                <rect
                  x={iconX - 3}
                  y={y - 9 - 3}
                  width={iconWidth + 6}
                  height={iconWidth + 6}
                  fill="#fff"
                  rx={2}
                />
                <image
                  href={hasPR ? '/icons/alert-pr.svg' : '/icons/alert-diff.svg'}
                  x={iconX}
                  y={y - 9}
                  width="16" height="16"
                />
              </>
            )}
            <text
              ref={textRef}
              x={nameX}
              y={y + 4}
              textAnchor="end"
              className="pm-prj-name"
              style={{ cursor: 'pointer', ...(alertColor ? { fill: alertColor } : {}) }}
              onClick={() => onProjectClick && onProjectClick(project)}
            >
              {project.name}
            </text>
          </>
        );
      })()}
    </g>
  );
}
