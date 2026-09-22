package bootstrapv4

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Manifest is written only after the candidate binary and goals package have
// been installed through their respective human-controlled activation paths.
// A build/qualification manifest is deliberately not an active manifest.
type Manifest struct {
	Version                      string `json:"version"`
	KernelVersion                string `json:"kernel_version"`
	SourceCommit                 string `json:"source_commit"`
	SourceTreeDigest             string `json:"source_tree_digest"`
	BuildModified                bool   `json:"build_modified"`
	ActiveBinaryPath             string `json:"active_binary_path"`
	ActiveBinaryDigest           string `json:"active_binary_digest"`
	GoalsPackageID               string `json:"goals_package_id"`
	GoalsPackageVersion          string `json:"goals_package_version"`
	GoalsPackageContentDigest    string `json:"goals_package_content_digest"`
	GoalsPackageExecutableDigest string `json:"goals_package_executable_digest"`
	GoalsPackageContractDigest   string `json:"goals_package_contract_digest"`
	ValidationProfileDigest      string `json:"validation_profile_digest"`
	SpecificationBundleDigest    string `json:"specification_bundle_digest"`
	QualificationEvidenceDigest  string `json:"qualification_evidence_digest"`
}

type ActivePackageIdentity struct {
	ID, Version, ContentDigest, ExecutableDigest, ContractDigest string
}

func (m Manifest) Validate() error {
	if m.Version != "1" || m.KernelVersion != contracts.WorkPlanSafetyKernelVersion || m.SourceCommit == "" || m.ActiveBinaryPath == "" || m.GoalsPackageID == "" || m.GoalsPackageVersion == "" {
		return errors.New("activation manifest identity is incomplete")
	}
	for name, digest := range map[string]string{
		"active binary":          m.ActiveBinaryDigest,
		"package content":        m.GoalsPackageContentDigest,
		"package executable":     m.GoalsPackageExecutableDigest,
		"package contract":       m.GoalsPackageContractDigest,
		"validation profile":     m.ValidationProfileDigest,
		"specification bundle":   m.SpecificationBundleDigest,
		"qualification evidence": m.QualificationEvidenceDigest,
		"source tree":            m.SourceTreeDigest,
	} {
		if err := contracts.ValidateSHA256Digest(digest); err != nil {
			return fmt.Errorf("%s digest: %w", name, err)
		}
	}
	return nil
}

// VerifyFile binds a safety-bearing plan to the exact manifest bytes and the
// process image actually executing the check. Missing, drifted, copied, or
// source-only manifests fail closed.
func VerifyFile(path string, binding contracts.WorkPlanSafetyBinding, activePackage ActivePackageIdentity) error {
	manifest, err := VerifyManifestFile(path, binding, activePackage)
	if err != nil {
		return err
	}
	return verifyProcessImage(manifest)
}

