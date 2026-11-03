package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// toID 문자열 ID를 ObjectId 또는 문자열로 변환하여 MongoDB 쿼리에 사용
func toID(id string) interface{} {
	oid, err := bson.ObjectIDFromHex(id)
	if err == nil {
		return oid
	}
	return id
}

// --- 앱 스코프 (D8, D9) ---
// authGroups/notices 는 app 필드로 제품별 분리 관리한다 ('platform'|'gojira'|'ragaas'…).
// 'platform' 은 app 필드가 없는 레거시 문서(마이그레이션 전 데이터)도 포함해 하위호환.
// 'all' 은 platform 관리 화면(메타 관리 콘솔) 전용 특수값 — 필터 없이 전체 조회한다(D9).
func appFilter(app string) bson.M {
	if app == "all" {
		return bson.M{}
	}
	if app == "" || app == "platform" {
		return bson.M{"$or": []bson.M{{"app": "platform"}, {"app": bson.M{"$exists": false}}}}
	}
	return bson.M{"app": app}
}

// NormalizeApp 빈 app 파라미터를 기본 스코프로 정규화
func NormalizeApp(app string) string {
	if app == "" {
		return "platform"
	}
	return app
}

// --- 컬렉션 접근 ---

func Users() *mongo.Collection       { return database.Collection("users") }
func AuthGroups() *mongo.Collection   { return database.Collection("authGroups") }
func OrgUnits() *mongo.Collection     { return database.Collection("orgUnits") }
func UserProjects() *mongo.Collection { return database.Collection("userProjects") }
func UserConnections() *mongo.Collection {
	return database.Collection("userConnections")
}

// --- 인덱스 ---

func InitAccountIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createUnique := func(col *mongo.Collection, field string) {
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: field, Value: 1}},
			Options: options.Index().SetUnique(true),
		})
		if err != nil {
			log.Printf("index %s.%s: %v", col.Name(), field, err)
		}
	}

	createUnique(Users(), "knoxId")

	// authGroups: 구 code 단독 unique 를 (app, code) 복합 unique 로 교체 (D8 앱 스코핑)
	_ = AuthGroups().Indexes().DropOne(ctx, "code_1")
	_, errAG := AuthGroups().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "app", Value: 1}, {Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if errAG != nil {
		log.Printf("index authGroups compound (app,code): %v", errAG)
	}

	// orgUnits: 옛 code unique index 정리 (있으면 drop, 없으면 무시)
	_ = OrgUnits().Indexes().DropOne(ctx, "code_1")

	// orgUnits: name + type 복합 유니크 (code 폐지)
	_, err0 := OrgUnits().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}, {Key: "type", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err0 != nil {
		log.Printf("index orgUnits compound (name,type): %v", err0)
	}

	// userProjects: userId + projectId 복합 유니크
	_, err := UserProjects().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "projectId", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Printf("index userProjects compound: %v", err)
	}

	// userConnections: userId 인덱스
	_, err = UserConnections().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "userId", Value: 1}},
	})
	if err != nil {
		log.Printf("index userConnections.userId: %v", err)
	}

	log.Println("account indexes initialized")
}

// --- 공통 CRUD ---

