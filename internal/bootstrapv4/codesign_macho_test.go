package bootstrapv4

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"testing"
)

// syntheticImage builds a thin 64-bit Mach-O whose signed code is `pages`
// 4 KiB pages and whose single sha256 CodeDirectory covers exactly that code.
type syntheticOptions struct {
	pages           int
	noSignature     bool
	scatter         bool
	codeLimitDelta  int
	fat             bool
	flipHashSlot    bool
	wrongSlotMagic  bool
	hashType        byte
	pageShift       byte
	extraSignature  int
	skipCodeDirSlot bool
	// honestLimit makes the page hashes cover exactly the (shifted) code limit,
	// so the only defect left is bytes between the limit and the signature.
	honestLimit bool
}

func syntheticImage(t testing.TB, o syntheticOptions) ([]byte, [csCDHashSize]byte) {
	t.Helper()
	if o.pages == 0 {
		o.pages = 3
	}
	if o.hashType == 0 {
		o.hashType = csHashSHA256
	}
	if o.pageShift == 0 {
		o.pageShift = 12
	}
	page := 1 << o.pageShift
	const headerSize, cmdSize = 32, 16
	codeLen := o.pages*page - 100 // last page is partial
	code := make([]byte, codeLen)
	for i := range code {
		code[i] = byte(i*7 + 3)
	}
	sigOffset := codeLen
	nSlots := (codeLen + page - 1) / page
	const hashSize = 32
	cdHeader := 88
	cd := make([]byte, cdHeader+nSlots*hashSize)
	binary.BigEndian.PutUint32(cd[0:], csMagicCodeDirectory)
	binary.BigEndian.PutUint32(cd[4:], uint32(len(cd)))
	binary.BigEndian.PutUint32(cd[8:], 0x20400)
	binary.BigEndian.PutUint32(cd[16:], uint32(cdHeader))
	binary.BigEndian.PutUint32(cd[28:], uint32(nSlots))
	binary.BigEndian.PutUint32(cd[32:], uint32(sigOffset+o.codeLimitDelta))
	cd[36], cd[37], cd[39] = hashSize, o.hashType, o.pageShift
	if o.scatter {
		binary.BigEndian.PutUint32(cd[44:], 1)
	}
	for i := 0; i < nSlots; i++ {
		start, stop := i*page, (i+1)*page
		if stop > codeLen {
			stop = codeLen
		}
		sum := sha256.Sum256(code[start:stop])
		copy(cd[cdHeader+i*hashSize:], sum[:])
	}
	if o.flipHashSlot {
		cd[cdHeader] ^= 0xff
	}
	blobLen := 12 + 8 + len(cd)
	blob := make([]byte, blobLen)
	binary.BigEndian.PutUint32(blob[0:], csMagicEmbeddedSignature)
	binary.BigEndian.PutUint32(blob[4:], uint32(blobLen))
	binary.BigEndian.PutUint32(blob[8:], 1)
	slotType := uint32(csSlotCodeDirectory)
	if o.skipCodeDirSlot {
		slotType = 5 // a requirements-like slot, ignored
	}
	binary.BigEndian.PutUint32(blob[12:], slotType)
	binary.BigEndian.PutUint32(blob[16:], 20)
	copy(blob[20:], cd)
	if o.wrongSlotMagic {
		binary.BigEndian.PutUint32(blob[20:], 0)
	}
	// cdhash of the directory as the kernel would report it.
	sum := sha256.Sum256(cd)
	var cdhash [csCDHashSize]byte
	copy(cdhash[:], sum[:csCDHashSize])

	image := make([]byte, headerSize+cmdSize, sigOffset+len(blob)+o.extraSignature)
	binary.LittleEndian.PutUint32(image[0:], machoMagic64LE)
	if o.fat {
		binary.LittleEndian.PutUint32(image[0:], 0xcafebabe)
	}
	if o.noSignature {
		binary.LittleEndian.PutUint32(image[16:], 0)
		binary.LittleEndian.PutUint32(image[20:], 0)
		image = image[:headerSize]
	} else {
		binary.LittleEndian.PutUint32(image[16:], 1)
		binary.LittleEndian.PutUint32(image[20:], cmdSize)
		binary.LittleEndian.PutUint32(image[32:], machoLoadCodeSignature)
		binary.LittleEndian.PutUint32(image[36:], cmdSize)
		binary.LittleEndian.PutUint32(image[40:], uint32(sigOffset))
		binary.LittleEndian.PutUint32(image[44:], uint32(len(blob)))
	}
	// Code fills [len(header+cmds), sigOffset); the header lives inside it.
	for len(image) < sigOffset {
		image = append(image, code[len(image)])
	}
	copy(code, image[:headerSize+cmdSize]) // keep hashes consistent with the header bytes
	// Recompute hashes over the final code bytes.
	final := append([]byte(nil), image[:sigOffset]...)
	for i := 0; i < nSlots; i++ {
		start, stop := i*page, (i+1)*page
		limit := sigOffset
		if o.honestLimit {
			limit = sigOffset + o.codeLimitDelta
		}
		if stop > limit {
			stop = limit
		}
		sum := sha256.Sum256(final[start:stop])
		copy(cd[cdHeader+i*hashSize:], sum[:])
	}
	if o.flipHashSlot {
		cd[cdHeader] ^= 0xff
	}
	copy(blob[20:], cd)
	if o.wrongSlotMagic {
		binary.BigEndian.PutUint32(blob[20:], 0)
	}
	cdSum := sha256.Sum256(cd)
	copy(cdhash[:], cdSum[:csCDHashSize])
	image = append(image, blob...)
	image = append(image, make([]byte, o.extraSignature)...)
	return image, cdhash
}

