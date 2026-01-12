package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "flyio-machine",
	Short: "Fly.io Platform Machines - Container image management",
	Long:  `Manages container images with FSM orchestration, S3 storage, and vulnerability scanning.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().String("sqlite-path", ".artifacts/images.db", "SQLite database path")
	rootCmd.PersistentFlags().String("fsm-db-path", ".artifacts/fsm.db", "FSM BoltDB path")
	rootCmd.PersistentFlags().String("s3-bucket", "flyio-platform-hiring-challenge", "S3 bucket name")
	rootCmd.PersistentFlags().String("s3-region", "us-east-1", "S3 region")
	rootCmd.PersistentFlags().Int64("max-file-size", 2*1024*1024*1024, "Max file size in bytes")
	rootCmd.PersistentFlags().Int64("max-total-size", 20*1024*1024*1024, "Max total extraction size")
	rootCmd.PersistentFlags().Float64("max-compression-ratio", 100.0, "Max compression ratio")

	viper.BindPFlag("sqlite-path", rootCmd.PersistentFlags().Lookup("sqlite-path"))
	viper.BindPFlag("fsm-db-path", rootCmd.PersistentFlags().Lookup("fsm-db-path"))
	viper.BindPFlag("s3-bucket", rootCmd.PersistentFlags().Lookup("s3-bucket"))
	viper.BindPFlag("s3-region", rootCmd.PersistentFlags().Lookup("s3-region"))
	viper.BindPFlag("max-file-size", rootCmd.PersistentFlags().Lookup("max-file-size"))
	viper.BindPFlag("max-total-size", rootCmd.PersistentFlags().Lookup("max-total-size"))
	viper.BindPFlag("max-compression-ratio", rootCmd.PersistentFlags().Lookup("max-compression-ratio"))
}
