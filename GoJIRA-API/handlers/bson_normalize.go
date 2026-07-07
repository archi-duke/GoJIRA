package handlers

// bson.D → yaml.v3 오염 방지 정규화.
//
// 배경 (NBBALL 과제 issueLinks 128건 실사고): db.FindIssueByKey 등으로 Mongo 에서
// 읽은 bson.M 문서는 중첩 문서가 bson.D(= []bson.E, Key/Value 구조체 배열)로
// 디코드된다. 이 bson.D 를 그대로 gopkg.in/yaml.v3 의 yaml.Marshal 에 넘기면
// yaml.v3 는 구조체 배열이라는 사실만 보고 각 원소를 {key: ..., value: ...} 맵으로
// 직렬화해버린다. 즉 정상적인
//   issueLinks:
//     - id: "..."
//       type: {name: "Contains(WBSGantt)"}
//       outwardIssue: {key: "NBBALL-34"}
// 이 아래처럼 키/값 쌍 배열로 오염된다.
//   issueLinks:
//     - - key: id
//         value: "..."
//       - key: type
//         value:
//           - key: name
//             value: "Contains(WBSGantt)"
//       ...
// 이렇게 오염된 YAML 이 localWrites 로 로컬 clone 에 그대로 기록됐고, 이후
// 재-ingest 될 때 Mongo 캐시까지 오염시켜 Relation Map 의 연결선이 전부
// 사라지는 사고로 이어졌다 (NBBALL issueLinks 128건).
//
// 해결책: yaml.Marshal 직전에 Mongo 조회 결과를 재귀적으로 순수
// map[string]interface{} / []interface{} 로 변환한다 (normalizeBsonValue).
// bson.D/bson.M/bson.A 어떤 조합으로 중첩되어 있어도 최종적으로는 yaml.v3 가
// 맵/배열로 올바르게 인식하는 형태만 남는다.

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

// normalizeBsonValue — Mongo 디코드 결과(bson.D/bson.M/bson.A 및 그 중첩)를
// yaml.Marshal 이 맵/배열로 올바르게 직렬화할 수 있는 순수
// map[string]interface{} / []interface{} / 스칼라로 재귀 변환한다.
func normalizeBsonValue(v interface{}) interface{} {
	switch t := v.(type) {
	case bson.D:
		m := make(map[string]interface{}, len(t))
		for _, e := range t {
			m[e.Key] = normalizeBsonValue(e.Value)
		}
		return m
	case bson.M:
		m := make(map[string]interface{}, len(t))
		for k, val := range t {
			m[k] = normalizeBsonValue(val)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{}, len(t))
		for k, val := range t {
			m[k] = normalizeBsonValue(val)
		}
		return m
	case bson.A:
		s := make([]interface{}, len(t))
		for i, item := range t {
			s[i] = normalizeBsonValue(item)
		}
		return s
	case []interface{}:
		s := make([]interface{}, len(t))
		for i, item := range t {
			s[i] = normalizeBsonValue(item)
		}
		return s
	default:
		// 스칼라(string/number/bool/time.Time/primitive.ObjectID/nil 등)는 그대로 반환.
		return v
	}
}

// normalizeBsonDoc — 최상위 이슈 doc(bson.M) 을 정규화한 bson.M 으로 반환하는
// 편의 래퍼. yaml.Marshal 직전에 호출한다.
func normalizeBsonDoc(doc bson.M) bson.M {
	normalized := make(bson.M, len(doc))
	for k, v := range doc {
		normalized[k] = normalizeBsonValue(v)
	}
	return normalized
}
