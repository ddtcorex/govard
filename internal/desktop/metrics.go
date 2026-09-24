package desktop

import (
	"math"
	"os/user"

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
