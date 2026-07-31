package alp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	packedMagic           = "ALP3"
	packedFormatVersion   = byte(1)
	packedVersionOffset   = len(packedMagic)
	packedFrameOffset     = packedVersionOffset + 1
	packedBitWidthOffset  = packedFrameOffset + 8
	packedExpOffset       = packedBitWidthOffset + 1
	packedFactorOffset    = packedExpOffset + 1
	packedExceptionOffset = packedFactorOffset + 1
	packedHeaderSize      = packedExceptionOffset + 2
	packedExceptionSize   = 10
)

var (
	// ErrInvalidEncoding reports a malformed packed-frame value.
	ErrInvalidEncoding = errors.New("alp: invalid packed encoding")

	// ErrUnsupportedVersion reports a packed frame with an unknown format version.
	ErrUnsupportedVersion = errors.New("alp: unsupported packed format version")
)

// Pack serializes a Vector to bytes.
//
// The format is an 18-byte header, bit-packed encoded values, then exceptions.
// The header contains the four-byte "ALP3" magic, a format version,
// frame-of-reference (8 bytes), bit width, exponent, factor, and exception
// count (2 bytes). Multi-byte values are in little-endian order.
func Pack(vector *Vector) []byte {
	packedBits := VectorSize * int(vector.BitWidth)
	packedBytes := (packedBits + 7) / 8
	exceptionBytes := int(vector.NumExceptions) * packedExceptionSize

	buffer := make([]byte, packedHeaderSize+packedBytes+exceptionBytes)
	copy(buffer[:len(packedMagic)], packedMagic)
	buffer[packedVersionOffset] = packedFormatVersion
	binary.LittleEndian.PutUint64(
		buffer[packedFrameOffset:packedBitWidthOffset],
		uint64(vector.FrameOfRef),
	)
	buffer[packedBitWidthOffset] = vector.BitWidth
	buffer[packedExpOffset] = vector.Exp
	buffer[packedFactorOffset] = vector.Factor
	binary.LittleEndian.PutUint16(
		buffer[packedExceptionOffset:packedHeaderSize],
		vector.NumExceptions,
	)

	if vector.BitWidth > 0 {
		packBits(buffer[packedHeaderSize:packedHeaderSize+packedBytes], vector.Encoded[:], vector.BitWidth)
	}

	exceptionOffset := packedHeaderSize + packedBytes
	for i := uint16(0); i < vector.NumExceptions; i++ {
		binary.LittleEndian.PutUint16(buffer[exceptionOffset:], vector.ExceptionPos[i])
		binary.LittleEndian.PutUint64(buffer[exceptionOffset+2:], vector.ExceptionBits[i])
		exceptionOffset += packedExceptionSize
	}

	return buffer
}

// Unpack deserializes one complete packed frame from src into dst. It rejects
// malformed, truncated, unsupported, and trailing data. On error, dst is
// unchanged.
func Unpack(dst *Vector, src []byte) error {
	if len(src) < packedHeaderSize {
		return fmt.Errorf("alp: unpack header: %w", io.ErrUnexpectedEOF)
	}

	if string(src[:len(packedMagic)]) != packedMagic {
		return fmt.Errorf("alp: unpack magic: %w", ErrInvalidEncoding)
	}
	if src[packedVersionOffset] != packedFormatVersion {
		return fmt.Errorf(
			"alp: unpack version %d: %w",
			src[packedVersionOffset],
			ErrUnsupportedVersion,
		)
	}

	var decoded Vector
	decoded.FrameOfRef = int64(binary.LittleEndian.Uint64(
		src[packedFrameOffset:packedBitWidthOffset],
	))
	decoded.BitWidth = src[packedBitWidthOffset]
	decoded.Exp = src[packedExpOffset]
	decoded.Factor = src[packedFactorOffset]
	decoded.NumExceptions = binary.LittleEndian.Uint16(
		src[packedExceptionOffset:packedHeaderSize],
	)

	if decoded.BitWidth > 64 {
		return fmt.Errorf(
			"alp: unpack bit width %d: %w",
			decoded.BitWidth,
			ErrInvalidEncoding,
		)
	}
	if decoded.Exp >= numExp {
		return fmt.Errorf(
			"alp: unpack exponent %d: %w",
			decoded.Exp,
			ErrInvalidEncoding,
		)
	}
	if decoded.Factor >= numFact {
		return fmt.Errorf(
			"alp: unpack factor %d: %w",
			decoded.Factor,
			ErrInvalidEncoding,
		)
	}
	if decoded.NumExceptions > VectorSize {
		return fmt.Errorf(
			"alp: unpack exception count %d: %w",
			decoded.NumExceptions,
			ErrInvalidEncoding,
		)
	}

	packedBits := VectorSize * int(decoded.BitWidth)
	packedBytes := (packedBits + 7) / 8
	expectedSize := packedHeaderSize +
		packedBytes +
		int(decoded.NumExceptions)*packedExceptionSize
	if len(src) < expectedSize {
		return fmt.Errorf(
			"alp: unpack frame: got %d bytes, need %d: %w",
			len(src),
			expectedSize,
			io.ErrUnexpectedEOF,
		)
	}
	if len(src) > expectedSize {
		return fmt.Errorf(
			"alp: unpack trailing data: got %d bytes, want %d: %w",
			len(src),
			expectedSize,
			ErrInvalidEncoding,
		)
	}

	if decoded.BitWidth > 0 {
		unpackBits(
			decoded.Encoded[:],
			src[packedHeaderSize:packedHeaderSize+packedBytes],
			decoded.BitWidth,
		)
	}

	exceptionOffset := packedHeaderSize + packedBytes
	var previousPosition uint16
	for i := uint16(0); i < decoded.NumExceptions; i++ {
		position := binary.LittleEndian.Uint16(src[exceptionOffset:])
		if position >= VectorSize {
			return fmt.Errorf(
				"alp: unpack exception %d position %d: %w",
				i,
				position,
				ErrInvalidEncoding,
			)
		}
		if i > 0 && position <= previousPosition {
			return fmt.Errorf(
				"alp: unpack exception %d position %d after %d: %w",
				i,
				position,
				previousPosition,
				ErrInvalidEncoding,
			)
		}
		decoded.ExceptionPos[i] = position
		decoded.ExceptionBits[i] = binary.LittleEndian.Uint64(src[exceptionOffset+2:])
		previousPosition = position
		exceptionOffset += packedExceptionSize
	}

	*dst = decoded
	return nil
}

