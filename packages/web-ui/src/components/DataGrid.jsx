// 플랫폼 표준 데이터 그리드 — 자체 구현 (외부 그리드 라이브러리 의존 없음).
// 예전 KendoReact Grid 래퍼와 같은 props 를 받으므로 소비 앱은 코드를 바꾸지 않아도 된다.
//
// 기본(클라이언트) 모드 — data 전체를 받아 그리드가 정렬·필터·페이징을 직접 처리한다.
//   data:        원본 배열 (필터/정렬 전)
//   columns:     [{ field, title, width?, cell?, filterable?, sortable?, resizable? }]
//                cell: (props) => <td>…</td> — props 는 { dataItem, field, rowIndex, columnIndex }.
//                예전 Kendo 커스텀 셀과 같은 모양이라 그대로 쓸 수 있다.
//   initialSort: [{ field, dir: 'asc' | 'desc' }] (선택)
//   pageSize:    지정하면 페이징을 켠다 (선택, 기본은 전체 표시)
//   pageSizes:   페이지 크기 선택 목록 (선택, 예 [10, 20, 50])
//   resizable:   컬럼 경계를 끌어 폭을 조절 (선택, 기본 false)
//   filterable / sortable: 그리드 전체 기본값 (기본 true). 컬럼별 값이 우선한다.
//
// 서버(제어) 모드 — total 을 주면 data 를 "이미 처리된 현재 페이지" 로 보고 그대로 그린다.
//   sort, filter, skip, take, total 을 부모가 쥐고, 바뀔 때마다 아래 콜백으로 알린다.
//   onSortChange({ sort }) · onFilterChange({ filter }) · onPageChange({ page: { skip, take } })
//   onDataStateChange({ dataState: { sort, filter, skip, take } }) — 위 셋을 한 번에 받고 싶을 때
//   filter 형식: { logic: 'and', filters: [{ field, operator: 'contains', value }] }
//
// 기타: style, className, height, noRecords(빈 목록 문구) 를 받는다.
import React, { useMemo, useState } from 'react';
import './DataGrid.css';

const EMPTY = [];

function toText(v) {
  if (v === null || v === undefined) return '';
  return String(v);
}

function getField(item, field) {
  if (!field) return undefined;
  if (!field.includes('.')) return item?.[field];
  return field.split('.').reduce((o, k) => (o == null ? undefined : o[k]), item);
}

function compareValues(a, b) {
  const aNil = a === null || a === undefined || a === '';
  const bNil = b === null || b === undefined || b === '';
  if (aNil && bNil) return 0;
  if (aNil) return 1; // 빈 값은 방향과 무관하게 뒤로
  if (bNil) return -1;
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  if (a instanceof Date && b instanceof Date) return a - b;
  return toText(a).localeCompare(toText(b), 'ko', { numeric: true, sensitivity: 'base' });
}

// filter 기술자 → { field: value } (contains 만 지원)
function filterToMap(filter) {
  const map = {};
  (filter?.filters || []).forEach((f) => {
    if (f && f.field) map[f.field] = toText(f.value);
  });
  return map;
}

function mapToFilter(map) {
  const filters = Object.entries(map)
    .filter(([, v]) => v !== '')
    .map(([field, value]) => ({ field, operator: 'contains', value, ignoreCase: true }));
  return filters.length ? { logic: 'and', filters } : undefined;
}

function applyFilter(rows, map) {
  const entries = Object.entries(map).filter(([, v]) => v !== '');
  if (!entries.length) return rows;
  const needles = entries.map(([f, v]) => [f, v.toLowerCase()]);
  return rows.filter((r) => needles.every(([f, v]) => toText(getField(r, f)).toLowerCase().includes(v)));
}

function applySort(rows, sort) {
  const s = (sort || []).filter((x) => x && x.field && x.dir);
  if (!s.length) return rows;
  return rows
    .map((r, i) => [r, i])
    .sort(([a, ia], [b, ib]) => {
      for (const { field, dir } of s) {
        const av = getField(a, field);
        const bv = getField(b, field);
        const aNil = av === null || av === undefined || av === '';
        const bNil = bv === null || bv === undefined || bv === '';
        if (aNil !== bNil) return aNil ? 1 : -1; // 빈 값은 항상 뒤로
        const c = compareValues(av, bv);
        if (c !== 0) return dir === 'desc' ? -c : c;
      }
      return ia - ib; // 같은 값은 원래 순서 유지
    })
    .map(([r]) => r);
}

