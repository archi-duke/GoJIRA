package handlers

// issue_local.go 순수 헬퍼 단위 테스트 — Mongo 없이 실행 (web-local-view-model.md §Phase 2).

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"
)

const baseIssueYAML = `
key: NBBALL-12
subkey: FR_001
issueType: Requirement
summary: 게임방 생성 기능
status: To Do
priority: Medium
updated: "2026-01-01T00:00:00.000+0900"
`

func mustParseYAML(t *testing.T, content string) bson.M {
	t.Helper()
	var doc bson.M
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	return doc
}

// ── localMergeUpdate (PUT /Issue) ──

func TestLocalMergeUpdate_OverwritesFieldsAndUpdatesTimestamp(t *testing.T) {
	lw, err := localMergeUpdate(baseIssueYAML, "NBBALL-12", map[string]interface{}{
		"summary": "게임방 생성 기능 (개정)",
		"status":  "In Progress",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lw.Path != "data/issues/FR_001.yaml" {
		t.Fatalf("unexpected path: %s", lw.Path)
	}
	if lw.Delete {
		t.Fatalf("expected Delete=false")
	}

	doc := mustParseYAML(t, lw.Content)
	if doc["summary"] != "게임방 생성 기능 (개정)" {
		t.Fatalf("summary not overwritten: %v", doc["summary"])
	}
	if doc["status"] != "In Progress" {
		t.Fatalf("status not overwritten: %v", doc["status"])
	}
	// 병합되지 않은 필드는 보존되어야 한다.
	if doc["priority"] != "Medium" {
		t.Fatalf("priority should be preserved: %v", doc["priority"])
	}
	updated, _ := doc["updated"].(string)
	if updated == "" || updated == "2026-01-01T00:00:00.000+0900" {
		t.Fatalf("updated timestamp should be refreshed, got %q", updated)
	}
}

func TestLocalMergeUpdate_RejectsKeyMismatch(t *testing.T) {
	_, err := localMergeUpdate(baseIssueYAML, "NBBALL-99", map[string]interface{}{"summary": "x"})
	if err == nil {
		t.Fatal("expected error for key mismatch")
	}
	if localWriteStatus(err) != 400 {
		t.Fatalf("expected 400 status for key mismatch, got %d", localWriteStatus(err))
	}
}

func TestLocalMergeUpdate_RejectsMissingSubkey(t *testing.T) {
	yamlNoSubkey := `
key: NBBALL-12
issueType: Requirement
summary: 게임방 생성 기능
`
	_, err := localMergeUpdate(yamlNoSubkey, "NBBALL-12", map[string]interface{}{"summary": "x"})
	if err == nil {
		t.Fatal("expected error for missing subkey")
	}
	if localWriteStatus(err) != 409 {
		t.Fatalf("expected 409 status for missing subkey, got %d", localWriteStatus(err))
	}
}

func TestLocalMergeUpdate_RejectsInvalidResultingDoc(t *testing.T) {
	// data 로 issueLinks 를 오염된(NBBALL 류) 배열 shape 로 덮어써서 validateIssueDoc 이 거부하는지 확인.
	_, err := localMergeUpdate(baseIssueYAML, "NBBALL-12", map[string]interface{}{
		"issueLinks": []interface{}{
			[]interface{}{
				map[string]interface{}{"key": "id", "value": "tmp_1"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected validation error for malformed issueLinks")
	}
	if !strings.Contains(err.Error(), "malformed link shape") {
		t.Fatalf("expected malformed link shape error, got: %v", err)
	}
}

// ── localAddLink (POST /Issue/link) ──

func TestLocalAddLink_BothSides(t *testing.T) {
	inwardYAML := `
key: NBBALL-1
subkey: FEAT_001
issueType: Feature
summary: 상위 기능
`
	outwardYAML := `
key: NBBALL-2
subkey: FR_002
issueType: Requirement
summary: 하위 요구사항
`
	writes, err := localAddLink(inwardYAML, outwardYAML, "NBBALL-1", "NBBALL-2", "requirement-refine2", "tmp_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(writes) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(writes))
	}

	inwardDoc := mustParseYAML(t, writes[0].Content)
	links, ok := asSlice(inwardDoc["issueLinks"])
	if !ok || len(links) != 1 {
		t.Fatalf("expected 1 issueLinks entry in inward doc, got %#v", inwardDoc["issueLinks"])
	}
	link, ok := asMap(links[0])
	if !ok {
		t.Fatalf("issueLinks[0] not a map: %#v", links[0])
	}
	if link["id"] != "tmp_123" {
		t.Fatalf("unexpected id: %v", link["id"])
	}
	typeMap, ok := asMap(link["type"])
	if !ok || typeMap["name"] != "Contains(WBSGantt)" {
		t.Fatalf("unexpected type shape: %#v", link["type"])
	}
	outwardRef, ok := asMap(link["outwardIssue"])
	if !ok || outwardRef["key"] != "NBBALL-2" {
		t.Fatalf("unexpected outwardIssue shape: %#v", link["outwardIssue"])
	}
	if link["_addedBy"] != "requirement-refine2" {
		t.Fatalf("expected _addedBy set, got %v", link["_addedBy"])
	}

	outwardDoc := mustParseYAML(t, writes[1].Content)
	oLinks, ok := asSlice(outwardDoc["issueLinks"])
	if !ok || len(oLinks) != 1 {
		t.Fatalf("expected 1 issueLinks entry in outward doc, got %#v", outwardDoc["issueLinks"])
	}
	oLink, ok := asMap(oLinks[0])
	if !ok {
		t.Fatalf("issueLinks[0] not a map: %#v", oLinks[0])
	}
	inwardRef, ok := asMap(oLink["inwardIssue"])
	if !ok || inwardRef["key"] != "NBBALL-1" {
		t.Fatalf("unexpected inwardIssue shape: %#v", oLink["inwardIssue"])
	}
}

func TestLocalAddLink_OnlyInwardSide(t *testing.T) {
	inwardYAML := `
key: NBBALL-1
subkey: FEAT_001
issueType: Feature
summary: 상위 기능
`
	writes, err := localAddLink(inwardYAML, "", "NBBALL-1", "OTHERPRJ-9", "", "tmp_456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 write (inward only), got %d", len(writes))
	}
	if writes[0].Path != "data/issues/FEAT_001.yaml" {
		t.Fatalf("unexpected path: %s", writes[0].Path)
	}
}

func TestLocalAddLink_NewIssueLinksArrayWhenAbsent(t *testing.T) {
	// baseIssueYAML has no issueLinks field at all.
	writes, err := localAddLink(baseIssueYAML, "", "NBBALL-12", "NBBALL-99", "", "tmp_789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doc := mustParseYAML(t, writes[0].Content)
	links, ok := asSlice(doc["issueLinks"])
	if !ok || len(links) != 1 {
		t.Fatalf("expected new issueLinks array with 1 entry, got %#v", doc["issueLinks"])
	}
}

func TestLocalAddLink_RejectsWhenNeitherYamlGiven(t *testing.T) {
	_, err := localAddLink("", "", "NBBALL-1", "NBBALL-2", "", "tmp_1")
	if err == nil {
		t.Fatal("expected error when neither yaml is given")
	}
}

// ── localRemoveLink (DELETE /Issue/link, /Issue/link/{linkId}/of/{key}) ──

func withIssueLinks(base string, linkID string) string {
	return base + `
issueLinks:
  - id: "` + linkID + `"
    type: { name: "Contains(WBSGantt)" }
    outwardIssue: { key: "NBBALL-99" }
  - id: "other_link"
    type: { name: "Contains(WBSGantt)" }
    outwardIssue: { key: "NBBALL-77" }
`
}

func TestLocalRemoveLink_RemovesOnlyMatchingID(t *testing.T) {
	yamlWithLinks := withIssueLinks(baseIssueYAML, "tmp_123")
	writes, err := localRemoveLink([]string{yamlWithLinks}, "tmp_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(writes))
	}
	doc := mustParseYAML(t, writes[0].Content)
	links, ok := asSlice(doc["issueLinks"])
	if !ok || len(links) != 1 {
		t.Fatalf("expected 1 remaining issueLinks entry, got %#v", doc["issueLinks"])
	}
	remaining, _ := asMap(links[0])
	if remaining["id"] != "other_link" {
		t.Fatalf("expected 'other_link' to remain, got %v", remaining["id"])
	}
}

func TestLocalRemoveLink_ExcludesFilesWithoutMatch(t *testing.T) {
	yamlWithOtherLink := withIssueLinks(baseIssueYAML, "some_other_id")
	writes, err := localRemoveLink([]string{yamlWithOtherLink}, "tmp_does_not_exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(writes) != 0 {
		t.Fatalf("expected 0 writes when linkID not present, got %d", len(writes))
	}
}

func TestLocalRemoveLink_MultipleFilesOnlyMatchingIncluded(t *testing.T) {
	matching := withIssueLinks(baseIssueYAML, "tmp_match")
	nonMatching := withIssueLinks(`
key: NBBALL-13
subkey: FR_003
issueType: Requirement
summary: 다른 이슈
`, "unrelated_id")

	writes, err := localRemoveLink([]string{matching, nonMatching}, "tmp_match")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected exactly 1 write, got %d", len(writes))
	}
	if writes[0].Path != "data/issues/FR_001.yaml" {
		t.Fatalf("unexpected path: %s", writes[0].Path)
	}
}

// ── localDeleteIssue (DELETE /Issue) ──

func TestLocalDeleteIssue_ReturnsDeleteWrite(t *testing.T) {
	lw, err := localDeleteIssue(baseIssueYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !lw.Delete {
		t.Fatalf("expected Delete=true")
	}
	if lw.Content != "" {
		t.Fatalf("expected empty content for delete write, got %q", lw.Content)
	}
	if lw.Path != "data/issues/FR_001.yaml" {
		t.Fatalf("unexpected path: %s", lw.Path)
	}
}

func TestLocalDeleteIssue_RejectsMissingSubkey(t *testing.T) {
	yamlNoSubkey := `
key: NBBALL-12
issueType: Requirement
summary: 게임방 생성 기능
`
	_, err := localDeleteIssue(yamlNoSubkey)
	if err == nil {
		t.Fatal("expected error for missing subkey")
	}
	if localWriteStatus(err) != 409 {
		t.Fatalf("expected 409 status, got %d", localWriteStatus(err))
	}
}

// ── validateIssueDoc 위반 YAML 거부 (공통 경로) ──

func TestLocalHelpers_RejectMalformedYAMLViaValidate(t *testing.T) {
	malformed := `
key: NBBALL-12
subkey: FR_001
issueType: Requirement
issueLinks: [[{key: id, value: tmp_1}]]
`
	if _, err := localMergeUpdate(malformed, "NBBALL-12", map[string]interface{}{}); err == nil {
		t.Fatal("expected localMergeUpdate to reject malformed issueLinks")
	}
	if _, err := localDeleteIssue(malformed); err != nil {
		t.Fatalf("localDeleteIssue does not validate content shape (only subkey), unexpected error: %v", err)
	}
	if _, err := localAddLink(malformed, "", "NBBALL-12", "NBBALL-1", "", "tmp_1"); err == nil {
		t.Fatal("expected localAddLink to reject malformed base issueLinks")
	}
}

// TestLocalRemoveLink_RejectsWhenResultingDocInvalid — 제거 대상 id 는 매치되어
// 실제 제거가 일어나지만, 문서의 다른 필드(labels)가 여전히 위반 상태라면
// marshalIssueDoc 의 validateIssueDoc 검증에서 거부되어야 한다.
func TestLocalRemoveLink_RejectsWhenResultingDocInvalid(t *testing.T) {
	malformed := `
key: NBBALL-12
subkey: FR_001
issueType: Requirement
labels: [1, 2, 3]
issueLinks:
  - id: "tmp_1"
    outwardIssue: { key: "NBBALL-99" }
`
	_, err := localRemoveLink([]string{malformed}, "tmp_1")
	if err == nil {
		t.Fatal("expected error: removal succeeds but resulting doc still fails validateIssueDoc (malformed labels)")
	}
	if !strings.Contains(err.Error(), "labels") {
		t.Fatalf("expected labels validation error, got: %v", err)
	}
}
