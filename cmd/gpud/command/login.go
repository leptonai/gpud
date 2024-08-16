package command

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/leptonai/gpud/client"
	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/config"
	"github.com/leptonai/gpud/internal/login"
	"github.com/leptonai/gpud/internal/server"

	"github.com/urfave/cli"
)

func cmdLogin(cliContext *cli.Context) error {
	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	if err := client.BlockUntilServerReady(
		rootCtx,
		fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort),
	); err != nil {
		return err
	}

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

	components, err := state.GetComponents(rootCtx, db, uid)
	if err != nil {
		return fmt.Errorf("failed to get components: %w", err)
	}

	cliToken := cliContext.String("token")
	endpoint := cliContext.String("endpoint")

	dbToken, _ := state.GetLoginInfo(rootCtx, db, uid)
	token := dbToken
	if cliToken != "" {
		token = cliToken
	} else {
		fmt.Println("trying token from local store, if you want to override, use --token flag")
	}

	if token == "" {
		fmt.Print("Please visit https://dashboard.lepton.ai/ under Settings/Tokens to fetch your token\nPlease enter your token:")
		if _, err := fmt.Scanln(&token); err != nil && err.Error() != "unexpected newline" {
			return fmt.Errorf("failed reading input: %w", err)
		}
	}

	fifoFile, err := config.DefaultFifoFile()
	if err != nil {
		return fmt.Errorf("failed to get fifo file: %w", err)
	}

	if token != "" && endpoint != "" {
		hostname, err := os.Hostname()
		if err != nil {
			hostname = "UnknownName"
		}
		if err := login.Login(hostname, token, endpoint, components, uid); err != nil {
			return err
		}
	} else {
		fmt.Println("login skipped since token or endpoint not provided...")
		return nil
	}

	if err := server.WriteToken(token, fifoFile); err != nil {
		return fmt.Errorf("failed to write token: %v", err)
	}

	if token != dbToken {
		if err = state.UpdateLoginInfo(rootCtx, db, uid, token); err != nil {
			fmt.Println("machine logged in but failed to update token:", err)
		}
	}

	fmt.Printf("%s successfully logged into lepton.ai\n", checkMark)
	return nil
}
