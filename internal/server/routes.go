package server

import (
	"io/fs"
	"net/http"

	"setec-manager/internal/handlers"
	"setec-manager/web"

	"github.com/go-chi/chi/v5"
)

func (s *Server) setupRoutes() {
	h := handlers.New(s.Config, s.DB, s.HostingConfigs)

	// Static assets (embedded)
	staticFS, _ := fs.Sub(web.StaticFS, "static")
	s.Router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Public routes
	s.Router.Group(func(r chi.Router) {
		r.Get("/login", s.handleLoginPage)
		r.With(s.loginRateLimit).Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
	})

	// Authenticated routes
	s.Router.Group(func(r chi.Router) {
		r.Use(s.authRequired)

		// Dashboard
		r.Get("/", h.Dashboard)
		r.Get("/api/system/info", h.SystemInfo)

		// Auth status
		r.Get("/api/auth/status", s.handleAuthStatus)

		// Sites
		r.Get("/sites", h.SiteList)
		r.Get("/sites/new", h.SiteNewForm)
		r.Post("/sites", h.SiteCreate)
		r.Get("/sites/{id}", h.SiteDetail)
		r.Put("/sites/{id}", h.SiteUpdate)
		r.Delete("/sites/{id}", h.SiteDelete)
		r.Post("/sites/{id}/deploy", h.SiteDeploy)
		r.Post("/sites/{id}/restart", h.SiteRestart)
		r.Post("/sites/{id}/stop", h.SiteStop)
		r.Post("/sites/{id}/start", h.SiteStart)
		r.Get("/sites/{id}/logs", h.SiteLogs)
		r.Get("/sites/{id}/logs/stream", h.SiteLogStream)

		// AUTARCH
		r.Get("/autarch", h.AutarchStatus)
		r.Post("/autarch/install", h.AutarchInstall)
		r.Post("/autarch/update", h.AutarchUpdate)
		r.Get("/autarch/status", h.AutarchStatusAPI)
		r.Post("/autarch/start", h.AutarchStart)
		r.Post("/autarch/stop", h.AutarchStop)
		r.Post("/autarch/restart", h.AutarchRestart)
		r.Get("/autarch/config", h.AutarchConfig)
		r.Put("/autarch/config", h.AutarchConfigUpdate)
		r.Post("/autarch/dns/build", h.AutarchDNSBuild)

		// Git / Code Repository Management
		r.Get("/git", h.GitSetup)
		r.Get("/git/setup", h.GitSetupWizard)
		r.Post("/git/install", h.GitInstall)
		r.Get("/git/status", h.GitStatus)
		r.Post("/git/uninstall", h.GitUninstall)
		r.Get("/git/repos", h.GitRepos)
		r.Post("/git/repos", h.GitRepoCreate)
		r.Delete("/git/repos/{owner}/{repo}", h.GitRepoDelete)
		r.Get("/git/repos/{owner}/{repo}", h.GitRepoGet)
		r.Post("/git/repos/mirror", h.GitRepoMirror)
		r.Get("/git/users", h.GitUsers)
		r.Post("/git/users", h.GitUserCreate)
		r.Delete("/git/users/{username}", h.GitUserDelete)
		r.Post("/git/users/{username}/reset-password", h.GitUserResetPassword)
		r.Patch("/git/users/{username}", h.GitUserUpdate)
		r.Get("/git/orgs", h.GitOrgs)
		r.Post("/git/orgs", h.GitOrgCreate)
		r.Delete("/git/orgs/{name}", h.GitOrgDelete)
		r.Get("/git/orgs/{name}/members", h.GitOrgMembers)
		r.Put("/git/orgs/{name}/members/{username}", h.GitOrgAddMember)
		r.Delete("/git/orgs/{name}/members/{username}", h.GitOrgRemoveMember)

		// SSL
		r.Get("/ssl", h.SSLOverview)
		r.Post("/ssl/{domain}/issue", h.SSLIssue)
		r.Post("/ssl/{domain}/renew", h.SSLRenew)
		r.Get("/api/ssl/status", h.SSLStatus)

		// Nginx
		r.Get("/nginx", h.NginxStatus)
		r.Post("/nginx/reload", h.NginxReload)
		r.Post("/nginx/restart", h.NginxRestart)
		r.Get("/nginx/config/{domain}", h.NginxConfigView)
		r.Post("/nginx/test", h.NginxTest)

		// Firewall
		r.Get("/firewall", h.FirewallList)
		r.Post("/firewall/rules", h.FirewallAddRule)
		r.Delete("/firewall/rules/{id}", h.FirewallDeleteRule)
		r.Post("/firewall/enable", h.FirewallEnable)
		r.Post("/firewall/disable", h.FirewallDisable)
		r.Get("/api/firewall/status", h.FirewallStatus)

		// System users
		r.Get("/users", h.UserList)
		r.Post("/users", h.UserCreate)
		r.Delete("/users/{id}", h.UserDelete)

		// Panel users
		r.Get("/panel/users", h.PanelUserList)
		r.Post("/users/panel", h.PanelUserCreate)
		r.Patch("/users/panel/{id}", h.PanelUserUpdate)
		r.Delete("/users/panel/{id}", h.PanelUserDelete)
		r.Get("/users/panel/{id}/groups", h.PanelUserGroups)
		r.Put("/users/panel/{id}/groups", h.PanelUserSetGroups)
		r.Get("/users/panel/{id}/domains", h.PanelUserDomainAccess)
		r.Post("/users/panel/{id}/domains", h.PanelUserGrantDomain)
		r.Delete("/users/panel/{id}/domains/{accessId}", h.PanelUserRevokeDomain)

		// Group Management
		r.Get("/users/groups", h.GroupList)
		r.Post("/users/groups", h.GroupCreate)
		r.Patch("/users/groups/{id}", h.GroupUpdate)
		r.Delete("/users/groups/{id}", h.GroupDelete)
		r.Get("/users/groups/{id}/permissions", h.GroupPermissions)
		r.Put("/users/groups/{id}/permissions", h.GroupSetPermissions)
		r.Get("/users/groups/{id}/members", h.GroupMembers)

		// Backups
		r.Get("/backups", h.BackupList)
		r.Post("/backups/site/{id}", h.BackupSite)
		r.Post("/backups/full", h.BackupFull)
		r.Delete("/backups/{id}", h.BackupDelete)
		r.Get("/backups/{id}/download", h.BackupDownload)

		// Hosting Provider Management
		r.Get("/hosting", h.HostingProviders)
		r.Get("/hosting/{provider}", h.HostingProviderConfig)
		r.Post("/hosting/{provider}/config", h.HostingProviderSave)
		r.Post("/hosting/{provider}/test", h.HostingProviderTest)
		// DNS
		r.Get("/hosting/{provider}/dns/{domain}", h.HostingDNSList)
		r.Put("/hosting/{provider}/dns/{domain}", h.HostingDNSUpdate)
		r.Delete("/hosting/{provider}/dns/{domain}", h.HostingDNSDelete)
		r.Post("/hosting/{provider}/dns/{domain}/reset", h.HostingDNSReset)
		// Domains
		r.Get("/hosting/{provider}/domains", h.HostingDomainsList)
		r.Post("/hosting/{provider}/domains/check", h.HostingDomainsCheck)
		r.Post("/hosting/{provider}/domains/purchase", h.HostingDomainsPurchase)
		r.Put("/hosting/{provider}/domains/{domain}/nameservers", h.HostingDomainNameservers)
		r.Put("/hosting/{provider}/domains/{domain}/lock", h.HostingDomainLock)
		r.Put("/hosting/{provider}/domains/{domain}/privacy", h.HostingDomainPrivacy)
		// VPS
		r.Get("/hosting/{provider}/vms", h.HostingVMsList)
		r.Get("/hosting/{provider}/vms/{id}", h.HostingVMGet)
		r.Post("/hosting/{provider}/vms", h.HostingVMCreate)
		r.Get("/hosting/{provider}/datacenters", h.HostingDataCenters)
		// SSH Keys
		r.Get("/hosting/{provider}/ssh-keys", h.HostingSSHKeys)
		r.Post("/hosting/{provider}/ssh-keys", h.HostingSSHKeyAdd)
		r.Delete("/hosting/{provider}/ssh-keys/{id}", h.HostingSSHKeyDelete)
		// Billing
		r.Get("/hosting/{provider}/subscriptions", h.HostingSubscriptions)
		r.Get("/hosting/{provider}/catalog", h.HostingCatalog)

		// Monitoring
		r.Get("/monitor", h.MonitorPage)
		r.Get("/api/monitor/cpu", h.MonitorCPU)
		r.Get("/api/monitor/memory", h.MonitorMemory)
		r.Get("/api/monitor/disk", h.MonitorDisk)
		r.Get("/api/monitor/services", h.MonitorServices)

		// Logs
		r.Get("/logs", h.LogsPage)
		r.Get("/api/logs/system", h.LogsSystem)
		r.Get("/api/logs/nginx", h.LogsNginx)
		r.Get("/api/logs/stream", h.LogsStream)

		// Float Mode
		r.Post("/float/register", h.FloatRegister)
		r.Get("/float/sessions", h.FloatSessions)
		r.Delete("/float/sessions/{id}", h.FloatDisconnect)
		r.Get("/float/ws", s.FloatBridge.HandleWebSocket)
	})
}
