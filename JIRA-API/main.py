import uvicorn
from fastapi import FastAPI

from routers.issues import router as issues_router

app = FastAPI(
    title="JIRA Datacenter Mock API",
    description="JIRA Datacenter REST API v2 mock server",
    version="1.0.0",
)

app.include_router(issues_router)


@app.get("/rest/api/2/serverInfo")
def server_info():
    return {
        "baseUrl": "http://localhost:8000",
        "version": "9.12.0",
        "versionNumbers": [9, 12, 0],
        "deploymentType": "Server",
        "buildNumber": 912000,
        "serverTitle": "GoJIRA Mock Server",
    }


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8000, reload=True)
