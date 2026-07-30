package alp

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
)

// Compressor provides a unified interface for ALP compression.
// It automatically selects between ALP and ALP RD based on data characteristics.
type Compressor struct {
	cfg            Config
	encoder        Encoder
	rdEncoder      *RDEncoder
	encodedScratch []int64
	encodedBlock   EncodedBlock
	scheme         Scheme
}

// NewCompressor creates a new ALP compressor.
func NewCompressor(cfg Config) *Compressor {
	return &Compressor{
		cfg:     cfg,
		encoder: Encoder{cfg: cfg},
	}
}

// Analyze samples the data and selects the best compression scheme.
func (c *Compressor) Analyze(data []float64) Scheme {
	scheme, _ := c.analyze(data)
	return scheme
}

// analyze returns the encoded sample block so an immediate Compress call can
// pack it directly into the result's shared byte arena.
func (c *Compressor) analyze(data []float64) (Scheme, *EncodedBlock) {
	if len(data) == 0 {
		c.scheme = SchemeInvalid
		return c.scheme, nil
	}

	originalSize := len(data) * 8 // 8 bytes per float64

	// Sample and analyze with ALP
	c.encoder.Sample(data)

	// Actually encode a sample to measure real compressed size
	sampleSize := min(len(data), VectorSize)
	sample := data[:sampleSize]

	var block *EncodedBlock
	if sampleSize == VectorSize {
		block = c.encodeVector(sample)
	} else {
		block = c.encodePartial(sample)
	}

	// Calculate actual ALP size per value
	alpBlockSize := 21 + packedDataLen(len(block.Encoded), block.BitWidth) + len(block.ExceptionPos)*2 + len(block.Exceptions)*8
	alpBytesPerValue := float64(alpBlockSize) / float64(sampleSize)

	// Extrapolate to full data
	numBlocks := (len(data) + VectorSize - 1) / VectorSize
	alpEstSize := int(alpBytesPerValue*float64(len(data))) + numBlocks*5

	// If ALP compresses well (< 95% of original), use it
	if float64(alpEstSize) < float64(originalSize)*0.95 {
		c.scheme = SchemeALP
		return c.scheme, block
	}

	// For now, skip ALP RD as it doesn't compress well in current implementation
	// Use raw passthrough instead
	c.scheme = SchemeRaw
	return c.scheme, nil
}

func (c *Compressor) encodeVector(data []float64) *EncodedBlock {
	if cap(c.encodedScratch) < VectorSize {
		c.encodedScratch = make([]int64, VectorSize)
	}
	c.encodedScratch = c.encodedScratch[:VectorSize]
	c.encoder.encodeVectorTo(data, c.encodedScratch, false, &c.encodedBlock)
	return &c.encodedBlock
}

func (c *Compressor) encodePartial(data []float64) *EncodedBlock {
	if cap(c.encodedScratch) < len(data) {
		c.encodedScratch = make([]int64, len(data))
	}
	c.encodedScratch = c.encodedScratch[:len(data)]
	c.encoder.encodeTo(data, c.encodedScratch, &c.encodedBlock)
	return &c.encodedBlock
}

// Compress compresses a slice of float64 values.
func (c *Compressor) Compress(data []float64) *CompressedData {
	if len(data) == 0 {
		return &CompressedData{Scheme: SchemeInvalid}
	}

	var firstBlock *EncodedBlock
	// Analyze if not already done, retaining the sample block for compression.
	if c.scheme == SchemeInvalid {
		_, firstBlock = c.analyze(data)
	}

	result := &CompressedData{
		Scheme: c.scheme,
		Count:  len(data),
	}

	switch c.scheme {
	case SchemeALP:
		result.ALPBlocks = c.compressALP(data, firstBlock)
	case SchemeALPRD:
		result.RDBlocks = c.compressRD(data)
	case SchemeRaw:
		result.RawData = make([]uint64, len(data))
		for i, v := range data {
			result.RawData[i] = math.Float64bits(v)
		}
	}

	return result
}

