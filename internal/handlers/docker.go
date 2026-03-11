package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"setec-manager/internal/docker"
)

// ── Helpers ─────────────────────────────────────────────────────────

func (h *Handler) dockerInstalled() bool {
	if h.DockerClient == nil {
		return false
	}
	return h.DockerClient.Ping() == nil
}

func (h *Handler) requireDocker(w http.ResponseWriter) bool {
	if !h.dockerInstalled() {
		writeError(w, http.StatusServiceUnavailable, "Docker is not installed or not running")
		return false
	}
	return true
}

// ── Setup / Status ──────────────────────────────────────────────────

// DockerSetup renders the Docker management page (install wizard if not installed, dashboard if installed).
func (h *Handler) DockerSetup(w http.ResponseWriter, r *http.Request) {
	installed := h.dockerInstalled()
	var info *docker.DockerInfo
	if installed {
		info, _ = h.DockerClient.Info()
	}

	sites, _ := h.DB.ListSites()
	var domains []string
	for _, s := range sites {
		domains = append(domains, s.Domain)
	}

	var deployments []docker.Deployment
	if h.DockerDeployer != nil {
		deployments, _ = h.DockerDeployer.ListDeployments()
	}

	data := map[string]interface{}{
		"Installed":   installed,
		"Info":        info,
		"Domains":     domains,
		"Deployments": deployments,
	}
	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, data)
		return
	}
	h.render(w, "docker.html", data)
}

// DockerInstall installs Docker CE on the system.
func (h *Handler) DockerInstall(w http.ResponseWriter, r *http.Request) {
	if h.dockerInstalled() {
		writeError(w, http.StatusConflict, "Docker is already installed")
		return
	}

	if runtime.GOOS != "linux" {
		writeError(w, http.StatusBadRequest, "Docker auto-install is only supported on Linux")
		return
	}

	// Download the official convenience script
	resp, err := http.Get("https://get.docker.com")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("download install script: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("download install script: HTTP %d", resp.StatusCode))
		return
	}

	scriptPath := "/tmp/get-docker.sh"
	f, err := os.OpenFile(scriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write install script: %v", err))
		return
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write install script: %v", err))
		return
	}
	f.Close()

	// Run the install script
	cmd := exec.Command("sh", scriptPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("install docker: %s", string(out)))
		return
	}

	// Enable and start Docker
	exec.Command("systemctl", "enable", "docker").Run()
	if err := exec.Command("systemctl", "start", "docker").Run(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("start docker: %v", err))
		return
	}

	// Install docker-compose plugin
	exec.Command("apt-get", "install", "-y", "docker-compose-plugin").Run()

	// Re-initialize clients
	h.DockerClient = docker.New()
	stateDir := filepath.Dir(h.Config.Database.Path)
	h.DockerDeployer = docker.NewDeployer(h.DockerClient, stateDir, h.Config.Nginx.SitesAvailable)

	os.Remove(scriptPath)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "installed",
		"message": "Docker CE installed successfully",
	})
}

// DockerStatus returns Docker daemon information as JSON.
func (h *Handler) DockerStatus(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	info, err := h.DockerClient.Info()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ver, _ := h.DockerClient.Version()
	du, _ := h.DockerClient.DiskUsage()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"info":       info,
		"version":    ver,
		"disk_usage": du,
	})
}

// DockerPrune cleans up unused Docker resources.
func (h *Handler) DockerPrune(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}

	results := map[string]string{}
	out, err := exec.Command("docker", "system", "prune", "-a", "-f", "--volumes").CombinedOutput()
	if err != nil {
		results["error"] = string(out)
	} else {
		results["output"] = string(out)
		results["status"] = "pruned"
	}
	writeJSON(w, http.StatusOK, results)
}

// ── Container Management ────────────────────────────────────────────

// DockerContainers lists all containers.
func (h *Handler) DockerContainers(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	containers, err := h.DockerClient.ListContainers(true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, containers)
}

