package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ankorstore/mq-lease-service/internal/server"
	"github.com/ankorstore/mq-lease-service/internal/version"
	"github.com/ankorstore/mq-lease-service/pkg/util/logger"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func init() {
	serverCmd.Flags().Uint("port", 8080, "server listening port")
	serverCmd.Flags().String("config", "./config.yaml", "Configuration path")
	serverCmd.Flags().String("data", "./data", "Persistent state directory")
	serverCmd.Flags().Bool("log-debug", false, "Enable debug logging")
	serverCmd.Flags().Bool("log-json", true, "Enable JSON format logging")

	rootCmd.AddCommand(serverCmd)
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts lease server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		serverPort, _ := cmd.Flags().GetUint("port")
		configPath, _ := cmd.Flags().GetString("config")
		logDebug, _ := cmd.Flags().GetBool("log-debug")
		logJSON, _ := cmd.Flags().GetBool("log-json")
		persistentStateDir, _ := cmd.Flags().GetString("data")

		// Logger
		log := logger.New(logger.NewOpts{
			AppInfo: version.Version{},
			Debug:   logDebug,
			JSON:    logJSON,
		})
		ctx := log.WithContext(cmd.Context())

		// Main server
		srv := server.New(server.NewOpts{
			Port:               int(serverPort),
			ConfigPath:         configPath,
			PersistentStateDir: persistentStateDir,
		})

		grp, runCtx := errgroup.WithContext(ctx)

		// Signal handling (SIGTERM) to be able to gracefully shut down the server (both fiber + other resources, like the storage for ex).
		grp.Go(func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)
			select {
			case <-c:
				log.Info().Msg("Received termination signal, shutting down")
			case <-runCtx.Done():
			}
			return context.Canceled
		})
		grp.Go(func() error {
			return srv.Run(runCtx)
		})

		return grp.Wait()
	},
}
