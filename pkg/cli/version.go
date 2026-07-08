package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "打印版本号",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}
