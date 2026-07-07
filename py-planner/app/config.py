"""
Environment-driven settings for the py-planner service.
"""
from functools import lru_cache

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")

    port: int = 8000
    env: str = "development"

    go_engine_url: str = "http://localhost:8080"
    internal_api_secret: str = ""

    anthropic_api_key: str = ""
    gemini_api_key: str = ""
    default_llm_provider: str = "anthropic"

    stripe_test_secret_key: str = ""


@lru_cache
def get_settings() -> Settings:
    """Cached settings singleton — avoids re-reading .env on every request."""
    return Settings()