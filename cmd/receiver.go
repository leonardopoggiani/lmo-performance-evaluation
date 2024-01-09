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

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
