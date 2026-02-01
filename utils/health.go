package utils

import (
	sharedUtils "github.com/EasterCompany/dex-go-utils/utils"
)

// GetHealth returns the current health status of the service.
func GetHealth() Health {
	return sharedUtils.GetHealth()
}

// SetHealthStatus updates the health status of the service.
func SetHealthStatus(status string, message string) {
	sharedUtils.SetHealthStatus(status, message)
}
