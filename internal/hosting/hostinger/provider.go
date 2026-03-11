package hostinger

import "setec-manager/internal/hosting"

func init() {
	hosting.Register(New(""))
}
