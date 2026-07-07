// D9 (2026-07-07): D8 이관(migrate-platform-db.js) 이후 gojira DB 에 남은 Platform 원본
// 컬렉션 정리. mongosh 로 **root 계정**으로 실행한다 (drop 권한 필요). 멱등 — 재실행 안전.
//
//   docker cp cleanup-gojira-platform-collections.js <mongo-container>:/tmp/cleanup.js
//   docker exec <mongo-container> mongosh -u root -p <rootpass> --authenticationDatabase admin \
//     --file /tmp/cleanup.js
//   (마이그레이션 스크립트와 같은 패턴 — deploy/scripts/migrate-platform-db.js 참고)
//
// 하는 일:
//   gojira DB 의 users/authGroups/orgUnits/notices/userProjects/userConnections/notifications
//   (D8 이관 후 남은 원본) 각각에 대해, platform DB 의 같은 컬렉션 문서 수가 gojira DB 쪽
//   문서 수 이상일 때만("이관이 실제로 다 됐다"는 안전 가드) gojira 쪽을 drop 한다.
//   가드를 통과하지 못하면 경고만 출력하고 건너뛴다(수동 확인 필요).
//
// 주의: notifications 는 migrate-platform-db.js 가 다루지 않던 컬렉션이지만 D8 이관 범위에
// 포함되는 원본이라 여기서 같이 정리한다. platform DB 쪽에 없으면(count 0) 가드에 걸려
// drop 되지 않으므로 안전하다.

const COLLS = ['users', 'authGroups', 'orgUnits', 'notices', 'userProjects', 'userConnections', 'notifications'];
const src = db.getSiblingDB('gojira');
const dst = db.getSiblingDB('platform');

for (const c of COLLS) {
  const srcCount = src.getCollection(c).countDocuments();
  const dstCount = dst.getCollection(c).countDocuments();

  if (srcCount === 0) {
    print(`[skip] gojira.${c} 비어있음 — drop 대상 없음`);
    continue;
  }

  if (dstCount >= srcCount) {
    src.getCollection(c).drop();
    print(`[drop] gojira.${c} 제거 (platform.${c}=${dstCount}건 >= gojira.${c}=${srcCount}건)`);
  } else {
    print(`[WARN] gojira.${c} 건너뜀 — platform.${c}=${dstCount}건 < gojira.${c}=${srcCount}건 (이관 미완료 의심, 수동 확인 필요)`);
  }
}

print('== gojira 원본 컬렉션 정리 완료 ==');
