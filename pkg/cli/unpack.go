package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/pipeline"
)

var unpackFlags struct {
	output     string
	decodeCP   uint32
	encodeCP   uint32
}

var unpackCmd = &cobra.Command{
	Use:   "unpack <arc/dir>...",
	Short: "解包 ARC 档案并转换其中资源",
	Long: `递归收集输入路径中的 .arc 文件，解包到输出目录，
并自动将 CBG 图片转为 PNG、DSC 脚本提取为 .hazuki.txt、BGI 音频转为 OGG。`,
	Example: `  hazuki unpack game.arc
  hazuki unpack ./data -o ./output --decode-cp 932 --encode-cp 932`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := pipeline.Options{
			UnpackRoot:     unpackFlags.output,
			DecodeCodepage: unpackFlags.decodeCP,
			EncodeCodepage: unpackFlags.encodeCP,
		}
		cb := pipeline.Callbacks{
			OnLog: func(line string) {
				fmt.Fprintln(os.Stderr, line)
			},
			OnProgress: func(p pipeline.Progress) {
				fmt.Fprintf(os.Stderr, "\r[%d/%d] %s", p.Completed, p.Total, p.CurrentItem)
				if p.Completed >= p.Total {
					fmt.Fprintln(os.Stderr)
				}
			},
		}
		result, err := pipeline.RunFullUnpackPipeline(args, opts, cb)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "\n结果: ARC=%d, extracted=%d, processed=%d, converted=%d, skipped=%d\n",
			result.ArchiveCount, result.ExtractedCount, result.ProcessedCount, result.ConvertedCount, result.SkippedCount)
		return nil
	},
}

func init() {
	unpackCmd.Flags().StringVarP(&unpackFlags.output, "output", "o", "", "输出根目录（默认 ./unpack）")
	unpackCmd.Flags().Uint32Var(&unpackFlags.decodeCP, "decode-cp", 932, "解码代码页")
	unpackCmd.Flags().Uint32Var(&unpackFlags.encodeCP, "encode-cp", 932, "编码代码页")
}
