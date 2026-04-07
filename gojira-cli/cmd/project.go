package cmd

import "fmt"

func Project(args []string) {
	if len(args) == 0 || args[0] == "list" {
		projectList(args)
	} else {
		fmt.Println("Usage: gojira-cli project list [--code CODE]")
	}
}

func projectList(args []string) {
	code := parseFlag(args, "code")
	xbody := "{}"
	if code != "" {
		xbody = fmt.Sprintf(`{"jiraCode":"%s"}`, code)
	}
	result, err := apiGet("/Project", map[string]string{"XBody": xbody})
	if err != nil {
		fatal(err.Error())
	}
	projects, ok := result.([]interface{})
	if !ok {
		outputJSON(result)
		return
	}
	type P struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	var out []P
	for _, p := range projects {
		pm, _ := p.(map[string]interface{})
		jc, _ := pm["jiraCodes"].(map[string]interface{})
		main, _ := jc["main"].(map[string]interface{})
		c, _ := main["jiraCode"].(string)
		n, _ := pm["name"].(string)
		s, _ := pm["status"].(string)
		if s == "Deleted" {
			continue
		}
		if code != "" && c != code {
			continue
		}
		out = append(out, P{Code: c, Name: n, Status: s})
	}
	outputJSON(map[string]interface{}{"count": len(out), "projects": out})
}
