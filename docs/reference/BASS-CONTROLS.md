# Bass Control Guide

## Overview

The Bose SoundTouch Go client provides comprehensive bass control functionality through the `GET /bass` and `POST /bass` endpoints. This feature allows you to adjust bass levels from -9 (maximum bass cut) to +9 (maximum bass boost) with full validation and safety features.

## Implementation Status

✅ **Complete** - All bass control functionality implemented and tested
- Bass level retrieval with `GetBass()`
- Bass level adjustment with `SetBass()`
- Increment/decrement methods with safety limits
- CLI flags for easy bass management
- Real device validation with SoundTouch hardware
- Comprehensive error handling and range validation

## API Endpoints

### GET /bass

**Purpose**: Retrieve current bass settings

**Response Format:**
```xml
<bass deviceID="ABCD1234EFGH">
  <targetbass>3</targetbass>
  <actualbass>3</actualbass>
</bass>
```

**Fields:**
- `targetbass` - The desired bass level (-9 to +9)
- `actualbass` - The current actual bass level (may differ during adjustment)
- `deviceID` - Unique device identifier

### POST /bass

**Purpose**: Set bass level

**Request Format:**
```xml
<bass>5</bass>
```

**Valid Range**: -9 to +9
- **-9 to -1**: Bass cut (reduces bass frequencies)
- **0**: Neutral/flat bass response
- **+1 to +9**: Bass boost (enhances bass frequencies)

**Response**: HTTP 200 OK (no body) on success

## Client Library Usage

### Basic Bass Control

```go
import "github.com/gesellix/bose-soundtouch/pkg/client"

// Create client
config := client.ClientConfig{
    Host: "192.168.1.100",
    Port: 8090,
}
c := client.NewClient(config)

// Get current bass level
bass, err := c.GetBass()
if err != nil {
    fmt.Printf("Failed to get bass: %v\n", err)
}

fmt.Printf("Bass Level: %d (%s)\n", bass.GetLevel(), bass.String())

// Set bass level
err = c.SetBass(3)
if err != nil {
    fmt.Printf("Failed to set bass: %v\n", err)
}
```

### Bass Information Methods

```go
// Get bass level information
bass, err := c.GetBass()
if err != nil {
    return err
}

// Access bass level details
level := bass.GetLevel()          // Target bass level
actual := bass.GetActualLevel()   // Current actual level
isAtTarget := bass.IsAtTarget()   // true if target == actual

// Bass categorization
isBoost := bass.IsBassBoost()     // true if level > 0
isCut := bass.IsBassCut()         // true if level < 0
isFlat := bass.IsFlat()           // true if level == 0

// Human-readable descriptions
levelName := models.GetBassLevelName(level)     // "Slightly High", "Very Low", etc.
category := models.GetBassLevelCategory(level)  // "Bass Boost", "Bass Cut", "Flat"
```

### Safe Bass Control

```go
// SetBassSafe automatically clamps values to valid range
err := c.SetBassSafe(15)  // Will be clamped to +9
err = c.SetBassSafe(-15)  // Will be clamped to -9

// Increment/decrement with automatic clamping
newBass, err := c.IncreaseBass(2)  // Increase by 2, clamp if needed
newBass, err := c.DecreaseBass(1)  // Decrease by 1, clamp if needed
```

### Validation and Limits

```go
import "github.com/gesellix/bose-soundtouch/pkg/models"

// Validate bass level before setting
if models.ValidateBassLevel(level) {
    err := c.SetBass(level)
}

// Clamp to valid range
safeLevel := models.ClampBassLevel(100)  // Returns +9

// Constants
fmt.Printf("Bass range: %d to %d\n", models.BassLevelMin, models.BassLevelMax)  // -9 to 9
fmt.Printf("Default: %d\n", models.BassLevelDefault)  // 0
```

## CLI Usage

### Basic Commands

```bash
# Get current bass level
soundtouch-cli -host 192.168.1.100 -bass

# Set specific bass level
soundtouch-cli -host 192.168.1.100 -set-bass 3
soundtouch-cli -host 192.168.1.100 -set-bass -5

# Increment/decrement bass
soundtouch-cli -host 192.168.1.100 -inc-bass 1
soundtouch-cli -host 192.168.1.100 -dec-bass 2
```

### Real Examples

