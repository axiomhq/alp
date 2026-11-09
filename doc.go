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
// data, so Decode never reads out of bounds on a Vector it filled. Encode,
// Decode and Pack may panic or emit undecodable bytes for a hand-built
// Vector or State with out-of-range fields. A packed vector stores no value count or length index: store the
// count of a short block and the offset of each vector yourself.
//
// DecodeRD48 is a standalone decoder for one ALP-RD layout; this package has
// no ALP-RD encoder.
//
// The packed layout is stored by consumers, so changes to it must stay
// readable by existing decoders.
package alp