func findAll(ctx context.Context, col *mongo.Collection, filter bson.M) ([]bson.M, error) {
	if filter == nil {
		filter = bson.M{}
	}
	filter["isActive"] = true
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func findByID(ctx context.Context, col *mongo.Collection, id string) (bson.M, error) {
	var result bson.M
	err := col.FindOne(ctx, bson.M{"_id": toID(id)}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func insertDoc(ctx context.Context, col *mongo.Collection, doc bson.M) error {
	now := time.Now()
	doc["isActive"] = true
	doc["createdAt"] = now
	doc["updatedAt"] = now
	if doc["createdBy"] == nil {
		doc["createdBy"] = "system"
	}
	if doc["updatedBy"] == nil {
		doc["updatedBy"] = doc["createdBy"]
	}
	result, err := col.InsertOne(ctx, doc)
	if err == nil && result.InsertedID != nil {
		doc["_id"] = result.InsertedID
	}
	return err
}

func updateDoc(ctx context.Context, col *mongo.Collection, id string, updates bson.M) error {
	updates["updatedAt"] = time.Now()
	_, err := col.UpdateOne(ctx, bson.M{"_id": toID(id)}, bson.M{"$set": updates})
	return err
}

func softDelete(ctx context.Context, col *mongo.Collection, id string, by string) error {
	_, err := col.UpdateOne(ctx, bson.M{"_id": toID(id)}, bson.M{"$set": bson.M{
		"isActive":  false,
		"updatedAt": time.Now(),
		"updatedBy": by,
	}})
	return err
}

// --- Users ---

// FindAllUsers 앱 스코프의 사용자 목록 — 신원은 전역이지만 "그 앱의 사용자"는
// 그 앱의 authGroup 에 하나라도 속한 사용자로 정의한다 (D8: 전역 신원 + 앱별 권한).
// app == "all" (D9, platform 관리 화면 전용) 이면 전체 사용자를 그대로 반환한다.
func FindAllUsers(ctx context.Context, app string) ([]bson.M, error) {
	if app == "all" {
		return findAll(ctx, Users(), nil)
	}
	norm := NormalizeApp(app)
	groups, err := FindAllAuthGroups(ctx, norm)
	if err != nil {
		return nil, err
	}
	ids := make([]interface{}, 0, len(groups)*2)
	for _, g := range groups {
		ids = append(ids, g["_id"])
		if oid, ok := g["_id"].(bson.ObjectID); ok {
			ids = append(ids, oid.Hex()) // 링크가 hex 문자열로 저장된 경우 대응
		}
	}
	if len(ids) > 0 {
		return findAll(ctx, Users(), bson.M{"authGroupIds": bson.M{"$in": ids}})
	}
	// 이 앱에 소속된 사용자가 아직 한 명도 없다 — 빈 배열 반환 (D9: platform 전체 조회
	// 모델에서는 platform 화면에서 그 앱의 authGroup 을 만들고 사용자를 직접 배정하므로
	// 온보딩 후보 폴백이 불필요해졌다).
	return []bson.M{}, nil
}

func FindUserByID(ctx context.Context, id string) (bson.M, error) {
	return findByID(ctx, Users(), id)
}

func FindUserByKnoxID(ctx context.Context, knoxID string) (bson.M, error) {
	var result bson.M
	err := Users().FindOne(ctx, bson.M{"knoxId": knoxID, "isActive": true}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateUserGiteaToken knoxId로 사용자의 giteaToken 필드 업데이트
func UpdateUserGiteaToken(ctx context.Context, knoxID, token string) error {
	_, err := Users().UpdateOne(ctx, bson.M{"knoxId": knoxID}, bson.M{"$set": bson.M{"giteaToken": token}})
	return err
}

// GetUserGiteaToken knoxId로 사용자의 giteaToken 조회
func GetUserGiteaToken(ctx context.Context, knoxID string) string {
	user, err := FindUserByKnoxID(ctx, knoxID)
	if err != nil {
		return ""
	}
	token, _ := user["giteaToken"].(string)
	return token
}

// KnoxIdExists Knox ID 중복 여부 확인
func KnoxIdExists(ctx context.Context, knoxId string) (bool, error) {
	count, err := Users().CountDocuments(ctx, bson.M{"knoxId": knoxId, "isActive": true})
	return count > 0, err
}

// ActiveKnoxIDs 주어진 knoxId 중 활성 사용자로 등록된 것들을 한 번에 조회한다.
// 동명이인 목록처럼 여러 건의 등록 여부를 표시할 때 건별 조회를 피하기 위한 것.
func ActiveKnoxIDs(ctx context.Context, knoxIDs []string) map[string]bool {
	found := make(map[string]bool, len(knoxIDs))
	if len(knoxIDs) == 0 {
		return found
	}
	cursor, err := Users().Find(ctx,
		bson.M{"knoxId": bson.M{"$in": knoxIDs}, "isActive": true},
		options.Find().SetProjection(bson.M{"knoxId": 1}))
	if err != nil {
		log.Printf("ActiveKnoxIDs 조회 실패: %v", err)
		return found
	}
	var rows []bson.M
	if err := cursor.All(ctx, &rows); err != nil {
		log.Printf("ActiveKnoxIDs 디코드 실패: %v", err)
		return found
	}
	for _, row := range rows {
		if id, ok := row["knoxId"].(string); ok {
			found[id] = true
		}
	}
	return found
}

// EmailExists 이메일 중복 여부 확인
func EmailExists(ctx context.Context, email string) (bool, error) {
	count, err := Users().CountDocuments(ctx, bson.M{"email": email, "isActive": true})
	return count > 0, err
}

// EmailExistsExcept 특정 사용자를 제외하고 이메일 중복 여부 확인
func EmailExistsExcept(ctx context.Context, email, excludeID string) (bool, error) {
	count, err := Users().CountDocuments(ctx, bson.M{"email": email, "isActive": true, "_id": bson.M{"$ne": toID(excludeID)}})
	return count > 0, err
}

func InsertUser(ctx context.Context, doc bson.M) error {
	return insertDoc(ctx, Users(), doc)
}

func UpdateUser(ctx context.Context, id string, updates bson.M) error {
	return updateDoc(ctx, Users(), id, updates)
}

func SoftDeleteUser(ctx context.Context, id, by string) error {
	return softDelete(ctx, Users(), id, by)
}

// --- AuthGroups ---

func FindAllAuthGroups(ctx context.Context, app string) ([]bson.M, error) {
	return findAll(ctx, AuthGroups(), appFilter(app))
}

func InsertAuthGroup(ctx context.Context, doc bson.M) error {
	return insertDoc(ctx, AuthGroups(), doc)
}

func UpdateAuthGroup(ctx context.Context, id string, updates bson.M) error {
	return updateDoc(ctx, AuthGroups(), id, updates)
}

func SoftDeleteAuthGroup(ctx context.Context, id, by string) error {
	return softDelete(ctx, AuthGroups(), id, by)
}

// --- OrgUnits ---

func FindAllOrgUnits(ctx context.Context) ([]bson.M, error) {
	return findAll(ctx, OrgUnits(), nil)
}

func InsertOrgUnit(ctx context.Context, doc bson.M) error {
	return insertDoc(ctx, OrgUnits(), doc)
}

func UpdateOrgUnit(ctx context.Context, id string, updates bson.M) error {
	return updateDoc(ctx, OrgUnits(), id, updates)
}

func SoftDeleteOrgUnit(ctx context.Context, id, by string) error {
	return softDelete(ctx, OrgUnits(), id, by)
}

// --- UserProjects ---

func FindUserProjectsByUser(ctx context.Context, userID string) ([]bson.M, error) {
	return findAll(ctx, UserProjects(), bson.M{"userId": userID})
}

func InsertUserProject(ctx context.Context, doc bson.M) error {
	return insertDoc(ctx, UserProjects(), doc)
}

func SoftDeleteUserProject(ctx context.Context, id, by string) error {
	return softDelete(ctx, UserProjects(), id, by)
}

// --- UserConnections ---

func FindConnectionsByUser(ctx context.Context, userID string) ([]bson.M, error) {
	cursor, err := UserConnections().Find(ctx, bson.M{"userId": userID, "isOnline": true})
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func InsertConnection(ctx context.Context, doc bson.M) error {
	return insertDoc(ctx, UserConnections(), doc)
}

func UpdateConnection(ctx context.Context, id string, updates bson.M) error {
	return updateDoc(ctx, UserConnections(), id, updates)
}

// --- 권한 계산 ---

// CalcUserAuthority 는 사용자의 authGroupIds 중 해당 앱 스코프의 authGroups 만 조회하여
// authority 를 OR 연산한다 (D8: 같은 사용자가 앱마다 다른 권한을 가진다).
func CalcUserAuthority(ctx context.Context, userID string, app string) (map[string]int, error) {
	app = NormalizeApp(app)
	user, err := FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var groupIDs []interface{}
	switch v := user["authGroupIds"].(type) {
	case bson.A:
		groupIDs = []interface{}(v)
	case []interface{}:
		groupIDs = v
	case []string:
		for _, s := range v {
			groupIDs = append(groupIDs, s)
		}
	}

	if len(groupIDs) == 0 {
		return map[string]int{}, nil
	}

	// ObjectId/문자열 혼합 대응
	convertedIDs := make([]interface{}, len(groupIDs))
	for i, id := range groupIDs {
		if s, ok := id.(string); ok {
			convertedIDs[i] = toID(s)
		} else {
			convertedIDs[i] = id
		}
	}

	groupFilter := appFilter(app)
	groupFilter["_id"] = bson.M{"$in": convertedIDs}
	groupFilter["isActive"] = true
	cursor, err := AuthGroups().Find(ctx, groupFilter)
	if err != nil {
		return nil, err
	}
	var groups []bson.M
	if err := cursor.All(ctx, &groups); err != nil {
		return nil, err
	}

	// OR 연산으로 권한 합산
	merged := map[string]int{}
	for _, g := range groups {
		// authority 필드: bson.M 또는 bson.D 타입
		var authMap map[string]interface{}
		switch a := g["authority"].(type) {
		case bson.M:
			authMap = map[string]interface{}(a)
		case bson.D:
			authMap = make(map[string]interface{})
			for _, e := range a {
				authMap[e.Key] = e.Value
			}
		default:
			continue
		}
		for code, val := range authMap {
			var bits int
			switch v := val.(type) {
			case int32:
				bits = int(v)
			case int64:
				bits = int(v)
			case float64:
				bits = int(v)
			}
			merged[code] = merged[code] | bits
		}
	}

	return merged, nil
}
