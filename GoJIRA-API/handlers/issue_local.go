package handlers

// issue_local.go — Phase 2 로컬-우선 쓰기 (web-local-view-model.md §Phase 2).
//
// 배경: 공식본(Mongo)은 PR 머지(main push) 시점에만 갱신하고, 로컬 편집은 로컬
// clone 의 data/issues/*.yaml 만 갱신하는 모델로 전환한다. 클라이언트가 자기
// 로컬 YAML(진실)을 요청에 동봉하면(BaseYaml/InwardYaml·OutwardYaml/BaseFiles),
// 이 파일의 헬퍼들은 Mongo 에 전혀 접촉하지 않고 병합/링크 로직만 순수 함수로
// 수행해 localWrites 를 만들어 낸다 — 호출자(핸들러)가 즉시 응답하고 기존
// Mongo 기반 코드로 내려가지 않는다.
//
// 동봉 필드가 없는 구버전 요청은 각 핸들러의 기존 Mongo 경로를 그대로 탄다
// (전환기 호환 — 기존 동작을 깨지 않는다).
//
// 모든 헬퍼는 최종 doc 에 validateIssueDoc 을 적용해 위반 시 에러를 반환한다
// (오염 원천 차단 — NBBALL 사고 재발 방지, bson_normalize.go/issue_validate.go 참고).

import (
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"
)

const issueLinkTypeContains = "Contains(WBSGantt)"

var (
	// errLocalKeyMismatch — BaseYaml 의 key 가 요청의 key 와 다름 (400).
	errLocalKeyMismatch = errors.New("yaml key does not match request key")
	// errLocalNoSubkey — YAML doc 에 subkey 가 없어 파일 경로를 도출할 수 없음 (409).
	errLocalNoSubkey = errors.New("yaml has no subkey")
	// errLocalNoYamlGiven — link 추가 요청에 inward/outward 양쪽 다 YAML 이 없음 (400).
	errLocalNoYamlGiven = errors.New("neither inwardYaml nor outwardYaml given")
)

func nowTimestamp() string {
	return time.Now().Format("2006-01-02T15:04:05.000+0900")
}

// marshalIssueDoc — doc 을 정규화 + validateIssueDoc 검증 후 YAML 로 직렬화한다.
func marshalIssueDoc(doc bson.M) ([]byte, error) {
	if err := validateIssueDoc(doc); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	yamlBytes, err := yaml.Marshal(normalizeBsonValue(doc))
	if err != nil {
		return nil, fmt.Errorf("yaml marshal: %w", err)
	}
	return yamlBytes, nil
}

// localMergeUpdate — BaseYaml 을 파싱해 req.Data 를 단순 필드 덮어쓰기로 병합하고
// updated 를 갱신한다. Mongo 무접촉. key 불일치·subkey 없음은 에러로 반환한다.
func localMergeUpdate(baseYaml, reqKey string, data map[string]interface{}) (localWrite, error) {
	doc, docKey, err := parseIssueYAML(baseYaml)
	if err != nil {
		return localWrite{}, err
	}
	if docKey != reqKey {
		return localWrite{}, errLocalKeyMismatch
	}
	subkey, _ := doc["subkey"].(string)
	if subkey == "" {
		return localWrite{}, errLocalNoSubkey
	}

	for k, v := range data {
		doc[k] = v
	}
	doc["updated"] = nowTimestamp()

	yamlBytes, err := marshalIssueDoc(doc)
	if err != nil {
		return localWrite{}, err
	}
	return localWrite{Path: issueYamlPath(subkey), Content: string(yamlBytes)}, nil
}

// appendIssueLink — doc["issueLinks"] 가 없으면 새 배열을 만들고 link 를 append 한다.
func appendIssueLink(doc bson.M, link bson.M) {
	existing, _ := asSlice(doc["issueLinks"])
	existing = append(existing, link)
	doc["issueLinks"] = existing
}

// localAddLinkSide — 한쪽(inward 또는 outward) YAML 에 링크 엔트리를 append 하고
// localWrite 로 반환한다. yamlContent 가 비어있으면 (nil, nil) 을 반환해 건너뛴다.
func localAddLinkSide(yamlContent string, link bson.M) (*localWrite, error) {
	if yamlContent == "" {
		return nil, nil
	}
	doc, _, err := parseIssueYAML(yamlContent)
	if err != nil {
		return nil, err
	}
	subkey, _ := doc["subkey"].(string)
	if subkey == "" {
		return nil, errLocalNoSubkey
	}
	appendIssueLink(doc, link)
	doc["updated"] = nowTimestamp()

	yamlBytes, err := marshalIssueDoc(doc)
	if err != nil {
		return nil, err
	}
	return &localWrite{Path: issueYamlPath(subkey), Content: string(yamlBytes)}, nil
}

