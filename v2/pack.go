package alp

import "encoding/binary"

// Pack serializes a Vector to bytes.
// Format: header (12 bytes) + packed data + exceptions
func Pack(v *Vector) []byte {
	// Calculate size
	packedBits := VectorSize * int(v.BitWidth)
	packedBytes := (packedBits + 7) / 8
	excBytes := int(v.NumExceptions) * 10 // 2 bytes pos + 8 bytes value

	buf := make([]byte, 12+packedBytes+excBytes)

	// Header: frameOfRef(8) + bitWidth(1) + exp(1) + factor(1) + numExc(1)
	binary.LittleEndian.PutUint64(buf[0:8], uint64(v.FrameOfRef))
	buf[8] = v.BitWidth
	buf[9] = v.Exp
	buf[10] = v.Factor
	buf[11] = byte(v.NumExceptions)

	// Pack encoded values
	if v.BitWidth > 0 {
		packBits(buf[12:12+packedBytes], v.Encoded[:], v.BitWidth)
	}

	// Write exceptions
	excStart := 12 + packedBytes
	for i := uint16(0); i < v.NumExceptions; i++ {
		binary.LittleEndian.PutUint16(buf[excStart:], v.ExceptionPos[i])
		binary.LittleEndian.PutUint64(buf[excStart+2:], v.ExceptionBits[i])
		excStart += 10
	}

	return buf
}

// Unpack deserializes bytes to a Vector.
func Unpack(dst *Vector, src []byte) {
	if len(src) < 12 {
		return
	}

	// Read header
	dst.FrameOfRef = int64(binary.LittleEndian.Uint64(src[0:8]))
	dst.BitWidth = src[8]
	dst.Exp = src[9]
	dst.Factor = src[10]
	dst.NumExceptions = uint16(src[11])

	// Unpack encoded values
	packedBits := VectorSize * int(dst.BitWidth)
	packedBytes := (packedBits + 7) / 8

	if dst.BitWidth > 0 && len(src) >= 12+packedBytes {
		unpackBits(dst.Encoded[:], src[12:12+packedBytes], dst.BitWidth)
	}

	// Read exceptions
	excStart := 12 + packedBytes
	for i := uint16(0); i < dst.NumExceptions && excStart+10 <= len(src); i++ {
		dst.ExceptionPos[i] = binary.LittleEndian.Uint16(src[excStart:])
		dst.ExceptionBits[i] = binary.LittleEndian.Uint64(src[excStart+2:])
		excStart += 10
	}
}

// packBits packs values into dst using bitWidth bits per value.
func packBits(dst []byte, src []int64, bitWidth uint8) {
	if bitWidth == 0 {
		return
	}

	// Fast paths for common widths
	switch bitWidth {
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

	// General case
	bitPos := uint(0)
	for i := 0; i < VectorSize; i++ {
		v := uint64(src[i])
		remaining := uint(bitWidth)
		shift := uint(0)

		for remaining > 0 {
			byteIdx := bitPos / 8
			bitOffset := bitPos % 8
			bitsAvailable := 8 - bitOffset
			if bitsAvailable > remaining {
				bitsAvailable = remaining
			}

			mask := uint64((1 << bitsAvailable) - 1)
			bits := (v >> shift) & mask
			dst[byteIdx] |= byte(bits << bitOffset)

			bitPos += bitsAvailable
			remaining -= bitsAvailable
			shift += bitsAvailable
		}
	}
}

// unpackBits unpacks values from src using bitWidth bits per value.
func unpackBits(dst []int64, src []byte, bitWidth uint8) {
	if bitWidth == 0 {
		for i := range dst {
			dst[i] = 0
		}
		return
	}

	// Fast paths for common widths
	switch bitWidth {
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

	// General case
	bitPos := uint(0)
	for i := 0; i < VectorSize; i++ {
		var v uint64
		remaining := uint(bitWidth)
		shift := uint(0)

		for remaining > 0 {
			byteIdx := bitPos / 8
			if int(byteIdx) >= len(src) {
				break
			}

			bitOffset := bitPos % 8
			bitsAvailable := 8 - bitOffset
			if bitsAvailable > remaining {
				bitsAvailable = remaining
			}

			mask := byte((1 << bitsAvailable) - 1)
			bits := (src[byteIdx] >> bitOffset) & mask
			v |= uint64(bits) << shift

			bitPos += bitsAvailable
			remaining -= bitsAvailable
			shift += bitsAvailable
		}
		dst[i] = int64(v)
	}
}

// PackedSize returns the size of a packed Vector in bytes.
func PackedSize(v *Vector) int {
	packedBits := VectorSize * int(v.BitWidth)
	packedBytes := (packedBits + 7) / 8
	excBytes := int(v.NumExceptions) * 10
	return 12 + packedBytes + excBytes
}
