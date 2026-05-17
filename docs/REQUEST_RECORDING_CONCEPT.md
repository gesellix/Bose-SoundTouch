# Request Recording Concept

## Problem Statement

The current request recording system has fundamental issues when dealing with request cloning, body consumption, and multiple response scenarios. Specifically:

1. **Body Consumption**: HTTP request bodies can only be read once, leading to missing bodies in recordings
2. **Request Cloning**: A single original request may be cloned multiple times for different purposes (local handling, mirroring, recording)
3. **Multiple Responses**: The same logical request may generate different responses (local vs upstream mirror)
4. **Data Integrity**: No guarantee that recorded requests are identical across different execution paths

## Current Issues (Examples)

### Issue 1: Missing Request Bodies in Mirror Recordings

**Local Recording** (complete):
```http
### POST /v1/scmudc/AABBCCDDEEFF
POST /v1/scmudc/AABBCCDDEEFF
Host: events.api.bosecm.com
Content-Type: text/json; charset=utf-8
Content-Length: 587
Authorization: Bearer jGwEmFWr...

{"envelope":{"monoTime":234906,"payloadProtocolVersion":"3.1","payloadType":"scmudc","protocolVersion":"1.0","time":"2026-02-25T23:03:14.976349+00:00","uniqueId":"AABBCCDDEEFF"},"payload":{"deviceInfo":{"boseID":"1000001","deviceID":"AABBCCDDEEFF","deviceType":"SoundTouch 10","serialNumber":"I6332527703739342000020","softwareVersion":"27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29","systemSerialNumber":"069231P63364828AE"},"events":[{"data":{"play-state":"PAUSE_STATE"},"monoTime":234904,"time":"2026-02-25T23:03:14.973466+00:00","type":"play-state-changed"}]}}

{% raw %}
> {%
    // Response: 200 OK
%}
{% endraw %}
```

**Mirror Recording** (missing body):
```http
### POST /v1/scmudc/AABBCCDDEEFF
POST /v1/scmudc/AABBCCDDEEFF
Host: events.api.bosecm.com
Content-Type: text/json; charset=utf-8
Content-Length: 587
Authorization: Bearer jGwEmFWr...



{% raw %}
> {%
    // Response: 200 OK
    // Headers:
    // X-Proxy-Origin: upstream-mirror
%}
{% endraw %}
```

### Issue 2: Request Flow Complexity

Current middleware execution order:
```
1. MirrorMiddleware    - Buffers body, creates clones
2. RecordMiddleware    - Also buffers body
3. Application Handler - Processes request
4. Mirror Execution    - Async/sync mirror to upstream
5. Recording           - Multiple recording points
```

Problems:
- Multiple body reads across middleware chain
- Inconsistent request state between clones
- Race conditions in async scenarios
- No guarantee of request equivalence

## Proposed Solution: Context-Bound Request Snapshots

### Core Concept

Create **immutable request snapshots** early in the request lifecycle and propagate them through the **Request Context**. This ensures all downstream consumers (Mirroring, Recording, Parity Check) use identical data without re-reading the request body.

### Architecture (Context-Only)

```
┌─────────────────┐
│ Original Request│
└─────────┬───────┘
          │
          ▼
┌─────────────────┐    ┌──────────────────┐
│ Snapshot Creator│───▶│ Request Context  │
│  (Middleware)   │    │ (Pointer-based)  │
└─────────┬───────┘    └──────────────────┘
          │                      │
          ▼                      │ (Safe for async)
┌─────────────────┐              │
│   Middleware    │◀─────────────┘
│     Chain       │
└─────────┬───────┘
          │
      ┌───▼────┐ ┌─────────┐ ┌──────────────┐
      │ Local  │ │ Mirror  │ │ Recording    │
      │Handler │ │Execution│ │  System      │
      └────────┘ └─────────┘ └──────────────┘
```

### Request Snapshot Structure

```go
type RequestSnapshot struct {
    Method    string
    URL       *url.URL
    Headers   http.Header
    Body      []byte
    Host      string
    Timestamp time.Time
}

// Typed key for context safety
type contextKey struct{ name string }
var SnapshotKey = &contextKey{"request_snapshot"}
```

### Implementation Strategy

#### Phase 1: Snapshot Middleware

```go
func (s *Server) SnapshotMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Capture body once with size limit (e.g. 2MB)
        body, _ := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024))
        r.Body.Close()

        // 2. Create snapshot
        snapshot := &RequestSnapshot{
            Method:    r.Method,
            URL:       cloneURL(r.URL),
            Headers:   r.Header.Clone(),
            Body:      body,
            Host:      r.Host,
            Timestamp: time.Now(),
        }

        // 3. Inject pointer into context
        ctx := context.WithValue(r.Context(), SnapshotKey, snapshot)

        // 4. Restore r.Body for downstream compatibility
        r = r.WithContext(ctx)
        r.Body = io.NopCloser(bytes.NewReader(snapshot.Body))

        next.ServeHTTP(w, r)
    })
}
```

#### Phase 2: Downstream Consumption

Consumers (Mirror/Record) retrieve the snapshot directly from context:

```go
snapshot, ok := r.Context().Value(SnapshotKey).(*RequestSnapshot)
if ok {
    // Use snapshot.Body directly instead of io.ReadAll(r.Body)
}
```

## Hardware Considerations (Raspberry Pi Zero 2W)

To protect MicroSD health and optimize for limited memory:

