package srt

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
)

// SegmentsChecksum returns a stable SHA-256 checksum for a segment list.
// It is used to detect preprocessing or parsing differences.
func SegmentsChecksum(segments []Segment) [32]byte {
	h := sha256.New()
	io.WriteString(h, "segments_v1\n")
	io.WriteString(h, strconv.Itoa(len(segments)))
	io.WriteString(h, "\n")
	for _, seg := range segments {
		writeField(h, seg.StartTime)
		writeField(h, seg.EndTime)
		io.WriteString(h, strconv.Itoa(len(seg.Lines)))
		io.WriteString(h, "\n")
		for _, line := range seg.Lines {
			writeField(h, line)
		}
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// SegmentsChecksumHex returns a sha256-prefixed hex string checksum.
func SegmentsChecksumHex(segments []Segment) string {
	sum := SegmentsChecksum(segments)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func writeField(w io.Writer, value string) {
	io.WriteString(w, strconv.Itoa(len(value)))
	io.WriteString(w, ":")
	io.WriteString(w, value)
	io.WriteString(w, "\n")
}
