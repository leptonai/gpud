package command

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/config"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/process"
)

func cmdJoin(cliContext *cli.Context) (retErr error) {
	rootCtx, rootCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer rootCancel()
	endpoint := cliContext.String("endpoint")
	clusterName := cliContext.String("cluster-name")
	publicIP := cliContext.String("public-ip")
	provider := cliContext.String("provider")
	providerGPUShape := cliContext.String("provider-gpu-shape")
	providerInstanceType := cliContext.String("provider-instance-type")
	xrayNeeded := cliContext.Bool("xray-needed")
	totalCPU := cliContext.Int64("total-cpu")
	extraInfo := cliContext.String("extra-info")

	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}
	db, err := state.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer db.Close()

	uid, _, err := state.CreateMachineIDIfNotExist(rootCtx, db)
	if err != nil {
		return fmt.Errorf("failed to get machine uid: %w", err)
	}

	type payload struct {
		ID                   string `json:"id"`
		ClusterName          string `json:"cluster_name"`
		PublicIP             string `json:"public_ip"`
		Provider             string `json:"provider"`
		ProviderGPUShape     string `json:"provider_gpu_shape"`
		ProviderInstanceType string `json:"provider_instance_type"`
		XrayNeeded           bool   `json:"xray_needed"`
		TotalCPU             int64  `json:"total_cpu"`
		ExtraInfo            string `json:"extra_info"`
	}
	type RespErr struct {
		Error  string `json:"error"`
		Status string `json:"status"`
	}
	content := payload{
		ID:                   uid,
		ClusterName:          clusterName,
		PublicIP:             publicIP,
		Provider:             provider,
		ProviderGPUShape:     providerGPUShape,
		ProviderInstanceType: providerInstanceType,
		XrayNeeded:           xrayNeeded,
		TotalCPU:             totalCPU,
		ExtraInfo:            extraInfo,
	}
	rawPayload, _ := json.Marshal(&content)
	response, err := http.Post(fmt.Sprintf("https://%s/api/v1/join", endpoint), "application/json", bytes.NewBuffer(rawPayload))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %w", err)
		}
		var errorResponse RespErr
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("Error parsing error response: %v\nResponse body: %s", err, body)
		}
		return fmt.Errorf("failed to join: %v", errorResponse)
	}
	return handleJoinResponse(rootCtx, response.Body)
}

func handleJoinResponse(ctx context.Context, body io.Reader) error {
	dir, err := untarFiles("/tmp/", body)
	if err != nil {
		return err
	}
	scriptPath := filepath.Join(dir, "join.sh")
	return runCommand(ctx, scriptPath, nil)
}

func untarFiles(targetDir string, body io.Reader) (string, error) {
	var dir string
	gzipReader, err := gzip.NewReader(body)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		fpath := filepath.Join(targetDir, header.Name)
		if dir == "" {
			dir = fpath
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				panic(err)
			}
		case tar.TypeReg:
			outFile, err := os.Create(fpath)
			if err != nil {
				panic(err)
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tarReader); err != nil {
				panic(err)
			}
		}
	}
	return dir, nil
}

func runCommand(ctx context.Context, script string, result *string) error {
	var ops []process.OpOption

	p, err := process.New([][]string{{"bash", script}}, ops...)
	if err != nil {
		return err
	}
	if result != nil {
		go func() {
			stdoutReader := p.StdoutReader()
			if stdoutReader == nil {
				log.Logger.Errorf("failed to read stdout: %v", err)
				return
			}
			rawResult, err := io.ReadAll(p.StdoutReader())
			if err != nil {
				log.Logger.Errorf("failed to read stout: %v", err)
				return
			}
			*result = strings.TrimSpace(string(rawResult))
		}()
	}
	if err = p.Start(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-p.Wait():
		if err != nil {
			return err
		}
	}
	if err := p.Abort(ctx); err != nil {
		return err
	}
	return nil
}
