package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/asset"
	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/cbg"
)

var imageCmd = &cobra.Command{
	Use:   "img <png/cbg/dir>...",
	Short: "在 PNG 与 CBG 图片格式间转换",
	Long: `将 .png 编码为 .cbg，或将 .cbg 解码为 .png。
目录会被递归处理。`,
	Example: `  hazuki img image.cbg
  hazuki img image.png
  hazuki img ./cg`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, input := range args {
			if err := processImage(input); err != nil {
				return err
			}
		}
		return nil
	},
}

func processImage(input string) error {
	info, err := os.Stat(input)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(input, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			return convertImage(path)
		})
	}
	return convertImage(input)
}

func convertImage(path string) error {
	ext := strings.ToLower(filepath.Ext(path))

	data, info, err := asset.ReadAssetData(path)
	if err != nil {
		return fmt.Errorf("img: failed to read %s: %w", path, err)
	}

	switch info.Kind {
	case asset.CbgImage:
		img, err := cbg.DecodeCbgBytes(data)
		if err != nil {
			return fmt.Errorf("img: failed to decode %s: %w", path, err)
		}
		var outPath string
		if ext == ".cbg" {
			outPath = strings.TrimSuffix(path, ext) + ".png"
		} else {
			outPath = path + ".png"
		}
		if err := cbg.SavePng(img, outPath); err != nil {
			return fmt.Errorf("img: failed to save %s: %w", outPath, err)
		}
		fmt.Printf("%s -> %s\n", path, outPath)
		return nil

	case asset.PngImage:
		img, err := cbg.LoadPng(path)
		if err != nil {
			return fmt.Errorf("img: failed to load %s: %w", path, err)
		}
		outPath := strings.TrimSuffix(path, ext) + ".cbg"
		if err := cbg.EncodeCbg(img, outPath); err != nil {
			return fmt.Errorf("img: failed to encode %s: %w", outPath, err)
		}
		fmt.Printf("%s -> %s\n", path, outPath)
		return nil

	default:
		fmt.Fprintf(os.Stderr, "[SKIP] %s -> not a PNG/CBG file (%s)\n", path, info.Label)
		return nil
	}
}
