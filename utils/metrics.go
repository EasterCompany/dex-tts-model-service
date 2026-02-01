package utils

import (
	sharedUtils "github.com/EasterCompany/dex-go-utils/utils"
)

// GetMetrics returns current CPU and Memory usage metrics for the current process
// and any optional additional PIDs (like child processes).
func GetMetrics(pids ...int) SystemMetrics {
	return sharedUtils.GetMetrics(pids...)
}
