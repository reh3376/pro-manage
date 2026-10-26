# pm-sidecar

The Python sidecar from FSD §05: PDF/PPTX rendering, chart rendering for exports, xlsx import/export, and sandboxed derived-metric formulas. gRPC server; stateless.

Today the sidecar is the skeleton from slice 0a: a gRPC server with the standard health service on `:7071` and an HTTP `/healthz` on `:7070`, covered by a UATS spec and the compose smoke. The first real surface (`pm.sidecar.v1.RenderService` for exports, with its UDTS spec) is backlog item B-05 and arrives with the first export.

```bash
uv sync
uv run pm-sidecar          # or: uv run python -m pm_sidecar.main
uv run pytest
uv run ruff check .
```
