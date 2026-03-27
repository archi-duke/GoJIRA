JIRA Datacenter 버전을 모사하는 API를 구현.
JIRA Datacenter에서 사용되는 API 규격은 https://developer.atlassian.comd에서 v2 API를 참고해서 구현
이슈 생성, 조회, 목록 조회, 삭제, 라벨 추가/삭제,링크 생성/삭제에 해당하는 API를 구현하면 됨.
FastAPI를 사용해서 구현하고, DB는 issue 에 있는 json 파일들을 사용하면 됨.
API는 JIRA-API 폴더에 구현하면 됨.