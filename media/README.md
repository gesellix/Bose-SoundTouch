# Project artwork

Source files for the AfterTouch logo and the images the README embeds.
Everything here is a source or a preview, not something the service ships.

## The idea behind the logo

The mark is a single **braille cell**: two columns of three dots, the shape
braille uses for one character.

The letters it spells are **S** and **T**, for **S**ound**T**ouch. They are
not side by side. In braille they overlap:

| Letter | Dots       |
|--------|------------|
| S      | 2, 3, 4    |
| T      | 2, 3, 4, 5 |

T is S with one dot added. So the same cell can show both letters at once:
the blue dots (2, 3, 4) are the S, and **dot 5, in yellow, is the single dot
that turns that S into a T**. The accent colour is not decoration, it marks
the one dot the two letters differ by.

Dots 1 and 6 stay light grey. They are not part of either letter, they are
there so the shape still reads as a braille cell rather than as four
arbitrary dots.

The other fit is the obvious one: braille is writing meant to be read by
touch, on a project about a speaker called SoundTouch.

### The morse variant

`favicon-morse.*` encodes the same two initials in the other dot-based
writing system most people recognise: **S** is three dots, **T** is one
dash. The colour roles carry over, blue for the S, yellow for the T.

It is kept as an alternate. The braille cell is the mark actually in use
(the README heading, the admin UI, the landing page, the docs, and the
service's `/favicon.ico`).

### Palette

| Role                  | Colour    |
|-----------------------|-----------|
| Active dots (S)       | `#0055aa` |
| Accent dot 5 (the T)  | `#ffcc00` |
| Inactive dots (1, 6)  | `#eee`    |

Both variants use the same three, so they read as one family.

## Files

| File                      | What it is                                                                                                                                            |
|---------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| `favicon-braille.afphoto` | Affinity Photo document. The editable artwork, where the mark is drawn.                                                                               |
| `favicon-braille.png`     | 32x32 raster export from the Affinity document.                                                                                                       |
| `favicon-braille.svg`     | The vector the project builds from. Hand-maintained, and the copy in `pkg/service/handlers/web/img/` and `docs/static/favicon.svg` is byte-identical. |
| `docs-homepage.png`       | Screenshot the top-level `README.md` embeds.                                                                                                          |

## How the shipped icons are produced

The PNG and ICO files under `pkg/service/handlers/web/img/` are generated
from the SVGs in that same directory, not from the Affinity document:

```bash
go run ./cmd/favicon-gen   # from the repo root: its paths are relative to it
```

(`make build-favicon-gen` only builds the generator into `build/`, it does
not run it.)

`cmd/favicon-gen` renders each `favicon-*.svg` to a 32x32 PNG and to an ICO
containing 16, 32 and 48 pixel versions. Change the SVG, re-run it, and
commit what it writes. Do not hand-edit a PNG or ICO.

See `pkg/service/handlers/web/img/favicon.md` for what each dot means per
variant.
