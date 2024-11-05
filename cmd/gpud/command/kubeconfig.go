package command

import (
	"context"
	"time"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/aws"
	"github.com/leptonai/gpud/pkg/aws/eks"
)

func cmdKubeConfig(cliContext *cli.Context) (retErr error) {
	cfg, err := aws.New(&aws.Config{
		Region: cliContext.String("region"),
	})
	if err != nil {
		log.Logger.Warnw("failed to create aws config",
			"error", err,
		)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cluster, err := eks.GetCluster(ctx, cfg, cliContext.String("cluster"))
	cancel()
	if err != nil {
		log.Logger.Warnw("failed to get EKS cluster",
			"error", err,
		)
		return err
	}
	_, err = cluster.WriteKubeconfigWithAWSIAMAuthenticator(
		eks.WithKubeconfigFile(cliContext.String("file")),
		eks.WithClusterCAFile(cliContext.String("cluster-ca")),
		eks.WithRoleARN(cliContext.String("role")),
		eks.WithSessionName(cliContext.String("session")),
	)
	if err != nil {
		log.Logger.Warnw("failed to write kubeconfig/ca",
			"error", err,
		)
		return err
	}
	return nil
}
