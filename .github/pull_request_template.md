## Description

Brief description of the changes in this PR.

## Type of Change

Please check the type of change your PR introduces:

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Performance improvement
- [ ] Code refactoring (no functional changes)
- [ ] Test improvements
- [ ] Build/CI improvements

## Related Issues

- Fixes #[issue number]
- Relates to #[issue number]
- Part of #[issue number]

## Changes Made

### API Changes
- [ ] Added new endpoints
- [ ] Modified existing endpoints
- [ ] Added new CLI commands
- [ ] Modified existing CLI commands
- [ ] Added new configuration options

### Implementation Details
- Describe the main changes
- List any new dependencies
- Mention any architectural changes

## Testing

### Automated Tests
- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] All existing tests pass
- [ ] Test coverage maintained or improved

### Manual Testing
- [ ] Tested with real SoundTouch device(s)
- [ ] Tested CLI changes manually
- [ ] Tested in different network environments

**Device(s) tested with:**
- Device model: [e.g. SoundTouch 10]
- Device IP: [e.g. 192.0.2.100]
- Test results: [brief description]

### Test Commands
```bash
# Commands used to test this change
make test
go test ./pkg/client -v -run TestNewFeature
soundtouch-cli --host 192.0.2.100 new-command
```

## Documentation

- [ ] Updated relevant documentation
- [ ] Added code comments for complex logic
- [ ] Updated CLI help text
- [ ] Added usage examples
- [ ] Updated API documentation

**Documentation files updated:**
- [ ] README.md
- [ ] docs/API-Endpoints-Overview.md
- [ ] docs/CLI-REFERENCE.md
- [ ] Code documentation (godoc)

## Backward Compatibility

- [ ] This change is backward compatible
- [ ] This change includes breaking changes (requires major version bump)
- [ ] This change requires configuration migration

**Breaking changes (if any):**
- Describe what breaks
- Provide migration instructions

## Security Considerations

- [ ] No security implications
- [ ] Security review required
- [ ] Added input validation
- [ ] Updated authentication/authorization

## Performance Impact

- [ ] No performance impact
- [ ] Performance improvement
- [ ] Potential performance regression (justify why)

**Performance notes:**
- Measured impact: [benchmarks, timing, memory usage]
- Optimization opportunities: [if any]

## Code Quality

- [ ] Code follows project style guidelines
- [ ] No linting errors
- [ ] No security warnings
- [ ] Memory leaks checked (if applicable)

### Pre-submission Checklist

- [ ] `make check` passes (format, lint, vet)
- [ ] `make test` passes
- [ ] No TODO comments left in production code
- [ ] Error handling is comprehensive
- [ ] Logging is appropriate (not too verbose, not too quiet)

## Deployment Notes

Any special considerations for deployment:
- Configuration changes required
- Database migrations needed
- Service restart required
- Rollback procedures

## Screenshots (if applicable)

If this PR includes UI changes or CLI output changes, include screenshots or terminal output examples.

```bash
# Before
$ soundtouch-cli old-command
Old output...

# After  
$ soundtouch-cli new-command
New improved output...
```

## Additional Notes

Any additional information that reviewers should know:
- Design decisions and trade-offs
- Future work planned
- Alternative approaches considered
- References to external documentation

## Review Requests

**Areas that need special attention:**
- [ ] Error handling logic
- [ ] Performance critical sections
- [ ] Security implications
- [ ] API design choices
- [ ] Documentation clarity

**Specific questions for reviewers:**
1. Question about design choice X?
2. Is error handling sufficient in section Y?
3. Should we consider alternative approach Z?

---

**Reviewer Guidelines:**
- Check that all tests pass
- Verify documentation is updated
- Test manually if device access available  
- Consider backward compatibility
- Evaluate error handling and edge cases