// VerifyManifestFile performs the manifest-bytes, plan-binding, and active
// package predicates of VerifyFile without inspecting the executing process
// image. It exists so the two halves can be qualified independently; it is
// never a sufficient activation proof by itself.
func VerifyManifestFile(path string, binding contracts.WorkPlanSafetyBinding, activePackage ActivePackageIdentity) (Manifest, error) {
	if err := binding.Validate(); err != nil {
		return Manifest{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read bootstrap activation manifest: %w", err)
	}
	sum := sha256.Sum256(raw)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != binding.ActivationManifestDigest {
		return Manifest{}, fmt.Errorf("activation manifest digest mismatch: got %s want %s", got, binding.ActivationManifestDigest)
	}
	var manifest Manifest
	if err := contracts.UnmarshalExactJSON(raw, &manifest, true); err != nil {
		return Manifest{}, fmt.Errorf("decode activation manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	if manifest.ValidationProfileDigest != binding.ValidationProfileDigest || manifest.SpecificationBundleDigest != binding.SpecificationBundleDigest {
		return Manifest{}, errors.New("activation manifest does not bind the plan validation/specification identities")
	}
	if activePackage.ID == "" || activePackage.ID != manifest.GoalsPackageID || activePackage.Version != manifest.GoalsPackageVersion || activePackage.ContentDigest != manifest.GoalsPackageContentDigest || activePackage.ExecutableDigest != manifest.GoalsPackageExecutableDigest || activePackage.ContractDigest != manifest.GoalsPackageContractDigest {
		return Manifest{}, errors.New("active goals package identity does not match activation manifest")
	}
	return manifest, nil
}

// VerifyProcessImage binds activation to the executable image this process
// actually loaded, not to whatever bytes currently sit at its pathname.
//
// What is proven, per platform (see imageBindings for the same statement in
// code):
//
//   - Everywhere: the bytes read through the descriptor opened at package
//     initialization hash to the manifest digest, and the manifest's path still
//     resolves to that same file object. A pathname replacement while the
//     process is alive therefore fails closed in both directions (a manifest for
//     the new bytes mismatches the running file, and a manifest for the running
//     bytes no longer names the file at its path), so the supported protocol is
//     "replace, then restart".
//
//   - darwin: additionally, the kernel's exec-time code-directory hash for this
//     process (csops CS_OPS_CDHASH) must equal the hash of a CodeDirectory
//     embedded in those same bytes, and every code page of those bytes must match
//     that CodeDirectory. This binds the hashed file to the code this process
//     executes even when the file was overwritten in place through the same
//     inode (macOS does not refuse writes to a running executable, unlike
//     Linux's ETXTBSY) and even when the file was replaced between exec and
//     package initialization, because the kernel's record is taken at exec. An
//     executable with no embedded signature, a universal binary, or a process the
//     kernel reports as invalid fails closed. Bytes after the signed code limit
//     (the signature container itself) are not part of the executing code and
//     are covered only by the manifest digest.
//
//   - other platforms: no kernel code identity is consulted. The claim there
//     rests on the operating system refusing writes to executing images
//     (Linux: ETXTBSY), which this package does not prove; the image is
//     captured at package initialization, so a replacement between exec and that
//     instant is not observable. Windows is not qualified.
//
// VCS revision and modified state are compared but do not distinguish two
// builds; the digest and, on darwin, the code-directory binding do.
func VerifyProcessImage(manifest Manifest) error {
	return verifyProcessImage(manifest)
}

func verifyProcessImage(manifest Manifest) error {
	image, err := processImage()
	if err != nil {
		return err
	}
	expectedPath, err := filepath.EvalSymlinks(manifest.ActiveBinaryPath)
	if err != nil {
		return fmt.Errorf("resolve manifest executable symlinks: %w", err)
	}
	data, err := image.read()
	if err != nil {
		return err
	}
	if got := sha256Digest(data); got != manifest.ActiveBinaryDigest {
		return fmt.Errorf("running executable image digest mismatch: got %s want %s", got, manifest.ActiveBinaryDigest)
	}
	if err := bindExecutingCode(data); err != nil {
		return err
	}
	pathInfo, err := os.Stat(expectedPath)
	if err != nil {
		return fmt.Errorf("stat manifest executable: %w", err)
	}
	if !os.SameFile(image.info, pathInfo) {
		return fmt.Errorf("running executable image is no longer the file at %s; it was replaced while this process was running and must be restarted", expectedPath)
	}
	revisionFound, modifiedFound := false, false
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				revisionFound = true
				if setting.Value != manifest.SourceCommit {
					return fmt.Errorf("active binary source commit mismatch: got %s want %s", setting.Value, manifest.SourceCommit)
				}
			}
			if setting.Key == "vcs.modified" {
				modifiedFound = true
				modified, parseErr := strconv.ParseBool(setting.Value)
				if parseErr != nil || modified != manifest.BuildModified {
					return fmt.Errorf("active binary modified-build identity mismatch: got %s want %t", setting.Value, manifest.BuildModified)
				}
			}
		}
	}
	if !revisionFound || !modifiedFound {
		return errors.New("active binary lacks required VCS build identity")
	}
	return nil
}
