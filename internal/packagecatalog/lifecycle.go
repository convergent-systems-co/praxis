package packagecatalog

import (
	"fmt"
	"sort"
)

type InstallState string

const (
	PackageDiscovered      InstallState = "discovered"
	PackageDownloaded      InstallState = "downloaded"
	PackageVerified        InstallState = "verified"
	PackageInspected       InstallState = "inspected"
	PackageAuthorized      InstallState = "authorized"
	PackageInstalled       InstallState = "installed"
	PackageActive          InstallState = "active"
	PackageDisabled        InstallState = "disabled"
	PackageUpdating        InstallState = "updating"
	PackageRolledBack      InstallState = "rolled_back"
	PackageLocallyModified InstallState = "locally_modified"
	PackageRemoved         InstallState = "removed"
)

func CanTransitionInstall(from, to InstallState) bool {
	allowed := map[InstallState]map[InstallState]bool{
		PackageDiscovered:      {PackageDownloaded: true, PackageRemoved: true},
		PackageDownloaded:      {PackageVerified: true, PackageRemoved: true},
		PackageVerified:        {PackageInspected: true, PackageRemoved: true},
		PackageInspected:       {PackageAuthorized: true, PackageRemoved: true},
		PackageAuthorized:      {PackageInstalled: true, PackageRemoved: true},
		PackageInstalled:       {PackageActive: true, PackageDisabled: true, PackageUpdating: true, PackageRemoved: true},
		PackageActive:          {PackageDisabled: true, PackageUpdating: true, PackageLocallyModified: true, PackageRemoved: true},
		PackageDisabled:        {PackageActive: true, PackageUpdating: true, PackageRemoved: true},
		PackageUpdating:        {PackageInstalled: true, PackageRolledBack: true, PackageDisabled: true},
		PackageRolledBack:      {PackageInstalled: true, PackageActive: true, PackageDisabled: true},
		PackageLocallyModified: {PackageActive: true, PackageDisabled: true, PackageUpdating: true, PackageRemoved: true},
	}
	return allowed[from][to]
}

type UpdateReview struct {
	AddedCapabilities         []string
	RemovedCapabilities       []string
	EnforcementChanged       bool
	CryptoProfileChanged     bool
	RequiresReauthorization  bool
}

func ReviewUpdate(oldCaps, newCaps []string, enforcementChanged, cryptoChanged bool) UpdateReview {
	oldSet := toSet(oldCaps)
	newSet := toSet(newCaps)
	review := UpdateReview{EnforcementChanged: enforcementChanged, CryptoProfileChanged: cryptoChanged}
	for c := range newSet {
		if _, ok := oldSet[c]; !ok {
			review.AddedCapabilities = append(review.AddedCapabilities, c)
		}
	}
	for c := range oldSet {
		if _, ok := newSet[c]; !ok {
			review.RemovedCapabilities = append(review.RemovedCapabilities, c)
		}
	}
	sort.Strings(review.AddedCapabilities)
	sort.Strings(review.RemovedCapabilities)
	review.RequiresReauthorization = len(review.AddedCapabilities) > 0 || enforcementChanged || cryptoChanged
	return review
}

func ValidateInstallTransition(from, to InstallState) error {
	if !CanTransitionInstall(from, to) {
		return fmt.Errorf("invalid package transition %q -> %q", from, to)
	}
	return nil
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}
