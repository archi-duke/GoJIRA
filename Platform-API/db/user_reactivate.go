package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// 삭제된 사용자의 재등록.
//
// 사용자 삭제는 isActive=false 인 soft delete 인데 users.knoxId 에 unique 인덱스가
// 걸려 있다. 따라서 같은 knoxId 를 다시 등록할 때 insert 하면 E11000(duplicate key)로
// 실패한다. 비활성 레코드를 입력값으로 갱신하며 되살리는 방식으로 처리한다.

// reactivatePreserved — 재활성화 시 입력값으로 덮지 않고 보존할 필드.
// 생성 이력(_id/createdAt/createdBy)과 이미 발급된 Gitea 토큰은 유지해야
// 기존 clone·PR 이력과의 연결이 끊기지 않는다.
var reactivatePreserved = []string{"_id", "createdAt", "createdBy", "giteaToken"}

// FindInactiveUserByKnoxID — 같은 knoxId 의 비활성(삭제된) 사용자. 없으면 nil.
func FindInactiveUserByKnoxID(ctx context.Context, knoxID string) bson.M {
	var result bson.M
	err := Users().FindOne(ctx, bson.M{"knoxId": knoxID, "isActive": false}).Decode(&result)
	if err != nil {
		return nil
	}
	return result
}

// ReactivateFields — 비활성 레코드(existing)에 입력값(doc)을 입힐 때 실제로 $set 할
// 필드를 만든다. 보존 대상은 doc 에 있더라도 제외하고, isActive 를 true 로 되돌린다.
func ReactivateFields(doc, existing bson.M) bson.M {
	preserved := make(map[string]bool, len(reactivatePreserved))
	for _, k := range reactivatePreserved {
		preserved[k] = true
	}

	updates := bson.M{}
	for k, v := range doc {
		if preserved[k] {
			continue
		}
		updates[k] = v
	}
	updates["isActive"] = true
	updates["updatedAt"] = time.Now()
	return updates
}

// ReactivateUser — 비활성 레코드를 입력값으로 갱신하며 되살린다.
func ReactivateUser(ctx context.Context, id interface{}, updates bson.M) error {
	_, err := Users().UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": updates})
	return err
}
