package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Client — talks to the Docker Engine API over a Unix socket
// ---------------------------------------------------------------------------

const (
	defaultSocket = "/var/run/docker.sock"
	apiVersion    = "v1.44"
	apiBase       = "http://localhost/" + apiVersion
)

// Client wraps the Docker Engine HTTP API via Unix socket.
type Client struct {
	socketPath string
	httpClient *http.Client
}

// New returns a Client connected to the default Docker socket (/var/run/docker.sock).
func New() *Client {
	return NewWithSocket(defaultSocket)
}

// NewWithSocket returns a Client connected to the given Unix socket path.
func NewWithSocket(path string) *Client {
	return &Client{
		socketPath: path,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.DialTimeout("unix", path, 5*time.Second)
				},
			},
			Timeout: 120 * time.Second,
		},
	}
}

// ---------------------------------------------------------------------------
// Low-level HTTP helpers
// ---------------------------------------------------------------------------

func (c *Client) apiURL(path string) string {
	return apiBase + path
}

func (c *Client) doGet(path string) (*http.Response, error) {
	return c.httpClient.Get(c.apiURL(path))
}

func (c *Client) doPost(path string, body interface{}) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, c.apiURL(path), reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

func (c *Client) doPostRaw(path, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, c.apiURL(path), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.httpClient.Do(req)
}

