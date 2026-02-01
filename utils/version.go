package utils

import (
	sharedUtils "github.com/EasterCompany/dex-go-utils/utils"
)

// SetVersion sets the version information for the service.
func SetVersion(version, branch, commit, buildDate, arch string) {
	sharedUtils.SetVersion(version, branch, commit, buildDate, arch)
}

// GetVersion returns the version information for the service.
func GetVersion() Version {
	return sharedUtils.GetVersion()
}
