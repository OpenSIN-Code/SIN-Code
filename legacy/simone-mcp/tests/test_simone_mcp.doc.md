# `tests/test_simone_mcp.py` — Test Suite

Partner file: `tests/test_simone_mcp.py`

## Purpose
Comprehensive test suite covering all Simone MCP modules: core, protocol, A2A, HTTP, security, schemas, correlation, and CLI. Tests are organized by feature area with SEP-numeric prefixes for specification compliance.

## Test Categories

### Core Functionality
| Test | What It Tests |
|------|--------------|
| `test_cli_print_card` | CLI print-card command |
| `test_agent_card_shape` | Agent card structure |
| `test_health_action_and_async_task` | Health action + LSP task |
| `test_symbol_tools_on_python_file` | find_symbol, find_references, replace_symbol_body, insert_after_symbol |
| `test_project_overview_and_dashboard` | get_project_overview + dashboard HTML |
| `test_realtime_url_builder` | Supabase URL conversion |
| `test_replace_symbol_body_preserves_comments` | libcst/AST comment preservation |
| `test_replace_symbol_body_reports_engine` | Engine reporting (libcst vs ast) |
| `test_find_references_reports_engine` | Engine reporting (jedi vs regex) |
| `test_replace_nonexistent_function` | Error handling for missing symbol |
| `test_insert_after_preserves_rest` | Insertion without destroying other symbols |
| `test_treesitter_candidate_files_includes_js_ts` | JS/TS file inclusion |
| `test_js_regex_fallback` | Regex-based JS symbol extraction |

### Correlation (SEP-2663)
| Test | What It Tests |
|------|--------------|
| `test_correlation_generate_and_complete` | Basic lifecycle |
| `test_correlation_with_provided_id` | Custom correlation IDs |
| `test_correlation_failure_tracking` | Error tracking |
| `test_correlation_cleanup` | Stale call cleanup |
| `test_correlation_bounded_eviction` | LRU eviction (max 5) |
| `test_correlation_auto_cleanup` | Auto-cleanup every 64 ops |
| `test_correlation_thread_safety` | Concurrent thread safety |

### A2A Protocol
| Test | What It Tests |
|------|--------------|
| `test_a2a_agent_discover` | Agent card discovery |
| `test_a2a_agent_ping` | Ping with timestamp |
| `test_a2a_tool_list` | Tool listing |
| `test_a2a_tool_call` | Tool execution with correlation |
| `test_a2a_unknown_method` | Error handling |
| `test_a2a_message_send` | JSON action parsing |
| `test_a2a_message_send_plain_text` | Natural language fallback |
| `test_a2a_tasks_get` | Task retrieval |
| `test_a2a_invalid_jsonrpc_version` | Version validation |
| `test_a2a_missing_method` | Missing method error |
| `test_a2a_tool_call_invalid_tool` | Invalid tool error |
| `test_a2a_message_send_invalid_params` | Invalid params error |

### SEP-2663 Tasks Extension
| Test | What It Tests |
|------|--------------|
| `test_sep2663_protocol_version` | Version constants |
| `test_sep2663_initialize_declares_tasks_extension` | Capabilities declaration |
| `test_sep2663_server_creates_task_autonomously` | Task creation |
| `test_sep2663_tasks_get_returns_inline_result` | Inline results |
| `test_sep2663_tasks_get_failed_task_has_error` | Error reporting |
| `test_sep2663_tasks_update_resumes_input_required` | Task resumption |
| `test_sep2663_tasks_update_rejects_non_input_required` | Invalid update |
| `test_sep2663_removed_tasks_result_returns_method_not_found` | Removed method |
| `test_sep2663_removed_tasks_list_returns_method_not_found` | Removed method |
| `test_sep2663_notifications_tasks_not_status` | Notification format |
| `test_sep2663_tasks_cancel_still_works` | Task cancellation |

### SEP-2243 HTTP Header Standardization
| Test | What It Tests |
|------|--------------|
| `test_sep2243_mcp_method_header_mismatch` | Method header mismatch |
| `test_sep2243_mcp_method_header_matches` | Method header match |
| `test_sep2243_mcp_name_header_mismatch` | Name header mismatch |
| `test_sep2243_mcp_param_header_mismatch` | Param header mismatch |
| `test_sep2243_mcp_name_header_matches` | Name header match |