func (c *Client) doDelete(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, c.apiURL(path), nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

// decodeJSON reads and JSON-decodes the response body, returning an error for
// non-2xx status codes.
func decodeJSON(resp *http.Response, dst interface{}) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if dst == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// drainOK consumes and closes the response body, returning an error for
// non-2xx status codes. Used for endpoints that return no meaningful body.
func drainOK(resp *http.Response) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// ---------------------------------------------------------------------------
// Model types
// ---------------------------------------------------------------------------

// DockerInfo represents the response from GET /info.
type DockerInfo struct {
	ID                string `json:"ID"`
	Containers        int    `json:"Containers"`
	ContainersRunning int    `json:"ContainersRunning"`
	ContainersPaused  int    `json:"ContainersPaused"`
	ContainersStopped int    `json:"ContainersStopped"`
	Images            int    `json:"Images"`
	Driver            string `json:"Driver"`
	MemTotal          int64  `json:"MemTotal"`
	NCPU              int    `json:"NCPU"`
	ServerVersion     string `json:"ServerVersion"`
	OperatingSystem   string `json:"OperatingSystem"`
	OSType            string `json:"OSType"`
	Architecture      string `json:"Architecture"`
	KernelVersion     string `json:"KernelVersion"`
	DockerRootDir     string `json:"DockerRootDir"`
}

// DockerVersion represents the response from GET /version.
type DockerVersion struct {
	Version    string `json:"Version"`
	APIVersion string `json:"ApiVersion"`
	GoVersion  string `json:"GoVersion"`
	Os         string `json:"Os"`
	Arch       string `json:"Arch"`
	BuildTime  string `json:"BuildTime"`
	GitCommit  string `json:"GitCommit"`
}

// DiskUsage represents the response from GET /system/df.
type DiskUsage struct {
	LayersSize int64                `json:"LayersSize"`
	Images     []DiskUsageImage     `json:"Images"`
	Containers []DiskUsageContainer `json:"Containers"`
	Volumes    []DiskUsageVolume    `json:"Volumes"`
	BuildCache []DiskUsageBuild     `json:"BuildCache"`
}

// DiskUsageImage is an image entry in the disk usage report.
type DiskUsageImage struct {
	ID         string   `json:"Id"`
	RepoTags   []string `json:"RepoTags"`
	Size       int64    `json:"Size"`
	SharedSize int64    `json:"SharedSize"`
	Containers int      `json:"Containers"`
	Created    int64    `json:"Created"`
}

// DiskUsageContainer is a container entry in the disk usage report.
type DiskUsageContainer struct {
	ID         string   `json:"Id"`
	Names      []string `json:"Names"`
	Image      string   `json:"Image"`
	SizeRw     int64    `json:"SizeRw"`
	SizeRootFs int64    `json:"SizeRootFs"`
	State      string   `json:"State"`
	Status     string   `json:"Status"`
	Created    int64    `json:"Created"`
}

// DiskUsageVolume is a volume entry in the disk usage report.
type DiskUsageVolume struct {
	Name      string `json:"Name"`
	UsageData struct {
		Size     int64 `json:"Size"`
		RefCount int   `json:"RefCount"`
	} `json:"UsageData"`
}

// DiskUsageBuild is a build cache entry in the disk usage report.
type DiskUsageBuild struct {
	ID          string `json:"ID"`
	Size        int64  `json:"Size"`
	InUse       bool   `json:"InUse"`
	Shared      bool   `json:"Shared"`
	Description string `json:"Description"`
}

// Container represents a container returned by GET /containers/json.
type Container struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	Command string            `json:"Command"`
	Created int64             `json:"Created"`
	Status  string            `json:"Status"`
	State   string            `json:"State"`
	Ports   []ContainerPort   `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
	Mounts  []MountPoint      `json:"Mounts"`
}

// ContainerPort represents a port mapping on a container.
type ContainerPort struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

// MountPoint represents a mount on a container.
type MountPoint struct {
	Type        string `json:"Type"`
	Name        string `json:"Name,omitempty"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Driver      string `json:"Driver,omitempty"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
}

// ContainerDetail represents the full inspect response for a container.
type ContainerDetail struct {
	ID              string          `json:"Id"`
	Name            string          `json:"Name"`
	Image           string          `json:"Image"`
	Created         string          `json:"Created"`
	State           ContainerState  `json:"State"`
	Config          ContainerCfg    `json:"Config"`
	HostConfig      HostConfig      `json:"HostConfig"`
	NetworkSettings NetworkSettings `json:"NetworkSettings"`
	Mounts          []MountPoint    `json:"Mounts"`
	Platform        string          `json:"Platform"`
	RestartCount    int             `json:"RestartCount"`
	Args            []string        `json:"Args"`
	Path            string          `json:"Path"`
}

// ContainerState holds the runtime state of a container.
type ContainerState struct {
	Status     string `json:"Status"`
	Running    bool   `json:"Running"`
	Paused     bool   `json:"Paused"`
	Restarting bool   `json:"Restarting"`
	OOMKilled  bool   `json:"OOMKilled"`
	Dead       bool   `json:"Dead"`
	Pid        int    `json:"Pid"`
	ExitCode   int    `json:"ExitCode"`
	Error      string `json:"Error"`
	StartedAt  string `json:"StartedAt"`
	FinishedAt string `json:"FinishedAt"`
}

// ContainerCfg is the container's configuration from docker inspect.
type ContainerCfg struct {
	Hostname     string              `json:"Hostname"`
	Domainname   string              `json:"Domainname"`
	User         string              `json:"User"`
	Env          []string            `json:"Env"`
	Cmd          []string            `json:"Cmd"`
	Image        string              `json:"Image"`
	WorkingDir   string              `json:"WorkingDir"`
	Entrypoint   []string            `json:"Entrypoint"`
	Labels       map[string]string   `json:"Labels"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	Volumes      map[string]struct{} `json:"Volumes"`
}

// NetworkSettings holds a container's network configuration.
type NetworkSettings struct {
	IPAddress string                       `json:"IPAddress"`
	Gateway   string                       `json:"Gateway"`
	Ports     map[string][]PortBinding     `json:"Ports"`
	Networks  map[string]*EndpointSettings `json:"Networks"`
}

// EndpointSettings describes a container's attachment to a single network.
type EndpointSettings struct {
	IPAddress   string `json:"IPAddress"`
	Gateway     string `json:"Gateway"`
	MacAddress  string `json:"MacAddress"`
	NetworkID   string `json:"NetworkID"`
	EndpointID  string `json:"EndpointID"`
}

