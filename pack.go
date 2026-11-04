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

	packedBytes := (VectorSize*int(vector.BitWidth) + 7) / 8
	packBits(buffer[offset:offset+packedBytes], vector.Encoded[:], vector.BitWidth)

	offset += packedBytes
	for i := range vector.NumExceptions {
		binary.LittleEndian.PutUint16(buffer[offset:], vector.ExceptionPos[i])
		binary.LittleEndian.PutUint64(buffer[offset+2:], vector.ExceptionBits[i])
		offset += 10
	}
	return dst
}

// Unpack reads one Vector from the start of src and returns the bytes consumed.
// It returns an error wrapping ErrCorrupt on truncated or invalid input.
func Unpack(dst *Vector, src []byte) (int, error) {
	if len(src) < packedHeaderSize {
		return 0, fmt.Errorf("%w: %d header bytes, want %d", ErrCorrupt, len(src), packedHeaderSize)
	}
	dst.FrameOfRef = int64(binary.LittleEndian.Uint64(src[0:8]))
	dst.Delta = src[8]&packedDeltaFlag != 0
	dst.BitWidth = src[8] &^ packedDeltaFlag
	dst.Exp = src[9]
	dst.Factor = src[10]
	dst.NumExceptions = binary.LittleEndian.Uint16(src[11:13])
	if dst.BitWidth > 64 || dst.Exp >= numExp || dst.Factor >= numFact || dst.NumExceptions > VectorSize {
		return 0, fmt.Errorf("%w: bit width %d, exp %d, factor %d, exceptions %d", ErrCorrupt, dst.BitWidth, dst.Exp, dst.Factor, dst.NumExceptions)
	}

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
	packedBytes := (VectorSize*int(dst.BitWidth) + 7) / 8
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
		exceptionOffset += 10
	}
	return size, nil
}

func packBits(dst []byte, src []int64, width uint8) {
	switch width {
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

	bitPosition := uint(0)
	for i := range VectorSize {
		value := uint64(src[i])
		remaining := uint(width)
		shift := uint(0)

		for remaining > 0 {
			byteIndex := bitPosition / 8
			bitOffset := bitPosition % 8
			bitsAvailable := min(8-bitOffset, remaining)

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
	switch width {
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

	bitPosition := uint(0)
	for i := range VectorSize {
		var value uint64
		remaining := uint(width)
		shift := uint(0)

		for remaining > 0 {
			byteIndex := bitPosition / 8
			bitOffset := bitPosition % 8
			bitsAvailable := min(8-bitOffset, remaining)

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

// PackedSize returns the number of bytes Pack appends for vector.
func PackedSize(vector *Vector) int {
	size := packedHeaderSize + (VectorSize*int(vector.BitWidth)+7)/8 + int(vector.NumExceptions)*10
	if vector.Delta {
		size += 8
	}
	return size
}
