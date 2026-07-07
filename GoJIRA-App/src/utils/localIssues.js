// 로컬 이슈 YAML → 웹이 기대하는 이슈 객체 배열 변환 (웹 이슈 뷰 이원화 Phase 1 §3 "내 작업본").
//
// 런처 GET /issues 가 돌려주는 [{path, content(raw yaml)}] 를 js-yaml 로 파싱한다.
// docs/issue-yaml-schema.md 의 필드(key/subkey/issueType/summary/status/labels/issueLinks/
// reqtype 등)가 그대로 YAML top-level 키라서, Mongo 캐시 이슈 객체와 shape 이 사실상 동일 —
// 변환은 최소(파싱 + 방어적 필드 보정)만 수행한다.
// js-yaml v5 는 default export 가 없고 named export(`load`)만 제공한다.
import { load } from 'js-yaml';

/**
 * parseLocalIssues — [{path, content}] → 이슈 객체 배열.
 * 파싱 실패 또는 key 누락(스키마 §5 필수 필드) 파일은 건너뛰고 콘솔 경고만 남긴다 —
 * 한 이슈 파일의 문제로 전체 로컬뷰가 깨지면 안 되므로.
 * @param {{path:string, content:string}[]} files
 * @returns {object[]} 이슈 객체 배열 (RelationMap/SATAM/IssueList 가 기대하는 shape)
 */
export function parseLocalIssues(files) {
  if (!Array.isArray(files)) return [];
  const issues = [];
  for (const f of files) {
    const path = f?.path || '(unknown)';
    let doc;
    try {
      doc = load(f?.content || '');
    } catch (err) {
      console.warn(`[parseLocalIssues] YAML 파싱 실패 — 건너뜀: ${path}`, err);
      continue;
    }
    if (!doc || typeof doc !== 'object') {
      console.warn(`[parseLocalIssues] 이슈 문서가 비어있거나 객체가 아님 — 건너뜀: ${path}`);
      continue;
    }
    // 필수 필드(key) 누락 시 렌더/필터 로직(issue.key 로 매핑·연결)이 깨지므로 스킵.
    if (!doc.key) {
      console.warn(`[parseLocalIssues] key 필드 누락 — 건너뜀: ${path}`);
      continue;
    }
    issues.push(doc);
  }
  return issues;
}

export default parseLocalIssues;
