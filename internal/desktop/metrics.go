package desktop

import (
	"fmt"
	"log"
	"math"
	"os/user"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemService methods

func (s *SystemService) GetUserInfo() (res UserInfo, err error) {
	defer RecoverPanic(&err, "GetUserInfo")
	res = UserInfo{
		Username: "unknown",
		Name:     "Unknown User",
	}
	u, errCurrent := user.Current()
	if errCurrent != nil {
		return res, errCurrent
	}
	res.Username = u.Username
	res.Name = u.Name
	if res.Name == "" {
		res.Name = u.Username
	}
	return res, nil
}

func (s *SystemService) GetVersion() (v string, err error) {
	defer RecoverPanic(&err, "GetVersion")
	return Version, nil
}

func (s *SystemService) GetSystemMetrics() SystemMetrics {
	cpuUsage, memUsage := getSystemMetrics()
	return SystemMetrics{
		CPUUsage:    cpuUsage,
		MemoryUsage: memUsage,
	}
}

func (s *SystemService) GetResourceMetrics() (res string, err error) {
	defer RecoverPanic(&err, "GetResourceMetrics")
	// Assuming GetResourceMetrics existed, if not returning empty string
	return "{}", nil
}

func (s *SystemService) Quit() {
	s.platform.Quit()
}

// frontendErrorMaxLength caps a reported island crash: the client caps too,
// in its own units, so this is the second layer, not the only one.
const frontendErrorMaxLength = 2000

// ReportFrontendError records a render failure the desktop ErrorBoundary
// caught: an island crash is otherwise visible only in the user's devtools,
// which support cannot see. The message is trimmed and capped, never fatal.
func (s *SystemService) ReportFrontendError(message string) (res string, err error) {
	defer RecoverPanic(&err, "ReportFrontendError")
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "", fmt.Errorf("message is required")
	}
	// Cut by rune, never by byte: a byte cut can split a multi-byte rune
	// and write invalid UTF-8 to the log.
	runes := []rune(trimmed)
	if len(runes) > frontendErrorMaxLength {
		runes = runes[:frontendErrorMaxLength]
	}
	// %q keeps a multi-line component stack on one log line: a raw %s would
	// let a crafted message forge sibling log lines.
	log.Printf("frontend island error: %q", string(runes))
	return "Frontend error reported", nil
}

func getSystemMetrics() (float64, float64) {
	var systemCPU float64
	var systemMemory float64

	if percents, err := cpu.Percent(0, false); err == nil && len(percents) > 0 {
		systemCPU = roundMetric(percents[0])
	}

	if v, err := mem.VirtualMemory(); err == nil {
		systemMemory = roundMetric(bytesToMB(v.Used))
	}

	return systemCPU, systemMemory
}

func bytesToMB(bytes uint64) float64 {
	return float64(bytes) / (1024 * 1024)
}

func roundMetric(value float64) float64 {
	return math.Round(value*10) / 10
}
