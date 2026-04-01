package db

import (
	"context"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/jiraclient"
)

// StartAsyncSync starts a background goroutine that periodically pushes
// dirty issues to JIRA.
func StartAsyncSync(interval time.Duration) {
	go func() {
		for {
			time.Sleep(interval)
			syncDirtyToJira()
		}
	}()
	log.Printf("Async JIRA sync started (interval: %s)", interval)
}

func syncDirtyToJira() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dirtyIssues, err := FindDirtyIssues(ctx)
	if err != nil {
		log.Printf("sync: error finding dirty issues: %v", err)
		return
	}

	for _, issue := range dirtyIssues {
		key, _ := issue["key"].(string)
		if key == "" {
			continue
		}

		// Cache schema stores fields at top level (no "fields" wrapper).
		// Build fieldsMap from top-level keys for JIRA sync.
		fieldsMap := make(map[string]interface{})
		if fields, ok := issue["fields"].(bson.M); ok {
			for k, v := range fields {
				fieldsMap[k] = v
			}
		} else {
			// Flat cache schema — use top-level fields directly
			for k, v := range issue {
				if k == "_id" || k == "_dirty" || k == "_syncedAt" || k == "issueLinks" {
					continue
				}
				fieldsMap[k] = v
			}
		}

		// Sync all field changes to JIRA (including custom fields)
		if summary, ok := fieldsMap["summary"]; ok && summary != nil {
			_, err := jiraclient.GetIssue(key)
			if err != nil {
				createFields := map[string]interface{}{
					"summary": fieldsMap["summary"],
				}
				if it, ok := fieldsMap["issueType"]; ok {
					createFields["issuetype"] = map[string]interface{}{"name": it}
				}
				if desc, ok := fieldsMap["description"]; ok {
					createFields["description"] = desc
				}
				if _, err := jiraclient.CreateIssue(createFields); err != nil {
					log.Printf("sync: create %s failed: %v", key, err)
				}
			} else {
				// Convert all cache fields to JIRA field IDs
				updateFields := CacheToJiraFields(fieldsMap)
				if len(updateFields) > 0 {
					if err := jiraclient.UpdateIssue(key, updateFields); err != nil {
						log.Printf("sync: update %s failed: %v", key, err)
					}
				}
			}
		}

		// Sync new links (tmp_ prefixed) to JIRA
		syncNewLinks(ctx, issue)

		// Sync deleted links to JIRA
		syncDeletedLinks(ctx, key, issue)

		ClearDirtyFlag(ctx, key)
		log.Printf("sync: synced %s to JIRA", key)
	}
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

// PullFromJira fetches issues from JIRA and upserts into MongoDB.
// Only updates issues that are newer in JIRA (based on fields.updated).
func PullFromJira(ctx context.Context) (int, error) {
	count, _, err := PullFromJiraFiltered(ctx, "", nil)
	return count, err
}

// PullFromJiraFiltered fetches issues from JIRA with optional project and issuetype filter,
// transforms to cache_schema format, and upserts into MongoDB.
// PullFromJiraFiltered pulls issues from JIRA and returns (count, pulledKeys, error)
func PullFromJiraFiltered(ctx context.Context, projectKey string, issueTypes []string) (int, []string, error) {
	var result map[string]interface{}
	var err error

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

// PushToJira pushes only dirty (changed/added) issues from MongoDB to JIRA.
func PushToJira(ctx context.Context) (int, error) {
	dirtyIssues, err := FindDirtyIssues(ctx)
	if err != nil {
		return 0, err
	}

	pushed := 0
	for _, issue := range dirtyIssues {
		key, _ := issue["key"].(string)
		if key == "" {
			continue
		}

		// Cache schema stores fields at top level (no "fields" wrapper).
		fieldsMap := make(map[string]interface{})
		if fields, ok := issue["fields"].(bson.M); ok {
			for k, v := range fields {
				fieldsMap[k] = v
			}
		} else {
			for k, v := range issue {
				if k == "_id" || k == "_dirty" || k == "_syncedAt" || k == "issueLinks" {
					continue
				}
				fieldsMap[k] = v
			}
		}

		// Sync all field changes to JIRA (including custom fields)
		if summary, ok := fieldsMap["summary"]; ok && summary != nil {
			_, getErr := jiraclient.GetIssue(key)
			if getErr != nil {
				createFields := map[string]interface{}{
					"summary": fieldsMap["summary"],
				}
				if it, ok := fieldsMap["issueType"]; ok {
					createFields["issuetype"] = map[string]interface{}{"name": it}
				}
				if _, err := jiraclient.CreateIssue(createFields); err != nil {
					log.Printf("push: create %s failed: %v", key, err)
				}
			} else {
				// Convert all cache fields to JIRA field IDs
				updateFields := CacheToJiraFields(fieldsMap)
				if len(updateFields) > 0 {
					if err := jiraclient.UpdateIssue(key, updateFields); err != nil {
						log.Printf("push: update %s failed: %v", key, err)
					}
				}
			}
		}

		// Sync new links (tmp_ prefixed) via POST /rest/api/2/issueLink
		syncNewLinks(ctx, issue)

		// Sync deleted links to JIRA
		syncDeletedLinks(ctx, key, issue)

		ClearDirtyFlag(ctx, key)
		pushed++
	}

	return pushed, nil
}