### SEP-2549 TTL for List Results
| Test | What It Tests |
|------|--------------|
| `test_sep2549_tools_list_has_ttl` | Tool list TTL |
| `test_sep2549_resources_list_has_ttl` | Resource list TTL |
| `test_sep2549_resource_templates_list_has_ttl` | Template list TTL |
| `test_sep2549_prompts_list_has_ttl` | Prompt list TTL |

### Security
| Test | What It Tests |
|------|--------------|
| `test_rate_limit_triggers_when_bucket_full` | Rate limit 429 |
| `test_rate_limit_allows_under_max` | Normal request |
| `test_rate_limit_window_expires_old_entries` | Window expiry |
| `test_cors_blocks_disallowed_origin` | CORS rejection |
| `test_cors_allows_whitelisted_origin` | CORS acceptance |
| `test_cors_allows_no_origin_header` | Missing origin |
| `test_auth_bypasses_open_paths` | Open path auth bypass |
| `test_auth_rejects_missing_bearer` | Missing token |
| `test_auth_rejects_invalid_scheme` | Wrong scheme |
| `test_body_size_limit` | 1MB limit |
| `test_extract_client_ip_from_x_forwarded_for` | XFF parsing |
| `test_extract_client_ip_fallback` | Client IP fallback |
| `test_path_traversal_blocked` | Path traversal prevention |

### Protocol (Other)
| Test | What It Tests |
|------|--------------|
| `test_protocol_resources_subscribe` | Resource subscription |
| `test_protocol_resources_unsubscribe` | Resource unsubscription |
| `test_protocol_prompts_get_missing_required_arg` | Missing arg error |
| `test_protocol_prompts_get_unknown` | Unknown prompt error |
| `test_protocol_logging_set_level` | Log level setting |
| `test_protocol_logging_invalid_level` | Invalid level error |
| `test_protocol_completion` | Argument completion |
| `test_protocol_sampling_not_supported` | Sampling error |
| `test_protocol_elicitation_not_supported` | Elicitation error |
| `test_protocol_resources_read_not_found` | Resource not found |

### HTTP Endpoints
| Test | What It Tests |
|------|--------------|
| `test_http_delete_session` | DELETE /mcp |
| `test_http_delete_session_missing_id` | Missing session ID |
| `test_http_health_endpoint` | GET /health |
| `test_http_root_endpoint` | GET / |
| `test_http_well_known_agent_card` | GET /.well-known/agent-card.json |
| `test_http_a2a_endpoint` | POST /a2a/v1 |
| `test_http_invalid_json_body` | Invalid JSON |
| `test_http_dashboard` | GET /dashboard |

### Schema Validation
| Test | What It Tests |
|------|--------------|
| `test_pydantic_schemas_jsonrpc_validation` | JSON-RPC version |
| `test_pydantic_schemas_tool_call_params` | Tool call params |
| `test_pydantic_schemas_find_symbol_args` | Find symbol args |
| `test_pydantic_replace_symbol_body_requires_body` | Non-empty body |
| `test_pydantic_insert_after_requires_text` | Non-empty text |
| `test_sep2663_task_update_args_schema` | Task update schema |
| `test_sep2663_task_list_args_removed` | Removed task list |
| `test_sep2663_task_get_args_schema` | Task get schema |

### CLI
| Test | What It Tests |
|------|--------------|
| `test_cli_validate_basic` | Config validation |
| `test_mcp_stdio_initialization` | Stdio initialization |

## Relationship
- `src/simone_mcp/core.py` — tests all public functions
- `src/simone_mcp/protocol.py` — tests SEP-2663, SEP-2243, SEP-2549
- `src/simone_mcp/a2a_handler.py` — tests A2A methods
- `src/simone_mcp/http_app.py` — tests security + routes
- `src/simone_mcp/schemas.py` — tests Pydantic validation
- `src/simone_mcp/correlation.py` — tests correlation tracking
- `src/simone_mcp/cli.py` — tests CLI commands
- `src/simone_mcp/mcp_stdio.py` — tests stdio transport

## Dependencies
- `pytest` + `pytest-asyncio`
- `httpx` (for ASGI transport tests)
- `pathlib` (for temp file fixtures)

## Running Tests
```bash
pytest tests/test_simone_mcp.py -v
pytest tests/test_simone_mcp.py -k sep2663  # Only SEP-2663 tests
pytest tests/test_simone_mcp.py -k security  # Only security tests
```

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