```bash
# Check current bass settings
soundtouch-cli -host 192.168.1.100 -bass
# Output: Bass Level: 0 (Neutral)
#         Category: Flat

# Set bass boost
soundtouch-cli -host 192.168.1.100 -set-bass 6
# Output: ✓ Bass set to 6 (High)

# Reset to neutral
soundtouch-cli -host 192.168.1.100 -set-bass 0
# Output: ✓ Bass set to 0 (Neutral)

# Gradual bass adjustment
soundtouch-cli -host 192.168.1.100 -inc-bass 2
# Output: ✓ Bass increased to 2 (Slightly High)
```

### CLI Flags

| Flag | Description | Range | Default |
|------|-------------|-------|---------|
| `-bass` | Get current bass level | N/A | N/A |
| `-set-bass <level>` | Set bass level | -9 to +9 | N/A |
| `-inc-bass <amount>` | Increase bass | 1-3 | 1 |
| `-dec-bass <amount>` | Decrease bass | 1-3 | 1 |

### Safety Features

- **Validation**: Invalid ranges are rejected before sending to device
- **Clamping**: Values are automatically clamped to valid range with `-safe` methods
- **Limits**: Increment/decrement amounts are limited for safety (max 3 per command)
- **Error Messages**: Clear error messages for invalid inputs

## Bass Level Reference

### Level Descriptions

| Level | Name | Category | Description |
|-------|------|----------|-------------|
| -9 to -7 | Very Low | Bass Cut | Maximum bass reduction |
| -6 to -4 | Low | Bass Cut | Moderate bass reduction |
| -3 to -1 | Slightly Low | Bass Cut | Mild bass reduction |
| 0 | Neutral | Flat | No bass adjustment |
| 1 to 3 | Slightly High | Bass Boost | Mild bass enhancement |
| 4 to 6 | High | Bass Boost | Moderate bass enhancement |
| 7 to 9 | Very High | Bass Boost | Maximum bass enhancement |

### Practical Usage Guidelines

**For Different Music Genres:**
- **Classical/Acoustic**: -1 to +1 (subtle adjustments)
- **Rock/Pop**: +2 to +4 (moderate bass boost)
- **Electronic/Hip-Hop**: +4 to +6 (strong bass enhancement)
- **Vocals/Podcasts**: -2 to 0 (reduce bass for clarity)

**For Different Environments:**
- **Small rooms**: -1 to +2 (avoid overwhelming bass)
- **Large rooms**: +3 to +6 (compensate for space)
- **Near-field listening**: 0 to +2 (balanced response)
- **Background music**: -2 to +1 (non-intrusive)

## Integration Examples

### Smart Bass Management

```go
func adjustBassForContent(client *client.Client, contentType string) error {
    var targetBass int
    
    switch contentType {
    case "music":
        targetBass = 3  // Moderate bass boost for music
    case "podcast":
        targetBass = -1 // Slight bass cut for voice clarity
    case "movie":
        targetBass = 5  // Strong bass for movie experience
    default:
        targetBass = 0  // Neutral for unknown content
    }
    
    return client.SetBass(targetBass)
}
```

### Bass Presets

```go
type BassPreset struct {
    Name  string
    Level int
}

var bassPresets = []BassPreset{
    {"Flat", 0},
    {"Voice", -2},
    {"Music", 3},
    {"Movies", 5},
    {"Heavy", 7},
}

func applyBassPreset(client *client.Client, presetName string) error {
    for _, preset := range bassPresets {
        if preset.Name == presetName {
            return client.SetBass(preset.Level)
        }
    }
    return fmt.Errorf("preset not found: %s", presetName)
}
```

### Gradual Bass Adjustment

```go
func gradualBassChange(client *client.Client, targetLevel int, stepSize int) error {
    currentBass, err := client.GetBass()
    if err != nil {
        return err
    }
    
    current := currentBass.GetLevel()
    
    for current != targetLevel {
        var step int
        if targetLevel > current {
            step = min(stepSize, targetLevel-current)
            _, err = client.IncreaseBass(step)
        } else {
            step = min(stepSize, current-targetLevel)
            _, err = client.DecreaseBass(step)
        }
        
        if err != nil {
            return err
        }
        
        // Update current level
        currentBass, err = client.GetBass()
        if err != nil {
            return err
        }
        current = currentBass.GetLevel()
        
        time.Sleep(200 * time.Millisecond) // Brief pause between adjustments
    }
    
    return nil
}
```

## Error Handling and Troubleshooting

### Common Error Scenarios

