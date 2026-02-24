package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type projectMetricAggregate struct {
	project        string
	status         string
	cpuPercent     float64
	memoryBytes    uint64
	memoryLimit    uint64
	netRxBytes     uint64
	netTxBytes     uint64
	oomKilled      bool
	runningSamples int
}

func buildResourceMetrics() (ResourceMetricsSnapshot, error) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return ResourceMetricsSnapshot{}, err
	}

	containers, err := cli.ContainerList(ctx, containertypes.ListOptions{All: true})
	if err != nil {
		return ResourceMetricsSnapshot{}, err
	}

	projectMap := map[string]*projectInfo{}
	for _, c := range containers {
		projectName, serviceName := extractProjectAndService(c)
		if projectName == "" {
			continue
		}
		info := projectMap[projectName]
		if info == nil {
			info = &projectInfo{
				name:     projectName,
				services: map[string]bool{},
			}
			projectMap[projectName] = info
		}
		if strings.TrimSpace(serviceName) != "" {
			info.services[serviceName] = true
		}
		if c.State == "running" {
			info.runningCount++
		}
		if info.workingDir == "" {
			if wd := c.Labels["com.docker.compose.project.working_dir"]; wd != "" {
				info.workingDir = wd
			}
		}
		if len(info.configFiles) == 0 {
			if files := c.Labels["com.docker.compose.project.config_files"]; files != "" {
				info.configFiles = parseConfigFiles(files)
			}
		}
	}

	aggregates := map[string]*projectMetricAggregate{}
	for projectName, info := range projectMap {
		_ = loadProjectConfig(info)
		if !looksLikeGovard(info) {
			continue
		}
		status := "stopped"
		if info.runningCount > 0 {
			status = "running"
		}
		aggregates[projectName] = &projectMetricAggregate{
			project: projectName,
			status:  status,
		}
	}

	warnings := []string{}
	for _, c := range containers {
		projectName, _ := extractProjectAndService(c)
		agg := aggregates[projectName]
		if agg == nil || c.State != "running" {
			continue
		}

		stats, statsErr := readContainerStatsOneShot(ctx, cli, c.ID)
		if statsErr != nil {
			warnings = append(warnings, fmt.Sprintf("metrics unavailable for %s: %v", projectName, statsErr))
			continue
		}

		agg.cpuPercent += calculateCPUPercent(stats)
		agg.memoryBytes += stats.MemoryStats.Usage
		if stats.MemoryStats.Limit > 0 {
			agg.memoryLimit += stats.MemoryStats.Limit
		}
		rxBytes, txBytes := sumNetworkBytes(stats.Networks)
		agg.netRxBytes += rxBytes
		agg.netTxBytes += txBytes
		agg.runningSamples++

		oomKilled, inspectErr := readContainerOOMKilled(ctx, cli, c.ID)
		if inspectErr == nil && oomKilled {
			agg.oomKilled = true
		}
	}

	projectNames := make([]string, 0, len(aggregates))
	for projectName := range aggregates {
		projectNames = append(projectNames, projectName)
	}
	sort.Strings(projectNames)

	projects := make([]ProjectResourceMetric, 0, len(projectNames))
	summary := ResourceMetricsSummary{}
	for _, projectName := range projectNames {
		agg := aggregates[projectName]
		memoryPercent := 0.0
		if agg.memoryLimit > 0 {
			memoryPercent = (float64(agg.memoryBytes) / float64(agg.memoryLimit)) * 100
		}

		projectMetric := ProjectResourceMetric{
			Project:       agg.project,
			Status:        agg.status,
			CPUPercent:    roundMetric(agg.cpuPercent),
			MemoryMB:      roundMetric(bytesToMB(agg.memoryBytes)),
			MemoryPercent: roundMetric(memoryPercent),
			NetRxMB:       roundMetric(bytesToMB(agg.netRxBytes)),
			NetTxMB:       roundMetric(bytesToMB(agg.netTxBytes)),
			OOMKilled:     agg.oomKilled,
		}
		projects = append(projects, projectMetric)

		if agg.status == "running" {
			summary.ActiveProjects++
		}
		summary.CPUPercent += agg.cpuPercent
		summary.MemoryMB += bytesToMB(agg.memoryBytes)
		summary.NetRxMB += bytesToMB(agg.netRxBytes)
		summary.NetTxMB += bytesToMB(agg.netTxBytes)
		if agg.oomKilled {
			summary.OOMProjects++
		}
	}

	summary.CPUPercent = roundMetric(summary.CPUPercent)
	summary.MemoryMB = roundMetric(summary.MemoryMB)
	summary.NetRxMB = roundMetric(summary.NetRxMB)
	summary.NetTxMB = roundMetric(summary.NetTxMB)

	systemCPU, systemMemory := getSystemMetrics()

	return ResourceMetricsSnapshot{
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		SystemCPU:    systemCPU,
		SystemMemory: systemMemory,
		Summary:      summary,
		Projects:     projects,
		Warnings:     buildMetricsWarnings(projects, warnings),
	}, nil
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

