package cmd

import (
	"encoding/base64"
	"fmt"
	"net/url"
)

func Doc(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: gojira-cli doc <list|summarize|diff|get> [options]")
		return
	}
	switch args[0] {
	case "list":
		docList(args[1:])
	case "get":
		docGet(args[1:])
	case "summarize":
		docSummarize(args[1:])
	case "diff":
		docDiff(args[1:])
	default:
		fmt.Printf("Unknown subcommand: doc %s\n", args[0])
	}
}

func docList(args []string) {
	project := parseFlag(args, "project")
	if project == "" {
		fatal("--project is required")
	}
	ref := parseFlag(args, "ref")
	if ref == "" {
		ref = "main"
	}
	result, err := platformGet(fmt.Sprintf("/DocRepo/%s/tree?ref=%s", project, url.QueryEscape(ref)))
	if err != nil {
		fatal(err.Error())
	}
	tm, _ := result.(map[string]interface{})
	items, _ := tm["tree"].([]interface{})
	var files []string
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		if t, _ := m["type"].(string); t != "tree" {
			if p, _ := m["path"].(string); p != "" {
				files = append(files, p)
			}
		}
	}
	outputJSON(map[string]interface{}{"count": len(files), "files": files})
}

func docGet(args []string) {
	project := parseFlag(args, "project")
	file := parseFlag(args, "file")
	ref := parseFlag(args, "ref")
	if project == "" || file == "" {
		fatal("--project and --file are required")
	}
	if ref == "" {
		ref = "main"
	}
	result, err := platformGet(fmt.Sprintf("/DocRepo/%s/file?path=%s&ref=%s",
		project, url.QueryEscape(file), url.QueryEscape(ref)))
	if err != nil {
		fatal(err.Error())
	}
	rm, _ := result.(map[string]interface{})
	content, _ := rm["content"].(string)
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		fmt.Println(content)
	} else {
		fmt.Println(string(decoded))
	}
}

func docSummarize(args []string) {
	project := parseFlag(args, "project")
	file := parseFlag(args, "file")
	if project == "" || file == "" {
		fatal("--project and --file are required")
	}
	// Agent chat을 통해 요약 요청
	result, err := apiPost("/Agent/chat", map[string]interface{}{
		"message": fmt.Sprintf("%s 프로젝트의 %s 문서를 요약해줘", project, file),
		"context": map[string]string{"page": "docs", "projectCode": project, "filePath": file},
	})
	if err != nil {
		fatal(err.Error())
	}
	rm, _ := result.(map[string]interface{})
	fmt.Println(rm["reply"])
}

func docDiff(args []string) {
	project := parseFlag(args, "project")
	file := parseFlag(args, "file")
	if project == "" || file == "" {
		fatal("--project and --file are required")
	}
	result, err := apiPost("/Agent/chat", map[string]interface{}{
		"message": fmt.Sprintf("%s 프로젝트의 %s 파일 Diff를 분석해줘", project, file),
		"context": map[string]string{"page": "docs", "projectCode": project, "filePath": file},
	})
	if err != nil {
		fatal(err.Error())
	}
	rm, _ := result.(map[string]interface{})
	fmt.Println(rm["reply"])
}
