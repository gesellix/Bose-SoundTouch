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
        # The standalone /api/http-proxy declaration in browser_http_proxy.js
        # uses an UPPERCASE constant name. Same shape as above, different
        # identifier — keep both replacements; the lowercase one applies to
        # app_comm.js, the uppercase one to browser_http_proxy.js.
        'var PROXY_PATH = "/api/http-proxy";',
        'var PROXY_PATH = (window.__stockholmBase||"") + "/api/http-proxy";',
    ),
    (
        # browser_http_proxy.js's IIFE evaluates PROXY_PATH at script-load
        # time, but the injected bootstrap that defines window.__stockholmBase
        # is placed just before </head> — i.e. after the <script src=…> tags
        # for the bridge files. So PROXY_PATH would always fall back to the
        # unprefixed "/api/http-proxy", failing under STOCKHOLM_BASE_PATH.
        # Inline a lazy expression at the use site so it reads __stockholmBase
        # at call time, when bootstrap has finished. The var declaration above
        # remains patched but becomes dead code.
        'return PROXY_PATH + "?url=" + encodeURIComponent(target.href);',
        'return (window.__stockholmBase||"") + "/api/http-proxy?url=" + encodeURIComponent(target.href);',
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