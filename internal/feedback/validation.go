package feedback

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	MaxImages            = 6
	MaxImageBytes        = 5 * 1024 * 1024
	MaxArchives          = 3
	MaxArchiveBytes      = 10 * 1024 * 1024
	MaxDescriptionLength = 20000
	MaxRequestBytes      = MaxImages*MaxImageBytes + MaxArchives*MaxArchiveBytes + 1024*1024
)

type Image struct {
	Data      []byte
	Extension string
}

func ParseImage(data []byte) (Image, error) {
	if len(data) == 0 || len(data) > MaxImageBytes {
		return Image{}, fmt.Errorf("image exceeds the 5 MB limit or is empty")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || config.Width <= 0 || config.Height <= 0 {
		return Image{}, fmt.Errorf("invalid JPEG or PNG image")
	}
	if format == "jpeg" {
		format = "jpg"
	}
	return Image{Data: data, Extension: format}, nil
}

const maxExpandedArchiveBytes = 256 * 1024 * 1024

type Archive struct {
	Data      []byte
	Extension string
}

// ParseArchive identifies the container without extracting its contents.
func ParseArchive(data []byte) (Archive, error) {
	if len(data) == 0 || len(data) > MaxArchiveBytes {
		return Archive{}, fmt.Errorf("archive exceeds the 10 MB limit or is empty")
	}
	var extension string
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")), bytes.HasPrefix(data, []byte("PK\x05\x06")):
		if validZip(data) {
			extension = "zip"
		}
	case bytes.HasPrefix(data, []byte(rar4Signature)), bytes.HasPrefix(data, []byte(rar5Signature)):
		if validRAR(data) {
			extension = "rar"
		}
	case bytes.HasPrefix(data, []byte("7z\xbc\xaf\x27\x1c")):
		if valid7Zip(data) {
			extension = "7z"
		}
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		if validGzip(data) {
			extension = "gz"
		}
	default:
		if validTar(data) {
			extension = "tar"
		}
	}
	if extension == "" {
		return Archive{}, fmt.Errorf("invalid archive format")
	}
	return Archive{Data: data, Extension: extension}, nil
}

func validZip(data []byte) bool {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	remaining := int64(maxExpandedArchiveBytes)
	for _, file := range archive.File {
		offset, err := file.DataOffset()
		if err != nil || offset < 0 || offset > int64(len(data)) ||
			file.CompressedSize64 > uint64(int64(len(data))-offset) {
			return false
		}
		// Preserve support for encrypted ZIPs and methods unavailable in archive/zip.
		// Their directory, local headers, and packed sizes can still be checked.
		if file.Flags&1 != 0 || (file.Method != zip.Store && file.Method != zip.Deflate) {
			continue
		}
		if file.UncompressedSize64 > uint64(remaining) {
			return false
		}
		reader, err := file.Open()
		if err != nil {
			return false
		}
		checksum := crc32.NewIEEE()
		size, err := io.Copy(checksum, io.LimitReader(reader, remaining+1))
		reader.Close()
		if err != nil || size != int64(file.UncompressedSize64) || checksum.Sum32() != file.CRC32 {
			return false
		}
		remaining -= size
	}
	return true
}

func valid7Zip(data []byte) bool {
	const signatureHeaderSize = 32
	if len(data) < signatureHeaderSize ||
		crc32.ChecksumIEEE(data[12:signatureHeaderSize]) != binary.LittleEndian.Uint32(data[8:12]) {
		return false
	}
	offset := binary.LittleEndian.Uint64(data[12:20])
	size := binary.LittleEndian.Uint64(data[20:28])
	rest := data[signatureHeaderSize:]
	if offset > uint64(len(rest)) || size > uint64(len(rest))-offset {
		return false
	}
	return crc32.ChecksumIEEE(rest[offset:offset+size]) == binary.LittleEndian.Uint32(data[28:32])
}

func validGzip(data []byte) bool {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return false
	}
	defer reader.Close()
	size, err := io.Copy(io.Discard, io.LimitReader(reader, maxExpandedArchiveBytes+1))
	return err == nil && size <= maxExpandedArchiveBytes
}

func validTar(data []byte) bool {
	const blockSize = 512
	if len(data) < 2*blockSize || len(data)%blockSize != 0 {
		return false
	}
	var terminator [2 * blockSize]byte
	source := bytes.NewReader(data)
	reader := tar.NewReader(source)
	remaining := int64(maxExpandedArchiveBytes)
	for {
		beforeHeader := source.Len()
		header, err := reader.Next()
		if err == io.EOF {
			if beforeHeader-source.Len() < len(terminator) {
				return false
			}
			end := len(data) - source.Len()
			return bytes.Equal(data[end-len(terminator):end], terminator[:]) &&
				bytes.Count(data[end:], []byte{0}) == source.Len()
		}
		if err != nil || header.Size > remaining {
			return false
		}
		remaining -= header.Size
		if _, err := io.Copy(io.Discard, reader); err != nil || source.Len() < len(terminator) {
			return false
		}
	}
}

func ValidateContent(description string, count, archiveCount int) error {
	if archiveCount > MaxArchives {
		return fmt.Errorf("at most 3 archives are allowed")
	}
	if count > MaxImages {
		return fmt.Errorf("at most 6 images are allowed")
	}
	if utf8.RuneCountInString(description) > MaxDescriptionLength {
		return fmt.Errorf("feedback text exceeds the length limit")
	}
	if strings.TrimSpace(description) == "" && count == 0 && archiveCount == 0 {
		return fmt.Errorf("provide feedback text or attachments")
	}
	return nil
}
