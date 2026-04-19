# HTTP Body Diff Tool

This tool extracts and compares response bodies from two `.http` files.
It supports XML and JSON normalization (pretty-printing) and automatically masks common "noisy" fields like timestamps to make actual differences easier to spot.

## Usage

```bash
go run scripts/http-diff/main.go <path/to/file1.http> <path/to/file2.http>
```

To generate a side-by-side HTML report:

```bash
go run scripts/http-diff/main.go --html report.html <path/to/file1.http> <path/to/file2.http>
```

## Features

- **Body Extraction**: Automatically finds the response body within the `/* ... */` comment block at the end of the file.
- **Side-by-Side View**: Generates an HTML report with a clear side-by-side comparison.
- **Normalization**:
  - Pretty-prints XML and JSON.
  - Trims whitespace from XML character data.
- **Noise Reduction**:
  - Automatically replaces ISO 8601 timestamps with `[TIMESTAMP]`.
  - Masks specific XML tags: `<updatedOn>`, `<createdOn>`, `<lastModified>`, `<timestamp>`.
  - Masks specific JSON keys: `timestamp`, `updatedOn`, `createdOn`, `expires_at`.
- **Diff Output**:
  - Displays a line-by-line diff.
  - Show context for unchanged parts (first and last two lines, with `...` in between).
  - Uses `+` for additions and `-` for deletions.
