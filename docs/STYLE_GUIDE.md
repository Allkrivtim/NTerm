# NTerm visual language

## Character

NTerm is a quiet native utility, not a branded content surface. The interface
should disappear during work. Prefer fewer controls, smaller groups and stable
placement over decorative personality.

## Rules

1. Use neutral grayscale surfaces. Product accent colors are not allowed.
2. Reserve color for terminal ANSI output, shell syntax and semantic
   success/error states. Syntax colors stay muted and functional; they never
   become product accents.
3. Use Liquid Glass only for functional floating layers: the composer, tab
   controls, settings panels and suggestion popovers.
4. Keep content flat. Command blocks use spacing and separators, not cards.
5. Use one border, one subtle highlight and at most one soft shadow per control.
6. Prefer the system appearance and system typography. Theme and terminal font
   choices are user preferences, not brand styling.
7. Avoid hero copy, logos inside the product, oversized empty states, gradients
   with color, glow, badges and permanently visible instructional text.
8. Controls appear on hover/focus when discoverability remains intact.
9. Motion is short (100–160 ms), geometric and optional under reduced motion.
10. Every panel must remain legible with reduced transparency enabled.

## Geometry

- Main content is fluid across the window with a responsive 18–42 px gutter.
- Composer radius: 11 px; compact controls: 8–10 px.
- Control height: 28–44 px depending on text editing needs.
- Separators: 1 px at low contrast.
- Terminal text: 10–24 px configurable, 13 px default.
- Terminal line height is configurable from 1.2–2.0; block rhythm offers
  compact, comfortable and spacious presets without changing content order.
- Cursor shape and blinking apply to the interactive VT surface. Shell syntax
  colours and command timestamps are optional presentation layers.
- Shell syntax follows macOS semantic color families: blue for commands,
  purple for options, teal for paths, warm orange for quoted text, cyan for
  variables and muted pink for operators. Light and dark themes use separate
  contrast-safe values.

## App icon

The app icon is the only place where the white terminal mark `>_` is used. It
must not be repeated in the window, tab strip, composer, settings or empty
states.

The macOS traffic-light controls are the only non-terminal use of saturated
color. They are native window semantics, not product accents.