func TestVerifyExecutedImageAcceptsExactlyTheSignedImage(t *testing.T) {
	image, cdhash := syntheticImage(t, syntheticOptions{})
	if err := verifyExecutedImage(image, cdhash); err != nil {
		t.Fatalf("a genuine signed image was refused: %v", err)
	}
	// A signature-container-only change (bytes after the code limit, outside the
	// CodeDirectory) is not a change of executing code.
	padded, _ := syntheticImage(t, syntheticOptions{extraSignature: 16})
	if err := verifyExecutedImage(padded, cdhash); err != nil {
		t.Fatalf("padding after the signature container changed code identity: %v", err)
	}
}

func TestVerifyExecutedImageRefusesEveryCodeChange(t *testing.T) {
	image, cdhash := syntheticImage(t, syntheticOptions{})
	// Any single byte flip anywhere in the signed code, header included, is
	// refused, even though the CodeDirectory (and therefore the cdhash) is
	// untouched: matching the directory alone must not be enough.
	sigOffset := int(binary.LittleEndian.Uint32(image[40:44]))
	for _, at := range []int{0, 5, 33, 100, 4095, 4096, 4097, 8191, 8192, sigOffset - 1} {
		mutated := append([]byte(nil), image...)
		mutated[at] ^= 0x01
		if err := verifyExecutedImage(mutated, cdhash); err == nil {
			t.Fatalf("a flipped byte at %d was accepted", at)
		}
	}
	// The kernel's cdhash naming a different directory is refused.
	other := cdhash
	other[0] ^= 0xff
	if err := verifyExecutedImage(image, other); err == nil {
		t.Fatal("a file whose CodeDirectory the kernel never recorded was accepted")
	}
	// A directory paired with foreign code: same signature blob, different code.
	foreign, _ := syntheticImage(t, syntheticOptions{pages: 3})
	foreign[1000] ^= 0x55
	if err := verifyExecutedImage(foreign, cdhash); err == nil {
		t.Fatal("a CodeDirectory paired with foreign code was accepted")
	}
}

func TestVerifyExecutedImageFailsClosedOnUnsupportedOrMalformedImages(t *testing.T) {
	good, cdhash := syntheticImage(t, syntheticOptions{})
	cases := map[string]syntheticOptions{
		"no embedded signature":               {noSignature: true},
		"universal binary":                    {fat: true},
		"scatter-signed directory":            {scatter: true},
		"code limit before the signature":     {codeLimitDelta: -1},
		"code limit past the signature":       {codeLimitDelta: 1},
		"corrupted page hash":                 {flipHashSlot: true},
		"directory slot with the wrong magic": {wrongSlotMagic: true},
		"no directory slot":                   {skipCodeDirSlot: true},
	}
	for name, options := range cases {
		image, hash := syntheticImage(t, options)
		if err := verifyExecutedImage(image, hash); err == nil {
			t.Errorf("%s: accepted", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
	// Truncation at every length never panics and is never accepted.
	for length := 0; length < len(good); length += 97 {
		if err := verifyExecutedImage(good[:length], cdhash); err == nil {
			t.Fatalf("truncated image of %d bytes was accepted", length)
		}
	}
	if err := verifyExecutedImage(nil, cdhash); err == nil {
		t.Fatal("empty image was accepted")
	}
}

func TestVerifyExecutedImageSupportsOtherPageSizes(t *testing.T) {
	image, cdhash := syntheticImage(t, syntheticOptions{pageShift: 14})
	if err := verifyExecutedImage(image, cdhash); err != nil {
		t.Fatalf("16 KiB pages: %v", err)
	}
}

// Bytes between the code limit and the embedded signature would be executed
// (they are part of the file) yet covered by no page hash. Even when every page
// hash is honest for the shortened limit, the exact-limit check refuses.
func TestVerifyExecutedImageRefusesUnhashedBytesBeforeTheSignature(t *testing.T) {
	good, cdhash := syntheticImage(t, syntheticOptions{})
	if err := verifyExecutedImage(good, cdhash); err != nil {
		t.Fatalf("control: %v", err)
	}
	short, shortHash := syntheticImage(t, syntheticOptions{codeLimitDelta: -1, honestLimit: true})
	if err := verifyExecutedImage(short, shortHash); err == nil || !strings.Contains(err.Error(), "does not end exactly at the embedded signature") {
		t.Fatalf("an image with unhashed bytes before its signature was accepted: %v", err)
	}
}
