package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

// ResolveGraph reconstructs an executable graph only from the active,
// digest-bound package artifact. Package registration alone is insufficient.
func (s *Store) ResolveGraph(ctx context.Context, id, version string) (kernel.GraphDef, RegisteredContent, error) {
	content, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, id, version)
	if err != nil {
		return kernel.GraphDef{}, RegisteredContent{}, err
	}
	if digestPackageBytes(content.ArtifactBytes) != content.Content.Digest {
		return kernel.GraphDef{}, RegisteredContent{}, errors.New("persisted graph artifact digest mismatch")
	}
	var graph kernel.GraphDef
	if err := json.Unmarshal(content.ArtifactBytes, &graph); err != nil {
		return kernel.GraphDef{}, RegisteredContent{}, fmt.Errorf("decode package graph artifact: %w", err)
	}
	if graph.ID != content.Content.ID || graph.Version != content.Content.Version {
		return kernel.GraphDef{}, RegisteredContent{}, errors.New("package graph artifact identity does not match content reference")
	}
	if err := graph.Validate(); err != nil {
		return kernel.GraphDef{}, RegisteredContent{}, fmt.Errorf("validate package graph artifact: %w", err)
	}
	return graph, content, nil
}
