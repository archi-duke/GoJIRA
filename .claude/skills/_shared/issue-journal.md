# 웹발 이슈 변경 저널 (`GOJIRA_ISSUE_JOURNAL`) 처리 지침

웹 로컬뷰("내 작업") 에서 사용자가 이슈를 편집하면(생성/수정/삭제/링크), 그 변경은 즉시
로컬 clone 의 `data/issues/*.yaml` 에 반영되지만 **설계 일관성 검토는 되지 않은 상태**다.
런처가 그 변경 내역을 **변경 저널** 에 누적 기록해 두고, 에이전트 세션(=너)에게 검토를
위임한다 (web-local-view-model.md §Phase 3 옵션(a)).

- **위치**: env `GOJIRA_ISSUE_JOURNAL` 이 가리키는 절대경로 (`~/.gojira/journal/<CODE>.yaml`).
  clone 안이 아니라 런처 영역이다 — 커밋/PR 에 섞이지 않으니 git 상태 걱정 없이 편집해도 된다.
- **파일이 없거나 비어 있으면 할 일 없음** — 이 문서의 나머지는 무시하고 원래 작업을 계속한다.

## 항목 shape (YAML 리스트, append-only)

```yaml
- ts: "2026-07-07T18:30:00+09:00"   # 변경 시각 (RFC3339, 런처가 스탬프)
  user: "duke.kimm"                 # 변경한 사용자
  op: "link"                        # create | update | delete | link | unlink
  keys: ["NBBALL-3", "NBBALL-7"]    # 영향받은 이슈 key(들)
  summary: "링크 연결: NBBALL-3 → NBBALL-7"
  fields: ["summary", "priority"]   # update 시 변경된 필드명 — 있을 때만
```

## 처리 절차

저널의 각 항목에 대해, 변경이 설계 산출물에 미치는 영향을 검토한다:

1. **대상 이슈 파악** — `keys` 의 이슈 YAML(`data/issues/`)을 읽어 현재 상태를 확인한다.
   (delete 면 파일이 이미 없을 수 있다 — 그 자체가 검토 대상.)
2. **SATAM 매트릭스 정합** — 변경된 이슈가 NFR/ADSN 이거나 ADSN 과 링크돼 있으면,
   risk/sensitivity/mitigation 관계가 여전히 성립하는지 검토한다. 링크 삭제·이슈 삭제로
   mitigation 참조가 끊기지 않았는지 특히 확인 (재판정 필요 시 `satam-analyze`).
3. **유스케이스 산출물** — VOS/Feature/UC 계열 변경이면 `business/use-cases.md` 와
   VOS↔Feature 커버리지가 깨지지 않았는지 확인 (재정립 필요 시 `usecase-analyze` MODE=relink).
4. **설계 문서** — `architecture/` 문서(drivers/style/design-decisions/integrated)가 변경된
   이슈를 인용하고 있으면 본문·인용 key 가 실제 이슈 상태와 어긋나지 않는지 확인한다.
5. **링크 정합** — link/unlink/delete 항목은 양쪽 이슈의 links 필드가 대칭인지, 고아
   링크가 남지 않았는지 확인한다 (`issue-analyze` 활용 가능).

수정이 필요하면 해당 스킬(issue-update / issue-link / 문서 편집 등)로 직접 반영한다.

## 처리 후 저널 정리 (필수)

- **처리 완료한 항목은 저널 파일에서 제거한다.** 전부 처리했으면 파일 내용을 비운다
  (빈 파일로 두면 됨 — 파일 삭제까지는 불필요).
- 판단이 서지 않아 **보류한 항목은 파일에 남겨두고**, 무엇을 왜 보류했는지 사용자에게
  보고한다 (다음 세션이 이어받는다).
- 저널은 append-only 로 쌓이므로, 정리하지 않으면 다음 세션이 같은 항목을 중복 검토하게
  된다 — 반드시 소비(제거)까지 마칠 것.
