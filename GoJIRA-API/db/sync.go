package db

import (
	"context"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/jiraclient"
)

// SyncIssueToJira syncs a single issue to JIRA asynchronously.
// changedFields contains only the fields that were actually modified.
// If nil, syncs links only (for link create/delete operations).
func SyncIssueToJira(key string, changedFields ...map[string]interface{}) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		log.Printf("sync: starting sync for %s", key)

		// Sync changed fields to JIRA
		if len(changedFields) > 0 && changedFields[0] != nil {
			fields := changedFields[0]
			jiraFields := CacheToJiraFields(fields)
			if len(jiraFields) > 0 {
				if err := jiraclient.UpdateIssue(key, jiraFields); err != nil {
					log.Printf("sync: update %s failed: %v", key, err)
				} else {
					log.Printf("sync: updated %s (%d fields)", key, len(jiraFields))
				}
			}
		}

		// Sync links if issue exists in DB
		issue, err := FindIssueByKey(ctx, key)
		if err == nil && issue != nil {
			syncNewLinks(ctx, issue)
			syncDeletedLinks(ctx, key, issue)
		}

		ClearDirtyFlag(ctx, key)
		log.Printf("sync: completed sync for %s", key)
	}()
}

// syncDeletedLinks processes _deletedLinks and deletes them from JIRA.
// tmp_ prefixed IDs are skipped (never existed in JIRA).
func syncDeletedLinks(ctx context.Context, key string, issue bson.M) {
	var ids []interface{}
	switch v := issue["_deletedLinks"].(type) {
	case bson.A:
		ids = []interface{}(v)
	case []interface{}:
		ids = v
	default:
		return
	}

	for _, raw := range ids {
		linkID, _ := raw.(string)
		if linkID == "" || strings.HasPrefix(linkID, "tmp_") {
			continue
		}
		if err := jiraclient.DeleteIssueLink(linkID); err != nil {
			log.Printf("sync: delete link %s from JIRA failed: %v", linkID, err)
		} else {
			log.Printf("sync: deleted link %s from JIRA (issue %s)", linkID, key)
		}
	}

	// Clear _deletedLinks array
	Issues().UpdateOne(ctx, bson.M{"key": key}, bson.M{
		"$set": bson.M{"_deletedLinks": bson.A{}},
	})
}

// syncNewLinks finds tmp_ prefixed links in an issue and creates them in JIRA.
// issueLinks are stored at the top level of the MongoDB document (not inside fields).
func syncNewLinks(ctx context.Context, issue bson.M) {
	key, _ := issue["key"].(string)
	rawLinks := issue["issueLinks"]
	if rawLinks == nil {
		return
	}

	var links []interface{}
	switch v := rawLinks.(type) {
	case bson.A:
		links = []interface{}(v)
	case []interface{}:
		links = v
	default:
		log.Printf("sync: %s issueLinks unexpected type %T", key, rawLinks)
		return
	}

	created := false
	for _, raw := range links {
		m := toBsonM(raw)
		if m == nil {
			continue
		}

		linkID, _ := m["id"].(string)
		if !strings.HasPrefix(linkID, "tmp_") {
			continue
		}

		// Only process from the inward side (has outwardIssue) to avoid duplicate
		outwardM := toBsonM(m["outwardIssue"])
		if outwardM == nil {
			continue
		}
		outwardKey, _ := outwardM["key"].(string)
		if key == "" || outwardKey == "" {
			continue
		}

		if err := jiraclient.CreateIssueLink(key, outwardKey); err != nil {
			log.Printf("sync: create link %s->%s failed: %v", key, outwardKey, err)
		} else {
			log.Printf("sync: created link %s->%s in JIRA", key, outwardKey)
			created = true
		}
	}

	// After creating links in JIRA, re-fetch the issue to get real link IDs
	if created {
		refreshIssueLinks(ctx, key)
	}
}

// refreshIssueLinks fetches issuelinks from JIRA and updates MongoDB with real IDs.
func refreshIssueLinks(ctx context.Context, key string) {
	jiraIssue, err := jiraclient.GetIssue(key)
	if err != nil {
		log.Printf("sync: refresh links for %s failed: %v", key, err)
		return
	}
	fields, _ := jiraIssue["fields"].(map[string]interface{})
	if fields == nil {
		return
	}
	newLinks := transformLinks(fields["issuelinks"])
	if newLinks == nil {
		newLinks = []interface{}{}
	}
	Issues().UpdateOne(ctx, bson.M{"key": key}, bson.M{
		"$set": bson.M{"issueLinks": newLinks},
	})
	log.Printf("sync: refreshed links for %s (%d links)", key, len(newLinks))
}

// toBsonM converts various BSON types to bson.M for uniform access.
func toBsonM(v interface{}) bson.M {
	switch t := v.(type) {
	case bson.M:
		return t
	case bson.D:
		m := bson.M{}
		for _, e := range t {
			m[e.Key] = e.Value
		}
		return m
	case map[string]interface{}:
		return bson.M(t)
	}
	return nil
}

// PullFromJiraFiltered fetches issues from JIRA with optional project and issuetype filter,
// transforms to cache_schema format, and upserts into MongoDB.
// PullFromJiraFiltered pulls issues from JIRA and returns (count, pulledKeys, error)
func PullFromJiraFiltered(ctx context.Context, projectKey string, issueTypes []string) (int, []string, error) {
	var result map[string]interface{}
	var err error

	// 이슈는 projectId 로 단일 소속. projectKey 가 주어지면 _id 확보해서 함께 저장.
	var projectID interface{}
	if projectKey != "" {
		if prj := FindProjectByCode(ctx, projectKey); prj != nil {
			projectID = prj["_id"]
		}
	}

	if projectKey != "" || len(issueTypes) > 0 {
		// Build JQL
		clauses := []string{}
		if projectKey != "" {
			clauses = append(clauses, "project = "+projectKey)
		}
		if len(issueTypes) > 0 {
			quoted := make([]string, len(issueTypes))
			for i, t := range issueTypes {
				quoted[i] = "\"" + t + "\""
			}
			clauses = append(clauses, "issuetype in ("+joinStrings(quoted, ",")+")")
		}
		jql := joinStrings(clauses, " AND ")
		result, err = jiraclient.SearchIssues(jql, 0, 10000)
	} else {
		result, err = jiraclient.ListIssues(0, 10000)
	}
	if err != nil {
		return 0, nil, err
	}

	issues, _ := result["issues"].([]interface{})
	upserted := 0
	var pulledKeys []string

	for _, raw := range issues {
		issue, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		key, _ := issue["key"].(string)
		if key == "" {
			continue
		}

		// 프로젝트 키가 지정된 경우, 해당 프로젝트 이슈만 처리
		if projectKey != "" && !strings.HasPrefix(key, projectKey+"-") {
			continue
		}

		// Transform to cache_schema format
		doc := TransformToCache(issue)
		if projectID != nil {
			doc["projectId"] = projectID
		}
		doc["_dirty"] = false
		doc["_syncedAt"] = time.Now()

		if err := UpsertIssue(ctx, key, doc); err != nil {
			log.Printf("pull: upsert %s failed: %v", key, err)
			continue
		}
		upserted++
		pulledKeys = append(pulledKeys, key)
	}

	return upserted, pulledKeys, nil
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

