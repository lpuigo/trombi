package imageio

import "encoding/binary"

// readJPEGOrientation does a minimal, dependency-free scan of a JPEG file's
// markers to find the EXIF Orientation tag (0x0112), without pulling in a
// general-purpose EXIF library (see Docs/SPEC_trombinoscope.md §7/§8: a
// phone photo whose orientation tag is ignored would otherwise fail face
// detection with no visible cause). It returns ok=false if there is no EXIF
// APP1 segment or no Orientation tag.
func readJPEGOrientation(data []byte) (orientation int, ok bool) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, false
	}

	pos := 2
	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			return 0, false
		}
		marker := data[pos+1]
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			pos += 2
			continue
		}
		if marker == 0xDA { // start of scan: no more markers to inspect
			return 0, false
		}
		segLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		if segLen < 2 || pos+2+segLen > len(data) {
			return 0, false
		}
		segment := data[pos+4 : pos+2+segLen]

		if marker == 0xE1 && len(segment) >= 6 && string(segment[:6]) == "Exif\x00\x00" {
			return parseTIFFOrientation(segment[6:])
		}

		pos += 2 + segLen
	}
	return 0, false
}

// parseTIFFOrientation reads the Orientation tag (0x0112) out of a TIFF
// header (the payload of an EXIF APP1 segment, after the "Exif\x00\x00"
// preamble).
func parseTIFFOrientation(tiff []byte) (int, bool) {
	if len(tiff) < 8 {
		return 0, false
	}

	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, false
	}

	ifdOffset := order.Uint32(tiff[4:8])
	if int(ifdOffset)+2 > len(tiff) {
		return 0, false
	}

	entryCount := int(order.Uint16(tiff[ifdOffset : ifdOffset+2]))
	base := int(ifdOffset) + 2
	const entrySize = 12
	for i := range entryCount {
		start := base + i*entrySize
		if start+entrySize > len(tiff) {
			break
		}
		tag := order.Uint16(tiff[start : start+2])
		if tag != 0x0112 {
			continue
		}
		value := int(order.Uint16(tiff[start+8 : start+10]))
		if value < 1 || value > 8 {
			return 0, false
		}
		return value, true
	}
	return 0, false
}