1. **No Intermediate Disk Storage**: Snapshots exist only in memory; they are never written to disk until the final `.http` recording is generated.
2. **Memory Management**: Use `sync.Pool` for temporary buffers to reduce GC churn on the single-core/low-memory SoC.
3. **Automatic Cleanup**: Snapshots are naturally garbage collected once the Request Context and all child goroutines (detached mirrors/recordings) finish.
4. **Body Capping**: Strict limits on snapshot size prevent OOM (Out-of-Memory) conditions.

#### Phase 2: Response Capture System

```go
type ResponseRecorder struct {
    http.ResponseWriter
    snapshot     *ResponseSnapshot
    snapshotID   string
    source       string
    startTime    time.Time
}

func (r *ResponseRecorder) WriteHeader(statusCode int) {
    r.snapshot.StatusCode = statusCode
    r.snapshot.Headers = r.Header().Clone()
    r.ResponseWriter.WriteHeader(statusCode)
}

func (r *ResponseRecorder) Write(data []byte) (int, error) {
    r.snapshot.Body = append(r.snapshot.Body, data...)
    return r.ResponseWriter.Write(data)
}

func (r *ResponseRecorder) finalize() {
    r.snapshot.Duration = time.Since(r.startTime)
    r.snapshot.Timestamp = time.Now()
}
```

#### Phase 3: Recording System Integration

```go
type RecordingManager struct {
    storage   SnapshotStorage
    recorder  *Recorder
    patterns  []string
}

func (rm *RecordingManager) RecordInteraction(snapshotID string, response *ResponseSnapshot) {
    // Retrieve immutable request snapshot
    request, exists := rm.storage.Get(snapshotID)
    if !exists {
        log.Printf("Request snapshot not found: %s", snapshotID)
        return
    }

    // Record with guaranteed data integrity
    rm.recorder.RecordInteraction(request, response)
}

func (r *Recorder) RecordInteraction(req *RequestSnapshot, res *ResponseSnapshot) error {
    // Generate .http file with complete data
    var buf bytes.Buffer

    // Write request
    fmt.Fprintf(&buf, "### %s %s\n", req.Method, req.URL.String())
    fmt.Fprintf(&buf, "%s %s\n", req.Method, req.URL.String())
    fmt.Fprintf(&buf, "Host: %s\n", req.Host)

    for k, vv := range req.Headers {
        for _, v := range vv {
            fmt.Fprintf(&buf, "%s: %s\n", k, v)
        }
    }

    buf.WriteString("\n")
    buf.Write(req.Body)
    buf.WriteString("\n\n")

    // Write response
{% raw %}
    buf.WriteString("> {% \n")
{% endraw %}
    fmt.Fprintf(&buf, "    // Response: %d %s\n", res.StatusCode, http.StatusText(res.StatusCode))
    buf.WriteString("    // Headers:\n")

    for k, vv := range res.Headers {
        for _, v := range vv {
            fmt.Fprintf(&buf, "    // %s: %s\n", k, v)
        }
    }

{% raw %}
    buf.WriteString("%}\n\n")
{% endraw %}

    if len(res.Body) > 0 {
        buf.WriteString("/*\n")
        buf.Write(res.Body)
        buf.WriteString("\n*/\n")
    } else {
        buf.WriteString("// [Binary response body: 0 bytes]\n")
    }

    // Write to file
    return r.writeToFile(buf.Bytes(), req, res)
}
```

## Migration Strategy

### Phase 1: Introduce Snapshot System
- Add SnapshotMiddleware as first middleware
- Maintain existing recording system for compatibility
- Gradual migration of recording points

### Phase 2: Update Mirror System
- Modify MirrorMiddleware to use snapshots
- Ensure mirror requests use snapshot data
- Test parity between old and new systems

### Phase 3: Consolidate Recording
- Replace existing recording middleware
- Unified recording system using context-bound snapshots
- Remove duplicate body reading code

### Phase 4: Cleanup
- Remove legacy recording code
- Optimize memory usage with sync.Pool
- Performance validation on target hardware (Pi Zero)

## Benefits

1. **Zero Extra Disk IO**: Protecs MicroSD by avoiding snapshot disk persistence
2. **Memory Efficiency**: Natural lifecycle tied to Request Context
3. **Data Integrity**: Request data is captured once and remains immutable
4. **Consistency**: All consumers use identical request data
5. **Traceability**: Clear lineage from original request to all recordings
6. **Performance**: Reduces duplicate body reads and re-cloning

## Implementation Considerations

### Memory Management
- Use `sync.Pool` for byte buffers
- Strict size limits on captured bodies
- Rely on GC for snapshot cleanup

### Performance Impact
- Single body read vs multiple reads (net positive)
- Memory overhead for snapshot storage (manageable)
- Context propagation overhead (minimal)

### Backward Compatibility
- Maintain existing .http file format
- Preserve existing API contracts
- Gradual migration path

## Testing Strategy

### Unit Tests
- Snapshot creation and immutability
- Response recording accuracy
- Memory cleanup verification

### Integration Tests
- End-to-end request/response recording
- Mirror functionality with snapshots
- Parity validation between old/new systems

### Performance Tests
- Memory usage comparison
- Throughput impact analysis
- Large request body handling

## Future Enhancements

1. **Compression**: Compress stored snapshots for memory efficiency
2. **Streaming**: Support for streaming request/response bodies
3. **Filtering**: Selective snapshot creation based on patterns
4. **Analytics**: Request/response analysis and metrics
5. **Export**: Snapshot export for debugging and analysis

## Conclusion

This snapshot-based approach provides a robust foundation for reliable request recording while solving the current issues with body consumption and data inconsistency. The phased implementation ensures minimal disruption while delivering immediate benefits.
