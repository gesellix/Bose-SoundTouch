### Overview of Recent Improvements and Next Steps

This document summarizes the improvements made to the **Marge service** to improve parity with the upstream Bose SoundTouch service, along with open issues and proposed next steps.

#### ✅ Completed Improvements (Marge Service)
*   **Timestamp-based ID Generation**: Implemented a 9-digit ID schema (`YYMMDD` + 3-digit counter) for `recent` items, ensuring IDs are large, unique, and stay within the 32-bit integer range.
*   **Automatic Source Learning**: The service now extracts and persists full metadata (credentials, provider IDs, and custom names) from incoming `POST /recent` requests. This improves parity for subsequent `GET /recents` calls.
*   **Source Provider Mapping**: Synchronized local source provider IDs and timestamps with upstream data. The `RADIO_BROWSER` provider is included in the public `/streaming/sourceproviders` list to maintain internal functionality while acknowledging it as a parity gap.
*   **Credential Preservation**: Improved `AddRecent` to correctly extract and echo back base64 tokens/credentials provided in the incoming request, improving source learning.
*   **XML Formatting Parity**:
    *   Added `standalone="yes"` to the XML declaration for all Marge responses, including `recent`, `presets`, `full account`, `software update`, and `sourceproviders`.
    *   Enforced self-closing `<sourceSettings/>` tags for parity.
    *   Standardized date formatting to UTC with milliseconds (`.000+00:00`).
    *   Fixed casing for `/streaming/sourceproviders`: Root element is `<sourceProviders>`, but child elements are `<sourceprovider>` (all lowercase), matching upstream behavior.
    *   Implemented structured XML marshaling with consistent 2-space indentation for recents and source providers.
*   **Improved TuneIn Parity**: Fixed TuneIn source mapping to use ID `25` and ensuring `sourcename` is empty in responses, matching upstream behavior for station playback.
*   **High-Fidelity Full Account Sync**: Refactored the `/streaming/account/{accountId}/full` response to match the upstream structure. This includes:
    *   **Structured XML Marshaling**: Replaced manual string concatenation with structured Go models and `xml.Marshal` for the entire response.
    *   **Specific Response Models**: Introduced `FullResponseSource`, `FullResponsePreset`, and `FullResponseRecent` to accurately reflect the upstream structure where `<source>` is a child element, rather than a set of attributes.
    *   **Correct Nesting**: Ensured that `<presets>` and `<recents>` correctly nest their associated `<source>` details, resolving previous data omissions.
    *   **Device Identity**: Added `<serialNumber>` and `<updatedOn>` to both the top-level `<device>` and its `<attachedProduct>`, ensuring consistent device identification.
    *   **Field-Level Parity**: Mapped missing fields like `<contentItemType>` and `<productlabel>` to match upstream expectations.
    *   **Improved Source Matching**: Enhanced internal logic to correctly link presets and recents to their configured sources based on multiple identifiers (ID, Key, or Type).
*   **Verified Parity Mismatch Fixes**: Comprehensive reproduction tests (`TestParityMismatchReproduction_V2` and `TestParityMismatchReproduction_V3`) now confirm parity for identified mismatches in `POST /recent` and `GET /recents`, including credentials and source-specific metadata.
*   **Unified Response Logic**: Refactored the code so that both `POST /recent` and `GET /recents` use the same formatting functions, guaranteeing consistency.
*   **Robust Parity Detection**: Updated the local parity checker to be whitespace-insensitive for XML bodies, significantly reducing noise from minor indentation or newline differences.
*   **Maintainable XML Generation**: Reduced cyclomatic complexity and code duplication in `marge.go` by extracting focused helper functions for mapping internal data to response-specific XML models.

---

#### 🛠️ Open Issues and Next Steps

Based on the latest `parity_mismatches`, here are the recommended areas for further work:

#### 1. BMX / TuneIn Playback Parity (Medium)
Current mismatches in `/bmx/tunein/v1/playback/station/...` show differences in reporting URLs and missing links:
*   **Mismatched Parameters**: Local reporting URLs use `listen_id=3432432423`, while upstream uses a different session-based ID.
*   **Missing Links**: Some upstream responses include additional `_links` or metadata that are currently omitted in local responses.
*   **Action**: Improve the `HandleTuneInPlayback` logic to better mirror the upstream response structure and parameter generation.

#### 2. Presets and Recents Parity (Medium)
Further align the standalone `GET /presets` and `GET /recents` endpoints with the refined structural improvements introduced for the `/full` account response:
*   **Source Nesting**: Ensure the standalone responses also use the specialized nested `<source>` structure instead of mixed attributes when appropriate.
*   **Field Completeness**: Verify all metadata fields (e.g., `<contentItemType>`, `<lastplayedat>`) are consistently populated across all access paths.
*   **Action**: Evaluate if the specialized `FullResponsePreset` and `FullResponseRecent` models should be shared or mirrored in the standalone handlers.

#### 3. OAuth / Spotify Token Noise (Low/Medium)
The `/oauth/device/.../token` endpoint frequently reports mismatches because tokens are naturally different between local and upstream.
*   **The Issue**: This creates "noise" in your parity reports that isn't actually a bug.
*   **Action**: Update the parity detection logic (or the handler) to selectively ignore the `access_token` field while still verifying that the rest of the JSON structure (expires_in, scope, token_type) matches.

#### 4. Large IDs for Other Models (Medium)
While we fixed IDs for `recents`, other models like `presets` or `sources` might still use small auto-incrementing integers.
*   **Action**: Evaluate if other endpoints should also transition to the timestamp-based ID schema to further reduce diff noise.

#### 5. Improved Data Persistence (Continuous)
Continue the "learning" approach for other services. For example, if we see a new `sourceproviderid` in a Spotify or TuneIn request, we should ensure it is stored and reused.

#### 6. Local Reboot & Device State Management (Continuous)
Analysis of device reboot logs revealed several data requirements:
*   **Power-On Details Tracking**: Implemented extraction and persistence of detailed device information (serial numbers, firmware version, product details, and MAC addresses) from the `POST /streaming/support/power_on` request. This data is now stored in the local datastore, improving our ability to respond accurately to subsequent management requests.
*   **Source Provider Mapping**: Synchronized local source provider IDs and timestamps with upstream data. The `RADIO_BROWSER` provider is included in the public `/streaming/sourceproviders` list to maintain internal functionality while acknowledging it as a parity gap.
