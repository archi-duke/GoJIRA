다음 요구사항을 기준으로 MongoDB 기반 사용자 계정 관리 기능을 설계/구현하라.

목표

사용자, 조직, 권한 그룹, 프로젝트, 접속 상태를 분리된 컬렉션으로 관리하고, 권한은 그룹 기반으로 계산한다.

핵심 요구사항
조직 정보와 권한 그룹 정보를 분리할 것
사용자 문서에는 이름 문자열 대신 참조 ID를 저장할 것
로그인 상태와 SignalR connectionId는 사용자 컬렉션이 아니라 별도 userConnections 컬렉션에서 관리할 것
사용자 권한은 개별 사용자 문서에 직접 저장하지 말고 authGroups의 권한을 합산하여 계산할 것
프로젝트 참여 정보는 userProjects 컬렉션으로 분리할 것
모든 컬렉션에는 isActive, createdAt, updatedAt를 둘 것
soft delete 기준으로 설계할 것
MongoDB 인덱스까지 제안할 것
컬렉션
users
authGroups
orgUnits
projects
userProjects
userConnections
권한 모델
권한은 {권한코드: 비트마스크값} 구조
CRUD 비트마스크 사용
예: C=8, R=4, U=2, D=1
사용자의 최종 권한은 소속 authGroup들의 authority를 OR 연산하여 계산
구현 요청 항목
각 컬렉션의 MongoDB schema 설계안
NestJS 또는 ASP.NET Core 기준 entity/model 예시
REST API 목록
권한 계산 service 예시 코드
SignalR 연결/해제 시 userConnections 처리 예시 코드
인덱스 설계
초기 seed 데이터 예시
향후 확장 포인트 정리
주의사항
group이라는 명칭은 조직 그룹과 권한 그룹이 혼동되므로 직접 사용하지 말 것
조직 단위는 orgUnit, 권한 그룹은 authGroup으로 분리할 것
users 컬렉션에 logIn, conID를 두지 말 것
users 컬렉션에 authority를 직접 중복 저장하지 말 것


* 스키마 구조 (참고하여 구현할것)
다음 요구사항을 기준으로 MongoDB 기반 사용자 계정 관리 스키마를 설계하고, 구현 가능한 수준의 모델/DTO/API 초안을 작성하라.

[설계 원칙]
- 조직 정보와 권한 정보를 분리한다.
- 사용자와 접속 상태를 분리한다.
- 권한은 그룹 기반으로 계산한다.
- 문자열 이름 참조 대신 ObjectId 참조를 사용한다.
- soft delete(isActive=false) 기반으로 설계한다.

[컬렉션]
1. users
2. authGroups
3. orgUnits
4. projects
5. userProjects
6. userConnections

[users 예시]
{
  "_id": ObjectId,
  "knoxId": "batman",
  "name": "김덕겸",
  "email": "batman@example.com",
  "authGroupIds": [ObjectId],
  "orgUnitIds": [ObjectId],
  "isActive": true,
  "createdAt": ISODate,
  "updatedAt": ISODate,
  "createdBy": "system",
  "updatedBy": "system"
}

[authGroups 예시]
{
  "_id": ObjectId,
  "code": "MASTER",
  "name": "Master",
  "description": "System administrator",
  "authority": {
    "ALLPRJ": 15,
    "USR": 15,
    "ORG": 15,
    "PRJ": 15
  },
  "isActive": true,
  "createdAt": ISODate,
  "updatedAt": ISODate,
  "createdBy": "system",
  "updatedBy": "system"
}

[orgUnits 예시]
{
  "_id": ObjectId,
  "code": "JL-TEAM",
  "name": "Justice League",
  "type": "team",
  "parentOrgUnitId": null,
  "sortOrder": 1,
  "isActive": true,
  "createdAt": ISODate,
  "updatedAt": ISODate,
  "createdBy": "system",
  "updatedBy": "system"
}

[projects 예시]
{
  "_id": ObjectId,
  "code": "ABC",
  "name": "ABC Project",
  "description": "ABC system modernization",
  "isActive": true,
  "createdAt": ISODate,
  "updatedAt": ISODate,
  "createdBy": "system",
  "updatedBy": "system"
}

[userProjects 예시]
{
  "_id": ObjectId,
  "userId": ObjectId,
  "projectId": ObjectId,
  "role": "developer",
  "isActive": true,
  "joinedAt": ISODate,
  "leftAt": null,
  "createdAt": ISODate,
  "updatedAt": ISODate,
  "createdBy": "system",
  "updatedBy": "system"
}

[userConnections 예시]
{
  "_id": ObjectId,
  "userId": ObjectId,
  "connectionId": "signalr-1234567890",
  "clientType": "web",
  "isOnline": true,
  "connectedAt": ISODate,
  "lastSeenAt": ISODate,
  "disconnectedAt": null,
  "createdAt": ISODate,
  "updatedAt": ISODate
}

[권한 규칙]
- CRUD 비트마스크 사용
- C=8, R=4, U=2, D=1
- 사용자의 최종 권한은 authGroups.authority 를 권한코드별 OR 연산하여 계산한다.
- users 에 authority 를 중복 저장하지 않는다.

[구현 요청]
1. MongoDB schema 설계안
2. ASP.NET Core 또는 NestJS 기준 model/entity 코드
3. DTO 설계
4. REST API 목록
5. 권한 계산 service 코드 예시
6. SignalR 연결/해제 처리 예시
7. MongoDB 인덱스 생성 코드
8. seed 데이터 예시
9. 확장 포인트 정리


** 구현 규칙
1. users 컬렉션에는 authority, logIn, conID를 저장하지 않는다.
2. 최종 권한은 users.authGroupIds -> authGroups.authority 를 조회하여 OR 연산으로 계산한다.
3. 조직은 orgUnits 단일 컬렉션으로 관리하고 parentOrgUnitId 로 계층을 표현한다.
4. 프로젝트 참여는 userProjects 컬렉션으로 분리한다.
5. 삭제는 물리 삭제보다 isActive=false 기반 soft delete 를 기본으로 한다.
6. createdAt, updatedAt, createdBy, updatedBy 를 모든 주요 컬렉션에 둔다.
7. API 응답 모델에서는 ObjectId 대신 문자열 id 로 변환해서 반환한다.
8. 조회 시 join 성격의 정보가 필요하면 aggregation 또는 service layer 조합으로 처리한다.


*** 주의!!!!
1차: 스키마/인덱스/DTO 설계
2차: API/Service/권한 계산 코드
3차: SignalR 연결 처리 및 샘플 Controller/Hub
4차: seed 데이터와 테스트 코드