func readContainerStatsOneShot(ctx context.Context, cli *client.Client, containerID string) (containertypes.StatsResponse, error) {
	reader, err := cli.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return containertypes.StatsResponse{}, err
	}
	defer reader.Body.Close()

	var stats containertypes.StatsResponse
	if err := json.NewDecoder(reader.Body).Decode(&stats); err != nil {
		return containertypes.StatsResponse{}, err
	}
	return stats, nil
}

func readContainerOOMKilled(ctx context.Context, cli *client.Client, containerID string) (bool, error) {
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, err
	}
	if inspect.State == nil {
		return false, nil
	}
	return inspect.State.OOMKilled, nil
}

func calculateCPUPercent(stats containertypes.StatsResponse) float64 {
	return calculateCPUPercentFromDeltas(
		stats.CPUStats.CPUUsage.TotalUsage,
		stats.PreCPUStats.CPUUsage.TotalUsage,
		stats.CPUStats.SystemUsage,
		stats.PreCPUStats.SystemUsage,
		stats.CPUStats.OnlineCPUs,
		len(stats.CPUStats.CPUUsage.PercpuUsage),
	)
}

func calculateCPUPercentFromDeltas(
	currentUsage uint64,
	previousUsage uint64,
	currentSystem uint64,
	previousSystem uint64,
	onlineCPUs uint32,
	perCPUCount int,
) float64 {
	if currentUsage <= previousUsage || currentSystem <= previousSystem {
		return 0
	}

	cpuDelta := float64(currentUsage - previousUsage)
	systemDelta := float64(currentSystem - previousSystem)
	if systemDelta <= 0 {
		return 0
	}

	cpuCount := float64(onlineCPUs)
	if cpuCount <= 0 {
		if perCPUCount > 0 {
			cpuCount = float64(perCPUCount)
		} else {
			cpuCount = 1
		}
	}

	return (cpuDelta / systemDelta) * cpuCount * 100
}

func sumNetworkBytes(networks map[string]containertypes.NetworkStats) (uint64, uint64) {
	var rxBytes uint64
	var txBytes uint64
	for _, stats := range networks {
		rxBytes += stats.RxBytes
		txBytes += stats.TxBytes
	}
	return rxBytes, txBytes
}

func bytesToMB(bytes uint64) float64 {
	return float64(bytes) / (1024 * 1024)
}

func roundMetric(value float64) float64 {
	return math.Round(value*10) / 10
}

func buildMetricsWarnings(projects []ProjectResourceMetric, input []string) []string {
	var oomProjects []string
	for _, project := range projects {
		if project.OOMKilled {
			oomProjects = append(oomProjects, project.Project)
		}
	}
	sort.Strings(oomProjects)

	warnings := append([]string{}, input...)
	if len(oomProjects) > 0 {
		warnings = append(warnings, "OOM kill detected in: "+strings.Join(oomProjects, ", "))
	}
	return uniqueStrings(warnings)
}
