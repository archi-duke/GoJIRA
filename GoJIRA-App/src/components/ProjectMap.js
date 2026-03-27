import React, { useEffect, useRef, useState, useCallback } from 'react';
import './ProjectMap.css';
import ContextMenu from './ContextMenu';
import ProjectDialog from './ProjectDialog';

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

export default function ProjectMap() {
  const [projects, setProjects] = useState([]);
  const [graphWidth, setGraphWidth] = useState(0);
  const [viewRange, setViewRange] = useState(getDefaultView);
  const [ctxMenu, setCtxMenu] = useState(null); // { x, y, project }
  const [dialog, setDialog] = useState(null);   // { project, mode: 'detail'|'edit' }
  const graphRef = useRef(null);
  const headerRef = useRef(null);
  const sidebarRef = useRef(null);
  const containerRef = useRef(null);

  useEffect(() => {
    fetch('http://localhost:9000/api/v2/Project', {
      headers: { XBody: JSON.stringify({}) },
    })
      .then((res) => res.json())
      .then((data) => {
        if (Array.isArray(data)) setProjects(data);
      })
      .catch(() => setProjects([]));
  }, []);

  function handleContextMenu(e, project) {
    e.preventDefault();
    setCtxMenu({ x: e.clientX, y: e.clientY, project });
  }

  function handleMenuAction(action, project) {
    switch (action) {
      case 'detail':
        setDialog({ project, mode: 'detail' });
        break;
      case 'edit':
        setDialog({ project, mode: 'edit' });
        break;
      case 'delete':
        if (window.confirm(`"${project.Name}" 과제를 삭제하시겠습니까?`)) {
          fetch('http://localhost:9000/api/v2/Project', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ...project, State: 'Deleted' }),
          }).then(() => {
            setProjects((prev) => prev.filter((p) => p.JiraCode !== project.JiraCode));
          });
        }
        break;
      case 'relationMap':
      case 'satam':
        alert(`${action} 기능은 준비 중입니다.`);
        break;
      default:
        break;
    }
  }

  function handleDialogSave(updated) {
    // New project (POST) vs existing (PUT)
    const isNew = !projects.some((p) => p.JiraCode === updated.JiraCode);
    fetch('http://localhost:9000/api/v2/Project', {
      method: isNew ? 'POST' : 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(updated),
    })
      .then((res) => res.json())
      .then((data) => {
        if (Array.isArray(data)) setProjects(data);
      });
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
    return {
      Name: '',
      JiraCode: '',
      State: 'In Progress',
      DevTeam: '',
      DevGroup: '',
      Feature: '',
      Members: [],
      Architect: { Name: '', KnoxID: '', Department: '' },
      PL: { Name: '', KnoxID: '', Department: '' },
      Schedules: [
        { PA: '' }, { EA: '' }, { ER1: '' }, { ER2: '' }, { CA: '' },
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

  const allDates = [];
  projects.forEach((p) => {
    (p.Schedules || []).forEach((s) => {
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

  // Build month list for current view range
  const monthList = [];
  const cur = new Date(rangeStart.getFullYear(), rangeStart.getMonth(), 1);
  while (cur < rangeEnd) {
    const monthStart = new Date(cur);
    const monthEnd = new Date(cur.getFullYear(), cur.getMonth() + 1, 1);
    const days = daysBetween(monthStart, monthEnd);
    monthList.push({ year: cur.getFullYear(), month: cur.getMonth() + 1, days });
    cur.setMonth(cur.getMonth() + 1);
  }

  const monthCount = monthList.length;
  const evenWidth = graphWidth / monthCount;
  const months = monthList.map((m) => ({
    ...m,
    label: `${m.year}.${String(m.month).padStart(2, '0')}`,
    width: Math.max(MIN_MONTH_WIDTH, evenWidth),
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

  // Calculate left padding: ensure the earliest project name is not clipped
  // Estimate max project name width (~7px per char) + margin
  let leftPadding = 0;
  projects.forEach((p) => {
    const nameWidth = (p.Name || '').length * 7 + 16;
    if (nameWidth > leftPadding) leftPadding = nameWidth;
  });

  // Assign x positions with left padding
  let accX = leftPadding;
  for (const m of months) {
    m.x = accX;
    accX += m.width;
  }
  const totalWidth = accX;
  const totalHeight = projects.length * ROW_HEIGHT;

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
          {projects.map((p, i) => (
            <div key={i} className="pm-sidebar-row" style={{ height: ROW_HEIGHT }}>
              {p.Name}
            </div>
          ))}
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
            {projects.map((_, i) => (
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
              {projects.map((p, rowIdx) => {
                const now = new Date();
                // 18: Status === "Done" -> completed
                const isCompleted = (p.State || '').toLowerCase() === 'done';
                // 19: overdue = last milestone passed but not Done
                const scheds = (p.Schedules || []);
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
                    rangeStart={rangeStart}
                    rangeEnd={rangeEnd}
                    isCompleted={isCompleted}
                    isOverdue={isOverdue}
                    onContextMenu={handleContextMenu}
                  />
                );
              })}
            </g>
          </svg>
        </div>
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

      const spanMonths = DEFAULT_SPAN_YEARS * 12;

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
function ProjectRow({ project, rowIdx, dateToX, totalWidth, rangeStart, rangeEnd, isCompleted, isOverdue, onContextMenu }) {
  const schedules = project.Schedules || [];
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
          {project.Name}
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
          {project.Name}
        </text>
      </g>
    );
  }

  // Visible milestones (clipped by SVG clipPath, but we render all)
  const firstX = allMilestones[0].x;
  const lastX = allMilestones[allMilestones.length - 1].x;

  // Left overflow indicator
  const hasLeftOverflow = firstDate < rangeStart;
  // Right overflow indicator
  const hasRightOverflow = lastDate >= rangeEnd;

  const cls = isCompleted ? 'completed' : isOverdue ? 'overdue' : 'active';

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

      {/* Connecting line */}
      <line x1={firstX} y1={y} x2={lastX} y2={y} className="pm-line" />

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
      {allMilestones.map((m, i) => (
        <g key={i}>
          <circle cx={m.x} cy={y} r={MILESTONE_R} className="pm-milestone" />
          <text x={m.x} y={y + 4} textAnchor="middle" className="pm-day">
            {dayOfMonth(m.date)}
          </text>
          <text
            x={m.x}
            y={y - MILESTONE_R - 4}
            textAnchor="middle"
            className="pm-ms-label"
          >
            {m.name}
          </text>
        </g>
      ))}

      {/* Project name (rendered last = on top) with white background */}
      {(() => {
        const nameX = firstX - MILESTONE_R - 10;
        const nameW = (project.Name || '').length * 8 + 8;
        return (
          <>
            <rect
              x={nameX - nameW}
              y={y - 9}
              width={nameW}
              height={18}
              fill="#fff"
              rx={2}
            />
            <text
              x={nameX}
              y={y + 4}
              textAnchor="end"
              className="pm-prj-name"
            >
              {project.Name}
            </text>
          </>
        );
      })()}
    </g>
  );
}
