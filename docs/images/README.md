# Images for Migration Guide

This directory contains images, screenshots, and diagrams referenced in the migration guide and other documentation.

## Required Images for Migration Guide

The following images need to be created to complete the migration guide:

### Dashboard Screenshots
- **dashboard-home.png** - Main SoundTouch Service dashboard homepage
- **account-creation.png** - Account creation form with fields filled
- **account-dashboard.png** - Fresh account dashboard showing ready state
- **device-discovery.png** - Device discovery page showing found speakers
- **device-registration.png** - Device registration dialog with options
- **migration-setup.png** - Migration configuration dialog
- **migration-progress.png** - Migration progress tracker showing phases
- **migration-health.png** - Migration health monitoring dashboard
- **account-migration.png** - Account-wide migration progress overview
- **migration-complete.png** - Completed migration dashboard view
- **backup-setup.png** - Backup configuration settings page

### Setup and Preparation
- **usb-remote-services.png** - USB drive setup showing file structure
- **raspberry-pi-setup.png** - Raspberry Pi with connected cables (optional)

### Process Diagrams
- **migration-flow-diagram.png** - Flow chart showing migration phases
- **network-topology.png** - Network diagram showing Pi, router, speakers
- **data-flow-diagram.png** - How data flows between components

## Image Requirements

### Technical Specifications
- **Format**: PNG preferred for screenshots, SVG for diagrams
- **Resolution**: Minimum 1200px width for screenshots
- **File Size**: Keep under 500KB when possible for fast loading
- **Naming**: Use descriptive kebab-case names as shown above

### Content Guidelines
- **Clean Interface**: Show realistic but clean interface states
- **Consistent Styling**: Use consistent colors and styling across images
- **Readable Text**: Ensure all text in screenshots is legible
- **Example Data**: Use realistic example data (Living Room Speaker, etc.)
- **Status Indicators**: Show clear success/error states with appropriate colors

### Placeholder Content
Until real screenshots are available, consider:
- **Mockups**: Create simple mockups showing the expected interface
- **Wireframes**: Basic wireframes indicating layout and content
- **Diagrams**: Technical diagrams can be created immediately
- **Text Placeholders**: Use `[Image: Description]` in documentation

## Creating the Images

### For Dashboard Screenshots
1. Set up the enhanced SoundTouch service
2. Create sample account and register devices
3. Take screenshots at key points in the migration process
4. Edit for clarity (highlight important elements, add annotations)

### For Diagrams
1. Use tools like Lucidchart, draw.io, or similar
2. Follow consistent color scheme:
   - Blue: SoundTouch Service components
   - Green: Healthy/successful states
   - Orange: Warning/in-progress states
   - Red: Error/problematic states
   - Gray: External/third-party components

### For Physical Setup
1. Take photos of actual hardware setup
2. Show USB drive preparation process
3. Demonstrate network connections if helpful

## Alternative Text Requirements

Each image should have appropriate alt text for accessibility:

```markdown
![Alt text describing the image content](../images/image-name.png)
*Caption: Additional context or explanation*
```

## Future Enhancements

Consider adding:
- **Video Walkthroughs**: Screen recordings of key processes
- **Interactive Demos**: Web-based interactive guides
- **Troubleshooting Screenshots**: Common error states and solutions
- **Mobile Views**: How to access from mobile devices