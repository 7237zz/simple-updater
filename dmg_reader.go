package simpleupdater

// The in-memory UDIF reader in this file is adapted from the DMG reader in
// deploymenttheory/go-apfs-v2. See THIRD_PARTY_NOTICES.md.

import (
	"bytes"
	"compress/bzip2"
	"compress/zlib"
	"container/list"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/deploymenttheory/go-apfs-v2/pkg/disk"
	"github.com/go-compressions/lzfse"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
	"howett.net/plist"
)

const (
	dmgSectorSize    = 0x200
	dmgFooterSize    = 512
	dmgSignature     = "koly"
	dmgMishSignature = "mish"

	dmgChunkZeroFill     = 0x00000000
	dmgChunkUncompressed = 0x00000001
	dmgChunkIgnored      = 0x00000002
	dmgChunkADC          = 0x80000004
	dmgChunkZLIB         = 0x80000005
	dmgChunkBZ2          = 0x80000006
	dmgChunkLZFSE        = 0x80000007
	dmgChunkLZMA         = 0x80000008
	dmgChunkComment      = 0x7ffffffe
	dmgChunkLast         = 0xffffffff

	dmgChunkCacheMaxBytes = 128 << 20
)

var dmgXZMagic = []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}

type dmgFooter struct {
	Signature             [4]byte
	Version               uint32
	HeaderSize            uint32
	Flags                 uint32
	RunningDataForkOffset uint64
	DataForkOffset        uint64
	DataForkLength        uint64
	RsrcForkOffset        uint64
	RsrcForkLength        uint64
	SegmentNumber         uint32
	SegmentCount          uint32
	SegmentID             [16]byte
	DataChecksum          [136]byte
	PlistOffset           uint64
	PlistLength           uint64
	Reserved1             [64]byte
	CodeSignatureOffset   uint64
	CodeSignatureLength   uint64
	Reserved2             [40]byte
	MasterChecksum        [136]byte
	ImageVariant          uint32
	SectorCount           uint64
	Reserved3             uint32
	Reserved4             uint32
	Reserved5             uint32
}

type dmgBlockData struct {
	Signature        [4]byte
	Version          uint32
	StartSector      uint64
	SectorCount      uint64
	DataOffset       uint64
	BuffersNeeded    uint32
	BlockDescriptors uint32
	Reserved         [6]uint32
	Checksum         [136]byte
	ChunkCount       uint32
}

type dmgChunk struct {
	Type             uint32
	Comment          uint32
	DiskOffset       uint64
	DiskLength       uint64
	CompressedOffset uint64
	CompressedLength uint64
}

type dmgPartition struct {
	Name        string
	StartSector uint64
	SectorCount uint64
	DataOffset  uint64
	Chunks      []dmgChunk
}

type dmgPlist struct {
	ResourceFork *dmgResourceFork `plist:"resource-fork"`
}

type dmgResourceFork struct {
	Blkx []dmgBlkxEntry `plist:"blkx"`
}

type dmgBlkxEntry struct {
	Name   string `plist:"Name"`
	CFName string `plist:"CFName"`
	Data   []byte `plist:"Data"`
}

type setupDMGReader struct {
	source           io.ReaderAt
	size             int64
	footer           dmgFooter
	partitions       []dmgPartition
	fsPartitionIndex int
	fsOffset         uint64
	fsSize           uint64

	cacheMu   sync.Mutex
	cache     map[int][]byte
	cacheLRU  *list.List
	cacheElem map[int]*list.Element
	cacheSize int
}

