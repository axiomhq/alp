// Package alp implements Adaptive Lossless floating-Point compression.
//
// ALP (SIGMOD 2024, https://dl.acm.org/doi/10.1145/3626717) is a lossless
// scheme for float64 data. Most real-world floats are decimals: scaling them
// by a power of ten yields an exact integer, which is then frame-of-reference
// shifted and bit-packed. Values that do not round-trip (NaN, ±Inf, -0 and
// non-decimal floats) are stored raw as exceptions.
//
// Usage: Sample a column once to get a State, then Encode each block of
// VectorSize values into a Vector and Pack it. Unpack and Decode reverse this.
// Unpack validates its input and returns an error on truncated or corrupt
// data, so Decode never reads out of bounds.
//
// The packed layout is stored by consumers, so changes to it must stay
// readable by existing decoders.
package alp
