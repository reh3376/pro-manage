"""Smoke tests for the sidecar skeleton."""

from __future__ import annotations

import json
import threading
from http.client import HTTPConnection
from http.server import ThreadingHTTPServer

from pm_sidecar import __version__
from pm_sidecar.main import HealthHTTP


def test_healthz_reports_service_and_version() -> None:
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), HealthHTTP)
    port = httpd.server_address[1]
    t = threading.Thread(target=httpd.serve_forever, daemon=True)
    t.start()
    try:
        conn = HTTPConnection("127.0.0.1", port, timeout=5)
        conn.request("GET", "/healthz")
        resp = conn.getresponse()
        assert resp.status == 200
        body = json.loads(resp.read())
        assert body == {"status": "ok", "service": "pm-sidecar", "version": __version__}
        conn.request("GET", "/nope")
        assert conn.getresponse().status == 404
    finally:
        httpd.shutdown()
