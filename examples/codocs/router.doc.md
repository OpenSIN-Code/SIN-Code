# router.py

HTTP Router für Survey-Callback-API. Leitet Provider-Webhooks an Handler weiter.

## Berührt

- `handlers.py` — registriert sich per `router.add_handler()`
- `config.yaml` — liest `router.port`

## Config

- `router.port: 8443` — musste von 8080 weg wegen macOS AirPlay-Konflikt
