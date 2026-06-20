from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI(title="media-matcher", version="0.1.0")


class MatchRequest(BaseModel):
    url: str | None = None
    text: str | None = None


@app.get("/healthz")
def healthz():
    return {"ok": True, "phase": 2}


@app.post("/phash")
def phash(_: MatchRequest):
    return {"score": 0.0, "status": "stub"}


@app.post("/scene")
def scene(_: MatchRequest):
    return {"score": 0.0, "status": "stub"}


@app.post("/audio")
def audio(_: MatchRequest):
    return {"score": 0.0, "status": "stub"}


@app.post("/transcribe")
def transcribe(_: MatchRequest):
    return {"text": "", "status": "stub"}


@app.post("/ocr")
def ocr(_: MatchRequest):
    return {"text": "", "status": "stub"}


@app.post("/embed")
def embed(_: MatchRequest):
    return {"vector": [0.0] * 384, "status": "stub"}


@app.post("/sentiment")
def sentiment(_: MatchRequest):
    return {"score": 0.0, "status": "stub"}
