package uploader

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Config struct {
	Endpoint      string
	Bucket        string
	AccessKeyID   string
	SecretKey     string
	Region        string
	UsePathStyle  bool
	Workers       int
	QueueSize     int
	RecordingDir  string
}

type Uploader struct {
	client *s3.Client
	bucket string
	workers int

	pathCh chan string
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	recordingDir string
}

func NewUploader(cfg Config) (*Uploader, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = 10
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 500
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID, cfg.SecretKey, "",
		)),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})

	ctx, cancel := context.WithCancel(context.Background())

	return &Uploader{
		client:       client,
		bucket:       cfg.Bucket,
		workers:      cfg.Workers,
		pathCh:       make(chan string, cfg.QueueSize),
		ctx:          ctx,
		cancel:       cancel,
		recordingDir: cfg.RecordingDir,
	}, nil
}

func (u *Uploader) Channel() chan<- string {
	return u.pathCh
}

func (u *Uploader) Start() error {
	if err := u.ensureBucket(); err != nil {
		return err
	}

	for i := 0; i < u.workers; i++ {
		u.wg.Add(1)
		go u.worker(i)
	}

	log.Printf("uploader: %d workers started, bucket=%s", u.workers, u.bucket)
	return nil
}

func (u *Uploader) Stop() {
	close(u.pathCh)
	u.wg.Wait()
	u.cancel()
	log.Println("uploader: stopped")
}

func (u *Uploader) ensureBucket() error {
	_, err := u.client.HeadBucket(u.ctx, &s3.HeadBucketInput{
		Bucket: &u.bucket,
	})
	if err == nil {
		return nil
	}

	log.Printf("uploader: bucket %s not found, creating...", u.bucket)
	_, err = u.client.CreateBucket(u.ctx, &s3.CreateBucketInput{
		Bucket: &u.bucket,
	})
	if err != nil {
		return err
	}

	log.Printf("uploader: bucket %s created", u.bucket)
	return nil
}

func (u *Uploader) worker(id int) {
	defer u.wg.Done()
	for path := range u.pathCh {
		u.upload(path)
	}
}

func (u *Uploader) upload(localPath string) {
	data, err := os.ReadFile(localPath)
	if err != nil {
		log.Printf("uploader: read %s: %v", localPath, err)
		return
	}

	hash := md5.Sum(data)
	md5b64 := base64.StdEncoding.EncodeToString(hash[:])
	md5hex := hex.EncodeToString(hash[:])

	key := u.s3Key(localPath)

	backoff := time.Second
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		result, err := u.client.PutObject(u.ctx, &s3.PutObjectInput{
			Bucket:     &u.bucket,
			Key:        &key,
			Body:       bytes.NewReader(data),
			ContentMD5: &md5b64,
		})
		if err != nil {
			lastErr = err
			continue
		}

		etag := aws.ToString(result.ETag)
		etag = strings.Trim(etag, `"`)
		if etag != "" && !strings.EqualFold(etag, md5hex) {
			lastErr = fmt.Errorf("ETag mismatch: got %s, want %s", etag, md5hex)
			continue
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		log.Printf("uploader: failed %s after 3 attempts: %v", key, lastErr)
		return
	}

	if err := os.Remove(localPath); err != nil {
		log.Printf("uploader: remove %s: %v", localPath, err)
		return
	}

	log.Printf("uploader: uploaded %s (%d bytes)", key, len(data))
}

func (u *Uploader) s3Key(localPath string) string {
	base := u.recordingDir
	if base != "" && !strings.HasSuffix(base, "/") {
		base += "/"
	}

	if base != "" {
		localPath = strings.TrimPrefix(localPath, base)
	}

	parts := strings.SplitN(localPath, string(filepath.Separator), 2)
	if len(parts) != 2 {
		return localPath
	}

	cameraID := parts[0]
	filename := parts[1]

	if len(filename) == 17 && filename[8] == 'T' {
		dateStr := filename[:8]
		timeStr := filename[9:15]
		return cameraID + "/" + dateStr[:4] + "/" + dateStr[4:6] + "/" + dateStr[6:8] + "/" + timeStr + ".mp4"
	}

	return cameraID + "/" + filename
}
