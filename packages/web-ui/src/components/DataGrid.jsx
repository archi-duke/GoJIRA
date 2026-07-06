// 플랫폼 표준 데이터 그리드 — KendoReact Grid 래퍼 (계약 04 §1: Kendo 는 web-ui 만 소유,
// 앱은 이 래퍼만 소비한다). 소팅/필터는 kendo-data-query process() 로 클라이언트 처리.
//
// props:
//   data:        원본 배열 (필터/소트 전)
//   columns:     [{ field, title, width?, cell?, filterable?, sortable? }] — GridColumn 에 그대로 전달
//   initialSort: [{ field, dir }] (선택)
//   pageSize:    지정 시 페이징 활성 — 페이지 크기 선택(10/20/50/100) 포함 (선택, 기본 비활성)
//   filterable:  필터 행 표시 여부 (기본 true). false 로 바뀌면 걸려있던 필터도 해제 —
//                숨은 필터가 몰래 적용된 채 남는 혼동 방지 (토글 UI 용)
// 나머지 props 는 Grid 로 전달. 필터 변경 시 1페이지로 복귀.
import React, { useState, useEffect } from 'react';
import { Grid, GridColumn } from '@progress/kendo-react-grid';
import { process } from '@progress/kendo-data-query';
import '@progress/kendo-theme-bootstrap/dist/all.css';

export default function DataGrid({ data, columns, initialSort = [], pageSize, filterable = true, children, ...rest }) {
  const [dataState, setDataState] = useState({
    sort: initialSort,
    filter: undefined,
    skip: 0,
    take: pageSize,
  });

  useEffect(() => {
    if (!filterable) setDataState((s) => (s.filter ? { ...s, filter: undefined } : s));
  }, [filterable]);

  const result = process(data || [], dataState);

  return (
    <Grid
      data={result}
      sortable
      filterable={filterable}
      pageable={pageSize ? { pageSizes: [10, 20, 50, 100], buttonCount: 5, info: true } : false}
      {...dataState}
      onDataStateChange={(e) =>
        setDataState(
          e.dataState.filter !== dataState.filter
            ? { ...e.dataState, skip: 0 } // 필터가 바뀌면 1페이지로
            : e.dataState
        )
      }
      {...rest}
    >
      {columns.map((c) => (
        <GridColumn key={c.field || c.title} {...c} />
      ))}
      {children}
    </Grid>
  );
}
