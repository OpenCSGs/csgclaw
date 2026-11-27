package feedback

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
)

const (
	rar4Signature = "Rar!\x1a\x07\x00"
	rar5Signature = "Rar!\x1a\x07\x01\x00"
)

// Check block headers and packed sizes without decompressing file data.
func validRAR(data []byte) bool {
	if bytes.HasPrefix(data, []byte(rar5Signature)) {
		return validRAR5(data[len(rar5Signature):])
	}
	return validRAR4(data[len(rar4Signature):])
}

func validRAR4(data []byte) bool {
	const mainBlock = 0x73
	const endBlock = 0x7b
	first := true
	for len(data) >= 7 {
		kind := data[2]
		flags := binary.LittleEndian.Uint16(data[3:5])
		size := int(binary.LittleEndian.Uint16(data[5:7]))
		if size < 7 || size > len(data) || uint16(crc32.ChecksumIEEE(data[2:size])) != binary.LittleEndian.Uint16(data) {
			return false
		}
		if first && (kind != mainBlock || size < 13 || flags&0x0080 != 0) {
			return false
		}
		first = false
		packedSize := uint64(0)
		if flags&0x8000 != 0 {
			if size < 11 {
				return false
			}
			packedSize = uint64(binary.LittleEndian.Uint32(data[7:11]))
			if kind == 0x74 && flags&0x0100 != 0 {
				if size < 40 {
					return false
				}
				packedSize |= uint64(binary.LittleEndian.Uint32(data[32:36])) << 32
			}
		}
		if packedSize > uint64(len(data)-size) {
			return false
		}
		data = data[size+int(packedSize):]
		if kind == endBlock {
			return true
		}
	}
	return false
}

func validRAR5(data []byte) bool {
	first := true
	for len(data) >= 5 {
		size, sizeBytes := binary.Uvarint(data[4:])
		if sizeBytes <= 0 || sizeBytes > 3 || size > uint64(len(data)-4-sizeBytes) {
			return false
		}
		headerEnd := 4 + sizeBytes + int(size)
		if crc32.ChecksumIEEE(data[4:headerEnd]) != binary.LittleEndian.Uint32(data) {
			return false
		}
		header := bytes.NewReader(data[4+sizeBytes : headerEnd])
		kind, kindErr := binary.ReadUvarint(header)
		flags, flagsErr := binary.ReadUvarint(header)
		if kindErr != nil || flagsErr != nil || (first && kind != 1) {
			return false
		}
		first = false
		if flags&1 != 0 {
			extraSize, err := binary.ReadUvarint(header)
			if err != nil || extraSize > uint64(header.Len()) {
				return false
			}
		}
		packedSize := uint64(0)
		if flags&2 != 0 {
			var err error
			packedSize, err = binary.ReadUvarint(header)
			if err != nil {
				return false
			}
		}
		if packedSize > uint64(len(data)-headerEnd) {
			return false
		}
		if kind == 5 {
			return true
		}
		data = data[headerEnd+int(packedSize):]
	}
	return false
}
