package alp

import (
	"encoding/binary"
	"math"
	"unsafe"
)

// DecodeRD48 decodes the common ALP-RD layout with two 16-bit prefixes and
// one packed 48-bit suffix per value. Each bit in leftCodes selects one of the
// prefixes, LSB first within each byte, as the high 16 bits of the value;
// rightParts stores the low 48 bits as six little-endian bytes per value.
// dst must not overlap rightParts.
//
// DecodeRD48 panics if leftCodes or rightParts are too short for dst.
func DecodeRD48(dst []float64, leftCodes, rightParts []byte, prefixes [2]uint16) {
	leftBytes := len(dst) / 8
	if len(dst)%8 != 0 {
		leftBytes++
	}
	if len(leftCodes) < leftBytes {
		panic("alp: RD48 left codes too short")
	}
	if len(rightParts)/6 < len(dst) {
		panic("alp: RD48 right parts too short")
	}

	prefix := [2]uint64{
		uint64(prefixes[0]) << 48,
		uint64(prefixes[1]) << 48,
	}
	var prefixByNibble [16][4]uint64
	for code := range prefixByNibble {
		for bit := range prefixByNibble[code] {
			prefixByNibble[code][bit] = prefix[code>>bit&1]
		}
	}

	i := 0
	rightOffset := 0
	for ; i+8 <= len(dst); i += 8 {
		left := leftCodes[i>>3]
		low := &prefixByNibble[left&0xf]
		high := &prefixByNibble[left>>4]
		right := rightParts[rightOffset : rightOffset+48 : rightOffset+48]
		word0 := binary.LittleEndian.Uint64(right)
		word1 := binary.LittleEndian.Uint64(right[8:])
		word2 := binary.LittleEndian.Uint64(right[16:])
		word3 := binary.LittleEndian.Uint64(right[24:])
		word4 := binary.LittleEndian.Uint64(right[32:])
		word5 := binary.LittleEndian.Uint64(right[40:])
		const mask48 = uint64(1)<<48 - 1

		// float64 and uint64 have identical size and alignment. Storing the
		// reconstructed IEEE-754 bits through this fixed-size view avoids a
		// temporary uint64 slice and does not involve pointer write barriers.
		out := (*[8]uint64)(unsafe.Pointer(&dst[i]))
		out[0] = low[0] | word0&mask48
		out[1] = low[1] | word0>>48 | (word1&0xffffffff)<<16
		out[2] = low[2] | word1>>32 | (word2&0xffff)<<32
		out[3] = low[3] | word2>>16
		out[4] = high[0] | word3&mask48
		out[5] = high[1] | word3>>48 | (word4&0xffffffff)<<16
		out[6] = high[2] | word4>>32 | (word5&0xffff)<<32
		out[7] = high[3] | word5>>16
		rightOffset += 48
	}

	for ; i < len(dst); i++ {
		offset := i * 6
		right := uint64(binary.LittleEndian.Uint32(rightParts[offset:])) |
			uint64(binary.LittleEndian.Uint16(rightParts[offset+4:]))<<32
		left := uint64(leftCodes[i>>3] >> (i & 7) & 1)
		dst[i] = math.Float64frombits(prefix[left] | right)
	}
}
