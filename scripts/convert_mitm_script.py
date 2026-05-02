from mitmproxy import http
import os
from datetime import datetime

def load(loader):
    loader.add_option(
        name="out_dir",
        typespec=str,
        default="_/mitm",
        help="Output directory",
    )

def request(flow: http.HTTPFlow):
    # This is called when a request is received, but in replay mode we process it in 'response' or 'done'
    pass

def response(flow: http.HTTPFlow):
    # This is called when a response is received
    process_flow(flow)

def error(flow: http.HTTPFlow):
    # This is called when an error occurs
    process_flow(flow)

def process_flow(flow: http.HTTPFlow):
    from mitmproxy import ctx
    out_dir = ctx.options.out_dir

    # Sequence number is not easily available, but we can use a global counter
    if not hasattr(ctx, "seq"):
        ctx.seq = 0
    ctx.seq += 1
    seq = ctx.seq

    req = flow.request
    resp = flow.response

    # Normalize path for directory
    path = req.path
    if '?' in path:
        path = path.split('?')[0]

    dir_path = path
    # Apply replacements as seen in other scripts
    dir_path = dir_path.replace("9569497", "{accountId}")
    dir_path = dir_path.replace("A81B6A536A98", "{device_id}")

    # Ensure dir_path doesn't have double slashes and is relative
    dir_path = dir_path.lstrip('/')

    full_out_dir = os.path.join(out_dir, "mirror", dir_path)
    os.makedirs(full_out_dir, exist_ok=True)

    # Use timestamp from flow
    dt = datetime.fromtimestamp(flow.timestamp_start)
    timestamp = dt.strftime("%Y%m%d-%H%M%S.%f")[:-3]

    method = req.method
    filename = f"{seq:04d}-{timestamp}-{method}.http"
    file_path = os.path.join(full_out_dir, filename)

    with open(file_path, "wb") as f:
        # Request meta
        f.write(f"### {method} {req.url}\n".encode())
        f.write(f"{method} {req.path}\n".encode())
        f.write(f"Host: {req.host}\n".encode())
        for k, v in req.headers.items():
            f.write(f"{k}: {v}\n".encode())
        f.write(b"\n")
        if req.content:
            f.write(req.content)
        f.write(b"\n\n")

        # Response body (optional, but let's include it if it's small or expected)
        # Check if we should save the response body to a separate file
        if resp and resp.content:
            body_filename = filename.replace(".http", ".xml" if "xml" in resp.headers.get("content-type", "") else ".body")
            body_path = os.path.join(full_out_dir, body_filename)
            with open(body_path, "wb") as bf:
                bf.write(resp.content)

        # Response meta
        f.write(b"> {%\n")
        if resp:
            f.write(f"    // Response: {resp.status_code} {resp.reason}\n".encode())
            f.write(b"    //\n")
            f.write(b"    // Headers:\n")
            for k, v in resp.headers.items():
                f.write(f"    // {k}: {v}\n".encode())
        else:
            f.write(b"    // No response\n")
        f.write(b"%}\n")

    # print(f"Generated {file_path}")

    # WebSocket messages
    if flow.websocket:
        ws_dir = os.path.join(full_out_dir, f"{seq:04d}-websocket")
        os.makedirs(ws_dir, exist_ok=True)
        for i, msg in enumerate(flow.websocket.messages):
            direction = "client" if msg.from_client else "server"
            content = msg.content
            # Apply replacements to content as well if it's text
            if msg.type == 1: # Text
                content_str = content.decode('utf-8', errors='ignore')
                content_str = content_str.replace("9569497", "{accountId}")
                content_str = content_str.replace("A81B6A536A98", "{device_id}")
                content = content_str.encode('utf-8')

            msg_filename = f"{i:04d}-{direction}.{'bin' if msg.type == 2 else 'txt'}"
            msg_path = os.path.join(ws_dir, msg_filename)
            with open(msg_path, "wb") as mf:
                mf.write(content)

            # Write a small metadata file for the message
            with open(msg_path + ".meta", "w") as mmf:
                mmf.write(f"type: {msg.type}\n")
                mmf.write(f"direction: {direction}\n")
                mmf.write(f"timestamp: {msg.timestamp}\n")
