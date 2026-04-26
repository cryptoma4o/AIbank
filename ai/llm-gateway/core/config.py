from pydantic_settings import BaseSettings
from pydantic import Field


class Settings(BaseSettings):
    # Comma-separated: "model-name=http://backend-url"
    model_backends: str = Field(
        default="gemma-4=http://vllm-gemma:8000,qwen-3=http://vllm-qwen:8000",
        alias="MODEL_BACKENDS",
    )
    log_requests: bool = Field(default=True, alias="LOG_REQUESTS")
    port: int = Field(default=8100, alias="PORT")

    model_config = {"populate_by_name": True}


settings = Settings()
