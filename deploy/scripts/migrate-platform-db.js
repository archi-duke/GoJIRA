// D8 (2026-07-07): 플랫폼 데이터 platform DB 분리 + 앱 스코핑 마이그레이션.
// mongosh 로 **root 계정**으로 실행한다 (grant 포함). 멱등 — 재실행 안전.
//
//   docker exec shared-mongo mongosh -u root -p <rootpass> --authenticationDatabase admin \
//     --file /path/migrate-platform-db.js
//   (컨테이너 밖 파일이면: docker cp 로 넣거나 --eval "$(cat ...)" )
//
// 하는 일:
//   1. gojira_app 계정에 platform DB dbOwner grant (앱 계정 하나로 두 DB 사용)
//   2. gojira DB 의 플랫폼 소유 컬렉션 6종 → platform DB 로 복사 (_id 보존)
//      users, authGroups, orgUnits, userProjects, userConnections, notices
//   3. 앱 스코핑: authGroups/notices 에 app:'platform' 태그 (기존 데이터 귀속 결정)
//   4. GoJIRA 연속성: platform authGroups 를 app:'gojira' 로 복제하고, 각 사용자의
//      authGroupIDs 에 대응 복제 그룹 id 를 추가 — 기존 사용자 전원이 gojira 스코프에서
//      종전과 같은 권한을 유지한다. (RAGaaS 는 빈 상태로 시작)
//
// 주의: 원본 gojira DB 컬렉션은 지우지 않는다(백업 겸). 검증 후 수동 정리.

const COLLS = ['users', 'authGroups', 'orgUnits', 'userProjects', 'userConnections', 'notices'];
const src = db.getSiblingDB('gojira');
const dst = db.getSiblingDB('platform');

// ── 1. grant ──────────────────────────────────────────────────────────────
try {
  db.getSiblingDB('gojira').grantRolesToUser('gojira_app', [{ role: 'dbOwner', db: 'platform' }]);
  print('[1] gojira_app 에 platform dbOwner grant 완료');
} catch (e) {
  print('[1] grant 실패(이미 있거나 권한 부족): ' + e.message);
}

// ── 2. 컬렉션 복사 (멱등: 대상에 문서가 있으면 건너뜀) ────────────────────
for (const c of COLLS) {
  const n = src.getCollection(c).countDocuments();
  const already = dst.getCollection(c).countDocuments();
  if (already > 0) {
    print(`[2] platform.${c} 이미 ${already}건 — 복사 생략`);
    continue;
  }
  if (n === 0) { print(`[2] gojira.${c} 비어있음 — 생략`); continue; }
  const docs = src.getCollection(c).find().toArray();
  dst.getCollection(c).insertMany(docs);
  print(`[2] gojira.${c} → platform.${c} ${n}건 복사`);
}

// ── 3. 앱 스코핑: 기존 데이터 → app:'platform' ────────────────────────────
for (const c of ['authGroups', 'notices']) {
  const r = dst.getCollection(c).updateMany({ app: { $exists: false } }, { $set: { app: 'platform' } });
  print(`[3] platform.${c} app:'platform' 태그 ${r.modifiedCount}건`);
}

// ── 4. GoJIRA 연속성: platform 그룹 복제 → app:'gojira' + 사용자 링크 ─────
const platGroups = dst.authGroups.find({ app: 'platform', isActive: { $ne: false } }).toArray();
const idMap = {}; // platform group _id(str) → gojira group _id
for (const g of platGroups) {
  const exist = dst.authGroups.findOne({ app: 'gojira', code: g.code });
  if (exist) { idMap[g._id.toString()] = exist._id; continue; }
  const dup = Object.assign({}, g);
  dup._id = g._id + '_gojira'; // _id 는 문자열 관례 (예: ag_master → ag_master_gojira)
  dup.app = 'gojira';
  dst.authGroups.insertOne(dup);
  idMap[g._id.toString()] = dup._id;
  print(`[4] authGroup '${g.code}' → gojira 복제 (${dup._id})`);
}
let linked = 0;
for (const u of dst.users.find({}).toArray()) {
  const ids = (u.authGroupIds || []).map(x => x.toString());
  const add = [];
  for (const pid of ids) {
    const gid = idMap[pid];
    if (gid && !ids.includes(gid.toString())) add.push(gid);
  }
  if (add.length) {
    dst.users.updateOne({ _id: u._id }, { $push: { authGroupIds: { $each: add } } });
    linked++;
  }
}
print(`[4] 사용자 ${linked}명에 gojira 그룹 링크 추가`);

// ── 5. 구 unique index 정리: authGroups.code 단독 unique → (app, code) 는 API 가 생성 ──
try { dst.authGroups.dropIndex('code_1'); print('[5] authGroups code_1 단독 unique 제거'); }
catch (e) { print('[5] code_1 없음(정상): ' + e.codeName); }

print('== 마이그레이션 완료 ==');
