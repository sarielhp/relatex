package mediamodel

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"regexp"
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
)

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// PixelLimit is PNGImage::PIXEL_LIMIT: pixels the browser decodes, a 16384-pixel square.
const PixelLimit = 268_435_456

// MaxBytes is the largest media representation an asset holds.
const MaxBytes = 67_108_864

var pngFields = []string{"sha256", "bytes", "pixel_width", "pixel_height", "mime_type", "data_uri", "source_data_uri",
	"png_sha256", "png_bytes", "trailing_bytes"}

var pngDepths = map[byte][]byte{0: {1, 2, 4, 8, 16}, 2: {8, 16}, 3: {1, 2, 4, 8}, 4: {8, 16}, 6: {8, 16}}

var chunkName = regexp.MustCompile(`\A[A-Za-z]{4}\z`)

// PNG is PNGImage: bounded static PNG syntax and byte identity.
type PNG struct {
	data     []byte
	header   []uint32
	color    byte
	hasData  bool
	closed   bool
	ended    bool
	palette  bool
	pngBytes int
	Metadata rec.Map
}

func hexDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// PNGFromBytes is PNGImage.from_bytes: the declared identity fields of expected must match.
func PNGFromBytes(data []byte, expected rec.Map) *PNG {
	if len(data) < 33 || len(data) > MaxBytes || !bytes.HasPrefix(data, pngSignature) || string(data[12:16]) != "IHDR" {
		rec.Fail("Missing PNG header")
	}
	identity := rec.Map{"sha256": hexDigest(data), "bytes": int64(len(data)),
		"pixel_width": int64(binary.BigEndian.Uint32(data[16:20])), "pixel_height": int64(binary.BigEndian.Uint32(data[20:24]))}
	for key, value := range identity {
		if declared, ok := expected[key]; ok && !rec.Equal(declared, value) {
			rec.Fail("PNG media identity differs")
		}
	}
	return NewPNG(data, identity)
}

// NewPNG is PNGImage.new(bytes, expected:).
func NewPNG(data []byte, expected rec.Map) *PNG {
	if len(data) < 33 || len(data) > MaxBytes || !bytes.HasPrefix(data, pngSignature) {
		rec.Fail("Unsupported or oversized PNG source")
	}
	p := &PNG{data: data}
	p.scanChunks()
	p.Metadata = rec.Map{"sha256": hexDigest(data), "bytes": int64(len(data)), "pixel_width": int64(p.header[0]),
		"pixel_height": int64(p.header[1]), "mime_type": "image/png", "png_bytes": int64(p.pngBytes),
		"trailing_bytes": int64(len(data) - p.pngBytes), "png_sha256": hexDigest(data[:p.pngBytes])}
	for _, key := range []string{"sha256", "bytes", "pixel_width", "pixel_height"} {
		if !rec.Equal(p.Metadata[key], expected[key]) {
			rec.Fail("PNG bytes differ from observed source identity")
		}
	}
	return p
}