// localAddLink — inward/outward 양쪽(또는 한쪽) YAML 에 Contains(WBSGantt) 링크
// 엔트리를 append 한다. Mongo 무접촉, db.SyncIssueToJira 호출 없음.
func localAddLink(inwardYaml, outwardYaml, inwardKey, outwardKey, source, linkID string) ([]localWrite, error) {
	if inwardYaml == "" && outwardYaml == "" {
		return nil, errLocalNoYamlGiven
	}

	var writes []localWrite

	inwardLink := bson.M{
		"id":           linkID,
		"type":         bson.M{"name": issueLinkTypeContains},
		"outwardIssue": bson.M{"key": outwardKey},
	}
	if source != "" {
		inwardLink["_addedBy"] = source
	}
	lw, err := localAddLinkSide(inwardYaml, inwardLink)
	if err != nil {
		return nil, fmt.Errorf("inward yaml: %w", err)
	}
	if lw != nil {
		writes = append(writes, *lw)
	}

	outwardLink := bson.M{
		"id":          linkID,
		"type":        bson.M{"name": issueLinkTypeContains},
		"inwardIssue": bson.M{"key": inwardKey},
	}
	if source != "" {
		outwardLink["_addedBy"] = source
	}
	lw, err = localAddLinkSide(outwardYaml, outwardLink)
	if err != nil {
		return nil, fmt.Errorf("outward yaml: %w", err)
	}
	if lw != nil {
		writes = append(writes, *lw)
	}

	return writes, nil
}

// localRemoveLinkFrom — 단일 YAML 문서에서 issueLinks 중 id==linkID 항목을 제거한다.
// 실제 제거가 발생한 경우에만 (localWrite, true) 를 반환하고, 해당 없음(미포함)이면
// (localWrite{}, false, nil) 을 반환해 localWrites 에서 제외한다.
func localRemoveLinkFrom(yamlContent, linkID string) (localWrite, bool, error) {
	doc, _, err := parseIssueYAML(yamlContent)
	if err != nil {
		return localWrite{}, false, err
	}
	subkey, _ := doc["subkey"].(string)
	if subkey == "" {
		return localWrite{}, false, errLocalNoSubkey
	}

	links, ok := asSlice(doc["issueLinks"])
	if !ok {
		return localWrite{}, false, nil
	}

	removed := false
	newLinks := make([]interface{}, 0, len(links))
	for _, item := range links {
		if m, ok := asMap(item); ok {
			if id, _ := m["id"].(string); id == linkID {
				removed = true
				continue
			}
		}
		newLinks = append(newLinks, item)
	}
	if !removed {
		return localWrite{}, false, nil
	}

	doc["issueLinks"] = newLinks
	doc["updated"] = nowTimestamp()

	yamlBytes, err := marshalIssueDoc(doc)
	if err != nil {
		return localWrite{}, false, err
	}
	return localWrite{Path: issueYamlPath(subkey), Content: string(yamlBytes)}, true, nil
}

// localRemoveLink — baseFiles 각각에서 linkID 링크 엔트리를 제거하고, 실제 제거가
// 일어난 파일만 localWrites 로 모은다. Mongo 무접촉, db.SyncIssueToJira 호출 없음.
func localRemoveLink(baseFiles []string, linkID string) ([]localWrite, error) {
	var writes []localWrite
	for _, y := range baseFiles {
		lw, removed, err := localRemoveLinkFrom(y, linkID)
		if err != nil {
			return nil, err
		}
		if removed {
			writes = append(writes, lw)
		}
	}
	return writes, nil
}

// localDeleteIssue — BaseYaml 을 파싱해 subkey 를 추출하고 delete:true localWrite 를
// 반환한다. Mongo 무접촉(MoveIssueToTrash 등 호출 없음) — 공식본 삭제는 머지
// reconcile 이 수행한다.
func localDeleteIssue(baseYaml string) (localWrite, error) {
	doc, _, err := parseIssueYAML(baseYaml)
	if err != nil {
		return localWrite{}, err
	}
	subkey, _ := doc["subkey"].(string)
	if subkey == "" {
		return localWrite{}, errLocalNoSubkey
	}
	return localWrite{Path: issueYamlPath(subkey), Delete: true}, nil
}

// localWriteStatus — 로컬 헬퍼 에러를 HTTP status code 로 매핑한다.
// errLocalNoSubkey 는 409(Conflict), 그 외(파싱 실패/key 불일치/validate 실패/
// YAML 없음)는 400(Bad Request).
func localWriteStatus(err error) int {
	if errors.Is(err, errLocalNoSubkey) {
		return 409
	}
	return 400
}
