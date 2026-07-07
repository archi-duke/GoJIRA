// 숫자야구(NBBALL) 프로젝트 issueLinks 데이터 손상 복구 (2026-07-07).
//
// 증상: Relation Map 에서 이슈 간 연결선이 안 그려짐(링크는 있는데 표시 안 됨).
// 원인: NBBALL 의 issueLinks 128건 전부가 정상적인 중첩 객체
//   {id, type:{name}, outwardIssue:{key}, _addedBy} 대신
//   [{key:"id",value:...}, {key:"type",value:[{key:"name",value:...}]}, ...]
//   형태(키/값 쌍의 배열)로 저장돼 있었다. 전부 _addedBy:"usecase-analyze2" 태그가
//   붙은 자동 링크 생성 도구가 만든 것으로 추정 — GoJIRA-API 자체 코드
//   (handlers/link.go, db/mongo.go AddIssueLink)는 bson.M 을 정상적으로 써서
//   이 손상을 만들 수 없음을 확인했다(그 코드로 만든 GOJIRA/TEST 프로젝트 링크는
//   전부 정상). 외부 도구가 직접 DB 에 쓴 것으로 판단, 데이터만 복구한다.
//
// 실행 (mongosh, gojira_app 계정으로 충분 — gojira DB 내 issues 컬렉션 갱신):
//   docker cp repair-nbball-issuelinks.js shared-mongo:/tmp/repair.js
//   docker exec shared-mongo mongosh -u gojira_app -p <pass> --authenticationDatabase gojira \
//     --file /tmp/repair.js
//
// 멱등 — 이미 정상 형태인 링크는 그대로 둔다. 실행 후 malformed 카운트 0 확인.

const g = db.getSiblingDB("gojira");

// [{key,value},...] 형태(bson.D 가 JSON 배열로 잘못 평탄화된 것) → 중첩 객체로 재귀 변환.
// 정상 값(문자열/불리언/숫자/이미 객체인 값)은 그대로 통과.
function unflatten(v) {
  if (Array.isArray(v) && v.length > 0 && v.every(e => e && typeof e === 'object' && !Array.isArray(e) && 'key' in e && 'value' in e && Object.keys(e).length === 2)) {
    const obj = {};
    v.forEach(e => { obj[e.key] = unflatten(e.value); });
    return obj;
  }
  return v;
}

let scanned = 0, fixedIssues = 0, fixedLinks = 0;
g.issues.find({ key: /^NBBALL-/ }).forEach(doc => {
  scanned++;
  const links = doc.issueLinks || [];
  if (links.length === 0) return;
  let changed = false;
  const repaired = links.map(l => {
    if (Array.isArray(l)) { changed = true; fixedLinks++; return unflatten(l); }
    return l;
  });
  if (changed) {
    g.issues.updateOne({ _id: doc._id }, { $set: { issueLinks: repaired } });
    fixedIssues++;
  }
});

print(`scanned NBBALL issues: ${scanned}, fixed issues: ${fixedIssues}, fixed links: ${fixedLinks}`);

// 검증
let remaining = 0;
g.issues.find({ key: /^NBBALL-/ }).forEach(doc => {
  (doc.issueLinks || []).forEach(l => { if (Array.isArray(l)) remaining++; });
});
print(`remaining malformed after repair: ${remaining}`);
