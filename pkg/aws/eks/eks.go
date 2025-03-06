// Package eks implements EKS utils.
package eks

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_eks_v2 "github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/olekukonko/tablewriter"
	clientcmd_api_v1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/yaml"

	"github.com/leptonai/gpud/pkg/log"
)

type Op struct {
	clusterName    string
	limit          int
	kubeconfigFile string
	clusterCAFile  string
	roleARN        string
	sessionName    string
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
}

func WithClusterName(name string) OpOption {
	return func(op *Op) {
		op.clusterName = name
	}
}

func WithLimit(limit int) OpOption {
	return func(op *Op) {
		op.limit = limit
	}
}

// Set kubeconfig file path to write.
func WithKubeconfigFile(v string) OpOption {
	return func(op *Op) {
		op.kubeconfigFile = v
	}
}

// Set cluster CA file path to write.
func WithClusterCAFile(v string) OpOption {
	return func(op *Op) {
		op.clusterCAFile = v
	}
}

func WithRoleARN(v string) OpOption {
	return func(op *Op) {
		op.roleARN = v
	}
}

func WithSessionName(v string) OpOption {
	return func(op *Op) {
		op.sessionName = v
	}
}

func GetCluster(ctx context.Context, cfg aws.Config, clusterName string, opts ...OpOption) (Cluster, error) {
	cli := aws_eks_v2.NewFromConfig(cfg)
	return getClusterWithClient(ctx, cfg.Region, cli, clusterName, opts...)
}

func getClusterWithClient(ctx context.Context, region string, cli *aws_eks_v2.Client, clusterName string, opts ...OpOption) (Cluster, error) {
	cs, err := listClusters(ctx, region, cli, append(opts, WithClusterName(clusterName), WithLimit(1))...)
	if err != nil {
		return Cluster{}, err
	}
	if len(cs) != 1 {
		return Cluster{}, errors.New("not found")
	}
	return cs[0], nil
}

func listClusters(ctx context.Context, region string, cli *aws_eks_v2.Client, opts ...OpOption) ([]Cluster, error) {
	ret := &Op{}
	ret.applyOpts(opts)

	clusters := make([]Cluster, 0)

	var nextToken *string = nil
done:
	for i := 0; i < 20; i++ {
		clusterNames := []string{ret.clusterName}
		if ret.clusterName == "" {
			out, err := cli.ListClusters(ctx, &aws_eks_v2.ListClustersInput{
				NextToken: nextToken,
			})
			if err != nil {
				return nil, err
			}
			clusterNames = out.Clusters
			nextToken = out.NextToken
		}

		log.Logger.Infof("inspecting %d clusters", len(clusterNames))
		for _, cname := range clusterNames {
			cl, err := inspectCluster(ctx, region, "UNKNOWN", cli, nil, cname, opts...)
			if err != nil {
				return nil, err
			}

			clusters = append(clusters, cl)
			if ret.limit > 0 && len(clusters) >= ret.limit {
				log.Logger.Infof("already listed %d clusters with limit %d -- skipping the rest", len(clusters), ret.limit)
				break done
			}
		}

		log.Logger.Infof("listed %d clusters so far with limit %d", len(clusters), ret.limit)
		if nextToken == nil {
			// no more resources are available
			break
		}

		// TODO: add wait to prevent api throttle (rate limit)?
	}

	sort.SliceStable(clusters, func(i, j int) bool {
		return clusters[i].ARN < clusters[j].ARN
	})
	return clusters, nil
}

