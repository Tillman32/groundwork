package plugin

import "fmt"

var (
	// Standard capabilities that plugins can declare
	CapabilityPackageInstall   = "packages.install"
	CapabilityPackageRemove    = "packages.remove"
	CapabilityPackageList      = "packages.list"
	CapabilityOSUpdate         = "os.update"
	CapabilityOSUpdateCheck    = "os.update.check"
	CapabilityDiskCleanup      = "disk.cleanup"
	CapabilityDiskUsage        = "disk.usage"
	CapabilityLocalAdminList   = "localadmin.list"
	CapabilityLocalAdminManage = "localadmin.manage"
	CapabilityServiceManage    = "service.manage"
	CapabilityRegistryManage   = "registry.manage"
	CapabilityFileManage       = "file.manage"
	CapabilityScriptExecute    = "script.execute"
)

var AllCapabilities = []string{
	CapabilityPackageInstall,
	CapabilityPackageRemove,
	CapabilityPackageList,
	CapabilityOSUpdate,
	CapabilityOSUpdateCheck,
	CapabilityDiskCleanup,
	CapabilityDiskUsage,
	CapabilityLocalAdminList,
	CapabilityLocalAdminManage,
	CapabilityServiceManage,
	CapabilityRegistryManage,
	CapabilityFileManage,
	CapabilityScriptExecute,
}

func ValidateCapabilities(caps []string) error {
	known := make(map[string]bool)
	for _, c := range AllCapabilities {
		known[c] = true
	}
	for _, c := range caps {
		if !known[c] {
			return fmt.Errorf("unknown capability: %s", c)
		}
	}
	return nil
}

func CapabilityDescription(cap string) string {
	descriptions := map[string]string{
		CapabilityPackageInstall:   "Install software packages",
		CapabilityPackageRemove:    "Remove software packages",
		CapabilityPackageList:      "List installed packages",
		CapabilityOSUpdate:         "Install OS updates",
		CapabilityOSUpdateCheck:    "Check for available OS updates",
		CapabilityDiskCleanup:      "Clean up disk space",
		CapabilityDiskUsage:        "Report disk usage",
		CapabilityLocalAdminList:   "List local administrators",
		CapabilityLocalAdminManage: "Add/remove local administrators",
		CapabilityServiceManage:    "Manage Windows services",
		CapabilityRegistryManage:   "Manage registry keys",
		CapabilityFileManage:       "Manage files and directories",
		CapabilityScriptExecute:    "Execute arbitrary scripts",
	}
	if desc, ok := descriptions[cap]; ok {
		return desc
	}
	return cap
}