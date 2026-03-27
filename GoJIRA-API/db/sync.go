package db

import (
	"context"
	"log"
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

		fields, _ := issue["fields"].(bson.M)
		if fields == nil {
			continue
		}

		// Convert bson.M to map[string]interface{} for jiraclient
		fieldsMap := make(map[string]interface{})
		for k, v := range fields {
			fieldsMap[k] = v
		}

		// Check if this issue exists in JIRA
		_, err := jiraclient.GetIssue(key)
		if err != nil {
			// Issue doesn't exist in JIRA — create it
			createFields := map[string]interface{}{
				"summary": fieldsMap["summary"],
			}
			if it, ok := fieldsMap["issuetype"]; ok {
				createFields["issuetype"] = it
			}
			if desc, ok := fieldsMap["description"]; ok {
				createFields["description"] = desc
			}
			if _, err := jiraclient.CreateIssue(createFields); err != nil {
				log.Printf("sync: create %s failed: %v", key, err)
				continue
			}
		} else {
			// Issue exists — update it
			updateFields := make(map[string]interface{})
			if s, ok := fieldsMap["summary"]; ok {
				updateFields["summary"] = s
			}
			if d, ok := fieldsMap["description"]; ok {
				updateFields["description"] = d
			}
			if l, ok := fieldsMap["labels"]; ok {
				updateFields["labels"] = l
			}
			if len(updateFields) > 0 {
				if err := jiraclient.UpdateIssue(key, updateFields); err != nil {
					log.Printf("sync: update %s failed: %v", key, err)
					continue
				}
			}
		}

		ClearDirtyFlag(ctx, key)
		log.Printf("sync: synced %s to JIRA", key)
	}
}

// PullFromJira fetches issues from JIRA and upserts into MongoDB.
// Only updates issues that are newer in JIRA (based on fields.updated).
func PullFromJira(ctx context.Context) (int, error) {
	result, err := jiraclient.ListIssues(0, 10000)
	if err != nil {
		return 0, err
	}

	issues, _ := result["issues"].([]interface{})
	upserted := 0

	for _, raw := range issues {
		issue, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		key, _ := issue["key"].(string)
		if key == "" {
			continue
		}

		// Check if local version exists and compare updated time
		local, err := FindIssueByKey(ctx, key)
		if err == nil && local != nil {
			localFields, _ := local["fields"].(bson.M)
			remoteFields, _ := issue["fields"].(map[string]interface{})
			localUpdated, _ := localFields["updated"].(string)
			remoteUpdated, _ := remoteFields["updated"].(string)

			// Skip if local is same or newer
			if localUpdated >= remoteUpdated {
				continue
			}
		}

		doc := bson.M{
			"key":       key,
			"id":        issue["id"],
			"self":      issue["self"],
			"expand":    issue["expand"],
			"fields":    issue["fields"],
			"_dirty":    false,
			"_syncedAt": time.Now(),
		}

		if err := UpsertIssue(ctx, key, doc); err != nil {
			log.Printf("pull: upsert %s failed: %v", key, err)
			continue
		}
		upserted++
	}

	return upserted, nil
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

		fields, _ := issue["fields"].(bson.M)
		if fields == nil {
			continue
		}

		fieldsMap := make(map[string]interface{})
		for k, v := range fields {
			fieldsMap[k] = v
		}

		// Try update first, create if not found
		_, getErr := jiraclient.GetIssue(key)
		if getErr != nil {
			createFields := map[string]interface{}{
				"summary": fieldsMap["summary"],
			}
			if it, ok := fieldsMap["issuetype"]; ok {
				createFields["issuetype"] = it
			}
			if _, err := jiraclient.CreateIssue(createFields); err != nil {
				log.Printf("push: create %s failed: %v", key, err)
				continue
			}
		} else {
			updateFields := make(map[string]interface{})
			if s, ok := fieldsMap["summary"]; ok {
				updateFields["summary"] = s
			}
			if d, ok := fieldsMap["description"]; ok {
				updateFields["description"] = d
			}
			if l, ok := fieldsMap["labels"]; ok {
				updateFields["labels"] = l
			}
			if len(updateFields) > 0 {
				if err := jiraclient.UpdateIssue(key, updateFields); err != nil {
					log.Printf("push: update %s failed: %v", key, err)
					continue
				}
			}
		}

		ClearDirtyFlag(ctx, key)
		pushed++
	}

	return pushed, nil
}
