package alp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrCorrupt is returned by Unpack for truncated or invalid input.
var ErrCorrupt = errors.New("alp: corrupt vector")

const (
	packedHeaderSize = 13
	// packedExceptionSize is a uint16 position plus the uint64 original bits.
	packedExceptionSize = 10
	// packedDeltaFlag marks delta mode in the bit-width byte; Base then follows the header.
	packedDeltaFlag = 0x80
)

// Pack appends vector to dst as header, delta base, bit-packed values, then exceptions.
func Pack(dst []byte, vector *Vector) []byte {
	start := len(dst)
	dst = append(dst, make([]byte, PackedSize(vector))...)
	buffer := dst[start:]
	binary.LittleEndian.PutUint64(buffer[0:8], uint64(vector.FrameOfRef))
	buffer[8] = vector.BitWidth
	buffer[9] = vector.Exp
	buffer[10] = vector.Factor
	binary.LittleEndian.PutUint16(buffer[11:13], vector.NumExceptions)
	offset := packedHeaderSize
	if vector.Delta {
		buffer[8] |= packedDeltaFlag
		binary.LittleEndian.PutUint64(buffer[offset:], uint64(vector.Base))
		offset += 8
	}

	packedBytes := packedBitsSize(vector.BitWidth)
	packBits(buffer[offset:offset+packedBytes], vector.Encoded[:], vector.BitWidth)

	offset += packedBytes
	for i := range vector.NumExceptions {
		binary.LittleEndian.PutUint16(buffer[offset:], vector.ExceptionPos[i])
		binary.LittleEndian.PutUint64(buffer[offset+2:], vector.ExceptionBits[i])
		offset += packedExceptionSize
	}
	return dst
}

// Unpack reads one Vector from the start of src and returns the bytes consumed.
// It returns an error wrapping ErrCorrupt on truncated or invalid input; header
// errors leave dst untouched, but later errors may leave it partly written, so discard it.
func Unpack(dst *Vector, src []byte) (int, error) {
	if len(src) < packedHeaderSize {
		return 0, fmt.Errorf("%w: %d header bytes, want %d", ErrCorrupt, len(src), packedHeaderSize)
	}
	bitWidth := src[8] &^ packedDeltaFlag
	exp, factor := src[9], src[10]
	numExceptions := binary.LittleEndian.Uint16(src[11:13])
	if bitWidth > 64 || exp >= numExp || factor >= numFact || numExceptions > VectorSize {
		return 0, fmt.Errorf("%w: bit width %d, exp %d, factor %d, exceptions %d", ErrCorrupt, bitWidth, exp, factor, numExceptions)
	}
	dst.FrameOfRef = int64(binary.LittleEndian.Uint64(src[0:8]))
	dst.Delta = src[8]&packedDeltaFlag != 0
	dst.BitWidth, dst.Exp, dst.Factor, dst.NumExceptions = bitWidth, exp, factor, numExceptions

	size := PackedSize(dst)
	if len(src) < size {
		return 0, fmt.Errorf("%w: %d bytes, want %d", ErrCorrupt, len(src), size)
	}

	offset := packedHeaderSize
	dst.Base = 0
	if dst.Delta {
		dst.Base = int64(binary.LittleEndian.Uint64(src[offset:]))
		offset += 8
	}
	packedBytes := packedBitsSize(dst.BitWidth)
	unpackBits(dst.Encoded[:], src[offset:offset+packedBytes], dst.BitWidth)

	// Exceptions must ascend so each position is stored once.
	exceptionOffset := offset + packedBytes
	for i := range dst.NumExceptions {
		position := binary.LittleEndian.Uint16(src[exceptionOffset:])
		if position >= VectorSize || (i > 0 && position <= dst.ExceptionPos[i-1]) {
			return 0, fmt.Errorf("%w: exception %d at position %d", ErrCorrupt, i, position)
		}
		dst.ExceptionPos[i] = position
		dst.ExceptionBits[i] = binary.LittleEndian.Uint64(src[exceptionOffset+2:])
		exceptionOffset += packedExceptionSize
	}
	return size, nil
}

func packBits(dst []byte, src []int64, width uint8) {
	switch width {
	case 0:
		return
	case 8:
		for i := range VectorSize {
			dst[i] = byte(src[i])
		}
		return
	case 16:
		for i := range VectorSize {
			binary.LittleEndian.PutUint16(dst[i*2:], uint16(src[i]))
		}
		return
	case 32:
		for i := range VectorSize {
			binary.LittleEndian.PutUint32(dst[i*4:], uint32(src[i]))
		}
		return
	}

	// Values fill a 64-bit accumulator; VectorSize is a multiple of 64, so it ends word-aligned.
	w := uint(width)
	mask := ^uint64(0) >> (64 - w)
	var acc uint64
	var n, offset uint
	for _, s := range src[:VectorSize] {
		v := uint64(s) & mask
		acc |= v << n
		n += w
		if n >= 64 {
			binary.LittleEndian.PutUint64(dst[offset:], acc)
			offset += 8
			n -= 64
			acc = v >> (w - n)
		}
	}
}

func unpackBits(dst []int64, src []byte, width uint8) {
	switch width {
	case 0:
		clear(dst[:VectorSize])
		return
	case 8:
		for i := range VectorSize {
			dst[i] = int64(src[i])
		}
		return
	case 16:
		for i := range VectorSize {
			dst[i] = int64(binary.LittleEndian.Uint16(src[i*2:]))
		}
		return
	case 32:
		for i := range VectorSize {
			dst[i] = int64(binary.LittleEndian.Uint32(src[i*4:]))
		}
		return
	}

	w := uint(width)
	mask := ^uint64(0) >> (64 - w)
	var acc uint64
	var n, offset uint
	for i := range VectorSize {
		if n >= w {
			dst[i] = int64(acc & mask)
			acc >>= w
			n -= w
			continue
		}
		next := binary.LittleEndian.Uint64(src[offset:])
		offset += 8
		dst[i] = int64((acc | next<<n) & mask)
		acc = next >> (w - n)
		n += 64 - w
	}
}

// PackedSize returns the number of bytes Pack appends for vector.
func PackedSize(vector *Vector) int {
	size := packedHeaderSize + packedBitsSize(vector.BitWidth) + int(vector.NumExceptions)*packedExceptionSize
	if vector.Delta {
		size += 8
	}
	return size
}

// packedBitsSize returns the bytes needed for VectorSize values of width bits.
func packedBitsSize(width uint8) int {
	return (VectorSize*int(width) + 7) / 8
}
