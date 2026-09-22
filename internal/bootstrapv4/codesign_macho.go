package bootstrapv4

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
)

// This file is pure parsing and hashing (no operating-system calls), so the
// binding logic is unit-tested on every platform. The operating-system half,
// which asks the kernel for the code-directory hash of the image it actually
// executed, is in process_image_darwin.go.

const (
	machoMagic64LE           = 0xfeedfacf
	machoLoadCodeSignature   = 0x1d
	csMagicEmbeddedSignature = 0xfade0cc0
	csMagicCodeDirectory     = 0xfade0c02
	csSlotCodeDirectory      = 0
	csSlotAlternateFirst     = 0x1000
	csSlotAlternateLast      = 0x1004
	csHashSHA1               = 1
	csHashSHA256             = 2
	csHashSHA256Truncated    = 3
	csHashSHA384             = 4
	csCDHashSize             = 20
)

func csHasher(hashType byte) (func() hash.Hash, bool) {
	switch hashType {
	case csHashSHA1:
		return sha1.New, true
	case csHashSHA256, csHashSHA256Truncated:
		return sha256.New, true
	case csHashSHA384:
		return sha512.New384, true
	}
	return nil, false
}

// signedCodeDirectory is one CodeDirectory embedded in a Mach-O signature.
type signedCodeDirectory struct {
	blob   []byte
	cdhash [csCDHashSize]byte
}

// machoSignature is the embedded signature of a thin 64-bit Mach-O image.
type machoSignature struct {
	codeSignatureOffset uint64
	directories         []signedCodeDirectory
}

func be32(b []byte) uint32 { return binary.BigEndian.Uint32(b) }

// parseMachoSignature reads the embedded code signature of a thin 64-bit
// little-endian Mach-O image. Universal (fat) images, images without an
// embedded signature, and malformed structures fail closed with an explicit
// reason: identity cannot then be established from the file.
func parseMachoSignature(data []byte) (machoSignature, error) {
	if len(data) < 32 {
		return machoSignature{}, errors.New("executable image is too small to be a Mach-O file")
	}
	switch magic := binary.LittleEndian.Uint32(data[0:4]); magic {
	case machoMagic64LE:
	case 0xcafebabe, 0xbebafeca, 0xcafebabf, 0xbfbafeca:
		return machoSignature{}, errors.New("universal (fat) Mach-O executables are not supported for image identity; ship a thin binary")
	default:
		return machoSignature{}, fmt.Errorf("executable image is not a thin 64-bit little-endian Mach-O file (magic %#x)", magic)
	}
	ncmds := binary.LittleEndian.Uint32(data[16:20])
	sizeofcmds := binary.LittleEndian.Uint32(data[20:24])
	if uint64(32)+uint64(sizeofcmds) > uint64(len(data)) {
		return machoSignature{}, errors.New("Mach-O load commands exceed the file")
	}
	var dataOffset, dataSize uint32
	found := false
	offset := uint64(32)
	end := offset + uint64(sizeofcmds)
	for i := uint32(0); i < ncmds; i++ {
		if offset+8 > end {
			return machoSignature{}, errors.New("truncated Mach-O load command")
		}
		cmd := binary.LittleEndian.Uint32(data[offset : offset+4])
		size := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if size < 8 || offset+size > end {
			return machoSignature{}, errors.New("malformed Mach-O load command size")
		}
		if cmd == machoLoadCodeSignature {
			if found {
				return machoSignature{}, errors.New("Mach-O image carries more than one code-signature command")
			}
			if size < 16 {
				return machoSignature{}, errors.New("malformed Mach-O code-signature command")
			}
			dataOffset = binary.LittleEndian.Uint32(data[offset+8 : offset+12])
			dataSize = binary.LittleEndian.Uint32(data[offset+12 : offset+16])
			found = true
		}
		offset += size
	}
	if !found {
		return machoSignature{}, errors.New("executable carries no embedded code signature; its loaded identity cannot be established (sign it, e.g. codesign -s - on darwin/amd64)")
	}
	if uint64(dataOffset)+uint64(dataSize) > uint64(len(data)) || dataSize < 12 {
		return machoSignature{}, errors.New("embedded code signature lies outside the file")
	}
	blob := data[dataOffset : uint64(dataOffset)+uint64(dataSize)]
	if be32(blob[0:4]) != csMagicEmbeddedSignature {
		return machoSignature{}, errors.New("embedded code signature is not an embedded-signature superblob")
	}
	length, count := uint64(be32(blob[4:8])), uint64(be32(blob[8:12]))
	if length > uint64(len(blob)) || 12+count*8 > length {
		return machoSignature{}, errors.New("embedded signature superblob is malformed")
	}
	signature := machoSignature{codeSignatureOffset: uint64(dataOffset)}
	for i := uint64(0); i < count; i++ {
		slotType, slotOffset := be32(blob[12+i*8:16+i*8]), uint64(be32(blob[16+i*8:20+i*8]))
		if slotType != csSlotCodeDirectory && (slotType < csSlotAlternateFirst || slotType > csSlotAlternateLast) {
			continue
		}
		if slotOffset+8 > length {
			return machoSignature{}, errors.New("CodeDirectory slot lies outside the superblob")
		}
		if be32(blob[slotOffset:slotOffset+4]) != csMagicCodeDirectory {
			return machoSignature{}, errors.New("CodeDirectory slot has the wrong magic")
		}
		cdLength := uint64(be32(blob[slotOffset+4 : slotOffset+8]))
		if cdLength < 44 || slotOffset+cdLength > length {
			return machoSignature{}, errors.New("CodeDirectory length is malformed")
		}
		cd := blob[slotOffset : slotOffset+cdLength]
		hashType := cd[37]
		newHash, ok := csHasher(hashType)
		if !ok {
			continue // A CodeDirectory the kernel cannot have chosen for an unknown hash.
		}
		h := newHash()
		h.Write(cd)
		var directory signedCodeDirectory
		directory.blob = cd
		copy(directory.cdhash[:], h.Sum(nil)[:csCDHashSize])
		signature.directories = append(signature.directories, directory)
	}
	if len(signature.directories) == 0 {
		return machoSignature{}, errors.New("embedded signature contains no supported CodeDirectory")
	}
	return signature, nil
}

