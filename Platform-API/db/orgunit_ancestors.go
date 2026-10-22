package db

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// 사용자 소속(orgUnitIds)은 지정한 조직의 상위 조직까지 포함한다.
//
// 배경: 하위 조직만 지정하면 상위 조직 기준으로 거는 조회·권한 판정에서 그 사용자가
// 빠진다. 소속을 저장하는 시점에 parentOrgUnitId 체인을 따라 상위를 채워 넣어,
// 읽는 쪽마다 조상 계산을 반복하지 않게 한다.

// orgIDString — orgUnits._id 는 문자열("org_root" 등)과 ObjectID 가 섞여 있고
// users.orgUnitIds 는 항상 문자열이다. 비교·저장은 문자열로 통일한다.
func orgIDString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case bson.ObjectID:
		return t.Hex()
	case nil:
		return ""
	default:
		return ""
	}
}

// toStringSlice — bson 에서 읽은 배열을 문자열 슬라이스로. 문자열이 아닌 원소는 버린다.
func toStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case bson.A:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// ExpandOrgAncestors — ids 에 각 조직의 상위 조직(parentOrgUnitId 체인)을 더해 돌려준다.
//
// - 입력 순서를 유지하고 중복을 제거한다.
// - 상위는 orgs 에 실제로 존재하는 조직만 추가한다(끊긴 체인에서 중단).
// - 순환 참조(A→B→A)는 같은 조직을 다시 만나는 지점에서 중단한다.
// - ids 자체는 orgs 에 없더라도 보존한다 — 사용자가 지정한 값을 조용히 버리지 않는다.
func ExpandOrgAncestors(ids []string, orgs []bson.M) []string {
	parent := make(map[string]string, len(orgs))
	exists := make(map[string]bool, len(orgs))
	for _, o := range orgs {
		id := orgIDString(o["_id"])
		if id == "" {
			continue
		}
		exists[id] = true
		parent[id] = orgIDString(o["parentOrgUnitId"])
	}

	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}

	for _, id := range ids {
		add(id)
		walked := map[string]bool{id: true}
		for cur := parent[id]; cur != ""; cur = parent[cur] {
			if walked[cur] || !exists[cur] {
				break
			}
			walked[cur] = true
			add(cur)
		}
	}
	return out
}

// WithOrgAncestors — 저장할 문서의 orgUnitIds 를 상위 조직까지 포함하도록 확장한다.
//
// orgUnitIds 키가 없으면 아무것도 하지 않는다 — 소속을 건드리지 않는 부분 수정에서
// 기존 소속을 덮어쓰면 안 되기 때문이다. 조직 목록 조회에 실패하면 확장만 생략하고
// 원본을 그대로 둔다(소속 지정 자체를 막지 않는다).
func WithOrgAncestors(ctx context.Context, doc bson.M) {
	raw, ok := doc["orgUnitIds"]
	if !ok {
		return
	}
	ids := toStringSlice(raw)
	if len(ids) == 0 {
		return
	}
	orgs, err := FindAllOrgUnits(ctx)
	if err != nil {
		log.Printf("WithOrgAncestors: 조직 목록 조회 실패 — 상위 조직 확장 생략: %v", err)
		return
	}
	doc["orgUnitIds"] = ExpandOrgAncestors(ids, orgs)
}
