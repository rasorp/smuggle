package agent

import (
	"fmt"

	"github.com/hashicorp/nomad/api"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/helper/retry"
)

// setupNomadClient initializes and returns a Nomad API client based on the
// agent's configuration. It also performs an initial connectivity check to
// ensure the client can communicate with the Nomad server. This check will be
// retried until successful or a timeout occurs, so the function may block for
// a short period.
func (a *Agent) setupNomadClient() (*api.Client, error) {

	nomadClient, err := config.NomadClient(a.cfg.Nomad)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nomad client: %w", err)
	}

	// Perform an initial API connectivity check. The leader endpoint does not
	// require ACL authentication and is a lighweight call, so it is the best
	// choice for this.
	return nomadClient, retry.Retry(
		func() error {
			_, err := nomadClient.Status().Leader()
			if err != nil {
				a.logger.Warn("failed to ping the Nomad API", zap.Error(err))
			}
			return err
		},
	)
}
