# pm-sidecar

The Python sidecar from FSD §05: PDF/PPTX rendering, chart rendering for exports, xlsx import/export, and sandboxed derived-metric formulas. gRPC server; stateless.

Slice 0 ships the skeleton only: a gRPC server with the standard health service on `:7071` and an HTTP `/healthz` on `:7070`.

```bash
uv sync
uv run pm-sidecar          # or: uv run python -m pm_sidecar.main
uv run pytest
uv run ruff check .
```
