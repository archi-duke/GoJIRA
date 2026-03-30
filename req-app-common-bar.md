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
  onPull: () => void,      // Pull 버튼 클릭 시 실행 (컴포넌트별 fetch 로직)
  onEditChange: (editMode: boolean) => void,  // 편집모드 변경 콜백
  options: [               // 컴포넌트별 커스텀 버튼 목록
    { label: string, handler: () => void }
  ]
}
```

### 3. 동작
- Pull 버튼 : onPull 호출 → JIRA에서 이슈 재로딩
- 편집모드 버튼 : 내부 editMode state 토글 → onEditChange(newMode) 호출
- 옵션 버튼 : 각 handler 호출 (컴포넌트가 자유롭게 정의)

### 4. 스타일
- 기본 버튼 : lightgray 배경, border 없음, 검은색 글자, font-weight normal
- 편집모드 활성 : tomato 배경, 흰색 글자
- Back 버튼 : 1px solid #ccc border, 흰색 배경

### 5. 적용 대상
- RelationMap, IssueList 등 프로젝트 하위 뷰 컴포넌트에 공통 적용