func openSetupDMG(source io.ReaderAt, size int64) (*setupDMGReader, error) {
	if source == nil {
		return nil, fmt.Errorf("DMG reader is nil")
	}
	if size < dmgFooterSize {
		return nil, fmt.Errorf("DMG is too small: %d bytes", size)
	}

	reader := &setupDMGReader{
		source:           source,
		size:             size,
		fsPartitionIndex: -1,
		cache:            make(map[int][]byte),
		cacheLRU:         list.New(),
		cacheElem:        make(map[int]*list.Element),
	}

	if err := reader.readFooter(); err != nil {
		return nil, err
	}
	if err := reader.parsePlist(); err != nil {
		return nil, err
	}
	if err := reader.findFilesystemPartition(); err != nil {
		return nil, err
	}
	return reader, nil
}

func (r *setupDMGReader) Size() int64 {
	return int64(r.fsSize)
}

func (r *setupDMGReader) readFooter() error {
	footerData := make([]byte, dmgFooterSize)
	if _, err := r.source.ReadAt(footerData, r.size-dmgFooterSize); err != nil {
		return fmt.Errorf("read DMG footer: %w", err)
	}
	if err := binary.Read(bytes.NewReader(footerData), binary.BigEndian, &r.footer); err != nil {
		return fmt.Errorf("parse DMG footer: %w", err)
	}
	if string(r.footer.Signature[:]) != dmgSignature {
		return fmt.Errorf("invalid DMG signature: %q", r.footer.Signature[:])
	}
	return nil
}

func (r *setupDMGReader) parsePlist() error {
	if r.footer.PlistLength == 0 {
		return fmt.Errorf("DMG has no plist data")
	}
	if r.footer.PlistOffset > uint64(r.size) || r.footer.PlistLength > uint64(r.size)-r.footer.PlistOffset {
		return fmt.Errorf("DMG plist is outside the input bounds")
	}

	plistData := make([]byte, int(r.footer.PlistLength))
	if _, err := r.source.ReadAt(plistData, int64(r.footer.PlistOffset)); err != nil {
		return fmt.Errorf("read DMG plist: %w", err)
	}

	var doc dmgPlist
	if _, err := plist.Unmarshal(plistData, &doc); err != nil {
		return fmt.Errorf("parse DMG plist: %w", err)
	}
	if doc.ResourceFork == nil || len(doc.ResourceFork.Blkx) == 0 {
		return fmt.Errorf("DMG plist has no blkx entries")
	}

	for i := range doc.ResourceFork.Blkx {
		partition, err := r.parsePartition(&doc.ResourceFork.Blkx[i])
		if err != nil {
			continue
		}
		r.partitions = append(r.partitions, *partition)
	}
	if len(r.partitions) == 0 {
		return fmt.Errorf("DMG contains no readable partitions")
	}
	return nil
}

func (r *setupDMGReader) parsePartition(entry *dmgBlkxEntry) (*dmgPartition, error) {
	if entry == nil || len(entry.Data) == 0 {
		return nil, fmt.Errorf("empty DMG partition")
	}

	partition := &dmgPartition{Name: entry.Name}
	if partition.Name == "" {
		partition.Name = entry.CFName
	}

	buf := bytes.NewReader(entry.Data)
	var block dmgBlockData
	if err := binary.Read(buf, binary.BigEndian, &block); err != nil {
		return nil, fmt.Errorf("parse DMG block header: %w", err)
	}
	if string(block.Signature[:]) != dmgMishSignature {
		return nil, fmt.Errorf("invalid DMG block signature: %q", block.Signature[:])
	}

	partition.StartSector = block.StartSector
	partition.SectorCount = block.SectorCount
	partition.DataOffset = block.DataOffset
	partition.Chunks = make([]dmgChunk, 0, block.ChunkCount)

	for i := uint32(0); i < block.ChunkCount; i++ {
		var chunk dmgChunk
		if err := binary.Read(buf, binary.BigEndian, &chunk); err != nil {
			return nil, fmt.Errorf("parse DMG chunk %d: %w", i, err)
		}
		chunk.DiskOffset = (chunk.DiskOffset + block.StartSector) * dmgSectorSize
		chunk.DiskLength *= dmgSectorSize
		chunk.CompressedOffset += block.DataOffset + r.footer.DataForkOffset
		partition.Chunks = append(partition.Chunks, chunk)
	}

	return partition, nil
}