// ContainerConfig is the request body for creating a container.
type ContainerConfig struct {
	Image            string                `json:"Image"`
	Cmd              []string              `json:"Cmd,omitempty"`
	Entrypoint       []string              `json:"Entrypoint,omitempty"`
	Env              []string              `json:"Env,omitempty"`
	ExposedPorts     map[string]struct{}   `json:"ExposedPorts,omitempty"`
	Labels           map[string]string     `json:"Labels,omitempty"`
	Volumes          map[string]struct{}   `json:"Volumes,omitempty"`
	WorkingDir       string                `json:"WorkingDir,omitempty"`
	Hostname         string                `json:"Hostname,omitempty"`
	User             string                `json:"User,omitempty"`
	HostConfig       *HostConfig           `json:"HostConfig,omitempty"`
	NetworkingConfig *NetworkingConfig     `json:"NetworkingConfig,omitempty"`
}

// HostConfig holds host-specific container configuration.
type HostConfig struct {
	PortBindings  map[string][]PortBinding `json:"PortBindings,omitempty"`
	Binds         []string                 `json:"Binds,omitempty"`
	RestartPolicy RestartPolicy            `json:"RestartPolicy,omitempty"`
	Memory        int64                    `json:"Memory,omitempty"`
	NanoCPUs      int64                    `json:"NanoCpus,omitempty"`
	NetworkMode   string                   `json:"NetworkMode,omitempty"`
	LogConfig     *LogConfig               `json:"LogConfig,omitempty"`
	CapAdd        []string                 `json:"CapAdd,omitempty"`
	CapDrop       []string                 `json:"CapDrop,omitempty"`
	DNS           []string                 `json:"Dns,omitempty"`
	ExtraHosts    []string                 `json:"ExtraHosts,omitempty"`
}

// PortBinding represents a host IP/port binding.
type PortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// RestartPolicy describes the container restart policy.
type RestartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

// LogConfig configures the logging driver for a container.
type LogConfig struct {
	Type   string            `json:"Type"`
	Config map[string]string `json:"Config"`
}

// NetworkingConfig holds networking configuration for container creation.
type NetworkingConfig struct {
	EndpointsConfig map[string]*EndpointConfig `json:"EndpointsConfig,omitempty"`
}

// EndpointConfig describes endpoint configuration for a network.
type EndpointConfig struct {
	IPAMConfig *IPAMCfg `json:"IPAMConfig,omitempty"`
	Aliases    []string `json:"Aliases,omitempty"`
}

// IPAMCfg holds IPAM-specific endpoint settings.
type IPAMCfg struct {
	IPv4Address string `json:"IPv4Address,omitempty"`
	IPv6Address string `json:"IPv6Address,omitempty"`
}

// CreateResponse is returned by the container create endpoint.
type CreateResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

// ContainerStats holds parsed CPU/memory/IO stats for a container.
type ContainerStats struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage  int64   `json:"memory_usage"`
	MemoryLimit  int64   `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	NetInput     int64   `json:"net_input"`
	NetOutput    int64   `json:"net_output"`
	BlockRead    int64   `json:"block_read"`
	BlockWrite   int64   `json:"block_write"`
	PIDs         int     `json:"pids"`
}

// Image represents an image from GET /images/json.
type Image struct {
	ID          string            `json:"Id"`
	ParentID    string            `json:"ParentId"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Size        int64             `json:"Size"`
	VirtualSize int64             `json:"VirtualSize"`
	Created     int64             `json:"Created"`
	Labels      map[string]string `json:"Labels"`
}

// ImageDetail represents the full inspect response for an image.
type ImageDetail struct {
	ID            string       `json:"Id"`
	RepoTags      []string     `json:"RepoTags"`
	RepoDigests   []string     `json:"RepoDigests"`
	Size          int64        `json:"Size"`
	VirtualSize   int64        `json:"VirtualSize"`
	Created       string       `json:"Created"`
	Architecture  string       `json:"Architecture"`
	Os            string       `json:"Os"`
	Author        string       `json:"Author"`
	DockerVersion string       `json:"DockerVersion"`
	Config        ContainerCfg `json:"Config"`
}

// Volume represents a Docker volume.
type Volume struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
	CreatedAt  string            `json:"CreatedAt"`
	Options    map[string]string `json:"Options"`
}

