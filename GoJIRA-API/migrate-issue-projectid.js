// migrate-issue-projectid.js
//
// 기존 이슈에 projectId 백필 + projects.issues 배열 폐기.
//
// 사용법:
//   mongosh <mongo-uri>/gojira --file scripts/migrate-issue-projectid.js
//
// 동작:
//   1. 모든 projects 의 jiraCodes.main.jiraCode → _id 매핑 build
//   2. projectId 가 비어있는 issues 순회 → key prefix 가 jiraCode 와 일치하는
//      첫 프로젝트의 _id 를 set
//   3. projects.issues 배열 unset (단일 소속 모델로 전환)
//
// 외부 코드 이슈(다른 jiraCode 로 시작하는 이슈) 는 skip 으로 보고됨.
// 그 경우 사용자가 수동으로 projectId 매핑 후 재실행.

print('=== migrate-issue-projectid ===');

const projects = db.projects.find({}, { jiraCodes: 1 }).toArray();
const codeToId = {};
for (const p of projects) {
  const code = p.jiraCodes && p.jiraCodes.main && p.jiraCodes.main.jiraCode;
  if (!code) continue;
  if (codeToId[code]) {
    print(`WARN: 같은 jiraCode "${code}" 의 프로젝트가 2개 이상. 첫 매칭(_id=${codeToId[code]}) 유지.`);
    continue;
  }
  codeToId[code] = p._id;
}
print(`프로젝트 ${Object.keys(codeToId).length}개 매핑 완료.`);

let updated = 0;
let skipped = 0;
const skippedKeys = [];

const cursor = db.issues.find({ projectId: { $exists: false } });
while (cursor.hasNext()) {
  const iss = cursor.next();
  const prefix = (iss.key || '').split('-')[0];
  const pid = codeToId[prefix];
  if (!pid) {
    skipped++;
    if (skippedKeys.length < 20) skippedKeys.push(iss.key);
    continue;
  }
  db.issues.updateOne({ _id: iss._id }, { $set: { projectId: pid } });
  updated++;
}

print(`updated: ${updated}`);
print(`skipped (매칭 프로젝트 없음): ${skipped}`);
if (skippedKeys.length > 0) {
  print('skipped 예시 (최대 20개): ' + skippedKeys.join(', '));
}

// projects.issues 배열은 단일 소속 모델 전환으로 더 이상 사용 안 함
const r = db.projects.updateMany({}, { $unset: { issues: '' } });
print(`projects.issues unset → matched=${r.matchedCount}, modified=${r.modifiedCount}`);

print('=== done ===');
