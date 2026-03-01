package desktop

import (
	"math"
	"os/user"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemService methods

func (s *SystemService) GetUserInfo() UserInfo {
	u, err := user.Current()
	if err != nil {
		return UserInfo{Username: "unknown", Name: "Unknown User"}
	}
	name := u.Name
	if name == "" {
		name = u.Username
	}
	return UserInfo{
		Username: u.Username,
		Name:     name,
	}
}

func (s *SystemService) GetSystemMetrics() SystemMetrics {
	cpuUsage, memUsage := getSystemMetrics()
	return SystemMetrics{
		CPUUsage:    cpuUsage,
		MemoryUsage: memUsage,
	}
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