// Descriptor is PNGImage#descriptor.
func (p *PNG) Descriptor() rec.Map {
	result := rec.Map{}
	for key, value := range p.Metadata {
		result[key] = value
	}
	result["source_data_uri"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(p.data)
	result["data_uri"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(p.data[:p.pngBytes])
	return result
}

// ValidatePNGDescriptor is PNGImage.validate_descriptor!.
func ValidatePNGDescriptor(data rec.Map) rec.Map {
	valid := data != nil && rec.SameKeys(data, pngFields) && data["mime_type"] == "image/png"
	for _, key := range []string{"data_uri", "source_data_uri"} {
		text, ok := data[key].(string)
		valid = valid && ok && len(text) >= len(pngPrefix) && text[:len(pngPrefix)] == pngPrefix
	}
	if !valid {
		rec.Fail("Invalid PNG asset descriptor")
	}
	source, err := strictDecode(data["source_data_uri"].(string)[len(pngPrefix):])
	if err != nil {
		rec.Fail("Invalid PNG asset encoding: %s", err.Error())
	}
	if !rec.Equal(NewPNG(source, data).Descriptor(), data) {
		rec.Fail("Noncanonical PNG asset encoding")
	}
	return data
}

const pngPrefix = "data:image/png;base64,"

type base64Error string

func (e base64Error) Error() string { return string(e) }

// strictDecode is Base64.strict_decode64: no line breaks, canonical padding.
func strictDecode(text string) ([]byte, error) {
	if bytes.ContainsAny([]byte(text), "\r\n") {
		return nil, base64Error("invalid base64")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil {
		return nil, base64Error("invalid base64")
	}
	return data, nil
}

func (p *PNG) scanChunks() {
	offset := 8
	chunks := 0
	for offset < len(p.data) {
		if offset+12 > len(p.data) {
			rec.Fail("Truncated PNG chunk")
		}
		length := int(binary.BigEndian.Uint32(p.data[offset:]))
		if offset+12+length > len(p.data) || length < 0 {
			rec.Fail("Oversized PNG chunk")
		}
		name := p.data[offset+4 : offset+8]
		payload := p.data[offset+8 : offset+8+length]
		crc := binary.BigEndian.Uint32(p.data[offset+8+length:])
		if !chunkName.Match(name) || crc32.ChecksumIEEE(p.data[offset+4:offset+8+length]) != crc {
			rec.Fail("Invalid PNG chunk checksum or type")
		}
		p.chunk(string(name), payload)
		offset += 12 + length
		if p.ended {
			p.pngBytes = offset
			break
		}
		chunks++
		if chunks > 4096 {
			rec.Fail("Excessive PNG chunk inventory")
		}
	}
	if !p.ended || !p.hasData || (p.color == 3 && !p.palette) {
		rec.Fail("Incomplete PNG image")
	}
}

func (p *PNG) chunk(name string, payload []byte) {
	if p.header == nil && name != "IHDR" {
		rec.Fail("PNG must start with IHDR")
	}
	switch name {
	case "IHDR":
		p.readHeader(payload)
	case "PLTE":
		if p.palette || p.hasData || len(payload) == 0 || len(payload) > 768 || len(payload)%3 > 0 {
			rec.Fail("Invalid PNG palette")
		}
		p.palette = true
	case "IDAT":
		if p.closed {
			rec.Fail("Noncontiguous PNG data")
		}
		p.hasData = true
	case "IEND":
		if len(payload) != 0 || !p.hasData {
			rec.Fail("Invalid PNG end marker")
		}
		p.ended = true
	default:
		p.ancillary(name, payload)
	}
}

func (p *PNG) ancillary(name string, payload []byte) {
	if (name[0] >= 'A' && name[0] <= 'Z') || slices.Contains([]string{"acTL", "fcTL", "fdAT"}, name) {
		rec.Fail("Unsupported PNG critical or animated data")
	}
	// Browsers apply an EXIF orientation that the PDF ignores; only the default is safe.
	if name == "eXIf" {
		if orientation, found := exifOrientation(payload); found && orientation != 1 {
			rec.Fail("Unsupported PNG EXIF orientation")
		}
	}
	if p.hasData {
		p.closed = true
	}
}

// exifOrientation is the Orientation tag (0x0112) of the first TIFF image directory.
func exifOrientation(payload []byte) (uint16, bool) {
	if len(payload) < 2 {
		rec.Fail("Malformed PNG EXIF data")
	}
	var order binary.ByteOrder
	switch string(payload[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		rec.Fail("Malformed PNG EXIF data")
	}
	if len(payload) < 8 || order.Uint16(payload[2:]) != 42 {
		rec.Fail("Malformed PNG EXIF data")
	}
	directory := int(order.Uint32(payload[4:]))
	if directory < 0 || directory+2 > len(payload) {
		rec.Fail("Malformed PNG EXIF data")
	}
	count := int(order.Uint16(payload[directory:]))
	if len(payload) < directory+2+count*12 {
		rec.Fail("Malformed PNG EXIF data")
	}
	for index := 0; index < count; index++ {
		entry := payload[directory+2+index*12:]
		if order.Uint16(entry) == 0x0112 {
			return order.Uint16(entry[8:]), true
		}
	}
	return 0, false
}

func (p *PNG) readHeader(payload []byte) {
	if p.header != nil || len(payload) != 13 {
		rec.Fail("Duplicate or malformed PNG header")
	}
	width, height := binary.BigEndian.Uint32(payload), binary.BigEndian.Uint32(payload[4:])
	depth, color, compression, filter, interlace := payload[8], payload[9], payload[10], payload[11], payload[12]
	p.header = []uint32{width, height}
	p.color = color
	if width == 0 || height == 0 || uint64(width)*uint64(height) > PixelLimit || !slices.Contains(pngDepths[color], depth) ||
		compression != 0 || filter != 0 || interlace > 1 {
		rec.Fail("Unsupported PNG header geometry or encoding")
	}
}
