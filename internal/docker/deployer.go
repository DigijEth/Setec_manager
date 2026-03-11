package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Labels used to identify managed containers
// ---------------------------------------------------------------------------

const (
	labelManaged    = "setec.managed"
	labelDeployment = "setec.deployment"
	labelDomain     = "setec.domain"
	labelReplica    = "setec.replica"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// DeployConfig holds all parameters for a one-click deployment.
type DeployConfig struct {
	Name     string            `json:"name"`     // container/app name
	Image    string            `json:"image"`    // Docker image (e.g. "autarch-cloud:latest")
	Domain   string            `json:"domain"`   // domain to route to (e.g. "app1.example.com")
	Port     int               `json:"port"`     // internal container port
	HostPort int               `json:"host_port"` // host port (auto-assigned if 0)
	Env      map[string]string `json:"env"`      // environment variables
	Volumes  map[string]string `json:"volumes"`  // host:container volume mappings
	Labels   map[string]string `json:"labels"`   // extra container labels
	Memory   int64             `json:"memory"`   // memory limit in bytes (0 = unlimited)
	CPUs     float64           `json:"cpus"`     // CPU limit (0 = unlimited)
	Restart  string            `json:"restart"`  // restart policy: "always", "unless-stopped", "on-failure"
	Network  string            `json:"network"`  // docker network name
	SSL      bool              `json:"ssl"`      // whether to configure SSL via Let's Encrypt
	SSLEmail string            `json:"ssl_email"` // email for Let's Encrypt
}

// Deployment represents a running managed deployment.
type Deployment struct {
	Name        string `json:"name"`
	Image       string `json:"image"`
	Domain      string `json:"domain"`
	Status      string `json:"status"`
	ContainerID string `json:"container_id"`
	HostPort    int    `json:"host_port"`
	Created     string `json:"created"`
	URL         string `json:"url"`
	Replicas    int    `json:"replicas"`
}

// deployState is the persistent state stored on disk.
type deployState struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Domain      string            `json:"domain"`
	Port        int               `json:"port"`
	HostPort    int               `json:"host_port"`
	Env         map[string]string `json:"env"`
	Volumes     map[string]string `json:"volumes"`
	Labels      map[string]string `json:"labels"`
	Memory      int64             `json:"memory"`
	CPUs        float64           `json:"cpus"`
	Restart     string            `json:"restart"`
	Network     string            `json:"network"`
	SSL         bool              `json:"ssl"`
	SSLEmail    string            `json:"ssl_email"`
	ContainerID string            `json:"container_id"`
	Replicas    int               `json:"replicas"`
	Created     time.Time         `json:"created"`
	Updated     time.Time         `json:"updated"`
}

// ---------------------------------------------------------------------------
// Deployer
// ---------------------------------------------------------------------------

// Deployer manages one-click container deployments with Nginx reverse proxy.
type Deployer struct {
	docker       *Client
	deployments  map[string]*deployState
	mu           sync.RWMutex
	stateFile    string
	nginxSites   string   // path to sites-available
	nginxEnabled string   // path to sites-enabled
	portRange    [2]int   // [min, max] for auto-assigned host ports
}

// NewDeployer creates a new Deployer backed by the given Client. The stateDir
// is where the deployment state JSON is persisted. nginxSites is the path to
// the Nginx sites-available directory.
func NewDeployer(docker *Client, stateDir, nginxSites string) *Deployer {
	nginxEnabled := strings.Replace(nginxSites, "sites-available", "sites-enabled", 1)
	d := &Deployer{
		docker:       docker,
		deployments:  make(map[string]*deployState),
		stateFile:    filepath.Join(stateDir, "docker-deployments.json"),
		nginxSites:   nginxSites,
		nginxEnabled: nginxEnabled,
		portRange:    [2]int{10000, 20000},
	}
	d.loadState()
	return d
}