// Network represents a Docker network.
type Network struct {
	ID         string                      `json:"Id"`
	Name       string                      `json:"Name"`
	Driver     string                      `json:"Driver"`
	Scope      string                      `json:"Scope"`
	Internal   bool                        `json:"Internal"`
	IPAM       IPAM                        `json:"IPAM"`
	Containers map[string]NetworkContainer `json:"Containers"`
	Options    map[string]string           `json:"Options"`
	Labels     map[string]string           `json:"Labels"`
	Created    string                      `json:"Created"`
}

// IPAM holds IP address management configuration for a network.
type IPAM struct {
	Driver string     `json:"Driver"`
	Config []IPAMPool `json:"Config"`
}

// IPAMPool holds a subnet/gateway pair.
type IPAMPool struct {
	Subnet  string `json:"Subnet"`
	Gateway string `json:"Gateway"`
}

// NetworkContainer describes a container attached to a network.
type NetworkContainer struct {
	Name        string `json:"Name"`
	EndpointID  string `json:"EndpointID"`
	MacAddress  string `json:"MacAddress"`
	IPv4Address string `json:"IPv4Address"`
	IPv6Address string `json:"IPv6Address"`
}

// ComposeService represents a service from docker compose ps.
type ComposeService struct {
	Name    string `json:"Name"`
	Command string `json:"Command"`
	State   string `json:"State"`
	Status  string `json:"Status"`
	Ports   string `json:"Ports"`
	Service string `json:"Service"`
	Image   string `json:"Image"`
}

// ---------------------------------------------------------------------------
// System
// ---------------------------------------------------------------------------

// Ping checks whether the Docker daemon is accessible.
func (c *Client) Ping() error {
	resp, err := c.httpClient.Get(apiBase + "/_ping")
	if err != nil {
		return fmt.Errorf("docker ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker ping: status %d", resp.StatusCode)
	}
	return nil
}

