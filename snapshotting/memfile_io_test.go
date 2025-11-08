package snapshotting

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vhive-serverless/vhive/storage"
)

var (
	fileSize               int64
	customChunkSize        int
	memFileOptimizationMode bool
	memLoadWorkerCountArg     int
)

const (
	chunking        = true
	baseFolder       = "./tmp_test"
	revision         = "test-revision"
	bufferSize       = 4 * 1024 * 1024 // in Bytes
	imageName        = "image-name-sample"
	lazyMode         = false
	wsPulling        = false
	snapshotsBucket  = "memfile-io-test-bucket"
	minioAddr        = "10.0.1.1:9000"
	minioAccessKey   = "minio"
	minioSecretKey   = "minio123"
)

func init() {
	flag.Int64Var(&fileSize, "fileSize", 512, "size of mem_file in MegaBytes")
	flag.IntVar(&customChunkSize, "chunkSize", 512, "custom chunk size in KiboBytes")
	flag.BoolVar(&memFileOptimizationMode, "memOpt", false, "enable memfile optimization")
	flag.IntVar(&memLoadWorkerCountArg, "workerCount", 8, "number of workers for memfile upload/download")
}

func CreateRandomMemFile() {
	memFilePath := filepath.Join(baseFolder, revision, "mem_file")
	log.Printf("mem file path to create: %s", memFilePath)
	file, err := os.Create(memFilePath)
	if err != nil {
		log.Fatalf("failed to create file: %v", err)
	}
	defer file.Close()

	buf := make([]byte, bufferSize)
	var written int64

	fileSize = fileSize * 1024 * 1024

	for written < fileSize {
		toWrite := bufferSize
		if remaining := fileSize - written; int64(toWrite) > remaining {
			toWrite = int(remaining)
		}

		if _, err := rand.Read(buf[:toWrite]); err != nil {
			log.Fatalf("failed to generate random data: %v", err)
		}

		if _, err := file.Write(buf[:toWrite]); err != nil {
			log.Fatalf("failed to write to file: %v", err)
		}

		written += int64(toWrite)
	}

	log.Printf("%vB file created at %s", fileSize, memFilePath)
}

func uploadTest(objectStore storage.ObjectStorage, t *testing.T) {
	mgr := NewSnapshotManager(baseFolder, objectStore, chunking, true, lazyMode, wsPulling)
	mgr.SetMemFileOptimizationMode(memFileOptimizationMode)
	mgr.SetCustomChunkSize(customChunkSize * 1024)
	mgr.SetMemLoadWorkerCount(memLoadWorkerCountArg)

	snap, err := mgr.InitSnapshot(revision, imageName)
	if err != nil {
		t.Fatalf("failed to InitSnapshot: %v", err)
	}

	if err := mgr.CommitSnapshot(revision); err != nil {
		t.Fatalf("failed to CommitSnapshot: %v", err)
	}

	fmt.Println("Starting upload test...")
	start := time.Now()
	if err := mgr.uploadMemFile(snap); err != nil {
		t.Fatalf("uploadMemFile failed: %v", err)
	}
	fmt.Printf("Upload completed in %s\n", time.Since(start))
}

func downloadTest(objectStore storage.ObjectStorage, t *testing.T) {
	mgr := NewSnapshotManager(baseFolder, objectStore, chunking, true, lazyMode, wsPulling)
	mgr.SetMemFileOptimizationMode(memFileOptimizationMode)
	mgr.SetCustomChunkSize(customChunkSize * 1024)
	mgr.SetMemLoadWorkerCount(memLoadWorkerCountArg)

	snap, err := mgr.InitSnapshot(revision, imageName)
	if err != nil {
		t.Fatalf("failed to InitSnapshot: %v", err)
	}

	if err := mgr.CommitSnapshot(revision); err != nil {
		t.Fatalf("failed to CommitSnapshot: %v", err)
	}

	fmt.Println("Starting download test...")
	start := time.Now()
	if err := mgr.downloadMemFile(snap); err != nil {
		t.Fatalf("downloadMemFile failed: %v", err)
	}
	fmt.Printf("Download completed in %s\n", time.Since(start))
}

func TestMemFileIO(t *testing.T) {
	flag.Parse() // parse command line flags
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	fmt.Println("=== Memory File Upload/Download Test ===")

	chunkCount := (fileSize * 1024 * 1024) / (int64(customChunkSize) * 1024)
	if chunkCount > 64000 {
		t.Error("Too many chunks (>64000)! You will hit FS link cap!")
	}

	revisionDir := filepath.Join(baseFolder, revision)
	if err := os.MkdirAll(revisionDir, 0777); err != nil {
		t.Fatalf("creating base folder: %v", err)
	}
	defer os.RemoveAll(baseFolder)

	CreateRandomMemFile()

	minioClient, _ := minio.New(minioAddr, &minio.Options{
		Creds:  credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
		Secure: false,
	})

	objectStore, err := storage.NewMinioStorage(minioClient, snapshotsBucket)
	if err != nil {
		t.Fatalf("failed to create MinIO storage: %v", err)
	}

	uploadTest(objectStore, t)

	os.RemoveAll(baseFolder)
	if err := os.MkdirAll(revisionDir, 0777); err != nil {
		t.Fatalf("creating base folder: %v", err)
	}

	downloadTest(objectStore, t)
}