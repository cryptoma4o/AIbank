from __future__ import annotations
import uvicorn
from fastapi import FastAPI
from models.schemas import ChatRequest, ChatResponse
from agent.dialogue import chat

app = FastAPI(
    title="Conversational Onboarding Agent",
    description="Multi-turn dialogue agent for business client onboarding",
    version="0.1.0",
)


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/v1/chat", response_model=ChatResponse)
async def chat_endpoint(req: ChatRequest) -> ChatResponse:
    return await chat(req)


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8106, reload=False)
