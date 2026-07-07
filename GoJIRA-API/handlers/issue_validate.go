package handlers

// 이슈 ingest 구조 검증 (web-local-view-model.md Phase 0).
//
// NBBALL 과제 사고 재현: issueLinks 128건이 정상 중첩 객체
//   {id, type:{name}, outwardIssue:{key}} 대신 키/값 쌍 배열
//   [{key:"id",value:"..."},{key:"type",value:[...]},...] 형태로 저장되어
// Relation Map 연결선이 전부 사라졌다. parseIssueYAML 직후, Mongo 캐시에
// upsert 되기 전에 이 검증을 통과해야 한다.

import (
	"fmt"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// key 형식: 대문자로 시작하는 프로젝트 코드 + "-" + 숫자. 예: NBBALL-12.
var issueKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*-\d+$`)

// asMap — bson.M / map[string]interface{} / bson.D 를 공통 map[string]interface{} 로 변환.
// YAML 파싱 결과는 보통 map[string]interface{} 이지만, Mongo 조회 경로(bson.D)에서도
// 재사용 가능하도록 양쪽 다 처리한다 (db/sync.go 의 toBsonM 패턴 참고).
func asMap(v interface{}) (map[string]interface{}, bool) {
	switch t := v.(type) {
	case bson.M:
		return t, true
	case map[string]interface{}:
		return t, true
	case bson.D:
		m := make(map[string]interface{}, len(t))
		for _, e := range t {
			m[e.Key] = e.Value
		}
		return m, true
	}
	return nil, false
}

// asSlice — []interface{} / bson.A 를 공통 []interface{} 로 변환.
func asSlice(v interface{}) ([]interface{}, bool) {
	switch t := v.(type) {
	case []interface{}:
		return t, true
	case bson.A:
		return []interface{}(t), true
	}
	return nil, false
}

// validateIssueDoc — ingest 되는 이슈 doc 의 최소 구조를 검증한다.
// 필수 조건은 key/issueType 뿐이고, 나머지 필드는 "존재하면 타입 검증" 원칙을 따른다
// (기존 정상 데이터 재-ingest 시 깨지지 않도록).
func validateIssueDoc(doc bson.M) error {
	key, ok := doc["key"].(string)
	if !ok || key == "" {
		return fmt.Errorf("key: required non-empty string")
	}
	if !issueKeyPattern.MatchString(key) {
		return fmt.Errorf("key: %q does not match format <PROJECT>-<number> (e.g. NBBALL-12)", key)
	}

	issueType, ok := doc["issueType"].(string)
	if !ok || issueType == "" {
		return fmt.Errorf("issueType: required non-empty string")
	}

	if v, exists := doc["summary"]; exists && v != nil {
		if _, ok := v.(string); !ok {
			return fmt.Errorf("summary: expected string, got %T", v)
		}
	}

	if v, exists := doc["labels"]; exists && v != nil {
		labels, ok := asSlice(v)
		if !ok {
			return fmt.Errorf("labels: expected array, got %T", v)
		}
		for i, l := range labels {
			if _, ok := l.(string); !ok {
				return fmt.Errorf("labels[%d]: expected string, got %T", i, l)
			}
		}
	}

	if v, exists := doc["issueLinks"]; exists && v != nil {
		links, ok := asSlice(v)
		if !ok {
			return fmt.Errorf("issueLinks: expected array, got %T", v)
		}
		for i, item := range links {
			if err := validateIssueLink(i, item); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateIssueLink — issueLinks[i] 원소 검증.
// 원소가 배열이면 NBBALL 류 [{key,value},...] 오염이므로 map 여부를 보기 전에 즉시 거부한다.
func validateIssueLink(i int, item interface{}) error {
	if _, isArr := asSlice(item); isArr {
		return fmt.Errorf("issueLinks[%d]: element is an array, expected object — malformed link shape", i)
	}
	link, ok := asMap(item)
	if !ok {
		return fmt.Errorf("issueLinks[%d]: expected object, got %T", i, item)
	}

	if v, exists := link["id"]; exists && v != nil {
		if _, ok := v.(string); !ok {
			return fmt.Errorf("issueLinks[%d].id: expected string, got %T", i, v)
		}
	}

	if v, exists := link["type"]; exists && v != nil {
		typeMap, ok := asMap(v)
		if !ok {
			return fmt.Errorf("issueLinks[%d].type: expected object, got %T", i, v)
		}
		if nv, exists := typeMap["name"]; exists && nv != nil {
			if _, ok := nv.(string); !ok {
				return fmt.Errorf("issueLinks[%d].type.name: expected string, got %T", i, nv)
			}
		}
	}

	hasOutward, err := validateLinkedIssueRef(link, "outwardIssue", i)
	if err != nil {
		return err
	}
	hasInward, err := validateLinkedIssueRef(link, "inwardIssue", i)
	if err != nil {
		return err
	}
	if !hasOutward && !hasInward {
		return fmt.Errorf("issueLinks[%d]: neither outwardIssue nor inwardIssue present", i)
	}

	return nil
}

// validateLinkedIssueRef — outwardIssue/inwardIssue 필드 검증.
// 반환값의 bool 은 "필드가 존재했는지" — outward/inward 둘 중 최소 하나 존재 검사에 쓰인다.
func validateLinkedIssueRef(link map[string]interface{}, field string, i int) (bool, error) {
	v, exists := link[field]
	if !exists || v == nil {
		return false, nil
	}
	ref, ok := asMap(v)
	if !ok {
		return true, fmt.Errorf("issueLinks[%d].%s: expected object, got %T", i, field, v)
	}
	k, ok := ref["key"].(string)
	if !ok || k == "" {
		return true, fmt.Errorf("issueLinks[%d].%s.key: required non-empty string", i, field)
	}
	return true, nil
}