// DockerContainerGet returns details for a single container.
func (h *Handler) DockerContainerGet(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	id := paramStr(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}
	detail, err := h.DockerClient.GetContainer(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// DockerContainerCreate creates a new container.
func (h *Handler) DockerContainerCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}

	var body struct {
		Name       string            `json:"name"`
		Image      string            `json:"image"`
		Ports      map[string]string `json:"ports"`
		Env        map[string]string `json:"env"`
		Volumes    map[string]string `json:"volumes"`
		Restart    string            `json:"restart"`
		Memory     int64             `json:"memory"`
		CPUs       float64           `json:"cpus"`
		Cmd        string            `json:"cmd"`
		Entrypoint string            `json:"entrypoint"`
		Network    string            `json:"network"`
		Labels     map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Image == "" {
		writeError(w, http.StatusBadRequest, "image is required")
		return
	}

	cfg := docker.ContainerConfig{
		Image:  body.Image,
		Labels: body.Labels,
	}

	if body.Cmd != "" {
		cfg.Cmd = strings.Fields(body.Cmd)
	}
	if body.Entrypoint != "" {
		cfg.Entrypoint = strings.Fields(body.Entrypoint)
	}

	for k, v := range body.Env {
		cfg.Env = append(cfg.Env, k+"="+v)
	}

	portBindings := make(map[string][]docker.PortBinding)
	exposedPorts := make(map[string]struct{})
	for hostPort, containerPort := range body.Ports {
		key := containerPort + "/tcp"
		exposedPorts[key] = struct{}{}
		portBindings[key] = []docker.PortBinding{{HostPort: hostPort}}
	}
	if len(exposedPorts) > 0 {
		cfg.ExposedPorts = exposedPorts
	}

	var binds []string
	for hostPath, containerPath := range body.Volumes {
		binds = append(binds, hostPath+":"+containerPath)
	}

	hc := &docker.HostConfig{
		PortBindings: portBindings,
		Binds:        binds,
	}
	if body.Restart != "" {
		hc.RestartPolicy = docker.RestartPolicy{Name: body.Restart}
	}
	if body.Memory > 0 {
		hc.Memory = body.Memory
	}
	if body.CPUs > 0 {
		hc.NanoCPUs = int64(body.CPUs * 1e9)
	}
	if body.Network != "" {
		hc.NetworkMode = body.Network
	}
	cfg.HostConfig = hc

	cr, err := h.DockerClient.CreateContainer(body.Name, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := h.DockerClient.StartContainer(cr.ID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("created but failed to start: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":     cr.ID,
		"status": "running",
	})
}

// DockerContainerStart starts a stopped container.
func (h *Handler) DockerContainerStart(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	id := paramStr(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}
	if err := h.DockerClient.StartContainer(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// DockerContainerStop stops a running container.
func (h *Handler) DockerContainerStop(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	id := paramStr(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}
	if err := h.DockerClient.StopContainer(id, 10); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// DockerContainerRestart restarts a container.
func (h *Handler) DockerContainerRestart(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	id := paramStr(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}
	if err := h.DockerClient.RestartContainer(id, 10); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

// DockerContainerRemove removes a container.
func (h *Handler) DockerContainerRemove(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	id := paramStr(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}
	if err := h.DockerClient.RemoveContainer(id, true); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// DockerContainerLogs returns container logs.
func (h *Handler) DockerContainerLogs(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	id := paramStr(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}
	logs, err := h.DockerClient.ContainerLogs(id, 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": logs})
}

// DockerContainerStats returns container resource usage.
func (h *Handler) DockerContainerStats(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	id := paramStr(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}
	stats, err := h.DockerClient.ContainerStats(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ── Image Management ────────────────────────────────────────────────

// DockerImages lists all images.
func (h *Handler) DockerImages(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	images, err := h.DockerClient.ListImages()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, images)
}

// DockerImagePull pulls an image from a registry.
func (h *Handler) DockerImagePull(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	var body struct {
		Image string `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Image == "" {
		writeError(w, http.StatusBadRequest, "image reference is required")
		return
	}
	if err := h.DockerClient.PullImage(body.Image); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "pulled",
		"image":  body.Image,
	})
}

// DockerImageRemove removes an image.
func (h *Handler) DockerImageRemove(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	id := paramStr(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "image id is required")
		return
	}
	if err := h.DockerClient.RemoveImage(id, false); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ── Volume Management ───────────────────────────────────────────────

// DockerVolumes lists all volumes.
func (h *Handler) DockerVolumes(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	volumes, err := h.DockerClient.ListVolumes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, volumes)
}

// DockerVolumeCreate creates a new volume.
func (h *Handler) DockerVolumeCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	var body struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	vol, err := h.DockerClient.CreateVolume(body.Name, body.Labels)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, vol)
}

// DockerVolumeRemove removes a volume.
func (h *Handler) DockerVolumeRemove(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	name := paramStr(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "volume name is required")
		return
	}
	if err := h.DockerClient.RemoveVolume(name, false); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ── Network Management ──────────────────────────────────────────────

// DockerNetworks lists all networks.
func (h *Handler) DockerNetworks(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	networks, err := h.DockerClient.ListNetworks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, networks)
}

// DockerNetworkCreate creates a new network.
func (h *Handler) DockerNetworkCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	var body struct {
		Name   string `json:"name"`
		Driver string `json:"driver"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	net, err := h.DockerClient.CreateNetwork(body.Name, body.Driver)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, net)
}

// DockerNetworkRemove removes a network.
func (h *Handler) DockerNetworkRemove(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	id := paramStr(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "network id is required")
		return
	}
	if err := h.DockerClient.RemoveNetwork(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ── One-Click Deploy ────────────────────────────────────────────────

// DockerDeploy creates a new one-click deployment with domain routing.
func (h *Handler) DockerDeploy(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	if h.DockerDeployer == nil {
		writeError(w, http.StatusServiceUnavailable, "deployer not initialized")
		return
	}

	var cfg docker.DeployConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if cfg.Name == "" || cfg.Image == "" {
		writeError(w, http.StatusBadRequest, "name and image are required")
		return
	}

	dep, err := h.DockerDeployer.Deploy(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, dep)
}

// DockerDeployments lists all managed deployments.
func (h *Handler) DockerDeployments(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	if h.DockerDeployer == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	deps, err := h.DockerDeployer.ListDeployments()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if deps == nil {
		deps = []docker.Deployment{}
	}
	writeJSON(w, http.StatusOK, deps)
}

// DockerDeploymentGet returns details for a single deployment.
func (h *Handler) DockerDeploymentGet(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	name := paramStr(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "deployment name is required")
		return
	}
	if h.DockerDeployer == nil {
		writeError(w, http.StatusServiceUnavailable, "deployer not initialized")
		return
	}
	dep, err := h.DockerDeployer.GetDeployment(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dep)
}

// DockerUndeploy removes a deployment and its container + nginx config.
func (h *Handler) DockerUndeploy(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	name := paramStr(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "deployment name is required")
		return
	}
	if h.DockerDeployer == nil {
		writeError(w, http.StatusServiceUnavailable, "deployer not initialized")
		return
	}
	if err := h.DockerDeployer.Undeploy(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "undeployed"})
}

// DockerDeploymentUpdate pulls a new image and recreates the container.
func (h *Handler) DockerDeploymentUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	name := paramStr(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "deployment name is required")
		return
	}
	if h.DockerDeployer == nil {
		writeError(w, http.StatusServiceUnavailable, "deployer not initialized")
		return
	}
	if err := h.DockerDeployer.UpdateDeployment(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DockerDeploymentScale scales the number of replicas for a deployment.
func (h *Handler) DockerDeploymentScale(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	name := paramStr(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "deployment name is required")
		return
	}
	if h.DockerDeployer == nil {
		writeError(w, http.StatusServiceUnavailable, "deployer not initialized")
		return
	}
	var body struct {
		Replicas int `json:"replicas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Replicas < 1 {
		writeError(w, http.StatusBadRequest, "replicas must be >= 1")
		return
	}
	if err := h.DockerDeployer.Scale(name, body.Replicas); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "scaled", "replicas": fmt.Sprintf("%d", body.Replicas)})
}

// ── Compose ─────────────────────────────────────────────────────────

// DockerComposeUp deploys from a compose file.
func (h *Handler) DockerComposeUp(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	var body struct {
		Dir     string `json:"dir"`
		Project string `json:"project"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	projectDir := body.Dir

	// If raw content is provided, write it to a temp directory
	if body.Content != "" {
		projectName := body.Project
		if projectName == "" {
			projectName = "setec-compose"
		}
		tmpDir := filepath.Join(os.TempDir(), "setec-compose", projectName)
		os.MkdirAll(tmpDir, 0755)
		composePath := filepath.Join(tmpDir, "docker-compose.yml")
		if err := os.WriteFile(composePath, []byte(body.Content), 0644); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("write compose file: %v", err))
			return
		}
		projectDir = tmpDir
	}

	if projectDir == "" {
		writeError(w, http.StatusBadRequest, "project directory or content is required")
		return
	}

	if err := h.DockerClient.ComposeUp(projectDir, body.Project); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deployed"})
}

// DockerComposeDown tears down a compose project.
func (h *Handler) DockerComposeDown(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	project := paramStr(r, "project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "project name is required")
		return
	}

	projectDir := filepath.Join(os.TempDir(), "setec-compose", project)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		projectDir = "."
	}

	if err := h.DockerClient.ComposeDown(projectDir, project); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// DockerComposePS lists services in a compose project.
func (h *Handler) DockerComposePS(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	project := paramStr(r, "project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "project name is required")
		return
	}

	projectDir := filepath.Join(os.TempDir(), "setec-compose", project)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		projectDir = "."
	}

	services, err := h.DockerClient.ComposePS(projectDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, services)
}

// DockerComposeLogs returns logs for a compose project.
func (h *Handler) DockerComposeLogs(w http.ResponseWriter, r *http.Request) {
	if !h.requireDocker(w) {
		return
	}
	project := paramStr(r, "project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "project name is required")
		return
	}

	projectDir := filepath.Join(os.TempDir(), "setec-compose", project)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		projectDir = "."
	}

	logs, err := h.DockerClient.ComposeLogs(projectDir, 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": logs})
}
