"""
POST /parse — turns an uploaded or pasted OpenAPI spec into a ParsedConnection
(tool list + base_url), ready to be forwarded by the caller to go-engine's
POST /api/tools/connections.
"""
import json

from fastapi import APIRouter, Form, HTTPException, UploadFile

from app.spec_parser.openapi_parser import load_spec_bytes, parse_openapi_spec
from app.spec_parser.tool_schema import ParseSpecRequest, ParsedConnection

router = APIRouter(prefix="/parse", tags=["parse"])


@router.post("/upload", response_model=ParsedConnection)
async def parse_uploaded_spec(
    file: UploadFile,
    connection_name: str = Form(...),
    base_url: str | None = Form(default=None),
    auth_type: str = Form(default="none"),
    auth_config: str = Form(default="{}"),
) -> ParsedConnection:
    """Accepts a multipart file upload (.json or .yaml/.yml OpenAPI spec)."""
    raw = await file.read()
    if not raw:
        raise HTTPException(status_code=400, detail="uploaded file is empty")

    try:
        spec = load_spec_bytes(raw)
    except Exception as exc:
        raise HTTPException(status_code=400, detail=f"failed to parse spec file: {exc}") from exc

    try:
        parsed_auth_config = json.loads(auth_config) if auth_config else {}
    except json.JSONDecodeError as exc:
        raise HTTPException(status_code=400, detail=f"auth_config is not valid JSON: {exc}") from exc

    try:
        result = parse_openapi_spec(spec, connection_name, base_url)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc

    result.auth_type = auth_type
    result.auth_config = parsed_auth_config
    return result


@router.post("/json", response_model=ParsedConnection)
async def parse_json_spec(req: ParseSpecRequest) -> ParsedConnection:
    """Accepts a JSON body with the spec already decoded (used for programmatic calls / tests)."""
    try:
        result = parse_openapi_spec(req.spec, req.connection_name, req.base_url)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc

    result.auth_type = req.auth_type
    result.auth_config = req.auth_config
    return result