// Deploy creates a container and configures Nginx reverse proxy for domain
// routing. This is the primary one-click deploy method.
func (d *Deployer) Deploy(cfg DeployConfig) (*Deployment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Validate required fields.
	if cfg.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if cfg.Image == "" {
		return nil, fmt.Errorf("image is required")
	}

	// Check for existing deployment.
	if _, exists := d.deployments[cfg.Name]; exists {
		return nil, fmt.Errorf("deployment %q already exists", cfg.Name)
	}

	// Defaults.
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.Restart == "" {
		cfg.Restart = "unless-stopped"
	}

	// Auto-assign host port if not specified.
	hostPort := cfg.HostPort
	if hostPort == 0 {
		hostPort = d.findAvailablePort()
	}

	// Step 1: Pull image (best-effort — may be a local image).
	_ = d.docker.PullImage(cfg.Image)

	// Step 2: Build the container config.
	containerName := "deploy-" + cfg.Name

	// Merge labels.
	labels := map[string]string{
		labelManaged:    "true",
		labelDeployment: cfg.Name,
		labelDomain:     cfg.Domain,
	}
	for k, v := range cfg.Labels {
		labels[k] = v
	}

	// Build environment slice.
	var envSlice []string
	for k, v := range cfg.Env {
		envSlice = append(envSlice, k+"="+v)
	}

	// Build port bindings.
	portKey := fmt.Sprintf("%d/tcp", cfg.Port)
	portBindings := map[string][]PortBinding{
		portKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(hostPort)}},
	}

	// Build volume binds.
	var binds []string
	for hostPath, containerPath := range cfg.Volumes {
		binds = append(binds, hostPath+":"+containerPath)
	}

	// Build host config.
	hc := &HostConfig{
		PortBindings: portBindings,
		Binds:        binds,
		RestartPolicy: RestartPolicy{
			Name: cfg.Restart,
		},
	}
	if cfg.Memory > 0 {
		hc.Memory = cfg.Memory
	}
	if cfg.CPUs > 0 {
		// Convert CPUs to NanoCPUs (1 CPU = 1e9 NanoCPUs).
		hc.NanoCPUs = int64(cfg.CPUs * 1e9)
	}
	if cfg.Network != "" {
		hc.NetworkMode = cfg.Network
	}

	containerCfg := ContainerConfig{
		Image:  cfg.Image,
		Env:    envSlice,
		Labels: labels,
		ExposedPorts: map[string]struct{}{
			portKey: {},
		},
		HostConfig: hc,
	}

	// Step 3: Create the container.
	cr, err := d.docker.CreateContainer(containerName, containerCfg)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}

	// Step 4: Start the container.
	if err := d.docker.StartContainer(cr.ID); err != nil {
		// Clean up on failure.
		d.docker.RemoveContainer(cr.ID, true)
		return nil, fmt.Errorf("start container: %w", err)
	}

	// Step 5: Generate Nginx config for domain routing.
	if cfg.Domain != "" {
		if err := d.writeNginxConfig(cfg.Name, cfg.Domain, hostPort, 1); err != nil {
			// Container is running but nginx failed — log but don't fail.
			_ = err
		} else {
			d.enableNginxSite(cfg.Name)
			reloadNginx()
		}

		// Step 6: Issue SSL certificate if requested.
		if cfg.SSL && cfg.SSLEmail != "" {
			go d.issueSSL(cfg.Domain, cfg.SSLEmail)
		}
	}

	// Save state.
	now := time.Now()
	state := &deployState{
		Name:        cfg.Name,
		Image:       cfg.Image,
		Domain:      cfg.Domain,
		Port:        cfg.Port,
		HostPort:    hostPort,
		Env:         cfg.Env,
		Volumes:     cfg.Volumes,
		Labels:      cfg.Labels,
		Memory:      cfg.Memory,
		CPUs:        cfg.CPUs,
		Restart:     cfg.Restart,
		Network:     cfg.Network,
		SSL:         cfg.SSL,
		SSLEmail:    cfg.SSLEmail,
		ContainerID: cr.ID,
		Replicas:    1,
		Created:     now,
		Updated:     now,
	}
	d.deployments[cfg.Name] = state
	d.saveState()

	return d.toDeployment(state), nil
}

// Undeploy stops and removes a deployment, including its container(s) and
// Nginx configuration.
func (d *Deployer) Undeploy(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, ok := d.deployments[name]
	if !ok {
		return fmt.Errorf("deployment %q not found", name)
	}

	// Stop and remove the primary container.
	containerName := "deploy-" + name
	d.docker.StopContainer(containerName, 10)
	d.docker.RemoveContainer(containerName, true)

	// Remove replica containers.
	for i := 2; i <= state.Replicas; i++ {
		replicaName := fmt.Sprintf("deploy-%s-%d", name, i)
		d.docker.StopContainer(replicaName, 10)
		d.docker.RemoveContainer(replicaName, true)
	}

	// Remove Nginx config.
	d.disableNginxSite(name)
	d.removeNginxConfig(name)
	reloadNginx()

	delete(d.deployments, name)
	d.saveState()

	return nil
}

