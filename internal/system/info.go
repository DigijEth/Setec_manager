package system

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ── Types ───────────────────────────────────────────────────────────

type CoreUsage struct {
	Core    int     `json:"core"`
	User    float64 `json:"user"`
	System  float64 `json:"system"`
	Idle    float64 `json:"idle"`
	IOWait  float64 `json:"iowait"`
	Percent float64 `json:"percent"`
}

type CPUInfo struct {
	Overall float64     `json:"overall"`
	Idle    float64     `json:"idle"`
	Cores   []CoreUsage `json:"cores"`
}

type MemInfo struct {
	TotalBytes     uint64 `json:"total_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	BuffersBytes   uint64 `json:"buffers_bytes"`
	CachedBytes    uint64 `json:"cached_bytes"`
	SwapTotalBytes uint64 `json:"swap_total_bytes"`
	SwapUsedBytes  uint64 `json:"swap_used_bytes"`
	SwapFreeBytes  uint64 `json:"swap_free_bytes"`
	Total          string `json:"total"`
	Used           string `json:"used"`
	Free           string `json:"free"`
	Available      string `json:"available"`
	Buffers        string `json:"buffers"`
	Cached         string `json:"cached"`
	SwapTotal      string `json:"swap_total"`
	SwapUsed       string `json:"swap_used"`
	SwapFree       string `json:"swap_free"`
}

type DiskInfo struct {
	Filesystem string `json:"filesystem"`
	Size       string `json:"size"`
	Used       string `json:"used"`
	Available  string `json:"available"`
	UsePercent string `json:"use_percent"`
	MountPoint string `json:"mount_point"`
}

type NetInfo struct {
	Interface  string `json:"interface"`
	RxBytes    uint64 `json:"rx_bytes"`
	RxPackets  uint64 `json:"rx_packets"`
	RxErrors   uint64 `json:"rx_errors"`
	RxDropped  uint64 `json:"rx_dropped"`
	TxBytes    uint64 `json:"tx_bytes"`
	TxPackets  uint64 `json:"tx_packets"`
	TxErrors   uint64 `json:"tx_errors"`
	TxDropped  uint64 `json:"tx_dropped"`
	RxHuman    string `json:"rx_human"`
	TxHuman    string `json:"tx_human"`
}

type UptimeInfo struct {
	Seconds      float64 `json:"seconds"`
	IdleSeconds  float64 `json:"idle_seconds"`
	HumanReadable string `json:"human_readable"`
}

type LoadInfo struct {
	Load1       float64 `json:"load_1"`
	Load5       float64 `json:"load_5"`
	Load15      float64 `json:"load_15"`
	RunningProcs int    `json:"running_procs"`
	TotalProcs   int    `json:"total_procs"`
}

type ProcessInfo struct {
	PID     int     `json:"pid"`
	User    string  `json:"user"`
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	RSS     int64   `json:"rss"`
	Command string  `json:"command"`
}

// ── CPU ─────────────────────────────────────────────────────────────

// readCPUStats reads /proc/stat and returns a map of cpu label to field slices.
func readCPUStats() (map[string][]uint64, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, fmt.Errorf("opening /proc/stat: %w", err)
	}
	defer f.Close()

	result := make(map[string][]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		label := fields[0]
		var vals []uint64
		for _, field := range fields[1:] {
			v, _ := strconv.ParseUint(field, 10, 64)
			vals = append(vals, v)
		}
		result[label] = vals
	}
	return result, scanner.Err()
}

// parseCPUFields converts raw jiffie counts into a CoreUsage.
// Fields: user, nice, system, idle, iowait, irq, softirq, steal, guest, guest_nice
func parseCPUFields(core int, before, after []uint64) CoreUsage {
	cu := CoreUsage{Core: core}
	if len(before) < 5 || len(after) < 5 {
		return cu
	}

	// Sum all fields for total jiffies
	var totalBefore, totalAfter uint64
	for _, v := range before {
		totalBefore += v
	}
	for _, v := range after {
		totalAfter += v
	}

	totalDelta := float64(totalAfter - totalBefore)
	if totalDelta == 0 {
		return cu
	}

	userDelta := float64((after[0] + after[1]) - (before[0] + before[1]))
	systemDelta := float64(after[2] - before[2])
	idleDelta := float64(after[3] - before[3])
	var iowaitDelta float64
	if len(after) > 4 && len(before) > 4 {
		iowaitDelta = float64(after[4] - before[4])
	}

	cu.User = userDelta / totalDelta * 100
	cu.System = systemDelta / totalDelta * 100
	cu.Idle = idleDelta / totalDelta * 100
	cu.IOWait = iowaitDelta / totalDelta * 100
	cu.Percent = 100 - cu.Idle

	return cu
}

// GetCPUUsage samples /proc/stat twice with a brief interval to compute usage.
func GetCPUUsage() (CPUInfo, error) {
	info := CPUInfo{}

	before, err := readCPUStats()
	if err != nil {
		return info, err
	}

	time.Sleep(250 * time.Millisecond)

	after, err := readCPUStats()
	if err != nil {
		return info, err
	}

	// Overall CPU (the "cpu" aggregate line)
	if bv, ok := before["cpu"]; ok {
		if av, ok := after["cpu"]; ok {
			overall := parseCPUFields(-1, bv, av)
			info.Overall = overall.Percent
			info.Idle = overall.Idle
		}
	}

	// Per-core
	for i := 0; ; i++ {
		label := fmt.Sprintf("cpu%d", i)
		bv, ok1 := before[label]
		av, ok2 := after[label]
		if !ok1 || !ok2 {
			break
		}
		info.Cores = append(info.Cores, parseCPUFields(i, bv, av))
	}

	return info, nil
}

// ── Memory ──────────────────────────────────────────────────────────

func GetMemory() (MemInfo, error) {
	info := MemInfo{}

	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return info, fmt.Errorf("opening /proc/meminfo: %w", err)
	}
	defer f.Close()

	vals := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		v, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		vals[key] = v * 1024 // convert kB to bytes
	}
	if err := scanner.Err(); err != nil {
		return info, err
	}

	info.TotalBytes = vals["MemTotal"]
	info.FreeBytes = vals["MemFree"]
	info.AvailableBytes = vals["MemAvailable"]
	info.BuffersBytes = vals["Buffers"]
	info.CachedBytes = vals["Cached"]
	info.SwapTotalBytes = vals["SwapTotal"]
	info.SwapFreeBytes = vals["SwapFree"]
	info.SwapUsedBytes = info.SwapTotalBytes - info.SwapFreeBytes
	info.UsedBytes = info.TotalBytes - info.FreeBytes - info.BuffersBytes - info.CachedBytes
	if info.UsedBytes > info.TotalBytes {
		// Overflow guard: if buffers+cached > total-free, use simpler calculation
		info.UsedBytes = info.TotalBytes - info.AvailableBytes
	}

	info.Total = humanBytes(info.TotalBytes)
	info.Used = humanBytes(info.UsedBytes)
	info.Free = humanBytes(info.FreeBytes)
	info.Available = humanBytes(info.AvailableBytes)
	info.Buffers = humanBytes(info.BuffersBytes)
	info.Cached = humanBytes(info.CachedBytes)
	info.SwapTotal = humanBytes(info.SwapTotalBytes)
	info.SwapUsed = humanBytes(info.SwapUsedBytes)
	info.SwapFree = humanBytes(info.SwapFreeBytes)

	return info, nil
}

// ── Disk ────────────────────────────────────────────────────────────

func GetDisk() ([]DiskInfo, error) {
	// Try with filesystem type filters first for real block devices
	out, err := exec.Command("df", "-h", "--type=ext4", "--type=xfs", "--type=btrfs", "--type=ext3").CombinedOutput()
	if err != nil {
		// Fallback: exclude pseudo filesystems
		out, err = exec.Command("df", "-h", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=squashfs").CombinedOutput()
		if err != nil {
			// Last resort: all filesystems
			out, err = exec.Command("df", "-h").CombinedOutput()
			if err != nil {
				return nil, fmt.Errorf("df command failed: %w (%s)", err, string(out))
			}
		}
	}

	var disks []DiskInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		disks = append(disks, DiskInfo{
			Filesystem: fields[0],
			Size:       fields[1],
			Used:       fields[2],
			Available:  fields[3],
			UsePercent: fields[4],
			MountPoint: fields[5],
		})
	}

	return disks, nil
}

// ── Network ─────────────────────────────────────────────────────────

func GetNetwork() ([]NetInfo, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, fmt.Errorf("opening /proc/net/dev: %w", err)
	}
	defer f.Close()

	var interfaces []NetInfo
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum <= 2 {
			continue // skip the two header lines
		}

		line := scanner.Text()
		// Format: "  iface: rx_bytes rx_packets rx_errs rx_drop ... tx_bytes tx_packets tx_errs tx_drop ..."
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}

		iface := strings.TrimSpace(line[:colonIdx])
		rest := strings.TrimSpace(line[colonIdx+1:])
		fields := strings.Fields(rest)
		if len(fields) < 10 {
			continue
		}

		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		rxPackets, _ := strconv.ParseUint(fields[1], 10, 64)
		rxErrors, _ := strconv.ParseUint(fields[2], 10, 64)
		rxDropped, _ := strconv.ParseUint(fields[3], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		txPackets, _ := strconv.ParseUint(fields[9], 10, 64)
		txErrors, _ := strconv.ParseUint(fields[10], 10, 64)
		txDropped, _ := strconv.ParseUint(fields[11], 10, 64)

		interfaces = append(interfaces, NetInfo{
			Interface: iface,
			RxBytes:   rxBytes,
			RxPackets: rxPackets,
			RxErrors:  rxErrors,
			RxDropped: rxDropped,
			TxBytes:   txBytes,
			TxPackets: txPackets,
			TxErrors:  txErrors,
			TxDropped: txDropped,
			RxHuman:   humanBytes(rxBytes),
			TxHuman:   humanBytes(txBytes),
		})
	}

	return interfaces, scanner.Err()
}

// ── Uptime ──────────────────────────────────────────────────────────

func GetUptime() (UptimeInfo, error) {
	info := UptimeInfo{}

	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return info, fmt.Errorf("reading /proc/uptime: %w", err)
	}

	fields := strings.Fields(strings.TrimSpace(string(data)))
	if len(fields) < 2 {
		return info, fmt.Errorf("unexpected /proc/uptime format")
	}

	info.Seconds, _ = strconv.ParseFloat(fields[0], 64)
	info.IdleSeconds, _ = strconv.ParseFloat(fields[1], 64)

	// Build human readable string
	totalSec := int(info.Seconds)
	days := totalSec / 86400
	hours := (totalSec % 86400) / 3600
	minutes := (totalSec % 3600) / 60
	seconds := totalSec % 60

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d day%s", days, plural(days)))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d hour%s", hours, plural(hours)))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d minute%s", minutes, plural(minutes)))
	}
	if len(parts) == 0 || (days == 0 && hours == 0 && minutes == 0) {
		parts = append(parts, fmt.Sprintf("%d second%s", seconds, plural(seconds)))
	}

	info.HumanReadable = strings.Join(parts, ", ")

	return info, nil
}

// ── Load Average ────────────────────────────────────────────────────

func GetLoadAvg() (LoadInfo, error) {
	info := LoadInfo{}

	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return info, fmt.Errorf("reading /proc/loadavg: %w", err)
	}

	fields := strings.Fields(strings.TrimSpace(string(data)))
	if len(fields) < 4 {
		return info, fmt.Errorf("unexpected /proc/loadavg format")
	}

	info.Load1, _ = strconv.ParseFloat(fields[0], 64)
	info.Load5, _ = strconv.ParseFloat(fields[1], 64)
	info.Load15, _ = strconv.ParseFloat(fields[2], 64)

	// fields[3] is "running/total" format
	procParts := strings.SplitN(fields[3], "/", 2)
	if len(procParts) == 2 {
		info.RunningProcs, _ = strconv.Atoi(procParts[0])
		info.TotalProcs, _ = strconv.Atoi(procParts[1])
	}

	return info, nil
}

// ── Top Processes ───────────────────────────────────────────────────

func GetTopProcesses(n int) ([]ProcessInfo, error) {
	if n <= 0 {
		n = 10
	}

	// ps aux --sort=-%mem gives us processes sorted by memory usage descending
	out, err := exec.Command("ps", "aux", "--sort=-%mem").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ps command failed: %w (%s)", err, string(out))
	}

	var procs []ProcessInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		if len(procs) >= n {
			break
		}

		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		pid, _ := strconv.Atoi(fields[1])
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		rss, _ := strconv.ParseInt(fields[5], 10, 64)
		// Command is everything from field 10 onward (may contain spaces)
		command := strings.Join(fields[10:], " ")

		procs = append(procs, ProcessInfo{
			PID:     pid,
			User:    fields[0],
			CPU:     cpu,
			Mem:     mem,
			RSS:     rss,
			Command: command,
		})
	}

	return procs, nil
}

// ── Helpers ─────────────────────────────────────────────────────────

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), suffixes[exp])
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
