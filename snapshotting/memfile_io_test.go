package snapshotting

import (
	"crypto/rand"
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


const (
	fileSize				= 512 * 1024 * 1024 // in Bytes, size of mem_file to create
	chunking                = true
	memFileOptimizationMode = true	// use optimized download and upload

	baseFolder              = "./tmp_test"
	revision				= "test-revision"
	bufferSize 				= 4 * 1024 * 1024 // in Bytes
	imageName               = "image-name-sample"
	lazyMode                = false		// don't change this
	wsPulling               = false		// don't change this
	snapshotsBucket         = "memfile-io-test-bucket"
	minioAddr               = "10.0.1.1:9000"
	minioAccessKey          = "minio"
	minioSecretKey          = "minio123"
)


func CreateRandomMemFile() {
	mem_file_path := filepath.Join(baseFolder, revision, "mem_file")
	log.Printf("mem file path to create: %s", mem_file_path)
	file, err := os.Create(mem_file_path)
	if err != nil {
		log.Fatalf("failed to create file: %v", err)
	}
	defer file.Close()
	buf := make([]byte, bufferSize)
	var written int64

	for written < fileSize {
		toWrite := bufferSize
		if remaining := fileSize - written; int64(toWrite) > remaining {
			toWrite = int(remaining)
		}

		_, err := rand.Read(buf[:toWrite])
		if err != nil {
			log.Fatalf("failed to generate random data: %v", err)
		}

		_, err = file.Write(buf[:toWrite])
		if err != nil {
			log.Fatalf("failed to write to file: %v", err)
		}

		written += int64(toWrite)
	}

	log.Printf("%vB file created at %s", fileSize, mem_file_path)
}

func uploadTest(objectStore storage.ObjectStorage, t *testing.T) {
	mgr := NewSnapshotManager(baseFolder, objectStore, chunking, true, lazyMode, wsPulling)
	mgr.SetMemFileOptimizationMode(memFileOptimizationMode)

	snap, err := mgr.InitSnapshot(revision, imageName)
	if err != nil {
		t.Fatalf("failed to InitSnapshot: %v", err)
	}

	err = mgr.CommitSnapshot(revision)
	if err != nil {
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

	snap, err := mgr.InitSnapshot(revision, imageName)
	if err != nil {
		t.Fatalf("failed to InitSnapshot: %v", err)
	}

	err = mgr.CommitSnapshot(revision)
	if err != nil {
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
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	fmt.Println("=== Memory File Upload/Download Test ===")

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
