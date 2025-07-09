package downloader

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sync"
	"time"
)

type Downloader struct {
	Url        string          // 下载地址
	OutputPath string          // 输出文件路径
	ChunkSize  int64           // 分块大小
	Concurrent int             // 并发数
	mutex      sync.Mutex      // 互斥锁
	Ctx        context.Context // 上下文
	CancelFunc func()          // 取消函数
}

type Task struct {
	FileSize     int64   // 文件大小
	AcceptRanges bool    // 是否支持断点续传
	Chunks       []Chunk // 分块信息
}

type Chunk struct {
	Index int
	Start int64
	End   int64
}

func NewDownloader(url string, outputPath string) *Downloader {
	return &Downloader{
		Url:        url,
		OutputPath: outputPath,
		ChunkSize:  5 * 1024 * 1024,
		Concurrent: 5,
	}
}

func (d *Downloader) Run(ctx context.Context) error {
	d.Ctx, d.CancelFunc = context.WithCancel(ctx)

	// 解析下载任务
	task, err := d.newTask()
	if err != nil {
		return fmt.Errorf("初始化下载任务错误: %s", err)
	}

	// 创建目标文件
	file, err := os.OpenFile(d.OutputPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("创建文件失败: %s", err)
	}
	defer file.Close()

	// 并发下载
	var taskChan = make(chan Chunk, d.Concurrent)
	var errorChan = make(chan error, d.Concurrent)

	var wg sync.WaitGroup
	wg.Add(d.Concurrent)
	for i := 0; i < d.Concurrent; i++ {
		go d.consume(file, taskChan, errorChan, &wg)
	}

	go func() {
		for _, chunk := range task.Chunks {
			taskChan <- chunk
		}
		close(taskChan)
	}()

	wg.Wait()
	close(errorChan)
	select {
	case err = <-errorChan:
		return err
	default:
		return nil
	}
}

// 创建下载任务（保持不变）
func (d *Downloader) newTask() (*Task, error) {
	req, err := http.NewRequest(http.MethodGet, d.Url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: time.Second * 10}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status err: %s", resp.Status)
	}

	fileSize := resp.ContentLength
	if fileSize <= 0 {
		return nil, fmt.Errorf("错误的文件大小")
	}

	chunks := d.splitFile(fileSize)
	acceptRanges := resp.Header.Get("Accept-Ranges") == "bytes"
	return &Task{
		FileSize:     resp.ContentLength,
		AcceptRanges: acceptRanges,
		Chunks:       chunks,
	}, nil
}

// 分割文件（保持不变）
func (d *Downloader) splitFile(fileSize int64) []Chunk {
	numChunks := int(math.Ceil(float64(fileSize) / float64(d.ChunkSize)))
	chunks := make([]Chunk, numChunks)
	for i := 0; i < numChunks; i++ {
		start := int64(i) * d.ChunkSize
		end := start + d.ChunkSize - 1
		if end >= fileSize {
			end = fileSize - 1
		}
		chunks[i] = Chunk{
			Index: i,
			Start: start,
			End:   end,
		}
	}
	return chunks
}

func (d *Downloader) consume(file *os.File, chanChunk chan Chunk, errorChan chan error, wg *sync.WaitGroup) {
	defer wg.Done()
	for chunk := range chanChunk {
		select {
		case <-d.Ctx.Done():
			return
		default:
			if err := d.downloadChunk(file, &chunk); err != nil {
				errorChan <- err
				d.CancelFunc()
				return
			}
		}
	}
}

// 下载分片（核心修改）
func (d *Downloader) downloadChunk(file *os.File, chunk *Chunk) error {
	req, err := http.NewRequestWithContext(d.Ctx, http.MethodGet, d.Url, nil)
	if err != nil {
		return err
	}

	// 设置Range请求头
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", chunk.Start, chunk.End))
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	n, err := file.WriteAt(data, chunk.Start)
	if err != nil {
		return err
	}

	// 验证写入长度
	expected := chunk.End - chunk.Start + 1
	if int64(n) != expected {
		return fmt.Errorf("分片 %d 写入长度错误: 期望 %d, 实际 %d", chunk.Index, expected, n)
	}
	return nil
}
