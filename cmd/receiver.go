package cmd

import (
	"os"

	pkg "github.com/leonardopoggiani/lmo-performance-evaluation/pkg"
	"github.com/spf13/cobra"
	"github.com/withmandala/go-log"
)

// serveCmd represents the serve command
var receiverCmd = &cobra.Command{
	Use:   "receiver",
	Short: "Start the receiver process",
	Long: `Start the receiver process for the Live Migration Operator.
The receiver will be started in the test namespace and will:
- 
- `,
	Run: func(cmd *cobra.Command, args []string) {
		pkg.Receive(log.New(os.Stderr).WithColor())
	},
}

func init() {
	rootCmd.AddCommand(receiverCmd)
}
