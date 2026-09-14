package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/plugin"
)

type ResolvedPlugin struct {
	Manifest   plugin.Manifest
	Definition RegisteredContent
	Executable RegisteredContent
}

// ResolvePlugin reconstructs a package plugin only when its versioned
// definition and separately typed executable payload remain active, co-owned,
// and digest-bound after restart. It does not launch the plugin or grant a
// capability lease.
func (s *Store) ResolvePlugin(ctx context.Context, id, version string) (ResolvedPlugin, error) {
	definition, err := s.ResolveContent(ctx, packagecatalog.ContentPlugin, id, version)
	if err != nil {
		return ResolvedPlugin{}, err
	}
	manifest, err := decodePackagePluginManifest(definition.ArtifactBytes)
	if err != nil {
		return ResolvedPlugin{}, err
	}
	if manifest.ID != definition.Content.ID || manifest.Version != definition.Content.Version {
		return ResolvedPlugin{}, errors.New("plugin definition identity does not match package content")
	}
	executable, err := s.ResolveContent(ctx, packagecatalog.ContentPluginExecutable, manifest.ExecutableContentID, manifest.ExecutableContentVersion)
	if err != nil {
		return ResolvedPlugin{}, fmt.Errorf("resolve plugin executable content: %w", err)
	}
	if executable.PackageID != definition.PackageID || executable.PackageVersion != definition.PackageVersion || executable.PackageDigest != definition.PackageDigest {
		return ResolvedPlugin{}, errors.New("plugin definition and executable are not from the same package generation")
	}
	if manifest.ArtifactDigest != executable.Content.Digest || manifest.Entrypoint != executable.Content.Artifact {
		return ResolvedPlugin{}, errors.New("plugin definition does not bind exact executable digest and entrypoint")
	}
	return ResolvedPlugin{Manifest: manifest, Definition: definition, Executable: executable}, nil
}

func decodePackagePluginManifest(body []byte) (plugin.Manifest, error) {
	var manifest plugin.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return plugin.Manifest{}, fmt.Errorf("decode package plugin definition: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return plugin.Manifest{}, fmt.Errorf("validate package plugin definition: %w", err)
	}
	return manifest, nil
}

// validateVerifiedPluginContents rejects malformed or orphaned executable
// plugin material before package activation publishes any registry state.
func validateVerifiedPluginContents(pkg packagecatalog.VerifiedPackage) error {
	manifest := pkg.Manifest()
	referencedExecutables := map[string]bool{}
	for _, definition := range manifest.ContentsOf(packagecatalog.ContentPlugin) {
		body, err := pkg.ContentBytes(definition)
		if err != nil {
			return err
		}
		pluginManifest, err := decodePackagePluginManifest(body)
		if err != nil {
			return err
		}
		if pluginManifest.ID != definition.ID || pluginManifest.Version != definition.Version {
			return errors.New("plugin definition identity does not match package content")
		}
		executable, ok := manifest.Content(packagecatalog.ContentPluginExecutable, pluginManifest.ExecutableContentID, pluginManifest.ExecutableContentVersion)
		if !ok || executable.Digest != pluginManifest.ArtifactDigest || executable.Artifact != pluginManifest.Entrypoint {
			return errors.New("plugin definition does not bind a typed executable content item")
		}
		if _, err := pkg.ContentBytes(executable); err != nil {
			return err
		}
		key := executable.ID + "\x00" + executable.Version
		if referencedExecutables[key] {
			return errors.New("plugin executable content is referenced by multiple definitions")
		}
		referencedExecutables[key] = true
	}
	for _, executable := range manifest.ContentsOf(packagecatalog.ContentPluginExecutable) {
		if !referencedExecutables[executable.ID+"\x00"+executable.Version] {
			return errors.New("package contains orphaned executable plugin content")
		}
	}
	return nil
}