// Info returns system-wide Docker information.
func (c *Client) Info() (*DockerInfo, error) {
	resp, err := c.doGet("/info")
	if err != nil {
		return nil, fmt.Errorf("docker info: %w", err)
	}
	var info DockerInfo
	if err := decodeJSON(resp, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// Version returns the Docker engine version.
func (c *Client) Version() (*DockerVersion, error) {
	resp, err := c.doGet("/version")
	if err != nil {
		return nil, fmt.Errorf("docker version: %w", err)
	}
	var ver DockerVersion
	if err := decodeJSON(resp, &ver); err != nil {
		return nil, err
	}
	return &ver, nil
}

// DiskUsage returns disk usage information for images, containers, volumes, and
// build cache.
func (c *Client) DiskUsage() (*DiskUsage, error) {
	resp, err := c.doGet("/system/df")
	if err != nil {
		return nil, fmt.Errorf("docker disk usage: %w", err)
	}
	var du DiskUsage
	if err := decodeJSON(resp, &du); err != nil {
		return nil, err
	}
	return &du, nil
}

// ---------------------------------------------------------------------------
// Containers
// ---------------------------------------------------------------------------

// ListContainers returns containers. If all is true, stopped containers are
// included.
func (c *Client) ListContainers(all bool) ([]Container, error) {
	path := "/containers/json"
	if all {
		path += "?all=true"
	}
	resp, err := c.doGet(path)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	var containers []Container
	if err := decodeJSON(resp, &containers); err != nil {
		return nil, err
	}
	if containers == nil {
		return []Container{}, nil
	}
	return containers, nil
}

// GetContainer returns detailed information about a single container.
func (c *Client) GetContainer(id string) (*ContainerDetail, error) {
	resp, err := c.doGet("/containers/" + url.PathEscape(id) + "/json")
	if err != nil {
		return nil, fmt.Errorf("get container %s: %w", id, err)
	}
	var detail ContainerDetail
	if err := decodeJSON(resp, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// CreateContainer creates a new container with the given name and config.
// The name may be empty for an auto-generated name.
func (c *Client) CreateContainer(name string, cfg ContainerConfig) (*CreateResponse, error) {
	path := "/containers/create"
	if name != "" {
		path += "?name=" + url.QueryEscape(name)
	}
	resp, err := c.doPost(path, cfg)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	var cr CreateResponse
	if err := decodeJSON(resp, &cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

// StartContainer starts a stopped container.
func (c *Client) StartContainer(id string) error {
	resp, err := c.doPost("/containers/"+url.PathEscape(id)+"/start", nil)
	if err != nil {
		return fmt.Errorf("start container %s: %w", id, err)
	}
	// 204 = started, 304 = already started — both are fine.
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return nil
	}
	return drainOK(resp)
}

// StopContainer stops a running container with the given timeout (seconds).
func (c *Client) StopContainer(id string, timeout int) error {
	path := fmt.Sprintf("/containers/%s/stop?t=%d", url.PathEscape(id), timeout)
	resp, err := c.doPost(path, nil)
	if err != nil {
		return fmt.Errorf("stop container %s: %w", id, err)
	}
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return nil
	}
	return drainOK(resp)
}

// RestartContainer restarts a container with the given timeout (seconds).
func (c *Client) RestartContainer(id string, timeout int) error {
	path := fmt.Sprintf("/containers/%s/restart?t=%d", url.PathEscape(id), timeout)
	resp, err := c.doPost(path, nil)
	if err != nil {
		return fmt.Errorf("restart container %s: %w", id, err)
	}
	if resp.StatusCode == http.StatusNoContent {
		resp.Body.Close()
		return nil
	}
	return drainOK(resp)
}

// RemoveContainer removes a container. If force is true, the container is
// killed before removal.
func (c *Client) RemoveContainer(id string, force bool) error {
	path := fmt.Sprintf("/containers/%s?force=%v", url.PathEscape(id), force)
	resp, err := c.doDelete(path)
	if err != nil {
		return fmt.Errorf("remove container %s: %w", id, err)
	}
	if resp.StatusCode == http.StatusNoContent {
		resp.Body.Close()
		return nil
	}
	return drainOK(resp)
}

// ContainerLogs returns the last n lines of stdout+stderr from a container.
// Docker uses a multiplexed stream format with 8-byte headers per frame.
func (c *Client) ContainerLogs(id string, lines int) (string, error) {
	path := fmt.Sprintf("/containers/%s/logs?stdout=1&stderr=1&tail=%d",
		url.PathEscape(id), lines)
	resp, err := c.doGet(path)
	if err != nil {
		return "", fmt.Errorf("container logs %s: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("docker API %d: %s", resp.StatusCode, string(body))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return stripDockerLogHeaders(raw), nil
}

// stripDockerLogHeaders removes the 8-byte multiplexed stream headers that
// Docker prepends to each log frame. Each header contains:
//
//	[0]    stream type (0=stdin, 1=stdout, 2=stderr)
//	[1-3]  reserved
//	[4-7]  frame size (big-endian uint32)
func stripDockerLogHeaders(data []byte) string {
	var out strings.Builder
	i := 0
	for i < len(data) {
		if i+8 > len(data) {
			out.Write(data[i:])
			break
		}
		size := int(data[i+4])<<24 | int(data[i+5])<<16 | int(data[i+6])<<8 | int(data[i+7])
		i += 8
		end := i + size
		if end > len(data) {
			end = len(data)
		}
		out.Write(data[i:end])
		i = end
	}
	return out.String()
}

// ContainerStats returns a single stats snapshot for a container (non-streaming).
func (c *Client) ContainerStats(id string) (*ContainerStats, error) {
	resp, err := c.doGet("/containers/" + url.PathEscape(id) + "/stats?stream=false")
	if err != nil {
		return nil, fmt.Errorf("container stats %s: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docker API %d: %s", resp.StatusCode, string(body))
	}

	// Parse the raw stats JSON to extract CPU/memory/network/block IO.
	var raw struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage int64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage int64 `json:"system_cpu_usage"`
			OnlineCPUs     int   `json:"online_cpus"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage int64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage int64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage int64 `json:"usage"`
			Limit int64 `json:"limit"`
			Stats struct {
				Cache int64 `json:"cache"`
			} `json:"stats"`
		} `json:"memory_stats"`
		Networks map[string]struct {
			RxBytes int64 `json:"rx_bytes"`
			TxBytes int64 `json:"tx_bytes"`
		} `json:"networks"`
		BlkioStats struct {
			IoServiceBytesRecursive []struct {
				Op    string `json:"op"`
				Value int64  `json:"value"`
			} `json:"io_service_bytes_recursive"`
		} `json:"blkio_stats"`
		PidsStats struct {
			Current int `json:"current"`
		} `json:"pids_stats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}

	// CPU percentage: delta(container) / delta(system) * numCPUs * 100.
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage - raw.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(raw.CPUStats.SystemCPUUsage - raw.PreCPUStats.SystemCPUUsage)
	cpuPercent := 0.0
	if sysDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / sysDelta) * float64(raw.CPUStats.OnlineCPUs) * 100.0
	}

	// Memory: usage minus cache (active working set).
	memUsage := raw.MemoryStats.Usage - raw.MemoryStats.Stats.Cache
	if memUsage < 0 {
		memUsage = raw.MemoryStats.Usage
	}
	memLimit := raw.MemoryStats.Limit
	memPercent := 0.0
	if memLimit > 0 {
		memPercent = float64(memUsage) / float64(memLimit) * 100.0
	}

	// Aggregate network I/O across all interfaces.
	var netIn, netOut int64
	for _, iface := range raw.Networks {
		netIn += iface.RxBytes
		netOut += iface.TxBytes
	}

	// Aggregate block I/O.
	var blockRead, blockWrite int64
	for _, entry := range raw.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(entry.Op) {
		case "read":
			blockRead += entry.Value
		case "write":
			blockWrite += entry.Value
		}
	}

	return &ContainerStats{
		CPUPercent:    cpuPercent,
		MemoryUsage:  memUsage,
		MemoryLimit:  memLimit,
		MemoryPercent: memPercent,
		NetInput:     netIn,
		NetOutput:    netOut,
		BlockRead:    blockRead,
		BlockWrite:   blockWrite,
		PIDs:         raw.PidsStats.Current,
	}, nil
}

// ExecInContainer creates an exec instance inside a container, starts it, and
// returns the combined stdout+stderr output.
func (c *Client) ExecInContainer(id string, cmd []string) (string, error) {
	// Step 1: Create exec instance.
	execBody := map[string]interface{}{
		"AttachStdout": true,
		"AttachStderr": true,
		"Cmd":          cmd,
	}
	resp, err := c.doPost("/containers/"+url.PathEscape(id)+"/exec", execBody)
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}
	var execResp struct {
		ID string `json:"Id"`
	}
	if err := decodeJSON(resp, &execResp); err != nil {
		return "", err
	}

	// Step 2: Start exec instance.
	startBody := map[string]interface{}{
		"Detach": false,
		"Tty":    false,
	}
	resp2, err := c.doPost("/exec/"+execResp.ID+"/start", startBody)
	if err != nil {
		return "", fmt.Errorf("exec start: %w", err)
	}
	defer resp2.Body.Close()
	raw, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", err
	}
	return stripDockerLogHeaders(raw), nil
}

// ---------------------------------------------------------------------------
// Images
// ---------------------------------------------------------------------------

// ListImages returns all images on the host.
func (c *Client) ListImages() ([]Image, error) {
	resp, err := c.doGet("/images/json")
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	var images []Image
	if err := decodeJSON(resp, &images); err != nil {
		return nil, err
	}
	if images == nil {
		return []Image{}, nil
	}
	return images, nil
}

// PullImage pulls an image by reference (e.g. "nginx:latest"). It consumes the
// entire streaming response to completion. Returns an error if the pull fails.
func (c *Client) PullImage(ref string) error {
	image := ref
	tag := "latest"
	if idx := strings.LastIndex(ref, ":"); idx > 0 && !strings.Contains(ref[idx:], "/") {
		image = ref[:idx]
		tag = ref[idx+1:]
	}

	path := fmt.Sprintf("/images/create?fromImage=%s&tag=%s",
		url.QueryEscape(image), url.QueryEscape(tag))
	resp, err := c.doPost(path, nil)
	if err != nil {
		return fmt.Errorf("pull image %s: %w", ref, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull image %s: %d: %s", ref, resp.StatusCode, string(body))
	}

	// The response is a stream of JSON objects. Consume them to complete the
	// pull, and check for errors in the stream.
	dec := json.NewDecoder(resp.Body)
	for {
		var msg map[string]interface{}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("pull stream: %w", err)
		}
		if errMsg, ok := msg["error"]; ok {
			return fmt.Errorf("pull image %s: %v", ref, errMsg)
		}
	}
	return nil
}

// RemoveImage removes an image by ID or tag. If force is true, the image is
// force-removed even if containers reference it.
func (c *Client) RemoveImage(id string, force bool) error {
	path := fmt.Sprintf("/images/%s?force=%v", url.PathEscape(id), force)
	resp, err := c.doDelete(path)
	if err != nil {
		return fmt.Errorf("remove image %s: %w", id, err)
	}
	return drainOK(resp)
}

// InspectImage returns detailed information about an image.
func (c *Client) InspectImage(id string) (*ImageDetail, error) {
	resp, err := c.doGet("/images/" + url.PathEscape(id) + "/json")
	if err != nil {
		return nil, fmt.Errorf("inspect image %s: %w", id, err)
	}
	var detail ImageDetail
	if err := decodeJSON(resp, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// BuildImage builds a Docker image from a tar context. The tag parameter is
// the image name:tag (e.g. "myapp:latest").
func (c *Client) BuildImage(contextTar io.Reader, tag string) error {
	path := "/build?t=" + url.QueryEscape(tag)
	resp, err := c.doPostRaw(path, "application/x-tar", contextTar)
	if err != nil {
		return fmt.Errorf("build image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("build image: %d: %s", resp.StatusCode, string(body))
	}
	// Consume the streaming build output, checking for errors.
	dec := json.NewDecoder(resp.Body)
	for {
		var msg map[string]interface{}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("build stream: %w", err)
		}
		if errMsg, ok := msg["error"]; ok {
			return fmt.Errorf("build error: %v", errMsg)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Volumes
// ---------------------------------------------------------------------------

// ListVolumes returns all volumes on the host.
func (c *Client) ListVolumes() ([]Volume, error) {
	resp, err := c.doGet("/volumes")
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	var wrapper struct {
		Volumes []Volume `json:"Volumes"`
	}
	if err := decodeJSON(resp, &wrapper); err != nil {
		return nil, err
	}
	if wrapper.Volumes == nil {
		return []Volume{}, nil
	}
	return wrapper.Volumes, nil
}

// CreateVolume creates a named volume with optional labels.
func (c *Client) CreateVolume(name string, labels map[string]string) (*Volume, error) {
	body := map[string]interface{}{
		"Name":   name,
		"Labels": labels,
	}
	resp, err := c.doPost("/volumes/create", body)
	if err != nil {
		return nil, fmt.Errorf("create volume: %w", err)
	}
	var vol Volume
	if err := decodeJSON(resp, &vol); err != nil {
		return nil, err
	}
	return &vol, nil
}

// RemoveVolume removes a volume by name. If force is true, the volume is
// removed even if in use.
func (c *Client) RemoveVolume(name string, force bool) error {
	path := fmt.Sprintf("/volumes/%s?force=%v", url.PathEscape(name), force)
	resp, err := c.doDelete(path)
	if err != nil {
		return fmt.Errorf("remove volume %s: %w", name, err)
	}
	if resp.StatusCode == http.StatusNoContent {
		resp.Body.Close()
		return nil
	}
	return drainOK(resp)
}

// ---------------------------------------------------------------------------
// Networks
// ---------------------------------------------------------------------------

// ListNetworks returns all networks on the host.
func (c *Client) ListNetworks() ([]Network, error) {
	resp, err := c.doGet("/networks")
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	var networks []Network
	if err := decodeJSON(resp, &networks); err != nil {
		return nil, err
	}
	if networks == nil {
		return []Network{}, nil
	}
	return networks, nil
}

// CreateNetwork creates a new Docker network with the given name and driver.
// If driver is empty, "bridge" is used.
func (c *Client) CreateNetwork(name, driver string) (*Network, error) {
	if driver == "" {
		driver = "bridge"
	}
	body := map[string]interface{}{
		"Name":   name,
		"Driver": driver,
	}
	resp, err := c.doPost("/networks/create", body)
	if err != nil {
		return nil, fmt.Errorf("create network: %w", err)
	}
	var result struct {
		ID      string `json:"Id"`
		Warning string `json:"Warning"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &Network{
		ID:     result.ID,
		Name:   name,
		Driver: driver,
	}, nil
}

// RemoveNetwork removes a network by ID or name.
func (c *Client) RemoveNetwork(id string) error {
	resp, err := c.doDelete("/networks/" + url.PathEscape(id))
	if err != nil {
		return fmt.Errorf("remove network %s: %w", id, err)
	}
	if resp.StatusCode == http.StatusNoContent {
		resp.Body.Close()
		return nil
	}
	return drainOK(resp)
}

// ConnectContainer connects a container to a network.
func (c *Client) ConnectContainer(networkID, containerID string) error {
	body := map[string]interface{}{
		"Container": containerID,
	}
	resp, err := c.doPost("/networks/"+url.PathEscape(networkID)+"/connect", body)
	if err != nil {
		return fmt.Errorf("connect container: %w", err)
	}
	return drainOK(resp)
}

// DisconnectContainer disconnects a container from a network.
func (c *Client) DisconnectContainer(networkID, containerID string) error {
	body := map[string]interface{}{
		"Container": containerID,
		"Force":     true,
	}
	resp, err := c.doPost("/networks/"+url.PathEscape(networkID)+"/disconnect", body)
	if err != nil {
		return fmt.Errorf("disconnect container: %w", err)
	}
	return drainOK(resp)
}

// ---------------------------------------------------------------------------
// Compose (via docker CLI — no direct API exists)
// ---------------------------------------------------------------------------

// runCompose executes a docker compose command and returns combined output.
func runCompose(projectDir string, args ...string) (string, error) {
	cmdArgs := append([]string{"compose"}, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ComposeUp runs `docker compose up -d` in the given project directory.
func (c *Client) ComposeUp(projectDir string, projectName string) error {
	args := []string{}
	if projectName != "" {
		args = append(args, "-p", projectName)
	}
	args = append(args, "up", "-d")
	out, err := runCompose(projectDir, args...)
	if err != nil {
		return fmt.Errorf("compose up: %s: %w", out, err)
	}
	return nil
}

// ComposeDown runs `docker compose down` in the given project directory.
func (c *Client) ComposeDown(projectDir string, projectName string) error {
	args := []string{}
	if projectName != "" {
		args = append(args, "-p", projectName)
	}
	args = append(args, "down")
	out, err := runCompose(projectDir, args...)
	if err != nil {
		return fmt.Errorf("compose down: %s: %w", out, err)
	}
	return nil
}

// ComposePS runs `docker compose ps --format json` and returns parsed services.
func (c *Client) ComposePS(projectDir string) ([]ComposeService, error) {
	out, err := runCompose(projectDir, "ps", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("compose ps: %s: %w", out, err)
	}

	var services []ComposeService
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Some docker compose versions emit one JSON object per line.
		var svc ComposeService
		if err := json.Unmarshal([]byte(line), &svc); err != nil {
			// Try as array (other versions emit a JSON array).
			var arr []ComposeService
			if err2 := json.Unmarshal([]byte(line), &arr); err2 == nil {
				services = append(services, arr...)
			}
			continue
		}
		services = append(services, svc)
	}
	return services, nil
}

// ComposeLogs runs `docker compose logs --tail=N` and returns the output.
func (c *Client) ComposeLogs(projectDir string, lines int) (string, error) {
	out, err := runCompose(projectDir, "logs", "--tail="+strconv.Itoa(lines))
	if err != nil {
		return out, fmt.Errorf("compose logs: %w", err)
	}
	return out, nil
}

// ComposePull runs `docker compose pull` in the given project directory.
func (c *Client) ComposePull(projectDir string) error {
	out, err := runCompose(projectDir, "pull")
	if err != nil {
		return fmt.Errorf("compose pull: %s: %w", out, err)
	}
	return nil
}
