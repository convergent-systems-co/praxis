# N3 report — Darwin process-image identity (internal/bootstrapv4 only)

Task: repair Astra Review 3 N3 (in-place overwrite of a running executable accepted on darwin).

## 1. Claim now provable
- darwin/arm64 (tested here): `VerifyProcessImage` succeeds only if (a) the bytes read through the init-time descriptor hash to the manifest digest, (b) the manifest path still names that file object, AND (c) the kernel's exec-time code-directory hash for this process (csops CS_OPS_CDHASH) equals the hash of a CodeDirectory embedded in those same bytes, with every code page of those bytes matching that CodeDirectory's page hashes, and the kernel reports CS_VALID. (c) binds the hashed bytes to the code the process EXECUTES, so in-place overwrite through the same inode and a replacement between exec and package init are both refused. Not covered: bytes after the signed code limit (the signature container), which are covered only by (a). Unsigned executable, universal binary, scatter-signed directory, malformed signature, csops failure, or CS_VALID clear => fail closed with an explicit message. darwin/amd64 compiles/vets; unsigned amd64 builds fail closed by design (not run here).
- linux/other (code only, NOT tested for behaviour): no kernel code identity consulted. Guarantee rests on the OS refusing writes to executing images (Linux ETXTBSY), which this package does not prove; replacement between exec and package init is not observable. Windows unqualified. Stated in imageBindings and the VerifyProcessImage doc.

## 2. Empirical csops experiments (darwin 25.6.0 arm64, go1.27.1, CGO not required: syscall.Syscall6(169,...))
scratch: scratchpad/n3/exp (main.go, drive.py). A and B differ only by -X marker, same length; Go ad-hoc linker-signed.
- none:            RUNNING A cdhash 3b5f3690c53eb229995f0a4fcf179ce24c6b8857 flags 0x22020201 | AFTER identical
- inplace-write (r+b, write B over A's inode):  AFTER A cdhash 3b5f3690... (unchanged) flags 0x22020201, process kept running A
- inplace-trunc (wb, write B):                   AFTER A cdhash 3b5f3690... (unchanged), no kill
- rename replace:                                AFTER A cdhash 3b5f3690... (unchanged)
=> the kernel record is the exec-time image identity and survives in-place rewrite; it was NOT invalidated or the process killed. Also confirmed the file-side derivation (sha256(CodeDirectory)[:20]) reproduces the kernel value for this test binary, the candidate core (d875d237...), the installed core ~/bin/praxis (read-only, aef5a54c...), and the plugin (458acdd7...): all cgo/linker-signed layouts verify (codeLimit == signature offset, 4 KiB pages).

## 3. Files (all under internal/bootstrapv4)
process_image.go (read() replaces digest(); single read feeds digest+binding; no caching), activation.go (VerifyProcessImage doc rewritten, false ETXTBSY claim removed; verifyProcessImage calls bindExecutingCode), NEW codesign_macho.go (pure parse/verify, all platforms), NEW process_image_darwin.go (csops), NEW process_image_other.go (!darwin, documented no-op), tests: codesign_macho_test.go (synthetic Mach-O; every-byte-flip, foreign-code-with-genuine-directory, fat, no signature, scatter, codeLimit off-by-one, malformed, truncation), process_image_darwin_test.go (real kernel identity vs own file), process_image_replacement_test.go (Astra reproductions promoted: in-place truncate+write and same-length overwrite x manifest B/A; unlink+recreate, atomic rename, identical bytes new inode, symlink retarget; controls: direct, hardlink, symlink x2; exec->init window via Go build overlay, rename and in-place, plus control), testdata/imageprobe/main.go (prints `RAN <marker>` after the verdict to prove which code is still executing). Existing process_image_test.go untouched.

## 4. go.mod
No change (syscall only; x/sys stays indirect).

## 5. Mutation evidence (each restored byte-identical, hashes diffed)
M1 bindExecutingCode -> nil: in-place (B) x2 and exec->init x2 FAIL. M2 page-hash comparison disabled: parser regressions FAIL (every-byte-flip, malformed). M3 kernel cdhash match disabled: parser, DarwinKernel, in-place, exec->init FAIL. M4 bind call removed from verifyProcessImage: in-place x2, exec->init x2 FAIL.
Not mutation-testable here: the CS_VALID flag check (kernel does not clear it in these scenarios; observed 0x22020201 throughout) and the csops-error path for an unsealed executable (arm64 kernel refuses to run unsigned binaries).

## 6. Residual limits
Non-darwin claim is the OS's, unproven and untested; Windows unqualified. Signature-container bytes after the code limit are outside executing-code identity. Depends on Apple csops CS_OPS_CDHASH semantics and on the executable being signed (Go linker signs arm64 by default; amd64 needs `codesign -s -`). syscall.Syscall6 on darwin is a deprecated-but-present libc syscall(2) path; a future macOS or Go that removes it makes verification fail closed, not open. `go test ./internal/bootstrapv4` (10s), -race, vet, cross-vet linux/windows/darwin-amd64 all pass. No commit made. No edits outside internal/bootstrapv4.
