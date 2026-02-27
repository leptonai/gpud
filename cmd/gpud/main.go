package main

// @title GPUd API
// @version 1.0
// @description GPU monitoring and management daemon API
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:15132
// @BasePath /

import (
	"fmt"
	"io"
	"os"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/cmd/gpud/command"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
)

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	app := command.App()
	if err := app.Run(args); err != nil {
		if jsonErr, ok := gpudcommon.AsJSONCommandError(err); ok {
			if writeErr := gpudcommon.WriteJSONToWriter(stdout, jsonErr.Response()); writeErr != nil {
				_, _ = fmt.Fprintf(stderr, "%s %s\n", cmdcommon.WarningSign, writeErr)
			}
			return jsonErr.ExitStatus()
		}
		_, _ = fmt.Fprintf(stderr, "%s %s\n", cmdcommon.WarningSign, err)
		return 1
	}
	return 0
}
