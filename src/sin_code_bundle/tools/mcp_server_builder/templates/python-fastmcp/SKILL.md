---
name: sin-{{ slug }}
description: "{{ description }}"
version: {{ version }}
category: mcp
---

# {{ name }}

{{ description }}

## Tools

{% for tool in tools -%}
- `{{ tool }}`
{% endfor %}

## Usage

```bash
sin-{{ slug }}-mcp
```
