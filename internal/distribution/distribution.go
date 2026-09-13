package distribution

import (
	"context"
	"errors"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

type PackageRef struct {
	Source string `json:"source"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
}

func (r PackageRef) Validate() error {
	if r.Source == "" || r.Owner == "" || r.Repo == "" {
		return errors.New("package source, owner, and repo are required")
	}
	return nil
}

func (r PackageRef) String() string { return r.Owner + "/" + r.Repo }

type Candidate struct {
	Ref         PackageRef `json:"ref"`
	Description string     `json:"description,omitempty"`
	WebURL      string     `json:"web_url,omitempty"`
}

type Release struct {
	Ref            PackageRef                       `json:"ref"`
	Tag            string                           `json:"tag"`
	WebURL         string                           `json:"web_url,omitempty"`
	ManifestURL    string                           `json:"manifest_url"`
	ArtifactURL    string                           `json:"artifact_url"`
	SignatureURL   string                           `json:"signature_url"`
	ManifestDigest string                           `json:"manifest_digest"`
	Manifest       packagecatalog.Manifest          `json:"manifest"`
	ManifestBytes  []byte                           `json:"-"`
	Signature      packagecatalog.SignatureEnvelope `json:"signature"`
}

type Adapter interface {
	Discover(ctx context.Context, query string) ([]Candidate, error)
	Info(ctx context.Context, ref PackageRef, version string) (Release, error)
	Resolve(ctx context.Context, ref PackageRef, version string) (Release, error)
	FetchArtifact(ctx context.Context, release Release) ([]byte, error)
	CheckUpdate(ctx context.Context, ref PackageRef, installedVersion string) (Release, bool, error)
}
