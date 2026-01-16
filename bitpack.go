package alp

// BitPacker packs integers into a bit stream using variable bit widths.
// This implements Frame-of-Reference encoding where values are stored
// relative to a minimum value using only the bits required.
type BitPacker struct {
	buf    []byte
	bitPos uint
}

// NewBitPacker creates a new bit packer with the given initial capacity.
func NewBitPacker(capacity int) *BitPacker {
	return &BitPacker{
		buf: make([]byte, 0, capacity),
	}
}

// Pack packs a slice of uint64 values using the specified bit width.
// All values must fit within the given bit width.
func (p *BitPacker) Pack(values []uint64, bitWidth uint8) {
	if bitWidth == 0 || len(values) == 0 {
		return
	}

	// Calculate bytes needed
	totalBits := uint(len(values)) * uint(bitWidth)
	bytesNeeded := (totalBits + 7) / 8

	// Ensure we have enough space
	startLen := len(p.buf)
	for len(p.buf) < startLen+int(bytesNeeded) {
		p.buf = append(p.buf, 0)
	}

	// Pack each value
	for _, v := range values {
		p.packValue(v, bitWidth)
	}
}

// packValue packs a single value with the given bit width.
func (p *BitPacker) packValue(v uint64, bitWidth uint8) {
	remaining := uint(bitWidth)
	shift := uint(0)

	for remaining > 0 {
		byteIdx := p.bitPos / 8
		bitOffset := p.bitPos % 8

		// How many bits can we write to this byte?
		bitsAvailable := 8 - bitOffset
		if bitsAvailable > remaining {
			bitsAvailable = remaining
		}

		// Extract bits to write
		mask := uint64((1 << bitsAvailable) - 1)
		bits := (v >> shift) & mask

		// Write to buffer
		p.buf[byteIdx] |= byte(bits << bitOffset)

		p.bitPos += bitsAvailable
		remaining -= bitsAvailable
		shift += bitsAvailable
	}
}

// Bytes returns the packed byte slice.
func (p *BitPacker) Bytes() []byte {
	return p.buf
}

// Reset clears the packer for reuse.
func (p *BitPacker) Reset() {
	p.buf = p.buf[:0]
	p.bitPos = 0
}

// BitUnpacker unpacks integers from a bit stream.
type BitUnpacker struct {
	buf    []byte
	bitPos uint
}

// NewBitUnpacker creates a new bit unpacker from the given bytes.
func NewBitUnpacker(data []byte) *BitUnpacker {
	return &BitUnpacker{
		buf: data,
	}
}

// Unpack unpacks n values with the specified bit width.
func (u *BitUnpacker) Unpack(n int, bitWidth uint8) []uint64 {
	if bitWidth == 0 || n == 0 {
		return make([]uint64, n)
	}

	result := make([]uint64, n)
	for i := 0; i < n; i++ {
		result[i] = u.unpackValue(bitWidth)
	}
	return result
}

// UnpackInto unpacks values into an existing slice.
func (u *BitUnpacker) UnpackInto(dst []uint64, bitWidth uint8) {
	if bitWidth == 0 {
		for i := range dst {
			dst[i] = 0
		}
		return
	}

	for i := range dst {
		dst[i] = u.unpackValue(bitWidth)
	}
}

// unpackValue unpacks a single value with the given bit width.
func (u *BitUnpacker) unpackValue(bitWidth uint8) uint64 {
	var v uint64
	remaining := uint(bitWidth)
	shift := uint(0)

	for remaining > 0 {
		byteIdx := u.bitPos / 8
		bitOffset := u.bitPos % 8

		if int(byteIdx) >= len(u.buf) {
			break
		}

		// How many bits can we read from this byte?
		bitsAvailable := 8 - bitOffset
		if bitsAvailable > remaining {
			bitsAvailable = remaining
		}

		// Extract bits
		mask := byte((1 << bitsAvailable) - 1)
		bits := (u.buf[byteIdx] >> bitOffset) & mask

		// Add to result
		v |= uint64(bits) << shift

		u.bitPos += bitsAvailable
		remaining -= bitsAvailable
		shift += bitsAvailable
	}

	return v
}

// Reset resets the unpacker to the beginning.
func (u *BitUnpacker) Reset() {
	u.bitPos = 0
}

// PackedBlock represents a fully compressed ALP block.
type PackedBlock struct {
	// Compressed data bytes
	Data []byte
	// Number of values
	Count int
	// Frame-of-Reference base value
	FrameOfRef int64
	// Bit width for each value
	BitWidth uint8
	// Encoding parameters
	Exp    uint8
	Factor uint8
	// Exception positions
	ExceptionPos []uint16
	// Exception values (raw float bits)
	Exceptions []uint64
}

// PackBlock compresses an EncodedBlock into a PackedBlock.
func PackBlock(block *EncodedBlock) *PackedBlock {
	// Convert signed to unsigned for bit packing
	values := make([]uint64, len(block.Encoded))
	for i, v := range block.Encoded {
		// Values are already relative to FrameOfRef, so they're non-negative
		values[i] = uint64(v)
	}

	// Pack the values
	packer := NewBitPacker(len(values) * int(block.BitWidth+7) / 8)
	packer.Pack(values, block.BitWidth)

	return &PackedBlock{
		Data:         packer.Bytes(),
		Count:        len(block.Encoded),
		FrameOfRef:   block.FrameOfRef,
		BitWidth:     block.BitWidth,
		Exp:          block.Exp,
		Factor:       block.Factor,
		ExceptionPos: block.ExceptionPos,
		Exceptions:   block.Exceptions,
	}
}

