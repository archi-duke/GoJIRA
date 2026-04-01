## CommonBar — 공통 제어 UI 컴포넌트

### 1. Layout
```
[← Back] [Title]          [옵션1] [옵션2] ...  [Pull] [편집모드]
```
- `[← Back]` : 이전 화면으로 돌아가기 (onBack prop)
- `[Title]`  : 프로젝트명 + 현재 뷰 이름 (title prop)
- `[옵션*]`  : 각 컴포넌트가 제공하는 커스텀 버튼들 (options prop)
- `[Pull]`   : JIRA에서 이슈를 다시 읽어오기 (onPull prop)
- `[편집모드]` : 편집모드 토글 (CommonBar 내부 state, onEditChange prop으로 변경 전파)

### 2. Props
```js
{
  title: string,           // 상단 타이틀 텍스트
  onBack: () => void,      // ← 버튼 클릭 핸들러
  onPull: () => Promise,   // Pull 버튼 클릭 시 실행 (async, 완료까지 인디케이터 표시)
  onEditChange: (editMode: boolean) => void,  // 편집모드 변경 콜백
  options: [               // 컴포넌트별 커스텀 버튼 목록
    { label: string, active: boolean, handler: () => void }
  ],
  nav: [                   // 네비게이션 탭 목록
    { key: string, label: string, active: boolean, handler: () => void }
  ]
}
```

### 3. 동작
- Pull 버튼 : onPull 호출 → JIRA에서 이슈 재로딩
  - Pull 중에는 버튼이 "Pulling..."으로 변경되고 disabled 처리
  - 전체 화면 반투명 오버레이 + 스피너 인디케이터 표시 (z-index: 9999)
  - Pull 완료 시 자동 해제, 중복 클릭 방지
- 편집모드 버튼 : 내부 editMode state 토글 → onEditChange(newMode) 호출
- 옵션 버튼 : 각 handler 호출 (컴포넌트가 자유롭게 정의)
- 네비게이션 탭 : Issues / Relation Map / SATAM 간 전환 (nav prop으로 전달)

### 4. 스타일
- 기본 버튼 : lightgray 배경, border 없음, 검은색 글자, font-weight normal
- 편집모드 활성 : tomato 배경, 흰색 글자
- Back 버튼 : 1px solid #ccc border, 흰색 배경

### 5. 적용 대상
- IssueList, RelationMap, SATAM 등 프로젝트 하위 뷰 컴포넌트에 공통 적용
