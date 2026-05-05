#!/usr/bin/env python3
"""Patch Stockholm bridge JS files to use window.__stockholmBase for API paths.

Usage: patch-stockholm-bridge.py <file> [<file> ...]

Applied once by `make prepare-stockholm`. Idempotent — already-patched files
are left unchanged.
"""
import sys

REPLACEMENTS = [
    (
        'xhr.open("POST", "/api/native/appSend"',
        'xhr.open("POST", (window.__stockholmBase||"") + "/api/native/appSend"',
    ),
    (
        'xhr.open("GET", "/api/native/runQueue',
        'xhr.open("GET", (window.__stockholmBase||"") + "/api/native/runQueue',
    ),
    (
        'var proxyPath = "/api/http-proxy";',
        'var proxyPath = (window.__stockholmBase||"") + "/api/http-proxy";',
    ),
    (
        'return new URL(url, window.location.origin + "/").href;',
        'return new URL(url, window.location.origin + (window.__stockholmBase || "") + "/").href;',
    ),
]

for path in sys.argv[1:]:
    try:
        original = open(path).read()
        patched = original
        for old, new in REPLACEMENTS:
            patched = patched.replace(old, new)
        if patched != original:
            open(path, "w").write(patched)
    except FileNotFoundError:
        print(f"warning: {path} not found, skipping", file=sys.stderr)