// Scale creates multiple instances of a deployment behind a load-balanced
// Nginx upstream block. The replicas parameter sets the desired number of
// running instances.
func (d *Deployer) Scale(name string, replicas int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, ok := d.deployments[name]
	if !ok {
		return fmt.Errorf("deployment %q not found", name)
	}

	if replicas < 1 {
		replicas = 1
	}
	if replicas > 20 {
		replicas = 20
	}

	current := state.Replicas
	if current < 1 {
		current = 1
	}

	if replicas > current {
		// Scale up: create new replica containers.
		for i := current + 1; i <= replicas; i++ {
			port := d.findAvailablePort()
			replicaName := fmt.Sprintf("deploy-%s-%d", name, i)

			labels := map[string]string{
				labelManaged:    "true",
				labelDeployment: name,
				labelReplica:    strconv.Itoa(i),
			}

			var envSlice []string
			for k, v := range state.Env {
				envSlice = append(envSlice, k+"="+v)
			}

			var binds []string
			for hostPath, containerPath := range state.Volumes {
				binds = append(binds, hostPath+":"+containerPath)
			}

			portKey := fmt.Sprintf("%d/tcp", state.Port)
			hc := &HostConfig{
				PortBindings: map[string][]PortBinding{
					portKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(port)}},
				},
				Binds:         binds,
				RestartPolicy: RestartPolicy{Name: state.Restart},
			}
			if state.Memory > 0 {
				hc.Memory = state.Memory
			}
			if state.CPUs > 0 {
				hc.NanoCPUs = int64(state.CPUs * 1e9)
			}

			cfg := ContainerConfig{
				Image:  state.Image,
				Env:    envSlice,
				Labels: labels,
				ExposedPorts: map[string]struct{}{
					portKey: {},
				},
				HostConfig: hc,
			}

			cr, err := d.docker.CreateContainer(replicaName, cfg)
			if err != nil {
				return fmt.Errorf("create replica %d: %w", i, err)
			}
			if err := d.docker.StartContainer(cr.ID); err != nil {
				d.docker.RemoveContainer(cr.ID, true)
				return fmt.Errorf("start replica %d: %w", i, err)
			}
		}
	} else if replicas < current {
		// Scale down: remove excess replicas (highest first).
		for i := current; i > replicas; i-- {
			replicaName := fmt.Sprintf("deploy-%s-%d", name, i)
			d.docker.StopContainer(replicaName, 10)
			d.docker.RemoveContainer(replicaName, true)
		}
	}

	state.Replicas = replicas
	state.Updated = time.Now()

	// Regenerate Nginx config with upstream block if scaled.
	if state.Domain != "" {
		d.writeNginxConfig(name, state.Domain, state.HostPort, replicas)
		reloadNginx()
	}

	d.saveState()
	return nil
}

// ListDeployments returns all managed deployments with current container status.
func (d *Deployer) ListDeployments() ([]Deployment, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Get all containers to check live status.
	containers, _ := d.docker.ListContainers(true)
	containerStatus := make(map[string]string)
	for _, c := range containers {
		for _, n := range c.Names {
			// Docker prepends "/" to container names.
			containerStatus[strings.TrimPrefix(n, "/")] = c.State
		}
	}

	var deps []Deployment
	for _, state := range d.deployments {
		dep := d.toDeployment(state)
		// Update status from live container state.
		if status, ok := containerStatus["deploy-"+state.Name]; ok {
			dep.Status = status
		} else {
			dep.Status = "removed"
		}
		deps = append(deps, *dep)
	}
	return deps, nil
}

// GetDeployment returns the status of a single deployment.
func (d *Deployer) GetDeployment(name string) (*Deployment, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state, ok := d.deployments[name]
	if !ok {
		return nil, fmt.Errorf("deployment %q not found", name)
	}

	dep := d.toDeployment(state)

	// Check live container status.
	detail, err := d.docker.GetContainer("deploy-" + name)
	if err == nil {
		dep.Status = detail.State.Status
	} else {
		dep.Status = "removed"
	}

	return dep, nil
}

