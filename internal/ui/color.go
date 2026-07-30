package ui

// rgb unpacks a theme colour into the three channel arguments the renderer's
// drawing primitives take.
//
// Every primitive has an (red, green, blue uint8) signature, so without this a
// migration from literals to theme accessors turns one readable line into three
// indexed ones. Kept build-tag free so it stays covered by the headless CI
// build alongside the rest of the non-SDL logic.
func rgb(c [3]uint8) (uint8, uint8, uint8) { return c[0], c[1], c[2] }