func (r *setupDMGReader) findFilesystemPartition() error {
	if err := r.parseGPT(); err == nil {
		return nil
	}

	for _, hint := range []string{"Apple_APFS", "Apple_HFSX", "Apple_HFS"} {
		for i := range r.partitions {
			partition := &r.partitions[i]
			if strings.Contains(partition.Name, hint) {
				r.fsPartitionIndex = i
				r.fsOffset = partition.StartSector * dmgSectorSize
				r.fsSize = partition.SectorCount * dmgSectorSize
				return nil
			}
		}
	}

	return fmt.Errorf("no APFS or HFS+ partition found in DMG")
}

func (r *setupDMGReader) parseGPT() error {
	var headerPart, tablePart *dmgPartition
	for i := range r.partitions {
		partition := &r.partitions[i]
		switch partition.Name {
		case "GPT Header (Primary GPT Header : 1)":
			headerPart = partition
		case "GPT Partition Data (Primary GPT Table : 2)":
			tablePart = partition
		}
	}

	if headerPart == nil || tablePart == nil {
		for i := range r.partitions {
			partition := &r.partitions[i]
			if partition.Name == "disk image" || partition.Name == "(disk image)" || partition.Name == "GPT Partition Data" {
				r.fsPartitionIndex = i
				r.fsOffset = partition.StartSector * dmgSectorSize
				r.fsSize = partition.SectorCount * dmgSectorSize
				return nil
			}
		}
		return fmt.Errorf("DMG has no GPT metadata")
	}

	headerData, err := r.readPartitionData(headerPart)
	if err != nil {
		return fmt.Errorf("read GPT header: %w", err)
	}
	var header dmgGPTHeader
	if err := binary.Read(bytes.NewReader(headerData), binary.LittleEndian, &header); err != nil {
		return fmt.Errorf("parse GPT header: %w", err)
	}
	if err := header.verify(); err != nil {
		return err
	}

	tableData, err := r.readPartitionData(tablePart)
	if err != nil {
		return fmt.Errorf("read GPT partition table: %w", err)
	}
	partitions := make([]dmgGPTPartition, header.EntriesCount)
	if err := binary.Read(bytes.NewReader(tableData), binary.LittleEndian, &partitions); err != nil {
		return fmt.Errorf("parse GPT partition table: %w", err)
	}

	for _, gptPartition := range partitions {
		if gptPartition.isEmpty() {
			continue
		}
		typeGUID := gptPartition.Type.String()
		if typeGUID != dmgAppleAPFSGUID && typeGUID != dmgAppleHFSGUID {
			continue
		}
		for i := range r.partitions {
			partition := &r.partitions[i]
			if partition.StartSector == gptPartition.StartingLBA {
				r.fsPartitionIndex = i
				r.fsOffset = gptPartition.StartingLBA * dmgSectorSize
				r.fsSize = (gptPartition.EndingLBA - gptPartition.StartingLBA + 1) * dmgSectorSize
				return nil
			}
		}
	}

	return fmt.Errorf("APFS or HFS+ partition not found in GPT")
}

func (r *setupDMGReader) readPartitionData(partition *dmgPartition) ([]byte, error) {
	var output bytes.Buffer
	for i := range partition.Chunks {
		chunk := &partition.Chunks[i]
		if chunk.Type == dmgChunkComment || chunk.Type == dmgChunkLast {
			continue
		}
		data, err := r.decompressChunk(chunk)
		if err != nil {
			return nil, err
		}
		output.Write(data)
	}
	return output.Bytes(), nil
}