// UpdateDeployment pulls a new image and recreates the container(s) for a
// deployment, preserving all configuration.
func (d *Deployer) UpdateDeployment(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, ok := d.deployments[name]
	if !ok {
		return fmt.Errorf("deployment %q not found", name)
	}

	// Pull latest image.
	if err := d.docker.PullImage(state.Image); err != nil {
		return fmt.Errorf("pull image: %w", err)
	}

	// Recreate the primary container.
	containerName := "deploy-" + name
	d.docker.StopContainer(containerName, 10)
	d.docker.RemoveContainer(containerName, true)

	labels := map[string]string{
		labelManaged:    "true",
		labelDeployment: name,
		labelDomain:     state.Domain,
	}
	for k, v := range state.Labels {
		labels[k] = v
	}

	var envSlice []string
	for k, v := range state.Env {
		envSlice = append(envSlice, k+"="+v)
	}

	var binds []string
	for hostPath, containerPath := range state.Volumes {
		binds = append(binds, hostPath+":"+containerPath)
	}

	portKey := fmt.Sprintf("%d/tcp", state.Port)
	hc := &HostConfig{
		PortBindings: map[string][]PortBinding{
			portKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(state.HostPort)}},
		},
		Binds:         binds,
		RestartPolicy: RestartPolicy{Name: state.Restart},
	}
	if state.Memory > 0 {
		hc.Memory = state.Memory
	}
	if state.CPUs > 0 {
		hc.NanoCPUs = int64(state.CPUs * 1e9)
	}
	if state.Network != "" {
		hc.NetworkMode = state.Network
	}

	cfg := ContainerConfig{
		Image:  state.Image,
		Env:    envSlice,
		Labels: labels,
		ExposedPorts: map[string]struct{}{
			portKey: {},
		},
		HostConfig: hc,
	}

	cr, err := d.docker.CreateContainer(containerName, cfg)
	if err != nil {
		return fmt.Errorf("recreate container: %w", err)
	}

	if err := d.docker.StartContainer(cr.ID); err != nil {
		d.docker.RemoveContainer(cr.ID, true)
		return fmt.Errorf("start container: %w", err)
	}

	state.ContainerID = cr.ID
	state.Updated = time.Now()

	// Recreate replicas if scaled.
	for i := 2; i <= state.Replicas; i++ {
		replicaName := fmt.Sprintf("deploy-%s-%d", name, i)
		d.docker.StopContainer(replicaName, 10)
		d.docker.RemoveContainer(replicaName, true)

		replicaLabels := map[string]string{
			labelManaged:    "true",
			labelDeployment: name,
			labelReplica:    strconv.Itoa(i),
		}

		port := d.findAvailablePort()
		replicaHC := &HostConfig{
			PortBindings: map[string][]PortBinding{
				portKey: {{HostIP: "0.0.0.0", HostPort: strconv.Itoa(port)}},
			},
			Binds:         binds,
			RestartPolicy: RestartPolicy{Name: state.Restart},
		}
		if state.Memory > 0 {
			replicaHC.Memory = state.Memory
		}
		if state.CPUs > 0 {
			replicaHC.NanoCPUs = int64(state.CPUs * 1e9)
		}

		replicaCfg := ContainerConfig{
			Image:  state.Image,
			Env:    envSlice,
			Labels: replicaLabels,
			ExposedPorts: map[string]struct{}{
				portKey: {},
			},
			HostConfig: replicaHC,
		}

		rcr, err := d.docker.CreateContainer(replicaName, replicaCfg)
		if err != nil {
			continue
		}
		d.docker.StartContainer(rcr.ID)
	}

	d.saveState()
	return nil
}

// ---------------------------------------------------------------------------
// Nginx configuration
// ---------------------------------------------------------------------------