#### 1. Invalid Range Errors
```go
err := client.SetBass(15)
// Error: invalid bass level: 15 (must be between -9 and 9)
```

**Solution**: Use valid range (-9 to +9) or `SetBassSafe()` for auto-clamping.

#### 2. Device Connection Errors
```bash
soundtouch-cli -host 192.168.1.100 -bass
# Error: failed to get bass: API request failed with status 404
```

**Solutions:**
- Verify device IP address and port
- Check network connectivity
- Ensure device supports bass control

#### 3. Device-Specific Behavior
Some devices may:
- Override bass settings based on source or content
- Have limited bass range despite API acceptance
- Reset bass to default when changing sources

**Solutions:**
- Test bass control with different audio sources
- Check device capabilities and documentation
- Implement retry logic for critical applications

### Debugging Tips

1. **Check Current Settings**
   ```bash
   soundtouch-cli -host <ip> -bass
   ```

2. **Test Basic Functionality**
   ```bash
   soundtouch-cli -host <ip> -set-bass 0    # Reset to neutral
   soundtouch-cli -host <ip> -set-bass 1    # Small positive adjustment
   soundtouch-cli -host <ip> -bass          # Verify change
   ```

3. **Validate Range Handling**
   ```bash
   soundtouch-cli -host <ip> -set-bass 15   # Should fail validation
   soundtouch-cli -host <ip> -set-bass -15  # Should fail validation
   ```

## Testing

### Unit Tests

Run bass control tests:
```bash
go test ./pkg/models -v -run ".*Bass.*"
go test ./pkg/client -v -run ".*Bass.*"
```

### Integration Tests

Test with real hardware:
```bash
SOUNDTOUCH_TEST_HOST=192.168.1.100 go test ./pkg/client -v -run ".*Bass.*Integration.*"
```

### Manual Testing

```bash
# Complete bass control test sequence
soundtouch-cli -discover
soundtouch-cli -host <discovered-ip> -bass
soundtouch-cli -host <discovered-ip> -set-bass 3
soundtouch-cli -host <discovered-ip> -inc-bass 1
soundtouch-cli -host <discovered-ip> -dec-bass 2
soundtouch-cli -host <discovered-ip> -bass  # Verify final state
```

## Performance

### Typical Response Times
- **GetBass()**: 50-150ms
- **SetBass()**: 100-300ms
- **IncreaseBass()/DecreaseBass()**: 200-500ms (includes GET + SET + GET)

### Best Practices

1. **Cache Current State**: Minimize GET requests by tracking state locally
2. **Batch Operations**: Group multiple bass changes when possible
3. **Validate Locally**: Use client-side validation before API calls
4. **Handle Timeouts**: Implement appropriate timeout handling for network operations

## Device Compatibility

### Tested Devices
- **SoundTouch 10**: ✅ Full bass control support
- **SoundTouch 20**: ✅ Full bass control support
- **SoundTouch 30**: Expected to work (similar API)

### Known Limitations
1. **Source Dependencies**: Some sources may override bass settings
2. **Content-Based Adjustment**: Device may auto-adjust bass based on audio content
3. **Firmware Variations**: Different firmware versions may behave differently

### Compatibility Notes
- Bass control availability depends on device capabilities
- Some devices may have restricted bass ranges
- Certain audio sources may disable manual bass control

## Related Documentation

- **[API Endpoints Overview](API-ENDPOINTS.md)** - Complete API reference
- **[Volume Controls](VOLUME-CONTROLS.md)** - Related audio control documentation
- **[Client Usage Examples](../../cmd/soundtouch-cli/main.go)** - CLI implementation reference
- **[Models](../../pkg/models/bass.go)** - Bass model implementation

## API Compliance

### XML Format Requirements
The implementation follows the official SoundTouch API:
- Uses simple `<bass>level</bass>` structure for requests
- Handles `<bass><targetbass>` and `<actualbass>` in responses
- Validates range (-9 to +9) as per specification
- Provides proper error handling for invalid values

### Standards Compliance
- **HTTP Methods**: Proper GET for retrieval, POST for setting
- **Content-Type**: Correct `application/xml` headers
- **Error Codes**: Standard HTTP status codes
- **XML Encoding**: UTF-8 encoding as expected by devices

---

**Implementation Date**: 2026-01-09  
**Status**: ✅ Complete and tested  
**Real Device Validation**: SoundTouch 10, SoundTouch 20  
**API Compliance**: Full compliance with SoundTouch Web API specification
