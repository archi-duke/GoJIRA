package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SeedAccountData 초기 데이터 삽입 (이미 존재하면 skip)
func SeedAccountData() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// AuthGroups
	seedAuthGroups := []bson.M{
		{
			"_id": "ag_master", "code": "MASTER", "name": "Master",
			"description": "전체 관리 권한",
			"authority":   bson.M{"ALLPRJ": 15, "USR": 15, "ORG": 15, "PRJ": 15, "NOTICE": 15},
		},
		{
			"_id": "ag_editor", "code": "EDITOR", "name": "Editor",
			"description": "조회 + 수정 권한",
			"authority":   bson.M{"ALLPRJ": 6, "USR": 4, "ORG": 4, "PRJ": 6},
		},
		{
			"_id": "ag_viewer", "code": "VIEWER", "name": "Viewer",
			"description": "조회 전용",
			"authority":   bson.M{"ALLPRJ": 4, "USR": 4, "ORG": 4, "PRJ": 4},
		},
	}
	for _, ag := range seedAuthGroups {
		count, _ := AuthGroups().CountDocuments(ctx, bson.M{"_id": ag["_id"]})
		if count == 0 {
			insertDoc(ctx, AuthGroups(), ag)
			log.Printf("seed: authGroup %s created", ag["code"])
		}
	}

	// OrgUnits
	count, _ := OrgUnits().CountDocuments(ctx, bson.M{"_id": "org_root"})
	if count == 0 {
		insertDoc(ctx, OrgUnits(), bson.M{
			"_id": "org_root", "code": "ROOT", "name": "Organization",
			"type": "company", "parentOrgUnitId": "", "sortOrder": 0,
		})
		log.Println("seed: orgUnit ROOT created")
	}

	// Users
	count, _ = Users().CountDocuments(ctx, bson.M{"knoxId": "duke.kimm"})
	if count == 0 {
		insertDoc(ctx, Users(), bson.M{
			"_id": "usr_duke", "knoxId": "duke.kimm", "name": "Duke Kimm",
			"email":        "duke.kimm@example.com",
			"authGroupIds": []string{"ag_master"},
			"orgUnitIds":   []string{"org_root"},
		})
		log.Println("seed: user duke.kimm created")
	}
}
