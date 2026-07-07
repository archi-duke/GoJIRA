package handlers

// validateIssueDoc 단위 테스트 — NBBALL issueLinks 오염 재발 방지 검증 (web-local-view-model.md Phase 0).

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestValidateIssueDoc_Valid_MinimalFields(t *testing.T) {
	doc := bson.M{
		"key":       "NBBALL-12",
		"issueType": "Requirement",
	}
	if err := validateIssueDoc(doc); err != nil {
		t.Fatalf("expected minimal valid doc to pass, got error: %v", err)
	}
}

func TestValidateIssueDoc_Valid_IssueLinksShapes(t *testing.T) {
	doc := bson.M{
		"key":       "NBBALL-12",
		"issueType": "Requirement",
		"summary":   "게임방 생성 기능",
		"labels":    []interface{}{"backend", "api"},
		"issueLinks": []interface{}{
			// outwardIssue 형
			map[string]interface{}{
				"id":           "10053",
				"type":         map[string]interface{}{"name": "Contains(WBSGantt)"},
				"outwardIssue": map[string]interface{}{"key": "NBBALL-9"},
			},
			// inwardIssue 형
			map[string]interface{}{
				"id":          "10054",
				"inwardIssue": map[string]interface{}{"key": "NBBALL-10"},
			},
			// type 포함 + 둘 다 존재
			map[string]interface{}{
				"type":         map[string]interface{}{"name": "Blocks"},
				"outwardIssue": map[string]interface{}{"key": "NBBALL-11"},
				"inwardIssue":  map[string]interface{}{"key": "NBBALL-13"},
			},
		},
	}
	if err := validateIssueDoc(doc); err != nil {
		t.Fatalf("expected valid issueLinks shapes to pass, got error: %v", err)
	}
}

func TestValidateIssueDoc_Reject(t *testing.T) {
	cases := []struct {
		name    string
		doc     bson.M
		wantErr string
	}{
		{
			name: "missing key",
			doc: bson.M{
				"issueType": "Requirement",
			},
			wantErr: "key",
		},
		{
			name: "key format violation - lowercase",
			doc: bson.M{
				"key":       "nbball-12",
				"issueType": "Requirement",
			},
			wantErr: "key",
		},
		{
			name: "key format violation - no number",
			doc: bson.M{
				"key":       "NBBALL",
				"issueType": "Requirement",
			},
			wantErr: "key",
		},
		{
			name: "missing issueType",
			doc: bson.M{
				"key": "NBBALL-12",
			},
			wantErr: "issueType",
		},
		{
			name: "issueLinks element is an array (NBBALL 오염 재현)",
			doc: bson.M{
				"key":       "NBBALL-12",
				"issueType": "Requirement",
				"issueLinks": []interface{}{
					[]interface{}{
						map[string]interface{}{"key": "id", "value": "tmp_123"},
						map[string]interface{}{"key": "outwardIssue", "value": []interface{}{
							map[string]interface{}{"key": "key", "value": "NBBALL-9"},
						}},
					},
				},
			},
			wantErr: "malformed link shape",
		},
		{
			name: "outwardIssue is a string instead of object",
			doc: bson.M{
				"key":       "NBBALL-12",
				"issueType": "Requirement",
				"issueLinks": []interface{}{
					map[string]interface{}{
						"outwardIssue": "NBBALL-9",
					},
				},
			},
			wantErr: "outwardIssue",
		},
		{
			name: "outward and inward both missing",
			doc: bson.M{
				"key":       "NBBALL-12",
				"issueType": "Requirement",
				"issueLinks": []interface{}{
					map[string]interface{}{
						"id":   "10053",
						"type": map[string]interface{}{"name": "Contains"},
					},
				},
			},
			wantErr: "neither outwardIssue nor inwardIssue",
		},
		{
			name: "labels element is a number",
			doc: bson.M{
				"key":       "NBBALL-12",
				"issueType": "Requirement",
				"labels":    []interface{}{"backend", 42},
			},
			wantErr: "labels",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateIssueDoc(tc.doc)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error to contain %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// TestParseAndValidate_ValidYAML — 실제 YAML 문자열에서 시작해 parseIssueYAML → validateIssueDoc
// 흐름 전체를 검증한다 (파싱 결과 타입이 실제 ingest 경로와 동일하도록).
func TestParseAndValidate_ValidYAML(t *testing.T) {
	yamlContent := `
key: NBBALL-12
subkey: FR_001
issueType: Requirement
summary: 게임방 생성 기능
status: To Do
labels: []
issueLinks:
  - id: "10053"
    type: { name: "Contains(WBSGantt)" }
    outwardIssue: { key: NBBALL-9 }
  - id: "10054"
    inwardIssue: { key: NBBALL-10 }
`
	doc, key, err := parseIssueYAML(yamlContent)
	if err != nil {
		t.Fatalf("parseIssueYAML failed: %v", err)
	}
	if key != "NBBALL-12" {
		t.Fatalf("expected key NBBALL-12, got %q", key)
	}
	if err := validateIssueDoc(doc); err != nil {
		t.Fatalf("expected valid YAML to pass validation, got error: %v", err)
	}
}

// TestParseAndValidate_MalformedIssueLinks_YAML — NBBALL 사고를 실제 야기한 shape 를
// yaml.Unmarshal 로 재현: issueLinks 각 원소가 [{key,value},...] 배열.
func TestParseAndValidate_MalformedIssueLinks_YAML(t *testing.T) {
	yamlContent := `
key: NBBALL-12
issueType: Requirement
issueLinks: [[{key: id, value: tmp_123}, {key: outwardIssue, value: [{key: key, value: NBBALL-9}]}]]
`
	doc, key, err := parseIssueYAML(yamlContent)
	if err != nil {
		t.Fatalf("parseIssueYAML failed: %v", err)
	}
	if key != "NBBALL-12" {
		t.Fatalf("expected key NBBALL-12, got %q", key)
	}
	err = validateIssueDoc(doc)
	if err == nil {
		t.Fatal("expected malformed issueLinks shape to be rejected, got nil error")
	}
	if !strings.Contains(err.Error(), "malformed link shape") {
		t.Fatalf("expected error to mention malformed link shape, got: %v", err)
	}
}
