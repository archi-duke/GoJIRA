# vendor/pumlex — GoJIRA 폐쇄망 배포용 pumlex 서브셋

원본: https://github.com/archi-duke/pumlex (`/home/duke/works/pumlex`)

폐쇄망 번들이 외부 repo 에 의존하지 않도록 **pex-server 구동에 필요한 최소 파일만** 복사해 둔 것이다.
빌드 컨텍스트(레포 루트) 안에 있어 `docker compose build pumlex-server` 가 외부망 접근 없이 동작한다.

## 포함 범위

| 경로 | 역할 |
|---|---|
| `packages/pex-core/{geom,meta,inline}.js` | layout/meta 공용 모듈 (Node `require`). pex-server 가 SVG 후처리에 사용. |
| `packages/pex-server/server.js` | Express PlantUML 렌더러. `/render-with-layout` 등 제공. |
| `packages/pex-server/public/` | standalone 에디터 정적 자산 (`edit.html` / `demo-host.html`). |

**제외**: `pex-vscode` (VS Code 확장 — 서버 구동과 무관), `docs/`, 런타임 `data/`.

## 갱신 방법

원본 pumlex 가 바뀌면 아래로 다시 복사한다(파일 자체는 수정하지 않음):

```bash
SRC=/path/to/pumlex
VEN=deploy/vendor/pumlex
cp "$SRC/packages/pex-core/"{geom,meta,inline}.js  "$VEN/packages/pex-core/"
cp "$SRC/packages/pex-core/package.json"           "$VEN/packages/pex-core/"
cp "$SRC/packages/pex-server/"{server.js,package.json} "$VEN/packages/pex-server/"
cp -r "$SRC/packages/pex-server/public"            "$VEN/packages/pex-server/"
```

> 브라우저용 pex-core (`GoJIRA-App/public/pex-core/*.js`) 는 별도 vendoring 이며 이 디렉토리와 무관하다.
> 두 곳 모두 같은 원본에서 복사되므로 버전을 함께 맞춘다.
