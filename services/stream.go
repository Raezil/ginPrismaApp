package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	minioEndpoint     = "localhost:9000"
	useSSL            = false
	bucketName        = "videos"
	defaultBufferSize = 1024 * 1024
)

type Streaming struct {
	*minio.Client
}

func parseRange(rangeHeader string, fileSize int64) (int64, int64, error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range prefix")
	}

	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	if strings.Contains(rangeSpec, ",") {
		return 0, 0, fmt.Errorf("multiple ranges not supported")
	}

	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	var start, end int64
	var err error

	if parts[0] == "" {
		suffixLength, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		if suffixLength > fileSize {
			suffixLength = fileSize
		}
		start = fileSize - suffixLength
		end = fileSize - 1
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		if parts[1] != "" {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return 0, 0, err
			}
			if end >= fileSize {
				end = fileSize - 1
			}
		} else {
			end = fileSize - 1
		}
	}

	if start > end || start < 0 || end >= fileSize {
		return 0, 0, fmt.Errorf("invalid range values")
	}

	return start, end, nil
}

func NewMinioClient() (*minio.Client, error) {
	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	if accessKey == "" {
		log.Fatalln("Missing MINIO_ACCESS_KEY environment variable")
	}
	secretKey := os.Getenv("MINIO_SECRET_KEY")
	if secretKey == "" {
		log.Fatalln("Missing MINIO_SECRET_KEY environment variable")
	}

	minioClient, err := minio.New(minioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatalln("Error initializing MinIO client:", err)
	}
	return minioClient, nil
}
func NewStreaming() *Streaming {
	// Read MinIO credentials from environment variables
	minioClient, err := NewMinioClient()
	if err != nil {
		log.Fatalf("Failed to create MinIO client: %v", err)
	}
	return &Streaming{
		Client: minioClient,
	}
}

func (streaming *Streaming) GetObjectInfo(w http.ResponseWriter, objectName string) (*minio.ObjectInfo, error) {
	objectInfo, err := streaming.StatObject(context.Background(), bucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		log.Printf("Error getting object info for '%s': %v\n", objectName, err)
		return nil, err
	}
	return &objectInfo, nil
}

func (streaming *Streaming) Get(w http.ResponseWriter, objectName string) *minio.Object {
	object, err := streaming.GetObject(context.Background(), bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		http.Error(w, "Failed to get object", http.StatusInternalServerError)
		log.Printf("Error getting object '%s': %v\n", objectName, err)
		return nil
	}
	return object
}
func (streaming *Streaming) Stream(w http.ResponseWriter, r *http.Request) {
	objectName := r.FormValue("objectName")
	if objectName == "" {
		http.Error(w, "Missing 'objectName' parameter", http.StatusBadRequest)
		return
	}

	objectInfo, err := streaming.GetObjectInfo(w, objectName)
	if err != nil || objectInfo == nil {
		http.Error(w, "Failed to retrieve object info", http.StatusInternalServerError)
		return
	}

	fileSize := objectInfo.Size
	w.Header().Set("Accept-Ranges", "bytes")

	rangeHeader := r.Header.Get("Range")

	if rangeHeader == "" {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
		w.WriteHeader(http.StatusOK)

		object := streaming.Get(w, objectName)
		defer object.Close()

		if _, err := io.Copy(w, object); err != nil {
			log.Printf("Error streaming object '%s': %v\n", objectName, err)
		}
		return
	}

	start, end, err := parseRange(rangeHeader, fileSize)
	if err != nil {
		http.Error(w, "Invalid Range header", http.StatusBadRequest)
		log.Printf("Error parsing range '%s': %v\n", rangeHeader, err)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	w.WriteHeader(http.StatusPartialContent)

	streaming.ReadBuffer(objectName, w, start, end)

}

func (streaming *Streaming) ReadBuffer(objectName string, w http.ResponseWriter, start int64, end int64) {
	getOpts := minio.GetObjectOptions{}
	getOpts.SetRange(start, end)
	object := streaming.Get(w, objectName)
	defer object.Close()
	buffer := make([]byte, defaultBufferSize)
	for {
		n, err := object.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				log.Printf("Error writing to response for object '%s': %v\n", objectName, writeErr)
				break
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading object '%s': %v\n", objectName, err)
			}
			break
		}
	}
}

func (streaming *Streaming) VideoHandler(w http.ResponseWriter, r *http.Request) {
	streaming.Stream(w, r)
}