// compressALP compresses data using the standard ALP algorithm.
func (c *Compressor) compressALP(data []float64, firstBlock *EncodedBlock) []*PackedBlock {
	n := len(data)
	numVectors := (n + VectorSize - 1) / VectorSize
	blocks := make([]*PackedBlock, 0, numVectors)
	storage := make([]PackedBlock, numVectors)
	estimatedBytes := 0
	if firstBlock != nil {
		estimatedBytes = packedDataLen(n, firstBlock.BitWidth)
	}
	dataStorage := make([]byte, 0, estimatedBytes)
	start := 0
	if firstBlock != nil {
		dataLen := packedDataLen(len(firstBlock.Encoded), firstBlock.BitWidth)
		dataStorage = append(dataStorage, make([]byte, dataLen)...)
		if len(firstBlock.Encoded) == VectorSize {
			packBlockAbsoluteDataTo(firstBlock, &storage[0], dataStorage)
		} else {
			packBlockDataTo(firstBlock, &storage[0], dataStorage)
		}
		blocks = append(blocks, &storage[0])
		start = min(VectorSize, n)
	}

	for i := start; i < n; i += VectorSize {
		end := i + VectorSize
		if end > n {
			end = n
		}

		var encoded *EncodedBlock
		if end-i == VectorSize {
			encoded = c.encodeVector(data[i:end])
		} else {
			encoded = c.encodePartial(data[i:end])
		}

		if end-i == VectorSize {
			packed := &storage[len(blocks)]
			if len(blocks) == 0 && cap(dataStorage) == 0 {
				dataStorage = make([]byte, 0, packedDataLen(n, encoded.BitWidth))
			}
			dataStart := len(dataStorage)
			dataStorage = append(dataStorage, make([]byte, packedDataLen(len(encoded.Encoded), encoded.BitWidth))...)
			packBlockAbsoluteDataTo(encoded, packed, dataStorage[dataStart:])
			blocks = append(blocks, packed)
		} else {
			packed := &storage[len(blocks)]
			if len(blocks) == 0 && cap(dataStorage) == 0 {
				dataStorage = make([]byte, 0, packedDataLen(n, encoded.BitWidth))
			}
			dataStart := len(dataStorage)
			dataStorage = append(dataStorage, make([]byte, packedDataLen(len(encoded.Encoded), encoded.BitWidth))...)
			packBlockDataTo(encoded, packed, dataStorage[dataStart:])
			blocks = append(blocks, packed)
		}
	}

	return blocks
}

// compressRD compresses data using ALP RD algorithm.
func (c *Compressor) compressRD(data []float64) []*RDEncodedBlock {
	if c.rdEncoder == nil {
		c.rdEncoder = NewRDEncoder(c.cfg)
	}

	n := len(data)
	numVectors := (n + VectorSize - 1) / VectorSize
	blocks := make([]*RDEncodedBlock, 0, numVectors)

	for i := 0; i < n; i += VectorSize {
		end := i + VectorSize
		if end > n {
			end = n
		}

		var encoded *RDEncodedBlock
		if end-i == VectorSize {
			encoded = c.rdEncoder.EncodeVector(data[i:end])
		} else {
			encoded = c.rdEncoder.Encode(data[i:end])
		}

		blocks = append(blocks, encoded)
	}

	return blocks
}

// CompressedData holds the compressed representation.
type CompressedData struct {
	Scheme    Scheme
	Count     int
	ALPBlocks []*PackedBlock
	RDBlocks  []*RDEncodedBlock
	RawData   []uint64 // For SchemeRaw - uncompressed float bits
}

// Decompressor provides a unified interface for ALP decompression.
type Decompressor struct {
	decoder   *Decoder
	rdDecoder *RDDecoder
}

// NewDecompressor creates a new ALP decompressor.
func NewDecompressor() *Decompressor {
	return &Decompressor{
		decoder:   NewDecoder(),
		rdDecoder: NewRDDecoder(),
	}
}

// Decompress decompresses data back to float64 values.
func (d *Decompressor) Decompress(data *CompressedData) []float64 {
	if data.Count == 0 {
		return nil
	}

	var result []float64
	switch data.Scheme {
	case SchemeALP:
		result = make([]float64, data.Count)
		written := 0
		for _, packed := range data.ALPBlocks {
			written += d.decoder.decodePackedInto(packed, result[written:])
		}
	case SchemeALPRD:
		result = make([]float64, 0, data.Count)
		for _, block := range data.RDBlocks {
			result = append(result, d.rdDecoder.Decode(block)...)
		}
	case SchemeRaw:
		result = make([]float64, len(data.RawData))
		for i, bits := range data.RawData {
			result[i] = math.Float64frombits(bits)
		}
		return result
	default:
		return nil
	}

	// Trim to exact count (last block may have been padded)
	if len(result) > data.Count {
		result = result[:data.Count]
	}

	return result
}

