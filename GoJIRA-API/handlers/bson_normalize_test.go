package handlers

// NBBALL 과제 사고 재현 회귀 테스트 — bson.D 를 정규화 없이 yaml.Marshal 하면
// 오염이 재현되고(validateIssueDoc 이 거부), normalizeBsonDoc/normalizeBsonValue
// 를 거치면 왕복 후에도 정상 구조가 보존되는지 검증한다.

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"
)

// buildContaminatedIssueDoc — NBBALL 사고와 동일한 형태로 issueLinks 를
// bson.D(중첩 bson.D 포함)로 구성한 이슈 doc 을 만든다. Mongo 조회 결과가
// 이런 모양으로 디코드된다.
func buildContaminatedIssueDoc() bson.M {
	link := bson.D{
		{Key: "id", Value: "tmp_1"},
		{Key: "type", Value: bson.D{
			{Key: "name", Value: "Contains(WBSGantt)"},
		}},
		{Key: "outwardIssue", Value: bson.D{
			{Key: "key", Value: "NBBALL-34"},
		}},
	}
	return bson.M{
		"key":       "NBBALL-12",
		"issueType": "Architectural Design",
		"summary":   "테스트 이슈",
		"issueLinks": bson.A{
			link,
		},
	}
}

// roundTrip — doc 을 yaml.Marshal 후 다시 yaml.Unmarshal 하여 bson.M 으로 반환한다.
func roundTrip(t *testing.T, doc interface{}) bson.M {
	t.Helper()
	out, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := yaml.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	return bson.M(decoded)
}

// TestNormalize_ContaminationReproducedWithoutNormalize — normalize 없이
// bson.D 를 그대로 yaml.Marshal 하면 NBBALL 사고가 재현되어(issueLinks 원소가
// 배열로 직렬화됨) validateIssueDoc 이 이를 거부하는지 확인한다.
func TestNormalize_ContaminationReproducedWithoutNormalize(t *testing.T) {
	doc := buildContaminatedIssueDoc()

	decoded := roundTrip(t, doc)

	if err := validateIssueDoc(decoded); err == nil {
		t.Fatalf("expected validateIssueDoc to reject un-normalized bson.D contamination, got nil error; decoded=%#v", decoded)
	}
}

// TestNormalize_FixesContamination — normalizeBsonDoc 을 거친 후에는 왕복해도
// validateIssueDoc 을 통과하고, outwardIssue.key 값이 보존되는지 확인한다.
func TestNormalize_FixesContamination(t *testing.T) {
	doc := buildContaminatedIssueDoc()

	normalized := normalizeBsonDoc(doc)
	decoded := roundTrip(t, normalized)

	if err := validateIssueDoc(decoded); err != nil {
		t.Fatalf("expected validateIssueDoc to pass after normalize, got error: %v; decoded=%#v", err, decoded)
	}

	links, ok := asSlice(decoded["issueLinks"])
	if !ok || len(links) != 1 {
		t.Fatalf("expected 1 issueLinks entry, got %#v", decoded["issueLinks"])
	}
	linkMap, ok := asMap(links[0])
	if !ok {
		t.Fatalf("expected issueLinks[0] to be a map, got %T", links[0])
	}
	outward, ok := asMap(linkMap["outwardIssue"])
	if !ok {
		t.Fatalf("expected outwardIssue to be a map, got %T", linkMap["outwardIssue"])
	}
	if key, _ := outward["key"].(string); key != "NBBALL-34" {
		t.Fatalf("expected outwardIssue.key = NBBALL-34, got %q", key)
	}
}

// TestNormalizeBsonValue_NestedBsonA — bson.A 안에 bson.D/bson.A 가 중첩되어도
// 재귀적으로 순수 map[string]interface{}/[]interface{} 로 변환되는지 검증한다.
func TestNormalizeBsonValue_NestedBsonA(t *testing.T) {
	input := bson.A{
		bson.D{{Key: "a", Value: 1}},
		bson.A{"x", "y"},
		bson.M{"nested": bson.D{{Key: "b", Value: 2}}},
	}

	got := normalizeBsonValue(input)

	slice, ok := got.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", got)
	}
	if len(slice) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(slice))
	}

	m0, ok := slice[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected element 0 to be map[string]interface{}, got %T", slice[0])
	}
	if m0["a"] != 1 {
		t.Fatalf("expected a=1, got %#v", m0["a"])
	}

	s1, ok := slice[1].([]interface{})
	if !ok || len(s1) != 2 || s1[0] != "x" || s1[1] != "y" {
		t.Fatalf("expected [x y], got %#v", slice[1])
	}

	m2, ok := slice[2].(map[string]interface{})
	if !ok {
		t.Fatalf("expected element 2 to be map[string]interface{}, got %T", slice[2])
	}
	nested, ok := m2["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested to be map[string]interface{}, got %T", m2["nested"])
	}
	if nested["b"] != 2 {
		t.Fatalf("expected b=2, got %#v", nested["b"])
	}
}

// TestNormalizeBsonValue_ScalarsPassThrough — 스칼라 값(string/int/bool/nil)은
// 변형 없이 그대로 반환되는지 확인한다.
func TestNormalizeBsonValue_ScalarsPassThrough(t *testing.T) {
	cases := []interface{}{"hello", 42, true, 3.14, nil}
	for _, c := range cases {
		got := normalizeBsonValue(c)
		if got != c {
			t.Fatalf("expected scalar %#v to pass through unchanged, got %#v", c, got)
		}
	}
}

// TestNormalizeBsonDoc_Nil — nil 필드/빈 doc 을 넣어도 패닉 없이 처리되는지 확인한다.
func TestNormalizeBsonDoc_Nil(t *testing.T) {
	doc := bson.M{
		"key":        "NBBALL-1",
		"issueType":  "Task",
		"issueLinks": nil,
		"labels":     nil,
	}

	normalized := normalizeBsonDoc(doc)

	if normalized["issueLinks"] != nil {
		t.Fatalf("expected nil issueLinks to remain nil, got %#v", normalized["issueLinks"])
	}
	if normalized["labels"] != nil {
		t.Fatalf("expected nil labels to remain nil, got %#v", normalized["labels"])
	}
	if normalized["key"] != "NBBALL-1" {
		t.Fatalf("expected key preserved, got %#v", normalized["key"])
	}

	// normalizeBsonDoc(nil) 자체도 패닉 없이 빈 맵을 반환해야 한다.
	empty := normalizeBsonDoc(nil)
	if len(empty) != 0 {
		t.Fatalf("expected empty map for nil doc, got %#v", empty)
	}
}
