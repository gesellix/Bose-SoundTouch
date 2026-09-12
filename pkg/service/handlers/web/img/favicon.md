# Favicon Meanings

This directory contains favicons for the service in various formats (SVG, PNG, ICO). The icons use Morse code and Braille to represent the initials **S** (Sound) and **T** (Touch).

## Morse Variant (`favicon-morse.*`)
The icon represents the letters **S** and **T** in international Morse code:
- **S**: `...` (three dots), displayed in blue.
- **T**: `-` (one long dash), displayed in yellow.

## Sources

The PNG and ICO files here are generated, not hand-drawn: `cmd/favicon-gen`
renders them from the `.svg` next to them (32x32 PNG, plus an ICO with 16,
32 and 48 pixel versions). Change the SVG and re-run it rather than editing
a raster by hand.

The editable artwork behind the Braille mark is
`media/favicon-braille.afphoto` (Affinity Photo). See `media/README.md` for
the idea behind the logo and how the files relate.

## Braille Variant (`favicon-braille.*`)
The icon uses the 6-dot Braille grid to represent a stylized combination of the letters **S** and **T**:
- The blue dots represent the base shape.
- The yellow dot (dot 5) serves as an accent for the **T**.
- The light gray dots complete the 2x3 grid for better recognition as Braille.
