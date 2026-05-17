---
name: Feature request
about: Suggest an idea for this project
title: ''
labels: 'enhancement'
assignees: ''

---

**Is your feature request related to a problem? Please describe.**
A clear and concise description of what the problem is. Ex. I'm always frustrated when [...]

**Describe the solution you'd like**
A clear and concise description of what you want to happen.

**Describe alternatives you've considered**
A clear and concise description of any alternative solutions or features you've considered.

**Use case**
Describe your specific use case and how this feature would benefit you and other users.

**SoundTouch API Support**
- [ ] This feature is supported by the official SoundTouch API
- [ ] This feature is NOT supported by the SoundTouch API (custom enhancement)
- [ ] I'm not sure if this is supported by the SoundTouch API

**API Documentation Reference (if applicable)**
If this feature is based on a SoundTouch API endpoint, please provide:
- Endpoint URL: [e.g. GET /newendpoint]
- Documentation reference: [page number or section in official API docs]
- XML request/response examples: [if known]

**Implementation Details (optional)**
If you have ideas about how this could be implemented:
- Suggested package/module: [e.g. pkg/client, cmd/soundtouch-cli]
- Method signatures: [if you have suggestions]
- CLI commands: [if this affects the CLI tool]

**Device Compatibility**
- SoundTouch models this applies to: [e.g. all models, SoundTouch 20+, specific models]
- Have you tested this manually: [e.g. via curl, Postman, etc.]

**Examples**
Provide examples of how you would like to use this feature:

```go
// Go library example
client.NewFeature(parameters)
```

```bash
# CLI example
soundtouch-cli --host 192.0.2.100 new-feature --param value
```

**Priority**
- [ ] Critical - blocks important functionality
- [ ] High - would significantly improve user experience
- [ ] Medium - nice to have enhancement
- [ ] Low - minor improvement

**Additional context**
Add any other context, screenshots, or examples about the feature request here.

**Related Issues**
- Related to #[issue number]
- Depends on #[issue number]
- Blocks #[issue number]

---

**Checklist**
- [ ] I have searched existing issues to avoid duplicates
- [ ] I have checked the documentation to ensure this feature doesn't already exist
- [ ] I have provided a clear use case and rationale
- [ ] I have considered the impact on existing functionality
- [ ] I understand this may require SoundTouch API support to implement