package alp

import "encoding/binary"

const packedHeaderSize = 13

// Pack serializes a Vector to bytes.
//
// The format is a 13-byte header, bit-packed encoded values, then exceptions.
// The header contains frame-of-reference (8 bytes), bit width, exponent,
// factor, and exception count (2 bytes), all in little-endian order.
func Pack(vector *Vector) []byte {
	packedBits := VectorSize * int(vector.BitWidth)
	packedBytes := (packedBits + 7) / 8
	exceptionBytes := int(vector.NumExceptions) * 10

	buffer := make([]byte, packedHeaderSize+packedBytes+exceptionBytes)
	binary.LittleEndian.PutUint64(buffer[0:8], uint64(vector.FrameOfRef))
	buffer[8] = vector.BitWidth
	buffer[9] = vector.Exp
	buffer[10] = vector.Factor
	binary.LittleEndian.PutUint16(buffer[11:13], vector.NumExceptions)

	if vector.BitWidth > 0 {
		packBits(buffer[packedHeaderSize:packedHeaderSize+packedBytes], vector.Encoded[:], vector.BitWidth)
	}

	exceptionOffset := packedHeaderSize + packedBytes
	for i := uint16(0); i < vector.NumExceptions; i++ {
		binary.LittleEndian.PutUint16(buffer[exceptionOffset:], vector.ExceptionPos[i])
		binary.LittleEndian.PutUint64(buffer[exceptionOffset+2:], vector.ExceptionBits[i])
		exceptionOffset += 10
	}

	return buffer
}

// Unpack deserializes src into dst. If src is incomplete, Unpack decodes all
// complete fields and leaves unavailable data at its zero value.
func Unpack(dst *Vector, src []byte) {
	*dst = Vector{}
	if len(src) < packedHeaderSize {
		return
	}

	dst.FrameOfRef = int64(binary.LittleEndian.Uint64(src[0:8]))
	dst.BitWidth = src[8]
	dst.Exp = src[9]
	dst.Factor = src[10]
	dst.NumExceptions = binary.LittleEndian.Uint16(src[11:13])

	packedBits := VectorSize * int(dst.BitWidth)
	packedBytes := (packedBits + 7) / 8
	if packedBytes > len(src)-packedHeaderSize {
		return
	}

	if dst.BitWidth > 0 {
		unpackBits(dst.Encoded[:], src[packedHeaderSize:packedHeaderSize+packedBytes], dst.BitWidth)
	}

	exceptionOffset := packedHeaderSize + packedBytes
	completeExceptions := uint16(0)
	for completeExceptions < dst.NumExceptions && exceptionOffset+10 <= len(src) {
		dst.ExceptionPos[completeExceptions] = binary.LittleEndian.Uint16(src[exceptionOffset:])
		dst.ExceptionBits[completeExceptions] = binary.LittleEndian.Uint64(src[exceptionOffset+2:])
		completeExceptions++
		exceptionOffset += 10
	}
	dst.NumExceptions = completeExceptions
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
	exceptionBytes := int(vector.NumExceptions) * 10
	return packedHeaderSize + packedBytes + exceptionBytes
}
