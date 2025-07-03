package services

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

// UploadVideo handles multipart uploads of video files to MinIO.
func (streaming *Streaming) UploadVideo(c *gin.Context) {
	// Max upload size of, say, 100 MB. Adjust as you like.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 100<<20)

	// Read the file part from the form ("file" is the field name)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read file: " + err.Error()})
		return
	}
	defer file.Close()

	objectName := header.Filename
	fileSize := header.Size
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Upload to MinIO
	info, err := streaming.PutObject(
		context.Background(),
		bucketName,
		objectName,
		file,
		fileSize,
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		log.Printf("Failed to upload %s: %v\n", objectName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "upload successful",
		"objectName":  info.Key,
		"size":        info.Size,
		"contentType": contentType,
		"uploadTime":  info.LastModified,
	})
}
