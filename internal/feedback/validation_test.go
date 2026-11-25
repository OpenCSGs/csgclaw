package feedback

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"testing"
)

func TestValidateFeedback(t *testing.T) {
	for _, tc := range []struct {
		text     string
		count    int
		archives int
		valid    bool
	}{
		{"text", 0, 0, true}, {"", 1, 0, true}, {"", 0, 1, true}, {" ", 0, 0, false}, {"text", 7, 0, false}, {"text", 0, 4, false}, {strings.Repeat("中", MaxDescriptionLength+1), 0, 0, false},
	} {
		if err := ValidateContent(tc.text, tc.count, tc.archives); (err == nil) != tc.valid {
			t.Errorf("count=%d valid=%v err=%v", tc.count, tc.valid, err)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseImage(buffer.Bytes()); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{nil, []byte("fake.png"), make([]byte, MaxImageBytes+1)} {
		if _, err := ParseImage(data); err == nil {
			t.Error("accepted invalid image")
		}
	}
}

func TestParseArchive(t *testing.T) {
	var zipData, tarData, gzipData bytes.Buffer
	if err := zip.NewWriter(&zipData).Close(); err != nil {
		t.Fatal(err)
	}
	tarWriter := tar.NewWriter(&tarData)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "diagnostic.txt", Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzip.NewWriter(&gzipData).Close(); err != nil {
		t.Fatal(err)
	}
	for extension, data := range map[string][]byte{"zip": zipData.Bytes(), "tar": tarData.Bytes(), "gz": gzipData.Bytes()} {
		archive, err := ParseArchive(data)
		if err != nil || archive.Extension != extension {
			t.Errorf("extension=%s archive=%s err=%v", extension, archive.Extension, err)
		}
	}
	for _, data := range [][]byte{nil, []byte("diagnostic.zip"), []byte("PK\x03\x04"), make([]byte, MaxArchiveBytes+1)} {
		if _, err := ParseArchive(data); err == nil {
			t.Error("accepted invalid archive")
		}
	}
}

func TestParseArchiveRejectsTruncatedContainers(t *testing.T) {
	var gzipData, tarData bytes.Buffer
	gz := gzip.NewWriter(&gzipData)
	if _, err := gz.Write([]byte("diagnostics")); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(&tarData)
	if err := tw.WriteHeader(&tar.Header{Name: "diagnostics", Size: 11, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("diagnostics")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	rar4Main := []byte{0, 0, 0x73, 0, 0, 13, 0, 0, 0, 0, 0, 0, 0}
	rar4End := []byte{0, 0, 0x7b, 0, 0, 7, 0}
	for _, header := range [][]byte{rar4Main, rar4End} {
		binary.LittleEndian.PutUint16(header, uint16(crc32.ChecksumIEEE(header[2:])))
	}
	rar4 := append(append([]byte(rar4Signature), rar4Main...), rar4End...)
	rar5 := []byte(rar5Signature)
	for _, header := range [][]byte{{3, 1, 0, 0}, {3, 5, 0, 0}} {
		rar5 = binary.LittleEndian.AppendUint32(rar5, crc32.ChecksumIEEE(header))
		rar5 = append(rar5, header...)
	}

	for _, tc := range []struct {
		name, extension string
		data            []byte
		cut             int
	}{
		{"gzip", "gz", gzipData.Bytes(), 8},
		{"tar", "tar", tarData.Bytes(), 1024},
		{"rar4", "rar", rar4, len(rar4End)},
		{"rar5", "rar", rar5, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive, err := ParseArchive(tc.data)
			if err != nil || archive.Extension != tc.extension {
				t.Fatalf("valid archive was rejected: %v", err)
			}
			if _, err := ParseArchive(tc.data[:len(tc.data)-tc.cut]); err == nil {
				t.Fatal("truncated archive was accepted")
			}
		})
	}
}

func TestParse7ZipChecksNextHeader(t *testing.T) {
	data := make([]byte, 32)
	copy(data, "7z\xbc\xaf\x27\x1c")
	data[7] = 4
	packedData := []byte{1, 2, 3}
	nextHeader := []byte{1, 0}
	binary.LittleEndian.PutUint64(data[12:20], uint64(len(packedData)))
	binary.LittleEndian.PutUint64(data[20:28], uint64(len(nextHeader)))
	binary.LittleEndian.PutUint32(data[28:32], crc32.ChecksumIEEE(nextHeader))
	binary.LittleEndian.PutUint32(data[8:12], crc32.ChecksumIEEE(data[12:32]))
	data = append(append(data, packedData...), nextHeader...)

	if archive, err := ParseArchive(data); err != nil || archive.Extension != "7z" {
		t.Fatalf("valid 7z rejected: %v", err)
	}
	for _, size := range []int{32, len(data) - 1} {
		if _, err := ParseArchive(data[:size]); err == nil {
			t.Fatalf("accepted 7z truncated to %d bytes", size)
		}
	}
	data[len(data)-1] ^= 1
	if _, err := ParseArchive(data); err == nil {
		t.Fatal("accepted 7z with invalid next-header checksum")
	}
}

func TestParseZipRejectsCorruptEntryWithIntactDirectory(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: "diagnostic.txt", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("diagnostic body")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if archive, err := ParseArchive(data.Bytes()); err != nil || archive.Extension != "zip" {
		t.Fatalf("valid ZIP rejected: %v", err)
	}

	archive, err := zip.NewReader(bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := archive.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	data.Bytes()[offset] ^= 1
	if _, err := ParseArchive(data.Bytes()); err == nil {
		t.Fatal("accepted ZIP with corrupt entry and intact central directory")
	}
}
