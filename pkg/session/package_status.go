package session

import (
	"context"

	apiv1 "github.com/leptonai/gpud/api/v1"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
)

// processPackageStatus handles the packageStatus request
func (s *Session) processPackageStatus(ctx context.Context, response *Response) {
	packageStatus, err := gpudmanager.GlobalController.Status(ctx)
	if err != nil {
		response.Error = err.Error()
		return
	}
	var result []apiv1.PackageStatus
	for _, currPackage := range packageStatus {
		packagePhase := apiv1.UnknownPhase
		if currPackage.Skipped {
			packagePhase = apiv1.SkippedPhase
		} else if currPackage.IsInstalled {
			packagePhase = apiv1.InstalledPhase
		} else if currPackage.Installing {
			packagePhase = apiv1.InstallingPhase
		}
		status := "Unhealthy"
		if currPackage.Status {
			status = "Healthy"
		}
		result = append(result, apiv1.PackageStatus{
			Name:           currPackage.Name,
			Phase:          packagePhase,
			Status:         status,
			CurrentVersion: currPackage.CurrentVersion,
		})
	}
	response.PackageStatus = result
}