// UnpackBlock decompresses a PackedBlock back to an EncodedBlock.
func UnpackBlock(packed *PackedBlock) *EncodedBlock {
	unpacker := NewBitUnpacker(packed.Data)
	values := unpacker.Unpack(packed.Count, packed.BitWidth)

	// Convert unsigned back to signed
	encoded := make([]int64, packed.Count)
	for i, v := range values {
		encoded[i] = int64(v)
	}

	return &EncodedBlock{
		Encoded:      encoded,
		FrameOfRef:   packed.FrameOfRef,
		BitWidth:     packed.BitWidth,
		ExceptionPos: packed.ExceptionPos,
		Exceptions:   packed.Exceptions,
		Exp:          packed.Exp,
		Factor:       packed.Factor,
	}
}

// fastPack8 packs 8 values at once for common bit widths.
// This is optimized for cache-line aligned access.
func fastPack8(dst []byte, src []uint64, bitWidth uint8) int {
	switch bitWidth {
	case 0:
		return 0
	case 1:
		dst[0] = byte(src[0]&1) | byte((src[1]&1)<<1) | byte((src[2]&1)<<2) | byte((src[3]&1)<<3) |
			byte((src[4]&1)<<4) | byte((src[5]&1)<<5) | byte((src[6]&1)<<6) | byte((src[7]&1)<<7)
		return 1
	case 2:
		dst[0] = byte(src[0]&3) | byte((src[1]&3)<<2) | byte((src[2]&3)<<4) | byte((src[3]&3)<<6)
		dst[1] = byte(src[4]&3) | byte((src[5]&3)<<2) | byte((src[6]&3)<<4) | byte((src[7]&3)<<6)
		return 2
	case 4:
		dst[0] = byte(src[0]&0xF) | byte((src[1]&0xF)<<4)
		dst[1] = byte(src[2]&0xF) | byte((src[3]&0xF)<<4)
		dst[2] = byte(src[4]&0xF) | byte((src[5]&0xF)<<4)
		dst[3] = byte(src[6]&0xF) | byte((src[7]&0xF)<<4)
		return 4
	case 8:
		dst[0] = byte(src[0])
		dst[1] = byte(src[1])
		dst[2] = byte(src[2])
		dst[3] = byte(src[3])
		dst[4] = byte(src[4])
		dst[5] = byte(src[5])
		dst[6] = byte(src[6])
		dst[7] = byte(src[7])
		return 8
	case 16:
		for i := 0; i < 8; i++ {
			dst[i*2] = byte(src[i])
			dst[i*2+1] = byte(src[i] >> 8)
		}
		return 16
	case 32:
		for i := 0; i < 8; i++ {
			dst[i*4] = byte(src[i])
			dst[i*4+1] = byte(src[i] >> 8)
			dst[i*4+2] = byte(src[i] >> 16)
			dst[i*4+3] = byte(src[i] >> 24)
		}
		return 32
	default:
		// Fall back to general packing for other bit widths
		return -1
	}
}

// fastUnpack8 unpacks 8 values at once for common bit widths.
func fastUnpack8(dst []uint64, src []byte, bitWidth uint8) {
	switch bitWidth {
	case 0:
		for i := range dst {
			dst[i] = 0
		}
	case 1:
		b := src[0]
		dst[0] = uint64(b & 1)
		dst[1] = uint64((b >> 1) & 1)
		dst[2] = uint64((b >> 2) & 1)
		dst[3] = uint64((b >> 3) & 1)
		dst[4] = uint64((b >> 4) & 1)
		dst[5] = uint64((b >> 5) & 1)
		dst[6] = uint64((b >> 6) & 1)
		dst[7] = uint64((b >> 7) & 1)
	case 2:
		dst[0] = uint64(src[0] & 3)
		dst[1] = uint64((src[0] >> 2) & 3)
		dst[2] = uint64((src[0] >> 4) & 3)
		dst[3] = uint64((src[0] >> 6) & 3)
		dst[4] = uint64(src[1] & 3)
		dst[5] = uint64((src[1] >> 2) & 3)
		dst[6] = uint64((src[1] >> 4) & 3)
		dst[7] = uint64((src[1] >> 6) & 3)
	case 4:
		dst[0] = uint64(src[0] & 0xF)
		dst[1] = uint64((src[0] >> 4) & 0xF)
		dst[2] = uint64(src[1] & 0xF)
		dst[3] = uint64((src[1] >> 4) & 0xF)
		dst[4] = uint64(src[2] & 0xF)
		dst[5] = uint64((src[2] >> 4) & 0xF)
		dst[6] = uint64(src[3] & 0xF)
		dst[7] = uint64((src[3] >> 4) & 0xF)
	case 8:
		dst[0] = uint64(src[0])
		dst[1] = uint64(src[1])
		dst[2] = uint64(src[2])
		dst[3] = uint64(src[3])
		dst[4] = uint64(src[4])
		dst[5] = uint64(src[5])
		dst[6] = uint64(src[6])
		dst[7] = uint64(src[7])
	case 16:
		for i := 0; i < 8; i++ {
			dst[i] = uint64(src[i*2]) | uint64(src[i*2+1])<<8
		}
	case 32:
		for i := 0; i < 8; i++ {
			dst[i] = uint64(src[i*4]) | uint64(src[i*4+1])<<8 |
				uint64(src[i*4+2])<<16 | uint64(src[i*4+3])<<24
		}
	}
}
