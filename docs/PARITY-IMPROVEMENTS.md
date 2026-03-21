### Overview of Recent Improvements and Next Steps

This document summarizes the improvements made to the **Marge service** to improve parity with the upstream Bose SoundTouch service, along with open issues and proposed next steps.

#### ✅ Completed Improvements (Marge Service)
*   **Mapped Preset `buttonNumber`**: Correctly mapped the internal `ServicePreset.ID` or `ButtonNumber` to the `buttonNumber` XML attribute in the `/full` response and ensured it is persisted in the local datastore.
*   **High-Fidelity Device Metadata**: Improved the datastore to correctly extract, persist, and report detailed device `<components>` (e.g., `LIGHTSWITCH`, `SMSC`) and their firmware versions from upstream responses.
*   **Standardized Preferred Language**: Updated the default `preferredLanguage` to `de` in the `/full` response and added synchronization to persist it from upstream responses.
*   **Persisted Provider Settings**: Added support for persisting and echoing back `providerSettings` (e.g., `STREAMING_QUALITY`, `ELIGIBLE_FOR_TRIAL`) from the `/full` response.
*   **Populated `contentItemType`**: The `contentItemType` (e.g., `tracklisturl`) is now correctly synchronized from upstream, persisted in the local datastore, and returned in the `/full` response for both presets and recents.
*   **Standardized Credential Types**: Adjusted the logic for Spotify to use the correct `token_version_3` type when a token is present in the `/full` response, improving parity with the upstream service. The service now respects existing `credential_type` values from `Sources.xml` (e.g., `token_version_3` for Spotify) while providing sensible defaults for new or incomplete sources.
*   **Structured Sources (Sources.xml)**: Refactored `Sources.xml` to use an attribute-based structure (`sourceid`, `source`, `status`, `sourceAccount`, etc.) matching the real device's output. Removed redundant nested tags like `<sourcename>`, `<username>`, and `<name>`.
*   **Nested Recents (Recents.xml)**: Implemented a nested `<contentItem>` structure within `<recent>` entries in `Recents.xml`, maintaining exact parity with the device's persistence format while supporting legacy flat formats for backward compatibility.
*   **Inconsistent `serialNumber` Casing**: Fixed the casing mismatch in the `/full` response where the upstream uses camelCase `<serialNumber>` in the top-level `<device>` and lowercase `<serialnumber>` in the nested `<attachedProduct>`. Local responses now correctly mirror this inconsistency.
*   **Attribute-level Parity**:
    *   Ensured `sourceAccount=""` is preserved in XML even when empty, matching device behavior for sources like TUNEIN.
    *   Fixed casing for attributes like `deviceID` and `utcTime` in `Recents.xml`.
    *   Correctly mapped and persisted preset and recent `id` attributes during "Initial Data Sync".
*   **Device Name Consistency**: Fixed an issue where the device `<name>` was empty in some local `/full` responses by ensuring it is correctly populated from the datastore and synchronized from upstream.
*   **Improved XML Parity**: Empty `<name>` tags in the `/full` response are now self-closing (`<name/>`), matching upstream behavior.
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
    *   **Mapped Preset `buttonNumber`**: Correctly mapped the internal `ServicePreset.ID` to the `buttonNumber` XML attribute in the `/full` response.
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

Based on the latest `parity_mismatches` and the high-fidelity `/full` account response comparison (diff14), here are the recommended areas for further work:

#### 1. BMX / TuneIn Playback Parity (Medium)
Current mismatches in `/bmx/tunein/v1/playback/station/...` show differences in reporting URLs and missing links:
*   **Mismatched Parameters**: Local reporting URLs use `listen_id=1234567890`, while upstream uses a different session-based ID.
*   **Missing Links**: Some upstream responses include additional `_links` or metadata that are currently omitted in local responses.
*   **Action**: Improve the `HandleTuneInPlayback` logic to better mirror the upstream response structure and parameter generation.

#### 2. `/full` Account Response Data Gaps (Medium)
While structural parity for the `/full` response is high, several value-level gaps remain as shown in `diff14`:
*   **Timestamp Formats**: Upstream uses ISO-8601 with milliseconds (e.g., `2024-06-23T07:40:36.000+00:00`), whereas some local fields still use Unix epoch integers (e.g., `1234567890`).
*   **Provider Settings**: The `providerSettings` block in the local response currently lacks crucial values like `keyName`, `providerId`, and `boseId` (appearing as empty tags).
*   **Component Metadata**: Local component types are sometimes empty (`type=""`) compared to upstream values like `LIGHTSWITCH` or `SMSC`.
*   **Source/Preset Identifiers**: Local IDs (e.g., `100004`) differ from upstream IDs (e.g., `1234567`), though this may be expected due to different account/device environments.
*   **Action**: Update the mapping logic in `marge.go` and `setup.go` to ensure all fields in the `/full` response are correctly populated with high-fidelity values and standard ISO-8601 timestamps.

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

#### 7. Account Full Response (/full) Structural & Value Parity (Completed)
Structural and value gaps in the `/full` account response have been addressed:

**Key Fixes:**
*   **Structural**:
    *   **Nested Source Association**: Improved the matching logic in `mapRecentsToFullResponse` to correctly link recents to their specific `ConfiguredSource` (e.g., by matching `sourceid` attribute).
    *   **XML Tag Formatting**: Standardized self-closing tags and element formatting to match upstream's multi-line or empty-element formatting in various contexts.