// DecompressInto decompresses into a provided slice.
func (d *Decompressor) DecompressInto(data *CompressedData, dst []float64) int {
	if data.Count == 0 || len(dst) == 0 {
		return 0
	}

	written := 0

	switch data.Scheme {
	case SchemeALP:
		for _, packed := range data.ALPBlocks {
			written += d.decoder.decodePackedInto(packed, dst[written:])
			if written >= len(dst) {
				break
			}
		}
	case SchemeALPRD:
		for _, block := range data.RDBlocks {
			decoded := d.rdDecoder.Decode(block)
			n := copy(dst[written:], decoded)
			written += n
			if written >= len(dst) {
				break
			}
		}
	case SchemeRaw:
		n := min(len(data.RawData), len(dst))
		for i, bits := range data.RawData[:n] {
			dst[i] = math.Float64frombits(bits)
		}
		written = n
	}

	return written
}

// Wire format for serialization:
// [1 byte: scheme] [4 bytes: count] [blocks...]

// WriteTo serializes compressed data to a writer.
func (cd *CompressedData) WriteTo(w io.Writer) (int64, error) {
	var written int64

	// Write header
	header := make([]byte, 5)
	header[0] = byte(cd.Scheme)
	binary.LittleEndian.PutUint32(header[1:], uint32(cd.Count))
	n, err := w.Write(header)
	written += int64(n)
	if err != nil {
		return written, err
	}

	switch cd.Scheme {
	case SchemeALP:
		for _, block := range cd.ALPBlocks {
			n64, err := writePackedBlock(w, block)
			written += n64
			if err != nil {
				return written, err
			}
		}
	case SchemeALPRD:
		for _, block := range cd.RDBlocks {
			n64, err := writeRDBlock(w, block)
			written += n64
			if err != nil {
				return written, err
			}
		}
	case SchemeRaw:
		for _, bits := range cd.RawData {
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, bits)
			n, err := w.Write(buf)
			written += int64(n)
			if err != nil {
				return written, err
			}
		}
	}

	return written, nil
}

// ReadFrom deserializes compressed data from a reader.
func (cd *CompressedData) ReadFrom(r io.Reader) (int64, error) {
	var read int64

	// Read header
	header := make([]byte, 5)
	n, err := io.ReadFull(r, header)
	read += int64(n)
	if err != nil {
		return read, err
	}

	cd.Scheme = Scheme(header[0])
	cd.Count = int(binary.LittleEndian.Uint32(header[1:]))

	// Calculate number of blocks
	numBlocks := (cd.Count + VectorSize - 1) / VectorSize

	switch cd.Scheme {
	case SchemeALP:
		cd.ALPBlocks = make([]*PackedBlock, numBlocks)
		for i := range cd.ALPBlocks {
			block := &PackedBlock{}
			n64, err := readPackedBlock(r, block)
			read += n64
			if err != nil {
				return read, err
			}
			cd.ALPBlocks[i] = block
		}
	case SchemeALPRD:
		cd.RDBlocks = make([]*RDEncodedBlock, numBlocks)
		for i := range cd.RDBlocks {
			block := &RDEncodedBlock{}
			n64, err := readRDBlock(r, block)
			read += n64
			if err != nil {
				return read, err
			}
			cd.RDBlocks[i] = block
		}
	case SchemeRaw:
		cd.RawData = make([]uint64, cd.Count)
		for i := range cd.RawData {
			buf := make([]byte, 8)
			n, err := io.ReadFull(r, buf)
			read += int64(n)
			if err != nil {
				return read, err
			}
			cd.RawData[i] = binary.LittleEndian.Uint64(buf)
		}
	}

	return read, nil
}

func writePackedBlock(w io.Writer, block *PackedBlock) (int64, error) {
	var written int64

	// Block header: count(4) + frameOfRef(8) + bitWidth(1) + exp(1) + factor(1) + numExceptions(2)
	header := make([]byte, 17)
	binary.LittleEndian.PutUint32(header[0:], uint32(block.Count))
	binary.LittleEndian.PutUint64(header[4:], uint64(block.FrameOfRef))
	header[12] = block.BitWidth
	header[13] = block.Exp
	header[14] = block.Factor
	binary.LittleEndian.PutUint16(header[15:], uint16(len(block.ExceptionPos)))

	n, err := w.Write(header)
	written += int64(n)
	if err != nil {
		return written, err
	}

	// Write packed data
	dataLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(dataLen, uint32(len(block.Data)))
	n, err = w.Write(dataLen)
	written += int64(n)
	if err != nil {
		return written, err
	}

	n, err = w.Write(block.Data)
	written += int64(n)
	if err != nil {
		return written, err
	}

	// Write exceptions
	for _, pos := range block.ExceptionPos {
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, pos)
		n, err = w.Write(buf)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}

	for _, exc := range block.Exceptions {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, exc)
		n, err = w.Write(buf)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

