package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func EditSessions() *mongo.Collection {
	return database.Collection("editSessions")
}

func InitEditSessionIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// projectCode + status 복합 인덱스
	_, err := EditSessions().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "projectCode", Value: 1}, {Key: "status", Value: 1}},
	})
	if err != nil {
		log.Printf("index editSessions compound: %v", err)
	}

	// userId + projectCode 복합 유니크 (활성 세션은 하나만)
	_, err = EditSessions().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "projectCode", Value: 1}, {Key: "status", Value: 1}},
	})
	if err != nil {
		log.Printf("index editSessions user-project: %v", err)
	}

	log.Println("editSession indexes initialized")
}

// FindActiveSession 프로젝트의 활성 편집 세션 조회
func FindActiveSession(ctx context.Context, projectCode string) (bson.M, error) {
	var result bson.M
	err := EditSessions().FindOne(ctx, bson.M{
		"projectCode": projectCode,
		"status":      "active",
	}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// FindActiveSessionByUser 특정 사용자의 활성 세션 조회
func FindActiveSessionByUser(ctx context.Context, projectCode, userID string) (bson.M, error) {
	var result bson.M
	err := EditSessions().FindOne(ctx, bson.M{
		"projectCode": projectCode,
		"userId":      userID,
		"status":      "active",
	}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AcquireEditSession 편집 세션 lock 획득 시도.
//
// 같은 user 의 active 세션 정책:
//   - 같은 holderKind (예: ui ↔ ui, skill ↔ skill) : heartbeat 갱신 후 성공 (reentry 허용)
//   - 다른 holderKind (예: ui ↔ skill)              : 거부 + 기존 holder 정보 반환
//
// 다른 user 는 worktree 가 .worktrees/<userId>/ 로 분리되어 race 가 없으므로 신경 쓰지 않는다.
//
// holderKind 는 "ui" 또는 "skill". 빈 문자열은 backward compat 으로 "ui" 취급.
func AcquireEditSession(ctx context.Context, projectCode, userID, holderKind string) (acquired bool, existingHolderKind string, err error) {
	if holderKind == "" {
		holderKind = "ui"
	}

	existing, ferr := FindActiveSessionByUser(ctx, projectCode, userID)
	if ferr == nil && existing != nil {
		existingHK, _ := existing["holderKind"].(string)
		if existingHK == "" {
			existingHK = "ui" // 마이그레이션 기본값 — 기존 세션은 ui 로 간주
		}
		if existingHK != holderKind {
			return false, existingHK, nil
		}
		// 같은 holderKind 의 reentry — 하트비트 갱신
		id := existing["_id"]
		now := time.Now()
		EditSessions().UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{
			"lastHeartbeat": now,
			"updatedAt":     now,
		}})
		return true, "", nil
	}

	now := time.Now()
	_, err = EditSessions().InsertOne(ctx, bson.M{
		"projectCode":   projectCode,
		"userId":        userID,
		"holderKind":    holderKind,
		"status":        "active",
		"lastHeartbeat": now,
		"createdAt":     now,
		"updatedAt":     now,
	})
	if err != nil {
		return false, "", err
	}
	return true, "", nil
}

// UpdateHeartbeat 세션 하트비트 갱신
func UpdateHeartbeat(ctx context.Context, projectCode, userID string) error {
	now := time.Now()
	_, err := EditSessions().UpdateOne(ctx, bson.M{
		"projectCode": projectCode,
		"userId":      userID,
		"status":      "active",
	}, bson.M{"$set": bson.M{
		"lastHeartbeat": now,
		"updatedAt":     now,
	}})
	return err
}

// ReleaseEditSession 편집 세션 해제
func ReleaseEditSession(ctx context.Context, projectCode, userID string) error {
	now := time.Now()
	_, err := EditSessions().UpdateOne(ctx, bson.M{
		"projectCode": projectCode,
		"userId":      userID,
		"status":      "active",
	}, bson.M{"$set": bson.M{
		"status":    "expired",
		"updatedAt": now,
	}})
	return err
}

// ExpireStaleSessionsOpts 만료된 세션 정리 (TTL 기준)
func ExpireStaleSessions(ctx context.Context, ttl time.Duration) (int64, error) {
	cutoff := time.Now().Add(-ttl)
	result, err := EditSessions().UpdateMany(ctx, bson.M{
		"status":        "active",
		"lastHeartbeat": bson.M{"$lt": cutoff},
	}, bson.M{"$set": bson.M{
		"status":    "expired",
		"updatedAt": time.Now(),
	}})
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

// FindAllActiveSessions 모든 활성 세션 조회
func FindAllActiveSessions(ctx context.Context) ([]bson.M, error) {
	cursor, err := EditSessions().Find(ctx, bson.M{"status": "active"})
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// GetSessionTTL 설정에서 세션 TTL 조회 (기본 30분)
func GetSessionTTL() time.Duration {
	// 향후 DB/config에서 읽도록 확장 가능
	return 30 * time.Minute
}

// IsSessionExpired 세션이 TTL 기준으로 만료되었는지 확인
func IsSessionExpired(session bson.M, ttl time.Duration) bool {
	heartbeat, ok := session["lastHeartbeat"].(time.Time)
	if !ok {
		// primitive.DateTime 대응
		if dt, ok2 := session["lastHeartbeat"].(bson.DateTime); ok2 {
			heartbeat = dt.Time()
		} else {
			return true
		}
	}
	return time.Since(heartbeat) > ttl
}
