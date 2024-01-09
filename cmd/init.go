package cmd

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	pkg "github.com/leonardopoggiani/lmo-performance-evaluation/pkg"
	"github.com/spf13/cobra"
	"github.com/withmandala/go-log"
)

// serveCmd represents the serve command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Init db configuration for live migration operator",
	Run: func(cmd *cobra.Command, args []string) {
		logger := log.New(os.Stderr).WithColor()
		logger.Info("init db called")

		godotenv.Load(".env")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		db, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
		if err != nil {
			logger.Errorf("Unable to connect to database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close(ctx)

		pkg.CreateTable(ctx, db, "checkpoint_sizes", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, size FLOAT, checkpoint_type TEXT")
		pkg.CreateTable(ctx, db, "checkpoint_times", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")
		pkg.CreateTable(ctx, db, "restore_times", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")
		pkg.CreateTable(ctx, db, "docker_sizes", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")
		pkg.CreateTable(ctx, db, "total_times", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