func packBits(dst []byte, src []int64, width uint8) {
	if width == 0 {
		return
	}

	switch width {
	case 8:
		for i := 0; i < VectorSize; i++ {
			dst[i] = byte(src[i])
		}
		return
	case 16:
		for i := 0; i < VectorSize; i++ {
			binary.LittleEndian.PutUint16(dst[i*2:], uint16(src[i]))
		}
		return
	case 32:
		for i := 0; i < VectorSize; i++ {
			binary.LittleEndian.PutUint32(dst[i*4:], uint32(src[i]))
		}
		return
	}

	bitPosition := uint(0)
	for i := 0; i < VectorSize; i++ {
		value := uint64(src[i])
		remaining := uint(width)
		shift := uint(0)

		for remaining > 0 {
			byteIndex := bitPosition / 8
			bitOffset := bitPosition % 8
			bitsAvailable := 8 - bitOffset
			if bitsAvailable > remaining {
				bitsAvailable = remaining
			}

			mask := uint64((1 << bitsAvailable) - 1)
			bits := (value >> shift) & mask
			dst[byteIndex] |= byte(bits << bitOffset)

			bitPosition += bitsAvailable
			remaining -= bitsAvailable
			shift += bitsAvailable
		}
	}
}

func unpackBits(dst []int64, src []byte, width uint8) {
	if width == 0 {
		clear(dst)
		return
	}

	switch width {
	case 8:
		for i := 0; i < VectorSize; i++ {
			dst[i] = int64(src[i])
		}
		return
	case 16:
		for i := 0; i < VectorSize; i++ {
			dst[i] = int64(binary.LittleEndian.Uint16(src[i*2:]))
		}
		return
	case 32:
		for i := 0; i < VectorSize; i++ {
			dst[i] = int64(binary.LittleEndian.Uint32(src[i*4:]))
		}
		return
	}

	bitPosition := uint(0)
	for i := 0; i < VectorSize; i++ {
		var value uint64
		remaining := uint(width)
		shift := uint(0)

		for remaining > 0 {
			byteIndex := bitPosition / 8
			bitOffset := bitPosition % 8
			bitsAvailable := 8 - bitOffset
			if bitsAvailable > remaining {
				bitsAvailable = remaining
			}

			mask := byte((1 << bitsAvailable) - 1)
			bits := (src[byteIndex] >> bitOffset) & mask
			value |= uint64(bits) << shift

			bitPosition += bitsAvailable
			remaining -= bitsAvailable
			shift += bitsAvailable
		}
		dst[i] = int64(value)
	}
}

// PackedSize returns the number of bytes Pack will produce for vector.
func PackedSize(vector *Vector) int {
	packedBits := VectorSize * int(vector.BitWidth)
	packedBytes := (packedBits + 7) / 8
	exceptionBytes := int(vector.NumExceptions) * packedExceptionSize
	return packedHeaderSize + packedBytes + exceptionBytes
}