func (r *setupDMGReader) ReadAt(buf []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("negative DMG read offset: %d", off)
	}
	if r.fsPartitionIndex < 0 || r.fsPartitionIndex >= len(r.partitions) {
		return 0, fmt.Errorf("invalid DMG filesystem partition")
	}
	if off >= int64(r.fsSize) {
		return 0, io.EOF
	}

	originalLen := len(buf)
	requestLen := originalLen
	if remaining := int64(r.fsSize) - off; int64(requestLen) > remaining {
		requestLen = int(remaining)
	}
	buf = buf[:requestLen]

	absoluteOffset := off + int64(r.fsOffset)
	partition := &r.partitions[r.fsPartitionIndex]
	chunks := partition.Chunks

	n := 0
	chunkIndex := 0
	for chunkIndex < len(chunks) && int64(chunks[chunkIndex].DiskOffset+chunks[chunkIndex].DiskLength) <= absoluteOffset {
		chunkIndex++
	}

	for n < len(buf) && chunkIndex < len(chunks) {
		chunk := &chunks[chunkIndex]
		chunkStart := int64(chunk.DiskOffset)
		chunkEnd := int64(chunk.DiskOffset + chunk.DiskLength)

		if absoluteOffset < chunkStart {
			gap := min(int64(len(buf)-n), chunkStart-absoluteOffset)
			clear(buf[n : n+int(gap)])
			n += int(gap)
			absoluteOffset += gap
			continue
		}
		if absoluteOffset >= chunkEnd {
			chunkIndex++
			continue
		}

		chunkData, err := r.getChunk(chunkIndex)
		if err != nil {
			return n, fmt.Errorf("decompress DMG chunk: %w", err)
		}
		readStart := absoluteOffset - chunkStart
		available := int64(len(chunkData)) - readStart
		if available <= 0 {
			chunkIndex++
			continue
		}
		copyLen := int(min(int64(len(buf)-n), available))
		copy(buf[n:n+copyLen], chunkData[readStart:readStart+int64(copyLen)])
		n += copyLen
		absoluteOffset += int64(copyLen)
		if absoluteOffset >= chunkEnd {
			chunkIndex++
		}
	}

	if n < len(buf) {
		clear(buf[n:])
		n = len(buf)
	}
	if requestLen < originalLen {
		return n, io.EOF
	}
	return n, nil
}

func (r *setupDMGReader) getChunk(index int) ([]byte, error) {
	chunk := &r.partitions[r.fsPartitionIndex].Chunks[index]
	if chunk.Type == dmgChunkZeroFill || chunk.Type == dmgChunkIgnored || chunk.Type == dmgChunkComment || chunk.Type == dmgChunkLast {
		return r.decompressChunk(chunk)
	}

	r.cacheMu.Lock()
	if data, ok := r.cache[index]; ok {
		r.cacheLRU.MoveToFront(r.cacheElem[index])
		r.cacheMu.Unlock()
		return data, nil
	}
	r.cacheMu.Unlock()

	data, err := r.decompressChunk(chunk)
	if err != nil {
		return nil, err
	}

	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	if _, exists := r.cache[index]; !exists {
		r.cache[index] = data
		r.cacheElem[index] = r.cacheLRU.PushFront(index)
		r.cacheSize += len(data)
		for r.cacheSize > dmgChunkCacheMaxBytes && r.cacheLRU.Len() > 1 {
			oldest := r.cacheLRU.Back()
			oldIndex := oldest.Value.(int)
			r.cacheSize -= len(r.cache[oldIndex])
			delete(r.cache, oldIndex)
			delete(r.cacheElem, oldIndex)
			r.cacheLRU.Remove(oldest)
		}
	}
	return data, nil
}