function pageNumbers(current, count, buttonCount) {
  let start = Math.max(1, current - Math.floor(buttonCount / 2));
  const end = Math.min(count, start + buttonCount - 1);
  start = Math.max(1, end - buttonCount + 1);
  const out = [];
  for (let i = start; i <= end; i++) out.push(i);
  return out;
}

export default function DataGrid({
  data,
  columns = EMPTY,
  initialSort = EMPTY,
  pageSize,
  pageSizes,
  resizable = false,
  sortable = true,
  filterable = true,
  // 서버(제어) 모드
  total,
  sort: sortProp,
  filter: filterProp,
  skip: skipProp,
  take: takeProp,
  onSortChange,
  onFilterChange,
  onPageChange,
  onDataStateChange,
  // 표시
  style,
  className,
  height,
  noRecords = '표시할 데이터가 없습니다.',
}) {
  const controlled = total !== undefined;

  const [sortState, setSortState] = useState(initialSort);
  const [filterState, setFilterState] = useState({});
  const [skipState, setSkipState] = useState(0);
  const [takeState, setTakeState] = useState(pageSize);
  const [widths, setWidths] = useState({});

  const sort = controlled && sortProp !== undefined ? sortProp : sortState;
  const filterMap = controlled && filterProp !== undefined ? filterToMap(filterProp) : filterState;
  const skip = controlled && skipProp !== undefined ? skipProp : skipState;
  const take = controlled && takeProp !== undefined ? takeProp : takeState;
  const pageable = !!take;

  const rows = data || EMPTY;
  const filtered = useMemo(
    () => (controlled ? rows : applySort(applyFilter(rows, filterMap), sort)),
    [controlled, rows, filterMap, sort],
  );
  const totalCount = controlled ? total : filtered.length;
  const visible = controlled || !pageable ? filtered : filtered.slice(skip, skip + take);

  function emit(next) {
    const state = { sort, filter: mapToFilter(filterMap), skip, take, ...next };
    if (onDataStateChange) onDataStateChange({ dataState: state });
  }

  function changeSort(field) {
    const cur = sort.find((s) => s.field === field);
    const dir = !cur ? 'asc' : cur.dir === 'asc' ? 'desc' : null;
    const next = dir ? [{ field, dir }] : [];
    if (!controlled) {
      setSortState(next);
      setSkipState(0);
    }
    if (onSortChange) onSortChange({ sort: next });
    emit({ sort: next, skip: 0 });
  }

  function changeFilter(field, value) {
    const nextMap = { ...filterMap, [field]: value };
    const next = mapToFilter(nextMap);
    if (!controlled) {
      setFilterState(nextMap);
      setSkipState(0);
    }
    if (onFilterChange) onFilterChange({ filter: next });
    emit({ filter: next, skip: 0 });
  }

  function changePage(nextSkip, nextTake = take) {
    const pageCount = Math.max(1, Math.ceil(totalCount / nextTake));
    const clamped = Math.min(Math.max(0, nextSkip), (pageCount - 1) * nextTake);
    if (!controlled) {
      setSkipState(clamped);
      setTakeState(nextTake);
    }
    if (onPageChange) onPageChange({ page: { skip: clamped, take: nextTake } });
    emit({ skip: clamped, take: nextTake });
  }

  // 컬럼 폭 조절 — 헤더 오른쪽 경계를 끌면 그 컬럼 폭만 바뀐다.
  function startResize(e, key) {
    e.preventDefault();
    e.stopPropagation();
    const th = e.currentTarget.parentElement;
    const startX = e.clientX;
    const startW = th.getBoundingClientRect().width;
    function move(ev) {
      const w = Math.max(48, Math.round(startW + ev.clientX - startX));
      setWidths((prev) => ({ ...prev, [key]: `${w}px` }));
    }
    function up() {
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
      document.body.classList.remove('pf-grid-resizing');
    }
    document.body.classList.add('pf-grid-resizing');
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  }

  const colKey = (c, i) => c.field || c.title || `col-${i}`;
  const anyFilter = filterable && columns.some((c) => c.filterable !== false && c.field);

  const page = pageable ? Math.floor(skip / take) + 1 : 1;
  const pageCount = pageable ? Math.max(1, Math.ceil(totalCount / take)) : 1;
  const from = totalCount === 0 ? 0 : skip + 1;
  const to = pageable ? Math.min(skip + take, totalCount) : totalCount;

  const wrapStyle = height ? { height, ...style } : style;

  return (
    <div className={`pf-grid${className ? ` ${className}` : ''}`} style={wrapStyle}>
      <div className="pf-grid-scroll">
        <table className="pf-grid-table">
          <colgroup>
            {columns.map((c, i) => {
              const k = colKey(c, i);
              const w = widths[k] || c.width;
              return <col key={k} style={w ? { width: w } : undefined} />;
            })}
          </colgroup>
          <thead>
            <tr>
              {columns.map((c, i) => {
                const k = colKey(c, i);
                const canSort = sortable && c.sortable !== false && !!c.field;
                const s = canSort ? sort.find((x) => x.field === c.field) : null;
                const canResize = resizable && c.resizable !== false;
                return (
                  <th
                    key={k}
                    className={canSort ? 'pf-grid-sortable' : undefined}
                    onClick={canSort ? () => changeSort(c.field) : undefined}
                    aria-sort={s ? (s.dir === 'asc' ? 'ascending' : 'descending') : undefined}
                    title={canSort ? '클릭해 정렬' : undefined}
                  >
                    <span className="pf-grid-title">{c.title ?? c.field}</span>
                    {s && <span className="pf-grid-sort">{s.dir === 'asc' ? '▲' : '▼'}</span>}
                    {canResize && (
                      <span
                        className="pf-grid-resizer"
                        onMouseDown={(e) => startResize(e, k)}
                        onClick={(e) => e.stopPropagation()}
                      />
                    )}
                  </th>
                );
              })}
            </tr>
            {anyFilter && (
              <tr className="pf-grid-filter-row">
                {columns.map((c, i) => {
                  const k = colKey(c, i);
                  const can = c.filterable !== false && !!c.field;
                  return (
                    <th key={k}>
                      {can && (
                        <input
                          type="text"
                          className="pf-grid-filter"
                          value={filterMap[c.field] || ''}
                          placeholder="포함 검색"
                          aria-label={`${c.title ?? c.field} 필터`}
                          onChange={(e) => changeFilter(c.field, e.target.value)}
                        />
                      )}
                    </th>
                  );
                })}
              </tr>
            )}
          </thead>
          <tbody>
            {visible.length === 0 ? (
              <tr className="pf-grid-empty">
                <td colSpan={columns.length || 1}>{noRecords}</td>
              </tr>
            ) : (
              visible.map((item, r) => (
                <tr key={item?._id ?? item?.id ?? skip + r}>
                  {columns.map((c, i) => {
                    const k = colKey(c, i);
                    if (c.cell) {
                      const Cell = c.cell;
                      return (
                        <Cell key={k} dataItem={item} field={c.field} rowIndex={skip + r} columnIndex={i} />
                      );
                    }
                    return <td key={k}>{toText(getField(item, c.field))}</td>;
                  })}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      {pageable && (
        <div className="pf-grid-pager">
          <div className="pf-grid-pager-nav">
            <button type="button" onClick={() => changePage(0)} disabled={page <= 1} aria-label="첫 페이지">«</button>
            <button type="button" onClick={() => changePage(skip - take)} disabled={page <= 1} aria-label="이전 페이지">‹</button>
            {pageNumbers(page, pageCount, 5).map((n) => (
              <button
                type="button"
                key={n}
                className={n === page ? 'pf-grid-page-current' : undefined}
                aria-current={n === page ? 'page' : undefined}
                onClick={() => changePage((n - 1) * take)}
              >
                {n}
              </button>
            ))}
            <button type="button" onClick={() => changePage(skip + take)} disabled={page >= pageCount} aria-label="다음 페이지">›</button>
            <button type="button" onClick={() => changePage((pageCount - 1) * take)} disabled={page >= pageCount} aria-label="마지막 페이지">»</button>
            {pageSizes && pageSizes.length > 0 && (
              <select
                className="pf-grid-pagesize"
                value={take}
                aria-label="페이지당 행 수"
                onChange={(e) => changePage(0, Number(e.target.value))}
              >
                {pageSizes.map((n) => (
                  <option key={n} value={n}>{n}개씩</option>
                ))}
              </select>
            )}
          </div>
          <span className="pf-grid-pager-info">{from} - {to} / 전체 {totalCount}건</span>
        </div>
      )}
    </div>
  );
}