func inspectCluster(
	ctx context.Context,
	region string,
	mothershipState string,
	eksAPI *aws_eks_v2.Client,
	vpcToELBv2s map[string][]string,
	clusterName string,
	opts ...OpOption,
) (Cluster, error) {
	ret := &Op{}
	ret.applyOpts(opts)

	eksOut, err := eksAPI.DescribeCluster(
		ctx,
		&aws_eks_v2.DescribeClusterInput{
			Name: &clusterName,
		},
	)
	if err != nil {
		if IsErrClusterDeleted(err) {
			log.Logger.Infof("cluster %q already deleted", clusterName)
			return Cluster{Name: clusterName, Status: "DELETED"}, nil
		}
		return Cluster{}, err
	}

	platformVeresion := "UNKNOWN"
	if eksOut.Cluster.PlatformVersion != nil {
		platformVeresion = *eksOut.Cluster.PlatformVersion
	}
	vpcID := ""
	if eksOut.Cluster.ResourcesVpcConfig != nil {
		vpcID = *eksOut.Cluster.ResourcesVpcConfig.VpcId
	}

	oidcIssuer := ""
	if eksOut.Cluster.Identity != nil && eksOut.Cluster.Identity.Oidc != nil {
		oidcIssuer = *eksOut.Cluster.Identity.Oidc.Issuer
	}

	endpoint := ""
	if eksOut.Cluster.Endpoint != nil {
		endpoint = *eksOut.Cluster.Endpoint
	}

	ca := ""
	if eksOut.Cluster.CertificateAuthority != nil {
		ca = *eksOut.Cluster.CertificateAuthority.Data
	}

	version, status, health := GetClusterStatus(eksOut)
	attachedELBs := make([]string, 0)
	if vpcToELBv2s != nil {
		attachedELBs = vpcToELBv2s[vpcID]
	}
	c := Cluster{
		Name:   clusterName,
		ARN:    *eksOut.Cluster.Arn,
		Region: region,

		Version:         version,
		PlatformVersion: platformVeresion,
		MothershipState: mothershipState,
		Status:          status,
		Health:          health,

		CreatedAt: *eksOut.Cluster.CreatedAt,

		VPCID:             vpcID,
		ClusterSGID:       *eksOut.Cluster.ResourcesVpcConfig.ClusterSecurityGroupId,
		AttachedELBv2ARNs: attachedELBs,

		Endpoint:             endpoint,
		CertificateAuthority: ca,
		OIDCIssuer:           oidcIssuer,
	}

	return c, nil
}

type Cluster struct {
	Name   string `json:"name"`
	ARN    string `json:"arn"`
	Region string `json:"region"`

	Version         string `json:"version"`
	PlatformVersion string `json:"platform_version"`
	MothershipState string `json:"mothership_state"`
	Status          string `json:"status"`
	Health          string `json:"health"`

	CreatedAt time.Time `json:"created_at"`

	VPCID             string   `json:"vpc_id"`
	ClusterSGID       string   `json:"cluster_sg_id"`
	AttachedELBv2ARNs []string `json:"attached_elbv2_arns,omitempty"`

	Endpoint             string `json:"endpoint"`
	CertificateAuthority string `json:"certificate_authority"`
	OIDCIssuer           string `json:"oidc_issuer"`
}

func (c Cluster) String() string {
	buf := bytes.NewBuffer(nil)
	tb := tablewriter.NewWriter(buf)
	tb.SetAutoWrapText(false)
	tb.SetAlignment(tablewriter.ALIGN_LEFT)
	tb.SetCenterSeparator("*")
	tb.SetRowLine(true)
	tb.Append([]string{"CLUSTER KIND", "EKS"})
	tb.Append([]string{"NAME", c.Name})
	tb.Append([]string{"ARN", c.ARN})
	tb.Append([]string{"VERSION", c.Version})
	tb.Append([]string{"PLATFORM VERSION", c.PlatformVersion})
	tb.Append([]string{"MOTHERSHIP STATE", c.MothershipState})
	tb.Append([]string{"STATUS", c.Status})
	tb.Append([]string{"HEALTH", c.Health})
	tb.Append([]string{"CREATED AT", c.CreatedAt.String()})
	tb.Append([]string{"VPC ID", c.VPCID})
	tb.Append([]string{"SG ID", c.ClusterSGID})
	for i, arn := range c.AttachedELBv2ARNs {
		tb.Append([]string{fmt.Sprintf("ATTACHED ELBv2 ARN #%d", i+1), arn})
	}
	tb.Render()

	rs := buf.String()

	return rs
}

