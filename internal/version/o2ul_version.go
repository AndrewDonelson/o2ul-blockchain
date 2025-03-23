// file: /internal/version/o2ul_version.go
// description: O²UL custom version integration
// module: Core
// License: MIT
// Author: Andrew Donelson
// Copyright 2025 Andrew Donelson
// Portions Copyright 2014-2024 The go-ethereum Authors
// SPDX-License-Identifier: UNLICENSED

package version

import (
	"fmt"
	"time"
)

// Custom version constants for O²UL
const (
	// Phase defines the current development phase
	// Valid values: "prod", "alpha", "beta"
	O2ULPhase = "alpha"
)

// This init() function will run when the version package is imported
// and will override the default version string with our custom one
func init() {
	// Override the version.WithMeta string with our custom format
	WithMeta = CustomVersionWithCommit(gitCommit, gitDate)
}

// CustomVersionWithCommit returns the O²UL version with git commit and date
func CustomVersionWithCommit(commit, date string) string {
	now := time.Now().UTC()
	version := FormatVersion(now, O2ULPhase, commit)
	return version
}

// FormatVersion formats a version string using our custom format
func FormatVersion(timestamp time.Time, phase string, gitCommit string) string {
	// Format the version according to our scheme
	versionString := fmt.Sprintf("v%04d.%02d.%02d.%02d%02d-%s",
		timestamp.Year(),
		timestamp.Month(),
		timestamp.Day(),
		timestamp.Hour(),
		timestamp.Minute(),
		phase)

	// Add git commit if provided
	if len(gitCommit) >= 8 {
		shortCommit := gitCommit[:8]
		return fmt.Sprintf("%s [%s]", versionString, shortCommit)
	}

	return versionString
}
