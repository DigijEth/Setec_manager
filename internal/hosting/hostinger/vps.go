package hostinger

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"setec-manager/internal/hosting"
)

// hostingerVM is the Hostinger API representation of a virtual machine.
type hostingerVM struct {
	ID         int    `json:"id"`
	Hostname   string `json:"hostname"`
	Status     string `json:"status"`
	Plan       string `json:"plan"`
	DataCenter string `json:"data_center"`
	IPv4       string `json:"ipv4"`
	IPv6       string `json:"ipv6"`
	OS         string `json:"os"`
	CPUs       int    `json:"cpus"`
	RAMMB      int    `json:"ram_mb"`
	DiskGB     int    `json:"disk_gb"`
	CreatedAt  string `json:"created_at"`
}

// hostingerDataCenter is the Hostinger API representation of a data center.
type hostingerDataCenter struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
	Country  string `json:"country"`
}

// hostingerSSHKey is the Hostinger API representation of an SSH key.
type hostingerSSHKey struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	CreatedAt string `json:"created_at"`
}

// hostingerCreateVMRequest is the request body for creating a VM.
type hostingerCreateVMRequest struct {
	Hostname     string `json:"hostname"`
	Plan         string `json:"plan"`
	DataCenterID int    `json:"data_center_id"`
	OS           string `json:"template"`
	Password     string `json:"password,omitempty"`
	SSHKeyID     *int   `json:"ssh_key_id,omitempty"`
}

// hostingerCreateVMResponse is the response from the VM creation endpoint.
type hostingerCreateVMResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// hostingerAddSSHKeyRequest is the request body for adding an SSH key.
type hostingerAddSSHKeyRequest struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

// ListVMs retrieves all virtual machines in the account.
func (c *Client) ListVMs() ([]hosting.VirtualMachine, error) {
	var vms []hostingerVM
	if err := c.doRequest(http.MethodGet, "/api/vps/v1/virtual-machines", nil, &vms); err != nil {
		return nil, fmt.Errorf("list VMs: %w", err)
	}

	result := make([]hosting.VirtualMachine, 0, len(vms))
	for _, vm := range vms {
		result = append(result, toGenericVM(vm))
	}
	return result, nil
}

// GetVM retrieves a specific virtual machine by ID.
func (c *Client) GetVM(id string) (*hosting.VirtualMachine, error) {
	path := fmt.Sprintf("/api/vps/v1/virtual-machines/%s", url.PathEscape(id))

	var vm hostingerVM
	if err := c.doRequest(http.MethodGet, path, nil, &vm); err != nil {
		return nil, fmt.Errorf("get VM %s: %w", id, err)
	}

	result := toGenericVM(vm)
	return &result, nil
}

// CreateVM provisions a new virtual machine.
func (c *Client) CreateVM(req hosting.VMCreateRequest) (*hosting.OrderResult, error) {
	body := hostingerCreateVMRequest{
		Hostname: req.Hostname,
		Plan:     req.Plan,
		OS:       req.OS,
		Password: req.Password,
	}

	// Parse data center ID from string to int for the Hostinger API.
	dcID, err := strconv.Atoi(req.DataCenterID)
	if err != nil {
		return nil, fmt.Errorf("invalid data center ID %q: must be numeric", req.DataCenterID)
	}
	body.DataCenterID = dcID

	// Parse SSH key ID if provided.
	if req.SSHKeyID != "" {
		keyID, err := strconv.Atoi(req.SSHKeyID)
		if err != nil {
			return nil, fmt.Errorf("invalid SSH key ID %q: must be numeric", req.SSHKeyID)
		}
		body.SSHKeyID = &keyID
	}

	var resp hostingerCreateVMResponse
	if err := c.doRequest(http.MethodPost, "/api/vps/v1/virtual-machines", body, &resp); err != nil {
		return nil, fmt.Errorf("create VM: %w", err)
	}

	return &hosting.OrderResult{
		OrderID: resp.OrderID,
		Status:  resp.Status,
		Message: resp.Message,
	}, nil
}

// ListDataCenters retrieves all available data centers.
func (c *Client) ListDataCenters() ([]hosting.DataCenter, error) {
	var dcs []hostingerDataCenter
	if err := c.doRequest(http.MethodGet, "/api/vps/v1/data-centers", nil, &dcs); err != nil {
		return nil, fmt.Errorf("list data centers: %w", err)
	}

	result := make([]hosting.DataCenter, 0, len(dcs))
	for _, dc := range dcs {
		result = append(result, hosting.DataCenter{
			ID:       strconv.Itoa(dc.ID),
			Name:     dc.Name,
			Location: dc.Location,
			Country:  dc.Country,
		})
	}
	return result, nil
}

// ListSSHKeys retrieves all SSH keys in the account.
func (c *Client) ListSSHKeys() ([]hosting.SSHKey, error) {
	var keys []hostingerSSHKey
	if err := c.doRequest(http.MethodGet, "/api/vps/v1/public-keys", nil, &keys); err != nil {
		return nil, fmt.Errorf("list SSH keys: %w", err)
	}

	result := make([]hosting.SSHKey, 0, len(keys))
	for _, k := range keys {
		created, _ := time.Parse(time.RFC3339, k.CreatedAt)
		result = append(result, hosting.SSHKey{
			ID:        strconv.Itoa(k.ID),
			Name:      k.Name,
			PublicKey: k.PublicKey,
			CreatedAt: created,
		})
	}
	return result, nil
}

// AddSSHKey uploads a new SSH public key.
func (c *Client) AddSSHKey(name, publicKey string) (*hosting.SSHKey, error) {
	body := hostingerAddSSHKeyRequest{
		Name:      name,
		PublicKey: publicKey,
	}

	var key hostingerSSHKey
	if err := c.doRequest(http.MethodPost, "/api/vps/v1/public-keys", body, &key); err != nil {
		return nil, fmt.Errorf("add SSH key: %w", err)
	}

	created, _ := time.Parse(time.RFC3339, key.CreatedAt)
	return &hosting.SSHKey{
		ID:        strconv.Itoa(key.ID),
		Name:      key.Name,
		PublicKey: key.PublicKey,
		CreatedAt: created,
	}, nil
}

// DeleteSSHKey removes an SSH key by ID.
func (c *Client) DeleteSSHKey(id string) error {
	path := fmt.Sprintf("/api/vps/v1/public-keys/%s", url.PathEscape(id))
	if err := c.doRequest(http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("delete SSH key %s: %w", id, err)
	}
	return nil
}

// toGenericVM converts a Hostinger VM to the generic VirtualMachine type.
func toGenericVM(vm hostingerVM) hosting.VirtualMachine {
	created, _ := time.Parse(time.RFC3339, vm.CreatedAt)

	return hosting.VirtualMachine{
		ID:         strconv.Itoa(vm.ID),
		Hostname:   vm.Hostname,
		IPAddress:  vm.IPv4,
		IPv6:       vm.IPv6,
		Status:     vm.Status,
		Plan:       vm.Plan,
		DataCenter: vm.DataCenter,
		OS:         vm.OS,
		CPUs:       vm.CPUs,
		RAMBytes:   int64(vm.RAMMB) * 1024 * 1024,
		DiskBytes:  int64(vm.DiskGB) * 1024 * 1024 * 1024,
		CreatedAt:  created,
	}
}
