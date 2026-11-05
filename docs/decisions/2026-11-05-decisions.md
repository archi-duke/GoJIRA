# 2026-11-05 의사결정 로그

## D1. 로컬 런처를 "단일 진입점 + 로컬 서비스 플랫폼" 으로 재정의

- **문제**: RAGaaS 의 사내 DRM(NASCA) pptx/docx 를 사용자 PC 에서 Office COM 으로 텍스트 변환해야 함
  (서버에 보안 도구 설치 불가, 브라우저는 COM 접근 불가 — Pyodide 등 브라우저 내 Python 도 샌드박스라 불가).
  런처에 직접 넣으면 이미 5개 책임(소환·Git 작업본·로컬 파일 CRUD·이슈·데몬 운영)이 섞인 런처에
  제품 무관 기능까지 더해짐.
- **결정**:
  - 진입점은 `http://127.0.0.1:5599` 단일 유지 (HTTP — 로컬 인증서 배포 안 함, 포트 현행 유지).
  - 런처 책임 = 배포(카탈로그·설치·업데이트) + 소환(서비스 기동·감시) + 라우팅.
  - 기능은 독립 로컬 서비스가 제공, 웹은 **기능 경로**로 호출 (예: `POST /docs/pptx/text`).
  - 서비스 목록은 Platform-API 가 서빙하는 서명 카탈로그. 새 서비스 추가 시 런처 무수정.
  - 기존 런처 기능은 `gojira-workspace` 서비스로 이관, 기존 경로는 이행 기간 호환 별칭.
- **설계서**: [`../design-launcher-gateway.md`](../design-launcher-gateway.md)
- **첫 적용**: `ragaas-converter` (RAGaaS 레포). 선행 과제 — DRM 파일 임시 경로 사본의 복호화 수동 검증.

## D2. 런처 게이트웨이 세부 확정 (D1 후속)

- 서비스 경로는 `/v1` 버전 접두사 필수 (`/v1/docs/pptx/text`), `/launcher/*` 는 무버전.
- 서비스 공개 원칙 — 카탈로그의 모든 서비스는 모든 사용자에게 노출·설치 가능 (제품별 대상 제한 없음).
- 서비스 간 호출 허용 — 런처 경유, 호출자 비밀값 인증 + `permissions.call` 선언으로 대상 제한.
- converter 는 Windows 전용, macOS/Linux 는 `unsupported` 표시.
- 미결: 카탈로그 서명 키 보관·서명 주체.

## D3. 카탈로그 서명은 파이프라인 자동 서명

- 배포 빈도가 높아 담당자 수동 서명은 병목 → main 머지 시 `build-catalog.sh` 가 검증 후 ed25519 자동 서명.
- 통제 지점은 카탈로그 PR 리뷰 + 보호 브랜치. 키는 파이프라인 시크릿, main 서명 단계에서만 주입.
- 보완: `issuedAt` 역행 거부, `expiresAt`(30일) 주기 재서명, 런처 공개키 2슬롯(교체·유출 대응), 서명 감사 기록.
- 미결: 파이프라인 실행 위치(GitHub Actions vs 외부망 빌드 호스트 스크립트).

## D4. 서명 파이프라인은 사내 GitHub Actions

- 레포는 사내 GitHub 에서 운영 예정, Actions 사용 가능 확인.
- `catalog-validate`(PR, 키 없음) + `catalog-sign`(main push·주간 스케줄, `environment: catalog-signing`).
- 키는 environment secret(`CATALOG_SIGNING_KEY`, main 브랜치 한정), 서명 전용 self-hosted runner(ephemeral 권장).
- CODEOWNERS 로 카탈로그·서명 워크플로·스크립트 변경을 플랫폼 담당 승인 대상으로 지정.
- 사내망 안에서 빌드→서명→게시 완결 — 서명된 카탈로그의 번들 반입 불필요.
- 미결: 패키지 저장소(Releases vs launcher-dist 직접), 게시 경로 구현, 서명 직전 2차 승인 여부.

## D5. 이슈 #116 반영 — 런처 매니페스트 서명·키 분리·빌드 이전

- 런처 자기 업데이트도 서명 대상(런처는 공개키를 품은 신뢰 루트). 키 2종: 루트 키(런처 매니페스트, `launcher-release` environment 승인자 필수) / 카탈로그 키(자동).
- 서명 대상 바이너리는 Actions 에서 빌드·서명, 배포 서버 `launcher-dist` 재빌드 제거.
- 검증은 경고 모드(A) → 필수 모드(B) 2단계, `/api/v2/launcher/version` 구형식 유지(sig 필드만 추가).
- 서명 runner 는 runner group 으로 레포·워크플로 제한 + ephemeral. GHES runner 0개 → 구축이 선행(§14 0단계).
- 미결 추가: Authenticode 코드서명, A→B 전환 기준.
