package stereopair

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// DeleteMargeGroupGeneration removes one exact group generation from the
// backend configured by the freshly read speaker /info response.
func DeleteMargeGroupGeneration(httpClient *http.Client, ref GenerationRef) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	current, err := getMargeDeviceGroup(httpClient, ref)
	if err != nil {
		return fmt.Errorf("verify Marge group generation before deletion: %w", err)
	}

	if current.IsEmpty() {
		return nil
	}

	if ref.ExpectedGroup == nil || !sameGroupTopology(current, ref.ExpectedGroup) ||
		current.ID != ref.GroupID || current.MasterDeviceID != ref.DeviceID ||
		!groupContainsDevice(current, ref.DeviceID) {
		return fmt.Errorf("%w: delete Marge group generation: device is associated with unrelated generation or topology %q",
			ErrConflict, current.ID)
	}

	endpoint, err := MargeGroupGenerationURL(ref)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create Marge generation cleanup request: %w", err)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("delete Marge group generation: %w", err)
	}

	if response.StatusCode == http.StatusNotFound ||
		(response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices) {
		_, copyErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()

		if copyErr != nil {
			return fmt.Errorf("read Marge generation cleanup response: %w", copyErr)
		}

		if closeErr != nil {
			return fmt.Errorf("close Marge generation cleanup response: %w", closeErr)
		}

		return verifyMargeGroupGenerationDeleted(httpClient, ref)
	}

	body, err := readMargeGenerationCleanupResponse(response)
	if err != nil {
		return err
	}

	return handleMargeGenerationCleanupFailure(httpClient, ref, response.StatusCode, body)
}

func handleMargeGenerationCleanupFailure(
	httpClient *http.Client,
	ref GenerationRef,
	statusCode int,
	body []byte,
) error {
	if statusCode == http.StatusInternalServerError && margeWrappedGroupNotFound(body, ref) {
		return verifyMargeGroupGenerationDeleted(httpClient, ref)
	}

	if statusCode == http.StatusConflict {
		return fmt.Errorf("%w: delete Marge group generation: HTTP %d: %s",
			ErrConflict, statusCode, strings.TrimSpace(string(body)))
	}

	return fmt.Errorf("delete Marge group generation: HTTP %d: %s",
		statusCode, strings.TrimSpace(string(body)))
}

func readMargeGenerationCleanupResponse(response *http.Response) ([]byte, error) {
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1025))
	closeErr := response.Body.Close()

	if readErr != nil {
		return nil, fmt.Errorf("read Marge generation cleanup response: %w", readErr)
	}

	if closeErr != nil {
		return nil, fmt.Errorf("close Marge generation cleanup response: %w", closeErr)
	}

	if len(body) > 1024 {
		return nil, fmt.Errorf("delete Marge group generation: HTTP %d response exceeds 1024 bytes", response.StatusCode)
	}

	return body, nil
}

func verifyMargeGroupGenerationDeleted(httpClient *http.Client, ref GenerationRef) error {
	group, err := getMargeDeviceGroup(httpClient, ref)
	if err != nil {
		return fmt.Errorf("verify Marge group generation deletion: %w", err)
	}

	if group.IsEmpty() || group.ID != ref.GroupID {
		return nil
	}

	return fmt.Errorf("%w: delete Marge group generation: generation %s is still active", ErrConflict, ref.GroupID)
}

func margeWrappedGroupNotFound(body []byte, ref GenerationRef) bool {
	var response struct {
		XMLName xml.Name
		Message string `xml:",chardata"`
	}

	decoder := xml.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&response); err != nil || response.XMLName.Local != "error" {
		return false
	}

	for {
		token, err := decoder.Token()

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return false
		}

		if data, ok := token.(xml.CharData); !ok || strings.TrimSpace(string(data)) != "" {
			return false
		}
	}

	want := fmt.Sprintf("Unexpected error: 404: Group %s does not exist in account %s", ref.GroupID, ref.AccountID)

	return strings.TrimSpace(response.Message) == want
}

// RenameMargeGroupGeneration updates and verifies the name of one persisted
// generation at the backend configured by fresh speaker info. Topology, rather
// than the old name, is the retry guard so a degraded rename can converge.
func RenameMargeGroupGeneration(httpClient *http.Client, ref GenerationRef, name string) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Errorf("%w: persisted group name must not be empty", ErrInvalidRequest)
	}

	current, err := getMargeDeviceGroup(httpClient, ref)
	if err != nil {
		return fmt.Errorf("verify Marge group generation before rename: %w", err)
	}

	if current.IsEmpty() || ref.ExpectedGroup == nil || current.ID != ref.GroupID ||
		current.MasterDeviceID != ref.DeviceID || !groupContainsDevice(current, ref.DeviceID) ||
		!sameGroupTopology(current, ref.ExpectedGroup) {
		return fmt.Errorf("%w: rename Marge group generation: device is associated with unrelated generation or topology %q",
			ErrConflict, current.ID)
	}

	if current.Name == name {
		return nil
	}

	updated := cloneGroup(current)
	updated.Name = name
	updated.Status = ""
	updated.SenderIPAddress = ""

	body, err := xml.Marshal(updated)
	if err != nil {
		return fmt.Errorf("encode Marge group rename: %w", err)
	}

	mutationErr, err := postMargeGroupRename(httpClient, ref, body)
	if err != nil {
		return err
	}

	verified, verifyErr := getMargeDeviceGroup(httpClient, ref)
	if verifyErr == nil && verified.Name == name && sameGroupTopology(verified, updated) {
		return nil
	}

	if mutationErr != nil {
		return fmt.Errorf("rename Marge group generation: %w", mutationErr)
	}

	if verifyErr != nil {
		return fmt.Errorf("verify Marge group generation rename: %w", verifyErr)
	}

	return fmt.Errorf("%w: rename Marge group generation: generation %s did not retain name %q",
		ErrConflict, ref.GroupID, name)
}

