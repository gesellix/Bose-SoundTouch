# CLAUDE.md - Development Guidelines for Bose SoundTouch Project

## Documentation Overview

This document contains important development guidelines for working on the Bose SoundTouch project. Please also read the following documentation:

- **[PLAN.md](PLAN.md)** - Project planning and roadmap
- **[PROJECT-PATTERNS.md](PROJECT-PATTERNS.md)** - Project structure and design patterns
- **[API-Endpoints-Overview.md](API-Endpoints-Overview.md)** - API endpoints overview
- **[SoundTouch Web API.pdf](2025.12.18%20SoundTouch%20Web%20API.pdf)** - Official API documentation

## Development Guidelines

### 1. Tests are Mandatory

- **Implementation always with tests**: Every new functionality must be developed with corresponding tests
- **Unit tests preferred**: Where possible, unit tests should be written
- **Integration tests as alternative**: If unit tests are not practical, implement integration tests via mock servers
- **Sample data from live system**: Request/response data can be taken from a real SoundTouch system as examples
- **Respect privacy**: All personal data must be anonymized before use in tests

### 2. Cross-Platform Compatibility

The project must work on the following platforms:
- **Windows**
- **macOS** 
- **Linux**
- **WASM** (WebAssembly)

Platform-specific implementations are only allowed in justified exceptional cases.

### 3. KISS Principle (Keep It Simple, Stupid)

- Simplicity has top priority
- Complex solutions only when absolutely necessary
- Code should be self-explanatory and well readable
- Avoid over-engineering

### 4. Small Steps and Communication

- **Small, iterative steps**: Break large features into smaller, testable units
- **Don't hallucinate**: Don't make assumptions about unclear requirements
- **Ask instead of guess**: Always ask when unclear instead of speculating
- **Transparency**: Openly communicate uncertainties and limitations

### 5. Use Current Libraries

- Use current and well-maintained libraries where possible
- Regularly update outdated dependencies
- Apply security updates promptly
- Ensure compatibility with Go modules

### 6. Web-Specific Implementation

For web components:
- **Prefer plain HTML/JS/CSS**: Avoid heavy frameworks where possible
- Use modern web standards (ES6+, CSS Grid/Flexbox)
- Apply progressive enhancement
- Consider accessibility (a11y)
- Implement responsive design

## Additional Notes

- **Language: English** for code, commits, labels, and text in code
- Code comments in English
- **Documentation**: Completely in English for international accessibility
- Conduct regular code reviews
- Consider performance from the beginning

