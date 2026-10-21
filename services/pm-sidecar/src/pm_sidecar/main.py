"""Entry point: gRPC server with health service, plus an HTTP /healthz.

The render, spreadsheet, and derived-metric services register here in later
slices. Keeping HTTP /healthz alongside gRPC lets compose and CI probe the
sidecar without a gRPC client.
"""

from __future__ import annotations

import json
import logging
import os
import signal
import threading
from concurrent import futures
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc

from pm_sidecar import __version__

log = logging.getLogger("pm-sidecar")


def _json_logging() -> None:
    class JSONFormatter(logging.Formatter):
        def format(self, record: logging.LogRecord) -> str:
            payload: dict[str, Any] = {
                "level": record.levelname.lower(),
                "msg": record.getMessage(),
                "service": "pm-sidecar",
                "version": __version__,
                "logger": record.name,
            }
            return json.dumps(payload)

    handler = logging.StreamHandler()
    handler.setFormatter(JSONFormatter())
    logging.basicConfig(level=os.getenv("PM_LOG_LEVEL", "INFO").upper(), handlers=[handler])


class HealthHTTP(BaseHTTPRequestHandler):
    server_version = "pm-sidecar"

    def do_GET(self) -> None:  # noqa: N802 (http.server API)
        if self.path in ("/healthz", "/readyz"):
            body = json.dumps(
                {"status": "ok", "service": "pm-sidecar", "version": __version__}
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        self.send_response(404)
        self.end_headers()

    def log_message(self, format: str, *args: Any) -> None:  # noqa: A002
        return  # health probes are noise


def serve() -> None:
    _json_logging()
    grpc_port = int(os.getenv("PM_SIDECAR_GRPC_PORT", "7071"))
    http_port = int(os.getenv("PM_SIDECAR_HTTP_PORT", "7070"))

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)
    health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)
    server.add_insecure_port(f"[::]:{grpc_port}")
    server.start()
    log.info("grpc listening on %d", grpc_port)

    httpd = ThreadingHTTPServer(("", http_port), HealthHTTP)
    threading.Thread(target=httpd.serve_forever, name="healthz", daemon=True).start()
    log.info("http healthz listening on %d", http_port)

    stop = threading.Event()

    def _handle(signum: int, _frame: Any) -> None:
        log.info("signal %d received; shutting down", signum)
        stop.set()

    signal.signal(signal.SIGTERM, _handle)
    signal.signal(signal.SIGINT, _handle)
    stop.wait()

    health_servicer.set("", health_pb2.HealthCheckResponse.NOT_SERVING)
    httpd.shutdown()
    server.stop(grace=10).wait()


if __name__ == "__main__":
    serve()