func postMargeGroupRename(httpClient *http.Client, ref GenerationRef, body []byte) (error, error) {
	endpoint, err := MargeGroupGenerationURL(ref)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(xml.Header+string(body)))
	if err != nil {
		return nil, fmt.Errorf("create Marge group rename request: %w", err)
	}

	request.Header.Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")

	response, requestErr := httpClient.Do(request)
	if requestErr != nil {
		return requestErr, nil
	}

	return margeGroupRenameResponseError(response), nil
}

func margeGroupRenameResponseError(response *http.Response) error {
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1024))

	closeErr := response.Body.Close()

	switch {
	case response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices:
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	case readErr != nil:
		return fmt.Errorf("read response: %w", readErr)
	case closeErr != nil:
		return fmt.Errorf("close response: %w", closeErr)
	default:
		return nil
	}
}

// MargeGroupGenerationURL returns the standard generation-aware group endpoint
// below a speaker's configured Marge base URL.
func MargeGroupGenerationURL(ref GenerationRef) (string, error) {
	if !safeMargePathSegment(ref.AccountID) || !safeMargePathSegment(ref.GroupID) {
		return "", errors.New("speaker info has no safe Marge account or group ID")
	}

	return margeStreamingURL(ref.MargeURL, "account", ref.AccountID, "group", ref.GroupID)
}

// EnsureMargeNoGroupGenerations checks that the backend has no persisted group
// for speakers whose fresh physical group state is empty. The check is
// deliberately read-only so it cannot retire a concurrently created physical
// generation.
func EnsureMargeNoGroupGenerations(httpClient *http.Client, refs []GenerationRef) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	for i := range refs {
		ref := refs[i]

		group, err := getMargeDeviceGroup(httpClient, ref)
		if err != nil {
			return err
		}

		if group.IsEmpty() {
			continue
		}

		if !safeMargePathSegment(group.ID) || !groupContainsDevice(group, ref.DeviceID) {
			return errors.New("marge returned an unsafe or unrelated stale group generation")
		}

		return fmt.Errorf("persisted group generation %s still contains device %s",
			group.ID, ref.DeviceID)
	}

	return nil
}

func getMargeDeviceGroup(httpClient *http.Client, ref GenerationRef) (*models.Group, error) {
	if !safeMargePathSegment(ref.AccountID) || !safeMargePathSegment(ref.DeviceID) {
		return nil, errors.New("speaker info has no safe Marge account or device ID")
	}

	endpoint, err := margeStreamingURL(ref.MargeURL, "account", ref.AccountID, "device", ref.DeviceID, "group")
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create Marge group query: %w", err)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query Marge group generation: %w", err)
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	closeErr := response.Body.Close()

	if readErr != nil {
		return nil, fmt.Errorf("read Marge group generation: %w", readErr)
	}

	if closeErr != nil {
		return nil, fmt.Errorf("close Marge group response: %w", closeErr)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("query Marge group generation: HTTP %d: %s",
			response.StatusCode, strings.TrimSpace(string(body)))
	}

	var group models.Group
	if err := xml.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("decode Marge group generation: %w", err)
	}

	return &group, nil
}

func margeStreamingURL(margeURL string, segments ...string) (string, error) {
	for _, segment := range segments {
		if !safeMargePathSegment(segment) {
			return "", errors.New("unsafe Marge URL path segment")
		}
	}

	endpoint, err := url.Parse(strings.TrimSpace(margeURL))
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return "", fmt.Errorf("speaker info has no usable Marge URL %q", margeURL)
	}

	basePath := strings.TrimRight(endpoint.Path, "/")
	if !strings.HasSuffix(basePath, "/streaming") {
		basePath = path.Join(basePath, "streaming")
	}

	allSegments := append([]string{basePath}, segments...)
	endpoint.Path = path.Join(allSegments...)
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	return endpoint.String(), nil
}

func safeMargePathSegment(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && value != "." && value != ".." &&
		!strings.ContainsAny(value, "/?#\\\x00\r\n")
}
