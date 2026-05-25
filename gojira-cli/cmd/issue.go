package cmd

import "fmt"

func Issue(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: gojira-cli issue <list|create|update|delete|link> [options]")
		return
	}
	switch args[0] {
	case "list":
		issueList(args[1:])
	case "create":
		issueCreate(args[1:])
	case "update":
		issueUpdate(args[1:])
	case "delete":
		issueDelete(args[1:])
	case "link":
		issueLink(args[1:])
	default:
		fmt.Printf("Unknown subcommand: issue %s\n", args[0])
	}
}

func issueList(args []string) {
	project := parseFlag(args, "project")
	if project == "" {
		fatal("--project is required")
	}
	issueType := parseFlag(args, "type")
	reqType := parseFlag(args, "reqtype")
	xbody := fmt.Sprintf(`{"jiraCode":"%s"}`, project)
	result, err := apiGet("/Issue", map[string]string{"XBody": xbody})
	if err != nil {
		fatal(err.Error())
	}
	issues, ok := result.([]interface{})
	if !ok {
		outputJSON(result)
		return
	}
	type I struct {
		Key      string `json:"key"`
		Type     string `json:"issueType"`
		ReqType  string `json:"reqType,omitempty"`
		Summary  string `json:"summary"`
		Status   string `json:"status"`
		Priority string `json:"priority"`
	}
	var out []I
	for _, i := range issues {
		im, _ := i.(map[string]interface{})
		k, _ := im["key"].(string)
		t, _ := im["issueType"].(string)
		if issueType != "" && t != issueType {
			continue
		}
		rt, _ := im["reqtype"].(string)
		if reqType != "" && rt != reqType {
			continue
		}
		s, _ := im["summary"].(string)
		st, _ := im["status"].(string)
		p, _ := im["priority"].(string)
		out = append(out, I{Key: k, Type: t, ReqType: rt, Summary: s, Status: st, Priority: p})
	}
	outputJSON(map[string]interface{}{"count": len(out), "issues": out})
}

func issueCreate(args []string) {
	project := parseFlag(args, "project")
	issueType := parseFlag(args, "type")
	summary := parseFlag(args, "summary")
	description := parseFlag(args, "description")
	priority := parseFlag(args, "priority")
	reqType := parseFlag(args, "reqtype")
	if project == "" || issueType == "" || summary == "" {
		fatal("--project, --type, --summary are required")
	}
	body := map[string]interface{}{
		"key": project, "issueType": issueType, "summary": summary,
		"description": description, "priority": priority, "reqtype": reqType,
	}
	result, err := apiWriteLocal("POST", "/Issue", body)
	if err != nil {
		fatal(err.Error())
	}
	if err := applyLocalWrites(result); err != nil {
		fatal(err.Error())
	}
	outputJSON(result)
}

func issueUpdate(args []string) {
	key := parseFlag(args, "key")
	if key == "" {
		fatal("--key is required")
	}
	data := map[string]interface{}{"key": key, "data": map[string]interface{}{}}
	fields := data["data"].(map[string]interface{})
	for _, f := range []string{"summary", "description", "status", "priority", "assignee"} {
		if v := parseFlag(args, f); v != "" {
			fields[f] = v
		}
	}
	result, err := apiWriteLocal("PUT", "/Issue", data)
	if err != nil {
		fatal(err.Error())
	}
	if err := applyLocalWrites(result); err != nil {
		fatal(err.Error())
	}
	outputJSON(result)
}

func issueDelete(args []string) {
	key := parseFlag(args, "key")
	if key == "" {
		fatal("--key is required")
	}
	result, err := apiWriteLocal("DELETE", "/Issue", map[string]string{"key": key})
	if err != nil {
		fatal(err.Error())
	}
	if err := applyLocalWrites(result); err != nil {
		fatal(err.Error())
	}
	outputJSON(result)
}

func issueLink(args []string) {
	from := parseFlag(args, "from")
	to := parseFlag(args, "to")
	linkType := parseFlag(args, "type")
	if from == "" || to == "" || linkType == "" {
		fatal("--from, --to, --type are required")
	}
	result, err := apiWriteLocal("POST", "/Issue/link", map[string]string{
		"inwardKey": from, "outwardKey": to, "linkType": linkType,
	})
	if err != nil {
		fatal(err.Error())
	}
	if err := applyLocalWrites(result); err != nil {
		fatal(err.Error())
	}
	outputJSON(result)
}
