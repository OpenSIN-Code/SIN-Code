from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field, model_validator


class JsonRpcRequest(BaseModel):
    jsonrpc: Literal["2.0"] = "2.0"
    id: str | int | None = None
    method: str = Field(min_length=1)
    params: dict[str, Any] | list[Any] | None = None

    @model_validator(mode="after")
    def validate_method_not_empty(self) -> JsonRpcRequest:
        if not self.method.strip():
            raise ValueError("method must not be empty")
        return self


class JsonRpcResponse(BaseModel):
    jsonrpc: Literal["2.0"] = "2.0"
    id: str | int | None = None
    result: Any = None
    error: dict[str, Any] | None = None


class ToolCallParams(BaseModel):
    tool_name: str = Field(alias="name", min_length=1)
    arguments: dict[str, Any] = Field(default_factory=dict)
    correlation_id: str | None = None

    model_config = {"populate_by_name": True}


class MessagePart(BaseModel):
    type: str = "text"
    text: str = ""


class A2AMessage(BaseModel):
    parts: list[MessagePart] = Field(default_factory=list)


class MessageSendParams(BaseModel):
    message: A2AMessage


class FindSymbolArgs(BaseModel):
    symbol: str = Field(min_length=1)
    root: str | None = None


class FindReferencesArgs(BaseModel):
    symbol: str = Field(min_length=1)
    root: str | None = None


class ReplaceSymbolBodyArgs(BaseModel):
    symbol: str = Field(min_length=1)
    file: str = Field(min_length=1)
    body: str = Field(min_length=1)
    root: str | None = None


class InsertAfterSymbolArgs(BaseModel):
    symbol: str = Field(min_length=1)
    file: str = Field(min_length=1)
    text: str = Field(min_length=1)
    root: str | None = None


class ProjectOverviewArgs(BaseModel):
    root: str | None = None


class SymbolSearchArgs(BaseModel):
    query: str = Field(min_length=1)
    root: str | None = None


class StructuralEditArgs(BaseModel):
    editPayload: str = Field(min_length=1)


class MemoryQueryArgs(BaseModel):
    query: str = Field(min_length=1)
    root: str | None = None
    target_symbol: str | None = None


class TaskGetArgs(BaseModel):
    taskId: str = Field(min_length=1, alias="id")
    model_config = {"populate_by_name": True}


class TaskUpdateArgs(BaseModel):
    taskId: str = Field(min_length=1, alias="id")
    input: dict[str, Any] = Field(default_factory=dict)
    model_config = {"populate_by_name": True}


class TaskCancelArgs(BaseModel):
    taskId: str = Field(min_length=1, alias="id")
    model_config = {"populate_by_name": True}


TOOL_ARG_MODELS: dict[str, type[BaseModel]] = {
    "sin_simone_mcp_symbol_search": SymbolSearchArgs,
    "sin_simone_mcp_structural_edit": StructuralEditArgs,
    "sin_simone_mcp_memory_query": MemoryQueryArgs,
    "sin_simone_mcp_find_references": FindReferencesArgs,
    "sin_simone_mcp_project_overview": ProjectOverviewArgs,
    "sin_simone_mcp_health": ProjectOverviewArgs,
    "tasks/get": TaskGetArgs,
    "tasks/update": TaskUpdateArgs,
    "tasks/cancel": TaskCancelArgs,
}