func (c Cluster) KubeconfigWithAWSIAMAuthenticator(opts ...OpOption) (clientcmd_api_v1.Config, error) {
	ret := &Op{}
	ret.applyOpts(opts)

	cmd, err := exec.LookPath("aws-iam-authenticator")
	if err != nil {
		return clientcmd_api_v1.Config{}, fmt.Errorf("aws cli not found %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(c.CertificateAuthority)
	if err != nil {
		return clientcmd_api_v1.Config{}, fmt.Errorf("failed to decode certificate authority %w", err)
	}

	args := []string{
		"token",
		"--region",
		c.Region,
		"--cluster-id",
		c.Name,
	}
	if ret.roleARN != "" {
		args = append(args, "--role", ret.roleARN)
	}
	if ret.sessionName != "" {
		args = append(args, "--session-name", ret.sessionName)
	}

	kcfg := clientcmd_api_v1.Config{
		Clusters: []clientcmd_api_v1.NamedCluster{
			{
				Name: c.ARN,
				Cluster: clientcmd_api_v1.Cluster{
					Server:                   c.Endpoint,
					CertificateAuthorityData: decoded,
				},
			},
		},
		Contexts: []clientcmd_api_v1.NamedContext{
			{
				Name: c.ARN,
				Context: clientcmd_api_v1.Context{
					Cluster:  c.ARN,
					AuthInfo: c.ARN,
				},
			},
		},
		CurrentContext: c.ARN,
		AuthInfos: []clientcmd_api_v1.NamedAuthInfo{
			{
				Name: c.ARN,
				AuthInfo: clientcmd_api_v1.AuthInfo{
					Exec: &clientcmd_api_v1.ExecConfig{
						APIVersion: "client.authentication.k8s.io/v1beta1",
						Command:    cmd,
						Args:       args,
					},
				},
			},
		},
	}
	return kcfg, nil
}

func (c Cluster) WriteKubeconfigWithAWSIAMAuthenticator(opts ...OpOption) (string, error) {
	kcfg, err := c.KubeconfigWithAWSIAMAuthenticator(opts...)
	if err != nil {
		return "", err
	}
	return c.writeKubeconfigYAML(kcfg, opts...)
}

func (c Cluster) writeKubeconfigYAML(kcfg clientcmd_api_v1.Config, opts ...OpOption) (string, error) {
	ret := &Op{}
	ret.applyOpts(opts)

	b, err := yaml.Marshal(kcfg)
	if err != nil {
		return "", err
	}

	if ret.kubeconfigFile == "" {
		f, err := os.CreateTemp(os.TempDir(), "gpud-*.kubeconfig")
		if err != nil {
			return "", err
		}
		ret.kubeconfigFile = f.Name()
		f.Close()
		os.RemoveAll(ret.kubeconfigFile)
	}

	if _, err := os.Stat(filepath.Dir(ret.kubeconfigFile)); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Dir(ret.kubeconfigFile), 0755); err != nil {
			return "", err
		}
	}

	log.Logger.Infow("writing kubeconfig", "file", ret.kubeconfigFile)
	if err = os.WriteFile(ret.kubeconfigFile, b, 0644); err != nil {
		return "", err
	}

	if ret.clusterCAFile != "" {
		if _, err := os.Stat(filepath.Dir(ret.clusterCAFile)); os.IsNotExist(err) {
			if err = os.MkdirAll(filepath.Dir(ret.clusterCAFile), 0755); err != nil {
				return "", err
			}
		}
		decoded, err := base64.StdEncoding.DecodeString(c.CertificateAuthority)
		if err != nil {
			return "", fmt.Errorf("failed to decode certificate authority %w", err)
		}
		log.Logger.Infow("writing cluster ca", "file", ret.clusterCAFile)
		if err = os.WriteFile(ret.clusterCAFile, decoded, 0644); err != nil {
			return "", err
		}
	}

	return ret.kubeconfigFile, nil
}

// Returns version, status, and health information.
func GetClusterStatus(out *aws_eks_v2.DescribeClusterOutput) (string, string, string) {
	version := *out.Cluster.Version
	status := string(out.Cluster.Status)

	health := "OK"
	if out.Cluster.Health != nil && out.Cluster.Health.Issues != nil && len(out.Cluster.Health.Issues) > 0 {
		health = fmt.Sprintf("%+v", out.Cluster.Health.Issues)
	}

	return version, status, health
}

func IsErrClusterDeleted(err error) bool {
	if err == nil {
		return false
	}
	awsErr, ok := err.(awserr.Error)
	if ok && awsErr.Code() == "ResourceNotFoundException" &&
		strings.HasPrefix(awsErr.Message(), "No cluster found for") {
		// ResourceNotFoundException: No cluster found for name: aws-k8s-tester-155468BC717E03B003\n\tstatus code: 404, request id: 1e3fe41c-b878-11e8-adca-b503e0ba731d
		return true
	}

	// must check the string
	// sometimes EKS API returns untyped error value
	return strings.Contains(err.Error(), "No cluster found for")
}
