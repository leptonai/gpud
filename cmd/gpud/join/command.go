// Package join implements the "join" command.
package join

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	latencyedge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/osutil"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func Command(cliContext *cli.Context) (retErr error) {
	logLevel := cliContext.String("log-level")
	logFile := cliContext.String("log-file")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	log.Logger.Debugw("starting join command")

	if err := osutil.RequireRoot(); err != nil {
		return err
	}

	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRW.Close()

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer rootCancel()
	machineID, err := pkgmetadata.ReadMachineIDWithFallback(rootCtx, dbRW, dbRO)
	if err != nil {
		return err
	}

	// always read endpoint from state file
	endpoint, err := pkgmetadata.ReadMetadata(rootCtx, dbRO, pkgmetadata.MetadataKeyEndpoint)
	if err != nil {
		return fmt.Errorf("failed to read endpoint: %w", err)
	}
	if endpoint == "" {
		return errors.New("endpoint not found in state file")
	}

	// assume if not empty, it should have been persisted by the "gpud login" command
	privateIP, err := pkgmetadata.ReadMetadata(rootCtx, dbRO, pkgmetadata.MetadataKeyPrivateIP)
	if err != nil {
		return fmt.Errorf("failed to read private IP: %w", err)
	}

	// assume if not empty, it should have been persisted by the "gpud login" command
	publicIP, err := pkgmetadata.ReadMetadata(rootCtx, dbRO, pkgmetadata.MetadataKeyPublicIP)
	if err != nil {
		return fmt.Errorf("failed to read public IP: %w", err)
	}

	clusterName := cliContext.String("cluster-name")
	provider := cliContext.String("provider")
	providerInstanceID := cliContext.String("provider-instance-id")
	nodeGroup := cliContext.String("node-group")
	extraInfo := cliContext.String("extra-info")

	_, totalCPU, err := pkgmachineinfo.GetSystemResourceLogicalCores()
	if err != nil {
		return fmt.Errorf("failed to get system resource logical cores: %w", err)
	}

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return err
	}
	productName := nvmlInstance.ProductName()
	if cliContext.String("gpu-product") != "" {
		productName = cliContext.String("gpu-product")
	}

	// network section
	log.Logger.Debugw("measuring latencies to public tailscale DERP nodes to determine region")
	region := "unknown"
	latencies, _ := latencyedge.Measure(rootCtx)
	if len(latencies) > 0 {
		closest := latencies.Closest()
		region = closest.RegionCode
	}
	if cliContext.String("region") != "" {
		region = cliContext.String("region")
	}

	detectedProvider := pkgmachineinfo.GetProvider(publicIP)

	if !cliContext.Bool("skip-interactive") {
		reader := bufio.NewReader(os.Stdin)
		var input string
		if productName != "unknown" {
			fmt.Printf("We detect your gpu type is %v, if this is correct, press Enter. If not, please enter your gpu shape below\n", productName)
			input, err = reader.ReadString('\n')
			if err != nil {
				return err
			}
			if input != "\n" {
				productName = strings.TrimSpace(input)
			}
		}

		fmt.Printf("We detect your public IP is %v, if this is correct, press Enter. If not, please enter your public IP below\n", publicIP)
		input, err = reader.ReadString('\n')
		if err != nil {
			return err
		}
		if input != "\n" {
			publicIP = strings.TrimSpace(input)
		}

		if provider == "" {
			fmt.Printf("Provider name not specified, we detected your provider is %v, if correct, press Enter. If not, please enter your provider's name below\n", detectedProvider.Provider)
			input, err = reader.ReadString('\n')
			if err != nil {
				return err
			}
			if input != "\n" {
				provider = strings.TrimSpace(input)
			} else {
				provider = detectedProvider.Provider
			}
		}
		if providerInstanceID == "" {
			fmt.Printf("Provider instance id not specified, we detected your provider instance id is %v, if correct, press Enter. If not, please enter your provider instance id below\n", detectedProvider.InstanceID)
			input, err = reader.ReadString('\n')
			if err != nil {
				return err
			}
			if input != "\n" {
				providerInstanceID = strings.TrimSpace(input)
			} else {
				providerInstanceID = detectedProvider.InstanceID
			}
		}

		fmt.Printf("We detect your region is %v, if this is correct, press Enter. If not, please enter your region below\n", region)
		input, err = reader.ReadString('\n')
		if err != nil {
			return err
		}
		if input != "\n" {
			region = strings.TrimSpace(input)
		}
	} else {
		if provider == "" {
			provider = detectedProvider.Provider
		}
		if providerInstanceID == "" {
			providerInstanceID = detectedProvider.InstanceID
		}
	}

	fmt.Printf("%sWarning: GPUd will upgrade your container runtime to containerd, will affect your current running containers (if any)%s\n", "\033[33m", "\033[0m")
	fmt.Printf("%sWarning: GPUd will Reboot your machine to finish necessary setup%s\n", "\033[33m", "\033[0m")

	content := apiv1.JoinRequest{
		ID:                 machineID,
		ClusterName:        clusterName,
		PublicIP:           publicIP,
		Provider:           strings.Replace(provider, " ", "-", -1),
		ProviderInstanceID: providerInstanceID,
		ProviderGPUShape:   productName,
		TotalCPU:           totalCPU,
		NodeGroup:          nodeGroup,
		ExtraInfo:          extraInfo,
		Region:             region,
		PrivateIP:          privateIP,
	}

	rawPayload, _ := json.Marshal(&content)
	fmt.Println("Your machine will be initialized with following configuration")
	prettyJSON, _ := json.MarshalIndent(content, "", "  ")
	fmt.Println(string(prettyJSON))

	if !cliContext.Bool("skip-interactive") {
		fmt.Printf("Please look carefully about the above warning, if ok, please hit Enter\n")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if input != "\n" {
			fmt.Println("Non empty input received, GPUd join aborted.")
			return nil
		}
	}

	response, err := http.Post(createJoinURL(endpoint), "application/json", bytes.NewBuffer(rawPayload))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %w", err)
		}
		var errorResponse apiv1.JoinResponse
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("error parsing error response: %v %s", err, string(body))
		}
		return fmt.Errorf("failed to join: %v", errorResponse)
	}

	// persist on the successful join
	// so that next gpud up/run doesn't need to specify the same parameters
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyPublicIP, publicIP); err != nil {
		return fmt.Errorf("failed to record public IP: %w", err)
	}
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyProvider, provider); err != nil {
		return fmt.Errorf("failed to record provider: %w", err)
	}
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyNodeGroup, nodeGroup); err != nil {
		return fmt.Errorf("failed to record node group: %w", err)
	}
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyRegion, region); err != nil {
		return fmt.Errorf("failed to record region: %w", err)
	}
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyExtraInfo, extraInfo); err != nil {
		return fmt.Errorf("failed to record extra info: %w", err)
	}

	fmt.Println("Basic setup finished, GPUd is installing necessary components onto your machine, this may take 10 - 15 minutes.\nYou can run `gpud status` or `gpud status -w` to check the progress of each component.")
	return nil
}

// createJoinURL creates a URL for the join endpoint
func createJoinURL(endpoint string) string {
	host := endpoint
	url, _ := url.Parse(endpoint)
	if url.Host != "" {
		host = url.Host
	}
	return fmt.Sprintf("https://%s/api/v1/join", host)
}