func readPackedBlock(r io.Reader, block *PackedBlock) (int64, error) {
	var read int64

	header := make([]byte, 17)
	n, err := io.ReadFull(r, header)
	read += int64(n)
	if err != nil {
		return read, err
	}

	block.Count = int(binary.LittleEndian.Uint32(header[0:]))
	block.FrameOfRef = int64(binary.LittleEndian.Uint64(header[4:]))
	block.BitWidth = header[12]
	block.Exp = header[13]
	block.Factor = header[14]
	numExceptions := binary.LittleEndian.Uint16(header[15:])

	// Read packed data
	dataLenBuf := make([]byte, 4)
	n, err = io.ReadFull(r, dataLenBuf)
	read += int64(n)
	if err != nil {
		return read, err
	}
	dataLen := binary.LittleEndian.Uint32(dataLenBuf)

	block.Data = make([]byte, dataLen)
	n, err = io.ReadFull(r, block.Data)
	read += int64(n)
	if err != nil {
		return read, err
	}

	// Read exceptions
	block.ExceptionPos = make([]uint16, numExceptions)
	for i := range block.ExceptionPos {
		buf := make([]byte, 2)
		n, err = io.ReadFull(r, buf)
		read += int64(n)
		if err != nil {
			return read, err
		}
		block.ExceptionPos[i] = binary.LittleEndian.Uint16(buf)
	}

	block.Exceptions = make([]uint64, numExceptions)
	for i := range block.Exceptions {
		buf := make([]byte, 8)
		n, err = io.ReadFull(r, buf)
		read += int64(n)
		if err != nil {
			return read, err
		}
		block.Exceptions[i] = binary.LittleEndian.Uint64(buf)
	}

	return read, nil
}

func writeRDBlock(w io.Writer, block *RDEncodedBlock) (int64, error) {
	var written int64

	count := len(block.RightParts)

	// Header: count(4) + cuttingPos(1) + leftBitWidth(1) + rightBitWidth(1) + dictSize(1) + numExceptions(2)
	header := make([]byte, 10)
	binary.LittleEndian.PutUint32(header[0:], uint32(count))
	header[4] = block.CuttingPos
	header[5] = block.LeftBitWidth
	header[6] = block.RightBitWidth
	header[7] = uint8(len(block.Dictionary))
	binary.LittleEndian.PutUint16(header[8:], uint16(len(block.ExceptionPos)))

	n, err := w.Write(header)
	written += int64(n)
	if err != nil {
		return written, err
	}

	// Write dictionary
	for _, entry := range block.Dictionary {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, entry)
		n, err = w.Write(buf)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}

	// Write right parts
	for _, rp := range block.RightParts {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, rp)
		n, err = w.Write(buf)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}

	// Write left encoded
	n, err = w.Write(block.LeftEncoded)
	written += int64(n)
	if err != nil {
		return written, err
	}

	// Write exceptions
	for _, pos := range block.ExceptionPos {
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, pos)
		n, err = w.Write(buf)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}

	for _, exc := range block.Exceptions {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, exc)
		n, err = w.Write(buf)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

