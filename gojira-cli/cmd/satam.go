package cmd

import "fmt"

func SATAM(args []string) {
	if len(args) == 0 || args[0] == "analyze" {
		satamAnalyze(args)
	} else {
		fmt.Println("Usage: gojira-cli satam analyze --project CODE")
	}
}

func satamAnalyze(args []string) {
	project := parseFlag(args, "project")
	if project == "" {
		fatal("--project is required")
	}
	result, err := apiPost("/Agent/chat", map[string]interface{}{
		"message": fmt.Sprintf("%s 프로젝트의 SATAM 분석을 해줘", project),
		"context": map[string]string{"page": "satam", "projectCode": project},
	})
	if err != nil {
		fatal(err.Error())
	}
	rm, _ := result.(map[string]interface{})
	fmt.Println(rm["reply"])
}