func (r *setupDMGReader) decompressChunk(chunk *dmgChunk) ([]byte, error) {
	switch chunk.Type {
	case dmgChunkZeroFill, dmgChunkIgnored:
		return make([]byte, int(chunk.DiskLength)), nil
	case dmgChunkComment, dmgChunkLast:
		return nil, nil
	}

	if chunk.CompressedOffset > uint64(r.size) || chunk.CompressedLength > uint64(r.size)-chunk.CompressedOffset {
		return nil, fmt.Errorf("DMG chunk is outside the input bounds")
	}
	compressed := make([]byte, int(chunk.CompressedLength))
	if _, err := r.source.ReadAt(compressed, int64(chunk.CompressedOffset)); err != nil {
		return nil, err
	}

	switch chunk.Type {
	case dmgChunkUncompressed:
		return compressed, nil
	case dmgChunkZLIB:
		reader, err := zlib.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	case dmgChunkBZ2:
		return io.ReadAll(bzip2.NewReader(bytes.NewReader(compressed)))
	case dmgChunkLZFSE:
		data, err := lzfse.Decompress(compressed)
		if err != nil {
			return nil, fmt.Errorf("LZFSE decompression failed: %w", err)
		}
		return data, nil
	case dmgChunkLZMA:
		var reader io.Reader
		var err error
		if bytes.HasPrefix(compressed, dmgXZMagic) {
			reader, err = xz.NewReader(bytes.NewReader(compressed))
		} else {
			reader, err = lzma.NewReader(bytes.NewReader(compressed))
		}
		if err != nil {
			return nil, fmt.Errorf("LZMA decompression failed: %w", err)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("LZMA decompression failed: %w", err)
		}
		return data, nil
	case dmgChunkADC:
		data, err := disk.DecompressADC(compressed, int(chunk.DiskLength))
		if err != nil {
			return nil, fmt.Errorf("ADC decompression failed: %w", err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported DMG chunk type: 0x%x", chunk.Type)
	}
}

const (
	dmgGPTSignature  = "EFI PART"
	dmgGPTSectorSize = 0x200
	dmgAppleAPFSGUID = "7C3457EF-0000-11AA-AA11-00306543ECAC"
	dmgAppleHFSGUID  = "48465300-0000-11AA-AA11-00306543ECAC"
)

type dmgGPTGUID [16]byte

func (u dmgGPTGUID) String() string {
	return fmt.Sprintf("%02X%02X%02X%02X-%02X%02X-%02X%02X-%02X%02X-%02X%02X%02X%02X%02X%02X",
		u[3], u[2], u[1], u[0],
		u[5], u[4],
		u[7], u[6],
		u[8], u[9],
		u[10], u[11], u[12], u[13], u[14], u[15],
	)
}

type dmgGPTMagic [8]byte

type dmgGPTHeader struct {
	Signature       dmgGPTMagic
	Revision        uint32
	HeaderSize      uint32
	CRC32           uint32
	Reserved        uint32
	HeaderStartLBA  uint64
	BackupLBA       uint64
	FirstUsableLBA  uint64
	LastUsableLBA   uint64
	DiskGUID        dmgGPTGUID
	EntriesStart    uint64
	EntriesCount    uint32
	EntriesSize     uint32
	PartitionsCRC32 uint32
	Padding         [420]byte
}

func (h dmgGPTHeader) verify() error {
	if h.EntriesSize != 128 {
		return fmt.Errorf("unsupported GPT entry size: %d", h.EntriesSize)
	}
	if dmgGPTSectorSize-len(h.Padding) != int(h.HeaderSize) {
		return fmt.Errorf("invalid GPT header size: %d", h.HeaderSize)
	}
	if string(h.Signature[:]) != dmgGPTSignature {
		return fmt.Errorf("invalid GPT signature: %q", h.Signature[:])
	}
	return nil
}

type dmgGPTPartition struct {
	Type               dmgGPTGUID
	ID                 dmgGPTGUID
	StartingLBA        uint64
	EndingLBA          uint64
	Attributes         uint64
	PartitionNameUTF16 [72]uint8
}

func (p dmgGPTPartition) isEmpty() bool {
	return p.Type == (dmgGPTGUID{})
}