func readRDBlock(r io.Reader, block *RDEncodedBlock) (int64, error) {
	var read int64

	header := make([]byte, 10)
	n, err := io.ReadFull(r, header)
	read += int64(n)
	if err != nil {
		return read, err
	}

	count := int(binary.LittleEndian.Uint32(header[0:]))
	block.CuttingPos = header[4]
	block.LeftBitWidth = header[5]
	block.RightBitWidth = header[6]
	dictSize := int(header[7])
	numExceptions := binary.LittleEndian.Uint16(header[8:])

	// Read dictionary
	block.Dictionary = make([]uint64, dictSize)
	for i := range block.Dictionary {
		buf := make([]byte, 8)
		n, err = io.ReadFull(r, buf)
		read += int64(n)
		if err != nil {
			return read, err
		}
		block.Dictionary[i] = binary.LittleEndian.Uint64(buf)
	}

	// Read right parts
	block.RightParts = make([]uint64, count)
	for i := range block.RightParts {
		buf := make([]byte, 8)
		n, err = io.ReadFull(r, buf)
		read += int64(n)
		if err != nil {
			return read, err
		}
		block.RightParts[i] = binary.LittleEndian.Uint64(buf)
	}

	// Read left encoded
	block.LeftEncoded = make([]uint8, count)
	n, err = io.ReadFull(r, block.LeftEncoded)
	read += int64(n)
	if err != nil {
		return read, err
	}

	// Read exceptions
	block.ExceptionPos = make([]uint16, numExceptions)
	for i := range block.ExceptionPos {
		buf := make([]byte, 2)
		n, err = io.ReadFull(r, buf)
		read += int64(n)
		if err != nil {
			return read, err
		}
		block.ExceptionPos[i] = binary.LittleEndian.Uint16(buf)
	}

	block.Exceptions = make([]uint64, numExceptions)
	for i := range block.Exceptions {
		buf := make([]byte, 8)
		n, err = io.ReadFull(r, buf)
		read += int64(n)
		if err != nil {
			return read, err
		}
		block.Exceptions[i] = binary.LittleEndian.Uint64(buf)
	}

	return read, nil
}

// Convenience functions for simple usage

// CompressFloat64 compresses a slice of float64 values.
func CompressFloat64(data []float64) *CompressedData {
	c := NewCompressor(DefaultConfig())
	return c.Compress(data)
}

// DecompressFloat64 decompresses data back to float64 values.
func DecompressFloat64(data *CompressedData) []float64 {
	d := NewDecompressor()
	return d.Decompress(data)
}

// CompressFloat32 compresses a slice of float32 values by converting to float64.
// This is a convenience wrapper; for optimal float32 compression, use the
// specialized float32 encoder directly.
func CompressFloat32(data []float32) *CompressedData {
	f64 := make([]float64, len(data))
	for i, v := range data {
		f64[i] = float64(v)
	}
	return CompressFloat64(f64)
}

// DecompressFloat32 decompresses data back to float32 values.
func DecompressFloat32(data *CompressedData) []float32 {
	f64 := DecompressFloat64(data)
	f32 := make([]float32, len(f64))
	for i, v := range f64 {
		f32[i] = float32(v)
	}
	return f32
}

// CompressionRatio calculates the compression ratio achieved.
// Returns compressed size / original size.
func (cd *CompressedData) CompressionRatio() float64 {
	if cd.Count == 0 {
		return 1.0
	}

	originalSize := cd.Count * 8 // 8 bytes per float64
	compressedSize := cd.CompressedSize()

	return float64(compressedSize) / float64(originalSize)
}

// CompressedSize returns the approximate compressed size in bytes.
func (cd *CompressedData) CompressedSize() int {
	size := 5 // header

	switch cd.Scheme {
	case SchemeALP:
		for _, block := range cd.ALPBlocks {
			size += 21 // block header
			size += len(block.Data)
			size += len(block.ExceptionPos) * 2
			size += len(block.Exceptions) * 8
		}
	case SchemeALPRD:
		for _, block := range cd.RDBlocks {
			size += 10 // block header
			size += len(block.Dictionary) * 8
			size += len(block.RightParts) * 8
			size += len(block.LeftEncoded)
			size += len(block.ExceptionPos) * 2
			size += len(block.Exceptions) * 8
		}
	case SchemeRaw:
		size += len(cd.RawData) * 8
	}

	return size
}

// Bytes serializes compressed data to bytes.
func (cd *CompressedData) Bytes() []byte {
	var buf bytes.Buffer
	cd.WriteTo(&buf)
	return buf.Bytes()
}

// FromBytes deserializes compressed data from bytes.
func (cd *CompressedData) FromBytes(data []byte) error {
	_, err := cd.ReadFrom(bytes.NewReader(data))
	return err
}

// VerifyLossless checks that compression is truly lossless.
func VerifyLossless(original []float64, compressed *CompressedData) bool {
	decompressed := DecompressFloat64(compressed)
	if len(decompressed) != len(original) {
		return false
	}

	for i := range original {
		// Handle NaN specially (NaN != NaN)
		if math.IsNaN(original[i]) && math.IsNaN(decompressed[i]) {
			continue
		}
		if original[i] != decompressed[i] {
			// Check bit-exact equality
			if math.Float64bits(original[i]) != math.Float64bits(decompressed[i]) {
				return false
			}
		}
	}

	return true
}
