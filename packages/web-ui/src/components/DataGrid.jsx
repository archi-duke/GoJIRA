// 플랫폼 표준 데이터 그리드 — KendoReact Grid 래퍼 (계약 04 §1: Kendo 는 web-ui 만 소유,
// 앱은 이 래퍼만 소비한다). 소팅/필터는 kendo-data-query process() 로 클라이언트 처리.
//
// props:
//   data:        원본 배열 (필터/소트 전)
//   columns:     [{ field, title, width?, cell?, filterable?, sortable? }] — GridColumn 에 그대로 전달
//   initialSort: [{ field, dir }] (선택)
//   pageSize:    지정 시 페이징 활성 (선택, 기본 비활성)
// 나머지 props 는 Grid 로 전달.
import React, { useState } from 'react';
import { Grid, GridColumn } from '@progress/kendo-react-grid';
import { process } from '@progress/kendo-data-query';
import '@progress/kendo-theme-bootstrap/dist/all.css';

export default function DataGrid({ data, columns, initialSort = [], pageSize, children, ...rest }) {
  const [dataState, setDataState] = useState({
    sort: initialSort,
    filter: undefined,
    skip: 0,
    take: pageSize,
  });

  const result = process(data || [], dataState);

  return (
    <Grid
      data={result}
      sortable
      filterable
      pageable={!!pageSize}
      {...dataState}
      onDataStateChange={(e) => setDataState(e.dataState)}
      {...rest}
    >
      {columns.map((c) => (
        <GridColumn key={c.field || c.title} {...c} />
      ))}
      {children}
    </Grid>
  );
}