// writeNginxConfig generates an Nginx reverse proxy config for a deployment.
// When replicas > 1 it creates an upstream block for load balancing.
func (d *Deployer) writeNginxConfig(name, domain string, basePort, replicas int) error {
	if d.nginxSites == "" || domain == "" {
		return nil
	}

	var b strings.Builder

	b.WriteString("# Managed by Setec App Manager — do not edit manually\n")
	b.WriteString("# Deployment: " + name + "\n\n")

	// Build upstream block for load balancing when scaled.
	if replicas > 1 {
		fmt.Fprintf(&b, "upstream deploy_%s {\n", name)
		fmt.Fprintf(&b, "    server 127.0.0.1:%d;\n", basePort)
		for i := 2; i <= replicas; i++ {
			// Each replica gets a consecutive port from the base.
			fmt.Fprintf(&b, "    server 127.0.0.1:%d;\n", basePort+i-1)
		}
		b.WriteString("}\n\n")
	}

	proxyTarget := fmt.Sprintf("http://127.0.0.1:%d", basePort)
	if replicas > 1 {
		proxyTarget = fmt.Sprintf("http://deploy_%s", name)
	}

	// HTTP server block — redirects to HTTPS (or serves directly if no SSL).
	fmt.Fprintf(&b, "server {\n")
	fmt.Fprintf(&b, "    listen 80;\n")
	fmt.Fprintf(&b, "    server_name %s;\n\n", domain)
	fmt.Fprintf(&b, "    location /.well-known/acme-challenge/ {\n")
	fmt.Fprintf(&b, "        root /var/www/certbot;\n")
	fmt.Fprintf(&b, "    }\n\n")
	fmt.Fprintf(&b, "    location / {\n")
	fmt.Fprintf(&b, "        proxy_pass %s;\n", proxyTarget)
	fmt.Fprintf(&b, "        proxy_set_header Host $host;\n")
	fmt.Fprintf(&b, "        proxy_set_header X-Real-IP $remote_addr;\n")
	fmt.Fprintf(&b, "        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	fmt.Fprintf(&b, "        proxy_set_header X-Forwarded-Proto $scheme;\n")
	fmt.Fprintf(&b, "        proxy_http_version 1.1;\n")
	fmt.Fprintf(&b, "        proxy_set_header Upgrade $http_upgrade;\n")
	fmt.Fprintf(&b, "        proxy_set_header Connection \"upgrade\";\n")
	fmt.Fprintf(&b, "        proxy_buffering off;\n")
	fmt.Fprintf(&b, "        proxy_read_timeout 86400;\n")
	fmt.Fprintf(&b, "        client_max_body_size 100m;\n")
	fmt.Fprintf(&b, "    }\n")
	fmt.Fprintf(&b, "}\n")

	confPath := filepath.Join(d.nginxSites, "deploy-"+name)
	return os.WriteFile(confPath, []byte(b.String()), 0644)
}

func (d *Deployer) enableNginxSite(name string) {
	src := filepath.Join(d.nginxSites, "deploy-"+name)
	dst := filepath.Join(d.nginxEnabled, "deploy-"+name)
	os.Remove(dst)
	os.Symlink(src, dst)
}

func (d *Deployer) disableNginxSite(name string) {
	dst := filepath.Join(d.nginxEnabled, "deploy-"+name)
	os.Remove(dst)
}

func (d *Deployer) removeNginxConfig(name string) {
	src := filepath.Join(d.nginxSites, "deploy-"+name)
	os.Remove(src)
}

func reloadNginx() {
	exec.Command("systemctl", "reload", "nginx").Run()
}

// issueSSL uses certbot to issue a Let's Encrypt certificate for the domain.
func (d *Deployer) issueSSL(domain, email string) {
	if domain == "" || email == "" {
		return
	}
	exec.Command("certbot", "--nginx",
		"-d", domain,
		"--email", email,
		"--agree-tos",
		"--non-interactive",
		"--redirect",
	).Run()
}

// ---------------------------------------------------------------------------
// Port allocation
// ---------------------------------------------------------------------------

func (d *Deployer) findAvailablePort() int {
	used := make(map[int]bool)
	for _, dep := range d.deployments {
		used[dep.HostPort] = true
		// Also mark replica ports as used (they are sequential from HostPort).
		for i := 2; i <= dep.Replicas; i++ {
			used[dep.HostPort+i-1] = true
		}
	}
	for port := d.portRange[0]; port < d.portRange[1]; port++ {
		if !used[port] {
			return port
		}
	}
	return d.portRange[0]
}

// ---------------------------------------------------------------------------
// State persistence
// ---------------------------------------------------------------------------

func (d *Deployer) toDeployment(s *deployState) *Deployment {
	dep := &Deployment{
		Name:        s.Name,
		Image:       s.Image,
		Domain:      s.Domain,
		Status:      "unknown",
		ContainerID: s.ContainerID,
		HostPort:    s.HostPort,
		Created:     s.Created.Format(time.RFC3339),
		Replicas:    s.Replicas,
	}
	if s.Domain != "" {
		scheme := "http"
		if s.SSL {
			scheme = "https"
		}
		dep.URL = fmt.Sprintf("%s://%s", scheme, s.Domain)
	}
	return dep
}

func (d *Deployer) saveState() {
	data, err := json.MarshalIndent(d.deployments, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(d.stateFile), 0755)
	os.WriteFile(d.stateFile, data, 0600)
}

func (d *Deployer) loadState() {
	data, err := os.ReadFile(d.stateFile)
	if err != nil {
		return
	}
	json.Unmarshal(data, &d.deployments)
}
