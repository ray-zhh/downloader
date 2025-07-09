package downloader

import (
	"context"
	"testing"
)

func TestDownloader_Run(t *testing.T) {
	url := "https://hf-mirror.com/Kijai/MoGe_safetensors/resolve/main/MoGe_ViT_L_fp16.safetensors"
	outputPath := "C:\\Users\\Administrator\\Desktop\\test\\MoGe_ViT_L_fp16.safetensors"

	downloader := NewDownloader(url, outputPath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := downloader.Run(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("download success")
}
