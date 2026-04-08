package db

import "go.mongodb.org/mongo-driver/v2/bson"

// customFieldMap maps JIRA custom field IDs to cache_schema friendly names.
var customFieldMap = map[string]string{
	"customfield_15209": "subkey",
	"customfield_15235": "client",
	"customfield_15248": "Features",
	"customfield_15249": "Requirements",
	"customfield_15215": "actor",
	"customfield_15216": "precondition",
	"customfield_15217": "basicflow",
	"customfield_15218": "alternativeflow",
	"customfield_15219": "exceptionflow",
	"customfield_15220": "postcondition",
	"customfield_15247": "VOS",
	"customfield_18201": "Test",
	"customfield_12555": "reqtype",
	"customfield_18217": "qa",
	"customfield_15210": "source",
	"customfield_15211": "stimulus",
	"customfield_15208": "artifact",
	"customfield_15212": "response",
	"customfield_15213": "measure",
	"customfield_15207": "responseScenario",
	"customfield_15204": "qai",
	"customfield_15205": "qad",
	"customfield_15206": "qas",
	"customfield_18202": "A-Design",
	"customfield_18203": "D-Design",
	"customfield_18218": "dmodel",
	"customfield_18206": "objectives",
	"customfield_18207": "pros",
	"customfield_18208": "cons",
	"customfield_18209": "score",
	"customfield_18210": "eval",
	"customfield_18212": "sensitivity",
	"customfield_18214": "nonrisk",
	"customfield_18213": "mitigation",
	"customfield_18216": "risk",
	"customfield_18211": "decision",
	"customfield_18204": "adiagram",
	"customfield_18205": "bdiagram",
	"customfield_18215": "archdiagram",
}

// TransformToCache converts a raw JIRA issue (key, id, fields...) to cache_schema format.
func TransformToCache(issue map[string]interface{}) bson.M {
	fields := toMap(issue["fields"])

	doc := bson.M{
		"key":         issue["key"],
		"id":          issue["id"],
		"duedate":     fields["duedate"],
		"issueType":   nestedStr(fields, "issuetype", "name"),
		"components":  extractNames(fields["components"]),
		"summary":     fields["summary"],
		"description": fields["description"],
		"labels":      fields["labels"],
		"status":      simplify(fields["status"], "id", "name"),
		"resolution":  simplify(fields["resolution"], "id", "name"),
		"reporter":    simplifyUser(fields["reporter"]),
		"priority":    simplify(fields["priority"], "id", "name"),
		"issueLinks":  transformLinks(fields["issuelinks"]),
		"attachment":  fields["attachment"],
		"environment": fields["environment"],
	}

	// Map custom fields
	for cfID, cacheName := range customFieldMap {
		if val, ok := fields[cfID]; ok {
			if cacheName == "reqtype" {
				// Extract "value" or "name" from map (JIRA returns {"value":"NFR"} or {"name":"NFR"})
				if m := toMap(val); m != nil {
					if v, ok := m["value"]; ok {
						doc[cacheName] = v
					} else if v, ok := m["name"]; ok {
						doc[cacheName] = v
					} else {
						doc[cacheName] = val
					}
				} else {
					doc[cacheName] = val
				}
			} else {
				doc[cacheName] = val
			}
		}
	}

	return doc
}

func toMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func nestedStr(m map[string]interface{}, keys ...string) string {
	var cur interface{} = m
	for _, k := range keys {
		cm := toMap(cur)
		if cm == nil {
			return ""
		}
		cur = cm[k]
	}
	s, _ := cur.(string)
	return s
}

func simplify(v interface{}, keys ...string) interface{} {
	m := toMap(v)
	if m == nil {
		return nil
	}
	result := bson.M{}
	for _, k := range keys {
		result[k] = m[k]
	}
	return result
}

func simplifyUser(v interface{}) interface{} {
	m := toMap(v)
	if m == nil {
		return nil
	}
	return bson.M{
		"id":   m["accountId"],
		"name": m["displayName"],
	}
}

func extractNames(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return []string{}
	}
	names := make([]string, 0, len(arr))
	for _, item := range arr {
		if m := toMap(item); m != nil {
			if name, ok := m["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

// reverseCustomFieldMap returns a map from cache field names to JIRA custom field IDs.
var reverseCustomFieldMap map[string]string

func init() {
	reverseCustomFieldMap = make(map[string]string, len(customFieldMap))
	for cfID, cacheName := range customFieldMap {
		reverseCustomFieldMap[cacheName] = cfID
	}
}

// standardJiraFields is a whitelist of standard JIRA fields that can be set via REST API.
var standardJiraFields = map[string]bool{
	"summary":     true,
	"description": true,
	"labels":      true,
	"duedate":     true,
	"environment": true,
}

// CacheToJiraFields converts cache-schema field names back to JIRA custom field IDs.
// Only whitelisted standard fields and mapped custom fields are included.
// Returns a map suitable for JIRA REST API PUT /issue/{key}.
func CacheToJiraFields(cacheFields map[string]interface{}) map[string]interface{} {
	jiraFields := make(map[string]interface{})
	for k, v := range cacheFields {
		// Check if it's a mapped custom field
		if cfID, ok := reverseCustomFieldMap[k]; ok {
			if k == "reqtype" {
				if s, ok := v.(string); ok && s != "" {
					jiraFields[cfID] = map[string]interface{}{"value": s}
				}
			} else {
				jiraFields[cfID] = v
			}
		} else if standardJiraFields[k] {
			// Whitelisted standard JIRA field
			jiraFields[k] = v
		}
		// Everything else is silently skipped
	}
	return jiraFields
}

func transformLinks(v interface{}) []interface{} {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]interface{}, 0, len(arr))
	for _, item := range arr {
		m := toMap(item)
		if m == nil {
			continue
		}
		link := bson.M{
			"id":   m["id"],
			"type": simplify(m["type"], "name"),
		}
		if inward := toMap(m["inwardIssue"]); inward != nil {
			link["inwardIssue"] = bson.M{
				"key":    inward["key"],
				"fields": bson.M{"summary": nestedStr(inward, "fields", "summary")},
			}
		}
		if outward := toMap(m["outwardIssue"]); outward != nil {
			link["outwardIssue"] = bson.M{
				"key":    outward["key"],
				"fields": bson.M{"summary": nestedStr(outward, "fields", "summary")},
			}
		}
		result = append(result, link)
	}
	return result
}
