package db

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// 재활성화는 생성 이력과 발급된 Gitea 토큰을 보존해야 한다 — 새로 발급하면
// 과거 커밋·PR 이력과의 연결이 끊긴다.
func TestReactivateFieldsPreservesIdentityAndToken(t *testing.T) {
	oid := bson.NewObjectID()
	created := time.Now().Add(-24 * time.Hour)
	existing := bson.M{
		"_id":        oid,
		"knoxId":     "hong.gildong",
		"createdAt":  created,
		"createdBy":  "admin",
		"giteaToken": "oldtoken",
		"isActive":   false,
	}
	// 입력값이 보존 대상 필드를 덮으려 해도 무시돼야 한다.
	doc := bson.M{
		"knoxId":     "hong.gildong",
		"name":       "홍길동",
		"email":      "hong@example.com",
		"_id":        bson.NewObjectID(),
		"createdAt":  time.Now(),
		"createdBy":  "someone-else",
		"giteaToken": "newtoken",
	}

	updates := ReactivateFields(doc, existing)

	for _, k := range []string{"_id", "createdAt", "createdBy", "giteaToken"} {
		if _, ok := updates[k]; ok {
			t.Errorf("보존 대상 %q 가 updates 에 포함됐다 — 덮어쓰면 안 된다", k)
		}
	}
	if updates["name"] != "홍길동" || updates["email"] != "hong@example.com" {
		t.Errorf("입력값이 반영되지 않았다: %v", updates)
	}
	if updates["isActive"] != true {
		t.Errorf("isActive = %v, want true — 재활성화되어야 한다", updates["isActive"])
	}
	if _, ok := updates["updatedAt"]; !ok {
		t.Error("updatedAt 이 설정되지 않았다")
	}
}

// 보존 대상이 입력에 아예 없는 경우에도 동작해야 한다.
func TestReactivateFieldsWithoutPreservedKeysInInput(t *testing.T) {
	existing := bson.M{"_id": "u1", "giteaToken": "tok", "isActive": false}
	updates := ReactivateFields(bson.M{"name": "홍길동"}, existing)

	if updates["name"] != "홍길동" {
		t.Errorf("name 미반영: %v", updates)
	}
	if updates["isActive"] != true {
		t.Errorf("isActive = %v, want true", updates["isActive"])
	}
	if _, ok := updates["giteaToken"]; ok {
		t.Error("giteaToken 이 updates 에 들어갔다")
	}
}