// verifyExecutedImage proves that data (the bytes of the executable file) is
// the image the kernel says this process executed. kernelCDHash is the
// code-directory hash the kernel recorded for the executing process. The file
// must carry a CodeDirectory whose hash equals it, and every page of the file
// up to the directory's code limit must match that directory's page hashes.
// Matching the directory alone would let a CodeDirectory be paired with
// different code, so the page hashes are recomputed from the bytes.
func verifyExecutedImage(data []byte, kernelCDHash [csCDHashSize]byte) error {
	signature, err := parseMachoSignature(data)
	if err != nil {
		return err
	}
	var match *signedCodeDirectory
	for i := range signature.directories {
		if bytes.Equal(signature.directories[i].cdhash[:], kernelCDHash[:]) {
			match = &signature.directories[i]
			break
		}
	}
	if match == nil {
		return fmt.Errorf("the executable file does not carry the code identity this process was started with (kernel code-directory hash %x): the file was replaced or rewritten after this process started and must be restarted", kernelCDHash)
	}
	cd := match.blob
	version := be32(cd[8:12])
	nSpecial, nCode := uint64(be32(cd[24:28])), uint64(be32(cd[28:32]))
	codeLimit := uint64(be32(cd[32:36]))
	hashSize, hashType, pageShift := int(cd[36]), cd[37], uint(cd[39])
	if version >= 0x20100 && be32(cd[44:48]) != 0 {
		return errors.New("scatter-signed CodeDirectory is not supported for image identity")
	}
	if version >= 0x20300 && len(cd) >= 64 {
		if limit64 := binary.BigEndian.Uint64(cd[56:64]); limit64 != 0 {
			codeLimit = limit64
		}
	}
	newHash, _ := csHasher(hashType)
	if hashSize <= 0 || hashSize > newHash().Size() {
		return errors.New("CodeDirectory hash size is invalid")
	}
	hashOffset := uint64(be32(cd[16:20]))
	if hashOffset < nSpecial*uint64(hashSize) || hashOffset+nCode*uint64(hashSize) > uint64(len(cd)) {
		return errors.New("CodeDirectory hash slots lie outside the directory")
	}
	// The signature is appended after the signed code; anything between the
	// code limit and the signature would be unhashed.
	if codeLimit != signature.codeSignatureOffset || codeLimit > uint64(len(data)) {
		return fmt.Errorf("CodeDirectory code limit %d does not end exactly at the embedded signature (%d)", codeLimit, signature.codeSignatureOffset)
	}
	page := codeLimit
	if pageShift != 0 {
		if pageShift > 30 {
			return errors.New("CodeDirectory page size is invalid")
		}
		page = uint64(1) << pageShift
	}
	if page == 0 {
		return errors.New("CodeDirectory covers no code")
	}
	if want := (codeLimit + page - 1) / page; nCode != want {
		return fmt.Errorf("CodeDirectory has %d page hashes, the code limit requires %d", nCode, want)
	}
	h := newHash()
	for i := uint64(0); i < nCode; i++ {
		start := i * page
		stop := start + page
		if stop > codeLimit {
			stop = codeLimit
		}
		h.Reset()
		h.Write(data[start:stop])
		slot := cd[hashOffset+i*uint64(hashSize) : hashOffset+(i+1)*uint64(hashSize)]
		if !bytes.Equal(h.Sum(nil)[:hashSize], slot) {
			return fmt.Errorf("executable file page %d does not match the CodeDirectory the process executed: the file bytes differ from the running code", i)
		}
	}
	return nil
}
